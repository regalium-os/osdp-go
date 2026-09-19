// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd

import (
	"context"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/internal/secure"
)

// The device's half of the four-message handshake, SIA OSDP v2.2.2 §7.2:
//
//	CP -> PD  osdp_CHLNG   SCS_11   RND.A
//	PD -> CP  osdp_CCRYPT  SCS_12   cUID || RND.B || client cryptogram
//	CP -> PD  osdp_SCRYPT  SCS_13   server cryptogram
//	PD -> CP  osdp_RMAC_I  SCS_14   initial R-MAC
//
// Every octet of key material, cryptogram and chain is secure's. What lives
// here is which message is due, which block carries it, and what a failure at
// each step leaves behind.

// answerSecure routes a command that arrived inside a security block.
//
// The lock must be held.
func (d *Device) answerSecure(ctx context.Context, f frame.Frame) (cmd.Message, *frame.SecurityBlock) {
	if !d.secureEnabled() {
		// The capability report already said compliance zero, so a panel
		// reaching here was configured by hand or is talking to the wrong
		// address. Either way it learns at once rather than on a timeout.
		return nak(cmd.NAKEncryptionUnsup), nil
	}

	switch secure.BlockType(f.Security.Type) {
	case secure.SCS11:
		return d.answerChallenge(f.Data)
	case secure.SCS13:
		return d.answerCryptogram(f.Data)
	case secure.SCS15, secure.SCS17:
		return d.answerEstablished(ctx, f)
	default:
		// SCS_12, SCS_14, SCS_16 and SCS_18 travel the other way: a command
		// carrying one is a panel echoing a device's own block back, which no
		// correct implementation does.
		return nak(cmd.NAKEncryptionUnsup), nil
	}
}

// answerChallenge consumes osdp_CHLNG and returns osdp_CCRYPT under SCS_12.
//
// A challenge always starts a fresh session, even when one is established. That
// is not laxity: RND.A is new, so every session key derived from it is new, and
// a panel that re-challenges has already discarded whatever it held. Keeping
// the old session would leave this end authenticating against a chain the other
// end has forgotten.
//
// chlng is read and not retained.
//
// The lock must be held.
func (d *Device) answerChallenge(chlng []byte) (cmd.Message, *frame.SecurityBlock) {
	d.dropSession()

	session := secure.NewSession(d.cfg.suite, secure.RolePD, d.key)
	ccrypt, err := session.AnswerChallenge(chlng, d.cfg.nonce(), deviceUID(d.cfg.identity))
	if err != nil {
		// Only a malformed challenge reaches here -- a payload that is not the
		// eight octets of RND.A. It is a length error and is reported as one,
		// because a panel retrying the same malformed frame forever is the
		// alternative.
		session.Teardown()
		return nak(cmd.NAKCommandLength), nil
	}

	d.session = session
	return cmd.Message{Code: cmd.CCrypt, Data: ccrypt},
		&frame.SecurityBlock{Type: byte(secure.SCS12)}
}

// answerCryptogram consumes osdp_SCRYPT and returns osdp_RMAC_I under SCS_14.
//
// This is where the panel proves it holds the base key. A mismatch is not line
// noise and is not retried: the two ends disagree about the key, which is a
// commissioning fault, and the session is abandoned rather than resynchronised.
// The refusal travels in the clear because there is no session to seal it with.
//
// scrypt is read and not retained.
//
// The lock must be held.
func (d *Device) answerCryptogram(scrypt []byte) (cmd.Message, *frame.SecurityBlock) {
	if d.session == nil {
		// A server cryptogram for a challenge this device never answered. The
		// panel is a step ahead of us -- its osdp_CCRYPT was lost, or this
		// device restarted mid-handshake -- so it is told to begin again.
		return nak(cmd.NAKSecureRequired), nil
	}

	rmac, err := d.session.AnswerServerCryptogram(scrypt)
	if err != nil {
		d.dropSession()
		return nak(cmd.NAKSecureRequired), nil
	}

	return cmd.Message{Code: cmd.RMACI, Data: rmac},
		&frame.SecurityBlock{Type: byte(secure.SCS14)}
}

// answerEstablished handles a command on a running session: verify it,
// decipher it if it was enciphered, dispatch it, and answer in kind.
//
// # Why the plaintext dispatch table is reused unchanged
//
// Once the payload is open, a command is a command. Giving the secure path its
// own table would be two implementations of what osdp_OUT means, and the second
// one is where the divergence lives. The deciphered payload is handed to the
// same switch the plaintext path uses, with secured set so that osdp_KEYSET --
// the one command whose meaning genuinely depends on how it arrived -- can tell
// the difference.
//
// The lock must be held.
func (d *Device) answerEstablished(ctx context.Context, f frame.Frame) (cmd.Message, *frame.SecurityBlock) {
	if d.session == nil || !d.session.Established() {
		return nak(cmd.NAKSecureRequired), nil
	}

	payload, err := d.open(f)
	if err != nil {
		// A frame that fails its message authentication code is a frame
		// something on the line altered. Per SIA OSDP v2.2.2 §7.5 the response
		// is to abandon the session, not to resynchronise inside it -- so the
		// session goes, and reason 0x05 tells the panel exactly what is now
		// true: this device needs a secure channel and no longer has one.
		d.dropSession()
		return nak(cmd.NAKSecureRequired), nil
	}

	m := d.dispatch(ctx, frame.Frame{Code: f.Code, Data: payload}, true)
	return m, replyBlock(len(m.Data))
}
