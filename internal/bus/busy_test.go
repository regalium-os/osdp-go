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

// busy hands the bus an osdp_BUSY in answer to whatever it last sent.
func busy(t *testing.T, b *bus.Bus, d *bus.Device, seq uint8) bus.Event {
	t.Helper()

	ev, err := b.Reply(context.Background(), d, reply(d.Address, seq, cmd.Busy, nil), time.Now())
	if err != nil {
		t.Fatalf("Reply: %v", err)
	}
	return ev
}

// TestBusyIsNotSuccess.
//
// The reply that is easiest to get wrong, because it looks like an answer and
// is not one. A device that says busy never acted on the command; recording it
// as delivered loses a door release silently and leaves the panel reporting
// that the door opened.
func TestBusyIsNotSuccess(t *testing.T) {
	b := newBus(0x00)
	d := b.Devices()[0]
	online(t, b, d)

	b.Send(d, doorRelease())
	code, seq := sent(t, b)
	if code != cmd.Out {
		t.Fatalf("sent %s, want the release", code.Name(false))
	}

	if ev := busy(t, b, d, seq); ev.Kind != bus.KindBusy {
		t.Fatalf("osdp_BUSY produced %v, want busy", ev.Kind)
	}
	if d.Queued() == 0 {
		t.Fatal("the command was discarded; a busy device never acted on it")
	}
}

// TestAPollFollowsABusy, then the command is retried.
//
// Without the intervening poll a device that is busy twice running would never
// be asked anything else -- so a card presented at that reader during the busy
// period would go unreported, which is the one thing a poll cycle exists for.
func TestAPollFollowsABusy(t *testing.T) {
	b := newBus(0x00)
	d := b.Devices()[0]
	online(t, b, d)

	b.Send(d, doorRelease())
	_, seq := sent(t, b)
	busy(t, b, d, seq)

	if code, _ := sent(t, b); code != cmd.Poll {
		t.Errorf("after a busy the bus sent %s, want a poll first", code.Name(false))
	}
	if code, _ := sent(t, b); code != cmd.Out {
		t.Errorf("the command was not retried after the poll; sent %s", code.Name(false))
	}
}

// TestTheBackoffIsOneExchange: it is a pause, not a state the device sits in.
func TestTheBackoffIsOneExchange(t *testing.T) {
	b := newBus(0x00)
	d := b.Devices()[0]
	online(t, b, d)

	b.Send(d, doorRelease())
	b.Send(d, cmd.TextCommand(cmd.TextDisplay{Content: "WAIT"}))

	_, seq := sent(t, b)
	busy(t, b, d, seq)

	sent(t, b) // the poll
	if code, _ := sent(t, b); code != cmd.Out {
		t.Fatalf("sent %s, want the release retried", code.Name(false))
	}
	// No second backoff: the device did not say busy again.
	if code, _ := sent(t, b); code != cmd.Text {
		t.Errorf("sent %s, want the queue to carry on", code.Name(false))
	}
}

// TestABusyRetryIsNotARetransmission.
//
// A lost reply and a busy device need opposite sequence handling. A lost reply
// repeats the number, so a device that already acted replays its cached answer
// instead of acting twice. A busy device never acted and cached nothing, so the
// retry is a new command -- repeating the number would have the device replay
// the osdp_BUSY it already sent, forever.
func TestABusyRetryIsNotARetransmission(t *testing.T) {
	b := newBus(0x00)
	d := b.Devices()[0]
	online(t, b, d)

	b.Send(d, doorRelease())
	_, first := sent(t, b)
	busy(t, b, d, first)

	_, afterPoll := sent(t, b)
	_, retry := sent(t, b)

	if retry == first {
		t.Errorf("the retry repeated sequence %d; a busy device cached no answer "+
			"to replay and would answer busy again forever", retry)
	}
	if afterPoll == first {
		t.Errorf("the poll repeated sequence %d, which marks it a retransmission", first)
	}
}

// TestABusyDeviceStaysOnline: it answered, so it is plainly alive.
func TestABusyDeviceStaysOnline(t *testing.T) {
	b := newBus(0x00)
	d := b.Devices()[0]
	online(t, b, d)

	b.Send(d, doorRelease())
	_, seq := sent(t, b)
	busy(t, b, d, seq)

	if !d.Online() {
		t.Error("a device that answered was marked offline")
	}
}

// TestAPersistentlyBusyDeviceStillGetsPolled.
//
// The failure mode the backoff exists to prevent: a reader stuck busy must not
// stop the panel asking it questions, or a credential presented there is lost.
func TestAPersistentlyBusyDeviceStillGetsPolled(t *testing.T) {
	b := newBus(0x00)
	d := b.Devices()[0]
	online(t, b, d)

	b.Send(d, doorRelease())

	var polls int
	for range 10 {
		code, seq := sent(t, b)
		if code == cmd.Poll {
			polls++
			// Answer the poll normally; only the command draws a busy.
			if _, err := b.Reply(context.Background(), d,
				reply(0x00, seq, cmd.ACK, nil), time.Now()); err != nil {
				t.Fatalf("Reply: %v", err)
			}
			continue
		}
		busy(t, b, d, seq)
	}

	if polls == 0 {
		t.Fatal("a persistently busy device was never polled; card reads would be lost")
	}
	if d.Queued() == 0 {
		t.Error("the command was eventually dropped rather than kept")
	}
}
