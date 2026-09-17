// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package panel_test

import (
	"testing"
	"time"

	"github.com/regalium-os/osdp-go/internal/bus"
)

// eventWait bounds how long a test waits for the runtime to produce something.
// Generous, because it is a failure bound and not a timing assertion: a slow CI
// runner should not turn a working panel red.
const eventWait = 5 * time.Second

// waitFor consumes events until one of kind arrives, failing if the runtime
// stops or takes too long.
func waitFor(t *testing.T, events <-chan bus.Event, kind bus.Kind) bus.Event {
	t.Helper()

	deadline := time.After(eventWait)
	for {
		select {
		case ev, open := <-events:
			if !open {
				t.Fatalf("the runtime stopped before producing a %v event", kind)
			}
			if ev.Kind == kind {
				return ev
			}
		case <-deadline:
			t.Fatalf("no %v event within %v", kind, eventWait)
		}
	}
}

// waitClosed drains whatever is still buffered and asserts the channel closes.
//
// Draining first is the point: closing a channel does not discard what is
// already in it, so a receive keeps succeeding until the buffer empties. A test
// that checks for closure without draining is really checking that the buffer
// happened to be empty.
func waitClosed(t *testing.T, events <-chan bus.Event) {
	t.Helper()

	deadline := time.After(eventWait)
	for {
		select {
		case _, open := <-events:
			if !open {
				return
			}
		case <-deadline:
			t.Fatalf("the event channel was not closed within %v of Run returning", eventWait)
		}
	}
}
