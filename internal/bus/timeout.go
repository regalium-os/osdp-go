// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package bus

import (
	"context"

	"github.com/regalium-os/osdp-go/telemetry"
)

// What a silence means. Dispatching an answer is reply.go; this is the other
// half of an exchange, where nothing came back.

// Timeout records that a device did not answer in time.
//
// A single miss is noise. OfflineThreshold consecutive misses is a reader
// someone should go and look at, and only then is an event raised.
func (b *Bus) Timeout(ctx context.Context, d *Device) Event {
	ctx, span := telemetry.Start(ctx, "osdp.bus.timeout", d.Trace())
	defer span.End()

	d.misses++

	// Whatever was sent is unaccounted for: the device may have acted on it and
	// had its reply lost, or never heard it at all. Either way the bus must not
	// be the only thing that knew about it. It goes back on the queue, and the
	// retry repeats the sequence number so a device that did act replays its
	// answer rather than acting twice.
	d.retransmit()

	if d.misses < OfflineThreshold || d.State == Offline {
		return Event{Kind: KindNone, Device: d}
	}

	d.resync()
	return b.event(ctx, Event{Kind: KindOffline, Device: d})
}

// event attaches the event's attributes to the current span and returns it.
func (b *Bus) event(ctx context.Context, e Event) Event {
	_, span := telemetry.Start(ctx, "osdp.bus.event", e.Trace())
	span.End()
	return e
}
