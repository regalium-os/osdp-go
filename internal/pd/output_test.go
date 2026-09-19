// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd_test

import (
	"testing"
	"time"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/pd"
)

// osdp_OUT: the command that opens a door, and the one the retransmission
// rules are written for. Commissioning is comset_test.go.

// TestAnOutputCommandChangesTheReportedState.
//
// osdp_OUT is the command that opens a door, and osdp_OSTAT is how a panel
// confirms what the device did with it -- which is the only confirmation there
// is, an osdp_ACK saying only that the command arrived.
func TestAnOutputCommandChangesTheReportedState(t *testing.T) {
	d := newDevice(t, pd.WithContacts(0, 2, 1))

	release := cmd.OutputCommand(cmd.Output{
		Number:  1,
		Control: cmd.OutputTimedOn,
		Timer:   5 * time.Second,
	})
	replyCode(t, answer(t, d, command(t, 1, release)), cmd.ACK)

	changes, err := cmd.ParseOutputStatus(answer(t, d, command(t, 2, cmd.OutputStatusCommand())).Data)
	if err != nil {
		t.Fatalf("osdp_OSTATR would not parse: %v", err)
	}
	if len(changes) != 2 || changes[0].Active || !changes[1].Active {
		t.Errorf("output report was %+v, want only output 1 active", changes)
	}
}

// TestAMalformedOutputCommandIsRefused.
//
// Length and range are checked before anything is applied. A command naming an
// output this device does not have is refused whole, because half-applying it
// would acknowledge work that was not done -- and the panel would believe the
// door had been released.
func TestAMalformedOutputCommandIsRefused(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want cmd.NAKReason
	}{
		{"an empty payload", nil, cmd.NAKCommandLength},
		{"a truncated entry", []byte{0x00, 0x05, 0x00}, cmd.NAKCommandLength},
		{"an output this device does not have", []byte{0x07, 0x05, 0x00, 0x00}, cmd.NAKUnsupportedInput},
		{"a control code the specification does not define", []byte{0x00, 0x7F, 0x00, 0x00}, cmd.NAKUnsupportedInput},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := newDevice(t, pd.WithContacts(0, 2, 1))

			reply := answer(t, d, command(t, 1, cmd.Message{Code: cmd.Out, Data: tc.data}))
			replyCode(t, reply, cmd.NAK)
			expectNAK(tc.want)(t, reply.Data)

			// Nothing was applied: the outputs still read as they were.
			changes, err := cmd.ParseOutputStatus(answer(t, d, command(t, 2, cmd.OutputStatusCommand())).Data)
			if err != nil {
				t.Fatalf("osdp_OSTATR would not parse: %v", err)
			}
			for _, c := range changes {
				if c.Active {
					t.Errorf("output %d was energised by a refused command", c.Index)
				}
			}
		})
	}
}

// TestARefusedOutputCommandAppliesNoneOfIt.
//
// The single-entry cases above cannot tell a command refused whole from one
// refused after it had already done half its work, because there is no half.
// This one can: the first entry is perfectly good and the second is not, and a
// device that walks the list applying as it goes energises a strike and then
// tells the panel it did nothing.
//
// That is the worse of the two failures. A panel that receives osdp_NAK will
// retry or report; what it will not do is send the off command for a door it
// has been told was never opened.
func TestARefusedOutputCommandAppliesNoneOfIt(t *testing.T) {
	d := newDevice(t, pd.WithContacts(0, 2, 1))

	// Output 0 released, then an entry whose control code the specification
	// does not define.
	both := []byte{
		0x00, byte(cmd.OutputTimedOn), 0x00, 0x00,
		0x01, 0x7F, 0x00, 0x00,
	}
	reply := answer(t, d, command(t, 1, cmd.Message{Code: cmd.Out, Data: both}))
	replyCode(t, reply, cmd.NAK)
	expectNAK(cmd.NAKUnsupportedInput)(t, reply.Data)

	changes, err := cmd.ParseOutputStatus(answer(t, d, command(t, 2, cmd.OutputStatusCommand())).Data)
	if err != nil {
		t.Fatalf("osdp_OSTATR would not parse: %v", err)
	}
	for _, c := range changes {
		if c.Active {
			t.Errorf("output %d was energised by a command the device refused; "+
				"the panel believes that door never opened", c.Index)
		}
	}
}

// TestSeveralOutputsAreAppliedTogether.
//
// Sending several points in one command is not an efficiency: a device applies
// them in one operation, so a panel unlocking a door and lighting the strike
// indicator does both in the same poll cycle rather than in two. That is the
// difference between a door and its light agreeing, and it is the behaviour the
// all-or-nothing validation above must not have cost.
func TestSeveralOutputsAreAppliedTogether(t *testing.T) {
	d := newDevice(t, pd.WithContacts(0, 3, 1))

	both := cmd.OutputCommand(
		cmd.Output{Number: 0, Control: cmd.OutputTimedOn, Timer: time.Second},
		cmd.Output{Number: 2, Control: cmd.OutputOnAbort},
	)
	replyCode(t, answer(t, d, command(t, 1, both)), cmd.ACK)

	changes, err := cmd.ParseOutputStatus(answer(t, d, command(t, 2, cmd.OutputStatusCommand())).Data)
	if err != nil {
		t.Fatalf("osdp_OSTATR would not parse: %v", err)
	}
	want := []bool{true, false, true}
	for i, c := range changes {
		if c.Active != want[i] {
			t.Errorf("output %d is %v, want %v", i, c.Active, want[i])
		}
	}
}
