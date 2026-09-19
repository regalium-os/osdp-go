// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package osdp

import (
	"context"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/driver"
	"github.com/regalium-os/osdp-go/internal/provider"
	"github.com/regalium-os/osdp-go/internal/transport"
)

// Vendor and transport types.
//
// Provider and Port are both extension points, which is why they are here. A
// third party writes a provider for a reader this library has never seen, or a
// Port for a proprietary serial bridge, without changing anything internal.
type (
	// Provider adapts the core to one reader vendor. The interface is narrow on
	// purpose: vendor differences live in osdp_MFG messages and capability
	// negotiation, and nowhere else.
	Provider = provider.Provider

	// ProviderRegistry resolves a device to its provider by OUI, falling back
	// to Generic rather than refusing an unknown reader.
	ProviderRegistry = provider.Registry

	// Capabilities is what a device can do, reported and then reconciled.
	Capabilities = provider.Capabilities

	// Extension is a decoded osdp_MFG body.
	Extension = provider.Extension

	// Vendor is a Provider configured for one manufacturer: an OUI, documented
	// quirks, and a decoder for that vendor's messages.
	Vendor = provider.Vendor

	// Quirk is a documented deviation between what a device reports and what
	// it does. Every quirk must be justified by a fixture.
	Quirk = provider.Quirk

	// Port is the byte-oriented connection the core speaks through.
	// Implement it to add a transport.
	Port = transport.Port
)

// Quirks.
const (
	QuirkNoSecureChannel  = provider.QuirkNoSecureChannel
	QuirkUnderreportsLEDs = provider.QuirkUnderreportsLEDs
	QuirkSmallMessages    = provider.QuirkSmallMessages
)

// OUIHID is HID Corporation's IEEE registration.
//
// Gallagher and Salto identifiers are deliberately absent: this library has not
// been run against their hardware, and a wrong OUI is worse than a missing one
// because it silently routes a device to the wrong quirk set. Build those
// providers with the OUI read from the device's own osdp_PDID reply.
const OUIHID = provider.OUIHID

// Capability negotiation types.
//
// Capabilities and CapabilityReport are deliberately different things and the
// names are worth reading twice. A CapabilityReport is the osdp_PDCAP reply as
// the device sent it, entry by entry, unknown function codes included.
// Capabilities is the interpreted view a panel acts on. Going from the first to
// the second loses what this library does not understand, which is why both are
// public: an integrator with vendor documentation can read an entry that
// DeviceCapabilities ignores.
type (
	// CapabilityReport is the decoded osdp_PDCAP payload: everything a device
	// says it can do, in the order it said it.
	CapabilityReport = cmd.CapabilityReport

	// Capability is one entry of a report: a function code and the two
	// function-specific octets that qualify it.
	Capability = cmd.Capability

	// CapabilityFunction is an osdp_PDCAP function code, SIA OSDP v2.2.2 §6.6.
	CapabilityFunction = cmd.Function
)

// Capability function codes, SIA OSDP v2.2.2 §6.6.
const (
	CapContactStatus   = cmd.FuncContactStatus
	CapOutputControl   = cmd.FuncOutputControl
	CapCardDataFormat  = cmd.FuncCardDataFormat
	CapReaderLED       = cmd.FuncReaderLED
	CapReaderAudible   = cmd.FuncReaderAudible
	CapReaderText      = cmd.FuncReaderText
	CapTimeKeeping     = cmd.FuncTimeKeeping
	CapCheckCharacter  = cmd.FuncCheckCharacter
	CapCommSecurity    = cmd.FuncCommSecurity
	CapReceiveBuffer   = cmd.FuncReceiveBuffer
	CapCombinedMessage = cmd.FuncCombinedMessage
	CapSmartCard       = cmd.FuncSmartCard
	CapReaders         = cmd.FuncReaders
	CapBiometrics      = cmd.FuncBiometrics
	CapSecurePINEntry  = cmd.FuncSecurePINEntry
	CapOSDPVersion     = cmd.FuncOSDPVersion
)

