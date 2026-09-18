// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/regalium-os/osdp-go/protobuf/service"
)

// drain pages a listing to its end and returns every name it saw, plus how many
// requests that took. It fails rather than looping forever, because a paging
// bug that never terminates is the one that takes a panel down.
func drain(t *testing.T, s *service.MemoryStore, q service.Query) (names []string, pages int) {
	t.Helper()

	for {
		page, err := s.List(context.Background(), q)
		if err != nil {
			t.Fatalf("List(%+v): %v", q, err)
		}
		pages++
		names = append(names, namesOf(page.Events)...)

		if page.NextPageToken == "" {
			return names, pages
		}
		if pages > 100 {
			t.Fatal("the listing never ran out of pages")
		}
		q.PageToken = page.NextPageToken
	}
}

// TestPagingVisitsEveryEventExactlyOnce, in order, whether or not the total
// divides evenly by the page size. Anything less and an audit trail read
// through this API is not the audit trail.
func TestPagingVisitsEveryEventExactlyOnce(t *testing.T) {
	tests := []struct {
		name      string
		written   int
		size      int
		wantPages int
	}{
		{name: "a partial last page", written: 10, size: 3, wantPages: 4},
		{name: "an exact multiple", written: 9, size: 3, wantPages: 3},
		{name: "one at a time", written: 5, size: 1, wantPages: 5},
		{name: "one page holds it all", written: 4, size: 10, wantPages: 1},
		{name: "nothing stored", written: 0, size: 3, wantPages: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newStore(t, 64)
			want := reversed(appendTo(t, s, "devices/3", tt.written))

			got, pages := drain(t, s, service.Query{Parent: "devices/3", PageSize: tt.size})
			if !equal(got, want) {
				t.Errorf("paged names = %v, want %v", got, want)
			}

			// An exact multiple must not cost an extra empty page: a client
			// cannot tell "ask again" from "no more" if the token outlives the
			// events, so the store looks one event ahead instead.
			if pages != tt.wantPages {
				t.Errorf("took %d requests, want %d", pages, tt.wantPages)
			}
		})
	}
}

// TestPagingAcrossEveryDevice: the wildcard listing pages like any other, and
// the events interleave in the order the panel observed them rather than
// grouping by device.
func TestPagingAcrossEveryDevice(t *testing.T) {
	s := newStore(t, 64)

	var order []string
	for range 4 {
		order = append(order, appendTo(t, s, "devices/3", 1)...)
		order = append(order, appendTo(t, s, "devices/4", 1)...)
	}

	got, _ := drain(t, s, service.Query{Parent: "devices/-", PageSize: 3})
	if want := reversed(order); !equal(got, want) {
		t.Errorf("wildcard paging = %v, want %v", got, want)
	}
}

// TestAForeignTokenIsRefused.
//
// ListEventsRequest.page_token says all other arguments must match the call
// that produced it. A token answering silently under another parent would hand
// a client one device's trail while it believed it was reading another's --
// wrong evidence that looks like right evidence, which on an audit API is worse
// than an error.
func TestAForeignTokenIsRefused(t *testing.T) {
	s := newStore(t, 64)
	appendTo(t, s, "devices/3", 10)
	appendTo(t, s, "devices/4", 10)

	first, err := s.List(context.Background(), service.Query{Parent: "devices/3", PageSize: 3})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	tests := []struct {
		name   string
		parent string
		token  string
	}{
		{name: "another device", parent: "devices/4", token: first.NextPageToken},
		{name: "the wildcard", parent: "devices/-", token: first.NextPageToken},
		{name: "not a token at all", parent: "devices/3", token: "nonsense"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.List(context.Background(),
				service.Query{Parent: tt.parent, PageToken: tt.token})
			if !errors.Is(err, service.ErrInvalidPageToken) {
				t.Fatalf("List error = %v, want ErrInvalidPageToken", err)
			}
		})
	}
}

// TestEventsArrivingDuringAListingDoNotShiftThePage.
//
// The bus does not stop while a client pages. Everything that arrives is newer
// than every token already issued, so it appears above the page being read
// rather than sliding rows the client has not reached yet.
func TestEventsArrivingDuringAListingDoNotShiftThePage(t *testing.T) {
	s := newStore(t, 64)
	before := reversed(appendTo(t, s, "devices/3", 6))

	first, err := s.List(context.Background(), service.Query{Parent: "devices/3", PageSize: 3})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	// The reader badges three more people in while the client reads page one.
	appendTo(t, s, "devices/3", 3)

	rest, _ := drain(t, s, service.Query{
		Parent: "devices/3", PageSize: 3, PageToken: first.NextPageToken,
	})

	got := append(namesOf(first.Events), rest...)
	if !equal(got, before) {
		t.Errorf("the listing returned %v, want exactly the six events it started with, %v",
			got, before)
	}
}
