package bus

import (
	"context"
	"errors"
	"time"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/telemetry"
)

// Bus errors, matched with errors.Is.
var (
	// ErrWrongAddress reports a reply from a device other than the one
	// addressed. On a multidrop line this means two devices share an address,
	// which no amount of retrying will fix.
	ErrWrongAddress = errors.New("osdp/bus: reply came from a different address")

	// ErrNotAReply reports a frame without the reply flag arriving where a
	// reply was expected -- usually the panel hearing its own transmission
	// because the line did not turn around in time.
	ErrNotAReply = errors.New("osdp/bus: frame is not a reply")
)

// Reply processes a device's answer and returns what it meant.
//
// An error means the frame could not be attributed to this exchange at all. A
// device that answered with a refusal, or that lost synchronisation, is
// reported through the Event -- those are things that happen, not things that
// went wrong.
func (b *Bus) Reply(ctx context.Context, d *Device, f frame.Frame, now time.Time) (Event, error) {
	ctx, span := telemetry.Start(ctx, "osdp.bus.reply", d.Trace())
	defer span.End()

	if !f.IsReply {
		span.RecordError(ErrNotAReply)
		return Event{}, ErrNotAReply
	}
	if f.Address != d.Address {
		span.RecordError(ErrWrongAddress)
		return Event{}, ErrWrongAddress
	}

	d.LastSeen, d.misses = now, 0

	// Sequence zero from a device means it has lost synchronisation: it is
	// asking to start over rather than refusing. Honour that immediately --
	// continuing to send the sequence it no longer recognises just produces
	// another one of these.
	if f.Control.Sequence() == 0 {
		d.resync()
		return b.event(ctx, Event{Kind: KindResync, Device: d}), nil
	}

	msg, err := cmd.Decode(ctx, f)
	if err != nil {
		span.RecordError(err)
		return Event{}, err
	}
	return b.dispatch(ctx, d, msg)
}

// dispatch turns a decoded reply into an event and advances device state.
func (b *Bus) dispatch(ctx context.Context, d *Device, msg cmd.Message) (Event, error) {
	wasOffline := d.State == Offline

	switch msg.Code {
	case cmd.PDID:
		id, err := cmd.ParseDeviceID(msg.Data)
		if err != nil {
			return Event{}, err
		}
		d.ID, d.State = id, Identifying
		return b.event(ctx, Event{Kind: KindIdentified, Device: d, ID: id}), nil

	case cmd.PDCap:
		d.State = Online
		return b.event(ctx, Event{Kind: KindOnline, Device: d}), nil

	case cmd.Raw:
		card, err := cmd.ParseCardRead(msg.Data)
		if err != nil {
			return Event{}, err
		}
		return b.event(ctx, Event{Kind: KindCardRead, Device: d, Card: card}), nil

	case cmd.Keypad:
		entry, err := cmd.ParseKeypadEntry(msg.Data)
		if err != nil {
			return Event{}, err
		}
		return b.event(ctx, Event{Kind: KindKeypad, Device: d, Keypad: entry}), nil

	case cmd.NAK:
		reason, err := cmd.ParseNAK(msg.Data)
		if err != nil {
			return Event{}, err
		}
		return b.event(ctx, Event{Kind: KindNAK, Device: d, NAK: reason}), nil

	case cmd.MFGReply:
		mfg, err := cmd.ParseManufacturerMessage(msg.Data)
		if err != nil {
			return Event{}, err
		}
		return b.event(ctx, Event{Kind: KindManufacturer, Device: d, Manufacturer: mfg}), nil

	default:
		// osdp_ACK and everything else: the device is answering, which is all
		// a poll needs to establish.
		if wasOffline {
			d.State = Identifying
			return b.event(ctx, Event{Kind: KindOnline, Device: d}), nil
		}
		return b.event(ctx, Event{Kind: KindNone, Device: d}), nil
	}
}

// Timeout records that a device did not answer in time.
//
// A single miss is noise. OfflineThreshold consecutive misses is a reader
// someone should go and look at, and only then is an event raised.
func (b *Bus) Timeout(ctx context.Context, d *Device) Event {
	ctx, span := telemetry.Start(ctx, "osdp.bus.timeout", d.Trace())
	defer span.End()

	d.misses++
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
