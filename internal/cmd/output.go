// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package cmd

import "time"

// tick is the unit every duration in an OSDP command is expressed in.
//
// The protocol counts time in hundreds of milliseconds, in one octet or two.
// The builders here take a time.Duration and convert, because a caller
// reasoning about a door strike thinks in seconds and should not be the one
// dividing by a hundred.
const tick = 100 * time.Millisecond

// ticks converts d to the protocol's unit, saturating at limit rather than
// wrapping.
//
// Saturation matters: a wrapped timer is a door that relocks in 200ms instead
// of staying open for the ten minutes somebody asked for, and it would be
// silent. A caller wanting longer than the field can express needs to know the
// field cannot express it, which is what Validate reports.
func ticks(d time.Duration, limit int) int {
	if d <= 0 {
		return 0
	}
	n := int(d / tick)
	if n > limit {
		return limit
	}
	return n
}

// OutputControl is what an osdp_OUT command does to one output point.
// SIA OSDP v2.2.2 §6.9.
type OutputControl byte

// Output control codes.
//
// The distinction between "abort" and "allow to complete" is the one that
// matters in the field: a door held open by a timed operation and then told to
// lock has to be told which of the two instructions wins.
const (
	// OutputNOP leaves the output alone. It is the zero value, so an
	// unconfigured entry in a multi-output command changes nothing.
	OutputNOP OutputControl = 0x00

	// OutputOffAbort sets the permanent state off and abandons any timed
	// operation in progress.
	OutputOffAbort OutputControl = 0x01

	// OutputOnAbort sets the permanent state on and abandons any timed
	// operation in progress.
	OutputOnAbort OutputControl = 0x02

	// OutputOffAfterTimer sets the permanent state off, letting a timed
	// operation already running finish first.
	OutputOffAfterTimer OutputControl = 0x03

	// OutputOnAfterTimer sets the permanent state on, letting a timed
	// operation already running finish first.
	OutputOnAfterTimer OutputControl = 0x04

	// OutputTimedOn energises the output for the timer and then returns it to
	// its permanent state. This is how a door is released: a strike is held
	// for a few seconds and relocks itself, so a panel that crashes mid-grant
	// leaves a locked door rather than an open one.
	OutputTimedOn OutputControl = 0x05

	// OutputTimedOff de-energises the output for the timer and then returns it
	// to its permanent state. The failsafe counterpart of OutputTimedOn, for a
	// lock wired so that power holds it shut.
	OutputTimedOff OutputControl = 0x06
)

// Output is one entry of an osdp_OUT command.
type Output struct {
	// Number is the output point on the device, counting from zero.
	Number byte

	// Control is what to do with it.
	Control OutputControl

	// Timer is how long a timed operation lasts. It is ignored by the
	// permanent control codes, and is rounded down to the protocol's
	// hundred-millisecond unit. The field holds two octets, so the longest
	// expressible timer is 6553.5 seconds -- a little under two hours.
	Timer time.Duration
}

// maxOutputTimer is the largest value the two-octet timer field holds.
const maxOutputTimer = 0xFFFF

// OutputCommand builds an osdp_OUT message for one or more output points.
//
// The returned message owns its payload; nothing passed in is retained.
//
// Sending several outputs in one command is not merely an efficiency: a device
// applies them together, so a panel unlocking a door and lighting a strike
// indicator does both in the same poll cycle rather than in two, which is the
// difference between a door and its light agreeing.
func OutputCommand(outputs ...Output) Message {
	data := make([]byte, 0, len(outputs)*4)
	for _, o := range outputs {
		t := ticks(o.Timer, maxOutputTimer)
		data = append(data, o.Number, byte(o.Control), byte(t), byte(t>>8))
	}
	return Message{Code: Out, Data: data}
}

// Tone is what an osdp_BUZ command does to a reader's audible annunciator.
// SIA OSDP v2.2.2 §6.11.
type Tone byte

// Tone codes. Readers vary in what they can actually sound; the specification
// defines only silence and a default tone, and a device that offers more does
// so through osdp_MFG.
const (
	// ToneNOP leaves the annunciator alone.
	ToneNOP Tone = 0x00
	// ToneOff silences it.
	ToneOff Tone = 0x01
	// ToneDefault sounds the device's default tone.
	ToneDefault Tone = 0x02
)

// maxToneTime is the largest on or off time, each a single octet.
const maxToneTime = 0xFF

// BuzzerCommand builds an osdp_BUZ message.
//
// on and off are one cycle of the pattern and count is how many times it
// repeats; a count of zero means repeat until told otherwise, which is how a
// held-open-door alarm is sounded and why it needs an explicit ToneOff to stop.
//
// The returned message owns its payload.
func BuzzerCommand(reader byte, tone Tone, on, off time.Duration, count byte) Message {
	return Message{Code: Buz, Data: []byte{
		reader,
		byte(tone),
		byte(ticks(on, maxToneTime)),
		byte(ticks(off, maxToneTime)),
		count,
	}}
}
