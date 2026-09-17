// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package provider_test

import (
	"bufio"
	"context"
	"encoding/hex"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
)

// fixture is one record of the provider corpus: a frame off the wire and the
// expectations it asserts once the stack has finished with it.
type fixture struct {
	name string
	desc string
	want map[string]string
	raw  []byte
	line int
}

// knownKeys is every expectation the corpus may state.
//
// An unrecognised key is a fatal error rather than a skipped line. A corpus
// where ":secur true" silently asserts nothing is worse than no corpus, because
// it reports success for a check that never ran.
var knownKeys = []string{
	"vendor", "error",
	"readers", "leds", "buzzers", "displays", "inputs", "outputs",
	"secure", "defaultkey", "maxmessage",
	"oui", "recognised", "body",
}

// loadCorpus reads the fixture file. A record opens with @name, carries a !
// description and any number of :key value expectations, and is followed by
// whitespace-separated hex octets.
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
			cur = &fixture{
				name: strings.TrimPrefix(line, "@"),
				want: make(map[string]string),
				line: n,
			}
		case cur == nil:
			t.Fatalf("%s:%d: content before any @name", path, n)
		case strings.HasPrefix(line, "!"):
			cur.desc = strings.TrimPrefix(line, "!")
		case strings.HasPrefix(line, ":"):
			key, value, _ := strings.Cut(strings.TrimPrefix(line, ":"), " ")
			if !slices.Contains(knownKeys, key) {
				t.Fatalf("%s:%d: unknown expectation %q; a typo here asserts nothing\n"+
					"  known keys: %v", path, n, key, knownKeys)
			}
			cur.want[key] = strings.TrimSpace(value)
		default:
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

// decode takes the record from octets to a message, asserting on the way that
// the corpus itself is well formed: a fixture whose frame does not decode is a
// broken fixture, not a finding about the code under test.
func (fx fixture) decode(t *testing.T) (frame.Frame, cmd.Message) {
	t.Helper()

	f, err := frame.Decode(context.Background(), fx.raw)
	if err != nil {
		t.Fatalf("Decode: %v\n  %s\n  % X", err, fx.desc, fx.raw)
	}
	if !f.CheckOK() {
		t.Fatalf("fixture %q carries a bad error check; the corpus is wrong", fx.name)
	}

	m, err := cmd.Decode(context.Background(), f)
	if err != nil {
		t.Fatalf("cmd.Decode: %v", err)
	}
	return f, m
}

// value returns the expectation for key, failing when the record omits it.
func (fx fixture) value(t *testing.T, key string) string {
	t.Helper()
	v, ok := fx.want[key]
	if !ok {
		t.Fatalf("fixture %q (line %d) states no %q expectation", fx.name, fx.line, key)
	}
	return v
}

// number returns a numeric expectation.
func (fx fixture) number(t *testing.T, key string) int {
	t.Helper()
	n, err := strconv.Atoi(fx.value(t, key))
	if err != nil {
		t.Fatalf("fixture %q: %s: %v", fx.name, key, err)
	}
	return n
}

// flag returns a boolean expectation.
func (fx fixture) flag(t *testing.T, key string) bool {
	t.Helper()
	b, err := strconv.ParseBool(fx.value(t, key))
	if err != nil {
		t.Fatalf("fixture %q: %s: %v", fx.name, key, err)
	}
	return b
}

// octets returns a hex expectation, which may legitimately be empty.
func (fx fixture) octets(t *testing.T, key string) []byte {
	t.Helper()
	b, err := hex.DecodeString(fx.value(t, key))
	if err != nil {
		t.Fatalf("fixture %q: %s: %v", fx.name, key, err)
	}
	return b
}
