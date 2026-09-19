// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd_test

import (
	"testing"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
)

// osdp_COMSET: moving a device onto its own address. SIA OSDP v2.2.2 §6.14.

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
