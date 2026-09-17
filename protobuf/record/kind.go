// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package record

import (
	"fmt"

	"github.com/regalium-os/osdp-go"
	"github.com/regalium-os/osdp-go/protobuf/generated/go/event/v1/eventpbv1"
)

// kinds maps what the runtime reports to what the schema records.
//
// Not every runtime event is a domain event. Enrolment steps -- a device
// identifying itself, reporting its capabilities, completing a handshake -- are
// how the panel gets a device ready, not things that happened at a door. They
// are deliberately absent, and ErrNotRecordable says so rather than writing an
// EVENT_KIND_UNSPECIFIED row that means nothing to whoever reads the audit
// trail later.
var kinds = map[osdp.EventKind]eventpbv1.EventKind{
	osdp.EventCardRead:     eventpbv1.EventKind_EVENT_KIND_CARD_READ,
	osdp.EventKeypad:       eventpbv1.EventKind_EVENT_KIND_KEYPAD,
	osdp.EventOnline:       eventpbv1.EventKind_EVENT_KIND_DEVICE_ONLINE,
	osdp.EventOffline:      eventpbv1.EventKind_EVENT_KIND_DEVICE_OFFLINE,
	osdp.EventNAK:          eventpbv1.EventKind_EVENT_KIND_COMMAND_REFUSED,
	osdp.EventResync:       eventpbv1.EventKind_EVENT_KIND_RESYNC,
	osdp.EventManufacturer: eventpbv1.EventKind_EVENT_KIND_MANUFACTURER,
	osdp.EventSecureFailed: eventpbv1.EventKind_EVENT_KIND_SECURITY_VIOLATION,
	osdp.EventStatusChange: eventpbv1.EventKind_EVENT_KIND_STATUS_CHANGE,
}

// ErrNotRecordable reports an event the schema has no kind for.
//
// It is an error rather than a silently skipped row because the caller is the
// only one who can decide what to do about it: a panel streaming everything to
// an audit log wants to know it dropped something, and one filtering to door
// activity already knows.
var ErrNotRecordable = fmt.Errorf("record: this event kind has no domain record")

// Recordable reports whether an event kind becomes a domain record.
//
// Use it to filter a stream before converting, rather than converting and
// discarding errors -- an error you expect is an error you stop reading.
func Recordable(kind osdp.EventKind) bool {
	_, ok := kinds[kind]
	return ok
}

// KindOf returns the schema kind for a runtime event kind.
func KindOf(kind osdp.EventKind) (eventpbv1.EventKind, error) {
	k, ok := kinds[kind]
	if !ok {
		return eventpbv1.EventKind_EVENT_KIND_UNSPECIFIED,
			fmt.Errorf("%w: %v", ErrNotRecordable, kind)
	}
	return k, nil
}

// statusKinds maps a runtime contact kind to the schema's.
//
// The two enumerations are kept numerically equal on purpose, but they are
// converted rather than cast: equal today is not a guarantee, and a silent
// off-by-one between "tamper" and "power" is the kind of mistake that only
// surfaces in an incident review.
var statusKinds = map[osdp.StatusKind]eventpbv1.StatusKind{
	osdp.StatusInput:  eventpbv1.StatusKind_STATUS_KIND_INPUT,
	osdp.StatusOutput: eventpbv1.StatusKind_STATUS_KIND_OUTPUT,
	osdp.StatusTamper: eventpbv1.StatusKind_STATUS_KIND_TAMPER,
	osdp.StatusPower:  eventpbv1.StatusKind_STATUS_KIND_POWER,
	osdp.StatusLocal:  eventpbv1.StatusKind_STATUS_KIND_LOCAL,
}
