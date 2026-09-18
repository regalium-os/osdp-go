// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package bus_test

import (
	"testing"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/internal/secure"
)

// The device's half of the four-message handshake, and what it does once the
// session is up. What the device is, and what it claims, is peripheral_test.go.

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

	// A device told to change its key stores it and rebuilds its session from
	// it, exactly as a real one does. Without this a rotation test would prove
	// only that the panel changed its own mind.
	if cmd.Code(f.Code) == cmd.KeySet {
		installed, err := cmd.ParseKeySet(payload)
		if err != nil {
			t.Fatalf("device could not parse the key it was sent: %v", err)
		}
		reply := p.sealedReply(t, f.Address, seq, secure.SCS16, cmd.ACK, nil)
		p.adopt(secure.BaseKey(installed.Key))
		return reply
	}

	if p.card == nil {
		return p.sealedReply(t, f.Address, seq, secure.SCS16, cmd.ACK, nil)
	}
	// A credential is never sent in the clear on an established session.
	return p.sealedReply(t, f.Address, seq, secure.SCS18, cmd.Raw, p.card)
}
