// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"encoding/base64"
	"errors"
	"math"
	"strings"
	"testing"
)

// TestCursorsRoundTrip: a token is only useful if the store can read back the
// position it cut.
func TestCursorsRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		parent string
		seq    uint64
	}{
		{name: "first page boundary", parent: "devices/7", seq: 0},
		{name: "ordinary", parent: "devices/7", seq: 41},
		{name: "wildcard listing", parent: "devices/-", seq: 1234},
		{name: "counter ceiling", parent: "devices/lobby-north", seq: math.MaxUint64},

		// A device identifier is whatever the panel's enrolment scheme
		// allocated. The token must survive one that contains the separator
		// rather than silently decoding to a different parent.
		{name: "identifier carrying the separator", parent: "devices/a\x00b", seq: 9},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := encodeCursor(tt.parent, tt.seq)
			got, err := decodeCursor(token, tt.parent)
			if err != nil {
				t.Fatalf("decodeCursor(%q): %v", token, err)
			}
			if got != tt.seq {
				t.Errorf("seq = %d, want %d", got, tt.seq)
			}
		})
	}
}

// TestATokenIsOpaque: AIP-158 calls page tokens opaque, and a client reading
// one is depending on something this package does not promise. Base64 is what
// makes that visible, and what lets the token survive the query parameter the
// google.api.http binding sends it in.
func TestATokenIsOpaque(t *testing.T) {
	token := encodeCursor("devices/7", 41)
	if strings.Contains(token, "devices") || strings.Contains(token, "41") {
		t.Errorf("token %q shows its contents; it is supposed to be opaque", token)
	}
	for _, c := range token {
		if c == '/' || c == '+' || c == '=' || c == '&' || c == '?' {
			t.Errorf("token %q contains %q, which does not survive a URL query", token, c)
		}
	}
}

// TestATokenIsRefusedUnderAnotherQuery is the assertion behind
// ListEventsRequest.page_token: "all other arguments must match the call that
// produced it". A token silently answering under a different parent would hand
// a client one device's audit trail while it believed it was reading another's.
func TestATokenIsRefusedUnderAnotherQuery(t *testing.T) {
	tests := []struct {
		name   string
		token  string
		parent string
	}{
		{name: "another device", token: encodeCursor("devices/7", 41), parent: "devices/8"},
		{name: "wildcard for a device", token: encodeCursor("devices/-", 41), parent: "devices/7"},
		{name: "device for a wildcard", token: encodeCursor("devices/7", 41), parent: "devices/-"},
		{name: "not base64", token: "not a token!", parent: "devices/7"},
		{name: "base64 of nothing", token: "", parent: "devices/7"},
		{name: "no separator", token: encodeNoSeparator("devices/7"), parent: "devices/7"},
		{name: "sequence not a number", token: encodeRaw("devices/7\x00ff"), parent: "devices/7"},
		{name: "sequence negative", token: encodeRaw("devices/7\x00-1"), parent: "devices/7"},
		{name: "sequence overflows uint64", token: encodeRaw("devices/7\x0099999999999999999999"), parent: "devices/7"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := decodeCursor(tt.token, tt.parent); !errors.Is(err, ErrInvalidPageToken) {
				t.Fatalf("decodeCursor(%q, %q) error = %v, want ErrInvalidPageToken",
					tt.token, tt.parent, err)
			}
		})
	}
}

// encodeRaw makes a token out of bytes the encoder would never produce, so the
// decoder is tested against a hostile client rather than only against itself.
func encodeRaw(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}

// encodeNoSeparator makes a token whose payload holds no separator at all.
func encodeNoSeparator(parent string) string { return encodeRaw(parent) }
