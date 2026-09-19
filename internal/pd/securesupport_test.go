// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd_test

import (
	"testing"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/pd"
	"github.com/regalium-os/osdp-go/internal/secure"
)

// controller is the panel half of a Secure Channel.
//
// It is built on the same secure.Session the real bus uses, in RoleCP, and it
// composes its own frames rather than calling into anything the device shares.
// That is the point of it: the two ends of a secure channel must agree on
// exactly which octets the message authentication code covers, and an agreement
// between a function and itself proves nothing.
type controller struct {
	session *secure.Session
	seq     uint8
}

// testKey is an install-specific SCBK. It is not SCBK-D, so a device holding it
// reports itself secure rather than merely installable.
var testKey = secure.BaseKey{
	0xA0, 0xA1, 0xA2, 0xA3, 0xA4, 0xA5, 0xA6, 0xA7,
	0xA8, 0xA9, 0xAA, 0xAB, 0xAC, 0xAD, 0xAE, 0xAF,
}

// fixedNonce returns a constant random, which is exactly what a test wants and
// exactly what a deployment must not have: the whole handshake replays
// deterministically, and the failure of a cryptogram means the arithmetic
// changed rather than the entropy.
func fixedNonce(b byte) pd.NonceSource {
	return func() [8]byte { return [8]byte{b, b, b, b, b, b, b, b} }
}

// secureDevice builds a device that can establish a channel on key.
func secureDevice(t *testing.T, key secure.BaseKey, opts ...pd.Option) *pd.Device {
	t.Helper()

	base := []pd.Option{
		pd.WithIdentity(testIdentity),
		pd.WithContacts(2, 2, 1),
		pd.WithSecureChannel(standardSuite(t), key, fixedNonce(0xB0)),
	}
	return newDevice(t, append(base, opts...)...)
}

// standardSuite is the AES-128 suite the specification mandates, reached the
// way an application reaches it.
func standardSuite(t *testing.T) secure.CipherSuite {
	t.Helper()

	registry, err := secure.NewRegistry()
	if err != nil {
		t.Fatalf("secure.NewRegistry: %v", err)
	}
	return registry.Standard()
}

// newController returns a panel session holding key.
func newController(t *testing.T, key secure.BaseKey) *controller {
	t.Helper()
	return &controller{session: secure.NewSession(standardSuite(t), secure.RoleCP, key)}
}

// next advances the panel's 1, 2, 3 rotation.
func (c *controller) next() uint8 {
	if c.seq >= 3 {
		c.seq = 1
	} else {
		c.seq++
	}
	return c.seq
}

// handshake drives all four messages and leaves both ends established.
func (c *controller) handshake(t *testing.T, d *pd.Device) {
	t.Helper()

	chlng, err := c.session.BeginChallenge([8]byte{1, 2, 3, 4, 5, 6, 7, 8})
	if err != nil {
		t.Fatalf("BeginChallenge: %v", err)
	}

	ccrypt := answer(t, d, c.command(t, cmd.Message{Code: cmd.Chlng, Data: chlng}, secure.SCS11))
	if got := cmd.Code(ccrypt.Code); got != cmd.CCrypt {
		t.Fatalf("device answered the challenge with %s, want osdp_CCRYPT", got.Name(true))
	}

	scrypt, err := c.session.AnswerCryptogram(ccrypt.Data)
	if err != nil {
		t.Fatalf("the device's cryptogram did not verify: %v", err)
	}

	rmac := answer(t, d, c.command(t, cmd.Message{Code: cmd.SCrypt, Data: scrypt}, secure.SCS13))
	if got := cmd.Code(rmac.Code); got != cmd.RMACI {
		t.Fatalf("device answered the server cryptogram with %s, want osdp_RMAC_I", got.Name(true))
	}
	if err := c.session.CompleteHandshake(rmac.Data); err != nil {
		t.Fatalf("the device's initial R-MAC did not verify: %v", err)
	}
}
