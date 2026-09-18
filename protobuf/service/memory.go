// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"fmt"
	"sync"
	"time"

	eventpbv1 "github.com/regalium-os/osdp-go/protobuf/generated/go/event/v1/eventpbv1"
)

// StoreOption configures a MemoryStore. Adding one never breaks a caller.
type StoreOption func(*storeSettings)

// storeSettings are the options a store was built with.
type storeSettings struct{ now func() time.Time }

// WithClock supplies the clock a store stamps create_time from.
//
// Time is injected here for the same reason it is injected in the runtime: a
// retention test that has to wait for a real second is a test nobody runs. The
// default is time.Now, which is correct for a panel and useless for a table
// test.
//
// The function must be safe for concurrent use. It is called outside the
// store's lock, deliberately -- a caller-supplied function running under a lock
// the poll cycle needs is how a slow clock becomes a stalled bus -- so two
// appends racing may stamp create_time out of order with respect to each
// other. Ordering is carried by the sequence number, never by that timestamp.
func WithClock(now func() time.Time) StoreOption {
	return func(s *storeSettings) {
		if now != nil {
			s.now = now
		}
	}
}

// entry is one stored event and the ordering key it was given.
//
// An entry is immutable once appended. That is what lets a reader clone one
// after releasing the lock: eviction unlinks an entry, it never rewrites one,
// so a goroutine holding a pointer to an evicted entry is reading memory
// nobody will touch again rather than racing the writer.
type entry struct {
	seq    uint64
	device string
	ev     *eventpbv1.Event
}

// MemoryStore keeps the most recent events in memory and forgets the rest.
//
// # Why it is bounded
//
// A panel runs for years. An unbounded slice of every card read since
// commissioning is not a feature, it is the leak that takes the bus down at
// three in the morning, so this store holds a fixed number of events and
// discards the oldest to make room. A Get for an evicted event is ErrNotFound,
// which is the same answer as for one that never existed, deliberately: this
// store promises recency, not retention.
//
// It is therefore the right store for a commissioning bench, a demo panel, or
// the recent-activity view an operator watches, and the wrong store for an
// access log anybody has to answer for. Those want a Store backed by something
// durable, which is why Store is an interface.
//
// # Concurrency
//
// Safe for concurrent use. Appends from the poll cycle and reads from gRPC
// handlers may run at once; no caller needs its own lock.
//
// # Ownership
//
// It copies what it is given and returns copies of what it holds, so the byte
// slices inside an event -- a card read's data, a manufacturer body -- are
// never shared with the runtime buffers they came from.
type MemoryStore struct {
	now func() time.Time

	mu     sync.RWMutex
	ring   []*entry // fixed length; nil in slots not yet written
	next   int      // ring index the next append writes
	count  int      // entries currently held, never above len(ring)
	seq    uint64   // next sequence number; monotonic across evictions
	byName map[string]*entry
}

// NewMemoryStore returns a store holding at most capacity events.
//
// capacity is positional because there is no defensible default: how much of an
// audit trail an installation may lose is the installation's decision, and a
// number chosen here would be the wrong one silently.
//
// A zero MemoryStore is not usable; there is no useful zero value for a bounded
// buffer whose bound is the point of it.
func NewMemoryStore(capacity int, opts ...StoreOption) (*MemoryStore, error) {
	if capacity <= 0 {
		return nil, fmt.Errorf("service: store capacity must be positive, got %d", capacity)
	}

	cfg := storeSettings{now: time.Now}
	for _, opt := range opts {
		opt(&cfg)
	}

	return &MemoryStore{
		now:    cfg.now,
		ring:   make([]*entry, capacity),
		byName: make(map[string]*entry, capacity),
	}, nil
}

// Cap reports how many events the store holds at most. It never changes.
func (s *MemoryStore) Cap() int { return len(s.ring) }

// Len reports how many events the store currently holds.
//
// It is a snapshot: the poll cycle may append before the caller reads the
// result, so treat it as a gauge to report, never as an index to loop to.
func (s *MemoryStore) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.count
}
