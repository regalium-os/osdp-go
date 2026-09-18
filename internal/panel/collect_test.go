// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package panel_test

import (
	"sync"
	"testing"
	"time"

	"github.com/regalium-os/osdp-go/internal/bus"
	"github.com/regalium-os/osdp-go/internal/panel"
)

// collector drains a panel's event stream in the background.
//
// # Why every test that sends a command needs one
//
// The runtime stops the poll cycle when the event buffer fills. That is the
// documented backpressure and it is deliberate -- the alternative is dropping a
// credential -- but it has a consequence worth stating plainly: a stalled cycle
// never reaches the application's queued commands either, and Send waits for
// the run loop to accept them. A test that reads one event and then stops
// reading therefore deadlocks itself, with the test waiting on Send and the
// runtime waiting on the test.
//
// It does not fail quickly. Send has no deadline of its own; it waits on the
// context, which a blocked test never cancels. CI found this as a pair of tests
// hanging for the full ten-minute timeout, having passed locally for days
// because a fast machine emptied the buffer before the sends landed.
//
// Draining continuously is what the runtime's contract actually asks for, and
// it is what a real consumer does.
type collector struct {
	mu   sync.Mutex
	seen []bus.Event
	done chan struct{}
}

// collect starts draining p's events until the channel closes.
func collect(p *panel.Panel) *collector {
	c := &collector{done: make(chan struct{})}

	go func() {
		defer close(c.done)
		for ev := range p.Events() {
			c.mu.Lock()
			c.seen = append(c.seen, ev)
			c.mu.Unlock()
		}
	}()
	return c
}

// await blocks until an event of this kind has arrived, and returns it.
func (c *collector) await(t *testing.T, kind bus.Kind) bus.Event {
	t.Helper()

	deadline := time.Now().Add(eventWait)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		for _, ev := range c.seen {
			if ev.Kind == kind {
				c.mu.Unlock()
				return ev
			}
		}
		closed := c.closed()
		c.mu.Unlock()

		if closed {
			t.Fatalf("the runtime stopped before producing a %v event", kind)
		}
		time.Sleep(time.Millisecond)
	}

	t.Fatalf("no %v event within %v", kind, eventWait)
	return bus.Event{}
}

// closed reports whether the event stream has ended.
func (c *collector) closed() bool {
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}
