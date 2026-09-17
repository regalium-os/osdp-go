// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package secure

import "crypto/subtle"

// BlockSize is the AES block size, and the size of every key and MAC here.
const BlockSize = 16

// BaseKey is the secure channel base key (SCBK) shared with one device.
//
// It is passed by value so that no part of this package holds a reference to
// the caller's key material; the caller owns it and can zero it when the device
// is decommissioned.
type BaseKey [BlockSize]byte

// DefaultBaseKey is SCBK-D, the well-known key the specification defines for a
// device that has not yet been given an install-specific one.
//
// It is public knowledge and provides no confidentiality whatsoever. It exists
// so a panel can reach a factory-fresh reader long enough to install a real
// key. A session running on this key is not secure, and Session reports it as
// such so that a deployment can refuse to leave devices in that state.
var DefaultBaseKey = BaseKey{
	0x30, 0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37,
	0x38, 0x39, 0x3A, 0x3B, 0x3C, 0x3D, 0x3E, 0x3F,
}

// IsDefault reports whether k is the well-known SCBK-D.
//
// The comparison is constant time. It is not a secret-dependent branch in any
// meaningful sense, but making key comparisons constant time by default removes
// the need for anyone to decide which ones matter.
func (k BaseKey) IsDefault() bool {
	return subtle.ConstantTimeCompare(k[:], DefaultBaseKey[:]) == 1
}

// SessionKeys are the three keys derived for one secure channel session: one
// for confidentiality and two for the message authentication chain.
//
// They are derived fresh from the base key and the panel's random on every
// handshake, so a session key never outlives its session.
type SessionKeys struct {
	// Enc enciphers payloads (S-ENC).
	Enc [BlockSize]byte
	// MAC1 covers every block of a message but the last (S-MAC1).
	MAC1 [BlockSize]byte
	// MAC2 covers the final block, producing the MAC (S-MAC2).
	MAC2 [BlockSize]byte
}

// Zero overwrites the session keys.
//
// Go gives no guarantee that this is not optimised away, and a garbage
// collector may have copied the struct already. It is defence in depth, not a
// guarantee: it shortens the window in which a core dump or a swapped page
// yields a live session key, and costs nothing.
func (k *SessionKeys) Zero() {
	clear(k.Enc[:])
	clear(k.MAC1[:])
	clear(k.MAC2[:])
}
