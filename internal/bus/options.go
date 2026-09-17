// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package bus

import (
	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/internal/secure"
)

// NonceSource supplies RND.A, the control panel's random for one handshake.
//
// It must come from a cryptographically secure source: RND.A is half the input
// to session key derivation, and a predictable one lets anyone who has ever
// seen the base key replay a session. It is injected rather than read from
// crypto/rand here because the core computes and does not act -- which is also
// what lets a whole handshake replay deterministically in a test.
//
// It is called once per handshake attempt and must be safe to call repeatedly.
type NonceSource func() [8]byte

// KeyFor returns the secure channel base key for a device, and whether one is
// configured.
//
// Keys are per device, not per bus: the specification gives each reader its own
// SCBK, and a deployment that shares one key across a line has built a single
// point of compromise. Returning false means this device is not offered a
// secure channel at all, which is the right answer for a reader that has not
// been commissioned yet.
//
// The key is returned by value, so the bus never holds a reference to the
// caller's key material.
type KeyFor func(frame.Address) (secure.BaseKey, bool)

// Option configures a Bus at construction.
//
// Options exist so that adding a capability never breaks a caller: New keeps
// its three required arguments, and everything optional arrives this way.
type Option func(*Bus)

// WithSecureChannel makes the bus establish a Secure Channel with any device
// that both claims AES-128 in its osdp_CAP reply and has a key from keys.
//
// Without this option no device is ever offered a secure channel and the bus
// behaves exactly as it did before: enrolment ends at Online and traffic is
// plaintext. That default is deliberate. A panel that silently began encrypting
// would strand every device whose key the application had not yet loaded.
//
// suite is normally Registry.Standard. A device is asked for nothing the
// capability report did not offer, so a reader that does not implement Secure
// Channel is never interrupted by a challenge it would have to NAK.
func WithSecureChannel(suite secure.CipherSuite, keys KeyFor, nonce NonceSource) Option {
	return func(b *Bus) {
		b.suite, b.keys, b.nonce = suite, keys, nonce
	}
}

// secureEnabled reports whether the bus was configured to attempt a handshake.
func (b *Bus) secureEnabled() bool {
	return b.suite != nil && b.keys != nil && b.nonce != nil
}
