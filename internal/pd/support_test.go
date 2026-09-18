// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/internal/pd"
)

// testAddress is the address every device in these tests answers to. It is not
// zero, so a test that forgets to set an address fails rather than passing by
// coincidence.
const testAddress frame.Address = 0x01

// newDevice builds a device, failing the test rather than returning an error.
func newDevice(t *testing.T, opts ...pd.Option) *pd.Device {
	t.Helper()

	d, err := pd.New(testAddress, opts...)
	if err != nil {
		t.Fatalf("pd.New: %v", err)
	}
	return d
}

// command builds a panel command for testAddress at seq.
func command(t *testing.T, seq uint8, m cmd.Message) frame.Frame {
	t.Helper()
	return commandTo(t, testAddress, seq, m)
}

// poll is the command a panel sends most of the time.
func poll(t *testing.T, seq uint8) frame.Frame {
	t.Helper()
	return command(t, seq, cmd.Message{Code: cmd.Poll})
}

// commandTo builds a command and puts it through the wire in both directions.
//
// The round trip is the point. Handing Handle a frame this process composed
// would test the device against a frame no line ever carried: the error check
// would be one this package computed for itself, and the payload would be a
// slice the test still owns rather than one aliasing a read buffer. Both are
// exactly the conditions the ownership rules are written for.
func commandTo(t *testing.T, addr frame.Address, seq uint8, m cmd.Message) frame.Frame {
	t.Helper()

	ctx := context.Background()
	f, err := cmd.Encode(ctx, m, addr, seq, frame.SchemeCRC16)
	if err != nil {
		t.Fatalf("encoding %s: %v", m.Name(), err)
	}

	buf, err := f.Append(ctx, nil)
	if err != nil {
		t.Fatalf("appending %s: %v", m.Name(), err)
	}

	decoded, err := frame.Decode(ctx, buf)
	if err != nil {
		t.Fatalf("decoding %s: %v", m.Name(), err)
	}
	return decoded
}

// answer runs one exchange and asserts the device produced a usable reply.
func answer(t *testing.T, d *pd.Device, f frame.Frame) frame.Frame {
	t.Helper()

	reply, err := d.Handle(context.Background(), f)
	if err != nil {
		t.Fatalf("Handle(%s): %v", cmd.Code(f.Code).Name(false), err)
	}
	if !reply.IsReply {
		t.Error("the device answered with a command frame, not a reply")
	}
	if !reply.CheckOK() {
		t.Error("the device answered with a frame whose error check does not match")
	}
	if got, want := reply.Control.Sequence(), f.Control.Sequence(); got != want {
		t.Errorf("reply carried sequence %d, want %d: SIA OSDP v2.2.2 §5.7 "+
			"answers a command at the number it arrived with", got, want)
	}
	return reply
}

// wire is the octets a frame becomes, for the byte-exact comparisons that make
// "the same reply" mean the same reply rather than an equivalent one.
func wire(t *testing.T, f frame.Frame) []byte {
	t.Helper()

	buf, err := f.Append(context.Background(), nil)
	if err != nil {
		t.Fatalf("encoding a reply: %v", err)
	}
	return buf
}

// replyCode asserts which reply came back, naming both sides in the failure so
// a table test says what it meant rather than printing two numbers.
func replyCode(t *testing.T, reply frame.Frame, want cmd.Code) {
	t.Helper()

	if got := cmd.Code(reply.Code); got != want {
		t.Errorf("device answered %s, want %s", got.Name(true), want.Name(true))
	}
}

// fakeClock is time under the test's control. Nothing in this package sleeps,
// so a communication-timeout scenario is a table entry rather than a pause.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

// newClock starts at a fixed instant rather than at the zero time, so a test
// cannot pass because "never heard from" and "heard from at the epoch" happen
// to compare equal.
func newClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

// Now implements pd.Clock.
func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// advance moves the clock forward.
func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}
