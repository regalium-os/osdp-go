package provider

import (
	"context"

	"github.com/regalium-os/osdp-go/internal/cmd"
)

// Generic handles any device whose vendor has no provider registered.
//
// It trusts what a device says about itself and passes vendor messages through
// undecoded. That is the right default: a reader speaking plain OSDP needs no
// vendor knowledge at all, and inventing quirks for a device nobody has tested
// against would be worse than having none.
type Generic struct{}

// Name returns "generic".
func (Generic) Name() string { return "generic" }

// OUI returns zero, which is not a real IEEE identifier. Generic is reached
// through Registry.For falling back, never by matching this value.
func (Generic) OUI() uint32 { return 0 }

// DecodeExtension passes the body through unrecognised, preserving the OUI so a
// caller with vendor documentation can act on it.
func (Generic) DecodeExtension(_ context.Context, m cmd.ManufacturerMessage) (Extension, error) {
	return Extension{OUI: m.OUI, Kind: "raw", Payload: m.Body, Recognised: false}, nil
}

// EncodeExtension emits the payload verbatim under the extension's OUI.
func (Generic) EncodeExtension(_ context.Context, e Extension) (cmd.ManufacturerMessage, error) {
	return cmd.ManufacturerMessage{OUI: e.OUI, Body: e.Payload}, nil
}

// Reconcile returns what the device reported, unchanged.
func (Generic) Reconcile(_ cmd.DeviceID, reported Capabilities) Capabilities {
	return reported
}
