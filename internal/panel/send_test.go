// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package panel_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/regalium-os/osdp-go/internal/bus"
	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/panel"
)

// TestSendReachesTheDevice is the point of the whole command path: an
// application asks for a door to be released, and the octets arrive.
func TestSendReachesTheDevice(t *testing.T) {
	p, device := newPanel(t, 0x00)
	rec := &recorder{}
	go respond(t, device, rec.answer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := collect(p)
	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()
	events.await(t, bus.KindOnline)

	strike := cmd.OutputCommand(cmd.Output{
		Number: 0, Control: cmd.OutputTimedOn, Timer: 5 * time.Second,
	})
	if err := p.Send(ctx, 0x00, strike); err != nil {
		t.Fatalf("Send: %v", err)
	}

	got := rec.waitForCommand(t, cmd.Out)
	if want := []byte{0x00, 0x05, 0x32, 0x00}; string(got.Data) != string(want) {
		t.Errorf("the device received % X, want % X", got.Data, want)
	}

	cancel()
	<-done
}

// TestCommandsAreDeliveredInOrder. A grant is two commands -- release the
// strike, show green -- and a reader that received them the other way round
// would light green for a door that never opened.
func TestCommandsAreDeliveredInOrder(t *testing.T) {
	p, device := newPanel(t, 0x00)
	rec := &recorder{}
	go respond(t, device, rec.answer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := collect(p)
	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()
	events.await(t, bus.KindOnline)

	for _, m := range []cmd.Message{
		cmd.OutputCommand(cmd.Output{Number: 0, Control: cmd.OutputTimedOn, Timer: time.Second}),
		cmd.BuzzerCommand(0, cmd.ToneDefault, 0, 0, 1),
		cmd.LEDCommand(0, 0, cmd.LEDState{Control: cmd.LEDSet}, time.Second,
			cmd.LEDState{Control: cmd.LEDNOP}),
	} {
		if err := p.Send(ctx, 0x00, m); err != nil {
			t.Fatalf("Send: %v", err)
		}
	}

	rec.waitForCommand(t, cmd.LED) // the last of the three

	rec.mu.Lock()
	defer rec.mu.Unlock()
	var order []cmd.Code
	for _, m := range rec.seen {
		if m.Code != cmd.Poll && m.Code != cmd.ID && m.Code != cmd.Cap {
			order = append(order, m.Code)
		}
	}

	want := []cmd.Code{cmd.Out, cmd.Buz, cmd.LED}
	if len(order) < len(want) {
		t.Fatalf("the device saw %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Errorf("command %d was %s, want %s", i,
				order[i].Name(false), want[i].Name(false))
		}
	}

	cancel()
	<-done
}

// TestSendToAnAddressNotOnTheLine: the address list is fixed when the bus is
// built, so waiting will not make it valid.
func TestSendToAnAddressNotOnTheLine(t *testing.T) {
	p, device := newPanel(t, 0x00)
	go respond(t, device, cardReadOnPoll)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := collect(p)
	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()
	events.await(t, bus.KindOnline)

	err := p.Send(ctx, 0x7E, cmd.Message{Code: cmd.Poll})
	if !errors.Is(err, panel.ErrUnknownDevice) {
		t.Errorf("Send to an unpolled address = %v, want ErrUnknownDevice", err)
	}

	cancel()
	<-done
}

// TestSendAfterRunReturnsIsRefused rather than queued: accepting it would be a
// promise the runtime cannot keep.
func TestSendAfterRunReturnsIsRefused(t *testing.T) {
	p, _ := newPanel(t)

	if err := p.Run(context.Background()); !errors.Is(err, panel.ErrNoDevices) {
		t.Fatalf("Run = %v, want ErrNoDevices", err)
	}
	if err := p.Send(context.Background(), 0x00, cmd.Message{Code: cmd.Poll}); !errors.Is(err, panel.ErrNotRunning) {
		t.Errorf("Send after Run = %v, want ErrNotRunning", err)
	}
}
