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

// online brings a device up so it is sent commands rather than enrolment.
func online(t *testing.T, b *bus.Bus, d *bus.Device) {
	t.Helper()
	ctx, now := context.Background(), time.Now()

	b.Next(ctx)
	if _, err := b.Reply(ctx, d, reply(d.Address, 1, cmd.ACK, nil), now); err != nil {
		t.Fatalf("enrolment: %v", err)
	}
	b.Next(ctx)
	if _, err := b.Reply(ctx, d, reply(d.Address, 2, cmd.PDCap,
		[]byte{0x0D, 0x01, 0x01}), now); err != nil {
		t.Fatalf("capabilities: %v", err)
	}
}

// doorRelease is the command whose loss matters most.
func doorRelease() cmd.Message {
	return cmd.OutputCommand(cmd.Output{
		Number: 0, Control: cmd.OutputTimedOn, Timer: 5 * time.Second,
	})
}

// sent returns the code of the next command the bus wants to send.
func sent(t *testing.T, b *bus.Bus) (cmd.Code, uint8) {
	t.Helper()
	step, ok := b.Next(context.Background())
	if !ok {
		t.Fatal("the bus produced no step")
	}
	return cmd.Code(step.Frame.Code), step.Frame.Control.Sequence()
}

// TestAnUnansweredCommandIsRetried.
//
// This is the failure the whole mechanism exists for: a door release put on the
// wire, one burst of line noise, and the command gone with nothing told to the
// application. The panel had the only copy.
func TestAnUnansweredCommandIsRetried(t *testing.T) {
	ctx := context.Background()
	b := newBus(0x00)
	d := b.Devices()[0]
	online(t, b, d)

	b.Send(d, doorRelease())

	if code, _ := sent(t, b); code != cmd.Out {
		t.Fatalf("first command = %s, want osdp_OUT", code.Name(false))
	}
	b.Timeout(ctx, d) // the device never answered

	if code, _ := sent(t, b); code != cmd.Out {
		t.Errorf("after a lost reply the bus sent %s; the door command was lost",
			code.Name(false))
	}
}

// TestARetryRepeatsTheSequenceNumber.
//
// SIA OSDP v2.2.2 §5.7: a device caches the reply it gave to each sequence
// number. Repeating the number says "I did not hear you, say again", and the
// device replays its cached answer instead of acting again. Advancing it would
// present the retry as a new command -- and a door that was unlocked, whose
// reply was lost, would unlock a second time.
func TestARetryRepeatsTheSequenceNumber(t *testing.T) {
	ctx := context.Background()
	b := newBus(0x00)
	d := b.Devices()[0]
	online(t, b, d)

	b.Send(d, doorRelease())

	_, first := sent(t, b)
	b.Timeout(ctx, d)
	_, retry := sent(t, b)

	if retry != first {
		t.Errorf("retry used sequence %d, want %d repeated: a new sequence "+
			"presents the retry as a new command and the door opens twice",
			retry, first)
	}
}

// TestASuccessfulExchangeAdvancesTheSequence, so the repeat applies to the
// retry alone and not to everything after it.
func TestASuccessfulExchangeAdvancesTheSequence(t *testing.T) {
	ctx, now := context.Background(), time.Now()
	b := newBus(0x00)
	d := b.Devices()[0]
	online(t, b, d)

	b.Send(d, doorRelease())
	_, first := sent(t, b)
	b.Timeout(ctx, d)

	_, retry := sent(t, b)
	if _, err := b.Reply(ctx, d, reply(0x00, retry, cmd.ACK, nil), now); err != nil {
		t.Fatalf("Reply: %v", err)
	}

	if _, next := sent(t, b); next == first {
		t.Errorf("the sequence stayed at %d after a successful exchange", next)
	}
}

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
