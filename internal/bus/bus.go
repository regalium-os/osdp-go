// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package bus

import (
	"context"
	"time"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/internal/secure"
	"github.com/regalium-os/osdp-go/internal/transport"
	"github.com/regalium-os/osdp-go/telemetry"
)

// OfflineThreshold is how many consecutive unanswered polls mark a device
// offline. Three tolerates a burst of line noise without hiding a dead reader
// for long.
const OfflineThreshold = 3

// Bus sequences commands across the devices sharing one line.
//
// It is a state machine over events, not a loop over a port: Next says what
// should happen, and Reply or Timeout says what did. The driver decides when.
// That split is what lets a full online, offline and resynchronise scenario run
// as a table test with no hardware and no sleeping.
//
// A Bus is not safe for concurrent use. A line is inherently serial -- one
// device answers at a time -- so locking it would buy nothing.
type Bus struct {
	line    transport.Line
	devices []*Device
	current int
	scheme  frame.Scheme

	// Secure Channel configuration, all three nil unless WithSecureChannel was
	// supplied. They are read together and never individually; see
	// secureEnabled.
	suite secure.CipherSuite
	keys  KeyFor
	nonce NonceSource
}

// New returns a bus that will poll addrs in order.
//
// scheme selects the error check. Prefer frame.SchemeCRC16; the checksum exists
// for devices too old to manage a CRC.
//
// New starts nothing. It allocates device state and returns; a caller may
// construct a bus, inspect it and discard it without anything having happened
// on a line.
func New(line transport.Line, addrs []frame.Address, scheme frame.Scheme, opts ...Option) *Bus {
	devices := make([]*Device, 0, len(addrs))
	for _, a := range addrs {
		devices = append(devices, &Device{Address: a, State: Offline})
	}

	b := &Bus{line: line, devices: devices, scheme: scheme}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// Devices returns the devices on this bus, in poll order.
func (b *Bus) Devices() []*Device { return b.devices }

// Send queues a command for a device, to be sent when the cycle next reaches
// it.
//
// It is not safe to call concurrently with Next or Reply: a Bus drives one
// line, and a line is serial by nature. The runtime in package panel is what
// makes concurrent submission safe, by draining the application's requests on
// the same goroutine that drives the cycle.
//
// The message's payload is retained until the command is sent. Do not modify
// it afterwards.
//
// A device that is offline keeps its queued commands rather than losing them --
// see Device.Queued, which is how a caller notices a reader that is not coming
// back.
//
// It returns ErrMessageTooLarge when the command will not fit in the buffer the
// device reported in osdp_CAP. That is a refusal rather than a best effort: a
// device cannot reply to a frame it could not receive, so an oversized command
// would retry against a silence forever. See checkSize.
func (b *Bus) Send(d *Device, m cmd.Message) error {
	if err := b.checkSize(d, m); err != nil {
		return err
	}
	d.enqueue(m)
	return nil
}

// Line returns the physical parameters this bus was built for.
//
// The core does not act on them -- it has no idea what a baud rate is -- but
// the runtime driving the line does, and the turnaround in particular is not
// something it should be told twice and risk disagreeing about.
func (b *Bus) Line() transport.Line { return b.line }

// Step is one command the bus wants sent, and how long to wait for an answer.
type Step struct {
	// Device is the device being addressed.
	Device *Device

	// Frame is the sealed frame to transmit.
	Frame frame.Frame

	// Timeout is how long to wait for a reply before calling Timeout.
	Timeout time.Duration
}

// Next returns the command to send to the next device in the cycle.
//
// It advances the cycle on every call, so a device that does not answer does
// not stall the ones behind it. ok is false only when the bus has no devices.
func (b *Bus) Next(ctx context.Context) (Step, bool) {
	if len(b.devices) == 0 {
		return Step{}, false
	}

	d := b.devices[b.current]
	b.current = (b.current + 1) % len(b.devices)

	_, span := telemetry.Start(ctx, "osdp.bus.transaction", d.Trace())
	defer span.End()

	msg, block := b.commandFor(d)
	f, err := b.compose(ctx, d, msg, block)
	if err != nil {
		span.RecordError(err)
		return Step{}, false
	}

	return Step{Device: d, Frame: f, Timeout: b.line.ReplyTimeout}, true
}
