// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package bus

import "github.com/regalium-os/osdp-go/internal/cmd"

// A device's outbox: what the application has asked it to do, as distinct from
// what the device is, which is device.go.
//
// The queue is per device rather than per bus because delivery is per device: a
// line reaches one address at a time, and a command for a reader the cycle has
// just passed waits for the next pass regardless of what else is queued.

// Queued reports how many application commands are waiting for this device.
//
// A number that climbs rather than draining means the device is not being
// reached: commands are only sent to a device that is answering, so an offline
// reader accumulates them until it returns or the caller stops asking.
func (d *Device) Queued() int { return len(d.outbox) }

// enqueue adds a command to the back of the outbox.
func (d *Device) enqueue(m cmd.Message) { d.outbox = append(d.outbox, m) }

// dequeue takes the oldest waiting command, if any.
func (d *Device) dequeue() (cmd.Message, bool) {
	if len(d.outbox) == 0 {
		return cmd.Message{}, false
	}
	m := d.outbox[0]

	// Clear the vacated slot before reslicing: the payload may hold a
	// credential or key material, and a slice header pointing at it would keep
	// it alive for as long as the outbox does.
	d.outbox[0] = cmd.Message{}
	d.outbox = d.outbox[1:]
	return m, true
}

// outbound is what to send a device that is up: whatever the application has
// queued for it, or a poll.
//
// A poll is not filler. It is how a device reports a card read, so a bus with
// nothing to say still has to ask -- which is why an empty outbox produces a
// command rather than nothing.
func (d *Device) outbound() cmd.Message {
	if m, ok := d.dequeue(); ok {
		return m
	}
	return cmd.Message{Code: cmd.Poll}
}
