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

// playReader is a reader that identifies itself, declares one reader and no
// Secure Channel, and then presents a credential to every poll.
func playReader(ctx context.Context, port osdp.Port) {
	buf := make([]byte, 512)

	for ctx.Err() == nil {
		if err := port.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			return
		}
		n, err := port.Read(buf)
		if err != nil {
			return
		}

		command, err := osdp.Decode(ctx, buf[:n])
		if err != nil {
			return
		}

		var code byte
		var data []byte
		switch command.Code {
		case 0x61: // osdp_ID
			code = 0x45
			data = []byte{0x00, 0x06, 0x8E, 0x01, 0x02, 0x0A, 0x0B, 0x0C, 0x0D, 0x01, 0x00, 0x03}
		case 0x62: // osdp_CAP
			code = 0x46
			data = []byte{0x08, 0x01, 0x00, 0x09, 0x00, 0x00, 0x0D, 0x01, 0x01}
		default:
			code = 0x50 // osdp_RAW
			data = []byte{0x00, 0x01, 0x1A, 0x00, 0xAB, 0xCD, 0xEF, 0x80}
		}

		reply := osdp.Frame{
			IsReply: true,
			Address: command.Address,
			Control: osdp.NewControl(command.Control.Sequence(), osdp.SchemeCRC16, false),
			Code:    code,
			Data:    data,
		}
		reply.Seal()

		wire, err := reply.Append(ctx, nil)
		if err != nil {
			return
		}
		if _, err := port.Write(wire); err != nil {
			return
		}
	}
}
