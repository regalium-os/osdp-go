// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/regalium-os/osdp-go/internal/cmd"
)

// TestTextCommandLayout checks the wire layout against SIA OSDP v2.2.2 §6.12:
// reader, control, hold in seconds, row, column, length, then the characters.
func TestTextCommandLayout(t *testing.T) {
	m := cmd.TextCommand(cmd.TextDisplay{
		Reader:  0,
		Control: cmd.TextTemporary,
		Hold:    5 * time.Second,
		Row:     2,
		Column:  3,
		Content: "OPEN",
	})

	if m.Code != cmd.Text {
		t.Errorf("code = 0x%02X, want osdp_TEXT", byte(m.Code))
	}
	want := []byte{0x00, 0x03, 0x05, 0x02, 0x03, 0x04, 'O', 'P', 'E', 'N'}
	if !bytes.Equal(m.Data, want) {
		t.Errorf("payload = % X, want % X", m.Data, want)
	}
}

// TestTheHoldTimeIsSecondsNotTicks is the trap this command carries.
//
// osdp_LED and osdp_BUZ count in hundreds of milliseconds; osdp_TEXT counts in
// whole seconds. Reusing the one helper for both is how a message meant to hold
// for five seconds holds for half of one.
func TestTheHoldTimeIsSecondsNotTicks(t *testing.T) {
	m := cmd.TextCommand(cmd.TextDisplay{
		Control: cmd.TextTemporary,
		Hold:    5 * time.Second,
		Content: "X",
	})

	if got := m.Data[2]; got != 5 {
		t.Errorf("hold = %d, want 5 seconds (50 would mean it was read as ticks)", got)
	}
}

// TestSubSecondHoldRoundsDown, which a device reads as no time at all. Stated
// so the behaviour is a decision rather than a discovery.
func TestSubSecondHoldRoundsDown(t *testing.T) {
	m := cmd.TextCommand(cmd.TextDisplay{
		Control: cmd.TextTemporary,
		Hold:    900 * time.Millisecond,
		Content: "X",
	})

	if got := m.Data[2]; got != 0 {
		t.Errorf("hold = %d, want 0: the field cannot express less than a second", got)
	}
}

// TestTheZeroValueIsUsable: a permanent message at the top-left of the first
// reader is what a panel wants nearly always, and Row and Column count from one
// on the wire -- so a literal zero must not be sent as one.
func TestTheZeroValueIsUsable(t *testing.T) {
	m := cmd.TextCommand(cmd.TextDisplay{Content: "DOOR SECURE"})

	if got := m.Data[1]; got != byte(cmd.TextPermanent) {
		t.Errorf("control = 0x%02X, want permanent", got)
	}
	if m.Data[3] != 1 || m.Data[4] != 1 {
		t.Errorf("origin = row %d col %d, want 1/1: the wire numbers from one",
			m.Data[3], m.Data[4])
	}
}

// TestAnExplicitPositionIsNotRewritten: only zero is translated, because only
// zero could not have been meant.
func TestAnExplicitPositionIsNotRewritten(t *testing.T) {
	m := cmd.TextCommand(cmd.TextDisplay{Row: 1, Column: 1, Content: "X"})

	if m.Data[3] != 1 || m.Data[4] != 1 {
		t.Errorf("origin = row %d col %d, want 1/1 unchanged", m.Data[3], m.Data[4])
	}
}

// TestAnOverlongMessageIsTruncatedNotWrapped.
//
// The length is a single octet. A longer message would wrap it and leave the
// device reading the remainder of the text as though it were the next frame,
// which desynchronises every device on the line.
func TestAnOverlongMessageIsTruncatedNotWrapped(t *testing.T) {
	m := cmd.TextCommand(cmd.TextDisplay{Content: strings.Repeat("A", 300)})

	if got := int(m.Data[5]); got != cmd.MaxTextLength {
		t.Errorf("length octet = %d, want %d", got, cmd.MaxTextLength)
	}
	if got := len(m.Data) - 6; got != cmd.MaxTextLength {
		t.Errorf("carried %d characters, want %d; the length must match the payload",
			got, cmd.MaxTextLength)
	}
}

// TestTheLengthOctetAlwaysMatchesThePayload, for every length that matters.
// A disagreement here is a frame a device cannot parse.
func TestTheLengthOctetAlwaysMatchesThePayload(t *testing.T) {
	for _, n := range []int{0, 1, 16, 32, 254, 255, 256} {
		m := cmd.TextCommand(cmd.TextDisplay{Content: strings.Repeat("A", n)})

		declared := int(m.Data[5])
		carried := len(m.Data) - 6
		if declared != carried {
			t.Errorf("%d characters in: length octet says %d, payload carries %d",
				n, declared, carried)
		}
	}
}

// TestTemporaryReportsWhichCodesExpire, which is what a caller checks before
// deciding whether Hold means anything.
func TestTemporaryReportsWhichCodesExpire(t *testing.T) {
	for control, want := range map[cmd.TextControl]bool{
		cmd.TextPermanent:     false,
		cmd.TextPermanentWrap: false,
		cmd.TextTemporary:     true,
		cmd.TextTemporaryWrap: true,
	} {
		if got := control.Temporary(); got != want {
			t.Errorf("%v.Temporary() = %v, want %v", control, got, want)
		}
	}
}
