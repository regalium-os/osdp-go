// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package panel_test

import (
	"context"
	"testing"
	"time"

	"github.com/regalium-os/osdp-go/internal/bus"
	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/internal/transport"
)

// eventWait bounds how long a test waits for the runtime to produce something.
// Generous, because it is a failure bound and not a timing assertion: a slow CI
// runner should not turn a working panel red.
const eventWait = 5 * time.Second

// waitFor consumes events until one of kind arrives, failing if the runtime
// stops or takes too long.
func waitFor(t *testing.T, events <-chan bus.Event, kind bus.Kind) bus.Event {
	t.Helper()

	deadline := time.After(eventWait)
	for {
		select {
		case ev, open := <-events:
			if !open {
				t.Fatalf("the runtime stopped before producing a %v event", kind)
			}
			if ev.Kind == kind {
				return ev
			}
		case <-deadline:
			t.Fatalf("no %v event within %v", kind, eventWait)
		}
	}
}

// waitClosed drains whatever is still buffered and asserts the channel closes.
//
// Draining first is the point: closing a channel does not discard what is
// already in it, so a receive keeps succeeding until the buffer empties. A test
// that checks for closure without draining is really checking that the buffer
// happened to be empty.
func waitClosed(t *testing.T, events <-chan bus.Event) {
	t.Helper()

	deadline := time.After(eventWait)
	for {
		select {
		case _, open := <-events:
			if !open {
				return
			}
		case <-deadline:
			t.Fatalf("the event channel was not closed within %v of Run returning", eventWait)
		}
	}
}

// respond plays a peripheral device: read whatever the panel sent, answer with
// what answer returns, until the port closes.
func respond(t *testing.T, port transport.Port, answer func(frame.Frame) frame.Frame) {
	t.Helper()
	respondThenGoQuiet(t, port, answer, -1)
}

// respondThenGoQuiet answers the first n commands and afterwards only drains
// the line, which is how a reader that locks up behaves: still wired in, still
// receiving, no longer replying. A negative n answers forever.
func respondThenGoQuiet(
	t *testing.T, port transport.Port, answer func(frame.Frame) frame.Frame, n int,
) {
	t.Helper()
	ctx := context.Background()
	buf := make([]byte, 512)

	for answered := 0; ; answered++ {
		if n >= 0 && answered >= n {
			drain(port)
			return
		}
		if err := port.SetReadDeadline(time.Now().Add(eventWait)); err != nil {
			return
		}
		n, err := port.Read(buf)
		if err != nil {
			return // the port closed, which is how this goroutine ends
		}

		command, err := frame.Decode(ctx, buf[:n])
		if err != nil {
			return
		}

		wire, err := answer(command).Append(ctx, nil)
		if err != nil {
			return
		}
		if _, err := port.Write(wire); err != nil {
			return
		}
	}
}

// cardReadOnPoll is a reader that identifies itself, reports its capabilities,
// and then presents a credential to every poll.
func cardReadOnPoll(command frame.Frame) frame.Frame {
	seq := command.Control.Sequence()

	switch cmd.Code(command.Code) {
	case cmd.ID:
		return reply(command.Address, seq, cmd.PDID,
			[]byte{0x00, 0x06, 0x8E, 0x01, 0x02, 0x0A, 0x0B, 0x0C, 0x0D, 0x01, 0x00, 0x03})

	case cmd.Cap:
		// One reader, CRC-16, no Secure Channel: the plainest device there is.
		return reply(command.Address, seq, cmd.PDCap,
			[]byte{0x08, 0x01, 0x00, 0x09, 0x00, 0x00, 0x0D, 0x01, 0x01})

	default:
		// Reader 0, format 1, 26 bits of Wiegand.
		return reply(command.Address, seq, cmd.Raw,
			[]byte{0x00, 0x01, 0x1A, 0x00, 0xAB, 0xCD, 0xEF, 0x80})
	}
}

// reply builds a sealed reply frame from a device.
func reply(addr frame.Address, seq uint8, code cmd.Code, data []byte) frame.Frame {
	f := frame.Frame{
		IsReply: true,
		Address: addr,
		Control: frame.NewControl(seq, frame.SchemeCRC16, false),
		Code:    byte(code),
		Data:    data,
	}
	f.Seal()
	return f
}

// drain plays a device that is on the line but not answering: a reader with a
// cut receive pair, or one that has locked up.
//
// It has to read at all because the in-memory pipe is synchronous -- a write
// blocks until the far end takes it, where a real UART would not. Draining
// keeps the artefact of the transport out of the behaviour under test.
func drain(port transport.Port) {
	buf := make([]byte, 512)
	for {
		if err := port.SetReadDeadline(time.Now().Add(eventWait)); err != nil {
			return
		}
		if _, err := port.Read(buf); err != nil {
			return
		}
	}
}

// respondThenVanish answers n commands and then stops reading the line
// altogether, which is what a converter whose far side has died looks like:
// the panel's next write has nowhere to go.
func respondThenVanish(
	t *testing.T, port transport.Port, answer func(frame.Frame) frame.Frame, n int,
) {
	t.Helper()
	ctx := context.Background()
	buf := make([]byte, 512)

	for answered := 0; answered < n; answered++ {
		if err := port.SetReadDeadline(time.Now().Add(eventWait)); err != nil {
			return
		}
		read, err := port.Read(buf)
		if err != nil {
			return
		}
		command, err := frame.Decode(ctx, buf[:read])
		if err != nil {
			return
		}
		wire, err := answer(command).Append(ctx, nil)
		if err != nil {
			return
		}
		if _, err := port.Write(wire); err != nil {
			return
		}
	}
}
