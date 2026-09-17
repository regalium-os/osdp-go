// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package cmd

import "errors"

// Codec errors, matched with errors.Is.
var (
	// ErrShortPayload reports a payload too short for the structure its code
	// implies. On a shared line this is ordinary corruption, not a bug.
	ErrShortPayload = errors.New("osdp/cmd: payload shorter than the code requires")

	// ErrUnknownCode reports a code outside the specification's ranges.
	ErrUnknownCode = errors.New("osdp/cmd: unrecognised command or reply code")
)

// NAKReason is the error code carried by an osdp_NAK reply.
type NAKReason byte

// NAK reasons, SIA OSDP v2.2.2 §6.3.
const (
	NAKCheckFailed      NAKReason = 0x01 // bad checksum or CRC
	NAKCommandLength    NAKReason = 0x02 // command length error
	NAKUnknownCommand   NAKReason = 0x03 // command not implemented
	NAKSequenceError    NAKReason = 0x04 // unexpected sequence number
	NAKSecureRequired   NAKReason = 0x05 // this command requires a secure channel
	NAKEncryptionUnsup  NAKReason = 0x06 // encryption not supported by the device
	NAKBioTypeUnsup     NAKReason = 0x07
	NAKBioFormatUnsup   NAKReason = 0x08
	NAKUnsupportedInput NAKReason = 0x09
)

// String returns a human-readable reason.
func (r NAKReason) String() string {
	switch r {
	case NAKCheckFailed:
		return "error check failed"
	case NAKCommandLength:
		return "command length error"
	case NAKUnknownCommand:
		return "command not implemented"
	case NAKSequenceError:
		return "unexpected sequence number"
	case NAKSecureRequired:
		return "secure channel required"
	case NAKEncryptionUnsup:
		return "encryption not supported"
	case NAKBioTypeUnsup:
		return "biometric type not supported"
	case NAKBioFormatUnsup:
		return "biometric format not supported"
	case NAKUnsupportedInput:
		return "unsupported input value"
	default:
		return "unknown NAK reason"
	}
}

// ParseNAK decodes an osdp_NAK payload.
func ParseNAK(data []byte) (NAKReason, error) {
	if len(data) < 1 {
		return 0, ErrShortPayload
	}
	return NAKReason(data[0]), nil
}
