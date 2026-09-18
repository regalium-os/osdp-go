// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package service_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/regalium-os/osdp-go/protobuf/service"
)

// TestNewMemoryStoreRefusesANonPositiveCapacity: an unbounded store is the leak
// this type exists to prevent, so there is no spelling of "no limit".
func TestNewMemoryStoreRefusesANonPositiveCapacity(t *testing.T) {
	for _, capacity := range []int{0, -1, -1000} {
		if _, err := service.NewMemoryStore(capacity); err == nil {
			t.Errorf("NewMemoryStore(%d) succeeded; a store with no bound is the leak", capacity)
		}
	}
}

// TestAppendAllocatesTheName: the store owns identity, because only it knows
// which identifiers it has already used, and a page token that landed on a
// reused name would return a different event than the one it was cut from.
func TestAppendAllocatesTheName(t *testing.T) {
	s := newStore(t, 8)
	in := cardRecord(t, epoch)
	in.Name = "devices/99/events/somebody-elses-idea"

	stored, err := s.Append(context.Background(), "devices/3", in)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	device, event, err := service.ParseEventName(stored.GetName())
	if err != nil {
		t.Fatalf("the store allocated %q, which is not a valid name: %v", stored.GetName(), err)
	}
	if device != "3" {
		t.Errorf("name is under device %q, want the parent's device 3", device)
	}
	if event == "" {
		t.Error("the event identifier is empty")
	}

	// create_time is the store's, event_time is the caller's: only the panel
	// knows when it observed the event, and only the store knows when it
	// wrote it down.
	if stored.GetCreateTime() == nil {
		t.Error("create_time was not stamped")
	}
	if got, want := stored.GetEventTime().AsTime(), epoch; !got.Equal(want) {
		t.Errorf("event_time = %v, want the caller's %v", got, want)
	}
}

// TestAppendRefusesAParentItCannotFile: an event belongs to one device. A
// wildcard is a question, not an address.
func TestAppendRefusesAParentItCannotFile(t *testing.T) {
	s := newStore(t, 8)

	tests := []struct {
		name   string
		parent string
	}{
		{name: "wildcard", parent: "devices/-"},
		{name: "empty", parent: ""},
		{name: "an event name", parent: "devices/3/events/1"},
		{name: "wrong collection", parent: "readers/3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.Append(context.Background(), tt.parent, cardRecord(t, epoch))
			if !errors.Is(err, service.ErrInvalidParent) {
				t.Fatalf("Append to %q error = %v, want ErrInvalidParent", tt.parent, err)
			}
		})
	}
}

// TestTheStoreDoesNotAliasTheCallersEvent.
//
// The runtime reuses its read buffer between poll cycles, so a store holding a
// slice into a card read would watch its audit trail change underneath it. This
// is the assertion that the copy is real in both directions.
func TestTheStoreDoesNotAliasTheCallersEvent(t *testing.T) {
	s := newStore(t, 8)
	in := cardRecord(t, epoch)

	stored, err := s.Append(context.Background(), "devices/3", in)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	// The caller reuses its buffer, as the runtime does.
	in.GetCardRead().GetData()[0] ^= 0xFF
	in.Name = "devices/3/events/rewritten"

	// And the caller mutates what Append handed back.
	stored.GetCardRead().GetData()[1] ^= 0xFF

	got, err := s.Get(context.Background(), stored.GetName())
	if err != nil {
		t.Fatalf("Get after the caller scribbled on its copies: %v", err)
	}
	if !bytes.Equal(got.GetCardRead().GetData(), credential) {
		t.Errorf("stored credential = % X, want % X: the store aliased a caller's slice",
			got.GetCardRead().GetData(), credential)
	}
}

// TestGetDistinguishesAMalformedNameFromAMissingEvent. The two mean opposite
// things: one is a bug in the client, the other is the ordinary answer from a
// store that has forgotten.
func TestGetDistinguishesAMalformedNameFromAMissingEvent(t *testing.T) {
	s := newStore(t, 8)
	names := appendTo(t, s, "devices/3", 1)

	tests := []struct {
		name string
		in   string
		want error
	}{
		{name: "stored", in: names[0], want: nil},
		{name: "never stored", in: "devices/3/events/0000000000ffffff", want: service.ErrNotFound},
		{name: "another device", in: "devices/9/events/0000000000000000", want: service.ErrNotFound},
		{name: "malformed", in: "devices/3/events", want: service.ErrInvalidName},
		{name: "wildcard device", in: "devices/-/events/0000000000000000", want: service.ErrInvalidName},
		{name: "empty", in: "", want: service.ErrInvalidName},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.Get(context.Background(), tt.in)
			if !errors.Is(err, tt.want) {
				t.Fatalf("Get(%q) error = %v, want %v", tt.in, err, tt.want)
			}
		})
	}
}
