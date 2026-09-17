// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package bus_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/regalium-os/osdp-go/internal/bus"
	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
)

// How a device joins the bus, how it leaves, and what happens when the two ends
// disagree about where they are in the exchange. The helpers are in bus_test.go.

// TestDeviceComesOnlineThenIdentifies walks the state machine an offline device
// follows on returning: identify, capabilities, then ordinary polling.
func TestDeviceComesOnlineThenIdentifies(t *testing.T) {
	ctx, now := context.Background(), time.Now()
	b := newBus(0x00)
	d := b.Devices()[0]

	// Offline devices are asked what they are before anything else.
	step, _ := b.Next(ctx)
	if cmd.Code(step.Frame.Code) != cmd.ID {
		t.Fatalf("first command to an offline device = %s, want osdp_ID",
			cmd.Code(step.Frame.Code).Name(false))
	}

	id := []byte{0x00, 0x06, 0x8E, 0x01, 0x02, 0x0A, 0x0B, 0x0C, 0x0D, 0x01, 0x00, 0x03}
	ev, err := b.Reply(ctx, d, reply(0x00, 1, cmd.PDID, id), now)
	if err != nil {
		t.Fatalf("Reply: %v", err)
	}
	if ev.Kind != bus.KindIdentified {
		t.Fatalf("event = %v, want identified", ev.Kind)
	}
	if got := ev.ID.OUI(); got != 0x00068E {
		t.Errorf("OUI = 0x%06X, want 0x00068E", got)
	}

	// Identified devices are asked for capabilities next.
	step, _ = b.Next(ctx)
	if cmd.Code(step.Frame.Code) != cmd.Cap {
		t.Errorf("second command = %s, want osdp_CAP", cmd.Code(step.Frame.Code).Name(false))
	}

	ev, _ = b.Reply(ctx, d, reply(0x00, 2, cmd.PDCap, []byte{0x01, 0x01, 0x01}), now)
	if ev.Kind != bus.KindOnline {
		t.Errorf("event = %v, want online", ev.Kind)
	}

	// And thereafter, polled.
	step, _ = b.Next(ctx)
	if cmd.Code(step.Frame.Code) != cmd.Poll {
		t.Errorf("third command = %s, want osdp_POLL", cmd.Code(step.Frame.Code).Name(false))
	}
}

// TestOfflineRequiresRepeatedMisses: one dropped reply is line noise, not a
// dead reader. Only a run of them raises an event.
func TestOfflineRequiresRepeatedMisses(t *testing.T) {
	ctx, now := context.Background(), time.Now()
	b := newBus(0x00)
	d := b.Devices()[0]

	// Bring it online first.
	b.Next(ctx)
	b.Reply(ctx, d, reply(0x00, 1, cmd.ACK, nil), now)
	b.Next(ctx)
	b.Reply(ctx, d, reply(0x00, 2, cmd.PDCap, []byte{0x0D, 0x01, 0x01}), now)

	for i := 1; i < bus.OfflineThreshold; i++ {
		if ev := b.Timeout(ctx, d); ev.Kind != bus.KindNone {
			t.Fatalf("miss %d raised %v; only %d consecutive misses should",
				i, ev.Kind, bus.OfflineThreshold)
		}
	}
	if ev := b.Timeout(ctx, d); ev.Kind != bus.KindOffline {
		t.Errorf("miss %d raised %v, want offline", bus.OfflineThreshold, ev.Kind)
	}
	if d.Online() {
		t.Error("device still reports online after being marked offline")
	}
}

// TestSequenceZeroTriggersResync: a device saying it has lost synchronisation
// must be restarted, not argued with.
func TestSequenceZeroTriggersResync(t *testing.T) {
	ctx, now := context.Background(), time.Now()
	b := newBus(0x00)
	d := b.Devices()[0]

	b.Next(ctx)
	ev, err := b.Reply(ctx, d, reply(0x00, 0, cmd.ACK, nil), now)
	if err != nil {
		t.Fatalf("Reply: %v", err)
	}
	if ev.Kind != bus.KindResync {
		t.Errorf("event = %v, want resync", ev.Kind)
	}

	// The next command restarts identification.
	step, _ := b.Next(ctx)
	if cmd.Code(step.Frame.Code) != cmd.ID {
		t.Errorf("after resync the bus sent %s, want osdp_ID",
			cmd.Code(step.Frame.Code).Name(false))
	}
}

func TestCardReadIsReported(t *testing.T) {
	ctx, now := context.Background(), time.Now()
	b := newBus(0x00)
	d := b.Devices()[0]

	payload := []byte{0x00, 0x01, 0x1A, 0x00, 0xAB, 0xCD, 0xEF, 0x80}
	ev, err := b.Reply(ctx, d, reply(0x00, 1, cmd.Raw, payload), now)
	if err != nil {
		t.Fatalf("Reply: %v", err)
	}
	if ev.Kind != bus.KindCardRead {
		t.Fatalf("event = %v, want card_read", ev.Kind)
	}
	if ev.Card.BitCount != 26 {
		t.Errorf("bit count = %d, want 26", ev.Card.BitCount)
	}
}

func TestReplyFromTheWrongAddressIsRejected(t *testing.T) {
	ctx, now := context.Background(), time.Now()
	b := newBus(0x00, 0x01)
	d := b.Devices()[0]

	if _, err := b.Reply(ctx, d, reply(0x01, 1, cmd.ACK, nil), now); !errors.Is(err, bus.ErrWrongAddress) {
		t.Errorf("Reply from another address = %v, want ErrWrongAddress", err)
	}
}

// TestPanelHearingItselfIsRejected: on a half-duplex line a panel that reads
// too early sees its own command. It must not be mistaken for an answer.
func TestPanelHearingItselfIsRejected(t *testing.T) {
	ctx, now := context.Background(), time.Now()
	b := newBus(0x00)
	d := b.Devices()[0]

	command := frame.Frame{
		Address: 0x00,
		Control: frame.NewControl(1, frame.SchemeCRC16, false),
		Code:    byte(cmd.Poll),
	}
	command.Seal()

	if _, err := b.Reply(ctx, d, command, now); !errors.Is(err, bus.ErrNotAReply) {
		t.Errorf("Reply given a command frame = %v, want ErrNotAReply", err)
	}
}
