// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

//go:build linux || darwin

package driver

import (
	"context"
	"fmt"
	"os"
	"syscall"

	"github.com/regalium-os/osdp-go/internal/transport"
)

// DialSerial opens an RS-485 or RS-232 serial port.
//
//	port, err := driver.DialSerial(ctx, "/dev/ttyUSB0", driver.WithBaud(9600))
//
// # Why this is built on os.File
//
// The runtime's network poller accepts a serial descriptor opened non-blocking,
// which means SetReadDeadline and SetWriteDeadline are real: a blocked read is
// interrupted by the deadline rather than by a timer racing a syscall, and the
// poller parks the goroutine instead of spinning. That is the whole reason a
// panel can be cancelled while waiting on a silent line without burning a core
// on an idle bus. It was established by experiment rather than assumed.
//
// # Line settings
//
// 8N1, no flow control, no modem-control lines, and no input or output
// processing whatsoever. OSDP is a binary protocol on a shared wire: a tty
// layer that translated a carriage return, echoed input, or acted on a 0x03
// would corrupt frames in ways that look like line noise. Raw mode is not a
// preference here, it is a correctness requirement.
//
// # Half duplex
//
// RS-485 is a shared pair, so a transceiver has to stop driving before a device
// can answer. Most USB adapters switch direction automatically in hardware;
// those that need a GPIO or RTS toggle are not handled here, and a converter
// needing one will drop replies. transport.Line.Turnaround is the panel's
// side of the same problem.
//
// # Concurrency
//
// Safe for one reader and one writer concurrently, which is how a bus uses it.
// Close may be called from any goroutine.
//
// ctx bounds opening the port, not its lifetime: once open the port outlives
// the context, and Close is what ends it.
func DialSerial(ctx context.Context, device string, opts ...SerialOption) (transport.Port, error) {
	cfg := defaultSerialConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// O_NOCTTY: opening a tty must not make it this process's controlling
	// terminal, or a hangup on the line would deliver SIGHUP to the panel.
	// O_NONBLOCK is what makes the descriptor pollable, and with it the open
	// itself does not wait for carrier detect -- which an RS-485 adapter will
	// never assert.
	// #nosec G304 -- opening the serial device the operator configured is what
	// this function is for; the path is a deployment setting, not untrusted
	// input, and there is no safe-list of device names to check it against.
	file, err := os.OpenFile(device, os.O_RDWR|syscall.O_NOCTTY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", device, err)
	}

	if err := configureTermios(file, cfg.baud); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("configuring %s: %w", device, err)
	}

	return &serialPort{file: file, name: device}, nil
}

// configureTermios puts the port into raw mode at the requested speed.
//
// The two ioctls differ by platform in both number and structure layout, so
// each supplies its own get and set; everything above this line is shared.
func configureTermios(file *os.File, baud int) error {
	t, err := getTermios(file)
	if err != nil {
		return err
	}

	rawTermios(t)
	if err := setSpeed(t, baud); err != nil {
		return err
	}
	return setTermios(file, t)
}
