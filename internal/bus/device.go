// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package bus

import (
	"time"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/internal/secure"
)

// State is what the bus currently believes about one device.
type State uint8

const (
	// Offline means the device has missed enough polls to be considered gone.
	// It is still polled, because that is how it comes back.
	Offline State = iota
	// Identifying means the device answered and is being asked what it is.
	Identifying
	// Online means the device is answering and identified, without a secure
	// channel.
	Online
	// SecureHandshake means a secure session is being established.
	SecureHandshake
	// Secure means an established secure channel.
	Secure
)

// String implements fmt.Stringer and supplies the span attribute value.
func (s State) String() string {
	switch s {
	case Offline:
		return "offline"
	case Identifying:
		return "identifying"
	case Online:
		return "online"
	case SecureHandshake:
		return "secure_handshake"
	default:
		return "secure"
	}
}

// Device is one peripheral device on the line.
//
// It holds protocol state only: no connection, no goroutine, no timer. What
// happens and when is the driver's business; Device records what the exchange
// so far means.
type Device struct {
	// Address is the device's bus address.
	Address frame.Address

	// State is the current belief about the device.
	State State

	// ID is what the device reported to osdp_ID, valid once identified.
	ID cmd.DeviceID

	// Caps is what the device reported to osdp_CAP, entry by entry, exactly as
	// it answered -- function codes this library does not recognise included.
	//
	// It is what the device claimed, not what the panel should believe. Pass it
	// through the device's provider to get the reconciled view; the two are
	// deliberately kept apart so that a reader misbehaving in the field can be
	// asked whether it lied or whether we corrected it.
	//
	// The value is from the last successful enrolment. A device that has been
	// offline since may have been swapped for another at the same address, so
	// treat it as stale until Online reports true again.
	Caps cmd.CapabilityReport

	// session is the secure channel, nil until a handshake begins. The bus
	// owns it: it is not exported because a caller reaching into a live
	// session's MAC chain would desynchronise the device it belongs to.
	session *secure.Session

	// pending is the next handshake command, computed when the reply that
	// prompts it arrives. The handshake alternates strictly, so there is never
	// more than one outstanding.
	pending *secureStep

	// status is the last state reported for each kind of contact, which is
	// what turns a report into a change. See statusChanges.
	status map[cmd.StatusKind]*contacts

	// outbox holds commands the application has asked to be sent to this
	// device, oldest first.
	//
	// They wait their turn rather than interrupting: a line carries one
	// exchange at a time, so a command for a device the cycle has just passed
	// is sent when the cycle comes round again. That is a property of the wire,
	// not a scheduling choice, and a caller timing a door release should know
	// the latency is up to one full pass over the address list.
	outbox []cmd.Message

	// secureDeclined records that this device will not be offered a secure
	// channel again: it does not claim the capability, no key is configured
	// for it, or it failed to prove possession of the one that is. Retrying a
	// handshake the peer cannot pass turns one wrong key into a bus that never
	// carries traffic.
	secureDeclined bool

	// LastSeen is when a well-formed reply last arrived.
	LastSeen time.Time

	// seq is the sequence number for the next command, rotating 1, 2, 3.
	//
	// Zero is not part of the rotation. It means "this exchange is starting
	// over", which a device sends when it has lost synchronisation and a panel
	// sends when it is bringing a device back from offline.
	seq uint8

	// misses counts consecutive unanswered polls.
	misses int
}

// sequenceRotation is 1, 2, 3 and back to 1. Zero is reserved for resynchronisation.
const sequenceRotation = 3

// nextSequence advances and returns the sequence number for the next command.
func (d *Device) nextSequence() uint8 {
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
	d.seq = 0
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
