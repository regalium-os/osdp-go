// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd_test

import (
	"context"
	"testing"
	"time"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/pd"
)

// Sequence zero and what surrounds it: the panel restarting the exchange, and
// what the device is allowed to remember across it.

// TestSequenceZeroIsAnsweredAtZero.
//
// Per SIA OSDP v2.2.2 §5.7 zero is outside the 1-2-3 rotation: it is the
// panel saying it has lost track and is starting again. A device that answered
// at some other number would be telling a panel that has just cleared its state
// to look for a reply it is not expecting.
func TestSequenceZeroIsAnsweredAtZero(t *testing.T) {
	d := newDevice(t)

	reply := answer(t, d, poll(t, 0))
	if got := reply.Control.Sequence(); got != 0 {
		t.Errorf("a restart was answered at sequence %d, want 0", got)
	}
	replyCode(t, reply, cmd.ACK)
}

// TestSequenceZeroIsNeverReplayed.
//
// A second zero is a second restart, not a retransmission of the first. The
// panel only ever sends zero as the opening of a conversation, and the commands
// it opens with are idempotent, so re-executing is both safe and the only
// reading that lets a panel restart twice in a row.
func TestSequenceZeroIsNeverReplayed(t *testing.T) {
	ctx := context.Background()
	d := newDevice(t)

	for range 2 {
		if err := d.ReportCardRead(ctx, cmd.CardRead{BitCount: 8, Data: []byte{0x01}}); err != nil {
			t.Fatalf("ReportCardRead: %v", err)
		}
	}

	answer(t, d, poll(t, 0))
	answer(t, d, poll(t, 0))

	if got := d.Pending(); got != 0 {
		t.Errorf("%d events remain after two restarts, want 0: the second was "+
			"answered from a cache that sequence zero must never fill", got)
	}
}

// TestSequenceZeroForgetsTheCachedReply.
//
// The restart is the panel saying its own state is gone. A device that kept
// answering the number it cached before the restart would replay one command
// from the previous conversation into the new one.
func TestSequenceZeroForgetsTheCachedReply(t *testing.T) {
	ctx := context.Background()
	d := newDevice(t)

	if err := d.ReportCardRead(ctx, cmd.CardRead{BitCount: 8, Data: []byte{0x01}}); err != nil {
		t.Fatalf("ReportCardRead: %v", err)
	}

	answer(t, d, poll(t, 1)) // caches a reply at sequence 1
	answer(t, d, poll(t, 0)) // the panel restarts

	if err := d.ReportCardRead(ctx, cmd.CardRead{BitCount: 8, Data: []byte{0x02}}); err != nil {
		t.Fatalf("ReportCardRead: %v", err)
	}
	replyCode(t, answer(t, d, poll(t, 1)), cmd.Raw)

	if got := d.Pending(); got != 0 {
		t.Errorf("%d events remain, want 0: sequence 1 was replayed from before "+
			"the restart rather than executed", got)
	}
}

// TestAnEventSurvivesARestart.
//
// A card presented while the panel was rebooting is still a card that was
// presented, and the cardholder is still at the door. Only the reply cache
// belongs to one run of the exchange.
func TestAnEventSurvivesARestart(t *testing.T) {
	ctx := context.Background()
	d := newDevice(t)

	answer(t, d, poll(t, 1)) // a conversation is under way
	if err := d.ReportCardRead(ctx, cmd.CardRead{BitCount: 26, Data: []byte{1, 2, 3, 4}}); err != nil {
		t.Fatalf("ReportCardRead: %v", err)
	}

	replyCode(t, answer(t, d, poll(t, 0)), cmd.Raw)
}

// TestAnUnexpectedSequenceIsAcceptedByDefault.
//
// A jump means a frame was lost, which on a line shared with a motor is
// ordinary. Refusing it turns one lost command into a full resynchronisation,
// and the discipline that actually protects a door -- replaying a repeat -- is
// untouched either way. See WithStrictSequence for the other reading.
func TestAnUnexpectedSequenceIsAcceptedByDefault(t *testing.T) {
	d := newDevice(t)

	answer(t, d, poll(t, 1))
	replyCode(t, answer(t, d, poll(t, 3)), cmd.ACK)
}

// TestAnUnexpectedSequenceIsNAKedWhenStrict, with reason 0x04, so a panel that
// relies on the refusal to discover it has lost synchronisation gets it.
func TestAnUnexpectedSequenceIsNAKedWhenStrict(t *testing.T) {
	d := newDevice(t, pd.WithStrictSequence())

	answer(t, d, poll(t, 1))
	reply := answer(t, d, poll(t, 3))

	replyCode(t, reply, cmd.NAK)
	expectNAK(cmd.NAKSequenceError)(t, reply.Data)

	// And the refusal is cached like any other reply, so repeating the bad
	// number repeats the NAK rather than producing a second, different answer.
	replyCode(t, answer(t, d, poll(t, 3)), cmd.NAK)
}

// TestSilenceLongerThanTheTimeoutForgetsTheCachedReply.
//
// A device has no loop, so it notices the gap only when the next command
// arrives -- which is the moment it matters, because that command is almost
// certainly a restarted panel resuming mid-rotation with a sequence number that
// means nothing here.
func TestSilenceLongerThanTheTimeoutForgetsTheCachedReply(t *testing.T) {
	ctx := context.Background()
	clock := newClock()
	d := newDevice(t, pd.WithClock(clock), pd.WithCommunicationTimeout(time.Second))

	if err := d.ReportCardRead(ctx, cmd.CardRead{BitCount: 8, Data: []byte{0x01}}); err != nil {
		t.Fatalf("ReportCardRead: %v", err)
	}
	answer(t, d, poll(t, 1))

	clock.advance(2 * time.Second)
	if err := d.ReportCardRead(ctx, cmd.CardRead{BitCount: 8, Data: []byte{0x02}}); err != nil {
		t.Fatalf("ReportCardRead: %v", err)
	}
	replyCode(t, answer(t, d, poll(t, 1)), cmd.Raw)

	if got := d.Pending(); got != 0 {
		t.Errorf("%d events remain, want 0: the cache survived a gap longer than "+
			"the communication timeout", got)
	}
}

// TestSilenceWithinTheTimeoutKeepsTheCachedReply, so that an ordinary pause
// between poll cycles does not discard the reply the panel is about to ask for
// again.
func TestSilenceWithinTheTimeoutKeepsTheCachedReply(t *testing.T) {
	ctx := context.Background()
	clock := newClock()
	d := newDevice(t, pd.WithClock(clock), pd.WithCommunicationTimeout(time.Second))

	for _, bits := range []byte{0x01, 0x02} {
		if err := d.ReportCardRead(ctx, cmd.CardRead{BitCount: 8, Data: []byte{bits}}); err != nil {
			t.Fatalf("ReportCardRead: %v", err)
		}
	}
	answer(t, d, poll(t, 1))

	clock.advance(100 * time.Millisecond)
	answer(t, d, poll(t, 1))

	if _, seen := d.LastCommand(); !seen {
		t.Error("LastCommand reports the panel has never been heard from")
	}
	if got := d.Pending(); got != 1 {
		t.Errorf("%d events remain, want 1: the retry consumed a second event "+
			"instead of being answered from the cache", got)
	}
}
