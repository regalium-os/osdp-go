package osdp

import (
	"github.com/regalium-os/osdp-go/internal/bus"
	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/transport"
)

// Bus and message types.
type (
	// Bus sequences commands across the devices sharing one line. It is a
	// state machine over events: Next says what should happen, Reply and
	// Timeout say what did, and the driver decides when.
	Bus = bus.Bus

	// Device is one peripheral device's protocol state.
	Device = bus.Device

	// DeviceState is what the bus believes about a device.
	DeviceState = bus.State

	// Event is what one exchange produced. A device going offline is an Event,
	// not an error: on a bus of hundreds of readers, one not answering is
	// information to act on rather than a failure of the call that noticed.
	Event = bus.Event

	// EventKind classifies an Event.
	EventKind = bus.Kind

	// Step is one command the bus wants sent, and how long to wait for it.
	Step = bus.Step

	// Message is one command or reply: the code and its payload.
	Message = cmd.Message

	// Code is a command or reply code.
	Code = cmd.Code

	// DeviceID is what a device reports to osdp_ID.
	DeviceID = cmd.DeviceID

	// CardRead is a credential presented at a reader. It is personal data:
	// never log it, never attach it to a span.
	CardRead = cmd.CardRead

	// KeypadEntry is digits entered at a keypad, frequently a PIN.
	KeypadEntry = cmd.KeypadEntry

	// ManufacturerMessage is an osdp_MFG body: an OUI and a vendor-defined
	// remainder that only that vendor's provider understands.
	ManufacturerMessage = cmd.ManufacturerMessage

	// NAKReason is why a device refused a command.
	NAKReason = cmd.NAKReason
)

// Device states.
const (
	Offline         = bus.Offline
	Identifying     = bus.Identifying
	Online          = bus.Online
	SecureHandshake = bus.SecureHandshake
	SecureOnline    = bus.Secure
)

// Event kinds.
const (
	EventNone         = bus.KindNone
	EventOnline       = bus.KindOnline
	EventOffline      = bus.KindOffline
	EventIdentified   = bus.KindIdentified
	EventCardRead     = bus.KindCardRead
	EventKeypad       = bus.KindKeypad
	EventNAK          = bus.KindNAK
	EventResync       = bus.KindResync
	EventManufacturer = bus.KindManufacturer
)

// OfflineThreshold is how many consecutive unanswered polls mark a device
// offline.
const OfflineThreshold = bus.OfflineThreshold

// NewBus returns a bus that polls addrs in order, using the given error check.
// Prefer SchemeCRC16; the checksum exists for devices too old to manage a CRC.
func NewBus(line Line, addrs []Address, scheme Scheme) *Bus {
	return bus.New(line, addrs, scheme)
}

// Bus errors, matched with errors.Is.
var (
	// ErrWrongAddress means a reply came from a device other than the one
	// addressed, which on a multidrop line means two devices share an address.
	ErrWrongAddress = bus.ErrWrongAddress

	// ErrNotAReply means a command frame arrived where a reply was expected,
	// usually a panel hearing its own transmission.
	ErrNotAReply = bus.ErrNotAReply

	// ErrShortPayload means a payload was too short for the structure its code
	// implies. On a shared line this is ordinary corruption.
	ErrShortPayload = cmd.ErrShortPayload
)

// Frame-level aliases used by the bus API.
type (
	// Line describes the physical parameters of a bus.
	Line = transport.Line
)
