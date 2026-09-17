// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package frame_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/regalium-os/osdp-go/internal/frame"
)

// FuzzEncodeRoundTrip fuzzes the codec from the other end: build a frame from
// arbitrary field values, put it on the wire, read it back.
//
// # Why both directions
//
// FuzzDecodeRoundTrip feeds arbitrary octets in, and nearly all of them are
// rejected in the first few bytes -- which exercises the rejection path
// thoroughly and the codec barely at all. Starting from fields instead means
// every single execution produces a frame the encoder considers valid, so the
// length arithmetic, the security block, the reserved control bits and the
// check computation are all reached on every run rather than by luck.
//
// The two together are the claim: what the encoder emits, the decoder reads
// back identically, and what the decoder accepts, the encoder reproduces.
func FuzzEncodeRoundTrip(f *testing.F) {
	// address, control, code, mark, security type, security data, payload
	f.Add(uint8(0x00), uint8(0x05), uint8(0x60), false, uint8(0), []byte(nil), []byte(nil))
	f.Add(uint8(0x7F), uint8(0x0D), uint8(0x80), true, uint8(0x11), []byte{1, 2}, []byte{3, 4})
	f.Add(uint8(0x01), uint8(0x02), uint8(0x40), false, uint8(0), []byte(nil), []byte{0xFF})
	f.Add(uint8(0x00), uint8(0xFC), uint8(0x50), true, uint8(0x17), []byte(nil), make([]byte, 128))

	ctx := context.Background()
	f.Fuzz(func(t *testing.T,
		address, control, code uint8, mark bool,
		blockType uint8, blockData, payload []byte,
	) {
		built := frame.Frame{
			HasMark: mark,
			Address: frame.Address(address),
			Control: frame.Control(control),
			Code:    code,
			Data:    payload,
		}
		if !built.Address.Valid() {
			return // an address the encoder is documented to refuse
		}
		if built.Control.HasSecurityBlock() {
			built.Security = &frame.SecurityBlock{Type: blockType, Data: blockData}
		}

		built.Seal()

		wire, err := built.Append(ctx, nil)
		if err != nil {
			// The encoder refuses what the wire cannot describe. That is a
			// correct outcome and the only other one allowed: what it must
			// never do is emit a frame whose length field wrapped.
			if errors.Is(err, frame.ErrSecurityBlockTooLong) ||
				errors.Is(err, frame.ErrFrameTooLong) {
				return
			}
			t.Fatalf("a sealed frame would not encode: %v", err)
		}

		got, err := frame.Decode(ctx, wire)
		if err != nil {
			t.Fatalf("the encoder produced octets the decoder rejects: %v\n  % X", err, wire)
		}

		assertSameFrame(t, built, got, wire)

		back, err := got.Append(ctx, nil)
		if err != nil {
			t.Fatalf("Append after Decode: %v", err)
		}
		if !bytes.Equal(back, wire) {
			t.Errorf("round trip is not byte-exact\n  want % X\n  got  % X", wire, back)
		}
	})
}

// assertSameFrame compares the fields that travel on the wire.
//
// Every one of them is checked, not a sample: a field that survives encoding
// but is dropped by decoding produces a frame that still round-trips byte for
// byte while meaning something else entirely, and byte-exactness alone would
// not notice.
func assertSameFrame(t *testing.T, want, got frame.Frame, wire []byte) {
	t.Helper()

	switch {
	case got.HasMark != want.HasMark:
		t.Errorf("mark octet = %v, want %v\n  % X", got.HasMark, want.HasMark, wire)
	case got.IsReply != want.IsReply:
		t.Errorf("reply flag = %v, want %v", got.IsReply, want.IsReply)
	case got.Address != want.Address:
		t.Errorf("address = %#x, want %#x", got.Address, want.Address)
	case got.Control != want.Control:
		t.Errorf("control = %#x, want %#x: the reserved bits must survive",
			got.Control, want.Control)
	case got.Code != want.Code:
		t.Errorf("code = %#x, want %#x", got.Code, want.Code)
	case got.Check != want.Check:
		t.Errorf("check = %#x, want %#x", got.Check, want.Check)
	case !bytes.Equal(got.Data, want.Data):
		t.Errorf("payload = % X, want % X", got.Data, want.Data)
	case (got.Security == nil) != (want.Security == nil):
		t.Errorf("security block present = %v, want %v",
			got.Security != nil, want.Security != nil)
	case got.Security != nil && got.Security.Type != want.Security.Type:
		t.Errorf("security block type = %#x, want %#x",
			got.Security.Type, want.Security.Type)
	case got.Security != nil && !bytes.Equal(got.Security.Data, want.Security.Data):
		t.Errorf("security block data = % X, want % X",
			got.Security.Data, want.Security.Data)
	}
}
