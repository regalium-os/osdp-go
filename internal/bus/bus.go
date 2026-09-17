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

// compose turns a chosen command into the frame that carries it.
//
// The ordering is fixed by the specification and each step depends on the one
// before: the payload is enciphered first, because its length decides the
// frame's; then the frame is assembled so the length field is correct; then the
// message authentication code is computed over those octets; and only then is
// the error check written over everything. Doing any two of these in the other
// order produces a frame that will not authenticate.
func (b *Bus) compose(
	ctx context.Context, d *Device, msg cmd.Message, block *frame.SecurityBlock,
) (frame.Frame, error) {
	if block == nil {
		return cmd.Encode(ctx, msg, d.Address, d.nextSequence(), b.scheme)
	}

	kind := secure.BlockType(block.Type)
	if kind.Encrypted() {
		ciphertext, err := d.session.Seal(msg.Data, true)
		if err != nil {
			return frame.Frame{}, err
		}
		msg.Data = ciphertext
	}

	f, err := cmd.EncodeSecure(ctx, msg, d.Address, d.nextSequence(), b.scheme, *block)
	if err != nil {
		return frame.Frame{}, err
	}

	if kind.Established() {
		if err := authenticate(d.session, &f, true); err != nil {
			return frame.Frame{}, err
		}
	}

	f.Seal()
	return f, nil
}

// commandFor chooses what to ask a device, based on what is known about it,
// and the security block the command travels under if any.
//
// An offline or freshly returned device is asked what it is before anything
// else, then what it can do: a panel that starts issuing commands to a reader
// it has not identified is guessing about capabilities it could simply have
// asked for. Everything after that is an ordinary poll, wrapped by the secure
// channel once there is one.
func (b *Bus) commandFor(d *Device) (cmd.Message, *frame.SecurityBlock) {
	switch d.State {
	case Offline:
		return cmd.Message{Code: cmd.ID, Data: []byte{0x00}}, nil

	case Identifying:
		return cmd.Message{Code: cmd.Cap, Data: []byte{0x00}}, nil

	case SecureHandshake:
		return b.handshakeCommand(d)

	case Secure:
		poll := cmd.Message{Code: cmd.Poll}
		block := blockForCommand(len(poll.Data))
		return poll, &block

	default:
		return cmd.Message{Code: cmd.Poll}, nil
	}
}
