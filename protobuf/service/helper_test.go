// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package service_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/regalium-os/osdp-go"
	eventpbv1 "github.com/regalium-os/osdp-go/protobuf/generated/go/event/v1/eventpbv1"
	"github.com/regalium-os/osdp-go/protobuf/record"
	"github.com/regalium-os/osdp-go/protobuf/service"
)

// credential is a 26-bit Wiegand card number. It is here so that every test in
// this package runs with a real credential in the store: an assertion that
// nothing leaks is worth nothing if there was nothing to leak.
var credential = []byte{0xAB, 0xCD, 0xEF, 0x80}

// epoch is the fake clock's start. Time is injected so a retention test does
// not have to wait for a real second.
var epoch = time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)

// fakeClock advances by a second each time it is read, which is enough to make
// create_time distinguishable without making a test wait.
//
// It is atomic because WithClock requires a clock safe for concurrent use: the
// store reads it outside its own lock, so the concurrency test would otherwise
// be racing on the fixture rather than on the code under test.
func fakeClock() func() time.Time {
	var n atomic.Int64
	return func() time.Time {
		return epoch.Add(time.Duration(n.Add(1)) * time.Second)
	}
}

// newStore builds a store on the fake clock. Capacity is positional in the API
// for the same reason it is required here: how much an installation may forget
// is never a default worth guessing.
func newStore(t *testing.T, capacity int) *service.MemoryStore {
	t.Helper()

	s, err := service.NewMemoryStore(capacity, service.WithClock(fakeClock()))
	if err != nil {
		t.Fatalf("NewMemoryStore(%d): %v", capacity, err)
	}
	return s
}

// cardRecord is what the panel hands the store when somebody badges in: a
// runtime event converted by record, with the credential included so that the
// leak assertions have something to find.
//
// The name is left empty on purpose. Store.Append allocates it, because only
// the store knows what identifiers it has already used.
func cardRecord(t *testing.T, at time.Time) *eventpbv1.Event {
	t.Helper()

	ev := osdp.Event{
		Kind:   osdp.EventCardRead,
		Device: &osdp.Device{Address: 3},
		Card: osdp.CardRead{
			Reader: 0, Format: 1, BitCount: 26,
			Data: append([]byte(nil), credential...),
		},
	}

	recs, err := record.FromEvent(ev, "", at, record.WithCredentials())
	if err != nil {
		t.Fatalf("record.FromEvent: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("a card read produced %d records, want 1", len(recs))
	}
	return recs[0]
}

// appendTo stores n card reads under parent and returns their names, oldest
// first.
func appendTo(t *testing.T, s *service.MemoryStore, parent string, n int) []string {
	t.Helper()

	names := make([]string, 0, n)
	for i := range n {
		stored, err := s.Append(context.Background(), parent,
			cardRecord(t, epoch.Add(time.Duration(i)*time.Minute)))
		if err != nil {
			t.Fatalf("Append %d to %s: %v", i, parent, err)
		}
		names = append(names, stored.GetName())
	}
	return names
}

// newService wires a service onto a store, which is the only way a caller ever
// gets one: there is no service without somewhere to read from.
func newService(t *testing.T, store service.Store, opts ...service.Option) *service.EventService {
	t.Helper()

	svc, err := service.NewEventService(store, opts...)
	if err != nil {
		t.Fatalf("NewEventService: %v", err)
	}
	return svc
}
