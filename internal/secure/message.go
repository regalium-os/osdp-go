// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package secure

// MACTagSize is how much of the message authentication code travels on the
// wire: the first four octets of the sixteen computed.
//
// Four octets is what the specification allocates, and it is worth being clear
// about what that buys. A blind forgery succeeds with probability 2^-32, and an
// attacker who can inject frames onto an RS-485 line gets to keep trying. The
// defence is the chain: every MAC depends on the previous one, so a forgery
// must be produced live, against a chain value that has never been transmitted,
// and one failure tears the session down. Truncation is a protocol constant
// here, not a choice this package makes.
const MACTagSize = 4

// chainFor returns the MAC a message in this direction authenticates from.
//
// A command chains from the last reply MAC and a reply from the last command
// MAC. The two directions advancing separately is what stops a captured command
// being replayed back as a reply.
func (s *Session) chainFor(isCommand bool) [BlockSize]byte {
	if isCommand {
		return s.rmac
	}
	return s.cmac
}

// commit advances the chain for this direction. It is called only after the
// message has been accepted.
func (s *Session) commit(isCommand bool, mac [BlockSize]byte) {
	if isCommand {
		s.cmac = mac
		return
	}
	s.rmac = mac
}

// Seal enciphers a payload for an established session, for blocks SCS_17 and
// SCS_18. It does not advance the MAC chain; Authenticate does that, and must
// be called afterwards over the assembled frame.
//
// plaintext is read and not retained.
func (s *Session) Seal(plaintext []byte, isCommand bool) ([]byte, error) {
	if !s.Established() {
		return nil, ErrNotEstablished
	}
	return s.suite.Seal(s.keys, s.chainFor(isCommand), plaintext)
}

// Open deciphers a payload. Verify the frame's MAC first: this returns
// plaintext without having authenticated anything, because AES-CBC is malleable
// and an unverified payload is attacker-controlled.
func (s *Session) Open(ciphertext []byte, isCommand bool) ([]byte, error) {
	if !s.Established() {
		return nil, ErrNotEstablished
	}
	return s.suite.Open(s.keys, s.chainFor(isCommand), ciphertext)
}

// Authenticate computes the wire tag over the assembled frame octets and
// advances the chain.
//
// frame must be everything the MAC covers: the header, the security block, the
// command or reply code and the payload, with any ciphertext already in place.
// The specification authenticates the enciphered form, not the plaintext.
func (s *Session) Authenticate(frame []byte, isCommand bool) ([MACTagSize]byte, error) {
	var tag [MACTagSize]byte
	if !s.Established() {
		return tag, ErrNotEstablished
	}

	mac, err := s.suite.MAC(s.keys, s.chainFor(isCommand), frame)
	if err != nil {
		return tag, err
	}
	s.commit(isCommand, mac)
	return [MACTagSize]byte(mac[:MACTagSize]), nil
}

// Verify checks a received frame's tag and advances the chain on success.
//
// On failure the chain is left untouched and ErrMACMismatch is returned. The
// caller must tear the session down: a frame that fails authentication means
// something on the line altered it, and the specification's response is to
// abandon the session rather than resynchronise within it.
//
// The comparison is constant time.
func (s *Session) Verify(frame []byte, tag []byte, isCommand bool) error {
	if !s.Established() {
		return ErrNotEstablished
	}
	if len(tag) != MACTagSize {
		return ErrMACMismatch
	}

	mac, err := s.suite.MAC(s.keys, s.chainFor(isCommand), frame)
	if err != nil {
		return err
	}
	if !equalCT(tag, mac[:MACTagSize]) {
		return ErrMACMismatch
	}

	s.commit(isCommand, mac)
	return nil
}
