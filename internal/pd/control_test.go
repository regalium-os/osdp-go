// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd_test

import (
	"testing"
	"time"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/internal/pd"
)

// The two commands that change the device rather than merely ask it something.

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

// TestComsetAnswersAtTheOldAddressAndThenMoves. SIA OSDP v2.2.2 §6.14.
//
// The order is the whole of it. A device that moved before replying would
// answer from an address the panel is not listening to, and the commissioning
// would look like a device that stopped responding the instant it was
// configured -- which is indistinguishable from one that was misconfigured.
func TestComsetAnswersAtTheOldAddressAndThenMoves(t *testing.T) {
	const moved frame.Address = 0x05

	d := newDevice(t)
	change := cmd.CommunicationCommand(cmd.Communication{Address: byte(moved), Baud: 38400})

	reply := answer(t, d, command(t, 1, change))
	replyCode(t, reply, cmd.Com)
	if reply.Address != testAddress {
		t.Errorf("confirmed from address %#x, want the old %#x", byte(reply.Address), byte(testAddress))
	}

	got, err := cmd.ParseCommunication(reply.Data)
	if err != nil {
		t.Fatalf("osdp_COM would not parse: %v", err)
	}
	if got.Address != byte(moved) || got.Baud != 38400 {
		t.Errorf("confirmed %+v, want address %#x at 38400", got, byte(moved))
	}

	if d.Address() != moved {
		t.Fatalf("device is on address %#x, want %#x", byte(d.Address()), byte(moved))
	}
	replyCode(t, answer(t, d, commandTo(t, moved, 2, cmd.Message{Code: cmd.Poll})), cmd.ACK)
}

// TestComsetRefusesAnAddressNoDeviceCanHold, rather than moving to one nothing
// can reach. The configuration address is the case that matters: a device that
// took it would answer every command meant for every other device on the line.
func TestComsetRefusesAnAddressNoDeviceCanHold(t *testing.T) {
	d := newDevice(t)

	bad := cmd.Message{Code: cmd.ComSet, Data: []byte{byte(frame.BroadcastAddress), 0x80, 0x25, 0x00, 0x00}}
	reply := answer(t, d, command(t, 1, bad))
	replyCode(t, reply, cmd.NAK)
	expectNAK(cmd.NAKUnsupportedInput)(t, reply.Data)

	if d.Address() != testAddress {
		t.Errorf("device moved to %#x on a refused osdp_COMSET", byte(d.Address()))
	}
}

// TestTheReplyUsesTheCommandsCheckScheme.
//
// A panel speaking the checksum cannot verify a CRC-16 reply, and will retry
// what it reads as a corrupt frame until it gives up on the device.
func TestTheReplyUsesTheCommandsCheckScheme(t *testing.T) {
	for _, scheme := range []frame.Scheme{frame.SchemeCRC16, frame.SchemeChecksum} {
		t.Run(scheme.String(), func(t *testing.T) {
			d := newDevice(t)

			sent := frame.Frame{
				Address: testAddress,
				Control: frame.NewControl(1, scheme, false),
				Code:    byte(cmd.Poll),
			}
			sent.Seal()

			reply := answer(t, d, sent)
			if got := reply.Control.Scheme(); got != scheme {
				t.Errorf("answered a %s command with %s", scheme, got)
			}
		})
	}
}
