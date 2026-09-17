package bus

import (
	"time"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
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
