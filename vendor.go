package osdp

import (
	"context"

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
