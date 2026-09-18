// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package bus_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/regalium-os/osdp-go/internal/bus"
	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/secure"
)

// What travels over an established channel. Getting one established is
// secure_test.go; the helpers live there.

// TestSecureChannelCarriesCredentialsEnciphered: once a session exists, a card
// read comes back under SCS_18 and must be deciphered before it means anything.
func TestSecureChannelCarriesCredentialsEnciphered(t *testing.T) {
	b := secureBus(siteKey, true)
	d := b.Devices()[0]
	pd := newPeripheral(siteKey)

	establish(t, b, d, pd)

	// Reader 0, format 1, 26 bits of Wiegand.
	pd.card = []byte{0x00, 0x01, 0x1A, 0x00, 0xAB, 0xCD, 0xEF, 0x80}

	ev := exchange(t, b, d, pd)
	if ev.Kind != bus.KindCardRead {
		t.Fatalf("event = %v, want card_read", ev.Kind)
	}
	if ev.Card.BitCount != 26 {
		t.Errorf("bit count = %d, want 26", ev.Card.BitCount)
	}
}

// establish runs enrolment and the handshake to completion.
func establish(t *testing.T, b *bus.Bus, d *bus.Device, pd *peripheral) {
	t.Helper()

	for range 4 {
		exchange(t, b, d, pd)
	}
	if d.State != bus.Secure {
		t.Fatalf("state after enrolment = %v, want secure", d.State)
	}
}

// TestACommandWithAPayloadIsEnciphered.
//
// Until there was a command path, the only thing a secure session ever carried
// was osdp_POLL -- no payload, so SCS_15 and nothing to encipher. A door
// release is the first message with something in it, and something in it is
// exactly what must not travel in the clear on a wire anybody can reach.
func TestACommandWithAPayloadIsEnciphered(t *testing.T) {
	b := secureBus(siteKey, true)
	d := b.Devices()[0]
	pd := newPeripheral(siteKey)

	establish(t, b, d, pd)

	// Release the strike on output 0 for five seconds.
	b.Send(d, cmd.OutputCommand(cmd.Output{
		Number: 0, Control: cmd.OutputTimedOn, Timer: 5 * time.Second,
	}))

	ctx := context.Background()
	step, ok := b.Next(ctx)
	if !ok {
		t.Fatal("the bus produced no step")
	}

	if step.Frame.Security == nil {
		t.Fatal("the command carried no security block")
	}
	if got := secure.BlockType(step.Frame.Security.Type); got != secure.SCS17 {
		t.Errorf("security block = %v, want SCS_17 for a command with a payload", got)
	}

	// The plaintext must not be recognisable on the wire.
	wire, err := step.Frame.Append(ctx, nil)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if bytes.Contains(wire, []byte{0x00, 0x05, 0x32, 0x00}) {
		t.Error("the output control block is on the wire in the clear")
	}

	// And the device recovers exactly what the panel meant.
	if _, err := b.Reply(ctx, d, pd.answer(t, ctx, step.Frame), time.Now()); err != nil {
		t.Fatalf("Reply: %v", err)
	}

	last := pd.received[len(pd.received)-1]
	if last.Code != cmd.Out {
		t.Fatalf("the device received %s, want osdp_OUT", last.Code.Name(false))
	}
	if want := []byte{0x00, 0x05, 0x32, 0x00}; !bytes.Equal(last.Data, want) {
		t.Errorf("deciphered payload = % X, want % X", last.Data, want)
	}
}

// establishWithBuffer brings a secure session up against a device advertising a
// particular receive buffer.
func establishWithBuffer(t *testing.T, b *bus.Bus, d *bus.Device, pd *peripheral, buffer int) {
	t.Helper()
	pd.buffer = buffer
	establish(t, b, d, pd)
}
