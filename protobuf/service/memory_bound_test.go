// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package service_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/regalium-os/osdp-go/protobuf/service"
)

// TestTheStoreForgetsItsOldestEvents.
//
// A panel runs for years. This is the assertion that it does not grow a slice
// of every card read since commissioning, and that what it drops is the oldest
// rather than something arbitrary.
func TestTheStoreForgetsItsOldestEvents(t *testing.T) {
	const capacity, written = 4, 10

	s := newStore(t, capacity)
	names := appendTo(t, s, "devices/3", written)

	if got := s.Len(); got != capacity {
		t.Errorf("Len = %d, want %d: the bound is the point of this store", got, capacity)
	}
	if got := s.Cap(); got != capacity {
		t.Errorf("Cap = %d, want %d", got, capacity)
	}

	for i, name := range names {
		_, err := s.Get(context.Background(), name)
		evicted := i < written-capacity

		switch {
		case evicted && !errors.Is(err, service.ErrNotFound):
			t.Errorf("event %d (%s) survived eviction: err = %v", i, name, err)
		case !evicted && err != nil:
			t.Errorf("event %d (%s) should still be held: %v", i, name, err)
		}
	}
}

// TestIdentifiersAreNeverReused.
//
// The sequence behind an event identifier survives eviction. If it did not, a
// page token cut before an eviction could later name a different event, and a
// client paging through an audit trail would be handed rows it never asked for
// while believing it had asked for exactly those.
func TestIdentifiersAreNeverReused(t *testing.T) {
	s := newStore(t, 4)

	seen := map[string]bool{}
	for round := range 5 {
		for _, name := range appendTo(t, s, "devices/3", 4) {
			if seen[name] {
				t.Fatalf("round %d reused the name %s", round, name)
			}
			seen[name] = true
		}
	}
	if len(seen) != 20 {
		t.Errorf("allocated %d distinct names for 20 appends", len(seen))
	}
}

// TestIdentifiersSortInArrivalOrder: the identifier is the sequence in
// fixed-width hex, so a human reading a log sees the events in the order the
// panel observed them. It is a convenience, not a contract a client may parse,
// which is why this asserts the ordering and not the format.
func TestIdentifiersSortInArrivalOrder(t *testing.T) {
	s := newStore(t, 16)

	names := appendTo(t, s, "devices/3", 16)
	for i := 1; i < len(names); i++ {
		if names[i-1] >= names[i] {
			t.Fatalf("name %q does not sort before %q", names[i-1], names[i])
		}
	}
}

// TestTheStoreIsSafeForConcurrentUse.
//
// This is the case the store exists in, not a corner of it: the poll cycle
// appends from the bus goroutine while gRPC handlers read from theirs. Run
// under the race detector, which is where this test earns its place.
func TestTheStoreIsSafeForConcurrentUse(t *testing.T) {
	const writers, readers, each = 4, 4, 50

	s := newStore(t, 32)
	ctx := context.Background()

	var wg sync.WaitGroup
	for w := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			parent := fmt.Sprintf("devices/%d", w)
			for range each {
				if _, err := s.Append(ctx, parent, cardRecord(t, epoch)); err != nil {
					t.Errorf("Append to %s: %v", parent, err)
					return
				}
			}
		}()
	}

	for range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range each {
				page, err := s.List(ctx, service.Query{Parent: "devices/-", PageSize: 8})
				if err != nil {
					t.Errorf("List: %v", err)
					return
				}
				for _, ev := range page.Events {
					if _, err := s.Get(ctx, ev.GetName()); err != nil &&
						!errors.Is(err, service.ErrNotFound) {
						t.Errorf("Get(%s): %v", ev.GetName(), err)
						return
					}
				}
			}
		}()
	}

	wg.Wait()

	if got := s.Len(); got != 32 {
		t.Errorf("Len = %d after %d appends into a store of 32", got, writers*each)
	}
}
