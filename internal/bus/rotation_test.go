// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package bus_test

import (
	"context"
	"testing"
	"time"

	"github.com/regalium-os/osdp-go/internal/bus"
	"github.com/regalium-os/osdp-go/internal/secure"
)

// Commissioning: moving a device off the key printed in the specification and
// onto one only this installation knows. The helpers are in secure_test.go and
// keyset_test.go.

// commission runs the whole thing: enrol on SCBK-D, install a site key,
// re-handshake on it. It returns the event the installation produced.
func commission(t *testing.T, b *bus.Bus, d *bus.Device, pd *peripheral) bus.Event {
	t.Helper()
	ctx := context.Background()

	establish(t, b, d, pd)
	if !d.UsesDefaultKey() {
		t.Fatal("the device should have come up on SCBK-D")
	}

	if err := b.InstallKey(d, siteKeyB); err != nil {
		t.Fatalf("InstallKey: %v", err)
	}

	step, _ := b.Next(ctx)
	ev, err := b.Reply(ctx, d, pd.answer(t, ctx, step.Frame), time.Now())
	if err != nil {
		t.Fatalf("Reply: %v", err)
	}
	return ev
}

// defaultKeyBus offers SCBK-D, as a panel reaching a factory-fresh reader does.
func defaultKeyBus() *bus.Bus { return secureBus(secure.DefaultBaseKey, true) }

// TestADeviceIsMovedOffTheDefaultKey.
//
// A channel built on SCBK-D is authenticated against a key printed in the
// specification, which is to say not authenticated at all. It exists so a panel
// can reach a factory-fresh reader long enough to do this.
func TestADeviceIsMovedOffTheDefaultKey(t *testing.T) {
	b := defaultKeyBus()
	d := b.Devices()[0]
	pd := newPeripheral(secure.DefaultBaseKey)

	ev := commission(t, b, d, pd)

	if ev.Kind != bus.KindKeyInstalled {
		t.Fatalf("event = %v, want key_installed", ev.Kind)
	}
	if ev.DefaultKey {
		t.Error("the installed key was reported as SCBK-D; that is moving backwards")
	}
}

// TestTheSessionIsRebuiltOnTheNewKey.
//
// The session in force was derived from the key the device has just replaced,
// and the device has already stopped believing in it. Continuing to use it
// would fail every frame from here on.
func TestTheSessionIsRebuiltOnTheNewKey(t *testing.T) {
	ctx := context.Background()
	b := defaultKeyBus()
	d := b.Devices()[0]
	pd := newPeripheral(secure.DefaultBaseKey)

	commission(t, b, d, pd)

	if state, ok := d.SecureSession(); ok && state == secure.StateEstablished {
		t.Fatal("the old session survived the key that built it being replaced")
	}

	// Capabilities, then the four handshake messages, on the new key.
	for range 5 {
		step, ok := b.Next(ctx)
		if !ok {
			t.Fatal("the bus produced no step")
		}
		if _, err := b.Reply(ctx, d, pd.answer(t, ctx, step.Frame), time.Now()); err != nil {
			t.Fatalf("Reply: %v", err)
		}
	}

	if state, ok := d.SecureSession(); !ok || state != secure.StateEstablished {
		t.Fatalf("session = %v (present %v); the rebuild on the new key failed", state, ok)
	}
	if d.UsesDefaultKey() {
		t.Error("the rebuilt session is still on SCBK-D")
	}
}

// TestTheNewKeyIsAdoptedWithoutWaitingForTheApplication.
//
// The bus does not ask the keyring again after an installation. It cannot: the
// next handshake may begin microseconds after the acknowledgement, long before
// a consumer has read the event from a channel. An application that lost that
// race would have a device it could no longer talk to.
//
// The bus here is built with a keyring that still returns SCBK-D, exactly as an
// application that has not yet persisted the new key would have. The rebuild
// must succeed anyway.
func TestTheNewKeyIsAdoptedWithoutWaitingForTheApplication(t *testing.T) {
	ctx := context.Background()
	b := defaultKeyBus() // this keyring will never return siteKeyB
	d := b.Devices()[0]
	pd := newPeripheral(secure.DefaultBaseKey)

	commission(t, b, d, pd)

	for range 5 {
		step, _ := b.Next(ctx)
		if _, err := b.Reply(ctx, d, pd.answer(t, ctx, step.Frame), time.Now()); err != nil {
			t.Fatalf("Reply: %v", err)
		}
	}

	if d.UsesDefaultKey() {
		t.Fatal("the bus fell back to the stale keyring and re-handshook on SCBK-D")
	}
	if state, _ := d.SecureSession(); state != secure.StateEstablished {
		t.Fatalf("session = %v, want established on the installed key", state)
	}
}
