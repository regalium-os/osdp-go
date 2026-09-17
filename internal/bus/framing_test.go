// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package bus_test

import (
	"testing"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/internal/secure"
)

// deviceIDPayload is an osdp_PDID reply: HID's OUI, model 1, version 2.
var deviceIDPayload = []byte{
	0x00, 0x06, 0x8E, 0x01, 0x02, 0x0A, 0x0B, 0x0C, 0x0D, 0x01, 0x00, 0x03,
}

// handshakeReply builds a reply carrying a handshake security block.
//
// Handshake frames carry no message authentication code: there is no session to
// authenticate them with yet, which is what the cryptograms inside them are for.
func handshakeReply(
	addr frame.Address, seq uint8, block secure.BlockType, code cmd.Code, data []byte,
) frame.Frame {
	f := frame.Frame{
		IsReply:  true,
		Address:  addr,
		Control:  frame.NewControl(seq, frame.SchemeCRC16, true),
		Security: &frame.SecurityBlock{Type: byte(block)},
		Code:     byte(code),
		Data:     data,
	}
	f.Seal()
	return f
}

// sealedReply builds an authenticated reply, enciphering the payload when the
// block calls for it.
//
// The order is the specification's and each step depends on the one before: the
// payload is enciphered, the frame is assembled with room for the code reserved
// so the length field is final, the code is computed over everything preceding
// it, and the error check is written last. SIA OSDP v2.2.2 §7.
func (p *peripheral) sealedReply(
	t *testing.T, addr frame.Address, seq uint8,
	block secure.BlockType, code cmd.Code, payload []byte,
) frame.Frame {
	t.Helper()

	if block.Encrypted() {
		ciphertext, err := p.session.Seal(payload, false)
		if err != nil {
			t.Fatalf("device could not encipher its reply: %v", err)
		}
		payload = ciphertext
	}

	data := make([]byte, len(payload)+secure.MACTagSize)
	copy(data, payload)

	f := frame.Frame{
		IsReply:  true,
		Address:  addr,
		Control:  frame.NewControl(seq, frame.SchemeCRC16, true),
		Security: &frame.SecurityBlock{Type: byte(block)},
		Code:     byte(code),
		Data:     data,
	}

	body := f.AppendBody(nil)
	tag, err := p.session.Authenticate(body[:len(body)-secure.MACTagSize], false)
	if err != nil {
		t.Fatalf("device could not authenticate its reply: %v", err)
	}
	copy(f.Data[len(f.Data)-secure.MACTagSize:], tag[:])

	f.Seal()
	return f
}
