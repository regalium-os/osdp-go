// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package record

import (
	"fmt"
	"time"

	"github.com/regalium-os/osdp-go"
	"github.com/regalium-os/osdp-go/protobuf/generated/go/event/v1/eventpbv1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// FromEvent converts a runtime event into the domain record of it.
//
// name is the event's resource name, which the caller allocates because only it
// knows the identity scheme: the schema's pattern is
// devices/{device}/events/{event}, and nothing in the protocol supplies either
// half. An empty name is left empty rather than invented.
//
// The event's Device must not be nil; the runtime never produces one that is.
//
// # What is recorded about time
//
// at is when the panel observed the event, which is the only clock there is:
// OSDP devices do not timestamp their replies. On a bus polled every few tens
// of milliseconds the difference from the device's own notion of now is small,
// but it is a difference, and a forensic reconstruction should know which clock
// it is holding.
//
// # Credentials
//
// Card data and keypad digits are omitted unless WithCredentials is given. The
// format and the bit count are always recorded, because they identify the
// credential technology without identifying the holder.
func FromEvent(ev osdp.Event, name string, at time.Time, opts ...Option) (*eventpbv1.Event, error) {
	kind, err := KindOf(ev.Kind)
	if err != nil {
		return nil, err
	}
	if ev.Device == nil {
		return nil, fmt.Errorf("record: event of kind %v has no device", ev.Kind)
	}

	cfg := settings{}
	for _, opt := range opts {
		opt(&cfg)
	}

	state, secure := ev.Device.SecureSession()
	out := &eventpbv1.Event{
		Name:      name,
		Kind:      kind,
		EventTime: timestamppb.New(at),

		// Authenticated, not merely attempted: a session mid-handshake has
		// proved nothing yet, and recording it as secure would overstate what
		// this evidence is worth.
		Secure: secure && state == osdp.SecureEstablished,
	}

	attachPayload(out, ev, cfg)
	return out, nil
}

// attachPayload sets the one payload field the kind selects.
func attachPayload(out *eventpbv1.Event, ev osdp.Event, cfg settings) {
	switch ev.Kind {
	case osdp.EventCardRead:
		out.Payload = &eventpbv1.Event_CardRead{CardRead: cardRead(ev.Card, cfg)}

	case osdp.EventKeypad:
		out.Payload = &eventpbv1.Event_Keypad{Keypad: keypad(ev.Keypad, cfg)}

	case osdp.EventManufacturer:
		out.Payload = &eventpbv1.Event_Manufacturer{
			Manufacturer: manufacturer(ev.Manufacturer),
		}

	case osdp.EventNAK:
		out.RefusalCode = int32(ev.NAK)
		out.RefusalReason = ev.NAK.String()
	}
}

// cardRead records a credential presentation.
func cardRead(c osdp.CardRead, cfg settings) *eventpbv1.CardRead {
	out := &eventpbv1.CardRead{
		Reader:   int32(c.Reader),
		Format:   int32(c.Format),
		BitCount: int32(c.BitCount),
	}
	if cfg.credentials {
		out.Data = append([]byte(nil), c.Data...)
	}
	return out
}

// keypad records digits entered, which are frequently a PIN.
func keypad(k osdp.KeypadEntry, cfg settings) *eventpbv1.KeypadEntry {
	out := &eventpbv1.KeypadEntry{Reader: int32(k.Reader)}
	if cfg.credentials {
		out.Keys = append([]byte(nil), k.Keys...)
	}
	return out
}

// manufacturer records a vendor message. The body is carried verbatim: it is
// not a credential, and an integrator with vendor documentation can act on what
// this library could not.
func manufacturer(m osdp.ManufacturerMessage) *eventpbv1.ManufacturerEvent {
	return &eventpbv1.ManufacturerEvent{
		Oui:  int32(m.OUI),
		Body: append([]byte(nil), m.Body...),
	}
}
