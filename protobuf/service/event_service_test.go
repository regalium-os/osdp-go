// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package service_test

import (
	"context"
	"testing"

	eventpbv1 "github.com/regalium-os/osdp-go/protobuf/generated/go/event/v1/eventpbv1"
	"github.com/regalium-os/osdp-go/protobuf/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestTheGeneratedServerInterfaceIsSatisfied.
//
// The package asserts the type at compile time. This asserts the dispatch,
// which is a different thing: EventServiceServer is satisfied by embedding
// UnimplementedEventServiceServer, so a handler declared with the wrong
// signature still compiles and answers every call with Unimplemented. Calling
// through the interface is what catches that.
func TestTheGeneratedServerInterfaceIsSatisfied(t *testing.T) {
	store := newStore(t, 4)
	names := appendTo(t, store, "devices/3", 1)

	var srv eventpbv1.EventServiceServer = newService(t, store)

	if _, err := srv.GetEvent(context.Background(),
		&eventpbv1.GetEventRequest{Name: names[0]}); err != nil {
		t.Errorf("GetEvent through the generated interface: %v", err)
	}
	if _, err := srv.ListEvents(context.Background(),
		&eventpbv1.ListEventsRequest{Parent: "devices/3"}); err != nil {
		t.Errorf("ListEvents through the generated interface: %v", err)
	}
}

// TestNewEventServiceRefusesNoStore: there is no default worth guessing for
// where a panel's audit trail lives.
func TestNewEventServiceRefusesNoStore(t *testing.T) {
	if _, err := service.NewEventService(nil); err == nil {
		t.Error("NewEventService(nil) succeeded; a service with no store serves nothing")
	}
}

// TestGetEventStatusCodes.
//
// A client must be able to tell "you asked wrongly" from "there is no such
// event" without reading the message, which is what the codes are for.
func TestGetEventStatusCodes(t *testing.T) {
	store := newStore(t, 8)
	names := appendTo(t, store, "devices/3", 1)
	svc := newService(t, store)

	tests := []struct {
		name string
		in   string
		want codes.Code
	}{
		{name: "stored", in: names[0], want: codes.OK},
		{name: "never stored", in: "devices/3/events/00000000deadbeef", want: codes.NotFound},
		{name: "unknown device", in: "devices/99/events/0000000000000000", want: codes.NotFound},
		{name: "malformed", in: "devices/3/events", want: codes.InvalidArgument},
		{name: "empty", in: "", want: codes.InvalidArgument},
		{name: "a parent, not an event", in: "devices/3", want: codes.InvalidArgument},

		// AIP-159 again: ListEvents documents the wildcard for parent only.
		{name: "wildcard device", in: "devices/-/events/0000000000000000", want: codes.InvalidArgument},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.GetEvent(context.Background(),
				&eventpbv1.GetEventRequest{Name: tt.in})

			if code := status.Code(err); code != tt.want {
				t.Fatalf("GetEvent(%q) code = %v (%v), want %v", tt.in, code, err, tt.want)
			}
			if tt.want == codes.OK && got.GetName() != tt.in {
				t.Errorf("returned %q, want %q", got.GetName(), tt.in)
			}
		})
	}
}

// TestGetEventSurvivesANilRequest. gRPC does not deliver one, but "never panic"
// is a rule here and a handler is exported: an in-process caller reaching for
// the service directly is the case that finds this.
func TestGetEventSurvivesANilRequest(t *testing.T) {
	svc := newService(t, newStore(t, 4))

	if _, err := svc.GetEvent(context.Background(), nil); status.Code(err) != codes.InvalidArgument {
		t.Errorf("GetEvent(nil) code = %v, want InvalidArgument", status.Code(err))
	}
	if _, err := svc.ListEvents(context.Background(), nil); status.Code(err) != codes.InvalidArgument {
		t.Errorf("ListEvents(nil) code = %v, want InvalidArgument", status.Code(err))
	}
}
