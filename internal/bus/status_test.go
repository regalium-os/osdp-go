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

// report hands the bus one status reply and returns what it made of it.
func report(t *testing.T, b *bus.Bus, d *bus.Device, code cmd.Code, data []byte) bus.Event {
	t.Helper()

	ev, err := b.Reply(context.Background(), d, reply(d.Address, 1, code, data), time.Now())
	if err != nil {
		t.Fatalf("Reply(%s): %v", code.Name(true), err)
	}
	return ev
}

// TestTheFirstReportIsTheBaseline.
//
// A panel coming up to a door that is already standing open must be told. It
// has no previous state to compare against, so the whole first report is news --
// anything else and a contact that was abnormal at boot stays invisible until
// somebody closes it.
func TestTheFirstReportIsTheBaseline(t *testing.T) {
	b := newBus(0x00)
	d := b.Devices()[0]

	// Four inputs: the second one active -- a door standing open at boot.
	ev := report(t, b, d, cmd.IStatR, []byte{0x00, 0x01, 0x00, 0x00})

	if ev.Kind != bus.KindStatusChange {
		t.Fatalf("event = %v, want status_change", ev.Kind)
	}
	if len(ev.Status) != 4 {
		t.Fatalf("reported %d changes, want all 4 as the baseline", len(ev.Status))
	}
	if !ev.Status[1].Active || ev.Status[1].Index != 1 {
		t.Errorf("input 1 = %+v, want the open door reported active", ev.Status[1])
	}
}

// TestOnlyTransitionsAreReportedAfterThat: a door shut for a week is not an
// event, and treating it as one buries the door that just opened.
func TestOnlyTransitionsAreReportedAfterThat(t *testing.T) {
	b := newBus(0x00)
	d := b.Devices()[0]

	report(t, b, d, cmd.IStatR, []byte{0x00, 0x00, 0x00, 0x00}) // baseline

	if ev := report(t, b, d, cmd.IStatR, []byte{0x00, 0x00, 0x00, 0x00}); ev.Kind != bus.KindNone {
		t.Fatalf("an unchanged report produced %v, want none", ev.Kind)
	}

	ev := report(t, b, d, cmd.IStatR, []byte{0x00, 0x00, 0x01, 0x00})
	if ev.Kind != bus.KindStatusChange {
		t.Fatalf("event = %v, want status_change", ev.Kind)
	}
	if len(ev.Status) != 1 {
		t.Fatalf("reported %d changes, want only the contact that moved", len(ev.Status))
	}
	if got := ev.Status[0]; got.Index != 2 || !got.Active || got.Kind != cmd.StatusInput {
		t.Errorf("change = %+v, want input 2 active", got)
	}
}

// TestATamperIsReportedApartFromPower: osdp_LSTATR carries both in one reply,
// and a panel acts on them separately -- a reader torn off a wall and a reader
// on backup power are different incidents.
func TestATamperIsReportedApartFromPower(t *testing.T) {
	b := newBus(0x00)
	d := b.Devices()[0]

	ev := report(t, b, d, cmd.LStatR, []byte{0x01, 0x00})
	if len(ev.Status) != 2 {
		t.Fatalf("reported %d changes, want tamper and power", len(ev.Status))
	}

	byKind := map[cmd.StatusKind]bool{}
	for _, c := range ev.Status {
		byKind[c.Kind] = c.Active
	}
	if !byKind[cmd.StatusTamper] {
		t.Error("the tamper switch tripped and was not reported as a tamper")
	}
	if byKind[cmd.StatusPower] {
		t.Error("power was reported abnormal when the device said it was fine")
	}
}

// TestOneKindSaysNothingAboutAnother: a report covering inputs must not be
// taken as a statement about outputs, or the first output report would be
// silently treated as a non-change.
func TestOneKindSaysNothingAboutAnother(t *testing.T) {
	b := newBus(0x00)
	d := b.Devices()[0]

	report(t, b, d, cmd.IStatR, []byte{0x00, 0x00})

	ev := report(t, b, d, cmd.OStatR, []byte{0x00, 0x00})
	if ev.Kind != bus.KindStatusChange || len(ev.Status) != 2 {
		t.Fatalf("the first output report produced %v with %d changes, want a baseline of 2",
			ev.Kind, len(ev.Status))
	}
}

// TestStatusIsReadableAfterwards, and reading it cannot disturb it.
func TestStatusIsReadableAfterwards(t *testing.T) {
	b := newBus(0x00)
	d := b.Devices()[0]

	if _, ok := d.Status(cmd.StatusInput); ok {
		t.Error("a device that has reported nothing claims to know its inputs")
	}

	report(t, b, d, cmd.IStatR, []byte{0x00, 0x01})

	got, ok := d.Status(cmd.StatusInput)
	if !ok || len(got) != 2 || !got[1] {
		t.Fatalf("Status = %v, %v; want the reported state", got, ok)
	}

	got[1] = false
	if again, _ := d.Status(cmd.StatusInput); !again[1] {
		t.Error("the returned slice aliases the bus's state; a reader can corrupt it")
	}
}
