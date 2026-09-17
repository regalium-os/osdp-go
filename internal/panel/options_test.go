// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package panel_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/regalium-os/osdp-go/internal/bus"
	"github.com/regalium-os/osdp-go/internal/driver"
	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/internal/panel"
)

// countingClock records what the runtime asked of it. Safe for concurrent use
// because Run and the test read it from different goroutines.
type countingClock struct {
	mu     sync.Mutex
	nows   int
	sleeps int
	slept  time.Duration
}

func (c *countingClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nows++
	return time.Now()
}

func (c *countingClock) Sleep(ctx context.Context, d time.Duration) error {
	c.mu.Lock()
	c.sleeps++
	c.slept += d
	c.mu.Unlock()

	if d <= 0 {
		return nil
	}
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *countingClock) counts() (nows, sleeps int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.nows, c.sleeps
}

// TestTheInjectedClockIsTheOnlyOneUsed, and nothing keeps using it after Run
// returns.
//
// The second half is the lifecycle claim in the package doc -- no goroutine
// outlives Run -- checked by its observable consequence rather than by counting
// goroutines, which is a flaky thing to assert.
func TestTheInjectedClockIsTheOnlyOneUsed(t *testing.T) {
	cp, pd := driver.Pipe()
	defer func() { _ = pd.Close() }()

	clock := &countingClock{}
	// A turnaround the runtime must honour: half-duplex lines need it, and a
	// panel that skips it reads back its own transmission.
	line := testLine
	line.Turnaround = time.Millisecond

	p := panel.New(bus.New(line, []frame.Address{0x00}, frame.SchemeCRC16), cp,
		panel.WithClock(clock))
	defer func() { _ = p.Close() }()

	go respond(t, pd, cardReadOnPoll)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()

	waitFor(t, p.Events(), bus.KindOnline)
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitClosed(t, p.Events())

	nows, sleeps := clock.counts()
	if nows == 0 || sleeps == 0 {
		t.Fatalf("clock consulted %d times for Now and %d for Sleep; "+
			"the runtime is reading time from somewhere else", nows, sleeps)
	}

	// Nothing may still be running. Give a stray goroutine a moment to prove
	// it exists before concluding that none does.
	time.Sleep(50 * time.Millisecond)
	if afterNows, afterSleeps := clock.counts(); afterNows != nows || afterSleeps != sleeps {
		t.Errorf("the clock was consulted after Run returned (%d/%d then %d/%d); "+
			"something outlived it", nows, sleeps, afterNows, afterSleeps)
	}
}

// TestASlowConsumerStallsTheLineRatherThanLosingEvents.
//
// This is the backpressure decision made visible. With no buffer at all, an
// event is a rendezvous: the runtime does not move on until the application has
// taken it. The alternative -- dropping -- would mean quietly forgetting that
// somebody badged in, which is not a trade this library makes.
//
// The line stalling must not make the panel unstoppable, so cancellation is
// checked to win over a blocked send.
func TestASlowConsumerStallsTheLineRatherThanLosingEvents(t *testing.T) {
	cp, pd := driver.Pipe()
	defer func() { _ = pd.Close() }()

	p := panel.New(bus.New(testLine, []frame.Address{0x00}, frame.SchemeCRC16), cp,
		panel.WithEventBuffer(0))
	defer func() { _ = p.Close() }()

	go respond(t, pd, cardReadOnPoll)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()

	// Take nothing. The runtime blocks on its first event and stays blocked.
	time.Sleep(100 * time.Millisecond)
	select {
	case err := <-done:
		t.Fatalf("Run returned %v while its event was unconsumed; it dropped it", err)
	default:
	}

	// Cancellation still wins.
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run = %v, want nil for a cancelled context", err)
		}
	case <-time.After(eventWait):
		t.Fatal("a stalled panel did not stop when cancelled")
	}
}
