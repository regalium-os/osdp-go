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

// FuzzDecodeRoundTrip asserts the property the whole codec rests on: whatever
// Decode accepts, Append reproduces octet for octet.
//
// # Why a fuzzer and not more fixtures
//
// The hex corpus proves the frames somebody thought of. This proves the ones
// nobody did -- and on a multidrop line shared with noise, malformed input is
// the normal case rather than the exception. A codec that quietly normalises
// what it did not understand cannot be trusted to tell an operator what a
// misbehaving reader actually put on the wire, which is most of what a fixture
// corpus is for.
//
// # What is asserted
//
// Only round-trip exactness, and only for input Decode accepted. A frame with a
// bad error check counts as accepted: Decode returns it alongside ErrBadCheck
// precisely so it can be inspected, and it must survive re-encoding unaltered
// like any other.
func FuzzDecodeRoundTrip(f *testing.F) {
	seedCorpus(f)

	ctx := context.Background()
	f.Fuzz(func(t *testing.T, data []byte) {
		decoded, err := frame.Decode(ctx, data)
		if err != nil && !errors.Is(err, frame.ErrBadCheck) {
			return // rejected, and rejection is allowed to lose information
		}

		got, err := decoded.Append(ctx, nil)
		if err != nil {
			t.Fatalf("a frame Decode accepted would not re-encode: %v\n  in:  % X", err, data)
		}

		// Decode reads one frame from the front of the buffer and ignores what
		// follows, so the comparison is against the octets it claimed.
		want := data[:decoded.WireLen()]
		if !bytes.Equal(got, want) {
			t.Errorf("round trip is not byte-exact\n  in:   % X\n  out:  % X", want, got)
		}
	})
}

// FuzzDecodeNeverPanics.
//
// Malformed input is the normal case on a wire shared with noise, and a panel
// that crashes on a corrupt frame takes a building's doors with it. The library
// rule is never to panic; this is the check that it holds for input nobody
// wrote by hand.
func FuzzDecodeNeverPanics(f *testing.F) {
	seedCorpus(f)

	ctx := context.Background()
	f.Fuzz(func(t *testing.T, data []byte) {
		decoded, err := frame.Decode(ctx, data)

		// Every accessor, on whatever came back. A frame returned alongside an
		// error is still a value a caller may inspect.
		_ = decoded.Len()
		_ = decoded.WireLen()
		_ = decoded.Trace()
		_ = decoded.Control.Sequence()
		_ = decoded.Control.Scheme().Size()
		_ = decoded.Address.Valid()

		clone := decoded.Clone()
		if err == nil || errors.Is(err, frame.ErrBadCheck) {
			_ = clone.CheckOK()
			if _, aErr := clone.Append(ctx, nil); aErr != nil && decoded.Address.Valid() {
				t.Fatalf("Append failed on a valid address: %v", aErr)
			}
		}
	})
}

// FuzzCloneIsIndependent: a Frame aliases the buffer it was decoded from, and
// Clone is the documented escape from that. A caller reusing a read buffer
// across the poll cycle depends on it completely.
func FuzzCloneIsIndependent(f *testing.F) {
	seedCorpus(f)

	ctx := context.Background()
	f.Fuzz(func(t *testing.T, data []byte) {
		buf := append([]byte(nil), data...)

		decoded, err := frame.Decode(ctx, buf)
		if err != nil && !errors.Is(err, frame.ErrBadCheck) {
			return
		}
		clone := decoded.Clone()

		before, err := clone.Append(ctx, nil)
		if err != nil {
			return
		}

		// The next read lands in the same buffer.
		for i := range buf {
			buf[i] = 0xFF
		}

		after, err := clone.Append(ctx, nil)
		if err != nil {
			t.Fatalf("a clone stopped encoding after its source buffer changed: %v", err)
		}
		if !bytes.Equal(before, after) {
			t.Errorf("the clone still aliases the decode buffer\n  before: % X\n  after:  % X",
				before, after)
		}
	})
}

// seedCorpus gives the fuzzer the shapes a hand-written corpus already knows
// about, so it starts from valid frames rather than discovering the start-of-
// message octet by chance.
func seedCorpus(f *testing.F) {
	f.Helper()

	for _, fx := range loadCorpus(f, "testdata/framing.hex") {
		f.Add(fx.raw)
	}

	// A few shapes the corpus does not carry, to save the fuzzer the trouble.
	f.Add([]byte{})
	f.Add([]byte{0x53})
	f.Add([]byte{0xFF, 0x53, 0x00, 0x08, 0x00, 0x05, 0x60, 0xDA, 0x99})
	f.Add([]byte{0x53, 0x00, 0x08, 0x00, 0x0D, 0x02, 0x15, 0x60, 0x00, 0x00})
}
