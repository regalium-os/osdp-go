// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd_test

import (
	"context"
	"testing"
	"time"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/pd"
	"github.com/regalium-os/osdp-go/internal/secure"
)

// Every way a Secure Channel ends, and what the device is left holding. The
// happy path is secure_test.go.

// TestAWrongKeyIsRefusedAtTheServerCryptogram.
//
// The challenge and the device's own cryptogram succeed regardless of the key:
// the device simply computes from what it holds. The disagreement surfaces at
// osdp_SCRYPT, where the panel's proof is checked against the device's own
// derivation -- and a mismatch is a commissioning fault, not line noise, so the
// session is abandoned rather than retried inside itself.
func TestAWrongKeyIsRefusedAtTheServerCryptogram(t *testing.T) {
	var otherKey secure.BaseKey
	for i := range otherKey {
		otherKey[i] = 0x5A
	}

	d := secureDevice(t, testKey)
	c := newController(t, otherKey)

	chlng, err := c.session.BeginChallenge([8]byte{1, 2, 3, 4, 5, 6, 7, 8})
	if err != nil {
		t.Fatalf("BeginChallenge: %v", err)
	}
	ccrypt := answer(t, d, c.command(t, cmd.Message{Code: cmd.Chlng, Data: chlng}, secure.SCS11))

	// The panel notices first, because it checks the device's proof against its
	// own key before answering.
	if _, err := c.session.AnswerCryptogram(ccrypt.Data); err == nil {
		t.Fatal("the panel accepted a cryptogram computed from a different key")
	}

	// A device sent a server cryptogram it cannot verify refuses it and keeps
	// nothing: a half-built session is not a session.
	bogus := c.command(t, cmd.Message{Code: cmd.SCrypt, Data: make([]byte, secure.BlockSize)}, secure.SCS13)
	reply := answer(t, d, bogus)
	replyCode(t, reply, cmd.NAK)
	expectNAK(cmd.NAKSecureRequired)(t, reply.Data)

	if d.Secure() {
		t.Error("the device considers a session established after refusing the proof")
	}
}

// TestATamperedFrameTearsTheSessionDown.
//
// A frame that fails its message authentication code is a frame something on
// the line altered. Per SIA OSDP v2.2.2 §7.5 the response is to abandon the
// session rather than resynchronise within it, because the alternative is
// accepting whatever the attacker chose to change.
func TestATamperedFrameTearsTheSessionDown(t *testing.T) {
	d := secureDevice(t, testKey)
	c := newController(t, testKey)
	c.handshake(t, d)

	release := c.send(t, cmd.OutputCommand(cmd.Output{Number: 0, Control: cmd.OutputTimedOn}))
	release.Data[0] ^= 0xFF // something on the line flips a bit of the ciphertext
	release.Seal()          // and the error check still matches, as it would

	reply := answer(t, d, throughWire(t, release))
	replyCode(t, reply, cmd.NAK)
	expectNAK(cmd.NAKSecureRequired)(t, reply.Data)

	if d.Secure() {
		t.Error("the session survived a frame that did not authenticate")
	}
	if reply.Security != nil {
		t.Error("the refusal was sealed with a session that had just been " +
			"torn down, which the panel could not open")
	}
}

// TestPlaintextOnAnEstablishedSessionIsRefusedOnceAndThenTheChannelIsGone.
//
// The refusal is the security half: the reply to a poll can be a credential,
// and handing one back in the clear because somebody injected an
// unauthenticated frame is exactly the downgrade the channel exists to prevent.
//
// The teardown is the interoperability half. A device that refused forever
// would be unreachable by a panel that had simply restarted, so from the next
// command on it is an ordinary plaintext reader again, free to be challenged
// afresh whenever the panel is ready.
func TestPlaintextOnAnEstablishedSessionIsRefusedOnceAndThenTheChannelIsGone(t *testing.T) {
	ctx := context.Background()
	d := secureDevice(t, testKey)
	c := newController(t, testKey)
	c.handshake(t, d)

	if err := d.ReportCardRead(ctx, cmd.CardRead{
		Format: 1, BitCount: 8, Data: []byte{0x5A},
	}); err != nil {
		t.Fatalf("ReportCardRead: %v", err)
	}

	refused := answer(t, d, poll(t, 1))
	replyCode(t, refused, cmd.NAK)
	expectNAK(cmd.NAKSecureRequired)(t, refused.Data)

	if d.Secure() {
		t.Error("the session survived an unauthenticated command")
	}
	if got := d.Pending(); got != 1 {
		t.Errorf("%d events queued, want 1: the credential was reported in the "+
			"clear in answer to a frame nobody authenticated", got)
	}

	// And the device is reachable again rather than wedged.
	replyCode(t, answer(t, d, poll(t, 2)), cmd.Raw)
}

// TestARestartTearsTheSessionDown. SIA OSDP v2.2.2 §7.5: a session does not
// survive a restart of the exchange, because the panel's own session state went
// with the restart that prompted it.
//
// The restart arrives inside a security block, which a real panel would not
// send -- it sends plaintext, having forgotten the session. That is exactly why
// the test does not: a plaintext frame would be torn down by the downgrade rule
// instead, and the test would pass without the restart rule existing at all.
// A secured frame can only be refused if sequence zero dropped the session
// first.
func TestARestartTearsTheSessionDown(t *testing.T) {
	d := secureDevice(t, testKey)
	c := newController(t, testKey)
	c.handshake(t, d)

	reply := answer(t, d, c.commandAt(t, cmd.Message{Code: cmd.Poll}, secure.SCS15, 0))
	if d.Secure() {
		t.Error("the session survived sequence zero")
	}
	replyCode(t, reply, cmd.NAK)
	expectNAK(cmd.NAKSecureRequired)(t, reply.Data)
}

// TestSilenceTearsTheSessionDown, for the same reason and by the same route: a
// gap longer than the communication timeout is a panel that went away, and its
// session went with it.
func TestSilenceTearsTheSessionDown(t *testing.T) {
	clock := newClock()
	d := secureDevice(t, testKey,
		pd.WithClock(clock), pd.WithCommunicationTimeout(time.Second))
	c := newController(t, testKey)
	c.handshake(t, d)

	clock.advance(2 * time.Second)
	answer(t, d, c.send(t, cmd.Message{Code: cmd.Poll}))

	if d.Secure() {
		t.Error("the session survived a gap longer than the communication timeout")
	}
}
