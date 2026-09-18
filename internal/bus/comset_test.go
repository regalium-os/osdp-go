// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package bus_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/regalium-os/osdp-go/internal/bus"
	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
)

// comReply is a device confirming what it adopted.
func comReply(addr frame.Address, seq uint8, adopted cmd.Communication) frame.Frame {
	return reply(addr, seq, cmd.Com, cmd.CommunicationCommand(adopted).Data)
}

// TestADeviceIsMovedToANewAddress.
//
// The command that commissions a bus: readers ship answering to address 0, and
// a line of them has to be given distinct addresses one at a time before it can
// carry traffic at all.
func TestADeviceIsMovedToANewAddress(t *testing.T) {
	ctx := context.Background()
	b := newBus(0x00)
	d := b.Devices()[0]
	online(t, b, d)

	want := cmd.Communication{Address: 0x05, Baud: 9600}
	if err := b.SetCommunication(d, want); err != nil {
		t.Fatalf("SetCommunication: %v", err)
	}

	code, seq := sent(t, b)
	if code != cmd.ComSet {
		t.Fatalf("sent %s, want osdp_COMSET", code.Name(false))
	}

	ev, err := b.Reply(ctx, d, comReply(0x00, seq, want), time.Now())
	if err != nil {
		t.Fatalf("Reply: %v", err)
	}

	if ev.Kind != bus.KindCommunication {
		t.Fatalf("event = %v, want communication", ev.Kind)
	}
	if d.Address != 0x05 {
		t.Errorf("device is at %#x, want 0x05", byte(d.Address))
	}
	if ev.PreviousAddr != 0x00 {
		t.Errorf("previous address = %#x, want 0x00", byte(ev.PreviousAddr))
	}
	if ev.Communication.Baud != 9600 {
		t.Errorf("baud = %d, want 9600", ev.Communication.Baud)
	}
}

// TestTheReplyIsBelievedRatherThanTheRequest.
//
// A device may clamp a baud rate it cannot reach, or take an address other than
// the one asked for. What it says it did is the only version that matters;
// believing the request instead is how a panel talks confidently to nobody.
func TestTheReplyIsBelievedRatherThanTheRequest(t *testing.T) {
	ctx := context.Background()
	b := newBus(0x00)
	d := b.Devices()[0]
	online(t, b, d)

	if err := b.SetCommunication(d, cmd.Communication{Address: 0x05, Baud: 230400}); err != nil {
		t.Fatalf("SetCommunication: %v", err)
	}
	_, seq := sent(t, b)

	// The device took the address but only manages 9600.
	adopted := cmd.Communication{Address: 0x05, Baud: 9600}
	ev, err := b.Reply(ctx, d, comReply(0x00, seq, adopted), time.Now())
	if err != nil {
		t.Fatalf("Reply: %v", err)
	}

	if ev.Communication.Baud != 9600 {
		t.Errorf("reported baud = %d, want the 9600 the device said, not the 230400 asked for",
			ev.Communication.Baud)
	}
}

// TestTheExchangeRestartsAfterAMove.
//
// Everything the panel believed was learned over the old configuration, and the
// device has just thrown its side away. Carrying on with the old sequence and
// capability set would be talking to a device that no longer exists.
func TestTheExchangeRestartsAfterAMove(t *testing.T) {
	ctx := context.Background()
	b := newBus(0x00)
	d := b.Devices()[0]
	online(t, b, d)

	if err := b.SetCommunication(d, cmd.Communication{Address: 0x05, Baud: 9600}); err != nil {
		t.Fatalf("SetCommunication: %v", err)
	}
	_, seq := sent(t, b)
	if _, err := b.Reply(ctx, d, comReply(0x00, seq,
		cmd.Communication{Address: 0x05, Baud: 9600}), time.Now()); err != nil {
		t.Fatalf("Reply: %v", err)
	}

	step, ok := b.Next(ctx)
	if !ok {
		t.Fatal("the bus produced no step")
	}
	if cmd.Code(step.Frame.Code) != cmd.ID {
		t.Errorf("after a move the bus sent %s, want osdp_ID: the device is a stranger again",
			cmd.Code(step.Frame.Code).Name(false))
	}
	if step.Frame.Address != 0x05 {
		t.Errorf("the bus addressed %#x, want the new 0x05", byte(step.Frame.Address))
	}
}

// TestAnAddressAlreadyOnTheLineIsRefused.
//
// Two devices sharing an address is not a state the protocol recovers from:
// both answer every command, their replies collide, and the panel sees
// corruption it cannot attribute to anything. Refusing is the only moment
// anybody can prevent it.
func TestAnAddressAlreadyOnTheLineIsRefused(t *testing.T) {
	b := newBus(0x00, 0x01)
	d := b.Devices()[0]
	online(t, b, d)

	err := b.SetCommunication(d, cmd.Communication{Address: 0x01, Baud: 9600})
	if !errors.Is(err, bus.ErrAddressInUse) {
		t.Fatalf("SetCommunication onto an occupied address = %v, want ErrAddressInUse", err)
	}
	if d.Queued() != 0 {
		t.Error("the command was queued anyway")
	}
}

// TestTheBroadcastAddressIsRefused: a device answering on it would reply to
// every command meant for every other device on the line.
func TestTheBroadcastAddressIsRefused(t *testing.T) {
	b := newBus(0x00)
	d := b.Devices()[0]
	online(t, b, d)

	for _, addr := range []byte{0x7F, 0x80, 0xFF} {
		err := b.SetCommunication(d, cmd.Communication{Address: addr, Baud: 9600})
		if !errors.Is(err, bus.ErrInvalidAddress) {
			t.Errorf("address %#x = %v, want ErrInvalidAddress", addr, err)
		}
	}
}

// TestMovingADeviceToItsOwnAddressIsAllowed, which is how a baud change alone
// is expressed.
func TestMovingADeviceToItsOwnAddressIsAllowed(t *testing.T) {
	b := newBus(0x00, 0x01)
	d := b.Devices()[0]
	online(t, b, d)

	if err := b.SetCommunication(d, cmd.Communication{Address: 0x00, Baud: 19200}); err != nil {
		t.Errorf("changing only the baud rate was refused: %v", err)
	}
}
