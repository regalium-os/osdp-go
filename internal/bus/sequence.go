// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package bus

// The sequence number: how a device tells a fresh command from a repeat of one
// it has already answered. SIA OSDP v2.2.2 sec.5.7.

// sequenceRotation is 1, 2, 3 and back to 1. Zero is reserved for resynchronisation.
const sequenceRotation = 3

// nextSequence advances and returns the sequence number for the next command,
// or repeats the last one when this transmission is a retry.
//
// See repeatSequence for why a retry must not advance.
func (d *Device) nextSequence() uint8 {
	if d.repeatSequence {
		d.repeatSequence = false
		return d.seq
	}
	if d.seq >= sequenceRotation {
		d.seq = 1
	} else {
		d.seq++
	}
	return d.seq
}

// resync restarts the exchange at sequence zero, which is how a panel tells a
// device to forget what it thought was in flight.
func (d *Device) resync() {
	// A command that was in flight when the exchange restarted was never
	// acknowledged. It goes back on the queue to be sent once the device has
	// been identified again, rather than vanishing with the session.
	d.requeue()

	d.seq = 0
	d.repeatSequence = false
	d.State = Offline

	// A restarted exchange has no secure channel: the device's session state
	// went with it, and continuing to authenticate against a chain only this
	// end still believes in would fail every frame from here on.
	d.dropSession()
}

// Online reports whether the device is answering.
func (d *Device) Online() bool { return d.State >= Online }

// traceView is the projection of a Device that may appear in a span.
type traceView struct {
	Address int    `telemetry:"trace:osdp.device.address"`
	State   string `telemetry:"trace:osdp.device.state"`
	Misses  int    `telemetry:"trace:osdp.device.missed_polls"`
}

// Trace returns the span attributes for this device.
func (d *Device) Trace() any {
	return traceView{Address: int(d.Address), State: d.State.String(), Misses: d.misses}
}
