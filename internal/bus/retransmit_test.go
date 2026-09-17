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

// TestACommandSurvivesAFailedAuthentication.
//
// A frame that fails its MAC was produced by something on the line rather than
// by the device that was addressed. Treating it as a reply would release a
// command nothing has acted on -- and the command most likely to be in flight
// when somebody is interfering with the line is the one worth interfering with.
func TestACommandSurvivesAFailedAuthentication(t *testing.T) {
	ctx := context.Background()
	b := secureBus(siteKey, true)
	d := b.Devices()[0]
	pd := newPeripheral(siteKey)
	establish(t, b, d, pd)

	b.Send(d, doorRelease())

	step, _ := b.Next(ctx)
	if cmd.Code(step.Frame.Code) != cmd.Out {
		t.Fatalf("sent %s, want the release", cmd.Code(step.Frame.Code).Name(false))
	}

	answer := pd.answer(t, ctx, step.Frame)
	answer.Data[len(answer.Data)-1] ^= 0xFF // one bit of the MAC
	answer.Seal()

	ev, err := b.Reply(ctx, d, answer, time.Now())
	if err != nil {
		t.Fatalf("Reply: %v", err)
	}
	if ev.Kind != bus.KindSecureFailed {
		t.Fatalf("event = %v, want secure_failed", ev.Kind)
	}

	if d.Queued() == 0 {
		t.Error("the command was released by a frame that failed authentication")
	}
}
