// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package secure

// The handshake is four messages, alternating direction:
//
//	CP -> PD  osdp_CHLNG   SCS_11   RND.A
//	PD -> CP  osdp_CCRYPT  SCS_12   cUID || RND.B || client cryptogram
//	CP -> PD  osdp_SCRYPT  SCS_13   server cryptogram
//	PD -> CP  osdp_RMAC_I  SCS_14   initial R-MAC
//
// Each method below consumes the message it is given and returns the payload to
// send next, so a caller drives the exchange without knowing the layout.

// BeginChallenge starts the handshake and returns the osdp_CHLNG payload.
// Control panel only. rndA must come from a cryptographically secure source.
func (s *Session) BeginChallenge(rndA [8]byte) ([]byte, error) {
	if s.role != RoleCP {
		return nil, ErrWrongRole
	}
	s.rndA = rndA
	s.state = StateChallenged
	return append([]byte(nil), rndA[:]...), nil
}

// AnswerChallenge consumes osdp_CHLNG and returns the osdp_CCRYPT payload.
// Peripheral device only.
func (s *Session) AnswerChallenge(chlng []byte, rndB [8]byte, uid [8]byte) ([]byte, error) {
	if s.role != RolePD {
		return nil, ErrWrongRole
	}
	if len(chlng) != challengeSize {
		return nil, ErrMalformedHandshake
	}

	s.rndA, s.rndB, s.uid = [8]byte(chlng), rndB, uid

	keys, err := s.suite.DeriveSessionKeys(s.base, s.rndA)
	if err != nil {
		return nil, err
	}
	s.keys = keys

	client, err := s.suite.ClientCryptogram(keys, s.rndA, s.rndB)
	if err != nil {
		return nil, err
	}
	s.clientCryptogram = client
	s.state = StateCryptogram

	out := make([]byte, 0, ccryptSize)
	out = append(out, uid[:]...)
	out = append(out, rndB[:]...)
	return append(out, client[:]...), nil
}

// AnswerCryptogram consumes osdp_CCRYPT, verifies the device's proof, and
// returns the osdp_SCRYPT payload. Control panel only.
//
// A mismatch tears the session down and returns ErrCryptogramMismatch: the peer
// does not hold the base key, and nothing further should be exchanged with it.
func (s *Session) AnswerCryptogram(ccrypt []byte) ([]byte, error) {
	if s.role != RoleCP {
		return nil, ErrWrongRole
	}
	if s.state != StateChallenged || len(ccrypt) != ccryptSize {
		return nil, ErrMalformedHandshake
	}

	s.uid = [uidSize]byte(ccrypt[:uidSize])
	s.rndB = [8]byte(ccrypt[uidSize : uidSize+8])
	claimed := ccrypt[uidSize+8:]

	keys, err := s.suite.DeriveSessionKeys(s.base, s.rndA)
	if err != nil {
		return nil, err
	}
	s.keys = keys

	expected, err := s.suite.ClientCryptogram(keys, s.rndA, s.rndB)
	if err != nil {
		return nil, err
	}
	if !equalCT(claimed, expected[:]) {
		return nil, s.fail()
	}
	s.clientCryptogram = expected

	server, err := s.suite.ServerCryptogram(keys, s.rndA, s.rndB)
	if err != nil {
		return nil, err
	}
	s.serverCryptogram = server
	s.state = StateCryptogram
	return append([]byte(nil), server[:]...), nil
}

// AnswerServerCryptogram consumes osdp_SCRYPT, verifies the panel's proof, and
// returns the osdp_RMAC_I payload. Peripheral device only.
//
// On success the session is established and both MAC chains are seeded.
func (s *Session) AnswerServerCryptogram(scrypt []byte) ([]byte, error) {
	if s.role != RolePD {
		return nil, ErrWrongRole
	}
	if s.state != StateCryptogram || len(scrypt) != BlockSize {
		return nil, ErrMalformedHandshake
	}

	expected, err := s.suite.ServerCryptogram(s.keys, s.rndA, s.rndB)
	if err != nil {
		return nil, err
	}
	if !equalCT(scrypt, expected[:]) {
		return nil, s.fail()
	}
	s.serverCryptogram = expected

	rmac, err := s.suite.InitialRMAC(s.keys, expected)
	if err != nil {
		return nil, err
	}
	// Only the reply chain is seeded. The command chain stays zero and is
	// first written by the command that opens the session -- a reply is always
	// preceded by the command that prompted it, so the zero is never read.
	// Seeding both to the same value would make a command and a reply over
	// identical octets authenticate identically until the first exchange.
	s.rmac = rmac
	s.state = StateEstablished
	return append([]byte(nil), rmac[:]...), nil
}

// CompleteHandshake consumes osdp_RMAC_I and establishes the session.
// Control panel only.
func (s *Session) CompleteHandshake(rmacI []byte) error {
	if s.role != RoleCP {
		return ErrWrongRole
	}
	if s.state != StateCryptogram || len(rmacI) != BlockSize {
		return ErrMalformedHandshake
	}

	expected, err := s.suite.InitialRMAC(s.keys, s.serverCryptogram)
	if err != nil {
		return err
	}
	// The device echoes a value the panel can compute independently, so a
	// mismatch means the two disagree about the session and it cannot proceed.
	if !equalCT(rmacI, expected[:]) {
		return s.fail()
	}

	// See AnswerServerCryptogram: the command chain is deliberately left zero.
	s.rmac = expected
	s.state = StateEstablished
	return nil
}
