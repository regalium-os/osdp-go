// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package panel_test

import (
	"context"
	"testing"

	"github.com/regalium-os/osdp-go/internal/bus"
	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
)

// What a panel learns about a door without having asked: contact state arrives
// in answer to an ordinary poll. The helpers are in support_test.go.

// TestATamperReachesTheApplication.
//
// The whole point of status monitoring: a reader is pulled off a wall, the
// device says so in answer to an ordinary poll, and the application finds out
// without having asked anything.
func TestATamperReachesTheApplication(t *testing.T) {
	p, device := newPanel(t, 0x00)
	go respond(t, device, tamperOnPoll)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()

	ev := waitFor(t, p.Events(), bus.KindStatusChange)
	var tampered bool
	for _, c := range ev.Status {
		if c.Kind == cmd.StatusTamper && c.Active {
			tampered = true
		}
	}
	if !tampered {
		t.Errorf("status changes = %+v, want an active tamper", ev.Status)
	}

	cancel()
	<-done
}

// tamperOnPoll is a reader that enrols and then answers every poll with a
// local status reply saying its tamper switch has tripped.
func tamperOnPoll(command frame.Frame) frame.Frame {
	seq := command.Control.Sequence()

	switch cmd.Code(command.Code) {
	case cmd.ID:
		return reply(command.Address, seq, cmd.PDID,
			[]byte{0x00, 0x06, 0x8E, 0x01, 0x02, 0x0A, 0x0B, 0x0C, 0x0D, 0x01, 0x00, 0x03})
	case cmd.Cap:
		return reply(command.Address, seq, cmd.PDCap,
			[]byte{0x08, 0x01, 0x00, 0x09, 0x00, 0x00, 0x0D, 0x01, 0x01})
	default:
		// osdp_LSTATR: tamper tripped, power fine.
		return reply(command.Address, seq, cmd.LStatR, []byte{0x01, 0x00})
	}
}
