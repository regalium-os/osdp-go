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

	// received records the plaintext of every established-session command the
	// device accepted, so a test can ask what actually arrived rather than
	// what was queued.
	received []cmd.Message
}

// newPeripheral returns a device holding key.
func newPeripheral(key secure.BaseKey) *peripheral {
	return &peripheral{
		session: secure.NewSession(secure.AES128{}, secure.RolePD, key),
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
		0x0A, 0x80, 0x00, // receive buffer 128
		0x0D, 0x01, 0x01, // one reader
	}
}

// answerChallenge consumes osdp_CHLNG and returns osdp_CCRYPT under SCS_12.
func (p *peripheral) answerChallenge(t *testing.T, f frame.Frame, seq uint8) frame.Frame {
	t.Helper()

	ccrypt, err := p.session.AnswerChallenge(f.Data, p.rndB, p.uid)
	if err != nil {
		t.Fatalf("device could not answer the challenge: %v", err)
	}
	if p.badCryptogram {
		ccrypt[len(ccrypt)-1] ^= 0xFF
	}
	return handshakeReply(f.Address, seq, secure.SCS12, cmd.CCrypt, ccrypt)
}

// answerCryptogram consumes osdp_SCRYPT and returns osdp_RMAC_I under SCS_14.
func (p *peripheral) answerCryptogram(t *testing.T, f frame.Frame, seq uint8) frame.Frame {
	t.Helper()

	rmac, err := p.session.AnswerServerCryptogram(f.Data)
	if err != nil {
		t.Fatalf("device rejected the server cryptogram: %v", err)
	}
	return handshakeReply(f.Address, seq, secure.SCS14, cmd.RMACI, rmac)
}

// answerSecurely verifies an established-session command and answers in kind.
func (p *peripheral) answerSecurely(t *testing.T, f frame.Frame, seq uint8) frame.Frame {
	t.Helper()

	body := f.AppendBody(nil)
	split := len(f.Data) - secure.MACTagSize
	if split < 0 {
		t.Fatal("panel sent an established-session frame with no room for a MAC")
	}
	if err := p.session.Verify(body[:len(body)-secure.MACTagSize], f.Data[split:], true); err != nil {
		t.Fatalf("device rejected the panel's MAC: %v", err)
	}

	payload := f.Data[:split]
	if secure.BlockType(f.Security.Type).Encrypted() {
		plain, err := p.session.Open(payload, true)
		if err != nil {
			t.Fatalf("device could not decipher the command: %v", err)
		}
		payload = plain
	}
	p.received = append(p.received, cmd.Message{
		Code: cmd.Code(f.Code),
		Data: append([]byte(nil), payload...),
	})

	if p.card == nil {
		return p.sealedReply(t, f.Address, seq, secure.SCS16, cmd.ACK, nil)
	}
	// A credential is never sent in the clear on an established session.
	return p.sealedReply(t, f.Address, seq, secure.SCS18, cmd.Raw, p.card)
}
