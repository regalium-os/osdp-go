// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd_test

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/internal/pd"
)

// What the application has to say, and how it reaches a panel that never asks
// for it directly.

// TestACardReadIsReportedOnTheNextPoll, and survives the round trip intact.
//
// The bit count is the field worth checking: a 26-bit Wiegand credential, which
// is most of the installed base, occupies four octets of which six bits are
// padding, and a device that let the panel infer the length from the payload
// would hand it six bits of nothing as part of the card number.
func TestACardReadIsReportedOnTheNextPoll(t *testing.T) {
	ctx := context.Background()
	d := newDevice(t)

	want := cmd.CardRead{Reader: 0, Format: 1, BitCount: 26, Data: []byte{0x12, 0x34, 0x56, 0x80}}
	if err := d.ReportCardRead(ctx, want); err != nil {
		t.Fatalf("ReportCardRead: %v", err)
	}

	reply := answer(t, d, poll(t, 1))
	replyCode(t, reply, cmd.Raw)

	got, err := cmd.ParseCardRead(reply.Data)
	if err != nil {
		t.Fatalf("the device's own osdp_RAW would not parse: %v", err)
	}
	if got.Format != want.Format || got.BitCount != want.BitCount || !bytes.Equal(got.Data, want.Data) {
		t.Errorf("card read round-tripped as %+v, want %+v", got, want)
	}

	// And the queue is empty: an event reported twice is a door opened twice.
	replyCode(t, answer(t, d, poll(t, 2)), cmd.ACK)
}

// TestACredentialIsCopiedNotRetained.
//
// The caller should zero its buffer the moment ReportCardRead returns -- a
// credential identifies a person, and the shorter its life in memory the
// better. A device holding the caller's slice would report whatever the caller
// zeroed it to.
func TestACredentialIsCopiedNotRetained(t *testing.T) {
	ctx := context.Background()
	d := newDevice(t)

	credential := []byte{0x12, 0x34, 0x56, 0x80}
	if err := d.ReportCardRead(ctx, cmd.CardRead{Format: 1, BitCount: 26, Data: credential}); err != nil {
		t.Fatalf("ReportCardRead: %v", err)
	}
	for i := range credential {
		credential[i] = 0
	}

	got, err := cmd.ParseCardRead(answer(t, d, poll(t, 1)).Data)
	if err != nil {
		t.Fatalf("osdp_RAW would not parse: %v", err)
	}
	if bytes.Equal(got.Data, credential) {
		t.Error("the reported credential followed the caller's buffer to zero")
	}
}

// TestKeypadEntryIsReportedOnTheNextPoll. SIA OSDP v2.2.2 §6.12.
func TestKeypadEntryIsReportedOnTheNextPoll(t *testing.T) {
	ctx := context.Background()
	d := newDevice(t)

	want := cmd.KeypadEntry{Reader: 0, Keys: []byte{0x01, 0x02, 0x03, 0x04}}
	if err := d.ReportKeypad(ctx, want); err != nil {
		t.Fatalf("ReportKeypad: %v", err)
	}

	reply := answer(t, d, poll(t, 1))
	replyCode(t, reply, cmd.Keypad)

	got, err := cmd.ParseKeypadEntry(reply.Data)
	if err != nil {
		t.Fatalf("the device's own osdp_KEYPAD would not parse: %v", err)
	}
	if !bytes.Equal(got.Keys, want.Keys) {
		t.Errorf("keys round-tripped as %v, want %v", got.Keys, want.Keys)
	}
}

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
