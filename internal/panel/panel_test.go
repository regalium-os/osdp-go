// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package panel_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/regalium-os/osdp-go/internal/bus"
	"github.com/regalium-os/osdp-go/internal/driver"
	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/internal/panel"
	"github.com/regalium-os/osdp-go/internal/transport"
)

// testLine keeps the timeouts short: these tests exercise the lifecycle, and a
// suite that waits a second per silent device is a suite people stop running.
var testLine = transport.Line{Name: "test", Baud: 9600, ReplyTimeout: 50 * time.Millisecond}

// newPanel returns a panel driving addrs over one end of an in-memory pipe,
// with the device end handed back for a test to play the reader.
func newPanel(t *testing.T, addrs ...frame.Address) (*panel.Panel, transport.Port) {
	t.Helper()

	cp, pd := driver.Pipe()
	t.Cleanup(func() { _ = pd.Close() })

	p := panel.New(bus.New(testLine, addrs, frame.SchemeCRC16), cp)
	t.Cleanup(func() { _ = p.Close() })
	return p, pd
}

// TestNewStartsNothing is the lifecycle rule that matters most: a constructor
// that started a goroutine would make a panel impossible to build, inspect and
// discard, and would put traffic on a line before the caller asked for any.
func TestNewStartsNothing(t *testing.T) {
	_, device := newPanel(t, 0x00)

	// Nothing has run, so nothing can have been written. A real command would
	// arrive well inside this window; the pipe has no latency to speak of.
	if err := device.SetReadDeadline(time.Now().Add(50 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	if n, err := device.Read(make([]byte, 64)); !errors.Is(err, transport.ErrTimeout) {
		t.Errorf("New put %d octets on the line (err %v); it must start nothing", n, err)
	}
}

// TestRunPollsAndPublishes drives a whole enrolment through the runtime: the
// panel asks, a device answers, and the application receives events without
// touching a frame.
func TestRunPollsAndPublishes(t *testing.T) {
	p, device := newPanel(t, 0x00)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go respond(t, device, cardReadOnPoll)

	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()

	card := waitFor(t, p.Events(), bus.KindCardRead)
	if card.Card.BitCount != 26 {
		t.Errorf("bit count = %d, want 26", card.Card.BitCount)
	}

	cancel()
	if err := <-done; err != nil {
		t.Errorf("Run returned %v; a cancelled context is a clean stop", err)
	}
	waitClosed(t, p.Events())
}

// TestSilentDeviceGoesOfflineRatherThanFailing: on a bus of hundreds of
// readers, one not answering is information, not a reason to stop polling the
// other ninety-nine.
func TestSilentDeviceGoesOfflineRatherThanFailing(t *testing.T) {
	p, device := newPanel(t, 0x00)

	// A device only goes offline if it was online first: it answers osdp_ID
	// and osdp_CAP, and then stops.
	go respondThenGoQuiet(t, device, cardReadOnPoll, 2)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()

	if online := waitFor(t, p.Events(), bus.KindOnline); online.Device.Address != 0x00 {
		t.Fatalf("online event from 0x%02X, want 0x00", byte(online.Device.Address))
	}
	waitFor(t, p.Events(), bus.KindOffline)

	cancel()
	if err := <-done; err != nil {
		t.Errorf("Run returned %v; a device going quiet is not a failure", err)
	}
}

// TestRunRefusesToStartTwice: two drivers on one line would interleave commands
// mid-exchange and desynchronise every device on it.
func TestRunRefusesToStartTwice(t *testing.T) {
	p, device := newPanel(t, 0x00)
	go respond(t, device, cardReadOnPoll)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()
	waitFor(t, p.Events(), bus.KindOnline) // it is definitely running now

	if err := p.Run(context.Background()); !errors.Is(err, panel.ErrAlreadyRun) {
		t.Errorf("second Run = %v, want ErrAlreadyRun", err)
	}

	cancel()
	<-done
}

// TestRunWithoutDevicesIsAConfigurationError: a bus with no addresses is a
// mistake in the caller's setup, not a condition of the line, so it comes back
// as an error rather than as an event nobody is listening for yet.
func TestRunWithoutDevicesIsAConfigurationError(t *testing.T) {
	p, _ := newPanel(t)

	if err := p.Run(context.Background()); !errors.Is(err, panel.ErrNoDevices) {
		t.Errorf("Run = %v, want ErrNoDevices", err)
	}
	if _, open := <-p.Events(); open {
		t.Error("the event channel was left open after a failed Run")
	}
}

// TestCloseIsIdempotentAndSafeAfterAFailedRun.
func TestCloseIsIdempotentAndSafeAfterAFailedRun(t *testing.T) {
	p, _ := newPanel(t)

	if err := p.Run(context.Background()); !errors.Is(err, panel.ErrNoDevices) {
		t.Fatalf("Run = %v, want ErrNoDevices", err)
	}
	if err := p.Close(); err != nil {
		t.Errorf("first Close after a failed Run: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Errorf("second Close: %v; Close must be idempotent", err)
	}
}

// TestAWedgedWriteDoesNotOutlastTheContext.
//
// A port whose far end has stopped reading blocks a write indefinitely, and no
// context can interrupt a blocked syscall. Without a bound on the write, Run
// simply never returns -- which is how a panel becomes unkillable. This is the
// regression test for exactly that: the runtime must still stop, within roughly
// the time a device gets to answer.
func TestAWedgedWriteDoesNotOutlastTheContext(t *testing.T) {
	p, device := newPanel(t, 0x00)

	// The device answers enrolment and then stops reading entirely.
	go respondThenVanish(t, device, cardReadOnPoll, 2)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()

	waitFor(t, p.Events(), bus.KindOnline)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run = %v; cancellation is a clean stop even mid-write", err)
		}
	case <-time.After(eventWait):
		t.Fatal("Run never returned: a wedged write outlived the context")
	}
}
