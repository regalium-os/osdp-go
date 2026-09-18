// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"errors"

	eventpbv1 "github.com/regalium-os/osdp-go/protobuf/generated/go/event/v1/eventpbv1"
)

// Page size bounds, shared by EventService and by the stores in this package so
// that a page token produced under one is honoured by the other.
//
// DefaultPageSize is deliberately small. The caller of an audit API is usually
// a screen, and a panel answering a request for "the events" with ten thousand
// of them has turned a read into an outage.
//
// MaxPageSize matches the ceiling ListEventsRequest.page_size declares
// (buf.validate lte: 1000). Per AIP-158 a larger request is capped rather than
// refused: the field is advisory, the response says how many actually came
// back, and a client that asked for more gets a next_page_token instead of an
// error it cannot act on.
const (
	DefaultPageSize = 50
	MaxPageSize     = 1000
)

var (
	// ErrNotFound reports that no event is stored under the requested name.
	//
	// A bounded store returns this for an event it once held and has since
	// evicted, which is indistinguishable from one that never existed and is
	// meant to be: the schema promises no retention, and a store that
	// distinguished them would be promising one.
	ErrNotFound = errors.New("service: no such event")

	// ErrInvalidPageToken reports a page token the store did not issue, or one
	// issued against a different query.
	//
	// ListEventsRequest.page_token says "all other arguments must match the
	// call that produced it". This is what enforces it: a token carries the
	// parent it was cut from, and presenting it under another parent is an
	// error rather than a silently different page.
	ErrInvalidPageToken = errors.New("service: invalid page token")
)

// Query is one page's worth of request to a Store.
//
// It is the validated form of ListEventsRequest: EventService parses and bounds
// the request, and a Store receives only values it may act on directly. Filter
// and order_by are absent because no Store in this package implements them; see
// EventService.ListEvents, which refuses rather than ignoring them.
type Query struct {
	// Parent is the device whose events to list, as devices/{device}, or
	// devices/- for every device.
	Parent string

	// PageSize is the maximum number of events to return. Zero or negative
	// selects DefaultPageSize, so a zero Query is a usable one.
	PageSize int

	// PageToken continues a previous page. Empty starts at the newest event.
	PageToken string
}

// Page is one page of events and the cursor that follows it.
type Page struct {
	// Events are newest first, ordered by event_time descending, which is the
	// order ListEventsRequest.order_by documents as the default.
	//
	// The slice and the messages in it belong to the caller. A Store must not
	// retain either, and must not hand out a message it will later mutate.
	Events []*eventpbv1.Event

	// NextPageToken continues this listing, and is empty when the page just
	// returned was the last.
	NextPageToken string
}

// Store is where domain event records live between the poll cycle writing one
// and a client reading it.
//
// # Why this is an interface
//
// The schema module cannot know what an installation is willing to lose. A
// commissioning bench wants MemoryStore and no files on disk; a building whose
// access log is disclosable evidence wants a database with fsync and backups.
// Declaring the port here, and satisfying it there, is what makes that the
// application's decision rather than this package's.
//
// # Concurrency
//
// An implementation must be safe for concurrent use by multiple goroutines: the
// poll cycle appends from the bus goroutine while gRPC handlers read from
// theirs. That is the normal case, not a corner of it.
//
// # Ownership
//
// A Store copies what it is given and returns copies of what it holds. Neither
// side may assume it can mutate a message the other can still see -- events
// carry byte slices (a card read's data, a manufacturer body) which the runtime
// reuses between poll cycles, so an implementation that retained the caller's
// message would watch its audit trail change underneath it.
type Store interface {
	// Append stores ev as a new event under parent, which is devices/{device}
	// and must not be a wildcard.
	//
	// The store allocates the event identifier and sets name and create_time:
	// identity has to be unique within the store and stable for paging, and
	// only the store knows what it already holds. Whatever name ev arrives
	// with is replaced. event_time is the caller's, because only the caller
	// knows when the panel observed it.
	//
	// It returns the stored record, which is a copy: the caller may keep it,
	// and mutating it does not reach back into the store.
	Append(ctx context.Context, parent string, ev *eventpbv1.Event) (*eventpbv1.Event, error)

	// Get returns the event named name, or ErrNotFound. A malformed name is
	// ErrInvalidName, which is a different thing and is reported differently.
	Get(ctx context.Context, name string) (*eventpbv1.Event, error)

	// List returns one page of events matching q, newest first.
	//
	// A token in q.PageToken that this store did not issue, or that was issued
	// against another parent, is ErrInvalidPageToken.
	List(ctx context.Context, q Query) (Page, error)
}
