package provider

import "github.com/regalium-os/osdp-go/internal/cmd"

// FromReport derives Capabilities from what a device answered to osdp_CAP.
//
// This is the unreconciled view: it says what the device claims, with no vendor
// knowledge applied. A caller wanting the truth passes the result through the
// device's Provider.Reconcile, which is where a documented quirk overrides a
// claim. Keeping the two steps separate is deliberate -- when a reader
// misbehaves in the field, the first question is whether it lied or whether we
// corrected it, and that is only answerable if both values exist.
//
// Counts that the specification defines per reader stay per reader. A device
// with two readers and two LEDs on each reports Readers 2 and LEDs 2, not 4;
// multiplying them here would quietly invent an addressing scheme the wire
// does not have.
//
// A function the device omits contributes zero rather than a guess. Zero LEDs
// on a reader that plainly has one is a device that under-reports, which is a
// quirk with a fixture, not something to paper over here.
//
// r is read and not retained.
func FromReport(r cmd.CapabilityReport) Capabilities {
	capable, _ := r.SecureChannel()

	return Capabilities{
		Readers:  r.ItemsOf(cmd.FuncReaders),
		LEDs:     r.ItemsOf(cmd.FuncReaderLED),
		Buzzers:  r.ItemsOf(cmd.FuncReaderAudible),
		Displays: r.ItemsOf(cmd.FuncReaderText),
		Inputs:   r.ItemsOf(cmd.FuncContactStatus),
		Outputs:  r.ItemsOf(cmd.FuncOutputControl),

		SecureChannel:  capable,
		MaxMessageSize: r.ReceiveBufferSize(),
	}
}

// UsesDefaultKey reports whether the device says it is still holding SCBK-D,
// the default Secure Channel key published in the specification.
//
// It is not part of Capabilities because it is not a capability: it is a
// commissioning state that changes the moment a panel sets a site key. A panel
// should surface it -- a bus of devices on the default key is a bus with no
// confidentiality at all -- but it must not be cached alongside the things
// about a device that do not change.
func UsesDefaultKey(r cmd.CapabilityReport) bool {
	_, defaultKey := r.SecureChannel()
	return defaultKey
}
