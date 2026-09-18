// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"fmt"

	eventpbv1 "github.com/regalium-os/osdp-go/protobuf/generated/go/event/v1/eventpbv1"
	"github.com/regalium-os/osdp-go/telemetry"
)

// defaultOrderBy is the one sort order this service implements.
//
// ListEventsRequest.order_by documents the default as event_time descending,
// newest first, and AIP-132 spells that "event_time desc". A client that sends
// the default explicitly is asking for what it would have got anyway, so it is
// accepted; anything else is refused rather than silently ignored.
const defaultOrderBy = "event_time desc"

// Option configures an EventService. Adding one never breaks a caller.
type Option func(*serviceSettings)

// serviceSettings are the options a service was built with.
type serviceSettings struct {
	defaultPageSize int
	maxPageSize     int
}

// WithDefaultPageSize sets how many events a request that named no page size
// receives. A non-positive size is ignored, leaving DefaultPageSize.
func WithDefaultPageSize(n int) Option {
	return func(s *serviceSettings) {
		if n > 0 {
			s.defaultPageSize = n
		}
	}
}

// WithMaxPageSize caps how many events one page may carry, however many the
// client asked for.
//
// It exists because the ceiling is a property of the deployment rather than of
// the schema: a panel on a slow uplink serving an operator console wants a much
// smaller cap than MaxPageSize, and enforcing it here means a client cannot
// talk the panel into assembling a page it cannot send.
func WithMaxPageSize(n int) Option {
	return func(s *serviceSettings) {
		if n > 0 {
			s.maxPageSize = n
		}
	}
}

// EventService serves osdp.event.v1.EventService from a Store.
//
// It is read-only, as the schema is: there is no CreateEvent and no
// UpdateEvent, because an audit trail a peer can write to is not evidence of
// anything. Events reach the store through Store.Append, which the panel
// application calls as the poll cycle observes the bus.
//
// # Concurrency
//
// Safe for concurrent use; gRPC calls every method from its own goroutines. It
// holds no mutable state of its own, so it is exactly as safe as the Store it
// was given, which Store requires to be safe.
type EventService struct {
	eventpbv1.UnimplementedEventServiceServer

	store           Store
	defaultPageSize int
	maxPageSize     int
}

// Compile-time proof that the generated server interface is satisfied. If the
// schema grows a method, this breaks here rather than at a caller's
// RegisterEventServiceServer.
var _ eventpbv1.EventServiceServer = (*EventService)(nil)

// NewEventService returns a service reading from store.
//
// store is positional because there is no service without one and no default
// worth guessing: which events a panel keeps, and for how long, is the
// application's decision. New starts no goroutine and holds no resource, so a
// caller may construct one, inspect it and discard it.
func NewEventService(store Store, opts ...Option) (*EventService, error) {
	if store == nil {
		return nil, fmt.Errorf("service: EventService needs a store, got nil")
	}

	cfg := serviceSettings{defaultPageSize: DefaultPageSize, maxPageSize: MaxPageSize}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.defaultPageSize > cfg.maxPageSize {
		cfg.defaultPageSize = cfg.maxPageSize
	}

	return &EventService{
		store:           store,
		defaultPageSize: cfg.defaultPageSize,
		maxPageSize:     cfg.maxPageSize,
	}, nil
}

// GetEvent returns a single event.
//
// A name that is not devices/{device}/events/{event} is InvalidArgument, and
// that includes the "-" wildcard: ListEvents documents it for parent only, and
// "the event named X on any device" has no single answer. An event the store
// never held and one it has evicted are both NotFound, which is the same answer
// on purpose -- see MemoryStore.
func (s *EventService) GetEvent(
	ctx context.Context, req *eventpbv1.GetEventRequest,
) (*eventpbv1.Event, error) {
	device, _, err := ParseEventName(req.GetName())
	if err != nil {
		return nil, statusError(fmt.Errorf("%w: name %q", err, req.GetName()))
	}

	ctx, span := telemetry.Start(ctx, "osdp.service.get_event", getTrace{Device: device})
	defer span.End()

	ev, err := s.store.Get(ctx, req.GetName())
	if err != nil {
		span.RecordError(err)
		return nil, statusError(err)
	}
	return ev, nil
}

// getTrace is what the get_event span records.
//
// The device, not the event: an event identifier is a counter the store
// allocated and says nothing a trace can act on, while knowing which reader a
// slow read concerned is the reason to look at the trace at all.
type getTrace struct {
	Device string `telemetry:"trace:osdp.device.name"`
}
