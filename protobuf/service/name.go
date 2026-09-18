// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"errors"
	"strings"
)

// The collection identifiers the schema names. Event declares
//
//	pattern: "devices/{device}/events/{event}"
//
// in osdp/event/v1/event.proto, and house style is {device} rather than
// {device_id}: the segment holds the identifier, so repeating the word in the
// placeholder says nothing the pattern did not already say.
const (
	devicesCollection = "devices"
	eventsCollection  = "events"
)

// Wildcard is the parent segment that lists across every device.
//
// Per AIP-159 a wildcard is legal only where the API documents it, and
// ListEventsRequest.parent is the only place this one does: "devices/-" is how
// a panel-wide audit trail is read. GetEvent does not accept it, because the
// answer to "fetch the event named on any device" is not a single event.
const Wildcard = "-"

// ErrInvalidName reports a resource name that is not
// devices/{device}/events/{event}.
//
// It is a distinct sentinel from ErrNotFound because the two mean opposite
// things to a caller: a malformed name is a bug in the client, a missing event
// is the ordinary answer for a store that has forgotten it.
var ErrInvalidName = errors.New("service: malformed event resource name")

// ErrInvalidParent reports a parent that is not devices/{device}.
var ErrInvalidParent = errors.New("service: malformed parent resource name")

// EventName formats the resource name of an event.
//
// It does not validate: the identifiers come from the store that allocated
// them, and a store that allocates a segment containing a slash has a bug that
// a silent error return here would hide rather than fix. Callers handling
// client input parse instead.
func EventName(device, event string) string {
	return devicesCollection + "/" + device + "/" + eventsCollection + "/" + event
}

// DeviceName formats the parent resource name of a device's event collection.
func DeviceName(device string) string {
	return devicesCollection + "/" + device
}

// ParseEventName splits an event resource name into its device and event
// identifiers.
//
// The accepted shape is exactly the schema's pattern,
// ^devices/[^/]+/events/[^/]+$, so a name this function accepts is a name
// protovalidate accepts and vice versa. Wildcards are rejected in both
// positions; see Wildcard.
func ParseEventName(name string) (device, event string, err error) {
	parts := strings.Split(name, "/")
	if len(parts) != 4 ||
		parts[0] != devicesCollection || parts[2] != eventsCollection ||
		!validSegment(parts[1]) || !validSegment(parts[3]) {
		return "", "", ErrInvalidName
	}
	return parts[1], parts[3], nil
}

// ParseParent splits a parent resource name into its device identifier.
//
// The returned identifier is Wildcard when the caller asked for every device.
// Callers must handle that case rather than treating it as a device that
// happens to be called "-": no OSDP device can be, because the panel allocates
// these identifiers, but a Store implementation backed by a table somebody else
// writes to could see one.
func ParseParent(parent string) (device string, err error) {
	parts := strings.Split(parent, "/")
	if len(parts) != 2 || parts[0] != devicesCollection || parts[1] == "" {
		return "", ErrInvalidParent
	}
	if parts[1] != Wildcard && !validSegment(parts[1]) {
		return "", ErrInvalidParent
	}
	return parts[1], nil
}

// validSegment reports whether s may stand as one identifier in a resource
// name.
//
// Non-empty and not the wildcard is the whole rule. The absence of a slash is
// already guaranteed by the caller having split on it, and the schema's pattern
// constrains nothing further -- a device identifier is whatever the panel's
// enrolment scheme allocates, which is not this package's business to narrow.
func validSegment(s string) bool {
	return s != "" && s != Wildcard
}
