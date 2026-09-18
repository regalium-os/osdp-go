// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"fmt"
	"math"
	"sort"

	eventpbv1 "github.com/regalium-os/osdp-go/protobuf/generated/go/event/v1/eventpbv1"
	"google.golang.org/protobuf/proto"
)

// Get returns a copy of the named event, or ErrNotFound.
func (s *MemoryStore) Get(ctx context.Context, name string) (*eventpbv1.Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, _, err := ParseEventName(name); err != nil {
		return nil, err
	}

	s.mu.RLock()
	e, ok := s.byName[name]
	s.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	return cloneEvent(e.ev)
}

// cloneEvent deep-copies an event so neither side of the store can mutate what
// the other can still see.
//
// The type assertion is checked rather than written bare. proto.Clone is
// documented to return the same concrete type, but "never panic" is a rule in
// this repository and a rule that holds only while somebody else's doc comment
// does is not one.
func cloneEvent(ev *eventpbv1.Event) (*eventpbv1.Event, error) {
	msg := proto.Clone(ev)
	clone, ok := msg.(*eventpbv1.Event)
	if !ok {
		return nil, fmt.Errorf("service: proto.Clone returned %T, want *eventpbv1.Event", msg)
	}
	return clone, nil
}

// List returns one page of events matching q, newest first.
//
// # How a page is cut
//
// Events are ordered by the sequence number Append allocated, which is the
// order the panel observed them and therefore event_time descending, the
// default ListEventsRequest.order_by documents. A page token names the last
// event returned; the next page holds events strictly older than it.
//
// That bound is what makes paging stable on a bus that is still running.
// Everything arriving during a listing is newer than every token already
// issued, so it appears above the page a client is reading rather than shifting
// rows underneath it. The other direction is not defended against and cannot
// be: a client paging slowly through a store that is evicting will run off the
// bottom, and the listing simply ends. A store that must not lose the tail of a
// page is a store that must not evict.
func (s *MemoryStore) List(ctx context.Context, q Query) (Page, error) {
	if err := ctx.Err(); err != nil {
		return Page{}, err
	}
	device, err := ParseParent(q.Parent)
	if err != nil {
		return Page{}, err
	}

	size := q.PageSize
	if size <= 0 {
		size = DefaultPageSize
	}
	if size > MaxPageSize {
		size = MaxPageSize
	}

	// The bound is exclusive, so with no token every sequence number qualifies:
	// no event can carry the maximum until the counter has wrapped, which is
	// 1.8e19 events away and not a case worth encoding.
	upper := uint64(math.MaxUint64)
	if q.PageToken != "" {
		if upper, err = decodeCursor(q.PageToken, q.Parent); err != nil {
			return Page{}, err
		}
	}

	// One more than asked for, so a full page is distinguished from a full page
	// that happens to be the last one. Without it every listing ends with an
	// empty page, and a client cannot tell "no more" from "ask again".
	s.mu.RLock()
	found := s.page(device, upper, size+1)
	s.mu.RUnlock()

	out := Page{Events: make([]*eventpbv1.Event, 0, min(len(found), size))}
	if len(found) > size {
		out.NextPageToken = encodeCursor(q.Parent, found[size-1].seq)
		found = found[:size]
	}
	for _, e := range found {
		clone, err := cloneEvent(e.ev)
		if err != nil {
			return Page{}, err
		}
		out.Events = append(out.Events, clone)
	}
	return out, nil
}

// page walks the ring newest-first and returns up to want entries below upper,
// belonging to device or to any device when device is Wildcard.
//
// Callers must hold s.mu. The returned entries are immutable, so cloning them
// after the lock is released is safe; see entry.
func (s *MemoryStore) page(device string, upper uint64, want int) []*entry {
	// Sequence numbers increase with logical position, so the first position at
	// or above the cursor bounds the page and everything before it is a
	// candidate. Searching rather than scanning is what keeps a deep page cheap
	// on a store holding a year of traffic.
	start := sort.Search(s.count, func(i int) bool { return s.at(i).seq >= upper }) - 1

	out := make([]*entry, 0, min(want, s.count))
	for i := start; i >= 0 && len(out) < want; i-- {
		if e := s.at(i); device == Wildcard || e.device == device {
			out = append(out, e)
		}
	}
	return out
}

// at returns the entry at logical position i, where 0 is the oldest held.
//
// Callers must hold s.mu. The modulo is taken twice because next-count can be
// negative before the ring has wrapped, and Go's remainder keeps the sign of
// the dividend.
func (s *MemoryStore) at(i int) *entry {
	n := len(s.ring)
	return s.ring[((s.next-s.count+i)%n+n)%n]
}
