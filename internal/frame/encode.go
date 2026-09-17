// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package frame

import (
	"context"

	"github.com/regalium-os/osdp-go/telemetry"
)

// Append serialises f onto dst and returns the extended slice.
//
// The error check is written exactly as Frame.Check holds it, without being
// recomputed. A frame decoded with a bad check therefore re-encodes to the same
// octets it arrived as, which is what makes byte-exact fixture comparison
// possible. Call Seal first to write a correct check.
//
// dst may be nil. f is not modified, and neither Data nor Security.Data is
// retained beyond the call.
func (f Frame) Append(ctx context.Context, dst []byte) ([]byte, error) {
	_, span := telemetry.Start(ctx, "osdp.frame.encode", f.Trace())
	defer span.End()

	if !f.Address.Valid() {
		span.RecordError(ErrInvalidAddress)
		return dst, ErrInvalidAddress
	}

	if f.HasMark {
		dst = append(dst, Mark)
	}
	dst = f.AppendBody(dst)

	if f.Control.Scheme() == SchemeCRC16 {
		dst = append(dst, byte(f.Check), byte(f.Check>>8))
	} else {
		dst = append(dst, byte(f.Check))
	}
	return dst, nil
}

// AppendBody appends the octets the error check covers: start of message
// through the end of the data block, excluding any mark octet.
//
// It is exported because the secure channel authenticates exactly these octets
// -- header, security block, code and payload, with the length field already
// reporting the finished frame. A caller computing a message authentication
// code builds the frame with room for the code reserved at the end of Data,
// takes the body, and authenticates all but that reservation. Doing it any
// other way authenticates a length the receiver will never see.
//
// dst may be nil. f is not modified.
func (f Frame) AppendBody(dst []byte) []byte {
	total := f.Len()

	dst = append(dst, SOM)

	addr := byte(f.Address)
	if f.IsReply {
		addr |= replyFlag
	}
	dst = append(dst, addr, byte(total), byte(total>>8), byte(f.Control))

	if f.Security != nil {
		dst = append(dst, byte(f.Security.wireLen()), f.Security.Type)
		dst = append(dst, f.Security.Data...)
	}

	dst = append(dst, f.Code)
	return append(dst, f.Data...)
}

// ComputeCheck returns the error check the frame's octets call for, using the
// scheme the control octet declares.
func (f Frame) ComputeCheck() uint16 {
	body := f.AppendBody(make([]byte, 0, f.Len()))
	if f.Control.Scheme() == SchemeCRC16 {
		return CRC16(body)
	}
	return uint16(Checksum(body))
}

// Seal sets Check to the value the frame's octets call for. It is the last step
// before encoding a frame this process composed.
func (f *Frame) Seal() { f.Check = f.ComputeCheck() }

// CheckOK reports whether Check matches the octets it covers.
func (f Frame) CheckOK() bool { return f.Check == f.ComputeCheck() }
