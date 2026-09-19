// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/pd"
	"github.com/regalium-os/osdp-go/internal/secure"
)

// The Secure Channel, driven against a real secure.Session in RoleCP -- the
// same type and the same code path the control panel uses. A device that
// satisfies this satisfies the panel half of this repository, because there is
// nothing else in between. Failures and teardown are secure_failure_test.go.

// TestAHandshakeEstablishesASession. SIA OSDP v2.2.2 §7.2.
func TestAHandshakeEstablishesASession(t *testing.T) {
	d := secureDevice(t, testKey)
	c := newController(t, testKey)

	if d.Secure() {
		t.Error("a freshly built device reports an established session")
	}
	c.handshake(t, d)

	if !d.Secure() {
		t.Error("the device does not consider the session established after " +
			"four messages the panel accepted")
	}
	if !c.session.Established() {
		t.Error("the panel does not consider the session established")
	}
}

// TestAnEstablishedSessionCarriesCommandsAndReplies, in both block forms: an
// osdp_POLL has no payload and is authenticated only, an osdp_OUT has one and
// is enciphered.
func TestAnEstablishedSessionCarriesCommandsAndReplies(t *testing.T) {
	d := secureDevice(t, testKey)
	c := newController(t, testKey)
	c.handshake(t, d)

	reply := answer(t, d, c.send(t, cmd.Message{Code: cmd.Poll}))
	if got := secure.BlockType(reply.Security.Type); got != secure.SCS16 {
		t.Errorf("an empty reply travelled under %s, want SCS_16", got)
	}
	replyCode(t, reply, cmd.ACK)
	if payload := c.open(t, reply); len(payload) != 0 {
		t.Errorf("osdp_ACK carried %x, want nothing", payload)
	}

	// Every reply is opened, and it is not optional book-keeping: a command
	// chains its authentication code from the last reply's, so a panel that
	// skipped one would authenticate its next command from a chain value the
	// device has already moved past. That is the same mechanism a lost reply
	// exercises, and it is why a retransmission must replay rather than reseal.
	release := cmd.OutputCommand(cmd.Output{Number: 1, Control: cmd.OutputTimedOn})
	released := answer(t, d, c.send(t, release))
	replyCode(t, released, cmd.ACK)
	c.open(t, released)

	status := c.open(t, answer(t, d, c.send(t, cmd.OutputStatusCommand())))
	changes, err := cmd.ParseOutputStatus(status)
	if err != nil {
		t.Fatalf("osdp_OSTATR would not parse: %v", err)
	}
	if len(changes) != 2 || !changes[1].Active {
		t.Errorf("output report was %+v, want output 1 active: the enciphered "+
			"osdp_OUT did not reach the dispatch table intact", changes)
	}
}

// TestACredentialIsEncipheredOnAnEstablishedSession.
//
// This is what the channel is for. A card read answered under SCS_16 would be
// authenticated and perfectly readable by anyone with a probe on the line, so
// the test asserts both halves: the block says enciphered, and the octets of
// the credential do not appear anywhere in the frame that carries them.
func TestACredentialIsEncipheredOnAnEstablishedSession(t *testing.T) {
	ctx := context.Background()
	d := secureDevice(t, testKey)
	c := newController(t, testKey)
	c.handshake(t, d)

	credential := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	if err := d.ReportCardRead(ctx, cmd.CardRead{
		Format: 1, BitCount: 26, Data: credential,
	}); err != nil {
		t.Fatalf("ReportCardRead: %v", err)
	}

	reply := answer(t, d, c.send(t, cmd.Message{Code: cmd.Poll}))
	replyCode(t, reply, cmd.Raw)

	if got := secure.BlockType(reply.Security.Type); got != secure.SCS18 {
		t.Errorf("a credential travelled under %s, want SCS_18", got)
	}
	if bytes.Contains(wire(t, reply), credential) {
		t.Error("the credential appears verbatim in the frame that carried it")
	}

	got, err := cmd.ParseCardRead(c.open(t, reply))
	if err != nil {
		t.Fatalf("the deciphered osdp_RAW would not parse: %v", err)
	}
	if !bytes.Equal(got.Data, credential) || got.BitCount != 26 {
		t.Errorf("credential deciphered as %+v, want %x at 26 bits", got, credential)
	}
}

// TestARepeatOnAnEstablishedSessionReplaysWithoutDesynchronisingTheChains.
//
// This is the interaction that makes a secure device harder than a plaintext
// one. A repeat must be answered from the cache *before* the message
// authentication code is consulted: this end already verified that command once
// and advanced its command chain, so verifying the repeat would fail and tear
// down a session that is working perfectly.
//
// The panel deliberately never opens the first reply. That is what a lost reply
// means -- its chain did not advance either -- and it is why replaying the
// cached octets leaves the two ends in step rather than one exchange apart.
func TestARepeatOnAnEstablishedSessionReplaysWithoutDesynchronisingTheChains(t *testing.T) {
	ctx := context.Background()
	d := secureDevice(t, testKey)
	c := newController(t, testKey)
	c.handshake(t, d)

	if err := d.ReportCardRead(ctx, cmd.CardRead{
		Format: 1, BitCount: 8, Data: []byte{0x5A},
	}); err != nil {
		t.Fatalf("ReportCardRead: %v", err)
	}

	sent := c.send(t, cmd.Message{Code: cmd.Poll})
	lost := answer(t, d, sent) // the panel never sees this one
	replayed := answer(t, d, sent)

	if !bytes.Equal(wire(t, lost), wire(t, replayed)) {
		t.Fatalf("the retransmission produced\n  %x\nafter\n  %x",
			wire(t, replayed), wire(t, lost))
	}

	// The panel authenticates the reply it actually received. If the device had
	// re-sealed rather than replayed, its reply chain would have advanced twice
	// and this would fail.
	card, err := cmd.ParseCardRead(c.open(t, replayed))
	if err != nil {
		t.Fatalf("the replayed osdp_RAW would not parse: %v", err)
	}
	if !bytes.Equal(card.Data, []byte{0x5A}) {
		t.Errorf("replay carried %x, want 5a", card.Data)
	}

	// And the session keeps working, which is the proof the chains are in step
	// rather than merely that one frame matched.
	if !d.Secure() {
		t.Fatal("the session did not survive a retransmission")
	}
	replyCode(t, answer(t, d, c.send(t, cmd.Message{Code: cmd.Poll})), cmd.ACK)
}

// TestTheCapabilityReportMatchesTheKeyTheDeviceHolds.
//
// The two octets answer different questions, and a panel acts on each. A
// device with no key must not be challenged at all; a device on SCBK-D is
// installable but not secure, and a deployment should be able to see the
// difference rather than treating the channel as trustworthy.
func TestTheCapabilityReportMatchesTheKeyTheDeviceHolds(t *testing.T) {
	tests := []struct {
		name               string
		device             func(t *testing.T) *pd.Device
		capable, defaulted bool
	}{
		{"no key at all", func(t *testing.T) *pd.Device { return newDevice(t) }, false, false},
		{"an install-specific key", func(t *testing.T) *pd.Device {
			return secureDevice(t, testKey)
		}, true, false},
		{"SCBK-D, straight from the box", func(t *testing.T) *pd.Device {
			return secureDevice(t, secure.DefaultBaseKey)
		}, true, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			capable, defaulted := tc.device(t).Capabilities().SecureChannel()
			if capable != tc.capable || defaulted != tc.defaulted {
				t.Errorf("reported capable=%v default-key=%v, want %v and %v",
					capable, defaulted, tc.capable, tc.defaulted)
			}
		})
	}
}