// ParseCapabilities decodes an osdp_PDCAP payload. The entries are copied, so
// the report outlives the read buffer it came from.
func ParseCapabilities(data []byte) (CapabilityReport, error) {
	return cmd.ParseCapabilities(data)
}

// DeviceCapabilities derives the capability set a device claims, with no vendor
// knowledge applied. Pass the result through the device's Provider.Reconcile to
// get what the panel should actually believe; keeping the two apart is what
// makes it answerable, later, whether a reader lied or whether we corrected it.
func DeviceCapabilities(r CapabilityReport) Capabilities {
	return provider.FromReport(r)
}

// UsesDefaultKey reports whether a device says it is still holding SCBK-D, the
// default Secure Channel key published in the specification. A bus of devices
// on the default key has no confidentiality at all, and a panel should say so.
func UsesDefaultKey(r CapabilityReport) bool {
	return provider.UsesDefaultKey(r)
}

// NewProviderRegistry returns a registry holding the supplied providers plus
// the generic fallback. A later provider with the same OUI replaces an earlier
// one, so an application can override a built-in.
func NewProviderRegistry(providers ...Provider) *ProviderRegistry {
	return provider.NewRegistry(providers...)
}

// HID returns a provider for HID Global readers. It carries no quirks: that is
// a statement of what has been verified, not a claim that there are none.
func HID() Vendor { return provider.HID() }

// GenericProvider returns the fallback provider, which trusts what a device
// reports and passes vendor messages through undecoded.
func GenericProvider() Provider { return provider.Generic{} }

// DialTCP opens an OSDP connection over TCP. Serial-to-Ethernet converters are
// how most installed buses reach a panel that is not in the same cupboard.
func DialTCP(ctx context.Context, address string) (Port, error) {
	return driver.DialTCP(ctx, address)
}

// SerialOption configures a serial port at open time.
type SerialOption = driver.SerialOption

// SerialBaudRates are the line speeds SIA OSDP v2.2.2 defines. A device is
// required to support 9600 and negotiates upward with osdp_COMSET.
var SerialBaudRates = driver.SerialBaudRates

// DialSerial opens an RS-485 or RS-232 port directly, without a converter.
//
//	port, err := osdp.DialSerial(ctx, "/dev/ttyUSB0", osdp.WithBaud(9600))
//
// Supported on linux and darwin. Other platforms return ErrSerialUnsupported
// rather than failing to build, so one binary still targets every platform this
// library supports and can reach a line through DialTCP instead.
//
// The port is raw 8N1 with no flow control -- OSDP is binary on a shared wire,
// and a tty layer that translated a carriage return would corrupt frames in
// ways that look like line noise.
func DialSerial(ctx context.Context, device string, opts ...SerialOption) (Port, error) {
	return driver.DialSerial(ctx, device, opts...)
}

// WithBaud sets the line speed. The default is 9600, the only rate every OSDP
// device is required to support.
func WithBaud(baud int) SerialOption { return driver.WithBaud(baud) }

// Serial errors, matched with errors.Is.
var (
	// ErrUnsupportedBaud means a line speed OSDP does not define. Both ends
	// have to agree and a device only speaks the standard rates, so a typo
	// here produces a line that carries nothing.
	ErrUnsupportedBaud = driver.ErrUnsupportedBaud

	// ErrSerialUnsupported means this platform has no serial implementation.
	ErrSerialUnsupported = driver.ErrSerialUnsupported
)

// Pipe returns two Ports connected in memory: a panel and a device, for running
// the whole stack end to end without hardware.
func Pipe() (panel, device Port) { return driver.Pipe() }

// Transport errors, matched with errors.Is.
var (
	// ErrTimeout means a read deadline passed with no octets. It is the
	// ordinary signal that a device did not answer, not a fault.
	ErrTimeout = transport.ErrTimeout

	// ErrPortClosed means the port has been closed.
	ErrPortClosed = transport.ErrClosed

	// ErrShortWrite means a port accepted only part of a frame.
	ErrShortWrite = transport.ErrShortWrite
)
