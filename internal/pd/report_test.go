// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/regalium-os/osdp-go/internal/cmd"
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

// TestACredentialShorterThanItsBitCountIsRefused.
//
// The bit count is authoritative and is not implied by the payload length, so
// the two disagreeing is not a smaller credential -- it is one the panel has no
// reading of. It discards the reply and nothing anywhere raises an error, which
// makes a miscounted card read indistinguishable from a card never presented.
//
// The assertion runs the device's own output through the panel's own parser,
// because the two being the same code is the whole point: if this package can
// emit a frame cmd.ParseCardRead rejects, it has emitted a frame the panel
// rejects.
func TestACredentialShorterThanItsBitCountIsRefused(t *testing.T) {
	ctx := context.Background()
	d := newDevice(t)

	// 26 bits needs four octets.
	short := cmd.CardRead{Format: 1, BitCount: 26, Data: []byte{0x12, 0x34}}
	if err := d.ReportCardRead(ctx, short); !errors.Is(err, pd.ErrShortCredential) {
		t.Errorf("ReportCardRead returned %v, want ErrShortCredential", err)
	}
	if got := d.Pending(); got != 0 {
		t.Errorf("%d events queued, want 0: a credential the panel cannot read "+
			"was queued anyway", got)
	}

	// The exact boundary is accepted, and survives the round trip.
	exact := cmd.CardRead{Format: 1, BitCount: 26, Data: []byte{0x12, 0x34, 0x56, 0x80}}
	if err := d.ReportCardRead(ctx, exact); err != nil {
		t.Fatalf("ReportCardRead on a well-formed credential: %v", err)
	}
	if _, err := cmd.ParseCardRead(answer(t, d, poll(t, 1)).Data); err != nil {
		t.Errorf("the device emitted an osdp_RAW the panel cannot parse: %v", err)
	}
}
