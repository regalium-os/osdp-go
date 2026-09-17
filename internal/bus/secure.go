// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package bus

import (
	"context"
	"errors"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/internal/secure"
	"github.com/regalium-os/osdp-go/telemetry"
)

// errNoSession reports a handshake reply arriving for a device the bus is not
// handshaking with -- a device answering a challenge nobody sent it, or one
// whose session was torn down between the command and the answer.
//
// It is unexported because it never reaches a caller: a handshake that cannot
// proceed is reported as KindSecureFailed like every other way of failing one.
// It exists so the span says which way it failed.
var errNoSession = errors.New("osdp/bus: handshake reply with no session in progress")

// secureStep is one command of the handshake and the security block that
// carries it. The two travel together because the block type is what tells the
// device which message it is looking at.
type secureStep struct {
	msg   cmd.Message
	block frame.SecurityBlock
}

// afterCapabilities decides what a device does once it has said what it can do.
//
// The decision is the device's own answer first and the panel's configuration
// second, and never the other way round. A reader that does not claim AES-128
// is never challenged, and neither is one the application holds no key for:
// challenging anyway earns an osdp_NAK every cycle, forever, which is how a
// panel ends up hammering a reader it will never secure.
func (b *Bus) afterCapabilities(d *Device) {
	d.State = Online

	if !b.secureEnabled() || d.secureDeclined {
		return
	}
	if capable, _ := d.Caps.SecureChannel(); !capable {
		d.secureDeclined = true
		return
	}

	key, ok := d.baseKey(b.keys)
	if !ok {
		d.secureDeclined = true
		return
	}

	d.session = secure.NewSession(b.suite, secure.RoleCP, key)
	d.State = SecureHandshake
}

// handshakeCommand returns the next command of the four-message handshake.
//
// The exchange alternates strictly -- challenge, cryptogram, server cryptogram,
// initial R-MAC -- so only one command is ever outstanding, and the reply that
// prompts the next one computes it. This either hands over what the last reply
// produced or starts the exchange.
//
// Starting it again is what happens when a handshake reply is lost: a fresh
// RND.A restarts the derivation from the beginning, which is the only safe
// thing to do with a session whose peer may or may not have advanced.
func (b *Bus) handshakeCommand(d *Device) (cmd.Message, *frame.SecurityBlock) {
	if d.pending != nil {
		step := d.pending
		d.pending = nil
		return step.msg, &step.block
	}

	chlng, err := d.session.BeginChallenge(b.nonce())
	if err != nil {
		// Only a role error can land here, which would be a bug in this
		// package rather than anything the device did. Fall back to plaintext
		// rather than stalling the cycle on it.
		d.abandonSecure()
		d.State = Online
		return cmd.Message{Code: cmd.Poll}, nil
	}

	return cmd.Message{Code: cmd.Chlng, Data: chlng},
		&frame.SecurityBlock{Type: byte(secure.SCS11)}
}

// onCryptogram consumes osdp_CCRYPT and prepares osdp_SCRYPT.
//
// A mismatch means the device cannot prove it holds the base key. That is a
// commissioning fault, not line noise, so the device is not offered another
// handshake: the application hears KindSecureFailed and decides whether a
// reader it cannot authenticate belongs on this bus.
func (b *Bus) onCryptogram(ctx context.Context, d *Device, data []byte) (Event, error) {
	if d.session == nil {
		return b.secureFailure(ctx, d, errNoSession), nil
	}

	scrypt, err := d.session.AnswerCryptogram(data)
	if err != nil {
		return b.secureFailure(ctx, d, err), nil
	}

	d.pending = &secureStep{
		msg:   cmd.Message{Code: cmd.SCrypt, Data: scrypt},
		block: frame.SecurityBlock{Type: byte(secure.SCS13)},
	}
	return b.event(ctx, Event{Kind: KindNone, Device: d}), nil
}

// onInitialRMAC consumes osdp_RMAC_I and establishes the session.
//
// The device echoes a value the panel derives independently, so a mismatch
// means the two disagree about the session rather than about the key. It is
// treated the same way regardless: an unestablished session is not used.
func (b *Bus) onInitialRMAC(ctx context.Context, d *Device, data []byte) (Event, error) {
	if d.session == nil {
		return b.secureFailure(ctx, d, errNoSession), nil
	}
	if err := d.session.CompleteHandshake(data); err != nil {
		return b.secureFailure(ctx, d, err), nil
	}

	d.State = Secure
	return b.event(ctx, Event{
		Kind:       KindSecure,
		Device:     d,
		DefaultKey: d.session.UsingDefaultKey(),
	}), nil
}

// secureFailure abandons the handshake and reports it.
//
// The device drops back to plaintext rather than off the bus. That is the
// conservative choice for a library: refusing to talk to a reader is a policy
// decision belonging to the panel, which has the event it needs to make it.
//
// cause never reaches the caller -- an unauthenticated device is an event, not
// an error return -- but it does reach the span, which is where somebody
// working out why a bus will not come up secure at three in the morning is
// going to look.
func (b *Bus) secureFailure(ctx context.Context, d *Device, cause error) Event {
	ctx, span := telemetry.Start(ctx, "osdp.secure.failed", d.Trace())
	span.RecordError(cause)
	defer span.End()

	d.abandonSecure()
	d.State = Online
	return b.event(ctx, Event{Kind: KindSecureFailed, Device: d})
}
