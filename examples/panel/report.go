// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/regalium-os/osdp-go"
)

// report prints every event until the runtime stops.
//
// The channel closes when Run returns, so ranging over it is the whole loop --
// there is no second signal to watch and no way to be left hanging on one.
func report(ctx context.Context, p *osdp.Panel, cfg config) {
	for event := range p.Events() {
		line := describe(event)
		if line == "" {
			continue
		}
		fmt.Printf("%s  %-4s  %s\n",
			time.Now().Format("15:04:05.000"),
			fmt.Sprintf("%02X", byte(event.Device.Address)),
			line)

		switch {
		case event.Kind == osdp.EventCardRead && cfg.unlock > 0:
			releaseDoor(ctx, p, event.Device.Address, cfg.unlock)

		case event.Kind == osdp.EventOnline && cfg.setAddr >= 0:
			commission(ctx, p, event.Device.Address, cfg)

		case event.Kind == osdp.EventCommunication:
			// The device has moved. Nothing further this tool can usefully do:
			// carrying on would mean the converter needs the new speed too,
			// which is outside this program's control.
			fmt.Println("\ncommissioned. re-run with -devices " +
				fmt.Sprintf("%d", event.Communication.Address))
			return
		}
	}
}

// describe renders one event, or "" for the ones not worth a line.
//
// A credential is never printed. The format and the bit count say which
// technology was presented without saying who presented it, and that is the
// distinction this tool has to keep even when somebody is debugging at 3am and
// would quite like to see the number.
func describe(e osdp.Event) string {
	switch e.Kind {
	case osdp.EventOnline:
		// The capability report rides on this event unless a secure handshake
		// follows, in which case it gets one of its own. Either way it is the
		// most useful line this tool prints about a reader nobody has met.
		if len(e.Caps) > 0 {
			return "online — " + describeCapabilities(e.Caps)
		}
		return "online"
	case osdp.EventOffline:
		return "OFFLINE — stopped answering"
	case osdp.EventIdentified:
		return fmt.Sprintf("identified: OUI %06X, model %d, version %d, firmware %d.%d.%d",
			e.ID.OUI(), e.ID.Model, e.ID.Version,
			e.ID.Firmware[0], e.ID.Firmware[1], e.ID.Firmware[2])

	case osdp.EventCapabilities:
		return describeCapabilities(e.Caps)

	case osdp.EventCardRead:
		return fmt.Sprintf("CARD: %d bits, format %d, reader %d  (credential not shown)",
			e.Card.BitCount, e.Card.Format, e.Card.Reader)

	case osdp.EventKeypad:
		return fmt.Sprintf("KEYPAD: %d keys, reader %d  (digits not shown)",
			len(e.Keypad.Keys), e.Keypad.Reader)

	case osdp.EventStatusChange:
		return describeStatus(e.Status)

	case osdp.EventNAK:
		return fmt.Sprintf("refused the command: %s", e.NAK)
	case osdp.EventBusy:
		return "busy — command requeued, polling instead"
	case osdp.EventResync:
		return "lost synchronisation; restarting the exchange"

	case osdp.EventSecure:
		if e.DefaultKey {
			return "secure channel up — ON THE DEFAULT KEY, install a real one"
		}
		return "secure channel up"
	case osdp.EventSecureFailed:
		return "secure channel FAILED — carrying on in the clear"
	case osdp.EventKeyInstalled:
		return "new base key accepted; persist it"

	case osdp.EventCommunication:
		return fmt.Sprintf("MOVED from address %02X to %02X at %d baud — persist this, "+
			"there is no command for asking a device where it is",
			byte(e.PreviousAddr), e.Communication.Address, e.Communication.Baud)

	case osdp.EventManufacturer:
		return fmt.Sprintf("vendor message: OUI %06X, %d octets",
			e.Manufacturer.OUI, len(e.Manufacturer.Body))

	default:
		return ""
	}
}

// describeCapabilities prints what a device says it can do, which is the first
// thing worth knowing about a reader nobody has met before.
func describeCapabilities(caps osdp.CapabilityReport) string {
	claimed := osdp.DeviceCapabilities(caps)

	secure := "no secure channel"
	if capable, defaultKey := caps.SecureChannel(); capable {
		secure = "AES-128"
		if defaultKey {
			secure += " on the DEFAULT key"
		}
	}

	return fmt.Sprintf("capabilities: %d reader(s), %d LED(s), %d input(s), %d output(s), "+
		"%d-octet buffer, %s",
		claimed.Readers, claimed.LEDs, claimed.Inputs, claimed.Outputs,
		claimed.MaxMessageSize, secure)
}

// describeStatus renders the contacts that moved.
func describeStatus(changes []osdp.StatusChange) string {
	out := "status:"
	for _, c := range changes {
		state := "clear"
		if c.Active {
			state = "ACTIVE"
		}
		out += fmt.Sprintf("  %s[%d]=%s", c.Kind, c.Index, state)
	}
	return out
}

// commission moves a device onto its configured address.
//
// It waits for the device to come online first, because an address change is
// the one command that has to be sent to a device the panel is certain it is
// talking to: get it wrong and the reader is still there, still working, and
// unreachable.
func commission(ctx context.Context, p *osdp.Panel, addr osdp.Address, cfg config) {
	to := osdp.Communication{Address: byte(cfg.setAddr), Baud: cfg.setBaud}

	fmt.Printf("              moving to address %02X at %d baud\n", to.Address, to.Baud)
	if err := p.SetCommunication(ctx, addr, to); err != nil {
		fmt.Printf("              refused: %v\n", err)
	}
}

// releaseDoor sends the pair of commands that grant access.
//
// Both are timed rather than latched, so this tool exiting mid-grant leaves a
// locked door rather than an open one.
func releaseDoor(ctx context.Context, p *osdp.Panel, addr osdp.Address, hold time.Duration) {
	strike, indicator := osdp.Unlock(0, 0, 0, hold)

	for _, command := range []osdp.Message{strike, indicator} {
		if err := p.Send(ctx, addr, command); err != nil {
			fmt.Printf("              unlock failed: %v\n", err)
			return
		}
	}
	fmt.Printf("              releasing output 0 for %s\n", hold)
}
