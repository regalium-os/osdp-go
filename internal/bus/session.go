// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package bus

import "github.com/regalium-os/osdp-go/internal/secure"

// A device's secure channel, as the bus holds it: what a caller may ask about
// it, and how it is given up.
//
// The session itself is never handed out. Its MAC chains are synchronised with
// a physical device on the other end of a wire, so a caller that advanced one
// would break the link it was inspecting -- and the failure would show up as
// every subsequent frame failing authentication, a long way from the cause.

// SecureSession reports the state of this device's secure channel, and whether
// there is one at all. The session itself stays private: its MAC chains are
// synchronised with a physical device, and a caller advancing them would break
// the link it was trying to inspect.
func (d *Device) SecureSession() (secure.State, bool) {
	if d.session == nil {
		return secure.StateIdle, false
	}
	return d.session.State(), true
}

// UsesDefaultKey reports whether this device's secure channel runs on SCBK-D,
// the key printed in the specification. Such a session is authenticated against
// public knowledge, which is to say not authenticated at all.
func (d *Device) UsesDefaultKey() bool {
	return d.session != nil && d.session.UsingDefaultKey()
}

// dropSession abandons the secure channel, leaving it open to be rebuilt.
//
// The specification's response to a failed authentication is to abandon the
// session rather than to resynchronise within it: a frame that fails its MAC is
// a frame something on the line altered, and there is no way to tell how much
// of the exchange that something saw.
//
// Rebuilding is allowed here because a MAC failure on an established session
// says nothing about the key -- the handshake already proved both ends hold it,
// so the likeliest cause is the line rather than the configuration.
func (d *Device) dropSession() {
	if d.session != nil {
		d.session.Teardown()
		d.session = nil
	}
	d.pending = nil
}

// abandonSecure drops the session and stops this device being offered another.
//
// Used when the peer has shown it cannot complete a handshake: a cryptogram
// mismatch means it does not hold the base key, and no number of retries will
// change that. The application hears KindSecureFailed and decides whether a
// device it cannot authenticate belongs on the bus at all.
func (d *Device) abandonSecure() {
	d.dropSession()
	d.secureDeclined = true
}
