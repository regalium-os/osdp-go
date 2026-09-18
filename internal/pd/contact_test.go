// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd_test

import (
	"context"
	"errors"
	"testing"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/pd"
)

// Contact state a device volunteers in answer to a poll: a door opening, a
// tamper tripping. Credentials are report_test.go; the helpers are in
// support_test.go.

// TestAnInputChangeIsReportedUnsolicited.
//
// A panel that only learned about contacts when it asked would learn about a
// forced door on its next sweep rather than now, and "now" is the whole point
// of a door contact.
func TestAnInputChangeIsReportedUnsolicited(t *testing.T) {
	ctx := context.Background()
	d := newDevice(t, pd.WithContacts(4, 0, 1))

	if err := d.SetInput(ctx, 2, true); err != nil {
		t.Fatalf("SetInput: %v", err)
	}

	reply := answer(t, d, poll(t, 1))
	replyCode(t, reply, cmd.IStatR)

	changes, err := cmd.ParseInputStatus(reply.Data)
	if err != nil {
		t.Fatalf("osdp_ISTATR would not parse: %v", err)
	}
	if len(changes) != 4 || !changes[2].Active {
		t.Errorf("input report was %+v, want input 2 active out of four", changes)
	}
}

// TestStatusReportsCoalesceRatherThanQueue.
//
// A chattering door-position switch is a real and common field fault. Appending
// every snapshot would fill the queue with stale readings and push a card read
// out behind them; replacing keeps the newest, which is the only one that is
// still true.
func TestStatusReportsCoalesceRatherThanQueue(t *testing.T) {
	ctx := context.Background()
	d := newDevice(t, pd.WithContacts(2, 0, 1), pd.WithEventQueue(2))

	for range 10 {
		if err := d.SetInput(ctx, 0, true); err != nil {
			t.Fatalf("SetInput: %v", err)
		}
		if err := d.SetInput(ctx, 0, false); err != nil {
			t.Fatalf("SetInput: %v", err)
		}
	}

	if got := d.Pending(); got != 1 {
		t.Errorf("%d events queued after twenty state changes, want 1", got)
	}
	if err := d.ReportCardRead(ctx, cmd.CardRead{BitCount: 8, Data: []byte{1}}); err != nil {
		t.Errorf("a chattering contact crowded out a card read: %v", err)
	}
}

// TestTamperAndPowerShareOneReport, because osdp_LSTATR carries the pair and
// there is no way to send half of it.
func TestTamperAndPowerShareOneReport(t *testing.T) {
	ctx := context.Background()
	d := newDevice(t)

	if err := d.SetTamper(ctx, true); err != nil {
		t.Fatalf("SetTamper: %v", err)
	}
	if err := d.SetPower(ctx, true); err != nil {
		t.Fatalf("SetPower: %v", err)
	}
	if got := d.Pending(); got != 1 {
		t.Fatalf("%d events queued, want 1", got)
	}

	reply := answer(t, d, poll(t, 1))
	replyCode(t, reply, cmd.LStatR)

	changes, err := cmd.ParseLocalStatus(reply.Data)
	if err != nil {
		t.Fatalf("osdp_LSTATR would not parse: %v", err)
	}
	for _, c := range changes {
		if !c.Active {
			t.Errorf("%s reported normal after being set", c.Kind)
		}
	}
}

// TestAnUnknownInputIsRefused, because a device that accepted an input it does
// not have would report a status payload wider than its own capability report.
func TestAnUnknownInputIsRefused(t *testing.T) {
	d := newDevice(t, pd.WithContacts(2, 0, 1))

	if err := d.SetInput(context.Background(), 7, true); !errors.Is(err, pd.ErrNoSuchContact) {
		t.Errorf("SetInput on a device with two inputs returned %v, want ErrNoSuchContact", err)
	}
}
