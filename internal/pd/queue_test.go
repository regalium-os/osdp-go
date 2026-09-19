// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/internal/pd"
)

// The queue events wait in: how full is full, and what happens when the
// application and the poll loop reach for it at once. What a reader has to say
// is report_test.go.

// TestAFullQueueIsReportedRatherThanSwallowed.
//
// The alternative is a credential presented at a reader and silently forgotten,
// which at the panel is indistinguishable from a card that was never presented.
func TestAFullQueueIsReportedRatherThanSwallowed(t *testing.T) {
	ctx := context.Background()
	d := newDevice(t, pd.WithEventQueue(2))

	card := cmd.CardRead{BitCount: 8, Data: []byte{0x01}}
	for range 2 {
		if err := d.ReportCardRead(ctx, card); err != nil {
			t.Fatalf("ReportCardRead: %v", err)
		}
	}

	if err := d.ReportCardRead(ctx, card); !errors.Is(err, pd.ErrQueueFull) {
		t.Errorf("a third card read returned %v, want ErrQueueFull", err)
	}
	if got := d.Pending(); got != 2 {
		t.Errorf("%d events queued, want 2", got)
	}
}

// TestReportingConcurrentlyWithPolling.
//
// The claim on Device is that it is safe for concurrent use, and it is not a
// convenience: the goroutine that notices a card at the reader and the one
// answering the panel's poll are genuinely different goroutines, and the second
// must not wait for the first. Under -race this is the test that says so.
func TestReportingConcurrentlyWithPolling(t *testing.T) {
	const rounds = 200

	ctx := context.Background()
	d := newDevice(t, pd.WithContacts(2, 0, 1), pd.WithEventQueue(4))

	// The poll frames are built on the test's own goroutine: the helpers call
	// t.Fatalf, and that is only legal from the goroutine running the test.
	polls := [3]frame.Frame{poll(t, 1), poll(t, 2), poll(t, 3)}

	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		for i := range rounds {
			// A full queue is an expected outcome here, not a failure: the
			// poller is draining one event per poll and the reader is faster.
			_ = d.ReportCardRead(ctx, cmd.CardRead{BitCount: 8, Data: []byte{byte(i)}})
		}
	}()
	go func() {
		defer wg.Done()
		for i := range rounds {
			_ = d.SetInput(ctx, i%2, i%3 == 0)
		}
	}()
	go func() {
		defer wg.Done()
		for i := range rounds {
			if _, err := d.Handle(ctx, polls[i%3]); err != nil {
				t.Errorf("Handle: %v", err)
				return
			}
		}
	}()

	wg.Wait()
}
