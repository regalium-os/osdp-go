// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package panel

import (
	"context"
	"errors"
	"sync"

	"github.com/regalium-os/osdp-go/internal/bus"
	"github.com/regalium-os/osdp-go/internal/transport"
)

// Runtime errors, matched with errors.Is.
var (
	// ErrNoDevices reports a panel asked to run a bus with no addresses on it.
	// It is a configuration mistake rather than a line fault, which is why it
	// is an error return and not an event.
	ErrNoDevices = errors.New("osdp/panel: the bus has no devices")

	// ErrAlreadyRun reports a second call to Run. A panel drives one line once;
	// a second driver would interleave commands mid-exchange and desynchronise
	// every device on it.
	ErrAlreadyRun = errors.New("osdp/panel: Run has already been called")
)

// Panel drives one OSDP line: it polls the bus's devices in turn, puts the
// commands on a port, reads what comes back, and publishes the events.
//
// # Lifecycle
//
// New constructs and starts nothing, so a panel may be built, inspected and
// discarded. Run blocks until the context is cancelled or the line fails, and
// nothing it started outlives it. Close is idempotent and may be called after
// a failed Run.
//
// # Concurrency
//
// Run may be called once, from one goroutine. Events may be consumed from
// another; that is the expected shape. Close may be called from any.
type Panel struct {
	bus    *bus.Bus
	port   transport.Port
	clock  Clock
	events chan bus.Event

	// requests carries application commands from whatever goroutine submitted
	// them to the one that drives the line. It is the only way into the bus
	// from outside, which is what lets the bus stay single-threaded.
	requests chan request

	// stopped is closed when Run returns, so a caller blocked in Send is
	// released rather than waiting on a loop that has finished.
	stopped chan struct{}

	started   bool
	closeOnce sync.Once
	closeErr  error
}

// New returns a panel that will drive b over port.
//
// It starts no goroutine, opens nothing and blocks on nothing. port is not read
// from or written to until Run.
func New(b *bus.Bus, port transport.Port, opts ...Option) *Panel {
	cfg := defaults()
	for _, opt := range opts {
		opt(&cfg)
	}

	return &Panel{
		bus:      b,
		port:     port,
		clock:    cfg.clock,
		events:   make(chan bus.Event, cfg.buffer),
		requests: make(chan request, cfg.requests),
		stopped:  make(chan struct{}),
	}
}

// Events is the stream of what happened on the line.
//
// # Who closes it
//
// Run does, as it returns, and only then. A consumer ranging over this channel
// therefore terminates exactly when the runtime does, with no separate
// signal to watch.
//
// # What happens when the reader is slow
//
// The poll cycle stops until there is room. That is a decision, not an
// oversight: the alternative is dropping events, and the event this library
// most often carries is a credential presented at a door. A panel that cannot
// keep up with its own readers must slow down rather than quietly forget that
// somebody badged in. The buffer exists to absorb bursts; WithEventBuffer sizes
// it, and a consumer that stops reading altogether will stall the line.
//
// A cancelled context always wins over a full buffer, so a stalled panel still
// shuts down promptly.
func (p *Panel) Events() <-chan bus.Event { return p.events }

// Run drives the line until ctx is cancelled or the port fails.
//
// A cancelled context is not an error: Run returns nil, having closed the event
// channel. Anything else -- a port that closed underneath it, a write that did
// not complete -- is returned as it came.
//
// A device that stops answering is not a failure either. It is reported through
// the event stream, because on a bus of hundreds of readers one being
// unreachable is information to act on rather than a reason to stop polling the
// other ninety-nine.
//
// # How quickly it stops
//
// Cancellation is observed between transactions and while waiting to publish an
// event, both of which are immediate. It is not observed inside a blocked read
// or write, because no context can interrupt a blocked syscall: those are
// bounded by the line's ReplyTimeout instead. Shutdown therefore takes up to
// one reply timeout, not zero, and a port that cannot set a write deadline (see
// transport.WriteDeadliner) is bounded only by Close.
func (p *Panel) Run(ctx context.Context) error {
	if p.started {
		return ErrAlreadyRun
	}
	p.started = true
	defer close(p.events)

	// Release anyone blocked in Send. It closes before the event channel does,
	// so a caller cannot be told the panel is running by one signal while the
	// other says it has stopped.
	defer close(p.stopped)

	if len(p.bus.Devices()) == 0 {
		return ErrNoDevices
	}

	r := newReader(p.port)
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}

		// Take whatever the application has asked for since the last
		// transaction. On this goroutine and no other: it is what keeps the
		// bus's single-threaded contract true while Send is called from
		// anywhere.
		p.drainRequests()

		if err := p.cycle(ctx, r); err != nil {
			// Cancellation wins over whatever the port said. Shutting down
			// often stops the far end reading, so the last cycle can fail on a
			// write timeout that is a consequence of the stop rather than a
			// fault worth reporting as one.
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
	}
}

// Close releases the port. It is idempotent and safe after a failed Run.
//
// It does not stop Run; cancelling the context does that. Closing the port out
// from under a running panel makes Run return the port's error, which is a
// blunter shutdown than cancellation and reports as a failure rather than a
// clean stop.
func (p *Panel) Close() error {
	p.closeOnce.Do(func() { p.closeErr = p.port.Close() })
	return p.closeErr
}

// emit publishes an event, blocking until the consumer takes it or ctx is done.
// See Events for why blocking is the right answer here.
func (p *Panel) emit(ctx context.Context, ev bus.Event) error {
	if ev.Kind == bus.KindNone {
		return nil
	}
	select {
	case p.events <- ev:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
