// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd_test

import (
	"context"
	"testing"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/pd"
	"github.com/regalium-os/osdp-go/internal/secure"
)

// osdp_KEYSET: commissioning a reader off the key printed in the specification
// and onto one only this site holds. SIA OSDP v2.2.2 §6.16.

// TestKeysetCommissionsADeviceOffTheDefaultKey.
//
// The whole sequence, because no step of it is meaningful alone: a reader out
// of the box speaks on SCBK-D, is given a real key inside the channel that key
// secures, and afterwards answers to the new key and only the new key.
func TestKeysetCommissionsADeviceOffTheDefaultKey(t *testing.T) {
	var installed []secure.BaseKey
	d := secureDevice(t, secure.DefaultBaseKey, pd.WithKeyInstalled(
		func(_ context.Context, key secure.BaseKey) {
			installed = append(installed, key)
		},
	))

	c := newController(t, secure.DefaultBaseKey)
	c.handshake(t, d)

	if _, defaulted := d.Capabilities().SecureChannel(); !defaulted {
		t.Fatal("a device on SCBK-D does not report the default-key bit")
	}

	// The acknowledgement is sealed with the session the panel still holds. A
	// device that switched keys before answering would authenticate its own ACK
	// with a key the panel has not adopted, and the commissioning would fail at
	// the last step with the key already installed at one end.
	reply := answer(t, d, c.send(t, cmd.KeySetCommand(testKey[:])))
	replyCode(t, reply, cmd.ACK)
	c.open(t, reply)

	if len(installed) != 1 || installed[0] != testKey {
		t.Fatalf("the key install callback saw %d keys, want the new one once", len(installed))
	}
	if d.Secure() {
		t.Error("the session survived a key install; both ends must derive " +
			"fresh session keys from the new base key")
	}
	if _, defaulted := d.Capabilities().SecureChannel(); defaulted {
		t.Error("the device still advertises the default-key bit after being " +
			"given a real key")
	}

	// It answers to the new key.
	newController(t, testKey).handshake(t, d)
}

// TestTheOldKeyNoLongerWorksAfterAKeyInstall, which is the half that makes the
// install worth doing: a panel still holding SCBK-D is a panel that has not
// been told, and it must not be able to open the channel anyway.
func TestTheOldKeyNoLongerWorksAfterAKeyInstall(t *testing.T) {
	d := secureDevice(t, secure.DefaultBaseKey)

	c := newController(t, secure.DefaultBaseKey)
	c.handshake(t, d)
	c.open(t, answer(t, d, c.send(t, cmd.KeySetCommand(testKey[:]))))

	stale := newController(t, secure.DefaultBaseKey)
	chlng, err := stale.session.BeginChallenge([8]byte{9, 9, 9, 9, 9, 9, 9, 9})
	if err != nil {
		t.Fatalf("BeginChallenge: %v", err)
	}

	ccrypt := answer(t, d, stale.command(t, cmd.Message{Code: cmd.Chlng, Data: chlng}, secure.SCS11))
	if _, err := stale.session.AnswerCryptogram(ccrypt.Data); err == nil {
		t.Error("a panel still holding SCBK-D accepted the device's cryptogram " +
			"after the device was moved onto a new key")
	}
}

// TestAKeyInstallIsValidatedBeforeItIsAdopted.
//
// osdp_KEYSET defines exactly one key type, and a key of the wrong length is
// not a shorter key -- it is one neither end can derive from. Taking it would
// leave a reader holding something that is not an SCBK, with no command in the
// protocol able to say so.
func TestAKeyInstallIsValidatedBeforeItIsAdopted(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
		want    cmd.NAKReason
	}{
		{"a key too short to be an SCBK", []byte{0x01, 0x04, 1, 2, 3, 4}, cmd.NAKUnsupportedInput},
		{"a key type nobody can name", []byte{0x07, 0x10, 1, 2, 3, 4, 5, 6, 7, 8,
			9, 10, 11, 12, 13, 14, 15, 16}, cmd.NAKUnsupportedInput},
		{"a payload with no key at all", []byte{0x01}, cmd.NAKCommandLength},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := secureDevice(t, testKey)
			c := newController(t, testKey)
			c.handshake(t, d)

			reply := answer(t, d, c.send(t, cmd.Message{Code: cmd.KeySet, Data: tc.payload}))
			replyCode(t, reply, cmd.NAK)
			expectNAK(tc.want)(t, c.open(t, reply))

			// The session is untouched: a refused key is not a failed session.
			if !d.Secure() {
				t.Error("a refused key install tore the session down")
			}
			replyCode(t, answer(t, d, c.send(t, cmd.Message{Code: cmd.Poll})), cmd.ACK)
		})
	}
}

// TestAKeyInstallInTheClearIsRefusedAndNotAdopted.
//
// The payload is the base key. A panel that sent it unencrypted has handed the
// site key to anyone with a pair of probes and a cupboard door, and nothing
// afterwards takes it back -- so the device refuses rather than storing a key
// it must assume is already known.
func TestAKeyInstallInTheClearIsRefusedAndNotAdopted(t *testing.T) {
	var installed int
	d := secureDevice(t, secure.DefaultBaseKey, pd.WithKeyInstalled(
		func(context.Context, secure.BaseKey) { installed++ },
	))

	reply := answer(t, d, command(t, 1, cmd.KeySetCommand(testKey[:])))
	replyCode(t, reply, cmd.NAK)
	expectNAK(cmd.NAKSecureRequired)(t, reply.Data)

	if installed != 0 {
		t.Error("a key sent in the clear was installed")
	}

	// The conversation moves on before the handshake begins. A fresh controller
	// opens at sequence 1, and the refusal above is cached against that number:
	// re-using it would be answered from the cache, correctly, with the NAK.
	// That is the retransmission rule doing its job, and a panel is subject to
	// it exactly as this test is.
	replyCode(t, answer(t, d, poll(t, 2)), cmd.ACK)

	// And it is still the device it was: SCBK-D, still commissionable.
	newController(t, secure.DefaultBaseKey).handshake(t, d)
}
