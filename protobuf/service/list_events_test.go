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

// TestListEventsStatusCodes covers everything the handler refuses and why.
func TestListEventsStatusCodes(t *testing.T) {
	store := newStore(t, 32)
	appendTo(t, store, "devices/3", 4)
	svc := newService(t, store)

	tests := []struct {
		name string
		req  *eventpbv1.ListEventsRequest
		want codes.Code
	}{
		{
			name: "a device",
			req:  &eventpbv1.ListEventsRequest{Parent: "devices/3"},
			want: codes.OK,
		},
		{
			name: "every device",
			req:  &eventpbv1.ListEventsRequest{Parent: "devices/-"},
			want: codes.OK,
		},
		{
			name: "malformed parent",
			req:  &eventpbv1.ListEventsRequest{Parent: "devices/3/events/1"},
			want: codes.InvalidArgument,
		},
		{
			name: "empty parent",
			req:  &eventpbv1.ListEventsRequest{},
			want: codes.InvalidArgument,
		},
		{
			// The schema's own bound says gte 0. A client asking for minus five
			// events has a bug, and capping it would hide the bug.
			name: "negative page size",
			req:  &eventpbv1.ListEventsRequest{Parent: "devices/3", PageSize: -5},
			want: codes.InvalidArgument,
		},
		{
			// Per AIP-158 an oversized request is capped, not refused: the
			// response says how many came back and offers a token.
			name: "oversized page size",
			req:  &eventpbv1.ListEventsRequest{Parent: "devices/3", PageSize: service.MaxPageSize * 10},
			want: codes.OK,
		},
		{
			name: "a token this store never issued",
			req:  &eventpbv1.ListEventsRequest{Parent: "devices/3", PageToken: "nonsense"},
			want: codes.InvalidArgument,
		},
		{
			// Unimplemented rather than InvalidArgument: the request is well
			// formed, and calling it malformed sends somebody hunting for a
			// syntax error that is not there.
			name: "a filter",
			req: &eventpbv1.ListEventsRequest{
				Parent: "devices/3", Filter: "kind = EVENT_KIND_CARD_READ",
			},
			want: codes.Unimplemented,
		},
		{
			name: "another sort order",
			req:  &eventpbv1.ListEventsRequest{Parent: "devices/3", OrderBy: "event_time asc"},
			want: codes.Unimplemented,
		},
		{
			// AIP-132 spells the documented default this way. A client asking
			// for what it would have got anyway gets it.
			name: "the default sort order, spelled out",
			req:  &eventpbv1.ListEventsRequest{Parent: "devices/3", OrderBy: "event_time desc"},
			want: codes.OK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.ListEvents(context.Background(), tt.req)
			if code := status.Code(err); code != tt.want {
				t.Fatalf("ListEvents code = %v (%v), want %v", code, err, tt.want)
			}
		})
	}
}

// TestThePageSizeIsTheDeploymentsToSet.
//
// The ceiling belongs to the deployment rather than to the schema: a panel on a
// slow uplink cannot send a thousand events however politely it is asked, and
// enforcing that here means a client cannot talk it into assembling a page it
// will fail to deliver.
func TestThePageSizeIsTheDeploymentsToSet(t *testing.T) {
	store := newStore(t, 64)
	appendTo(t, store, "devices/3", 30)

	tests := []struct {
		name      string
		opts      []service.Option
		requested int32
		want      int
	}{
		{name: "service default", requested: 0, want: 30},
		{
			name: "a configured default",
			opts: []service.Option{service.WithDefaultPageSize(4)},
			want: 4,
		},
		{
			name:      "a configured cap beats the request",
			opts:      []service.Option{service.WithMaxPageSize(5)},
			requested: 25,
			want:      5,
		},
		{
			name:      "under the cap the request stands",
			opts:      []service.Option{service.WithMaxPageSize(20)},
			requested: 7,
			want:      7,
		},
		{
			// A default above the cap is a misconfiguration, not a licence to
			// exceed the cap.
			name: "a default above the cap is clamped to it",
			opts: []service.Option{
				service.WithDefaultPageSize(100), service.WithMaxPageSize(6),
			},
			want: 6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newService(t, store, tt.opts...)

			got, err := svc.ListEvents(context.Background(), &eventpbv1.ListEventsRequest{
				Parent: "devices/3", PageSize: tt.requested,
			})
			if err != nil {
				t.Fatalf("ListEvents: %v", err)
			}
			if n := len(got.GetEvents()); n != tt.want {
				t.Errorf("page held %d events, want %d", n, tt.want)
			}
		})
	}
}

// TestListEventsPagesToTheEnd over the RPC surface, which is the shape a client
// actually sees: events, a token, and eventually no token.
func TestListEventsPagesToTheEnd(t *testing.T) {
	store := newStore(t, 64)
	want := reversed(appendTo(t, store, "devices/3", 7))
	svc := newService(t, store)

	var got []string
	req := &eventpbv1.ListEventsRequest{Parent: "devices/3", PageSize: 2}
	for range 10 {
		resp, err := svc.ListEvents(context.Background(), req)
		if err != nil {
			t.Fatalf("ListEvents: %v", err)
		}
		got = append(got, namesOf(resp.GetEvents())...)
		if resp.GetNextPageToken() == "" {
			break
		}
		req.PageToken = resp.GetNextPageToken()
	}

	if !equal(got, want) {
		t.Errorf("paged names = %v, want %v", got, want)
	}
}
