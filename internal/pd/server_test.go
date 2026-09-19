// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/internal/pd"
)

// The runtime: Handle wired to a port. The port itself is port_test.go.

// runServer runs a server until the line goes quiet, and returns what it said.
//
// The context is cancelled as soon as the inbox drains, so no test waits on
// wall-clock time: the fake port reports ErrTimeout the moment it has nothing
// left, and one idle period later Run notices the cancellation.
func runServer(t *testing.T, d *pd.Device, port *fakePort, opts ...pd.ServerOption) []byte {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	base := []pd.ServerOption{pd.WithIdleTimeout(time.Millisecond)}
	s := pd.NewServer(d, port, append(base, opts...)...)
	defer func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	port.awaitIdle(t)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after its context was cancelled")
	}
	return port.written()
}

// TestTheServerAnswersAPollOnTheLine.
//
// The whole point of the runtime: octets in, octets out, with nothing in the
// test touching Handle directly.
func TestTheServerAnswersAPollOnTheLine(t *testing.T) {
	d := newDevice(t)
	port := newFakePort()
	port.feed(wire(t, poll(t, 1)))

	got := runServer(t, d, port)

	reply, err := frame.Decode(context.Background(), got)
	if err != nil {
		t.Fatalf("the server put something on the line that will not decode: %v", err)
	}
	if !reply.IsReply || reply.Address != testAddress {
		t.Errorf("reply came from %#x, want a reply from %#x", byte(reply.Address), byte(testAddress))
	}
	replyCode(t, reply, cmd.ACK)
}

// TestTheServerSaysNothingToAnotherDevicesCommand.
//
// Silence is the correct answer on a multidrop line, and this is where it
// actually matters: a device that answered would transmit over the reply the
// panel was waiting for.
func TestTheServerSaysNothingToAnotherDevicesCommand(t *testing.T) {
	d := newDevice(t)
	port := newFakePort()
	port.feed(wire(t, commandTo(t, 0x02, 1, cmd.Message{Code: cmd.Poll})))

	if got := runServer(t, d, port); len(got) != 0 {
		t.Errorf("the server transmitted %x in answer to another device's command", got)
	}
}

// TestTheServerResynchronisesPastNoise.
//
// A line shared with a motor delivers rubbish, half a frame from a collision,
// and then a real command. A reader that stopped at the first unparseable octet
// would be a reader that goes offline the first time somebody starts a lift.
func TestTheServerResynchronisesPastNoise(t *testing.T) {
	d := newDevice(t)
	port := newFakePort()
	port.feed([]byte{0x00, 0x11, 0x22, 0xA5, 0x5A})
	port.feed(wire(t, poll(t, 1)))

	got := runServer(t, d, port)
	if len(got) == 0 {
		t.Fatal("the server never answered the command behind the noise")
	}
	reply, err := frame.Decode(context.Background(), got)
	if err != nil {
		t.Fatalf("decoding the reply: %v", err)
	}
	replyCode(t, reply, cmd.ACK)
}

// TestTheServerStaysSilentOnACorruptFrameAndKeepsGoing.
//
// Two commands arrive: one the line mangled, one intact. The device must not
// answer the first -- the address octet of a frame that failed its check is no
// more trustworthy than the rest of it -- and must still answer the second.
func TestTheServerStaysSilentOnACorruptFrameAndKeepsGoing(t *testing.T) {
	d := newDevice(t)

	bad := poll(t, 1)
	bad.Check ^= 0xFFFF

	port := newFakePort()
	port.feed(wire(t, bad))
	port.feed(wire(t, poll(t, 2)))

	got := runServer(t, d, port)

	reply, err := frame.Decode(context.Background(), got)
	if err != nil {
		t.Fatalf("decoding the reply: %v", err)
	}
	if reply.Control.Sequence() != 2 {
		t.Errorf("the server answered sequence %d; the corrupt frame at "+
			"sequence 1 should have drawn silence", reply.Control.Sequence())
	}
}

// TestAPortFailureStopsTheServer, because a cable that came out is not
// something a reader can listen through.
func TestAPortFailureStopsTheServer(t *testing.T) {
	d := newDevice(t)
	s := pd.NewServer(d, &failingPort{}, pd.WithIdleTimeout(time.Millisecond))

	if err := s.Run(context.Background()); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("Run returned %v, want the port's own error", err)
	}
}

// TestTheServerRunsOnce. The buffers and the sequence state it accumulated
// belong to that run.
func TestTheServerRunsOnce(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	s := pd.NewServer(newDevice(t), newFakePort(), pd.WithIdleTimeout(time.Millisecond))
	if err := s.Run(ctx); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if err := s.Run(ctx); !errors.Is(err, pd.ErrAlreadyRun) {
		t.Errorf("second Run returned %v, want ErrAlreadyRun", err)
	}
}

// TestCloseIsIdempotentAndSafeAfterAFailedRun.
func TestCloseIsIdempotentAndSafeAfterAFailedRun(t *testing.T) {
	s := pd.NewServer(newDevice(t), &failingPort{}, pd.WithIdleTimeout(time.Millisecond))
	_ = s.Run(context.Background())

	for i := range 3 {
		if err := s.Close(); err != nil {
			t.Errorf("Close call %d: %v", i+1, err)
		}
	}
}

// TestNewServerStartsNothing: a server can be built, inspected and discarded
// without an octet reaching a line -- the same contract the panel keeps.
func TestNewServerStartsNothing(t *testing.T) {
	port := newFakePort()
	port.feed(wire(t, poll(t, 1)))

	_ = pd.NewServer(newDevice(t), port)

	if got := port.written(); len(got) != 0 {
		t.Errorf("NewServer put %x on the line", got)
	}
	port.mu.Lock()
	defer port.mu.Unlock()
	if port.deadlineSet {
		t.Error("NewServer touched the port's read deadline")
	}
	if !bytes.Equal(port.inbox, wire(t, poll(t, 1))) {
		t.Error("NewServer consumed octets from the line")
	}
}
