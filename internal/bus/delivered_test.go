// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package bus_test

import (
	"context"
	"testing"
	"time"

	"github.com/regalium-os/osdp-go/internal/bus"
	"github.com/regalium-os/osdp-go/internal/cmd"
)

// When a command must NOT come back. A device that answered has the command,
// whatever it thought of it, and sending it again would act twice. The helpers
// are in retransmit_test.go.

// TestAnAnsweredCommandIsNotRetried: the device replied, so the command is
// done. Sending it again would act twice.
func TestAnAnsweredCommandIsNotRetried(t *testing.T) {
	ctx, now := context.Background(), time.Now()
	b := newBus(0x00)
	d := b.Devices()[0]
	online(t, b, d)

	b.Send(d, doorRelease())
	_, seq := sent(t, b)
	if _, err := b.Reply(ctx, d, reply(0x00, seq, cmd.ACK, nil), now); err != nil {
		t.Fatalf("Reply: %v", err)
	}

	if code, _ := sent(t, b); code != cmd.Poll {
		t.Errorf("after an acknowledged command the bus sent %s, want osdp_POLL",
			code.Name(false))
	}
	if d.Queued() != 0 {
		t.Errorf("%d commands still queued after delivery", d.Queued())
	}
}

// TestARefusedCommandIsNotRetried. An osdp_NAK is an answer: the device
// received the command and declined it, and retrying produces the same refusal
// forever.
func TestARefusedCommandIsNotRetried(t *testing.T) {
	ctx, now := context.Background(), time.Now()
	b := newBus(0x00)
	d := b.Devices()[0]
	online(t, b, d)

	b.Send(d, doorRelease())
	_, seq := sent(t, b)

	ev, err := b.Reply(ctx, d, reply(0x00, seq, cmd.NAK, []byte{0x03}), now)
	if err != nil {
		t.Fatalf("Reply: %v", err)
	}
	if ev.Kind != bus.KindNAK {
		t.Fatalf("event = %v, want nak", ev.Kind)
	}

	if code, _ := sent(t, b); code != cmd.Poll {
		t.Errorf("after a refusal the bus sent %s, want osdp_POLL", code.Name(false))
	}
}

// TestOrderSurvivesARetry: a door release that jumped behind a display update
// because its first attempt was lost would be a surprising thing to debug.
func TestOrderSurvivesARetry(t *testing.T) {
	ctx := context.Background()
	b := newBus(0x00)
	d := b.Devices()[0]
	online(t, b, d)

	b.Send(d, doorRelease())
	b.Send(d, cmd.TextCommand(cmd.TextDisplay{Content: "GRANTED"}))

	sent(t, b) // the release goes out
	b.Timeout(ctx, d)

	if code, _ := sent(t, b); code != cmd.Out {
		t.Errorf("the retry sent %s, want the release ahead of the display",
			code.Name(false))
	}
}

// TestACommandSurvivesGoingOffline. A device that stops answering keeps its
// queue; one that was mid-command must not be the exception.
func TestACommandSurvivesGoingOffline(t *testing.T) {
	ctx := context.Background()
	b := newBus(0x00)
	d := b.Devices()[0]
	online(t, b, d)

	b.Send(d, doorRelease())
	sent(t, b)

	for range bus.OfflineThreshold {
		b.Timeout(ctx, d)
	}
	if d.Online() {
		t.Fatal("the device should be offline by now")
	}
	if d.Queued() == 0 {
		t.Error("going offline discarded the command the device never answered")
	}
}

// TestAnOldCommandIsNotResurrectedByALaterTimeout.
//
// The subtle half of releasing a delivered command. If the bus kept holding it
// after the device answered, the next unrelated timeout -- a poll lost to noise
// an hour later -- would find it still in hand and send it again. A door would
// open with nothing having asked it to, and the event stream would show a
// release nobody ordered.
func TestAnOldCommandIsNotResurrectedByALaterTimeout(t *testing.T) {
	ctx, now := context.Background(), time.Now()
	b := newBus(0x00)
	d := b.Devices()[0]
	online(t, b, d)

	// A release, delivered and acknowledged.
	b.Send(d, doorRelease())
	_, seq := sent(t, b)
	if _, err := b.Reply(ctx, d, reply(0x00, seq, cmd.ACK, nil), now); err != nil {
		t.Fatalf("Reply: %v", err)
	}

	// Later, an ordinary poll goes unanswered.
	if code, _ := sent(t, b); code != cmd.Poll {
		t.Fatalf("expected a poll after the release was acknowledged")
	}
	b.Timeout(ctx, d)

	if code, _ := sent(t, b); code != cmd.Poll {
		t.Errorf("a lost poll resurrected %s; the door would open unbidden",
			code.Name(false))
	}
	if d.Queued() != 0 {
		t.Errorf("%d commands queued; an acknowledged release was put back", d.Queued())
	}
}

// TestAPollIsNotRetried. A poll asks a question; losing it loses nothing, and
// re-sending it ahead of real work would let a noisy line starve the queue.
func TestAPollIsNotRetried(t *testing.T) {
	ctx := context.Background()
	b := newBus(0x00)
	d := b.Devices()[0]
	online(t, b, d)

	if code, _ := sent(t, b); code != cmd.Poll {
		t.Fatal("expected a poll from an idle device")
	}
	b.Timeout(ctx, d)

	if d.Queued() != 0 {
		t.Errorf("a lost poll was queued for retry (%d waiting)", d.Queued())
	}
}
