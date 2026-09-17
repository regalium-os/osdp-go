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

// StatusChange is not in the table above because the runtime does not yet
// report one: osdp_ISTATR, osdp_OSTATR and osdp_LSTATR are decoded by no part
// of the poll cycle. The schema kind exists and is unused, which is the honest
// state of it -- when the bus learns to report a contact change, this is where
// it joins.
var _ = eventpbv1.EventKind_EVENT_KIND_STATUS_CHANGE
