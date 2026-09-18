// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package service_test

import (
	"context"
	"testing"

	eventpbv1 "github.com/regalium-os/osdp-go/protobuf/generated/go/event/v1/eventpbv1"
	"github.com/regalium-os/osdp-go/protobuf/service"
)

// namesOf extracts the resource names of a page, in the order it returned them.
func namesOf(events []*eventpbv1.Event) []string {
	out := make([]string, 0, len(events))
	for _, ev := range events {
		out = append(out, ev.GetName())
	}
	return out
}

// reversed copies names newest-first, which is the order a listing returns.
func reversed(names []string) []string {
	out := make([]string, len(names))
	for i, name := range names {
		out[len(names)-1-i] = name
	}
	return out
}

// TestListReturnsNewestFirst. ListEventsRequest.order_by documents the default
// as event_time descending, and an operator watching a door wants the last
// thing that happened at the top, not the first thing that ever did.
func TestListReturnsNewestFirst(t *testing.T) {
	s := newStore(t, 16)
	names := appendTo(t, s, "devices/3", 5)

	page, err := s.List(context.Background(), service.Query{Parent: "devices/3"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got, want := namesOf(page.Events), reversed(names); !equal(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
	if page.NextPageToken != "" {
		t.Errorf("a five-event listing under the default page size returned a token %q",
			page.NextPageToken)
	}
}

// TestListSeparatesDevicesAndTheWildcardJoinsThem. "devices/-" is how a
// panel-wide audit trail is read; anything else is one device's.
func TestListSeparatesDevicesAndTheWildcardJoinsThem(t *testing.T) {
	s := newStore(t, 32)
	three := appendTo(t, s, "devices/3", 3)
	four := appendTo(t, s, "devices/4", 2)

	tests := []struct {
		name   string
		parent string
		want   []string
	}{
		{name: "one device", parent: "devices/3", want: reversed(three)},
		{name: "the other", parent: "devices/4", want: reversed(four)},
		{name: "every device", parent: "devices/-", want: reversed(append(append([]string{}, three...), four...))},
		{name: "a device with nothing", parent: "devices/9", want: []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, err := s.List(context.Background(), service.Query{Parent: tt.parent})
			if err != nil {
				t.Fatalf("List(%s): %v", tt.parent, err)
			}
			if got := namesOf(page.Events); !equal(got, tt.want) {
				t.Errorf("List(%s) = %v, want %v", tt.parent, got, tt.want)
			}
		})
	}
}

// TestListBoundsThePageSizeItself.
//
// The store defends its own bound rather than trusting the service to have
// applied one: a panel application calling Store.List directly, to render a
// local console, must not be able to ask for the whole ring by accident.
func TestListBoundsThePageSizeItself(t *testing.T) {
	s := newStore(t, 200)
	appendTo(t, s, "devices/3", 120)

	tests := []struct {
		name     string
		size     int
		want     int
		wantMore bool
	}{
		{name: "zero takes the default", size: 0, want: service.DefaultPageSize, wantMore: true},
		{name: "negative takes the default", size: -5, want: service.DefaultPageSize, wantMore: true},
		{name: "an ordinary size", size: 10, want: 10, wantMore: true},
		{name: "above the ceiling is capped", size: service.MaxPageSize + 1, want: 120},
		{name: "more than is held", size: 500, want: 120},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, err := s.List(context.Background(),
				service.Query{Parent: "devices/3", PageSize: tt.size})
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if got := len(page.Events); got != tt.want {
				t.Errorf("page held %d events, want %d", got, tt.want)
			}
			if got := page.NextPageToken != ""; got != tt.wantMore {
				t.Errorf("next_page_token present = %v, want %v", got, tt.wantMore)
			}
		})
	}
}

// equal compares two name slices.
func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
