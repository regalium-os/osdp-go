// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package bus

import (
	"context"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/internal/secure"
)

// Turning a chosen command into octets. What to send is bus.go; this is the
// order the pieces have to go on in, which the specification fixes and none of
// which is interchangeable.

// compose turns a chosen command into the frame that carries it.
//
// The ordering is fixed by the specification and each step depends on the one
// before: the payload is enciphered first, because its length decides the
// frame's; then the frame is assembled so the length field is correct; then the
// message authentication code is computed over those octets; and only then is
// the error check written over everything. Doing any two of these in the other
// order produces a frame that will not authenticate.
func (b *Bus) compose(
	ctx context.Context, d *Device, msg cmd.Message, block *frame.SecurityBlock,
) (frame.Frame, error) {
	if block == nil {
		return cmd.Encode(ctx, msg, d.Address, d.nextSequence(), b.scheme)
	}

	kind := secure.BlockType(block.Type)
	if kind.Encrypted() {
		ciphertext, err := d.session.Seal(msg.Data, true)
		if err != nil {
			return frame.Frame{}, err
		}
		msg.Data = ciphertext
	}

	f, err := cmd.EncodeSecure(ctx, msg, d.Address, d.nextSequence(), b.scheme, *block)
	if err != nil {
		return frame.Frame{}, err
	}

	if kind.Established() {
		if err := authenticate(d.session, &f, true); err != nil {
			return frame.Frame{}, err
		}
	}

	f.Seal()
	return f, nil
}

// commandFor chooses what to ask a device, based on what is known about it,
// and the security block the command travels under if any.
//
// An offline or freshly returned device is asked what it is before anything
// else, then what it can do: a panel that starts issuing commands to a reader
// it has not identified is guessing about capabilities it could simply have
// asked for. Everything after that is an ordinary poll, wrapped by the secure
// channel once there is one.
func (b *Bus) commandFor(d *Device) (cmd.Message, *frame.SecurityBlock) {
	switch d.State {
	case Offline:
		return cmd.Message{Code: cmd.ID, Data: []byte{0x00}}, nil

	case Identifying:
		return cmd.Message{Code: cmd.Cap, Data: []byte{0x00}}, nil

	case SecureHandshake:
		return b.handshakeCommand(d)

	case Secure:
		if d.deferring() {
			block := blockForCommand(0)
			return pollMessage(), &block
		}
		if d.withholdsKeySet() {
			// Should not happen -- this state means the session is up -- but a
			// key is not worth a "should".
			block := blockForCommand(0)
			return pollMessage(), &block
		}
		m := d.outbound()
		block := blockForCommand(len(m.Data))
		return m, &block

	default:
		if d.deferring() || d.withholdsKeySet() {
			return pollMessage(), nil
		}
		return d.outbound(), nil
	}
}
