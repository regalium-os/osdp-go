// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/rand"
	"time"

	"github.com/regalium-os/osdp-go"
)

// simulateReader plays a peripheral device on the far end of an in-memory port.
//
// It exists so -demo works with no hardware, and that is worth more than it
// sounds: when a real line is silent, the first question is whether the problem
// is the wiring or the tool. Running -demo answers it in two seconds.
//
// It is a plain reader -- no secure channel -- because the point is to exercise
// this program, not to reimplement a device. internal/bus's test peripheral is
// the one that does the cryptography.
func simulateReader(ctx context.Context, port osdp.Port, cfg config) {
	defer func() { _ = port.Close() }()

	buf := make([]byte, 512)
	var polls int

	for ctx.Err() == nil {
		if err := port.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			return
		}
		n, err := port.Read(buf)
		if err != nil {
			continue // a deadline passed; the panel has not asked anything
		}

		command, err := osdp.Decode(ctx, buf[:n])
		if err != nil {
			continue
		}

		polls++
		answer := simulatedAnswer(command, polls)
		wire, err := answer.Append(ctx, nil)
		if err != nil {
			return
		}
		if _, err := port.Write(wire); err != nil {
			return
		}
	}
}

// simulatedAnswer is what the pretend reader says.
//
// It presents a card every fiftieth poll, which at the default turnaround is
// roughly every half second: often enough to see the event loop work, rare
// enough that the output stays readable.
func simulatedAnswer(command osdp.Frame, polls int) osdp.Frame {
	const (
		codeID     = 0x61
		codeCap    = 0x62
		codePoll   = 0x60
		codeComSet = 0x6E
	)

	switch command.Code {
	case codeID:
		// HID's OUI, a made-up model, firmware 1.0.3.
		return simulatedReply(command, 0x45,
			[]byte{0x00, 0x06, 0x8E, 0x01, 0x02, 0x0A, 0x0B, 0x0C, 0x0D, 0x01, 0x00, 0x03})

	case codeCap:
		return simulatedReply(command, 0x46, []byte{
			0x01, 0x01, 0x04, // 4 monitored inputs
			0x02, 0x01, 0x04, // 4 controlled outputs
			0x04, 0x01, 0x02, // 2 LEDs
			0x08, 0x01, 0x00, // CRC-16
			0x09, 0x00, 0x00, // no secure channel
			0x0A, 0x80, 0x00, // 128-octet buffer
			0x0D, 0x01, 0x01, // one reader
		})

	case codeComSet:
		// A real device confirms what it adopted and applies the change after
		// replying. Echoing the request is the simple case; a device that
		// could not manage the requested baud would answer with the one it
		// settled for, and the panel is built to believe the reply rather than
		// the request. Worth knowing that this simulator does not exercise
		// that difference.
		return simulatedReply(command, 0x54, append([]byte(nil), command.Data...))

	case codePoll:
		if polls%50 == 0 {
			// Reader 0, format 1, 26 bits of Wiegand, with a random credential
			// so nobody is tempted to treat the output as a fixture.
			card := []byte{0x00, 0x01, 0x1A, 0x00, 0, 0, 0, 0}
			_, _ = rand.Read(card[4:])
			return simulatedReply(command, 0x50, card)
		}
	}

	return simulatedReply(command, 0x40, nil) // osdp_ACK
}

// simulatedReply builds a sealed reply at the sequence the panel used.
func simulatedReply(command osdp.Frame, code byte, data []byte) osdp.Frame {
	f := osdp.Frame{
		IsReply: true,
		Address: command.Address,
		Control: osdp.NewControl(command.Control.Sequence(), osdp.SchemeCRC16, false),
		Code:    code,
		Data:    data,
	}
	f.Seal()
	return f
}

// randomNonce supplies RND.A for a secure handshake.
//
// crypto/rand and nothing else: RND.A is half the input to session key
// derivation, and a predictable one lets anyone who has ever seen the base key
// replay a session.
func randomNonce() [8]byte {
	var n [8]byte
	if _, err := rand.Read(n[:]); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return n
}
