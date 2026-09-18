// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package record_test

import (
	"testing"
	"time"

	"github.com/regalium-os/osdp-go"
	eventpbv1 "github.com/regalium-os/osdp-go/protobuf/generated/go/event/v1/eventpbv1"
	"github.com/regalium-os/osdp-go/protobuf/record"
)

// Recording contact changes. The helpers are in event_test.go.

// TestAStatusReplyBecomesOneRecordPerContact.
//
// Three contacts moving is three things that happened. The schema's Event
// carries one payload, so flattening them into a single row would lose which
// contact did what -- and "something on this device changed" is not an audit
// trail anybody can act on.
func TestAStatusReplyBecomesOneRecordPerContact(t *testing.T) {
	ev := osdp.Event{
		Kind:   osdp.EventStatusChange,
		Device: &osdp.Device{Address: 0x00},
		Status: []osdp.StatusChange{
			{Kind: osdp.StatusInput, Index: 2, Active: true},
			{Kind: osdp.StatusTamper, Index: 0, Active: true},
			{Kind: osdp.StatusOutput, Index: 1, Active: false},
		},
	}

	got, err := record.FromEvent(ev, "devices/1/events/1", time.Now())
	if err != nil {
		t.Fatalf("FromEvent: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("produced %d records, want one per contact that moved", len(got))
	}

	for _, rec := range got {
		if rec.GetKind() != eventpbv1.EventKind_EVENT_KIND_STATUS_CHANGE {
			t.Errorf("kind = %v, want status change", rec.GetKind())
		}
	}

	// The tamper is the one that must survive intact: a reader torn off a wall
	// is the report nobody should be able to miss.
	tamper := got[1].GetStatusChange()
	if tamper.GetKind() != eventpbv1.StatusKind_STATUS_KIND_TAMPER || !tamper.GetActive() {
		t.Errorf("tamper record = %+v, want an active tamper", tamper)
	}

	if input := got[0].GetStatusChange(); input.GetIndex() != 2 || !input.GetActive() {
		t.Errorf("input record = %+v, want input 2 active", input)
	}
}

// TestTheTwoStatusEnumerationsAgree. They are numerically equal today, and
// converted rather than cast because equal today is not a guarantee -- a silent
// off-by-one between "tamper" and "power" surfaces only in an incident review.
func TestTheTwoStatusEnumerationsAgree(t *testing.T) {
	for _, tc := range []struct {
		runtime osdp.StatusKind
		schema  eventpbv1.StatusKind
	}{
		{osdp.StatusInput, eventpbv1.StatusKind_STATUS_KIND_INPUT},
		{osdp.StatusOutput, eventpbv1.StatusKind_STATUS_KIND_OUTPUT},
		{osdp.StatusTamper, eventpbv1.StatusKind_STATUS_KIND_TAMPER},
		{osdp.StatusPower, eventpbv1.StatusKind_STATUS_KIND_POWER},
		{osdp.StatusLocal, eventpbv1.StatusKind_STATUS_KIND_LOCAL},
	} {
		ev := osdp.Event{
			Kind:   osdp.EventStatusChange,
			Device: &osdp.Device{},
			Status: []osdp.StatusChange{{Kind: tc.runtime, Active: true}},
		}
		got := one(t, ev)

		if got.GetStatusChange().GetKind() != tc.schema {
			t.Errorf("%v recorded as %v, want %v",
				tc.runtime, got.GetStatusChange().GetKind(), tc.schema)
		}
	}
}

// TestAStatusChangeWithNoContactsIsAnError.
//
// The bus never produces one -- a status reply where nothing moved is not an
// event -- so an empty one means the caller built something the runtime does
// not. Returning no records and no error would let that pass as a successful
// conversion of nothing, which is the shape of bug that shows up as a gap in an
// audit trail months later.
func TestAStatusChangeWithNoContactsIsAnError(t *testing.T) {
	ev := osdp.Event{Kind: osdp.EventStatusChange, Device: &osdp.Device{}}

	got, err := record.FromEvent(ev, "", time.Now())
	if err == nil {
		t.Errorf("FromEvent produced %d records and no error for an empty change", len(got))
	}
}
