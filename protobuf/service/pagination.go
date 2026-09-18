// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"encoding/base64"
	"strconv"
	"strings"
)

// cursorSeparator joins the two halves of a page token.
//
// A NUL is used rather than a printable delimiter because a device identifier
// is whatever the panel's enrolment scheme allocates and could contain almost
// anything; a byte no identifier can carry is what keeps the split
// unambiguous without escaping.
const cursorSeparator = "\x00"

// encodeCursor makes the page token that continues a listing of parent below
// sequence number seq.
//
// The token is base64 so that it is opaque in the AIP-158 sense -- a client
// that decodes one and constructs its own is relying on something this package
// does not promise -- and raw URL encoding so that it survives the REST binding
// google.api.http declares, where it travels as a query parameter.
//
// It carries the parent because ListEventsRequest.page_token requires that all
// other arguments match the call that produced the token. Comparing the two is
// cheaper and more honest than trusting a client to resend them unchanged.
func encodeCursor(parent string, seq uint64) string {
	raw := parent + cursorSeparator + strconv.FormatUint(seq, 10)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// decodeCursor recovers the sequence number a page token continues from, and
// reports ErrInvalidPageToken unless the token was issued against parent.
//
// The returned bound is exclusive: the next page holds events strictly older
// than it. That is what makes paging stable while the bus is still running --
// events arriving during a listing are newer than every cursor already issued,
// so they appear above the page a client is reading rather than shifting the
// rows underneath it.
func decodeCursor(token, parent string) (uint64, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return 0, ErrInvalidPageToken
	}

	// Split at the last separator, not the first: the sequence number is the
	// only half known to be free of NUL, so the parent is everything to the
	// left of it and a parent carrying a stray separator still round-trips.
	idx := strings.LastIndex(string(raw), cursorSeparator)
	if idx < 0 {
		return 0, ErrInvalidPageToken
	}
	if string(raw[:idx]) != parent {
		return 0, ErrInvalidPageToken
	}

	seq, err := strconv.ParseUint(string(raw[idx+len(cursorSeparator):]), 10, 64)
	if err != nil {
		return 0, ErrInvalidPageToken
	}
	return seq, nil
}
