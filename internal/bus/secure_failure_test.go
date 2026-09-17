// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package bus_test

import (
	"context"
	"testing"
	"time"

	"github.com/regalium-os/osdp-go/internal/bus"
	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/internal/secure"
)

// What happens when a secure channel cannot be established, or stops being
// trustworthy once it has been. The helpers are in secure_test.go.

// TestHandshakeFailureIsNotRetried: a cryptogram mismatch means the reader does
// not hold the key, and no number of retries will change that. Hammering it
// every cycle is how one wrong key becomes a bus that carries nothing.
func TestHandshakeFailureIsNotRetried(t *testing.T) {
	b := secureBus(siteKey, true)
	d := b.Devices()[0]
	pd := newPeripheral(siteKey)
	pd.badCryptogram = true

	enrol(t, b, d, pd)
	if d.State != bus.SecureHandshake {
		t.Fatalf("state = %v, want secure_handshake", d.State)
	}

	ev := exchange(t, b, d, pd)
	if ev.Kind != bus.KindSecureFailed {
		t.Fatalf("a bad cryptogram produced %v, want secure_failed", ev.Kind)
	}
	if d.State != bus.Online {
		t.Errorf("state = %v, want online: the device stays on the bus", d.State)
	}
	if _, ok := d.SecureSession(); ok {
		t.Error("the failed session was kept")
	}

	code, secured := nextCode(t, b)
	if code != cmd.Poll || secured {
		t.Errorf("next command = %s (secure %v), want a plain osdp_POLL",
			code.Name(false), secured)
	}
}

// TestAlteredSecureReplyTearsDownTheSession: a frame that fails its MAC is a
// frame something on the line altered, and the specification's answer is to
// abandon the session rather than resynchronise inside it.
func TestAlteredSecureReplyTearsDownTheSession(t *testing.T) {
	ctx := context.Background()
	b := secureBus(siteKey, true)
	d := b.Devices()[0]
	pd := newPeripheral(siteKey)

	establish(t, b, d, pd)

	step, _ := b.Next(ctx)
	answer := pd.answer(t, ctx, step.Frame)
	answer.Data[len(answer.Data)-1] ^= 0xFF // one bit of the MAC
	answer.Seal()                           // and a check that still agrees

	ev, err := b.Reply(ctx, d, answer, time.Now())
	if err != nil {
		t.Fatalf("Reply: %v", err)
	}
	if ev.Kind != bus.KindSecureFailed {
		t.Fatalf("an altered reply produced %v, want secure_failed", ev.Kind)
	}
	if _, ok := d.SecureSession(); ok {
		t.Error("the session survived a failed authentication")
	}
	if d.State != bus.Offline {
		t.Errorf("state = %v, want offline: the exchange restarts", d.State)
	}
}

// TestEstablishedSessionRejectsUnauthenticatedReplies.
//
// A device that has proved it holds the key and then answers in the clear is
// either not that device any more or is being spoken for by something else on
// the line. Both cases are treated exactly as a MAC failure, and deliberately
// not distinguished from one in what goes back to the peer.
func TestEstablishedSessionRejectsUnauthenticatedReplies(t *testing.T) {
	for _, tc := range []struct {
		name  string
		reply func(pd *peripheral, sent frame.Frame) frame.Frame
	}{
		{
			name: "no-security-block",
			reply: func(_ *peripheral, sent frame.Frame) frame.Frame {
				return reply(sent.Address, sent.Control.Sequence(), cmd.ACK, nil)
			},
		},
		{
			name: "block-but-no-room-for-a-mac",
			reply: func(_ *peripheral, sent frame.Frame) frame.Frame {
				return handshakeReply(sent.Address, sent.Control.Sequence(),
					secure.SCS16, cmd.ACK, []byte{0x01, 0x02})
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			b := secureBus(siteKey, true)
			d := b.Devices()[0]
			pd := newPeripheral(siteKey)

			establish(t, b, d, pd)

			step, _ := b.Next(ctx)
			ev, err := b.Reply(ctx, d, tc.reply(pd, step.Frame), time.Now())
			if err != nil {
				t.Fatalf("Reply: %v", err)
			}
			if ev.Kind != bus.KindSecureFailed {
				t.Errorf("event = %v, want secure_failed", ev.Kind)
			}
			if _, ok := d.SecureSession(); ok {
				t.Error("the session survived an unauthenticated reply")
			}
		})
	}
}
