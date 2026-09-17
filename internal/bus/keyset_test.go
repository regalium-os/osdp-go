// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package bus_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/regalium-os/osdp-go/internal/bus"
	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/secure"
)

// siteKeyB is the key a commissioning panel installs, distinct from siteKey so
// a test can tell which of the two a handshake used.
var siteKeyB = secure.BaseKey{
	0xB0, 0xB1, 0xB2, 0xB3, 0xB4, 0xB5, 0xB6, 0xB7,
	0xB8, 0xB9, 0xBA, 0xBB, 0xBC, 0xBD, 0xBE, 0xBF,
}

// TestAKeyIsNeverInstalledInTheClear.
//
// The payload of an osdp_KEYSET is the key. Sending it on an unencrypted line
// hands the site key to anyone with a pair of probes, and no error returned
// afterwards takes it back: the key is out, and every device it was installed
// on has to be re-keyed. So the refusal is at the call site, before anything is
// queued.
func TestAKeyIsNeverInstalledInTheClear(t *testing.T) {
	b := newBus(0x00)
	d := b.Devices()[0]
	online(t, b, d) // online, but with no secure channel

	if err := b.InstallKey(d, siteKeyB); !errors.Is(err, bus.ErrNotSecure) {
		t.Fatalf("InstallKey on a plaintext line = %v, want ErrNotSecure", err)
	}
	if d.Queued() != 0 {
		t.Error("the key was queued anyway; a refusal must queue nothing")
	}
}

// TestAKeyIsWithheldWhenTheChannelCannotBeRebuilt.
//
// The dangerous path, and not an obvious one. A key is queued while the channel
// is good; the channel then fails and cannot be re-established -- a reader that
// has forgotten its key, or a line someone is interfering with. The device
// drops back to plaintext and keeps being polled, which is correct. What must
// not happen is the queued command going out on that plaintext line, because
// its payload is the site key.
//
// The call-site refusal cannot help here: the channel was established when
// InstallKey was called. Only the check before transmission can.
func TestAKeyIsWithheldWhenTheChannelCannotBeRebuilt(t *testing.T) {
	ctx := context.Background()
	b := secureBus(siteKey, true)
	d := b.Devices()[0]
	pd := newPeripheral(siteKey)

	establish(t, b, d, pd)
	if err := b.InstallKey(d, siteKeyB); err != nil {
		t.Fatalf("InstallKey: %v", err)
	}

	// From here the device can no longer prove it holds the key.
	pd.badCryptogram = true

	// Tear the session down: a frame arrives that fails authentication.
	step, _ := b.Next(ctx)
	answer := pd.answer(t, ctx, step.Frame)
	answer.Data[len(answer.Data)-1] ^= 0xFF
	answer.Seal()
	if _, err := b.Reply(ctx, d, answer, time.Now()); err != nil {
		t.Fatalf("Reply: %v", err)
	}

	// Drive the bus long enough to re-enrol, fail the handshake, and settle
	// into polling in the clear.
	var reachedPlaintext bool
	for range 12 {
		next, ok := b.Next(ctx)
		if !ok {
			t.Fatal("the bus produced no step")
		}

		if next.Frame.Security == nil {
			reachedPlaintext = true
		}
		if cmd.Code(next.Frame.Code) == cmd.KeySet {
			t.Fatal("the key was transmitted after the channel was lost")
		}
		if bytes.Contains(next.Frame.Data, siteKeyB[:]) {
			t.Fatal("the site key is on the wire in the clear")
		}

		if _, err := b.Reply(ctx, d, pd.answer(t, ctx, next.Frame), time.Now()); err != nil {
			t.Fatalf("Reply: %v", err)
		}
	}

	if !reachedPlaintext {
		t.Fatal("the device never dropped to plaintext; this test proved nothing")
	}
	if d.Queued() == 0 {
		t.Error("the key was discarded rather than withheld; it should still be waiting")
	}
}

// TestTheKeyTravelsEnciphered: it goes out under SCS_17, and the key material
// must not be recognisable in the octets.
func TestTheKeyTravelsEnciphered(t *testing.T) {
	ctx := context.Background()
	b := secureBus(siteKey, true)
	d := b.Devices()[0]
	pd := newPeripheral(siteKey)
	establish(t, b, d, pd)

	if err := b.InstallKey(d, siteKeyB); err != nil {
		t.Fatalf("InstallKey: %v", err)
	}

	step, ok := b.Next(ctx)
	if !ok {
		t.Fatal("the bus produced no step")
	}
	if step.Frame.Security == nil {
		t.Fatal("the key went out with no security block")
	}
	if got := secure.BlockType(step.Frame.Security.Type); got != secure.SCS17 {
		t.Errorf("security block = %v, want SCS_17", got)
	}

	wire, err := step.Frame.Append(ctx, nil)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if bytes.Contains(wire, siteKeyB[:]) {
		t.Fatal("the site key is on the wire in the clear")
	}

	// And the device recovers exactly the key the panel meant to send.
	if _, rErr := b.Reply(ctx, d, pd.answer(t, ctx, step.Frame), time.Now()); rErr != nil {
		t.Fatalf("Reply: %v", rErr)
	}
	last := pd.received[len(pd.received)-1]
	if last.Code != cmd.KeySet {
		t.Fatalf("the device received %s, want osdp_KEYSET", last.Code.Name(false))
	}

	payload, err := cmd.ParseKeySet(last.Data)
	if err != nil {
		t.Fatalf("ParseKeySet: %v", err)
	}
	if payload.Type != cmd.KeySCBK {
		t.Errorf("key type = %d, want SCBK", payload.Type)
	}
	if !bytes.Equal(payload.Key, siteKeyB[:]) {
		t.Errorf("deciphered key = % X, want % X", payload.Key, siteKeyB[:])
	}
}
