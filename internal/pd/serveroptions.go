// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd

import "time"

// defaultIdleTimeout is how long a server's read blocks before it looks up.
//
// It is a cancellation granularity, not a protocol timeout: a device is
// supposed to wait indefinitely for a panel that may be polling once a second
// or once a minute, so the only thing this bounds is how long Run takes to
// notice its context was cancelled. Half a second is short enough that shutdown
// feels immediate and long enough that a quiet line is not a busy loop.
//
// It is unrelated to WithCommunicationTimeout, which is about a panel that has
// gone away rather than about stopping.
const defaultIdleTimeout = 500 * time.Millisecond

// serverSettings are the options a Server was built with.
type serverSettings struct {
	clock Clock
	idle  time.Duration
}

// serverDefaults are what NewServer uses when no option says otherwise.
func serverDefaults() serverSettings {
	return serverSettings{clock: systemClock{}, idle: defaultIdleTimeout}
}

// ServerOption configures a Server at construction.
//
// It is a separate type from Option because the two configure different things:
// Option describes what the device *is* -- its identity, its contacts, its key
// -- and none of that changes because the same device is being served over a
// socket instead of a serial port. Merging them would let a caller pass
// WithIdentity to NewServer, where it would be silently ignored.
type ServerOption func(*serverSettings)

// WithServerClock supplies the clock the server computes port deadlines from.
//
// A nil clock is ignored. It is separate from the device's clock because the
// two measure different things: the device measures how long the panel has been
// silent, and the server measures how long to block in a read. A test usually
// wants a fake for the first and the real one for the second, since a port
// deadline is handed to the operating system and a fake time means a deadline
// in 1970.
func WithServerClock(c Clock) ServerOption {
	return func(s *serverSettings) {
		if c != nil {
			s.clock = c
		}
	}
}

// WithIdleTimeout sets how long a read blocks before the server checks whether
// its context has been cancelled. See defaultIdleTimeout for why this is a
// shutdown-latency knob rather than a protocol one.
//
// A non-positive value is ignored: a server that polled its context in a tight
// loop would spin a core to no purpose.
func WithIdleTimeout(d time.Duration) ServerOption {
	return func(s *serverSettings) {
		if d > 0 {
			s.idle = d
		}
	}
}
