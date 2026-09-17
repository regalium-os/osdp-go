// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package osdp_test

import (
	"context"
	"testing"
	"time"

	"github.com/regalium-os/osdp-go"
)

// TestRuntimeDrivesALineFromOutside is the worked example: everything a control
// panel does, through the public API, with nothing mocked but the wire.
//
// It lives in package osdp_test so that a re-export which quietly became a
// wrapper would fail to compile here rather than in a consumer's build.
func TestRuntimeDrivesALineFromOutside(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	panelPort, devicePort := osdp.Pipe()
	defer func() { _ = devicePort.Close() }()

	line := osdp.Line{Name: "door-1", Baud: 9600, ReplyTimeout: 200 * time.Millisecond}
	runtime := osdp.NewPanel(
		osdp.NewBus(line, []osdp.Address{0x00}, osdp.SchemeCRC16),
		panelPort,
	)
	defer func() { _ = runtime.Close() }()

	go playReader(ctx, devicePort)

	done := make(chan error, 1)
	go func() { done <- runtime.Run(ctx) }()

	// The application's whole job: range over what happened.
	var card osdp.CardRead
	for event := range runtime.Events() {
		if event.Kind == osdp.EventCardRead {
			card = event.Card
			cancel()
			break
		}
	}

	if err := <-done; err != nil {
		t.Fatalf("Run = %v; a cancelled context is a clean stop", err)
	}
	if card.BitCount != 26 {
		t.Errorf("bit count = %d, want 26", card.BitCount)
	}
	if card.Format != 1 {
		t.Errorf("format = %d, want 1", card.Format)
	}
}

// TestUnlockingADoorFromOutside: the thing a control panel exists to do, using
// only the public API.
func TestUnlockingADoorFromOutside(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	panelPort, devicePort := osdp.Pipe()
	defer func() { _ = devicePort.Close() }()

	line := osdp.Line{Name: "door-1", Baud: 9600, ReplyTimeout: 200 * time.Millisecond}
	runtime := osdp.NewPanel(
		osdp.NewBus(line, []osdp.Address{0x00}, osdp.SchemeCRC16),
		panelPort,
	)
	defer func() { _ = runtime.Close() }()

	log := &codeLog{}
	go playReaderRecording(ctx, devicePort, log)

	done := make(chan error, 1)
	go func() { done <- runtime.Run(ctx) }()

	// Wait until the reader is enrolled, then grant access.
	for event := range runtime.Events() {
		if event.Kind == osdp.EventCardRead {
			break
		}
	}

	strike, indicator := osdp.Unlock(0, 0, 0, 5*time.Second)
	for _, command := range []osdp.Message{strike, indicator} {
		if err := runtime.Send(ctx, 0x00, command); err != nil {
			t.Fatalf("Send: %v", err)
		}
	}

	log.awaitCode(t, 0x68) // osdp_OUT: the strike is released
	log.awaitCode(t, 0x69) // osdp_LED: and the reader shows green

	cancel()
	if err := <-done; err != nil {
		t.Errorf("Run = %v, want nil", err)
	}
}
