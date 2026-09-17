// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package osdp_test

import (
	"context"
	"testing"
	"time"

	"github.com/regalium-os/osdp-go"
)

// TestSecureChannelIsConfigurableFromOutside builds a bus with a Secure Channel
// through the public API alone and drives it to the point where the panel
// challenges the device.
//
// It lives in package osdp_test so that a re-export which quietly became a
// wrapper type would fail to compile here rather than in a consumer's build:
// the cipher suite, the key and the nonce source all have to be the same types
// the bus declares internally, or the option cannot be constructed at all.
func TestSecureChannelIsConfigurableFromOutside(t *testing.T) {
	ctx := context.Background()

	keys := func(osdp.Address) (osdp.BaseKey, bool) { return osdp.DefaultBaseKey, true }
	nonce := func() [8]byte { return [8]byte{1, 2, 3, 4, 5, 6, 7, 8} }

	line := osdp.Line{Name: "test", Baud: 9600, ReplyTimeout: time.Second}
	bus := osdp.NewBus(line, []osdp.Address{0x00}, osdp.SchemeCRC16,
		osdp.WithSecureChannel(osdp.AES128{}, keys, nonce))
	device := bus.Devices()[0]

	// Identify, then report capabilities including AES-128.
	answer(t, ctx, bus, device, 0x45, deviceID)
	event := answer(t, ctx, bus, device, 0x46, capabilities)

	if event.Kind != osdp.EventCapabilities {
		t.Fatalf("event = %v, want capabilities", event.Kind)
	}
	if capable, defaultKey := event.Caps.SecureChannel(); !capable || defaultKey {
		t.Errorf("SecureChannel = (%v, %v), want (true, false)", capable, defaultKey)
	}

	// The panel's next command is the challenge that opens the handshake.
	step, ok := bus.Next(ctx)
	if !ok {
		t.Fatal("the bus produced no step")
	}
	if step.Frame.Security == nil {
		t.Fatal("the challenge carried no security block")
	}
	if got := osdp.SecureBlockType(step.Frame.Security.Type); got != osdp.SCS11 {
		t.Errorf("security block = %v, want SCS_11", got)
	}
	if osdp.Code(step.Frame.Code) != 0x76 {
		t.Errorf("command = 0x%02X, want 0x76 (osdp_CHLNG)", step.Frame.Code)
	}
}

// deviceID is an osdp_PDID payload, and capabilities an osdp_PDCAP one from a
// reader that speaks AES-128 on a site key.
var (
	deviceID     = []byte{0x00, 0x06, 0x8E, 0x01, 0x02, 0x0A, 0x0B, 0x0C, 0x0D, 0x01, 0x00, 0x03}
	capabilities = []byte{0x09, 0x01, 0x00, 0x0A, 0x80, 0x00, 0x0D, 0x01, 0x01}
)

// answer sends the bus's next command nowhere and hands back the reply a device
// would have given, returning the event it produced.
func answer(
	t *testing.T, ctx context.Context, b *osdp.Bus, d *osdp.Device, code byte, data []byte,
) osdp.Event {
	t.Helper()

	step, ok := b.Next(ctx)
	if !ok {
		t.Fatal("the bus produced no step")
	}

	reply := osdp.Frame{
		IsReply: true,
		Address: d.Address,
		Control: osdp.NewControl(step.Frame.Control.Sequence(), osdp.SchemeCRC16, false),
		Code:    code,
		Data:    data,
	}
	reply.Seal()

	event, err := b.Reply(ctx, d, reply, time.Now())
	if err != nil {
		t.Fatalf("Reply: %v", err)
	}
	return event
}
