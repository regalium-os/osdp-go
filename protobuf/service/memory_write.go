// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"fmt"

	eventpbv1 "github.com/regalium-os/osdp-go/protobuf/generated/go/event/v1/eventpbv1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Append stores a copy of ev under parent and returns the stored record.
//
// It allocates the event identifier from a counter that survives eviction, so a
// name is never reused and a page token never lands on a different event than
// the one it was cut from. The identifier is the counter in fixed-width hex,
// which sorts in the order the events arrived -- convenient for a human reading
// a log, and not something a client should parse.
//
// See Store.Append for what is overwritten and what is kept.
func (s *MemoryStore) Append(
	ctx context.Context, parent string, ev *eventpbv1.Event,
) (*eventpbv1.Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	device, err := ParseParent(parent)
	if err != nil {
		return nil, err
	}
	if device == Wildcard {
		return nil, fmt.Errorf("%w: an event belongs to one device, not %q",
			ErrInvalidParent, Wildcard)
	}
	if ev == nil {
		return nil, fmt.Errorf("service: Append needs an event, got nil")
	}

	stored, err := cloneEvent(ev)
	if err != nil {
		return nil, err
	}
	now := s.now()
	stored.CreateTime = timestamppb.New(now)
	if stored.GetEventTime() == nil {
		// An audit row with no clock cannot be placed in the trail at all. The
		// write time is the closest honest answer and is the panel's clock
		// either way, which is what Event.event_time documents it to be.
		stored.EventTime = timestamppb.New(now)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	stored.Name = EventName(device, fmt.Sprintf("%016x", s.seq))
	e := &entry{seq: s.seq, device: device, ev: stored}
	s.seq++

	if s.count == len(s.ring) {
		delete(s.byName, s.ring[s.next].ev.GetName())
	} else {
		s.count++
	}
	s.ring[s.next] = e
	s.next = (s.next + 1) % len(s.ring)
	s.byName[stored.GetName()] = e

	return cloneEvent(stored)
}
