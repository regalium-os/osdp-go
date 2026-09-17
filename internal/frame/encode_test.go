// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package frame_test

import (
	"context"
	"errors"
	"testing"

	"github.com/regalium-os/osdp-go/internal/frame"
)

// What the encoder refuses to write. Reading a frame is decode_test.go; this is
// the other side, where a frame that cannot be described has to be rejected
// rather than truncated into one that can.

// TestAnUnrepresentableFrameIsRefused.
//
// Found by FuzzEncodeRoundTrip, and worth a named test of its own because the
// interesting case is not the one that errors.
//
// The security block's length is a single octet. A block of 254 data octets
// makes that length 256, which wraps to zero and produces a frame the decoder
// rejects -- noisy, but recoverable. A block of 300 wraps to 46, and the
// decoder accepts it: it reads a 46-octet block and takes the rest of the
// intended block as the command code and its payload. That frame is well
// formed, decodes cleanly, and means something nobody composed.
//
// Refusing to encode is the only outcome that cannot be mistaken for success.
func TestAnUnrepresentableFrameIsRefused(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name    string
		data    int
		payload int
		want    error
	}{
		{name: "the largest block that fits", data: 253, want: nil},
		{name: "one octet too many, and the length wraps to zero", data: 254,
			want: frame.ErrSecurityBlockTooLong},
		{name: "wrapped to a length the decoder believes", data: 300,
			want: frame.ErrSecurityBlockTooLong},
		{name: "a payload past what the header can describe", payload: 0x10000,
			want: frame.ErrFrameTooLong},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := frame.Frame{
				Address: 0x00,
				Control: frame.NewControl(1, frame.SchemeCRC16, tc.data > 0),
				Code:    0x60,
				Data:    make([]byte, tc.payload),
			}
			if tc.data > 0 {
				f.Security = &frame.SecurityBlock{Type: 0x17, Data: make([]byte, tc.data)}
			}
			f.Seal()

			wire, err := f.Append(ctx, nil)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Append = %v, want %v", err, tc.want)
			}
			if tc.want != nil {
				return
			}

			// The accepted case must still round trip.
			got, err := frame.Decode(ctx, wire)
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if got.Security == nil || len(got.Security.Data) != tc.data {
				t.Errorf("security block came back with %d octets, want %d",
					len(got.Security.Data), tc.data)
			}
		})
	}
}
