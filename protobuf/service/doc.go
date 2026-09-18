// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

// Package service serves the schema. It implements osdp.event.v1.EventService
// over a Store of domain event records.
//
// # Layer contract
//
// service is an edge layer of the schema module, above record and below
// whatever gRPC server the application builds. It owns resource-name syntax,
// page tokens, request validation and the mapping from a storage error to a
// gRPC status code. It does not own the wire protocol, does not import any
// osdp-go internal layer, and never converts a runtime event itself -- that is
// record's job, and keeping the two apart is what lets an application persist
// records it did not obtain from this library.
//
// It writes nothing on its own behalf. EventService is read-only because the
// schema is: events are written by the poll cycle as it observes the bus, and
// an audit trail a client can create or amend is not evidence of anything. The
// write path is Store.Append, called by the panel application, never by a peer.
//
// # Allowed imports
//
//	stdlib, record, the generated schema packages, telemetry, grpc
//
// # Storage is an interface on purpose
//
// Store is declared here, by the consumer, and MemoryStore merely satisfies it.
// The in-memory implementation is bounded and forgets its oldest events, which
// is the correct behaviour for a panel that runs for years and the wrong
// behaviour for an audit trail anybody has to answer for. An application that
// must keep everything supplies a Store backed by something durable; nothing in
// this package assumes otherwise.
//
// # Tracing
//
// Spans osdp.service.get_event and osdp.service.list_events open at the RPC
// boundary and carry the device and the resolved page size. No tagged field in
// this package can reach an event payload, so no credential can reach a trace
// through one -- which is the whole point of attributes being struct tags
// rather than calls.
package service
