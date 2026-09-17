// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package osdp

import "github.com/regalium-os/osdp-go/internal/panel"

// Runtime types.
//
// Everything else in this library computes: it takes arguments and returns a
// decision. Panel is the one thing that acts -- it owns a Port, waits on a
// clock, and turns those decisions into traffic.
type (
	// Panel drives one OSDP line. New constructs and starts nothing; Run
	// blocks until the context is cancelled or the line fails; Close is
	// idempotent. No goroutine outlives Run.
	Panel = panel.Panel

	// PanelOption configures a Panel at construction.
	PanelOption = panel.Option

	// Clock is the runtime's view of time, injected so that a poll cycle can
	// be driven without waiting for one.
	Clock = panel.Clock
)

// NewPanel returns a panel that will drive b over port.
//
// It starts no goroutine and touches no port until Run:
//
//	p := osdp.NewPanel(bus, port)
//	defer p.Close()
//
//	go func() {
//	    for event := range p.Events() {
//	        switch event.Kind {
//	        case osdp.EventCardRead: // event.Card is a credential; do not log it
//	        case osdp.EventOffline:  // a reader stopped answering
//	        }
//	    }
//	}()
//
//	if err := p.Run(ctx); err != nil { // nil when ctx is cancelled
//	    return err
//	}
//
// To act on a device rather than only listen to it, Panel.Send queues a
// command for the next time the cycle reaches that address:
//
//	strike, indicator := osdp.Unlock(0, 0, 0, 5*time.Second)
//	p.Send(ctx, addr, strike)
//	p.Send(ctx, addr, indicator)
//
// Send is safe to call from any goroutine while Run is executing. It is a
// method of the aliased type rather than something re-declared here, which is
// the whole reason the facade aliases instead of wrapping.
func NewPanel(b *Bus, port Port, opts ...PanelOption) *Panel {
	return panel.New(b, port, opts...)
}

// WithClock supplies the clock the runtime waits on. The default is the system
// clock; supply your own to drive a line faster than real time, or to test one
// without waiting for it.
func WithClock(c Clock) PanelOption { return panel.WithClock(c) }

// WithEventBuffer sizes the event channel.
//
// The runtime blocks rather than dropping when the buffer is full, so this is
// how long a consumer may stall before the poll cycle waits for it. Zero makes
// every event a rendezvous. See Panel.Events for why blocking is the choice.
func WithEventBuffer(n int) PanelOption { return panel.WithEventBuffer(n) }

// Runtime errors, matched with errors.Is.
var (
	// ErrUnknownDevice means a command was addressed to a device this panel
	// does not poll. The address list is fixed when the bus is built, so such
	// an address will not become valid by waiting.
	ErrUnknownDevice = panel.ErrUnknownDevice

	// ErrNotRunning means a command was submitted to a panel whose Run has
	// returned.
	ErrNotRunning = panel.ErrNotRunning

	// ErrNoDevices means a panel was asked to run a bus with no addresses on
	// it: a configuration mistake rather than a fault on the line.
	ErrNoDevices = panel.ErrNoDevices

	// ErrAlreadyRun means Run was called twice. A panel drives one line once;
	// a second driver would interleave commands mid-exchange and desynchronise
	// every device sharing it.
	ErrAlreadyRun = panel.ErrAlreadyRun
)
