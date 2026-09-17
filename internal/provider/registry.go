package provider

import (
	"github.com/regalium-os/osdp-go/internal/cmd"
)

// Registry resolves a device to the provider that handles it.
//
// Resolution is by OUI, taken from the device's osdp_PDID reply. A device whose
// vendor is not registered resolves to Generic, which is a working provider and
// not a failure: the great majority of OSDP traffic is vendor-neutral, and a
// panel should talk to an unknown reader rather than refuse it.
type Registry struct {
	byOUI   map[uint32]Provider
	generic Provider
}

// NewRegistry returns a registry holding the supplied providers plus Generic.
//
// A later provider with the same OUI replaces an earlier one, so an application
// can override a built-in vendor provider with its own.
func NewRegistry(providers ...Provider) *Registry {
	r := &Registry{byOUI: make(map[uint32]Provider, len(providers)), generic: Generic{}}
	for _, p := range providers {
		r.byOUI[p.OUI()] = p
	}
	return r
}

// For returns the provider handling this device, never nil.
func (r *Registry) For(id cmd.DeviceID) Provider {
	if p, ok := r.byOUI[id.OUI()]; ok {
		return p
	}
	return r.generic
}

// ForOUI returns the provider registered for oui, and whether one was found.
func (r *Registry) ForOUI(oui uint32) (Provider, bool) {
	p, ok := r.byOUI[oui]
	return p, ok
}

// Generic returns the fallback provider.
func (r *Registry) Generic() Provider { return r.generic }

// Names lists the registered vendors, excluding Generic.
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.byOUI))
	for _, p := range r.byOUI {
		out = append(out, p.Name())
	}
	return out
}
