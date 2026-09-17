// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package osdp_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/regalium-os/osdp-go"
)

// Peripheral devices, played from the far end of an in-memory port. They are
// scenery for the runtime tests next door: what a reader does, expressed
// through the same public API a consumer would use to write one.

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

// codeLog records every command code a device was sent.
//
// It is a slice rather than a channel on purpose. A channel has to be sized,
// and a poll cycle fills any size chosen: the codes the test cares about then
// arrive to find no room and are dropped, which makes the test fail sometimes
// and pass others. Keeping everything and searching it is deterministic.
type codeLog struct {
	mu    sync.Mutex
	codes []byte
}

func (l *codeLog) record(code byte) {
	l.mu.Lock()
	l.codes = append(l.codes, code)
	l.mu.Unlock()
}

// awaitCode blocks until the device has been sent want.
func (l *codeLog) awaitCode(t *testing.T, want byte) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		l.mu.Lock()
		for _, code := range l.codes {
			if code == want {
				l.mu.Unlock()
				return
			}
		}
		l.mu.Unlock()
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("command 0x%02X never reached the device", want)
}

// playReaderRecording is playReader with a note of every code it was sent.
func playReaderRecording(ctx context.Context, port osdp.Port, log *codeLog) {
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

		log.record(command.Code)

		reply := osdp.Frame{
			IsReply: true,
			Address: command.Address,
			Control: osdp.NewControl(command.Control.Sequence(), osdp.SchemeCRC16, false),
			Code:    0x40, // osdp_ACK
		}
		switch command.Code {
		case 0x61: // osdp_ID
			reply.Code = 0x45
			reply.Data = []byte{0x00, 0x06, 0x8E, 0x01, 0x02, 0x0A, 0x0B, 0x0C, 0x0D, 0x01, 0x00, 0x03}
		case 0x62: // osdp_CAP
			reply.Code = 0x46
			reply.Data = []byte{0x08, 0x01, 0x00, 0x09, 0x00, 0x00, 0x0D, 0x01, 0x01}
		case 0x60: // osdp_POLL
			reply.Code = 0x50
			reply.Data = []byte{0x00, 0x01, 0x1A, 0x00, 0xAB, 0xCD, 0xEF, 0x80}
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
