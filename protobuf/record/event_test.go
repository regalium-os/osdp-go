// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package record_test

import (
	"errors"
	"testing"
	"time"

	"github.com/regalium-os/osdp-go"
	eventpbv1 "github.com/regalium-os/osdp-go/protobuf/generated/go/event/v1/eventpbv1"
	"github.com/regalium-os/osdp-go/protobuf/record"
)

// credential is a 26-bit Wiegand card number, the commonest thing on a bus and
// the thing that must not leave the panel by accident.
var credential = []byte{0xAB, 0xCD, 0xEF, 0x80}

// cardEvent is what the runtime hands over when somebody badges in.
func cardEvent() osdp.Event {
	return osdp.Event{
		Kind:   osdp.EventCardRead,
		Device: &osdp.Device{Address: 0x00},
		Card: osdp.CardRead{
			Reader: 0, Format: 1, BitCount: 26, Data: credential,
		},
	}
}

// TestACredentialIsNotRecordedUnlessAsked is the assertion that matters most in
// this package. A record leaves the panel: over a network, into a database,
// into a backup, and into whatever reads that backup in five years.
func TestACredentialIsNotRecordedUnlessAsked(t *testing.T) {
	got, err := record.FromEvent(cardEvent(), "devices/1/events/1", time.Now())
	if err != nil {
		t.Fatalf("FromEvent: %v", err)
	}

	card := got.GetCardRead()
	if card == nil {
		t.Fatal("a card read recorded no card payload at all")
	}
	if len(card.GetData()) != 0 {
		t.Errorf("the credential was recorded by default: % X", card.GetData())
	}

	// What identifies the technology is kept; what identifies the holder is not.
	if card.GetBitCount() != 26 || card.GetFormat() != 1 {
		t.Errorf("format/bit count = %d/%d, want 1/26: these are not personal data",
			card.GetFormat(), card.GetBitCount())
	}
}

// TestCredentialsAreRecordedWhenAskedFor: a panel deciding access centrally has
// to send the number somewhere, and for those this is correct.
func TestCredentialsAreRecordedWhenAskedFor(t *testing.T) {
	got, err := record.FromEvent(cardEvent(), "devices/1/events/1", time.Now(),
		record.WithCredentials())
	if err != nil {
		t.Fatalf("FromEvent: %v", err)
	}

	if want := credential; string(got.GetCardRead().GetData()) != string(want) {
		t.Errorf("data = % X, want % X", got.GetCardRead().GetData(), want)
	}
}

// TestTheRecordDoesNotAliasTheEvent: the runtime reuses its read buffer between
// poll cycles, so a record holding a slice into it would change under whoever
// stored it.
func TestTheRecordDoesNotAliasTheEvent(t *testing.T) {
	ev := cardEvent()
	ev.Card.Data = append([]byte(nil), credential...)

	got, err := record.FromEvent(ev, "devices/1/events/1", time.Now(),
		record.WithCredentials())
	if err != nil {
		t.Fatalf("FromEvent: %v", err)
	}

	ev.Card.Data[0] = 0x00 // the next poll cycle overwrites the buffer
	if got.GetCardRead().GetData()[0] != 0xAB {
		t.Error("the record aliases the event's buffer; it changed underneath")
	}
}

// TestAnEnrolmentStepIsNotADomainEvent. A device identifying itself is how the
// panel gets it ready, not something that happened at a door -- and a row
// reading EVENT_KIND_UNSPECIFIED means nothing to whoever reads the audit trail.
func TestAnEnrolmentStepIsNotADomainEvent(t *testing.T) {
	for _, kind := range []osdp.EventKind{
		osdp.EventIdentified, osdp.EventCapabilities, osdp.EventSecure, osdp.EventNone,
	} {
		if record.Recordable(kind) {
			t.Errorf("%v is reported as recordable", kind)
		}

		ev := osdp.Event{Kind: kind, Device: &osdp.Device{}}
		if _, err := record.FromEvent(ev, "", time.Now()); !errors.Is(err, record.ErrNotRecordable) {
			t.Errorf("FromEvent(%v) = %v, want ErrNotRecordable", kind, err)
		}
	}
}

// TestARefusalRecordsWhatTheDeviceSaid, in both the code and a form somebody
// reading the audit trail can act on.
func TestARefusalRecordsWhatTheDeviceSaid(t *testing.T) {
	ev := osdp.Event{
		Kind:   osdp.EventNAK,
		Device: &osdp.Device{},
		NAK:    osdp.NAKReason(0x05), // secure channel required
	}

	got, err := record.FromEvent(ev, "", time.Now())
	if err != nil {
		t.Fatalf("FromEvent: %v", err)
	}
	if got.GetKind() != eventpbv1.EventKind_EVENT_KIND_COMMAND_REFUSED {
		t.Errorf("kind = %v, want command refused", got.GetKind())
	}
	if got.GetRefusalCode() != 5 || got.GetRefusalReason() == "" {
		t.Errorf("refusal = %d %q, want 5 and an explanation",
			got.GetRefusalCode(), got.GetRefusalReason())
	}
}

// TestAnUnauthenticatedExchangeIsRecordedAsSuch.
//
// A credential that arrived in the clear is a different piece of evidence from
// one that arrived over a secure channel, and after the fact there is no way to
// tell them apart unless it was written down at the time.
//
// Only the negative case is exercised here: constructing a device with an
// established session means driving a full four-message handshake, which the
// bus's own tests do against a peripheral holding the key. What is checked is
// that a device with no session is never recorded as secure -- the direction
// that would overstate the evidence.
func TestAnUnauthenticatedExchangeIsRecordedAsSuch(t *testing.T) {
	got, err := record.FromEvent(cardEvent(), "", time.Now())
	if err != nil {
		t.Fatalf("FromEvent: %v", err)
	}
	if got.GetSecure() {
		t.Error("a device with no secure session was recorded as secure")
	}
}

// TestADeviceIsRequired: the runtime never produces an event without one, and
// a record naming no device is not evidence of anything.
func TestADeviceIsRequired(t *testing.T) {
	if _, err := record.FromEvent(osdp.Event{Kind: osdp.EventOffline}, "", time.Now()); err == nil {
		t.Error("an event with no device was converted")
	}
}
