// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package frame

import "errors"

// Decoding errors.
//
// All are matched with errors.Is. A caller distinguishing a truncated read from
// a corrupt frame must never have to match on message text, because the two
// demand different responses: the first is "wait for more octets", the second
// is "resynchronise the line".
var (
	// ErrShortBuffer reports that the octets end before the frame does. It is
	// the normal condition on a byte stream and means wait for more, not fail.
	ErrShortBuffer = errors.New("osdp/frame: buffer shorter than the frame requires")

	// ErrNoStartOfMessage reports that the buffer does not begin with the
	// start-of-message octet, or with a mark octet followed by one.
	ErrNoStartOfMessage = errors.New("osdp/frame: no start-of-message octet")

	// ErrLengthMismatch reports that the declared length is impossible: it is
	// smaller than the header it sits in, or it disagrees with the buffer.
	ErrLengthMismatch = errors.New("osdp/frame: declared length is inconsistent")

	// ErrBadCheck reports that the trailing CRC or checksum does not match the
	// octets it covers.
	//
	// Decode returns a fully populated Frame alongside this error, so a caller
	// may inspect what arrived. Traffic on a shared line is noisy, and a frame
	// that fails its check is evidence, not something to discard unseen.
	ErrBadCheck = errors.New("osdp/frame: error check does not match")

	// ErrBadSecurityBlock reports a security block whose declared length does
	// not fit within the frame.
	ErrBadSecurityBlock = errors.New("osdp/frame: malformed security block")

	// ErrInvalidAddress reports an address above the broadcast address, which
	// cannot be represented in the seven bits the wire gives it.
	ErrInvalidAddress = errors.New("osdp/frame: address out of range")
)
