// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package panel_test

import (
	"context"
	"testing"
	"time"

	"github.com/regalium-os/osdp-go/internal/bus"
	"github.com/regalium-os/osdp-go/internal/cmd"
)

// A command must survive the line losing its reply. The helpers are in
// send_test.go and support_test.go.

// TestACommandSurvivesALostReply, through the whole runtime.
//
// The device is deaf for one exchange -- it reads the panel's frame and says
// nothing, which is what a burst of noise on the reply looks like. The command
// must still arrive.
func TestACommandSurvivesALostReply(t *testing.T) {
	p, device := newPanel(t, 0x00)
	rec := &recorder{}
	go respond(t, device, rec.swallowOne(cmd.Out))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()
	waitFor(t, p.Events(), bus.KindOnline)

	if err := p.Send(ctx, 0x00, cmd.OutputCommand(cmd.Output{
		Number: 0, Control: cmd.OutputTimedOn, Timer: 5 * time.Second,
	})); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Seen twice: the swallowed attempt, then the retry that got through.
	rec.waitForCount(t, cmd.Out, 2)

	cancel()
	<-done
}
