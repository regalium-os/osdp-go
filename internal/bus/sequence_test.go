// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package bus_test

import (
	"context"
	"testing"
	"time"

	"github.com/regalium-os/osdp-go/internal/cmd"
)

// The sequence number under retransmission. SIA OSDP v2.2.2 sec.5.7: a device
// caches the reply it gave to each sequence number, so repeating the number is
// what distinguishes "say again" from a second command. The helpers are in
// retransmit_test.go.

// TestARetryRepeatsTheSequenceNumber.
//
// SIA OSDP v2.2.2 §5.7: a device caches the reply it gave to each sequence
// number. Repeating the number says "I did not hear you, say again", and the
// device replays its cached answer instead of acting again. Advancing it would
// present the retry as a new command -- and a door that was unlocked, whose
// reply was lost, would unlock a second time.
func TestARetryRepeatsTheSequenceNumber(t *testing.T) {
	ctx := context.Background()
	b := newBus(0x00)
	d := b.Devices()[0]
	online(t, b, d)

	b.Send(d, doorRelease())

	_, first := sent(t, b)
	b.Timeout(ctx, d)
	_, retry := sent(t, b)

	if retry != first {
		t.Errorf("retry used sequence %d, want %d repeated: a new sequence "+
			"presents the retry as a new command and the door opens twice",
			retry, first)
	}
}

// TestASuccessfulExchangeAdvancesTheSequence, so the repeat applies to the
// retry alone and not to everything after it.
func TestASuccessfulExchangeAdvancesTheSequence(t *testing.T) {
	ctx, now := context.Background(), time.Now()
	b := newBus(0x00)
	d := b.Devices()[0]
	online(t, b, d)

	b.Send(d, doorRelease())
	_, first := sent(t, b)
	b.Timeout(ctx, d)

	_, retry := sent(t, b)
	if _, err := b.Reply(ctx, d, reply(0x00, retry, cmd.ACK, nil), now); err != nil {
		t.Fatalf("Reply: %v", err)
	}

	if _, next := sent(t, b); next == first {
		t.Errorf("the sequence stayed at %d after a successful exchange", next)
	}
}
