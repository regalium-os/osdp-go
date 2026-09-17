// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package cmd_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/regalium-os/osdp-go/internal/cmd"
)

// TestOutputCommandLayout checks the wire layout against SIA OSDP v2.2.2 §6.9:
// output number, control code, then a little-endian timer in units of 100ms.
func TestOutputCommandLayout(t *testing.T) {
	m := cmd.OutputCommand(cmd.Output{
		Number:  1,
		Control: cmd.OutputTimedOn,
		Timer:   3 * time.Second, // 30 ticks
	})

	if m.Code != cmd.Out {
		t.Errorf("code = 0x%02X, want osdp_OUT", byte(m.Code))
	}
	if want := []byte{0x01, 0x05, 0x1E, 0x00}; !bytes.Equal(m.Data, want) {
		t.Errorf("payload = % X, want % X", m.Data, want)
	}
}

// TestOutputCommandCarriesEveryPoint: a device applies a multi-output command
// together, which is how a door and the light beside it change in one cycle.
func TestOutputCommandCarriesEveryPoint(t *testing.T) {
	m := cmd.OutputCommand(
		cmd.Output{Number: 0, Control: cmd.OutputTimedOn, Timer: time.Second},
		cmd.Output{Number: 1, Control: cmd.OutputOffAbort},
	)

	if len(m.Data) != 8 {
		t.Fatalf("payload is %d octets, want 8 for two outputs", len(m.Data))
	}
	if m.Data[4] != 0x01 || m.Data[5] != byte(cmd.OutputOffAbort) {
		t.Errorf("second entry = % X, want output 1 set off", m.Data[4:8])
	}
}

// TestATimerTooLongToExpressSaturates.
//
// A wrapped timer is a door that relocks in a fraction of a second instead of
// staying open for the hour somebody asked for, and it would do so silently.
// Saturating is wrong in a way an operator can see; wrapping is wrong in a way
// they cannot.
func TestATimerTooLongToExpressSaturates(t *testing.T) {
	m := cmd.OutputCommand(cmd.Output{
		Number:  0,
		Control: cmd.OutputTimedOn,
		Timer:   24 * time.Hour,
	})

	if got := int(m.Data[2]) | int(m.Data[3])<<8; got != 0xFFFF {
		t.Errorf("timer = %d ticks, want the field's maximum 65535", got)
	}
}

// TestLEDCommandIsFourteenOctets, the width the Phase 1 fixture corpus decodes
// byte for byte in its led-command record.
func TestLEDCommandIsFourteenOctets(t *testing.T) {
	m := cmd.LEDCommand(0, 0,
		cmd.LEDState{Control: cmd.LEDSet, OnColour: cmd.ColourGreen, OffColour: cmd.ColourGreen},
		3*time.Second,
		cmd.LEDState{Control: cmd.LEDSetPermanent, OnColour: cmd.ColourRed, OffColour: cmd.ColourRed},
	)

	if m.Code != cmd.LED {
		t.Errorf("code = 0x%02X, want osdp_LED", byte(m.Code))
	}
	if len(m.Data) != cmd.LEDBlockSize {
		t.Fatalf("payload is %d octets, want %d", len(m.Data), cmd.LEDBlockSize)
	}

	// The temporary timer occupies octets 7 and 8, little-endian.
	if got := int(m.Data[7]) | int(m.Data[8])<<8; got != 30 {
		t.Errorf("temporary timer = %d ticks, want 30", got)
	}
	// And the permanent half follows, starting at octet 9.
	if m.Data[9] != byte(cmd.LEDSetPermanent) || m.Data[12] != byte(cmd.ColourRed) {
		t.Errorf("permanent half = % X, want set to red", m.Data[9:])
	}
}

// TestTheTwoLEDHalvesDoNotShareACodeTable is the mistake worth a test: the
// permanent half has no cancel, so its set code is 0x01 where the temporary
// half's is 0x02. Using the wrong one produces an LED that ignores the command
// rather than an error anybody would see.
func TestTheTwoLEDHalvesDoNotShareACodeTable(t *testing.T) {
	if cmd.LEDSetPermanent == cmd.LEDSet {
		t.Fatal("the two set codes are equal; one of them is wrong")
	}
	if cmd.LEDSetPermanent != 0x01 || cmd.LEDSet != 0x02 {
		t.Errorf("set codes are permanent=0x%02X temporary=0x%02X, want 0x01 and 0x02",
			byte(cmd.LEDSetPermanent), byte(cmd.LEDSet))
	}
}

// TestBuzzerCommandLayout, SIA OSDP v2.2.2 §6.11: reader, tone, on, off, count.
func TestBuzzerCommandLayout(t *testing.T) {
	m := cmd.BuzzerCommand(0, cmd.ToneDefault, 200*time.Millisecond, 300*time.Millisecond, 3)

	if m.Code != cmd.Buz {
		t.Errorf("code = 0x%02X, want osdp_BUZ", byte(m.Code))
	}
	if want := []byte{0x00, 0x02, 0x02, 0x03, 0x03}; !bytes.Equal(m.Data, want) {
		t.Errorf("payload = % X, want % X", m.Data, want)
	}
}

// TestSubTickDurationsRoundDownRatherThanToZeroSurprises: the protocol's unit
// is 100ms and anything finer cannot be expressed, so a caller asking for 50ms
// gets zero -- which for a blink time means steady, not instant.
func TestSubTickDurationsRoundDown(t *testing.T) {
	m := cmd.BuzzerCommand(0, cmd.ToneDefault, 50*time.Millisecond, 0, 1)
	if m.Data[2] != 0 {
		t.Errorf("on time = %d ticks, want 0: 50ms is below the protocol's unit", m.Data[2])
	}
}
