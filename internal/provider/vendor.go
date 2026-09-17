// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"

	"github.com/regalium-os/osdp-go/internal/cmd"
)

// Quirk is a documented deviation between what a device reports and what it
// does.
//
// Every quirk must be justified by a fixture showing the behaviour. A quirk
// without one is a rumour, and rumours accumulate until nobody dares remove
// them.
//
// The quirks below are exercised by the cap-quirked-vendor record in
// testdata/vendor.hex, which shows a device claiming AES-128 and a 1024-octet
// receive buffer and a panel believing neither.
type Quirk uint16

const (
	// QuirkNoSecureChannel marks a device that advertises Secure Channel but
	// fails the handshake.
	QuirkNoSecureChannel Quirk = 1 << iota

	// QuirkUnderreportsLEDs marks a device that reports fewer annunciators
	// than it drives.
	QuirkUnderreportsLEDs

	// QuirkSmallMessages marks a device that accepts less than it claims to.
	QuirkSmallMessages
)

// Vendor is a Provider configured for one manufacturer.
//
// It is the shape every vendor provider takes: an OUI, a set of documented
// quirks, and a decoder for that vendor's osdp_MFG messages. Adding support for
// a reader means filling this in, never touching the codec.
type Vendor struct {
	// VendorName appears in logs and traces.
	VendorName string

	// VendorOUI is the IEEE identifier in the device's osdp_PDID reply.
	VendorOUI uint32

	// Quirks are the documented deviations applied by Reconcile.
	Quirks Quirk

	// MaxMessageSize, when non-zero, caps what Reconcile will believe a device
	// accepts regardless of what it claims.
	MaxMessageSize int

	// Decode interprets an osdp_MFG body for this vendor. When nil, bodies are
	// passed through unrecognised, exactly as Generic does.
	Decode func(ctx context.Context, m cmd.ManufacturerMessage) (Extension, error)
}

// Name returns the vendor name.
func (v Vendor) Name() string { return v.VendorName }

// OUI returns the vendor's IEEE identifier.
func (v Vendor) OUI() uint32 { return v.VendorOUI }

// DecodeExtension delegates to the vendor decoder, or passes the body through.
func (v Vendor) DecodeExtension(ctx context.Context, m cmd.ManufacturerMessage) (Extension, error) {
	if v.Decode == nil {
		return Extension{OUI: m.OUI, Kind: "raw", Payload: m.Body}, nil
	}
	return v.Decode(ctx, m)
}

// EncodeExtension emits the payload under this vendor's OUI.
func (v Vendor) EncodeExtension(_ context.Context, e Extension) (cmd.ManufacturerMessage, error) {
	oui := e.OUI
	if oui == 0 {
		oui = v.VendorOUI
	}
	return cmd.ManufacturerMessage{OUI: oui, Body: e.Payload}, nil
}

// Reconcile applies this vendor's documented quirks to what a device reported.
func (v Vendor) Reconcile(_ cmd.DeviceID, reported Capabilities) Capabilities {
	out := reported

	if v.Quirks&QuirkNoSecureChannel != 0 {
		out.SecureChannel = false
	}
	if v.Quirks&QuirkSmallMessages != 0 && v.MaxMessageSize > 0 {
		if out.MaxMessageSize == 0 || out.MaxMessageSize > v.MaxMessageSize {
			out.MaxMessageSize = v.MaxMessageSize
		}
	}
	return out
}

// OUIHID is IEEE registration 00:06:8E, HID Corporation, used widely enough in
// OSDP deployments to be relied on.
//
// # On the absence of other vendors
//
// The Gallagher and Salto identifiers are NOT included, because this library
// has not been run against their hardware and a wrong OUI is worse than an
// absent one: it silently routes a device to the wrong quirk set. Construct
// those providers with the OUI read from the device's own osdp_PDID reply:
//
//	salto := provider.Vendor{VendorName: "salto", VendorOUI: observed}
//
// When a provider here is backed by a fixture from real traffic, its OUI can
// join this block.
const OUIHID uint32 = 0x00068E

// HID returns a provider for HID Global readers.
//
// It currently carries no quirks. That is a statement of what has been
// verified, not a claim that HID readers have none -- quirks are added when a
// fixture proves one, and not before.
func HID() Vendor {
	return Vendor{VendorName: "hid", VendorOUI: OUIHID}
}
