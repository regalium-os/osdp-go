package osdp_test

import (
	"testing"

	"github.com/regalium-os/osdp-go"
)

// TestCapabilityNegotiationFromOutside walks the enrolment path a control panel
// takes, using only the public surface: read an osdp_PDCAP reply, derive what
// the device claims, then let its provider disbelieve the parts it knows better
// than to trust.
//
// It lives in package osdp_test so that a re-export that quietly became a
// wrapper type would fail to compile here rather than in a consumer's build.
func TestCapabilityNegotiationFromOutside(t *testing.T) {
	// The payload of an osdp_PDCAP reply: function code, compliance, items.
	payload := []byte{
		0x01, 0x01, 0x04, // 4 monitored inputs
		0x02, 0x01, 0x04, // 4 controlled outputs
		0x04, 0x01, 0x02, // 2 LEDs per reader
		0x09, 0x01, 0x01, // AES-128, and still on the default key
		0x0A, 0x00, 0x04, // receive buffer 1024 octets, least significant first
		0x0D, 0x01, 0x01, // one card reader
	}

	report, err := osdp.ParseCapabilities(payload)
	if err != nil {
		t.Fatalf("ParseCapabilities: %v", err)
	}

	claimed := osdp.DeviceCapabilities(report)
	if !claimed.SecureChannel || claimed.MaxMessageSize != 1024 || claimed.Readers != 1 {
		t.Fatalf("claimed = %+v, want secure, 1024 octets, one reader", claimed)
	}
	if !osdp.UsesDefaultKey(report) {
		t.Error("a device reporting SCBK-D was not flagged; the bus is not confidential")
	}

	// A report is more than the interpreted view: an integrator with vendor
	// documentation must still be able to read the raw entry.
	if e, ok := report.Get(osdp.CapReceiveBuffer); !ok || e.Items != 0x04 {
		t.Errorf("raw entry for the receive buffer = %+v, %v; want the octets as sent", e, ok)
	}
	if !report.Supports(osdp.CapCommSecurity) {
		t.Error("Supports disagrees with the entry the device sent")
	}

	// The vendor has the last word. This is the same shape as the corpus
	// record cap-quirked-vendor, reached through the facade.
	v := osdp.Vendor{
		VendorName:     "example",
		VendorOUI:      0x0F0F0F,
		Quirks:         osdp.QuirkNoSecureChannel | osdp.QuirkSmallMessages,
		MaxMessageSize: 128,
	}
	believed := v.Reconcile(osdp.DeviceID{}, claimed)

	if believed.SecureChannel {
		t.Error("a quirk saying the handshake fails did not survive the facade")
	}
	if believed.MaxMessageSize != 128 {
		t.Errorf("max message size = %d, want the quirk's 128", believed.MaxMessageSize)
	}
	if believed.Readers != claimed.Readers {
		t.Error("Reconcile altered a capability no quirk covers")
	}
}
