// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package bus_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/regalium-os/osdp-go/internal/bus"
	"github.com/regalium-os/osdp-go/internal/cmd"
)

// onlineWithBuffer brings a device up having reported the given receive buffer.
func onlineWithBuffer(t *testing.T, b *bus.Bus, d *bus.Device, size int) {
	t.Helper()
	ctx, now := context.Background(), time.Now()

	b.Next(ctx)
	if _, err := b.Reply(ctx, d, reply(d.Address, 1, cmd.PDID, deviceIDPayload), now); err != nil {
		t.Fatalf("osdp_ID: %v", err)
	}

	b.Next(ctx)
	caps := []byte{
		0x0D, 0x01, 0x01, // one reader
		0x0A, byte(size), byte(size >> 8), // receive buffer, least significant first
	}
	if _, err := b.Reply(ctx, d, reply(d.Address, 2, cmd.PDCap, caps), now); err != nil {
		t.Fatalf("osdp_CAP: %v", err)
	}
}

// TestACommandTooLargeForTheDeviceIsRefused.
//
// A device cannot reply to a frame it could not receive. The panel sees a
// silence, retries with the same oversized frame, and the command sits at the
// head of that device's queue forever -- a reader that answers polls and
// nothing else. Refusing at the call site makes it an error at the moment the
// mistake is made.
func TestACommandTooLargeForTheDeviceIsRefused(t *testing.T) {
	b := newBus(0x00)
	d := b.Devices()[0]
	onlineWithBuffer(t, b, d, 64)

	long := cmd.TextCommand(cmd.TextDisplay{Content: strings.Repeat("A", 200)})

	err := b.Send(d, long)
	if !errors.Is(err, bus.ErrMessageTooLarge) {
		t.Fatalf("Send = %v, want ErrMessageTooLarge", err)
	}
	if d.Queued() != 0 {
		t.Error("the oversized command was queued anyway")
	}

	// The error has to say by how much, or a caller goes to the specification
	// to find out.
	if msg := err.Error(); !strings.Contains(msg, "64") || !strings.Contains(msg, "osdp_TEXT") {
		t.Errorf("error = %q, want it to name the command and the limit", msg)
	}
}

// TestACommandThatFitsIsAccepted, including one that exactly fills the buffer.
func TestACommandThatFitsIsAccepted(t *testing.T) {
	b := newBus(0x00)
	d := b.Devices()[0]
	onlineWithBuffer(t, b, d, 64)

	if err := b.Send(d, cmd.OutputCommand(cmd.Output{Number: 0})); err != nil {
		t.Fatalf("a four-octet command was refused: %v", err)
	}
	if d.Queued() != 1 {
		t.Error("the command was not queued")
	}
}

// TestADeviceThatReportedNoBufferIsTakenAtItsWord.
//
// A limit nobody stated is not a limit this package may invent. Plenty of
// devices omit the capability, and refusing their traffic on a guess would
// break buses that work.
func TestADeviceThatReportedNoBufferIsTakenAtItsWord(t *testing.T) {
	b := newBus(0x00)
	d := b.Devices()[0]
	online(t, b, d) // capabilities with no osdp_CAP_RECEIVE_BUFFERSIZE

	long := cmd.TextCommand(cmd.TextDisplay{Content: strings.Repeat("A", 200)})
	if err := b.Send(d, long); err != nil {
		t.Errorf("Send = %v; a device that stated no limit must not be given one", err)
	}
}

// TestTheEstimateMatchesTheFrameActuallyBuilt.
//
// The check computes a length before any frame exists, which is the point --
// it refuses before building one. That arithmetic has to agree with what
// compose really produces, and the two are in different files with no compiler
// keeping them honest. This is what keeps them honest.
func TestTheEstimateMatchesTheFrameActuallyBuilt(t *testing.T) {
	ctx := context.Background()

	for _, payload := range []int{0, 1, 4, 14, 32, 64} {
		b := newBus(0x00)
		d := b.Devices()[0]

		// A buffer of exactly the estimate must be accepted, and one octet
		// less refused -- which pins the estimate from both sides.
		onlineWithBuffer(t, b, d, 0xFFFF)
		msg := cmd.TextCommand(cmd.TextDisplay{Content: strings.Repeat("A", payload)})
		if err := b.Send(d, msg); err != nil {
			t.Fatalf("payload %d: %v", payload, err)
		}

		step, ok := b.Next(ctx)
		if !ok {
			t.Fatalf("payload %d: no step", payload)
		}
		wire, err := step.Frame.Append(ctx, nil)
		if err != nil {
			t.Fatalf("payload %d: Append: %v", payload, err)
		}

		tight := newBus(0x00)
		td := tight.Devices()[0]
		onlineWithBuffer(t, tight, td, len(wire))
		if err := tight.Send(td, msg); err != nil {
			t.Errorf("payload %d: a buffer of exactly %d octets refused the frame: %v",
				payload, len(wire), err)
		}

		short := newBus(0x00)
		sd := short.Devices()[0]
		onlineWithBuffer(t, short, sd, len(wire)-1)
		if err := short.Send(sd, msg); !errors.Is(err, bus.ErrMessageTooLarge) {
			t.Errorf("payload %d: a buffer one octet short accepted the frame (%v)",
				payload, err)
		}
	}
}

// TestTheEstimateAccountsForTheSecureChannel.
//
// A secure frame is bigger than its payload by a security block, the padding to
// the cipher's block boundary, and the message authentication code -- and the
// padding is the awkward one, because a payload that is already block aligned
// grows by a whole block rather than none. Underestimating here would let an
// oversized frame through on exactly the buses that can least afford it.
func TestTheEstimateAccountsForTheSecureChannel(t *testing.T) {
	ctx := context.Background()

	for _, payload := range []int{0, 1, 15, 16, 17, 32} {
		b := secureBus(siteKey, true)
		d := b.Devices()[0]
		pd := newPeripheral(siteKey)
		establish(t, b, d, pd)

		msg := cmd.TextCommand(cmd.TextDisplay{Content: strings.Repeat("A", payload)})
		if err := b.Send(d, msg); err != nil {
			t.Fatalf("payload %d: %v", payload, err)
		}

		step, ok := b.Next(ctx)
		if !ok {
			t.Fatalf("payload %d: no step", payload)
		}
		wire, err := step.Frame.Append(ctx, nil)
		if err != nil {
			t.Fatalf("payload %d: Append: %v", payload, err)
		}

		// Rebuild with the device reporting exactly that buffer, and one less.
		for _, tc := range []struct {
			buffer int
			refuse bool
		}{{len(wire), false}, {len(wire) - 1, true}} {
			fresh := secureBus(siteKey, true)
			fd := fresh.Devices()[0]
			establishWithBuffer(t, fresh, fd, newPeripheral(siteKey), tc.buffer)

			err := fresh.Send(fd, msg)
			if tc.refuse && !errors.Is(err, bus.ErrMessageTooLarge) {
				t.Errorf("payload %d: buffer %d accepted a %d-octet frame (%v)",
					payload, tc.buffer, len(wire), err)
			}
			if !tc.refuse && err != nil {
				t.Errorf("payload %d: buffer %d refused a %d-octet frame: %v",
					payload, tc.buffer, len(wire), err)
			}
		}
	}
}
