package frame_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/regalium-os/osdp-go/internal/frame"
)

// TestDecodeFields asserts the interpretation of each header field, not merely
// that a round trip survives. A codec that swapped two fields consistently
// would round-trip perfectly and still be wrong.
func TestDecodeFields(t *testing.T) {
	// osdp_POLL, CP to PD 0x00, CRC, sequence 1.
	raw := []byte{0x53, 0x00, 0x08, 0x00, 0x05, 0x60, 0xDA, 0x99}

	f, err := frame.Decode(context.Background(), raw)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if f.Address != 0x00 {
		t.Errorf("Address = 0x%02X, want 0x00", byte(f.Address))
	}
	if f.IsReply {
		t.Error("IsReply is set on a command from the control panel")
	}
	if got := f.Control.Sequence(); got != 1 {
		t.Errorf("Sequence = %d, want 1", got)
	}
	if got := f.Control.Scheme(); got != frame.SchemeCRC16 {
		t.Errorf("Scheme = %v, want crc16", got)
	}
	if f.Control.HasSecurityBlock() || f.Security != nil {
		t.Error("a security block was reported where none is declared")
	}
	if f.Code != 0x60 {
		t.Errorf("Code = 0x%02X, want 0x60 (osdp_POLL)", f.Code)
	}
	if len(f.Data) != 0 {
		t.Errorf("Data = % X, want empty", f.Data)
	}
	if f.Check != 0x99DA {
		t.Errorf("Check = 0x%04X, want 0x99DA (octets are little-endian)", f.Check)
	}
}

// TestDecodeReplyFlag: the high bit of the address octet is the direction, and
// must not leak into the address itself.
func TestDecodeReplyFlag(t *testing.T) {
	raw := []byte{0x53, 0x80, 0x08, 0x00, 0x05, 0x40, 0x68, 0x9F}

	f, err := frame.Decode(context.Background(), raw)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !f.IsReply {
		t.Error("IsReply is clear on a frame from a peripheral device")
	}
	if f.Address != 0x00 {
		t.Errorf("Address = 0x%02X, want 0x00; the reply flag must not be part of it",
			byte(f.Address))
	}
}

func TestDecodeErrors(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want error
	}{
		{"empty", nil, frame.ErrShortBuffer},
		{"mark only", []byte{0xFF}, frame.ErrShortBuffer},
		{"not a frame", []byte{0x41, 0x42, 0x43, 0x44, 0x45}, frame.ErrNoStartOfMessage},
		{"header truncated", []byte{0x53, 0x00, 0x08}, frame.ErrShortBuffer},
		{"body truncated", []byte{0x53, 0x00, 0x08, 0x00, 0x05, 0x60}, frame.ErrShortBuffer},
		{"length below the minimum", []byte{0x53, 0x00, 0x03, 0x00, 0x05, 0x60, 0x00, 0x00}, frame.ErrLengthMismatch},
		{"security block overruns", []byte{0x53, 0x00, 0x09, 0x00, 0x0D, 0x40, 0x11, 0x60, 0x00}, frame.ErrBadSecurityBlock},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := frame.Decode(context.Background(), tt.in)
			if !errors.Is(err, tt.want) {
				t.Errorf("Decode(% X) = %v, want %v", tt.in, err, tt.want)
			}
		})
	}
}

// TestDecodeReturnsFrameOnBadCheck: a corrupt frame is evidence, not rubbish.
func TestDecodeReturnsFrameOnBadCheck(t *testing.T) {
	raw := []byte{0x53, 0x00, 0x08, 0x00, 0x05, 0x60, 0x00, 0x00}

	f, err := frame.Decode(context.Background(), raw)
	if !errors.Is(err, frame.ErrBadCheck) {
		t.Fatalf("Decode = %v, want ErrBadCheck", err)
	}
	if f.Code != 0x60 {
		t.Error("the frame returned alongside ErrBadCheck must still be populated")
	}
	if f.CheckOK() {
		t.Error("CheckOK reports true for a frame with a corrupt check")
	}
}

// TestCloneBreaksAliasing documents the ownership contract by exercising it:
// Decode aliases the caller's buffer, and Clone is how a caller escapes that.
func TestCloneBreaksAliasing(t *testing.T) {
	raw := []byte{0x53, 0x00, 0x0B, 0x00, 0x05, 0x61, 0xAA, 0xBB, 0xCC, 0x00, 0x00}
	f, _ := frame.Decode(context.Background(), raw)

	aliased, cloned := f, f.Clone()
	raw[6] = 0xFF // simulate the next read into a reused buffer

	if len(aliased.Data) > 0 && aliased.Data[0] != 0xFF {
		t.Error("Decode is documented to alias the input buffer, but did not")
	}
	if len(cloned.Data) > 0 && cloned.Data[0] != 0xAA {
		t.Error("Clone must copy the payload out of the caller's buffer")
	}
}

func TestAppendRejectsInvalidAddress(t *testing.T) {
	f := frame.Frame{Address: 0xFF, Control: frame.NewControl(1, frame.SchemeCRC16, false)}
	if _, err := f.Append(context.Background(), nil); !errors.Is(err, frame.ErrInvalidAddress) {
		t.Errorf("Append with address 0xFF = %v, want ErrInvalidAddress", err)
	}
}

// TestSealProducesAValidFrame closes the loop: compose, seal, encode, decode.
func TestSealProducesAValidFrame(t *testing.T) {
	ctx := context.Background()
	f := frame.Frame{
		Address: 0x03,
		Control: frame.NewControl(2, frame.SchemeCRC16, false),
		Code:    0x60,
	}
	f.Seal()

	wire, err := f.Append(ctx, nil)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	got, err := frame.Decode(ctx, wire)
	if err != nil {
		t.Fatalf("Decode of a sealed frame: %v", err)
	}
	round, _ := got.Append(ctx, nil)
	if !bytes.Equal(round, wire) {
		t.Errorf("round trip differs\n  want % X\n  got  % X", wire, round)
	}
}
