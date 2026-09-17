// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package panel

import (
	"context"
	"time"
)

// Clock is the runtime's view of time.
//
// It is injected because the rest of this library has no clock at all: bus
// returns a decision and a deadline, and something has to be the thing that
// actually waits. Making that thing an interface is what keeps a poll cycle
// testable without a test that sleeps -- and a test suite that sleeps is a test
// suite people stop running.
//
// An implementation must be safe for concurrent use.
type Clock interface {
	// Now is the current time, used to stamp when a reply arrived and to
	// compute read deadlines.
	Now() time.Time

	// Sleep blocks for d, or until ctx is done, whichever comes first. It
	// returns ctx.Err() in the second case and nil in the first, so a caller
	// can treat a cancelled wait exactly like a cancelled anything else.
	//
	// A non-positive d returns immediately, without consulting ctx: the
	// turnaround on a line fast enough not to need one should not become a
	// cancellation check in the hot path.
	Sleep(ctx context.Context, d time.Duration) error
}

// systemClock is the real one, used when no other is supplied.
type systemClock struct{}

// Now returns the wall clock time.
func (systemClock) Now() time.Time { return time.Now() }

// Sleep waits for d or for ctx, using a timer that is always stopped.
func (systemClock) Sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}

	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
