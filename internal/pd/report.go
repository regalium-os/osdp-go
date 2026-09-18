// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd

import (
	"context"
	"encoding/binary"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/telemetry"
)

// The application's side of the device: what a reader has to say, and the queue
// it waits in until the panel asks.

// ReportCardRead queues a credential to be reported on the next poll.
//
// This is the hook the whole device exists for. A peripheral device never
// speaks first (SIA OSDP v2.2.2 §5.2), so a card presented at a reader is not
// sent -- it waits for the next osdp_POLL and is answered with osdp_RAW in
// place of the osdp_ACK that would otherwise have gone back. On a line polled
// several times a second that wait is imperceptible; it is still a wait, and it
// is why the queue exists rather than a callback.
//
// # Ownership
//
// c.Data is copied. The caller may reuse or zero its buffer the moment this
// returns, and should: a credential identifies a person, and the shorter its
// life in memory the better. This package holds the copy only until the next
// poll drains it.
//
// The credential never reaches a span. The trace records the format and the bit
// count, which is what an engineer debugging a reader needs, and nothing that
// would identify the holder.
//
// # Errors
//
// ErrQueueFull means the card read was not accepted and will not be reported.
// See WithEventQueue; the alternative to this error is a credential that was
// presented and silently forgotten.
//
// Safe for concurrent use.
func (d *Device) ReportCardRead(ctx context.Context, c cmd.CardRead) error {
	_, span := telemetry.Start(ctx, "osdp.pd.card")
	defer span.End()

	span.SetAttribute(telemetry.AttrCardFormat, int(c.Format))
	span.SetAttribute(telemetry.AttrCardBitCount, int(c.BitCount))

	d.mu.Lock()
	defer d.mu.Unlock()

	err := d.queue(cmd.Message{Code: cmd.Raw, Data: appendCardRead(nil, c)})
	span.RecordError(err)
	return err
}

// ReportKeypad queues keypad digits to be reported on the next poll, as
// osdp_KEYPAD. SIA OSDP v2.2.2 §6.12.
//
// Keys are copied, and are personal data for the same reason a card number is:
// they are frequently a PIN. Nothing about them reaches a span, not even the
// count, because the length of a PIN is worth knowing to somebody guessing it.
//
// Safe for concurrent use.
func (d *Device) ReportKeypad(ctx context.Context, k cmd.KeypadEntry) error {
	_, span := telemetry.Start(ctx, "osdp.pd.keypad")
	defer span.End()

	d.mu.Lock()
	defer d.mu.Unlock()

	err := d.queue(cmd.Message{Code: cmd.Keypad, Data: appendKeypad(nil, k)})
	span.RecordError(err)
	return err
}

// Pending returns how many events are waiting for a poll. It exists so an
// application can tell a busy reader from a bus that has stopped polling.
func (d *Device) Pending() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.pending)
}

// pollReply answers osdp_POLL: the oldest waiting event, or osdp_ACK.
//
// One event per poll is the protocol, not a throttle -- a reply carries a
// single code. The panel polls again immediately, so a queue drains at the poll
// rate rather than being held back by it.
//
// The lock must be held.
func (d *Device) pollReply() cmd.Message {
	if len(d.pending) == 0 {
		return ack()
	}
	m := d.pending[0]

	// Clear the vacated slot before reslicing. Without this the array keeps a
	// reference to a credential's payload for as long as the queue lives, which
	// is the difference between holding a card number for one poll and holding
	// it until the process exits.
	d.pending[0] = cmd.Message{}
	d.pending = d.pending[1:]
	return m
}

// queue appends an event, or reports that there is no room.
//
// The lock must be held.
func (d *Device) queue(m cmd.Message) error {
	if len(d.pending) >= d.cfg.queue {
		return ErrQueueFull
	}
	d.pending = append(d.pending, m)
	return nil
}

// coalesce queues a status report, replacing any report of the same kind
// already waiting.
//
// A status report is a full snapshot, so the newest one makes its predecessors
// redundant. Replacing rather than appending is what keeps a chattering door
// contact -- a real and common field fault -- from filling the queue with
// stale snapshots and pushing out a card read behind them.
//
// The lock must be held.
func (d *Device) coalesce(m cmd.Message) error {
	for i, waiting := range d.pending {
		if waiting.Code == m.Code {
			d.pending[i] = m
			return nil
		}
	}
	return d.queue(m)
}

// appendCardRead encodes an osdp_RAW payload onto dst and returns the extended
// slice: reader number, format code, a little-endian bit count, then the bits
// themselves most significant first. SIA OSDP v2.2.2 §6.10.
//
// The bit count is not implied by the payload length and cannot be: a 26-bit
// Wiegand credential, which is most of the installed base, occupies four octets
// of which six bits are padding. A panel that counted octets would read six
// bits of nothing as part of the card number.
//
// dst may be nil. c.Data is copied into dst and not retained.
func appendCardRead(dst []byte, c cmd.CardRead) []byte {
	dst = append(dst, c.Reader, c.Format)
	dst = binary.LittleEndian.AppendUint16(dst, c.BitCount)
	return append(dst, c.Data...)
}

// appendKeypad encodes an osdp_KEYPAD payload onto dst: reader number, digit
// count, then the digits. SIA OSDP v2.2.2 §6.12.
//
// The count is a single octet, so a longer entry is truncated to what the field
// can describe rather than being sent with a count that disagrees with it. A
// 256-digit PIN is not a case this has to serve well; a payload whose header
// lies about its own length is one no receiver can parse.
//
// dst may be nil. k.Keys is copied into dst and not retained.
func appendKeypad(dst []byte, k cmd.KeypadEntry) []byte {
	keys := k.Keys
	if len(keys) > 0xFF {
		keys = keys[:0xFF]
	}
	dst = append(dst, k.Reader, byte(len(keys)))
	return append(dst, keys...)
}
