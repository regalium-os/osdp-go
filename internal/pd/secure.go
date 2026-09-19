// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd

import (
	"context"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/internal/secure"
)

// NonceSource supplies RND.B, the device's random for one handshake.
//
// It must come from a cryptographically secure source. RND.B is half the input
// to the two cryptograms, and a device whose random is predictable can be
// impersonated by anyone who has ever recorded one of its sessions -- which on
// a wall-mounted reader is anyone with a screwdriver and an afternoon.
//
// It is injected rather than read from crypto/rand here for the same reason the
// panel injects RND.A: it keeps the entropy source the application's choice,
// and it lets a whole handshake replay deterministically in a test.
//
// It is called once per handshake attempt, with the device's lock held, and
// must be safe to call repeatedly. Holding the lock means a source that blocks
// blocks the whole device: read from a pool rather than from a syscall if the
// platform's entropy can stall.
type NonceSource func() [8]byte

// KeyInstalled is called when osdp_KEYSET installs a new base key.
//
// Persist it. A device whose installed key was never written down is a device
// nobody can talk to: the panel will offer the new key on its next handshake,
// and a reader that came back up holding the old one cannot answer. There is no
// command to ask a device what key it has.
//
// It is called with the device's lock held and must not call back into the
// Device. The key is passed by value, so this package holds no reference to
// what the callback does with it.
type KeyInstalled func(ctx context.Context, key secure.BaseKey)

// WithSecureChannel makes the device able to establish a Secure Channel.
//
// Without it the device is a plaintext reader: it reports
// osdp_CAP_COMMUNICATION_SECURITY at compliance zero and refuses an osdp_CHLNG
// with osdp_NAK reason 0x06. That is a complete and common device, and it stays
// the default because a device that claimed AES-128 without a key to back it
// would be challenged on every cycle and refuse every time.
//
// suite is normally secure.Registry.Standard -- the AES-128 suite of SIA OSDP
// v2.2.2 §7, which is what every reader in the field speaks. key is this
// device's own SCBK; the specification gives each device its own, and a
// deployment sharing one across a line has built a single point of compromise.
//
// A device holding secure.DefaultBaseKey reports the default-key bit in its
// capability report, so a panel can tell an installable reader from a secure
// one. SCBK-D is printed in the specification and secures nothing; it exists so
// that a real key can be installed over it.
//
// All three arguments are required. Any of them nil, and the option does
// nothing -- a half-configured secure channel is one that fails at the
// challenge rather than at construction, which is the harder fault to find.
func WithSecureChannel(suite secure.CipherSuite, key secure.BaseKey, nonce NonceSource) Option {
	return func(s *settings) {
		if suite == nil || nonce == nil {
			return
		}
		s.suite, s.key, s.nonce, s.secure = suite, key, nonce, true
	}
}

// WithKeyInstalled supplies the callback invoked when osdp_KEYSET installs a
// new base key. See KeyInstalled for why persisting it is not optional.
func WithKeyInstalled(fn KeyInstalled) Option {
	return func(s *settings) {
		if fn != nil {
			s.keyInstalled = fn
		}
	}
}

// Secure reports whether a Secure Channel session is currently established.
//
// It is the device's own view, which is the only honest one: the panel may
// believe a session is up for one further exchange after this end has torn it
// down, because a teardown is not announced.
func (d *Device) Secure() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.session != nil && d.session.Established()
}

// dropSession abandons any session and wipes the key material it derived.
//
// It is called on every path that invalidates a session: a restart at sequence
// zero, a communication timeout, a failed message authentication code, and a
// key install. Per SIA OSDP v2.2.2 §7.5 a session does not survive any of
// those, and continuing to authenticate against a chain only one end still
// believes in would fail every frame from here on.
//
// The lock must be held.
func (d *Device) dropSession() {
	if d.session != nil {
		d.session.Teardown()
		d.session = nil
	}
}

// secureEnabled reports whether the device was given a suite, a key and a
// source of randomness. The lock must be held.
func (d *Device) secureEnabled() bool { return d.cfg.secure }

// deviceUID is the cUID a device identifies itself with in osdp_CCRYPT.
//
// Per SIA OSDP v2.2.2 §7.2 it is eight octets drawn from the device's own
// osdp_PDID: the three-octet vendor code, the model, the version, and the low
// three octets of the serial number. It is not secret and takes no part in key
// derivation -- only RND.A does -- but it is how a panel recognises which
// physical unit answered a challenge, so a device that invented one would be
// indistinguishable from a different reader on the same key.
//
// The serial number is truncated rather than hashed because the specification
// says so, which means two units of the same model whose serials differ only
// above the low three octets share a cUID. That is the specification's problem
// and not one this package may fix unilaterally.
func deviceUID(id cmd.DeviceID) [8]byte {
	return [8]byte{
		id.VendorCode[0], id.VendorCode[1], id.VendorCode[2],
		id.Model, id.Version,
		byte(id.Serial), byte(id.Serial >> 8), byte(id.Serial >> 16),
	}
}

// replyBlock chooses the security block an established session's reply travels
// under, mirroring what the panel does for a command.
//
// A reply with a payload is enciphered (SCS_18); one without is authenticated
// only (SCS_16), because there is nothing to encipher and padding an empty
// payload would put a whole block of cipher on the wire for nothing. The
// osdp_ACK that answers most polls is the second case -- and an osdp_RAW
// carrying a credential is always the first.
func replyBlock(payload int) *frame.SecurityBlock {
	if payload > 0 {
		return &frame.SecurityBlock{Type: byte(secure.SCS18)}
	}
	return &frame.SecurityBlock{Type: byte(secure.SCS16)}
}
