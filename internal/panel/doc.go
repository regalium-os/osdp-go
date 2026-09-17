// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

// Package panel is the OSDP runtime: the loop that turns a bus's decisions into
// traffic on a real line.
//
// # Layer contract
//
// Everything beneath this package computes. bus says which device to address
// and how long to wait; frame says what the octets are; secure says what is
// authentic. None of them write to anything, and none of them know what time it
// is. panel is where that changes: it owns a transport.Port, consults a clock,
// blocks, and hands events to the application.
//
// That concentration is the point. There is exactly one place in this library
// that can hang, and it is this one -- which is worth knowing at three in the
// morning when a bus has gone quiet.
//
// panel depends on the core; the core never depends on panel. bus has no idea
// this package exists, which is what keeps a full online/offline/resync
// scenario runnable as a table test with no port and no clock.
//
// # Lifecycle
//
// New constructs and starts nothing. Run blocks until the context is cancelled
// or the line fails, and owns everything it started. Close is idempotent and
// may be called after a failed Run. No goroutine outlives Run.
//
// # Allowed imports
//
//	stdlib, frame, cmd, secure, bus, transport, telemetry
//
// # Tracing
//
// Span osdp.panel.cycle wraps one pass over the address list and is the parent
// of the bus, frame and secure spans beneath it.
package panel
