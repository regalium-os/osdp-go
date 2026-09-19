// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package bus_test

import (
	"context"
	"testing"
	"time"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
)

// A bus the size of a real one. Everything else in this package tests one or
// two devices, which is enough to prove a rule and not enough to notice a
// device being starved, a sequence number shared between two of them, or a
// queue that only drains for whoever is polled first.

// busSize is a floor plan rather than a round number: 32 readers is a
// mid-sized building, and the protocol allows 126.
const busSize = 32

// TestEveryDeviceOnAFullBusIsPolled.
//
// The starvation check. A cycle that visits the first device twice before
// reaching the last is a door that responds in twice the time nobody measured,
// and with two devices there is no room for the bug to hide in.
func TestEveryDeviceOnAFullBusIsPolled(t *testing.T) {
	ctx := context.Background()
	b := newBus(addresses(busSize)...)

	seen := map[frame.Address]int{}
	for range busSize * 3 {
		step, ok := b.Next(ctx)
		if !ok {
			t.Fatal("the bus produced no step")
		}
		seen[step.Device.Address]++
	}

	for _, addr := range addresses(busSize) {
		if seen[addr] != 3 {
			t.Errorf("address %#x polled %d times in three passes, want 3", byte(addr), seen[addr])
		}
	}
}

// TestSequenceNumbersAreIndependentPerDevice.
//
// Each device runs its own 1,2,3 rotation, and the panel tracks them
// separately. A shared counter would work perfectly on a one-device bench and
// desynchronise every reader on a real line -- and because a device NAKs a
// sequence it did not expect, the symptom would be a whole bus refusing
// commands for no visible reason.
func TestSequenceNumbersAreIndependentPerDevice(t *testing.T) {
	ctx, now := context.Background(), time.Now()
	b := newBus(addresses(busSize)...)

	// Answer only every third device, so the others fall behind: their
	// sequence numbers must not be dragged along by their neighbours'.
	for pass := range 3 {
		for i := range busSize {
			step, _ := b.Next(ctx)
			if i%3 != 0 {
				continue
			}
			if _, err := b.Reply(ctx, step.Device,
				reply(step.Device.Address, step.Frame.Control.Sequence(), cmd.ACK, nil),
				now); err != nil {
				t.Fatalf("pass %d, device %d: %v", pass, i, err)
			}
		}
	}

	// Every sequence in the rotation is 1, 2 or 3 -- never 0, which means a
	// device has lost synchronisation, and never above 3.
	for range busSize {
		step, _ := b.Next(ctx)
		if seq := step.Frame.Control.Sequence(); seq < 1 || seq > 3 {
			t.Fatalf("address %#x is at sequence %d, outside the 1-3 rotation",
				byte(step.Device.Address), seq)
		}
	}
}

// TestACommandReachesOneDeviceOnAFullBus.
//
// A queue is per device, so a command for the reader at the far end must not
// wait behind traffic for the near one -- but it does wait for the cycle to
// reach it, which is the latency the next test measures.
func TestACommandReachesOneDeviceOnAFullBus(t *testing.T) {
	ctx := context.Background()
	b := newBus(addresses(busSize)...)

	last := b.Devices()[busSize-1]
	online(t, b, last)

	if err := b.Send(last, doorRelease()); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Drive a full pass; the release must go to that device and to no other.
	var delivered bool
	for range busSize * 2 {
		step, _ := b.Next(ctx)
		if cmd.Code(step.Frame.Code) != cmd.Out {
			continue
		}
		if step.Device.Address != last.Address {
			t.Fatalf("the release went to %#x, not %#x",
				byte(step.Device.Address), byte(last.Address))
		}
		delivered = true
		break
	}

	if !delivered {
		t.Error("the release never reached the far end of the bus")
	}
}

// TestTheCycleTimeIsWhatTheWireAllows.
//
// Not a behaviour test -- an arithmetic one, recording the number nobody
// computes until a door feels slow.
//
// At 9600 baud with 8N1 framing every octet costs ten bit times: eight data,
// one start, one stop. An exchange is a command and its reply, so a bus of N
// devices takes N times that per pass, and no amount of software makes it
// faster. A panel promising sub-second door response on a large bus at 9600 is
// promising something the wire will not do; osdp_COMSET is the answer, which is
// why this library implements it.
func TestTheCycleTimeIsWhatTheWireAllows(t *testing.T) {
	const (
		bitsPerOctet = 10 // 8N1: eight data bits, one start, one stop
		pollOctets   = 8  // osdp_POLL with a CRC
		ackOctets    = 8  // osdp_ACK with a CRC
	)

	for _, tc := range []struct {
		baud    int
		devices int
	}{
		{9600, busSize},
		{9600, 126}, // the protocol's ceiling
		{115200, busSize},
	} {
		exchange := time.Duration(pollOctets+ackOctets) * bitsPerOctet *
			time.Second / time.Duration(tc.baud)
		cycle := exchange * time.Duration(tc.devices)

		t.Logf("%6d baud, %3d devices: %v per exchange, %v per full pass",
			tc.baud, tc.devices, exchange.Round(time.Microsecond),
			cycle.Round(time.Millisecond))

		// The floor the transport imposes. A ReplyTimeout shorter than one
		// exchange would time out devices that answered perfectly.
		if exchange <= 0 {
			t.Fatal("the arithmetic is wrong")
		}
	}
}

// addresses returns n consecutive device addresses starting at zero.
func addresses(n int) []frame.Address {
	out := make([]frame.Address, 0, n)
	for i := range n {
		out = append(out, frame.Address(i))
	}
	return out
}
