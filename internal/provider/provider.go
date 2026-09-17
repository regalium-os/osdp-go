package provider

import (
	"context"

	"github.com/regalium-os/osdp-go/internal/cmd"
)

// Capabilities is what a device can do, as reported by osdp_CAP and then
// reconciled against what it actually does.
//
// The two are not always the same. Readers in the field misreport: some claim a
// capability they do not implement, some omit one they do. A provider's job is
// to know which, for its vendor, and to say so here.
type Capabilities struct {
	// Readers is the number of card readers attached.
	Readers int
	// LEDs, Buzzers and Displays count the annunciators.
	LEDs     int
	Buzzers  int
	Displays int
	// Inputs and Outputs count the monitored and controlled contacts.
	Inputs  int
	Outputs int
	// SecureChannel reports whether the device supports OSDP Secure Channel.
	SecureChannel bool
	// MaxMessageSize is the largest frame the device accepts, in octets.
	MaxMessageSize int
}

// Extension is a decoded osdp_MFG body: a vendor-defined message that only that
// vendor's provider understands.
type Extension struct {
	// OUI identifies the vendor that defined the message.
	OUI uint32
	// Kind names the message within that vendor's scheme. It is a provider's
	// own vocabulary and means nothing outside it.
	Kind string
	// Payload is the decoded remainder, or the raw body when the provider does
	// not recognise the message.
	Payload []byte
	// Recognised reports whether the provider understood the message. An
	// unrecognised extension is passed through rather than discarded: a panel
	// integrator with vendor documentation can act on what this library cannot.
	Recognised bool
}

// Provider adapts the core to one reader vendor.
//
// The interface is deliberately narrow, and that narrowness is the design.
// Vendor differences live in exactly two places -- osdp_MFG extension messages
// and capability negotiation -- and nowhere else. A provider does not frame, does
// not sequence, and does not touch the secure channel.
//
// If a vendor appears to need its own frame codec, the finding belongs in the
// frame package as a quirk flag with a fixture proving it. Vendor forks of the
// wire format are how OSDP stacks rot, and this interface has no room for one
// on purpose.
type Provider interface {
	// Name identifies the vendor for logs and traces.
	Name() string

	// OUI is the IEEE identifier this provider claims, as it appears in
	// osdp_PDID and in the first three octets of an osdp_MFG body.
	OUI() uint32

	// DecodeExtension interprets an osdp_MFG body. A provider that does not
	// recognise the message returns an Extension with Recognised false rather
	// than an error: an unknown vendor message is not a protocol fault.
	DecodeExtension(ctx context.Context, m cmd.ManufacturerMessage) (Extension, error)

	// EncodeExtension builds an osdp_MFG body from a vendor message.
	EncodeExtension(ctx context.Context, e Extension) (cmd.ManufacturerMessage, error)

	// Reconcile adjusts what a device reported about itself. Most providers
	// return reported unchanged; the value is in the ones that cannot.
	Reconcile(id cmd.DeviceID, reported Capabilities) Capabilities
}
