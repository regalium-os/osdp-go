// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd

import (
	"time"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/secure"
)

// defaultEventQueue is how many unreported events a device holds.
//
// Eight is a handover depth rather than a backlog. A device reports at most one
// event per poll (SIA OSDP v2.2.2 §6.2), and a panel polls a small line several
// times a second, so eight absorbs a burst of card presentations and a chattering
// door contact without ever storing credentials for long. Deeper would mean
// reporting a card read seconds after the cardholder walked away.
const defaultEventQueue = 8

// settings are the options a device was built with. Everything here is fixed at
// construction and read without the mutex.
type settings struct {
	clock        Clock
	identity     cmd.DeviceID
	capabilities cmd.CapabilityReport
	inputs       int
	outputs      int
	readers      int
	queue        int
	timeout      time.Duration
	configAddr   bool
	strict       bool
	handler      Handler

	// The Secure Channel configuration. secure is the single flag the rest of
	// the package tests, so a half-supplied channel -- a suite with no source
	// of randomness -- is impossible to represent rather than merely unlikely.
	secure       bool
	suite        secure.CipherSuite
	key          secure.BaseKey
	nonce        NonceSource
	keyInstalled KeyInstalled
}

// defaults are what New uses when no option says otherwise: a plain reader with
// one reader head, no monitored inputs and no controlled outputs, which is what
// most of the installed base actually is.
func defaults() settings {
	return settings{clock: systemClock{}, readers: 1, queue: defaultEventQueue}
}

// validate reports a configuration the wire cannot carry.
func (s settings) validate() error {
	const maxContacts = 0xFF
	switch {
	case s.inputs < 0 || s.inputs > maxContacts,
		s.outputs < 0 || s.outputs > maxContacts,
		s.readers < 0 || s.readers > maxContacts:
		return ErrTooManyContacts
	case s.queue < 1:
		return ErrInvalidQueue
	}
	return nil
}

// Option configures a Device at construction.
//
// Options exist so that adding a capability never breaks a caller: New keeps
// its one required argument and everything else arrives this way. A nil Option
// is ignored, so a caller may build a slice with conditional entries.
type Option func(*settings)

// WithClock supplies the clock the device measures silence against.
//
// A nil clock is ignored rather than accepted. A device with no clock would
// fault the first time a command arrived, and the specification's advice about
// malformed input applies just as well to malformed configuration.
func WithClock(c Clock) Option {
	return func(s *settings) {
		if c != nil {
			s.clock = c
		}
	}
}

// WithIdentity supplies what the device answers osdp_ID with. SIA OSDP v2.2.2
// §6.4.
//
// VendorCode is the field that matters beyond identification: it is the IEEE
// OUI a panel's provider registry keys on, so a device that reports zeros gets
// the generic provider and none of its own quirk handling.
//
// The value is copied. cmd.DeviceID contains no slices, so nothing is retained.
func WithIdentity(id cmd.DeviceID) Option {
	return func(s *settings) { s.identity = id }
}

// WithCapabilities supplies the osdp_PDCAP report verbatim, in place of the one
// derived from the contact counts.
//
// Use it for a capability this package does not model -- a biometric sensor, a
// text display, a larger receive buffer. Nothing here reconciles the report
// against the rest of the configuration: a device claiming eight inputs and
// configured with four will answer osdp_ISTAT with four octets, and the panel
// will believe the last thing it was told. Keep them in step.
//
// The report is copied, so the caller may reuse the slice it passed.
func WithCapabilities(r cmd.CapabilityReport) Option {
	return func(s *settings) {
		if r != nil {
			s.capabilities = append(cmd.CapabilityReport(nil), r...)
		}
	}
}

// WithContacts sizes the device: monitored inputs, controlled outputs, and
// reader heads.
//
// The three are one option because they are one decision -- what this piece of
// hardware physically is -- and because they must agree with the capability
// report, which is derived from all three at once. Each is limited to 255 by
// the single item octet osdp_PDCAP gives it; New returns ErrTooManyContacts
// rather than reporting a count it would then contradict.
func WithContacts(inputs, outputs, readers int) Option {
	return func(s *settings) {
		s.inputs, s.outputs, s.readers = inputs, outputs, readers
	}
}

// WithConfigurationAddress makes the device also answer the configuration
// address 0x7F. SIA OSDP v2.2.2 §5.3.
//
// That address exists so a panel can reach a device whose address it does not
// know, which is how a reader out of the box is commissioned with osdp_COMSET.
// It is off by default because it is safe on exactly one kind of line: one with
// a single device on it. Enable it on a multidrop bus and every device answers
// the same command at once, and the panel hears a collision it cannot decode.
func WithConfigurationAddress() Option {
	return func(s *settings) { s.configAddr = true }
}

// WithCommunicationTimeout makes the device restart its exchange when the panel
// has been silent for longer than d.
//
// A device has no loop of its own, so it notices the silence only when the next
// command finally arrives -- which is the moment it matters, because that
// command is almost certainly the start of a new conversation with a panel that
// has been restarted or recabled. Restarting means forgetting the cached reply;
// queued events survive, because a card presented while the panel was away is
// still a card that was presented.
//
// Zero, the default, disables the check. A non-zero value should be comfortably
// longer than the panel's poll interval: too short and an ordinary pause
// between poll cycles discards a reply the panel is about to ask for again.
func WithCommunicationTimeout(d time.Duration) Option {
	return func(s *settings) {
		if d > 0 {
			s.timeout = d
		}
	}
}

// WithEventQueue sizes the queue of events awaiting a poll.
//
// See defaultEventQueue for why the default is small. A depth below one is
// refused by New: a device with nowhere to hold a card read cannot report one.
func WithEventQueue(n int) Option {
	return func(s *settings) { s.queue = n }
}

// WithStrictSequence makes an unexpected sequence number an osdp_NAK with
// reason 0x04 rather than an accepted command. SIA OSDP v2.2.2 §5.7.
//
// The default is lenient: a number that is neither a repeat of the last one nor
// the next in the 1-2-3 rotation is treated as a fresh command and the device
// resynchronises to it. That is the safer default against a real panel, because
// a single frame lost to noise then costs one command rather than a full
// restart of the exchange -- and the retransmission discipline that actually
// protects a door, replaying a repeated sequence number, is unaffected either
// way.
//
// Turn it on for conformance testing, or against a panel that relies on the NAK
// to discover it has lost synchronisation.
func WithStrictSequence() Option {
	return func(s *settings) { s.strict = true }
}
