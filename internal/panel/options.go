// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package panel

// defaultEventBuffer is how many events a panel holds before the poll cycle
// waits for the consumer.
//
// Sixteen is roughly one pass over a small line: enough that a consumer doing
// ordinary work -- writing a row to a database, say -- never stalls polling,
// and small enough that a consumer which has actually stopped is noticed
// quickly rather than after a megabyte of credentials has piled up in memory.
const defaultEventBuffer = 16

// settings are the options a panel was built with.
type settings struct {
	clock  Clock
	buffer int
}

// defaults are what New uses when no option says otherwise.
func defaults() settings {
	return settings{clock: systemClock{}, buffer: defaultEventBuffer}
}

// Option configures a Panel at construction.
//
// Options exist so that adding a capability never breaks a caller: New keeps
// its two required arguments and everything optional arrives this way.
type Option func(*settings)

// WithClock supplies the clock the runtime waits on.
//
// This is what makes a poll cycle testable without a test that sleeps. It is
// also the seam for a deployment that has its own notion of time -- a simulated
// line replayed faster than real time, for one.
//
// A nil clock is ignored rather than accepted: a panel with no clock would
// panic on its first turnaround, and the specification's own advice about
// malformed input applies just as well to malformed configuration.
func WithClock(c Clock) Option {
	return func(s *settings) {
		if c != nil {
			s.clock = c
		}
	}
}

// WithEventBuffer sizes the event channel.
//
// Larger absorbs longer consumer stalls at the cost of holding more credentials
// in memory; zero makes every event a rendezvous with the consumer, which is
// the strictest backpressure available and the right choice when losing the
// ordering between an event and its handling would matter.
//
// A negative size is ignored.
func WithEventBuffer(n int) Option {
	return func(s *settings) {
		if n >= 0 {
			s.buffer = n
		}
	}
}
