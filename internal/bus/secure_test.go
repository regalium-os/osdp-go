// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package bus_test

import (
	"context"
	"testing"
	"time"

	"github.com/regalium-os/osdp-go/internal/bus"
	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/internal/secure"
	"github.com/regalium-os/osdp-go/internal/transport"
)

// siteKey stands in for a commissioned device's SCBK. Any value but SCBK-D
// does, and using one that is not the default keeps TestDefaultKeyIsReported
// honest about what it is detecting.
var siteKey = secure.BaseKey{
	0xA0, 0xA1, 0xA2, 0xA3, 0xA4, 0xA5, 0xA6, 0xA7,
	0xA8, 0xA9, 0xAA, 0xAB, 0xAC, 0xAD, 0xAE, 0xAF,
}

// fixedNonce is RND.A, held constant so a handshake replays deterministically.
// A real panel passes crypto/rand; see bus.NonceSource for why this is injected.
func fixedNonce() [8]byte {
	return [8]byte{0xDE, 0xAD, 0xBE, 0xEF, 0x01, 0x02, 0x03, 0x04}
}

// secureBus returns a bus that will offer a secure channel to address 0x00,
// holding key for it.
func secureBus(key secure.BaseKey, known bool) *bus.Bus {
	line := transport.Line{Name: "test", Baud: 9600, ReplyTimeout: 200 * time.Millisecond}
	return bus.New(line, []frame.Address{0x00}, frame.SchemeCRC16,
		bus.WithSecureChannel(secure.AES128{}, func(frame.Address) (secure.BaseKey, bool) {
			return key, known
		}, fixedNonce),
	)
}

// exchange runs one command and its answer, returning the event it produced.
func exchange(t *testing.T, b *bus.Bus, d *bus.Device, pd *peripheral) bus.Event {
	t.Helper()
	ctx := context.Background()

	step, ok := b.Next(ctx)
	if !ok {
		t.Fatal("the bus produced no step")
	}
	ev, err := b.Reply(ctx, d, pd.answer(t, ctx, step.Frame), time.Now())
	if err != nil {
		t.Fatalf("Reply: %v", err)
	}
	return ev
}

// TestSecureChannelEstablishes drives a whole enrolment against a device that
// actually holds the key: identify, capabilities, then the four-message
// handshake, then authenticated traffic.
//
// Both ends compute their own message authentication codes over frames the
// other assembled, so this fails if the two disagree by a single octet about
// what the code covers.
func TestSecureChannelEstablishes(t *testing.T) {
	b := secureBus(siteKey, true)
	d := b.Devices()[0]
	pd := newPeripheral(siteKey)

	if ev := exchange(t, b, d, pd); ev.Kind != bus.KindIdentified {
		t.Fatalf("osdp_ID produced %v, want identified", ev.Kind)
	}

	// The capability reply is what decides there will be a handshake at all.
	ev := exchange(t, b, d, pd)
	if ev.Kind != bus.KindCapabilities {
		t.Fatalf("osdp_CAP produced %v, want capabilities", ev.Kind)
	}
	if capable, _ := ev.Caps.SecureChannel(); !capable {
		t.Fatal("the device claimed AES-128 and the report does not say so")
	}
	if d.State != bus.SecureHandshake {
		t.Fatalf("state = %v, want secure_handshake", d.State)
	}

	// osdp_CHLNG out, osdp_CCRYPT back: nothing to report yet.
	if cryptogram := exchange(t, b, d, pd); cryptogram.Kind != bus.KindNone {
		t.Fatalf("the cryptogram exchange produced %v, want none", cryptogram.Kind)
	}

	// osdp_SCRYPT out, osdp_RMAC_I back: the session is up.
	ev = exchange(t, b, d, pd)
	if ev.Kind != bus.KindSecure {
		t.Fatalf("the handshake produced %v, want secure", ev.Kind)
	}
	if ev.DefaultKey {
		t.Error("a session on a site key was reported as running on SCBK-D")
	}
	if d.State != bus.Secure {
		t.Fatalf("state = %v, want secure", d.State)
	}
	if state, ok := d.SecureSession(); !ok || state != secure.StateEstablished {
		t.Errorf("session state = %v (present %v), want established", state, ok)
	}

	// And now ordinary traffic, authenticated in both directions.
	if polled := exchange(t, b, d, pd); polled.Kind != bus.KindNone {
		t.Fatalf("a secure poll produced %v, want none", polled.Kind)
	}
}
