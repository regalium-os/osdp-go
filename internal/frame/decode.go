// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package frame

import (
	"context"

	"github.com/regalium-os/osdp-go/telemetry"
)

// Decode parses one frame from the front of buf.
//
// # Ownership
//
// The returned Frame aliases buf: Data and Security.Data point into it, and
// nothing is copied. Call Frame.Clone to retain the frame past the next write
// into buf. buf is never modified.
//
// # A failed error check is not a parse failure
//
// When the trailing CRC or checksum does not match, Decode returns a fully
// populated Frame together with ErrBadCheck. The frame is evidence: on a
// multidrop line shared with noise, a corrupt frame is the thing an operator
// most needs to see, and discarding it unparsed is how a wiring fault stays
// invisible for a week. Callers that do not care may simply check the error.
//
// Every other error returns the zero Frame. ErrShortBuffer specifically means
// "wait for more octets", not "resynchronise".
func Decode(ctx context.Context, buf []byte) (Frame, error) {
	_, span := telemetry.Start(ctx, "osdp.frame.decode")
	defer span.End()

	f, err := decode(buf)
	if f.Len() >= HeaderSize {
		telemetry.Apply(span, f.Trace())
	}
	span.RecordError(err)
	return f, err
}

// decode holds the parsing proper, so the tracing above stays readable.
func decode(buf []byte) (Frame, error) {
	var f Frame

	body := buf
	if len(body) > 0 && body[0] == Mark {
		f.HasMark = true
		body = body[1:]
	}

	if len(body) == 0 {
		return Frame{}, ErrShortBuffer
	}
	if body[0] != SOM {
		return Frame{}, ErrNoStartOfMessage
	}
	if len(body) < HeaderSize {
		return Frame{}, ErrShortBuffer
	}

	addr := body[1]
	f.IsReply = addr&replyFlag != 0
	f.Address = Address(addr &^ replyFlag)
	f.Control = Control(body[4])

	// The length octets are little-endian and count from the start of message
	// through the error check, excluding any mark octet.
	total := int(body[2]) | int(body[3])<<8
	scheme := f.Control.Scheme()

	if total < HeaderSize+1+scheme.Size() {
		return Frame{}, ErrLengthMismatch
	}
	if len(body) < total {
		return Frame{}, ErrShortBuffer
	}
	rest := body[HeaderSize:total]

	if f.Control.HasSecurityBlock() {
		sb, remainder, err := decodeSecurityBlock(rest)
		if err != nil {
			return Frame{}, err
		}
		f.Security, rest = sb, remainder
	}

	// What remains is the code, the payload, and the error check.
	if len(rest) < 1+scheme.Size() {
		return Frame{}, ErrLengthMismatch
	}
	f.Code = rest[0]

	dataEnd := len(rest) - scheme.Size()
	f.Data = rest[1:dataEnd]

	check := rest[dataEnd:]
	if scheme == SchemeCRC16 {
		f.Check = uint16(check[0]) | uint16(check[1])<<8
	} else {
		f.Check = uint16(check[0])
	}

	if !f.CheckOK() {
		return f, ErrBadCheck
	}
	return f, nil
}

// decodeSecurityBlock splits the leading security block off rest. The leading
// octet counts the whole block, itself and the type octet included, so a value
// below two cannot be valid.
func decodeSecurityBlock(rest []byte) (*SecurityBlock, []byte, error) {
	if len(rest) < 2 {
		return nil, nil, ErrBadSecurityBlock
	}
	n := int(rest[0])
	if n < 2 || n > len(rest) {
		return nil, nil, ErrBadSecurityBlock
	}
	return &SecurityBlock{Type: rest[1], Data: rest[2:n]}, rest[n:], nil
}
