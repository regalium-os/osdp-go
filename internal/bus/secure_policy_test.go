// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package bus_test

import (
	"context"
	"testing"

	"github.com/regalium-os/osdp-go/internal/bus"
	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/secure"
)

// enrol runs osdp_ID and osdp_CAP, stopping wherever that leaves the device.
func enrol(t *testing.T, b *bus.Bus, d *bus.Device, pd *peripheral) bus.Event {
	t.Helper()
	exchange(t, b, d, pd)
	return exchange(t, b, d, pd)
}

// nextCode returns the command the bus wants to send next, without answering.
func nextCode(t *testing.T, b *bus.Bus) (cmd.Code, bool) {
	t.Helper()
	step, ok := b.Next(context.Background())
	if !ok {
		t.Fatal("the bus produced no step")
	}
	return cmd.Code(step.Frame.Code), step.Frame.Security != nil
}

// TestSecureNotOfferedToADeviceThatDoesNotClaimIt: challenging a reader that
// never advertised Secure Channel earns an osdp_NAK every cycle, forever.
func TestSecureNotOfferedToADeviceThatDoesNotClaimIt(t *testing.T) {
	b := secureBus(siteKey, true)
	d := b.Devices()[0]
	pd := newPeripheral(siteKey)
	pd.plainReader = true

	if ev := enrol(t, b, d, pd); ev.Kind != bus.KindOnline {
		t.Fatalf("enrolment produced %v, want online", ev.Kind)
	}
	if d.State != bus.Online {
		t.Fatalf("state = %v, want online", d.State)
	}

	code, secured := nextCode(t, b)
	if code != cmd.Poll || secured {
		t.Errorf("next command = %s (secure %v), want a plain osdp_POLL",
			code.Name(false), secured)
	}
}

// TestSecureNotOfferedWithoutAKey: a device nobody has commissioned yet is
// polled in the clear rather than challenged with a key that does not exist.
func TestSecureNotOfferedWithoutAKey(t *testing.T) {
	b := secureBus(secure.BaseKey{}, false)
	d := b.Devices()[0]

	if ev := enrol(t, b, d, newPeripheral(siteKey)); ev.Kind != bus.KindOnline {
		t.Fatalf("enrolment produced %v, want online", ev.Kind)
	}
	if _, ok := d.SecureSession(); ok {
		t.Error("a session was created for a device with no key")
	}
}

// TestSecureChannelIsOffByDefault: a bus built without the option never puts a
// security block on the line. A panel that silently began encrypting would
// strand every device whose key the application had not loaded.
func TestSecureChannelIsOffByDefault(t *testing.T) {
	b := newBus(0x00)
	d := b.Devices()[0]

	if ev := enrol(t, b, d, newPeripheral(siteKey)); ev.Kind != bus.KindOnline {
		t.Fatalf("enrolment produced %v, want online", ev.Kind)
	}
	if _, secured := nextCode(t, b); secured {
		t.Error("a bus with no secure channel configured sent a security block")
	}
}

// TestDefaultKeyIsReported: a session on SCBK-D is authenticated against a key
// printed in the specification, and a deployment must be told so.
func TestDefaultKeyIsReported(t *testing.T) {
	b := secureBus(secure.DefaultBaseKey, true)
	d := b.Devices()[0]
	pd := newPeripheral(secure.DefaultBaseKey)

	enrol(t, b, d, pd)
	exchange(t, b, d, pd)

	ev := exchange(t, b, d, pd)
	if ev.Kind != bus.KindSecure {
		t.Fatalf("the handshake produced %v, want secure", ev.Kind)
	}
	if !ev.DefaultKey {
		t.Error("a session on SCBK-D was not reported as such")
	}
	if !d.UsesDefaultKey() {
		t.Error("the device does not report running on the default key")
	}
}
