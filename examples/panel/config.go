// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/regalium-os/osdp-go"
)

// What the operator asked for. Driving the line is main.go.

// config is what the flags produced.
type config struct {
	addr       string
	devices    []osdp.Address
	baud       int
	timeout    time.Duration
	turnaround time.Duration
	trace      bool
	demo       bool
	key        osdp.BaseKey
	secure     bool
	unlock     time.Duration
}

func parseFlags() config {
	var (
		addr       = flag.String("addr", "", "TCP address of a serial converter, e.g. converter:4001")
		devices    = flag.String("devices", "0", "comma-separated device addresses to poll")
		baud       = flag.Int("baud", 9600, "line speed, for the trace only")
		timeout    = flag.Duration("timeout", 200*time.Millisecond, "how long a device has to answer")
		turnaround = flag.Duration("turnaround", 10*time.Millisecond,
			"pause after transmitting, before expecting a reply")
		trace  = flag.Bool("trace", false, "print every octet in both directions")
		demo   = flag.Bool("demo", false, "run against a simulated reader, with no hardware")
		key    = flag.String("key", "", "32 hex characters: the SCBK to attempt a secure channel with")
		unlock = flag.Duration("unlock", 0, "release output 0 for this long when a card is read")
	)
	flag.Parse()

	if *addr == "" && !*demo {
		fmt.Fprintln(os.Stderr, "panel: give -addr, or -demo to run without hardware")
		flag.Usage()
		os.Exit(2)
	}

	cfg := config{
		addr:       *addr,
		devices:    parseAddresses(*devices),
		baud:       *baud,
		timeout:    *timeout,
		turnaround: *turnaround,
		trace:      *trace,
		demo:       *demo,
		unlock:     *unlock,
	}

	if *key != "" {
		raw, err := hex.DecodeString(*key)
		if err != nil || len(raw) != len(cfg.key) {
			fmt.Fprintf(os.Stderr, "panel: -key must be %d hex characters\n", len(cfg.key)*2)
			os.Exit(2)
		}
		copy(cfg.key[:], raw)
		cfg.secure = true
	}
	return cfg
}

// parseAddresses reads the -devices list.
func parseAddresses(list string) []osdp.Address {
	var out []osdp.Address
	for _, field := range strings.Split(list, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		n, err := strconv.ParseUint(field, 0, 8)
		if err != nil {
			fmt.Fprintf(os.Stderr, "panel: %q is not a device address\n", field)
			os.Exit(2)
		}
		out = append(out, osdp.Address(n))
	}
	return out
}
