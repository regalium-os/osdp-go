package provider_test

import (
	"context"
	"testing"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/provider"
)

func deviceWithOUI(oui uint32) cmd.DeviceID {
	return cmd.DeviceID{VendorCode: [3]byte{byte(oui >> 16), byte(oui >> 8), byte(oui)}}
}

// TestUnknownVendorFallsBackToGeneric: a panel must talk to a reader nobody has
// tested against, not refuse it.
func TestUnknownVendorFallsBackToGeneric(t *testing.T) {
	r := provider.NewRegistry(provider.HID())

	p := r.For(deviceWithOUI(0xABCDEF))
	if p.Name() != "generic" {
		t.Errorf("unknown vendor resolved to %q, want generic", p.Name())
	}
}

func TestKnownVendorResolves(t *testing.T) {
	r := provider.NewRegistry(provider.HID())

	p := r.For(deviceWithOUI(provider.OUIHID))
	if p.Name() != "hid" {
		t.Errorf("HID device resolved to %q, want hid", p.Name())
	}
}

// TestApplicationCanOverrideABuiltInProvider: an integrator with hardware in
// hand knows more than this library does.
func TestApplicationCanOverrideABuiltInProvider(t *testing.T) {
	custom := provider.Vendor{VendorName: "hid-site-specific", VendorOUI: provider.OUIHID}
	r := provider.NewRegistry(provider.HID(), custom)

	if got := r.For(deviceWithOUI(provider.OUIHID)).Name(); got != "hid-site-specific" {
		t.Errorf("resolved to %q, want the overriding provider", got)
	}
}

// TestUnrecognisedExtensionIsPassedThrough: an unknown vendor message is not a
// protocol fault, and discarding it would lose data a caller may understand.
func TestUnrecognisedExtensionIsPassedThrough(t *testing.T) {
	body := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	m := cmd.ManufacturerMessage{OUI: 0xABCDEF, Body: body}

	ext, err := provider.Generic{}.DecodeExtension(context.Background(), m)
	if err != nil {
		t.Fatalf("DecodeExtension: %v", err)
	}
	if ext.Recognised {
		t.Error("Generic claims to recognise a vendor message")
	}
	if string(ext.Payload) != string(body) {
		t.Errorf("payload = %x, want %x", ext.Payload, body)
	}
	if ext.OUI != m.OUI {
		t.Errorf("OUI = %06X, want %06X; the vendor must survive the round trip", ext.OUI, m.OUI)
	}
}

// TestQuirksOverrideWhatADeviceClaims is the whole point of Reconcile.
func TestQuirksOverrideWhatADeviceClaims(t *testing.T) {
	v := provider.Vendor{
		VendorName:     "test",
		VendorOUI:      0x010203,
		Quirks:         provider.QuirkNoSecureChannel | provider.QuirkSmallMessages,
		MaxMessageSize: 128,
	}
	reported := provider.Capabilities{SecureChannel: true, MaxMessageSize: 1024, Readers: 1}

	got := v.Reconcile(deviceWithOUI(0x010203), reported)
	if got.SecureChannel {
		t.Error("a device that fails the handshake is still reported as secure-capable")
	}
	if got.MaxMessageSize != 128 {
		t.Errorf("max message size = %d, want 128", got.MaxMessageSize)
	}
	if got.Readers != reported.Readers {
		t.Error("Reconcile altered a capability no quirk covers")
	}
}

// TestGenericTrustsTheDevice: inventing quirks for untested hardware is worse
// than having none.
func TestGenericTrustsTheDevice(t *testing.T) {
	reported := provider.Capabilities{SecureChannel: true, Readers: 2, MaxMessageSize: 256}
	if got := (provider.Generic{}).Reconcile(cmd.DeviceID{}, reported); got != reported {
		t.Errorf("Generic altered capabilities: %+v, want %+v", got, reported)
	}
}
