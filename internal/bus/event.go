package bus

import "github.com/regalium-os/osdp-go/internal/cmd"

// Kind classifies what an exchange produced.
type Kind uint8

const (
	// KindNone is an exchange that completed with nothing to report: the
	// overwhelming majority of poll replies.
	KindNone Kind = iota
	// KindOnline means a device that was offline has answered.
	KindOnline
	// KindOffline means a device has missed enough polls to be presumed gone.
	KindOffline
	// KindIdentified carries a device's osdp_PDID response.
	KindIdentified
	// KindCardRead carries a credential presented at a reader.
	KindCardRead
	// KindKeypad carries digits entered at a keypad.
	KindKeypad
	// KindNAK means the device refused the command and said why.
	KindNAK
	// KindResync means the exchange lost synchronisation and is restarting.
	KindResync
	// KindManufacturer carries an osdp_MFG reply for a provider to interpret.
	KindManufacturer
)

// Event is what one exchange produced.
//
// A device going offline is an Event, not an error return. On a bus of hundreds
// of readers, one not answering is Tuesday: it is information the application
// acts on, not a failure of the call that noticed it. Errors are reserved for
// conditions the caller must fix.
type Event struct {
	// Kind classifies the event.
	Kind Kind

	// Device is the device the exchange addressed. Never nil.
	Device *Device

	// ID is valid when Kind is KindIdentified.
	ID cmd.DeviceID

	// Card is valid when Kind is KindCardRead.
	//
	// It holds a credential. Do not log it and do not attach it to a span.
	Card cmd.CardRead

	// Keypad is valid when Kind is KindKeypad, and frequently holds a PIN.
	Keypad cmd.KeypadEntry

	// NAK is valid when Kind is KindNAK.
	NAK cmd.NAKReason

	// Manufacturer is valid when Kind is KindManufacturer.
	Manufacturer cmd.ManufacturerMessage
}

// traceEvent is the projection of an Event that may appear in a span.
//
// Card and Keypad are absent by construction. A credential must not reach a
// trace, and the way to guarantee that is for there to be no field carrying it.
type traceEvent struct {
	Kind    string `telemetry:"trace:osdp.event.kind"`
	Address int    `telemetry:"trace:osdp.device.address"`
	// For a card read, the shape is recorded and the credential is not.
	CardFormat   int `telemetry:"trace:osdp.card.format"`
	CardBitCount int `telemetry:"trace:osdp.card.bit_count"`
}

// Trace returns the span attributes for this event.
func (e Event) Trace() any {
	v := traceEvent{Kind: e.Kind.String()}
	if e.Device != nil {
		v.Address = int(e.Device.Address)
	}
	if e.Kind == KindCardRead {
		v.CardFormat = int(e.Card.Format)
		v.CardBitCount = int(e.Card.BitCount)
	}
	return v
}

// String implements fmt.Stringer.
func (k Kind) String() string {
	switch k {
	case KindOnline:
		return "online"
	case KindOffline:
		return "offline"
	case KindIdentified:
		return "identified"
	case KindCardRead:
		return "card_read"
	case KindKeypad:
		return "keypad"
	case KindNAK:
		return "nak"
	case KindResync:
		return "resync"
	case KindManufacturer:
		return "manufacturer"
	default:
		return "none"
	}
}
