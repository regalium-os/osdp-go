// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd

import (
	"errors"
	"fmt"
)

// ErrNoReply is the class of condition under which a peripheral device must put
// nothing at all on the wire.
//
// It is a class rather than a single error because a caller almost never needs
// to know which of the three it was. Every device on a multidrop line sees every
// octet, so a frame that is not this device's business is answered with silence;
// a device that answered anyway would transmit over the device that was actually
// addressed, and the panel would receive neither reply.
//
// The errors below all satisfy errors.Is(err, ErrNoReply). A runtime driving a
// port should treat that as "go back to reading", never as a fault.
var ErrNoReply = errors.New("osdp/pd: the device must not answer")

var (
	// ErrNotAddressed reports a command carrying another device's address.
	//
	// Per SIA OSDP v2.2.2 §5.3 a device answers its own address and nothing
	// else, with one exception: the configuration address 0x7F, which this
	// package answers only when WithConfigurationAddress says it may.
	ErrNotAddressed = fmt.Errorf("%w: addressed to another device", ErrNoReply)

	// ErrNotCommand reports a reply frame rather than a command -- another
	// device's answer, seen because RS-485 is a shared line.
	//
	// Per SIA OSDP v2.2.2 §5.4 the high bit of the address octet carries the
	// direction, which is what makes the two distinguishable at all.
	ErrNotCommand = fmt.Errorf("%w: the frame is a reply, not a command", ErrNoReply)

	// ErrCorrupt reports a frame whose error check does not match the octets it
	// covers.
	//
	// The specification defines osdp_NAK reason 0x01 for exactly this case and
	// this package will not send it. A frame that failed its check has an
	// address octet that failed its check too, so answering means transmitting
	// on a guess about who was being addressed. Silence costs the panel one
	// reply timeout; a collision costs it the device that really was addressed.
	ErrCorrupt = fmt.Errorf("%w: the error check does not match", ErrNoReply)
)

// ErrQueueFull reports that the device already holds as many unreported events
// as WithEventQueue allows.
//
// It is returned rather than swallowed because the alternative is a credential
// that was presented at a reader and then silently never mentioned again. The
// application knows whether to retry, to sound a tone at the reader, or to drop
// it; this package does not, and guessing would make a lost card read look like
// a card that was never presented.
var ErrQueueFull = errors.New("osdp/pd: event queue is full")

// ErrNoSuchContact reports an input index the device was not configured with.
// See WithContacts.
var ErrNoSuchContact = errors.New("osdp/pd: no such contact on this device")

// ErrTooManyContacts reports a contact count that cannot be expressed.
//
// osdp_PDCAP gives each count a single octet (SIA OSDP v2.2.2 §6.6), so a
// device claiming more than 255 inputs would report some smaller number and
// then answer osdp_ISTAT with a payload that disagrees with it.
var ErrTooManyContacts = errors.New("osdp/pd: more contacts than osdp_PDCAP can report")

// ErrInvalidQueue reports an event queue depth below one. A device with nowhere
// to hold a card read cannot report one.
var ErrInvalidQueue = errors.New("osdp/pd: event queue depth must be at least one")
