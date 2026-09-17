// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package osdp

import "github.com/regalium-os/osdp-go/internal/cmd"

// Contact monitoring: what a panel learns about a door rather than about a
// credential. A card reader that cannot tell you the door is standing open is
// a card reader, not an access-control system.
type (
	// StatusChange is one contact reported at one state: a door position, a
	// request-to-exit, a tamper switch, a strike relay.
	//
	// Active is the asserted state, which is not the same as the alarming one:
	// whether a closed contact means a door is shut or forced depends on how it
	// was wired, and that is the panel's business rather than this library's.
	StatusChange = cmd.StatusChange

	// StatusKind says which kind of contact a change concerns.
	StatusKind = cmd.StatusKind
)

// Kinds of contact, SIA OSDP v2.2.2 §6. The values match the domain schema's
// osdp.event.v1.StatusKind, so a change and the record of it cannot disagree
// about what changed.
const (
	StatusInput  = cmd.StatusInput
	StatusOutput = cmd.StatusOutput
	StatusTamper = cmd.StatusTamper
	StatusPower  = cmd.StatusPower
	StatusLocal  = cmd.StatusLocal
)

// A device with something to say reports it unsolicited, in answer to an
// ordinary poll, which is the usual way a panel learns a door opened. The four
// commands below exist for the other case: establishing what a device believes
// right now, after a panel restart or a line that has been quiet.

// LocalStatusCommand asks a device for its tamper and power state (osdp_LSTAT).
func LocalStatusCommand() Message { return cmd.LocalStatusCommand() }

// InputStatusCommand asks a device for its monitored inputs (osdp_ISTAT).
func InputStatusCommand() Message { return cmd.InputStatusCommand() }

// OutputStatusCommand asks a device for its controlled outputs (osdp_OSTAT).
func OutputStatusCommand() Message { return cmd.OutputStatusCommand() }

// ReaderStatusCommand asks a device for its reader heads (osdp_RSTAT).
func ReaderStatusCommand() Message { return cmd.ReaderStatusCommand() }
