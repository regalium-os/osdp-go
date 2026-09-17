// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package cmd

import "time"

// TextControl is what an osdp_TEXT command does to a reader's display.
// SIA OSDP v2.2.2 §6.12.
//
// The four codes are two choices crossed: whether the message persists or
// expires, and whether a line too long for the display wraps onto the next row
// or is cut off at the edge.
type TextControl byte

const (
	// TextPermanent shows the message until something replaces it, without
	// wrapping. It is the zero value's behaviour, and the right default: a
	// message a panel put on a door should stay there until the panel says
	// otherwise.
	TextPermanent TextControl = 0x01

	// TextPermanentWrap is the same, wrapping a long line onto the next row.
	TextPermanentWrap TextControl = 0x02

	// TextTemporary shows the message for Hold and then restores whatever was
	// there before, without wrapping.
	TextTemporary TextControl = 0x03

	// TextTemporaryWrap is the same, with wrapping.
	TextTemporaryWrap TextControl = 0x04
)

// Temporary reports whether this control code expires on a timer.
func (c TextControl) Temporary() bool {
	return c == TextTemporary || c == TextTemporaryWrap
}

// String implements fmt.Stringer.
func (c TextControl) String() string {
	switch c {
	case TextPermanent:
		return "permanent"
	case TextPermanentWrap:
		return "permanent_wrap"
	case TextTemporary:
		return "temporary"
	case TextTemporaryWrap:
		return "temporary_wrap"
	default:
		return "unknown"
	}
}

// Display field limits. The length is one octet, so a message longer than this
// cannot be expressed; the hold time is one octet of seconds.
const (
	MaxTextLength = 0xFF
	MaxTextHold   = 0xFF
)

// TextDisplay is a message to put on a reader's display.
//
// # The zero value is useful
//
// It shows a message permanently at the top-left of the first reader's display,
// which is what a panel wants the overwhelming majority of the time:
//
//	cmd.TextCommand(cmd.TextDisplay{Content: "DOOR SECURE"})
//
// That is worth doing deliberately rather than leaving to chance, because Row
// and Column are numbered from one on the wire and a zero in either is not a
// position at all. See those fields.
type TextDisplay struct {
	// Reader is the reader whose display to write, counting from zero.
	Reader byte

	// Control says whether the message persists or expires, and whether it
	// wraps. Zero means TextPermanent.
	Control TextControl

	// Hold is how long a temporary message stays up. It is ignored by the
	// permanent control codes.
	//
	// The wire carries this in WHOLE SECONDS, not in the hundred-millisecond
	// units osdp_LED and osdp_BUZ use. A duration finer than a second rounds
	// down, and half a second rounds down to zero -- which a device reads as
	// no time at all. The inconsistency is the specification's; hiding it
	// behind a time.Duration that silently means something different here than
	// it does two files away would be worse than naming it.
	Hold time.Duration

	// Row and Column are where the message starts, NUMBERED FROM ONE: the
	// top-left of a display is row 1, column 1.
	//
	// Zero is not a position, so zero in either field is sent as one. That
	// makes the zero value useful and costs a caller nothing, because a
	// literal zero could only ever have been a mistake.
	Row    byte
	Column byte

	// Content is the message, in ASCII.
	//
	// OSDP displays are ASCII devices. A multi-byte rune is sent as its UTF-8
	// octets and renders as that many pieces of rubbish, so a caller with
	// non-ASCII text must transliterate it before arriving here -- this
	// package will not silently alter a message somebody chose to display.
	//
	// A message longer than MaxTextLength is truncated: the length field is a
	// single octet, and a longer one would wrap it and leave the device
	// reading the remainder of the message as though it were a new frame.
	Content string
}

// TextCommand builds an osdp_TEXT message.
//
// The returned message owns its payload; the string is copied.
func TextCommand(d TextDisplay) Message {
	content := d.Content
	if len(content) > MaxTextLength {
		content = content[:MaxTextLength]
	}

	control := d.Control
	if control == 0 {
		control = TextPermanent
	}

	data := make([]byte, 0, textHeaderSize+len(content))
	data = append(data,
		d.Reader,
		byte(control),
		byte(seconds(d.Hold, MaxTextHold)),
		originOf(d.Row),
		originOf(d.Column),
		byte(len(content)),
	)
	return Message{Code: Text, Data: append(data, content...)}
}

// textHeaderSize is the fixed part of an osdp_TEXT payload: reader, control,
// hold time, row, column and length.
const textHeaderSize = 6

// originOf maps a zero to the wire's origin of one. See TextDisplay.Row.
func originOf(n byte) byte {
	if n == 0 {
		return 1
	}
	return n
}

// seconds converts d to whole seconds, saturating at limit.
//
// It is deliberately not ticks: osdp_TEXT counts in seconds where osdp_LED and
// osdp_BUZ count in hundreds of milliseconds, and sharing a helper between the
// two is how a message that should hold for five seconds holds for half of one.
func seconds(d time.Duration, limit int) int {
	if d <= 0 {
		return 0
	}
	n := int(d / time.Second)
	if n > limit {
		return limit
	}
	return n
}
