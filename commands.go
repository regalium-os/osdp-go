// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package osdp

import (
	"time"

	"github.com/regalium-os/osdp-go/internal/cmd"
)

// Command types.
//
// These are what a panel does rather than what it hears: release a door, light
// a reader, sound an annunciator. Everything else in this library observes; a
// Message built here is the library acting.
type (
	// Output is one entry of an osdp_OUT command: which output point, what to
	// do with it, and for how long.
	Output = cmd.Output

	// OutputControl is what an osdp_OUT does to an output point.
	OutputControl = cmd.OutputControl

	// LEDState is one of the two states a reader LED carries -- the permanent
	// one it rests in, and the temporary one an access decision sets.
	LEDState = cmd.LEDState

	// LEDControl says whether an osdp_LED sets a state, cancels it, or leaves
	// it alone.
	LEDControl = cmd.LEDControl

	// Colour is an LED colour. Red and green are the two every reader in the
	// field has.
	Colour = cmd.Colour

	// Tone is what an osdp_BUZ does to a reader's audible annunciator.
	Tone = cmd.Tone
)

// Output control codes, SIA OSDP v2.2.2 §6.9.
const (
	OutputNOP           = cmd.OutputNOP
	OutputOffAbort      = cmd.OutputOffAbort
	OutputOnAbort       = cmd.OutputOnAbort
	OutputOffAfterTimer = cmd.OutputOffAfterTimer
	OutputOnAfterTimer  = cmd.OutputOnAfterTimer

	// OutputTimedOn energises an output for a timer and then lets it fall back
	// on its own. It is how a door is released, and the reason it is the timed
	// form rather than on-then-off is that a panel which dies mid-grant must
	// leave a locked door behind it, not an open one.
	OutputTimedOn  = cmd.OutputTimedOn
	OutputTimedOff = cmd.OutputTimedOff
)

// LED control codes. The permanent half of an osdp_LED has no cancel, so its
// set code is LEDSetPermanent and not LEDSet; the two halves do not share a
// table, and using the wrong one produces an LED that ignores the command.
const (
	LEDNOP          = cmd.LEDNOP
	LEDCancel       = cmd.LEDCancel
	LEDSet          = cmd.LEDSet
	LEDSetPermanent = cmd.LEDSetPermanent
)

// LED colours, SIA OSDP v2.2.2 §6.10.
const (
	ColourBlack   = cmd.ColourBlack
	ColourRed     = cmd.ColourRed
	ColourGreen   = cmd.ColourGreen
	ColourAmber   = cmd.ColourAmber
	ColourBlue    = cmd.ColourBlue
	ColourMagenta = cmd.ColourMagenta
	ColourCyan    = cmd.ColourCyan
	ColourWhite   = cmd.ColourWhite
)

// Buzzer tone codes, SIA OSDP v2.2.2 §6.11.
const (
	ToneNOP     = cmd.ToneNOP
	ToneOff     = cmd.ToneOff
	ToneDefault = cmd.ToneDefault
)

// OutputCommand builds an osdp_OUT for one or more output points.
//
// Several in one command are applied together by the device, so a door and the
// light beside it change in the same poll cycle rather than in two.
func OutputCommand(outputs ...Output) Message { return cmd.OutputCommand(outputs...) }

// LEDCommand builds an osdp_LED for one LED.
//
// hold is how long the temporary state lasts before the reader returns to the
// permanent one by itself.
func LEDCommand(reader, led byte, temporary LEDState, hold time.Duration, permanent LEDState) Message {
	return cmd.LEDCommand(reader, led, temporary, hold, permanent)
}

// BuzzerCommand builds an osdp_BUZ. A count of zero repeats until an explicit
// ToneOff stops it, which is how a door-held-open alarm is sounded.
func BuzzerCommand(reader byte, tone Tone, on, off time.Duration, count byte) Message {
	return cmd.BuzzerCommand(reader, tone, on, off, count)
}

// Unlock builds the pair of commands that grant access at a door: release the
// strike for hold, and show green on the reader for the same time.
//
// It is a convenience over OutputCommand and LEDCommand, and worth having
// because the two must agree. Both are timed rather than latched, so a panel
// that stops talking mid-grant leaves a locked door and a reader telling the
// truth about it.
//
// The caller sends them in order; they are separate commands because they are
// separate messages on the wire.
func Unlock(output, reader, led byte, hold time.Duration) (strike, indicator Message) {
	strike = OutputCommand(Output{Number: output, Control: OutputTimedOn, Timer: hold})

	indicator = LEDCommand(reader, led,
		LEDState{Control: LEDSet, OnColour: ColourGreen, OffColour: ColourGreen},
		hold,
		LEDState{Control: LEDNOP},
	)
	return strike, indicator
}
