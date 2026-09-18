// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package service_test

import (
	"errors"
	"testing"

	"github.com/regalium-os/osdp-go/protobuf/service"
)

// TestParseEventNameAcceptsExactlyTheSchemaPattern pins the parser to
// ^devices/[^/]+/events/[^/]+$, which is what event.proto declares on
// Event.name. A name this accepts and protovalidate rejects, or the reverse, is
// a client that passes validation at one hop and fails at the next.
func TestParseEventNameAcceptsExactlyTheSchemaPattern(t *testing.T) {
	tests := []struct {
		name          string
		in            string
		device, event string
		wantErr       bool
	}{
		{name: "canonical", in: "devices/7/events/00000000000000ff", device: "7", event: "00000000000000ff"},
		{name: "identifiers may be words", in: "devices/lobby-north/events/a", device: "lobby-north", event: "a"},
		{name: "empty", in: "", wantErr: true},
		{name: "device collection only", in: "devices/7", wantErr: true},
		{name: "wrong collection", in: "readers/7/events/1", wantErr: true},
		{name: "wrong child collection", in: "devices/7/event/1", wantErr: true},
		{name: "empty device", in: "devices//events/1", wantErr: true},
		{name: "empty event", in: "devices/7/events/", wantErr: true},
		{name: "trailing segment", in: "devices/7/events/1/extra", wantErr: true},
		{name: "leading slash", in: "/devices/7/events/1", wantErr: true},

		// AIP-159: a wildcard is legal only where the API documents it, and
		// GetEvent documents it nowhere. "The event named 1 on any device" has
		// no single answer, so it must not have a single response.
		{name: "wildcard device", in: "devices/-/events/1", wantErr: true},
		{name: "wildcard event", in: "devices/7/events/-", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			device, event, err := service.ParseEventName(tt.in)
			if tt.wantErr {
				if !errors.Is(err, service.ErrInvalidName) {
					t.Fatalf("ParseEventName(%q) error = %v, want ErrInvalidName", tt.in, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseEventName(%q): %v", tt.in, err)
			}
			if device != tt.device || event != tt.event {
				t.Errorf("ParseEventName(%q) = %q/%q, want %q/%q",
					tt.in, device, event, tt.device, tt.event)
			}
		})
	}
}

// TestParseParentAcceptsTheWildcard: "devices/-" is how a panel-wide audit
// trail is read, and ListEventsRequest.parent documents it.
func TestParseParentAcceptsTheWildcard(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		device  string
		wantErr bool
	}{
		{name: "a device", in: "devices/7", device: "7"},
		{name: "every device", in: "devices/-", device: service.Wildcard},
		{name: "empty", in: "", wantErr: true},
		{name: "collection only", in: "devices", wantErr: true},
		{name: "empty identifier", in: "devices/", wantErr: true},
		{name: "an event name", in: "devices/7/events/1", wantErr: true},
		{name: "wrong collection", in: "readers/7", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			device, err := service.ParseParent(tt.in)
			if tt.wantErr {
				if !errors.Is(err, service.ErrInvalidParent) {
					t.Fatalf("ParseParent(%q) error = %v, want ErrInvalidParent", tt.in, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseParent(%q): %v", tt.in, err)
			}
			if device != tt.device {
				t.Errorf("ParseParent(%q) = %q, want %q", tt.in, device, tt.device)
			}
		})
	}
}

// TestNamesRoundTrip: what EventName formats, ParseEventName must read back.
// The two are the only places the pattern is written down, and a drift between
// them is a store whose own names it cannot look up.
func TestNamesRoundTrip(t *testing.T) {
	for _, pair := range [][2]string{
		{"7", "0000000000000001"},
		{"lobby-north", "abc"},
		{"0", "0"},
	} {
		name := service.EventName(pair[0], pair[1])
		device, event, err := service.ParseEventName(name)
		if err != nil {
			t.Fatalf("ParseEventName(%q): %v", name, err)
		}
		if device != pair[0] || event != pair[1] {
			t.Errorf("round trip of %q/%q gave %q/%q", pair[0], pair[1], device, event)
		}
		if got, want := service.DeviceName(pair[0]), "devices/"+pair[0]; got != want {
			t.Errorf("DeviceName(%q) = %q, want %q", pair[0], got, want)
		}
	}
}
