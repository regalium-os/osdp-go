// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd

import "time"

// Clock is the device's view of time.
//
// It is one method wide, and deliberately narrower than the runtime clock on
// the panel side: a peripheral device never waits for anything. It is driven
// entirely by the commands that arrive, so the only question it ever asks of a
// clock is what time it is now.
//
// Time is injected for the usual reason -- a communication-timeout test that
// sleeps for eight seconds is a test people stop running -- and for a second
// one peculiar to this side of the bus. A reader on an isolated line may have
// no wall clock worth trusting, and the application knows what it should use
// instead; a monotonic counter is a perfectly good implementation, because
// nothing here depends on the absolute value.
//
// An implementation must be safe for concurrent use.
type Clock interface {
	// Now is the current time. It is used only to measure the gap between one
	// command and the next.
	Now() time.Time
}

// systemClock is the real one, used when no option supplies another.
type systemClock struct{}

// Now returns the wall clock time.
func (systemClock) Now() time.Time { return time.Now() }
