// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

// Command panel drives an OSDP line and prints what happens on it.
//
// It is the tool to reach for when hardware is not behaving: it shows every
// step of enrolment, what each device says it is, and -- with -trace -- the
// octets in both directions. Most bring-up problems are visible in the first
// twenty lines of output.
//
//	# against a serial-to-Ethernet converter
//	go run ./examples/panel -addr converter:4001 -devices 0
//
//	# with no hardware at all, to check the tool itself works
//	go run ./examples/panel -demo
//
//	# everything on the wire, when a device is not answering
//	go run ./examples/panel -addr converter:4001 -devices 0,1 -trace
//
// There is no serial driver yet, which is why -addr takes a TCP address: a
// converter is the supported path to an RS-485 line today.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/regalium-os/osdp-go"
)

func main() {
	cfg := parseFlags()

	if err := run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "\npanel: %v\n", err)
		os.Exit(1)
	}
}

// run opens the line and drives it until interrupted.
func run(cfg config) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	port, err := openPort(ctx, cfg)
	if err != nil {
		return err
	}
	if cfg.trace {
		port = &tracingPort{Port: port}
	}

	line := osdp.Line{
		Name:         cfg.addr,
		Baud:         cfg.baud,
		ReplyTimeout: cfg.timeout,

		// On a half-duplex RS-485 line the panel must stop driving before the
		// device can answer, and reading too early reads back the tail of its
		// own transmission. It also paces the cycle: without it the bus polls
		// as fast as the port allows, which on a converter or an in-memory
		// pipe is as fast as the CPU allows.
		Turnaround: cfg.turnaround,
	}
	bus := osdp.NewBus(line, cfg.devices, osdp.SchemeCRC16, secureOption(cfg)...)

	p := osdp.NewPanel(bus, port)
	defer func() { _ = p.Close() }()

	announce(cfg)

	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()

	report(ctx, p, cfg)
	return <-done
}

// openPort connects to the converter, or stands up a simulated reader.
func openPort(ctx context.Context, cfg config) (osdp.Port, error) {
	if cfg.demo {
		panelSide, deviceSide := osdp.Pipe()
		go simulateReader(ctx, deviceSide, cfg)
		return panelSide, nil
	}

	port, err := osdp.DialTCP(ctx, cfg.addr)
	if err != nil {
		return nil, fmt.Errorf("dialling %s: %w", cfg.addr, err)
	}
	return port, nil
}

// secureOption builds the Secure Channel option, when a key was given.
func secureOption(cfg config) []osdp.BusOption {
	if !cfg.secure {
		return nil
	}
	return []osdp.BusOption{osdp.WithSecureChannel(
		osdp.AES128{},
		func(osdp.Address) (osdp.BaseKey, bool) { return cfg.key, true },
		randomNonce,
	)}
}

// announce prints what the panel is about to do, so a run that produces no
// events afterwards still says what it was trying.
func announce(cfg config) {
	where := cfg.addr
	if cfg.demo {
		where = "a simulated reader (-demo)"
	}

	fmt.Printf("polling %s\n", where)
	fmt.Printf("  devices  %v\n", cfg.devices)
	fmt.Printf("  timeout  %s per device\n", cfg.timeout)
	fmt.Printf("  pacing   %s turnaround\n", cfg.turnaround)
	if cfg.secure {
		fmt.Printf("  secure   AES-128, key supplied\n")
	}
	if cfg.unlock > 0 {
		fmt.Printf("  unlock   output 0 for %s on a card read\n", cfg.unlock)
	}
	fmt.Print("\nctrl-c to stop\n\n")
}
