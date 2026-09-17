// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package frame_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/regalium-os/osdp-go/internal/frame"
)

// fixture is one record of the hex corpus.
type fixture struct {
	name string
	desc string
	raw  []byte
	line int
}

// loadCorpus reads the fixture file. A record opens with @name, carries a !
// description, and is followed by whitespace-separated hex octets.
func loadCorpus(t *testing.T, path string) []fixture {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening corpus: %v", err)
	}
	defer f.Close()

	var (
		out  []fixture
		cur  *fixture
		scan = bufio.NewScanner(f)
		n    int
	)
	for scan.Scan() {
		n++
		line := strings.TrimSpace(scan.Text())
		switch {
		case line == "" || strings.HasPrefix(line, "#"):
		case strings.HasPrefix(line, "@"):
			if cur != nil {
				out = append(out, *cur)
			}
			cur = &fixture{name: strings.TrimPrefix(line, "@"), line: n}
		case strings.HasPrefix(line, "!"):
			if cur == nil {
				t.Fatalf("%s:%d: description before any @name", path, n)
			}
			cur.desc = strings.TrimPrefix(line, "!")
		default:
			if cur == nil {
				t.Fatalf("%s:%d: octets before any @name", path, n)
			}
			b, err := hex.DecodeString(strings.ReplaceAll(line, " ", ""))
			if err != nil {
				t.Fatalf("%s:%d: %v", path, n, err)
			}
			cur.raw = append(cur.raw, b...)
		}
	}
	if err := scan.Err(); err != nil {
		t.Fatalf("reading corpus: %v", err)
	}
	if cur != nil {
		out = append(out, *cur)
	}
	return out
}

// TestFixtureRoundTrip is the Phase 1 exit criterion: every frame in the corpus
// decodes, and re-encoding the result reproduces the input octet for octet.
//
// Byte-exactness is the whole point. A codec that decodes a frame and re-emits
// something merely equivalent -- reserved control bits cleared, a mark octet
// dropped, a bad CRC silently corrected -- cannot be trusted to tell you what a
// misbehaving reader actually put on the wire.
func TestFixtureRoundTrip(t *testing.T) {
	corpus := loadCorpus(t, "testdata/framing.hex")
	if len(corpus) == 0 {
		t.Fatal("corpus is empty; this test is checking nothing")
	}

	ctx := context.Background()
	for _, fx := range corpus {
		t.Run(fx.name, func(t *testing.T) {
			f, err := frame.Decode(ctx, fx.raw)

			// A corrupt error check still yields a frame; every other error
			// is a decode failure the corpus does not contain.
			if err != nil && !errors.Is(err, frame.ErrBadCheck) {
				t.Fatalf("Decode: %v\n  %s\n  % X", err, fx.desc, fx.raw)
			}

			got, err := f.Append(ctx, nil)
			if err != nil {
				t.Fatalf("Append: %v", err)
			}
			if !bytes.Equal(got, fx.raw) {
				t.Errorf("round trip is not byte-exact\n  %s\n  want % X\n  got  % X",
					fx.desc, fx.raw, got)
			}
			if n := f.WireLen(); n != len(fx.raw) {
				t.Errorf("WireLen = %d, want %d", n, len(fx.raw))
			}
		})
	}
}

// TestFixtureCheckAgreesWithReference asserts that the CRC and checksum this
// package computes match the independent bitwise reference that generated the
// corpus. Records named bad-* carry a deliberately corrupted check.
func TestFixtureCheckAgreesWithReference(t *testing.T) {
	ctx := context.Background()
	for _, fx := range loadCorpus(t, "testdata/framing.hex") {
		t.Run(fx.name, func(t *testing.T) {
			f, err := frame.Decode(ctx, fx.raw)
			corrupt := strings.HasPrefix(fx.name, "bad-")

			switch {
			case corrupt && !errors.Is(err, frame.ErrBadCheck):
				t.Errorf("corrupted fixture decoded cleanly; want ErrBadCheck, got %v", err)
			case corrupt && !bytes.Equal(mustAppend(t, f), fx.raw):
				t.Error("a frame with a bad check must re-encode unchanged")
			case !corrupt && err != nil:
				t.Errorf("Decode: %v", err)
			case !corrupt && !f.CheckOK():
				t.Error("CheckOK reports false for a frame the reference says is valid")
			}
		})
	}
}

func mustAppend(t *testing.T, f frame.Frame) []byte {
	t.Helper()
	b, err := f.Append(context.Background(), nil)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	return b
}
