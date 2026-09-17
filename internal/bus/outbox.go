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
//
// A dequeued command is held as in-flight rather than discarded. Until the
// device answers, the bus is still holding the only copy of it.
func (d *Device) outbound() cmd.Message {
	if m, ok := d.dequeue(); ok {
		d.inflight = &m
		return m
	}
	return cmd.Message{Code: cmd.Poll}
}

// withholdsKeySet reports that the next queued command is an osdp_KEYSET which
// must not go out, because the channel that would have enciphered it is gone.
//
// The check is here as well as in InstallKey because a channel can drop between
// the two. The payload of an osdp_KEYSET is the key: sending it on a line
// anybody can reach hands over every door it opens, and no error returned
// afterwards takes that back. So the command waits rather than travelling, and
// the bus polls instead.
func (d *Device) withholdsKeySet() bool {
	return len(d.outbox) > 0 &&
		d.outbox[0].Code == cmd.KeySet &&
		!d.secureEstablished()
}

// delivered records that the device answered whatever was last sent to it, and
// remembers which command that was.
//
// The code is kept because releasing the command and deciding what its
// acknowledgement means happen at different points in handling a reply, and by
// the second the first has already let go. A poll stands in for "nothing the
// application asked for", which is what an unqueued exchange acknowledges.
//
// A refusal counts. An osdp_NAK is the device saying it received the command
// and declined it, which is an answer: retrying a command a device has already
// rejected produces the same rejection forever.
func (d *Device) delivered() {
	d.acknowledged = cmd.Poll
	if d.inflight != nil {
		d.acknowledged = d.inflight.Code
		d.inflight = nil
	}
}

// retransmit puts an unanswered command back at the front of the queue and
// marks the next transmission as a repeat.
func (d *Device) retransmit() {
	if d.inflight == nil {
		return
	}
	d.requeue()
	d.repeatSequence = true
}

// requeue returns the in-flight command to the front of the queue.
//
// The front, not the back: commands to one device are delivered in the order
// they were submitted, and a door release that jumped behind a display update
// because the first attempt was lost would be a surprising thing to debug.
func (d *Device) requeue() {
	if d.inflight == nil {
		return
	}
	d.outbox = append([]cmd.Message{*d.inflight}, d.outbox...)
	d.inflight = nil
}
