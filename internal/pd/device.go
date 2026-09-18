// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd

import (
	"sync"
	"time"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
)

// Device is one peripheral device: a card reader, from the reader's own side.
//
// It is the server half of OSDP. The roles are inverted from intuition -- the
// control panel is the client and initiates every exchange, and a device never
// speaks first -- so a Device has no loop and no lifecycle to run. It is a
// function from a command to a reply, plus the state that makes the same
// command twice mean two different things, or the same thing twice. See Handle.
//
// # What it owns
//
// Its address, what it says it is (osdp_PDID), what it says it can do
// (osdp_PDCAP), the state of its contacts, the events waiting to be reported,
// and the sequencing discipline of SIA OSDP v2.2.2 §5.7 -- which is the part
// that is more than a switch statement.
//
// # Concurrency
//
// A Device is safe for concurrent use. That is not a convenience: the
// application goroutine that notices a card at the reader and the runtime
// goroutine that answers a poll are genuinely different goroutines, and the
// second must not have to wait for the first. Every method below takes the
// device's lock, and only one command is processed at a time -- which the
// protocol requires anyway, there being one panel.
//
// The zero value is not usable. Use New.
type Device struct {
	// cfg is fixed by New and never written afterwards, so it is read without
	// the mutex.
	cfg settings

	mu sync.Mutex

	// address is mutable because osdp_COMSET moves a device onto a new one.
	address frame.Address

	inputs       []bool
	outputs      []bool
	tamper       bool
	powerFailure bool

	// pending holds events awaiting a poll, oldest first.
	pending []cmd.Message

	// seen and lastSeen are how silence on the line is measured. seen is
	// separate because the zero time is a legitimate reading from an injected
	// clock, and "never heard from" must not look like "heard from at the
	// epoch".
	seen     bool
	lastSeen time.Time

	// The reply cache. See Handle.
	cached      frame.Frame
	cachedSeq   uint8
	cachedValid bool

	// adopting holds the address an osdp_COMSET asked for, applied after the
	// reply has been built. See applyCommunication.
	adopting *frame.Address
}

// New returns a device answering to addr.
//
// It starts nothing and touches nothing: a Device can be constructed,
// inspected and discarded without an octet reaching a line, which is the same
// contract the panel side keeps.
//
// addr must be a real device address. The configuration address 0x7F is
// refused, because it is not an address a device has -- it is the one a panel
// uses to reach a device whose address it does not know, and answering it is
// WithConfigurationAddress rather than a value here.
func New(addr frame.Address, opts ...Option) (*Device, error) {
	if !addr.Valid() || addr == frame.BroadcastAddress {
		return nil, frame.ErrInvalidAddress
	}

	cfg := defaults()
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if cfg.capabilities == nil {
		cfg.capabilities = defaultCapabilities(cfg.inputs, cfg.outputs, cfg.readers)
	}

	return &Device{
		cfg:     cfg,
		address: addr,
		inputs:  make([]bool, cfg.inputs),
		outputs: make([]bool, cfg.outputs),
	}, nil
}

// Address returns the address the device currently answers to, which is not
// necessarily the one it was built with: osdp_COMSET moves it.
func (d *Device) Address() frame.Address {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.address
}

// Identity returns what the device answers osdp_ID with.
func (d *Device) Identity() cmd.DeviceID { return d.cfg.identity }

// Capabilities returns what the device answers osdp_CAP with.
//
// The report is copied, so a caller cannot reach into the device's own answer
// and change what it claims after the panel has already believed it.
func (d *Device) Capabilities() cmd.CapabilityReport {
	return append(cmd.CapabilityReport(nil), d.cfg.capabilities...)
}

// LastCommand returns when a command was last accepted from the panel, and
// whether one ever has been.
//
// It is the device's own view of whether the bus is alive. A panel that has
// stopped polling is indistinguishable, from here, from a cable that has come
// out of the wall -- which is why the answer is a timestamp rather than a
// verdict.
func (d *Device) LastCommand() (time.Time, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.lastSeen, d.seen
}
