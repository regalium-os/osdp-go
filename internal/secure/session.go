// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package secure

import "crypto/subtle"

// Role is which end of the bus this session speaks for.
type Role uint8

const (
	// RoleCP is the control panel: it issues the challenge and drives the
	// handshake.
	RoleCP Role = iota
	// RolePD is a peripheral device: it answers.
	RolePD
)

// State is the secure channel session state, SIA OSDP v2.2.2 §7.
type State uint8

const (
	// StateIdle is a session that has not begun.
	StateIdle State = iota
	// StateChallenged means the challenge has been sent or received.
	StateChallenged
	// StateCryptogram means the client cryptogram has been exchanged.
	StateCryptogram
	// StateEstablished means both sides have proved possession of the base key
	// and the MAC chains are seeded.
	StateEstablished
	// StateFailed is terminal. A session that fails authentication is never
	// resumed; the panel rebuilds it from the challenge.
	StateFailed
)

// String implements fmt.Stringer and supplies the span attribute value.
func (s State) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateChallenged:
		return "challenged"
	case StateCryptogram:
		return "cryptogram"
	case StateEstablished:
		return "established"
	default:
		return "failed"
	}
}

// Payload sizes on the wire.
const (
	challengeSize = 8                       // RND.A
	uidSize       = 8                       // cUID
	ccryptSize    = uidSize + 8 + BlockSize // cUID || RND.B || client cryptogram
)

// Session drives one secure channel: the handshake, then the two MAC chains
// that authenticate every message afterwards.
//
// A Session is pure. It holds no clock, opens no sockets, and never generates
// randomness -- RND.A and RND.B are supplied by the caller, so a full handshake
// replays deterministically in a test and the entropy source stays the
// application's choice.
//
// A Session is not safe for concurrent use. One bus transaction runs at a time
// per device by the nature of the protocol, so this costs nothing.
type Session struct {
	suite CipherSuite
	role  Role
	base  BaseKey
	state State

	keys SessionKeys
	rndA [8]byte
	rndB [8]byte
	uid  [uidSize]byte

	// cmac and rmac are the running chains. A command authenticates from the
	// last rmac and updates cmac; a reply does the reverse. Keeping them
	// separate is what stops a captured command being replayed as a reply.
	cmac [BlockSize]byte
	rmac [BlockSize]byte

	clientCryptogram [BlockSize]byte
	serverCryptogram [BlockSize]byte
}

// NewSession returns an idle session. suite must not be nil; use
// Registry.Standard when in doubt, which is what every third-party reader
// expects.
func NewSession(suite CipherSuite, role Role, base BaseKey) *Session {
	return &Session{suite: suite, role: role, base: base}
}

// State reports the session state.
func (s *Session) State() State { return s.state }

// Established reports whether messages may now be sealed and authenticated.
func (s *Session) Established() bool { return s.state == StateEstablished }

// UsingDefaultKey reports whether this session runs on SCBK-D.
//
// Such a session is authenticated against a key printed in the specification,
// which is to say not authenticated at all. It exists to install a real key.
// A deployment should surface this and refuse to leave a device in that state.
func (s *Session) UsingDefaultKey() bool { return s.base.IsDefault() }

// Teardown returns the session to idle and zeroes the derived key material.
//
// Call it on any authentication failure. Continuing after a failed MAC means
// accepting a message that something on the line altered.
func (s *Session) Teardown() {
	s.keys.Zero()
	clear(s.cmac[:])
	clear(s.rmac[:])
	clear(s.rndA[:])
	clear(s.rndB[:])
	s.state = StateIdle
}

// fail marks the session unusable and wipes what it held.
func (s *Session) fail() error {
	s.Teardown()
	s.state = StateFailed
	return ErrCryptogramMismatch
}

// equalCT compares two byte slices in constant time.
func equalCT(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}
