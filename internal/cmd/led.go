// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package cmd

import "time"

// Colour is an LED colour in an osdp_LED command. SIA OSDP v2.2.2 §6.10.
type Colour byte

// LED colours.
//
// Readers differ in what they can actually show: a two-colour LED given
// ColourMagenta will pick something, and which something is the vendor's
// business. Red and green are the two every reader in the field has, which is
// why access decisions are conventionally signalled with those.
const (
	ColourBlack Colour = 0 // off
	ColourRed   Colour = 1
	ColourGreen Colour = 2
	ColourAmber Colour = 3
	ColourBlue  Colour = 4

	// The remaining three are in the specification and rare in hardware.
	ColourMagenta Colour = 5
	ColourCyan    Colour = 6
	ColourWhite   Colour = 7
)

// LEDControl is whether an osdp_LED command sets, cancels or ignores one of
// the two states an LED carries.
type LEDControl byte

const (
	// LEDNOP leaves this state alone. It is the zero value, so an
	// unconfigured half of a command changes nothing.
	LEDNOP LEDControl = 0x00

	// LEDCancel abandons the temporary state and returns the LED to its
	// permanent one. Valid only for the temporary half.
	LEDCancel LEDControl = 0x01

	// LEDSet applies the state described. For the temporary half it also
	// starts the timer; for the permanent half there is no timer.
	LEDSet LEDControl = 0x02
)

// LEDSetPermanent is the permanent half's set code.
//
// It is 0x01 rather than LEDSet's 0x02, because the two halves of an osdp_LED
// command do not share a code table -- the permanent half has no cancel, so its
// codes are numbered from one. Getting this wrong produces an LED that ignores
// the command rather than an error, which is why it has a name of its own.
const LEDSetPermanent LEDControl = 0x01

// LEDState is one of the two states an LED carries: what it shows, and how it
// blinks while showing it.
type LEDState struct {
	// Control says whether to set this state, cancel it, or leave it alone.
	Control LEDControl

	// On and Off are one blink cycle. Both zero with Control set means steady
	// illumination in OnColour.
	On, Off time.Duration

	// OnColour and OffColour are what the LED shows in each half of the cycle.
	// A steady light uses OnColour; a red-black blink is OnColour red and
	// OffColour black.
	OnColour, OffColour Colour
}

// LEDCommand builds an osdp_LED message for one LED.
//
// # The two states
//
// An LED carries a permanent state and a temporary one, and the temporary wins
// while its timer runs. That is exactly the shape an access decision needs: the
// permanent state is what the door looks like at rest -- red, say -- and a
// grant sets a temporary green for three seconds. When the timer expires the
// reader returns to red on its own, so a panel that dies mid-grant leaves a
// reader showing the truth rather than a permanent green.
//
// temporary.Timer is the duration the temporary state holds, taken from the
// temporary state's own On and Off being a blink cycle within it.
//
// The returned message owns its payload.
func LEDCommand(reader, led byte, temporary LEDState, hold time.Duration, permanent LEDState) Message {
	t := ticks(hold, maxLEDTimer)

	return Message{Code: LED, Data: []byte{
		reader,
		led,

		// The temporary half, nine octets: control, blink cycle, colours, and
		// the two-octet timer that makes it temporary.
		byte(temporary.Control),
		byte(ticks(temporary.On, maxLEDTime)),
		byte(ticks(temporary.Off, maxLEDTime)),
		byte(temporary.OnColour),
		byte(temporary.OffColour),
		byte(t), byte(t >> 8),

		// The permanent half, five octets: the same but with no timer, because
		// permanent is what it is until something says otherwise.
		byte(permanent.Control),
		byte(ticks(permanent.On, maxLEDTime)),
		byte(ticks(permanent.Off, maxLEDTime)),
		byte(permanent.OnColour),
		byte(permanent.OffColour),
	}}
}

// LED field limits: the blink times are one octet each, the temporary timer two.
const (
	maxLEDTime  = 0xFF
	maxLEDTimer = 0xFFFF
)

// LEDBlockSize is the fixed width of an osdp_LED control block.
//
// It is asserted by the Phase 1 fixture corpus, where a real fourteen-octet
// block is decoded byte for byte: see the led-command record in
// internal/frame/testdata/framing.hex.
const LEDBlockSize = 14
