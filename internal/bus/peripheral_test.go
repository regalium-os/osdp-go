// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package bus_test

import (
	"context"
	"testing"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/internal/secure"
)

// peripheral is a peripheral device that actually holds a key.
//
// It builds its own frames rather than calling into the bus's helpers, and that
// is the point of it: the two ends of a secure channel must agree on exactly
// which octets the message authentication code covers, and an agreement between
// a function and itself proves nothing. Everything here follows SIA OSDP
// v2.2.2 §7 directly -- the length field reports the finished frame, and the
// code covers every octet before itself.
type peripheral struct {
	session *secure.Session
	key     secure.BaseKey
	uid     [8]byte
	rndB    [8]byte

	// badCryptogram makes the device answer the challenge with a proof that
	// does not verify, standing in for a reader holding a different key.
	badCryptogram bool

	// card, when set, is returned in place of an osdp_ACK once the session is
	// established, enciphered as a credential must be.
	card []byte

	// plainReader makes the device report no Secure Channel capability, which
	// is what most of the installed base of older readers does.
	plainReader bool

	// buffer overrides the receive buffer size this device advertises, for
	// tests about what fits in it. Zero means the default.
	buffer int

	// received records the plaintext of every established-session command the
	// device accepted, so a test can ask what actually arrived rather than
	// what was queued.
	received []cmd.Message
}

// bufferSize is what this device claims it can receive.
func (p *peripheral) bufferSize() int {
	if p.buffer > 0 {
		return p.buffer
	}
	return 128
}

// adopt replaces the device's base key and abandons the session derived from
// the old one, which is what a real device does on osdp_KEYSET.
func (p *peripheral) adopt(key secure.BaseKey) {
	p.key = key
	p.session = secure.NewSession(secure.AES128{}, secure.RolePD, key)
}

// newPeripheral returns a device holding key.
func newPeripheral(key secure.BaseKey) *peripheral {
	return &peripheral{
		session: secure.NewSession(secure.AES128{}, secure.RolePD, key),
		key:     key,
		uid:     [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
		rndB:    [8]byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88},
	}
}

// answer produces the device's reply to a command, going through the wire in
// both directions so that the error check covers the authenticated frame too.
func (p *peripheral) answer(t *testing.T, ctx context.Context, sent frame.Frame) frame.Frame {
	t.Helper()

	wire, err := sent.Append(ctx, nil)
	if err != nil {
		t.Fatalf("panel frame would not encode: %v", err)
	}
	f, err := frame.Decode(ctx, wire)
	if err != nil {
		t.Fatalf("panel frame would not decode: %v", err)
	}

	seq := f.Control.Sequence()
	if f.Security == nil {
		return p.plainAnswer(t, f, seq)
	}

	switch secure.BlockType(f.Security.Type) {
	case secure.SCS11:
		return p.answerChallenge(t, f, seq)
	case secure.SCS13:
		return p.answerCryptogram(t, f, seq)
	default:
		return p.answerSecurely(t, f, seq)
	}
}

// plainAnswer handles enrolment, which happens before there is a session.
func (p *peripheral) plainAnswer(t *testing.T, f frame.Frame, seq uint8) frame.Frame {
	t.Helper()

	switch cmd.Code(f.Code) {
	case cmd.ID:
		return reply(f.Address, seq, cmd.PDID, deviceIDPayload)
	case cmd.Cap:
		return reply(f.Address, seq, cmd.PDCap, p.capabilities())
	default:
		return reply(f.Address, seq, cmd.ACK, nil)
	}
}

// capabilities is what this device claims.
func (p *peripheral) capabilities() []byte {
	security := byte(0x01) // AES-128 supported
	if p.plainReader {
		security = 0x00
	}
	return []byte{
		0x01, 0x01, 0x04, // 4 monitored inputs
		0x04, 0x01, 0x02, // 2 LEDs per reader
		0x08, 0x01, 0x00, // CRC-16
		0x09, security, 0x00,
		0x0A, byte(p.bufferSize()), byte(p.bufferSize() >> 8),
		0x0D, 0x01, 0x01, // one reader
	}
}
