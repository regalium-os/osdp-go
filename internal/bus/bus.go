package bus

import (
	"context"
	"time"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/internal/transport"
	"github.com/regalium-os/osdp-go/telemetry"
)

// OfflineThreshold is how many consecutive unanswered polls mark a device
// offline. Three tolerates a burst of line noise without hiding a dead reader
// for long.
const OfflineThreshold = 3

// Bus sequences commands across the devices sharing one line.
//
// It is a state machine over events, not a loop over a port: Next says what
// should happen, and Reply or Timeout says what did. The driver decides when.
// That split is what lets a full online, offline and resynchronise scenario run
// as a table test with no hardware and no sleeping.
//
// A Bus is not safe for concurrent use. A line is inherently serial -- one
// device answers at a time -- so locking it would buy nothing.
type Bus struct {
	line    transport.Line
	devices []*Device
	current int
	scheme  frame.Scheme
}

// New returns a bus that will poll addrs in order.
//
// scheme selects the error check. Prefer frame.SchemeCRC16; the checksum exists
// for devices too old to manage a CRC.
func New(line transport.Line, addrs []frame.Address, scheme frame.Scheme) *Bus {
	devices := make([]*Device, 0, len(addrs))
	for _, a := range addrs {
		devices = append(devices, &Device{Address: a, State: Offline})
	}
	return &Bus{line: line, devices: devices, scheme: scheme}
}

// Devices returns the devices on this bus, in poll order.
func (b *Bus) Devices() []*Device { return b.devices }

// Step is one command the bus wants sent, and how long to wait for an answer.
type Step struct {
	// Device is the device being addressed.
	Device *Device

	// Frame is the sealed frame to transmit.
	Frame frame.Frame

	// Timeout is how long to wait for a reply before calling Timeout.
	Timeout time.Duration
}

// Next returns the command to send to the next device in the cycle.
//
// It advances the cycle on every call, so a device that does not answer does
// not stall the ones behind it. ok is false only when the bus has no devices.
func (b *Bus) Next(ctx context.Context) (Step, bool) {
	if len(b.devices) == 0 {
		return Step{}, false
	}

	d := b.devices[b.current]
	b.current = (b.current + 1) % len(b.devices)

	_, span := telemetry.Start(ctx, "osdp.bus.transaction", d.Trace())
	defer span.End()

	msg := b.commandFor(d)
	f, err := cmd.Encode(ctx, msg, d.Address, d.nextSequence(), b.scheme)
	if err != nil {
		span.RecordError(err)
		return Step{}, false
	}

	return Step{Device: d, Frame: f, Timeout: b.line.ReplyTimeout}, true
}

// commandFor chooses what to ask a device, based on what is known about it.
//
// An offline or freshly returned device is asked what it is before anything
// else: a panel that starts issuing commands to a reader it has not identified
// is guessing about capabilities it could simply have asked for.
func (b *Bus) commandFor(d *Device) cmd.Message {
	switch d.State {
	case Offline:
		return cmd.Message{Code: cmd.ID, Data: []byte{0x00}}
	case Identifying:
		return cmd.Message{Code: cmd.Cap, Data: []byte{0x00}}
	default:
		return cmd.Message{Code: cmd.Poll}
	}
}
