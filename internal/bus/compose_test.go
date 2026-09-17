// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package bus

import (
	"bytes"
	"context"
	"testing"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/internal/secure"
	"github.com/regalium-os/osdp-go/internal/transport"
)

// This test is inside package bus because it exercises the command half of an
// established session, and nothing on the bus reaches it yet: the only command
// sent over a live session today is osdp_POLL, which carries no payload and so
// travels authenticated but unenciphered under SCS_15.
//
// The encrypted path is implemented rather than deferred because the first
// command that does carry a payload -- osdp_LED, osdp_OUT, osdp_KEYSET, which
// the specification requires to be secure -- must not be the occasion for
// working out the ordering from scratch. It is tested here so it is not
// shipped unexercised.

// establishedPair returns a control panel and a peripheral session that have
// completed a handshake against each other.
func establishedPair(t *testing.T) (cp, pd *secure.Session) {
	t.Helper()

	key := secure.BaseKey{
		0xA0, 0xA1, 0xA2, 0xA3, 0xA4, 0xA5, 0xA6, 0xA7,
		0xA8, 0xA9, 0xAA, 0xAB, 0xAC, 0xAD, 0xAE, 0xAF,
	}
	cp = secure.NewSession(secure.AES128{}, secure.RoleCP, key)
	pd = secure.NewSession(secure.AES128{}, secure.RolePD, key)

	chlng, err := cp.BeginChallenge([8]byte{1, 2, 3, 4, 5, 6, 7, 8})
	if err != nil {
		t.Fatalf("BeginChallenge: %v", err)
	}
	ccrypt, err := pd.AnswerChallenge(chlng, [8]byte{9, 10, 11, 12, 13, 14, 15, 16},
		[8]byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x00, 0x11})
	if err != nil {
		t.Fatalf("AnswerChallenge: %v", err)
	}
	scrypt, err := cp.AnswerCryptogram(ccrypt)
	if err != nil {
		t.Fatalf("AnswerCryptogram: %v", err)
	}
	rmac, err := pd.AnswerServerCryptogram(scrypt)
	if err != nil {
		t.Fatalf("AnswerServerCryptogram: %v", err)
	}
	if err := cp.CompleteHandshake(rmac); err != nil {
		t.Fatalf("CompleteHandshake: %v", err)
	}
	return cp, pd
}

// TestCommandWithAPayloadIsEnciphered: a command carrying data goes under
// SCS_17, and the device deciphers exactly what the panel meant to send.
func TestCommandWithAPayloadIsEnciphered(t *testing.T) {
	ctx := context.Background()
	cp, pd := establishedPair(t)

	b := New(transport.Line{Name: "test"}, []frame.Address{0x00}, frame.SchemeCRC16)
	d := b.Devices()[0]
	d.session, d.State = cp, Secure

	// An osdp_LED control block: reader 0, LED 0, permanent red.
	payload := []byte{0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

	block := blockForCommand(len(payload))
	if got := secure.BlockType(block.Type); got != secure.SCS17 {
		t.Fatalf("a command with a payload used %v, want SCS_17", got)
	}

	f, err := b.compose(ctx, d, cmd.Message{Code: cmd.LED, Data: payload}, &block)
	if err != nil {
		t.Fatalf("compose: %v", err)
	}

	wire, err := f.Append(ctx, nil)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if bytes.Contains(wire, payload) {
		t.Fatal("the payload is on the wire in the clear; SCS_17 enciphered nothing")
	}

	// The device does what a device does: decode, authenticate, decipher.
	got, err := frame.Decode(ctx, wire)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	body := got.AppendBody(nil)
	split := len(got.Data) - secure.MACTagSize

	if vErr := pd.Verify(body[:len(body)-secure.MACTagSize], got.Data[split:], true); vErr != nil {
		t.Fatalf("device rejected the panel's MAC: %v", vErr)
	}
	plain, err := pd.Open(got.Data[:split], true)
	if err != nil {
		t.Fatalf("device could not decipher the command: %v", err)
	}
	if !bytes.Equal(plain, payload) {
		t.Errorf("deciphered payload = % X, want % X", plain, payload)
	}
}

// TestPollTravelsAuthenticatedOnly: there is nothing to encipher in an
// osdp_POLL, and padding an empty payload would put a whole block of cipher on
// the wire for no gain. It is most of what a bus carries.
func TestPollTravelsAuthenticatedOnly(t *testing.T) {
	if got := secure.BlockType(blockForCommand(0).Type); got != secure.SCS15 {
		t.Errorf("a command with no payload used %v, want SCS_15", got)
	}
}
