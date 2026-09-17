// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

// Package osdp implements the SIA OSDP v2.2.2 / IEC 60839-11-5 access-control
// device protocol.
//
// # The public surface is deliberate
//
// The protocol layers live under internal/: framing, the command codec, secure
// channel, the poll-cycle state machine, the transports and the vendor
// providers. None of them is importable from outside this module. What a
// consumer can reach is exactly what this package re-exports, which means the
// internals stay free to change without breaking anyone.
//
// Re-export is by type alias, not by wrapper type. An alias is the same type,
// so a value obtained here satisfies interfaces declared internally and vice
// versa -- which is what keeps the extension points open. A third party can
// implement a Transport for a proprietary serial bridge, or a CipherSuite
// beside the mandated AES-128 one, without this repository being involved.
//
// # Observability
//
// Every layer boundary opens a span. Wire a tracer once and it applies to the
// whole stack:
//
//	ctx = telemetry.ContextWithTracer(ctx, telemetry.Bind(p.Tracing.Start))
//
// See the telemetry package, which is the one other package a consumer imports
// directly.
package osdp

import (
	"context"

	"github.com/regalium-os/osdp-go/internal/frame"
)

// Frame-level types. These are the wire itself, exposed because a control panel
// integrator debugging a misbehaving reader needs to see what actually arrived,
// not a normalised summary of it.
type (
	// Frame is one decoded OSDP frame. Decoding and re-encoding reproduces the
	// input octet for octet, a wrong error check included.
	Frame = frame.Frame

	// Address is a peripheral device address, 0x00 to 0x7E, or Broadcast.
	Address = frame.Address

	// Control is the control octet: sequence, check scheme, security flag.
	Control = frame.Control

	// Scheme selects the trailing error check, CRC-16 or the legacy checksum.
	Scheme = frame.Scheme

	// SecurityBlock is the optional block preceding the command or reply code.
	SecurityBlock = frame.SecurityBlock
)

// Wire constants.
const (
	// SOM is the start-of-message octet.
	SOM = frame.SOM

	// Mark is the optional synchronisation octet some devices send before SOM.
	Mark = frame.Mark

	// Broadcast is the configuration address every device answers.
	Broadcast = frame.BroadcastAddress

	// SchemeCRC16 is the CRC-16/AUG-CCITT check every current device supports.
	SchemeCRC16 = frame.SchemeCRC16

	// SchemeChecksum is the single-octet check retained for older devices.
	SchemeChecksum = frame.SchemeChecksum
)

// Decode parses one frame from the front of buf.
//
// The returned Frame aliases buf and nothing is copied; call Frame.Clone to
// retain it past the next write into buf. A frame whose error check fails is
// returned populated alongside ErrBadCheck, because corrupt traffic is the
// evidence an operator most needs.
func Decode(ctx context.Context, buf []byte) (Frame, error) {
	return frame.Decode(ctx, buf)
}

// NewControl builds a control octet with reserved bits clear.
func NewControl(seq uint8, scheme Scheme, security bool) Control {
	return frame.NewControl(seq, scheme, security)
}

// CRC16 computes the OSDP CRC-16/AUG-CCITT over b: polynomial 0x1021, initial
// value 0x1D0F, neither input nor output reflected.
func CRC16(b []byte) uint16 { return frame.CRC16(b) }

// Checksum computes the OSDP single-octet checksum over b.
func Checksum(b []byte) uint8 { return frame.Checksum(b) }

// Decoding errors, matched with errors.Is.
//
// These are vars because that is how Go sentinel errors work; they are the one
// exception to this repository's rule against package-level state, and must
// never be reassigned.
var (
	// ErrShortBuffer means the octets end before the frame does: wait for
	// more, rather than resynchronise.
	ErrShortBuffer = frame.ErrShortBuffer

	// ErrNoStartOfMessage means the buffer does not begin a frame.
	ErrNoStartOfMessage = frame.ErrNoStartOfMessage

	// ErrLengthMismatch means the declared length is impossible.
	ErrLengthMismatch = frame.ErrLengthMismatch

	// ErrBadCheck means the trailing CRC or checksum does not match. A
	// populated Frame is returned with it.
	ErrBadCheck = frame.ErrBadCheck

	// ErrBadSecurityBlock means the security block does not fit the frame.
	ErrBadSecurityBlock = frame.ErrBadSecurityBlock

	// ErrInvalidAddress means an address above Broadcast was supplied.
	ErrInvalidAddress = frame.ErrInvalidAddress
)
