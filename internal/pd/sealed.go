// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd

import (
	"context"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/internal/secure"
)

// Opening and sealing frames on an established session, SIA OSDP v2.2.2 §7.4.
// The order of operations here is the specification's and every step depends on
// the one before it; getting it wrong produces frames that verify against
// themselves and against nothing else.

// open verifies a command frame and returns its payload in the clear.
//
// # Ownership
//
// The returned slice aliases f.Data when the block is authenticated only, and
// is freshly allocated when the payload was enciphered. Neither outlives the
// caller's use of f, and the dispatch table copies whatever it keeps.
//
// The lock must be held.
func (d *Device) open(f frame.Frame) ([]byte, error) {
	if len(f.Data) < secure.MACTagSize {
		return nil, secure.ErrMACMismatch
	}

	// The code covers the finished frame: header, security block, command code
	// and payload, with the length field already reporting the code and the
	// error check that follow. Authenticating anything else authenticates an
	// octet string the sender never built.
	body := f.AppendBody(nil)
	split := len(f.Data) - secure.MACTagSize

	if err := d.session.Verify(body[:len(body)-secure.MACTagSize], f.Data[split:], true); err != nil {
		return nil, err
	}

	payload := f.Data[:split]
	if !secure.BlockType(f.Security.Type).Encrypted() {
		return payload, nil
	}

	// Deciphering happens only after the code has verified. AES-CBC is
	// malleable, so an unverified payload is whatever the attacker chose.
	return d.session.Open(payload, true)
}

// sealReply builds an authenticated reply, enciphering the payload when the
// block calls for it.
//
// Each step depends on the one before: the payload is enciphered, the frame is
// assembled with room for the code reserved so that the length field is already
// final, the code is computed over every octet preceding it, and the error
// check is written last over the finished frame.
//
// m.Data is neither retained nor modified; the returned frame owns its payload.
//
// The lock must be held.
func (d *Device) sealReply(
	ctx context.Context, m cmd.Message, seq uint8, scheme frame.Scheme, block frame.SecurityBlock,
) (frame.Frame, error) {
	payload := m.Data
	if secure.BlockType(block.Type).Encrypted() {
		ciphertext, err := d.session.Seal(payload, false)
		if err != nil {
			return frame.Frame{}, err
		}
		payload = ciphertext
	}

	data := make([]byte, len(payload)+secure.MACTagSize)
	copy(data, payload)

	f, err := cmd.EncodeSecure(
		ctx, cmd.Message{Code: m.Code, IsReply: true, Data: data},
		d.address, seq, scheme, block,
	)
	if err != nil {
		return frame.Frame{}, err
	}

	body := f.AppendBody(nil)
	tag, err := d.session.Authenticate(body[:len(body)-secure.MACTagSize], false)
	if err != nil {
		return frame.Frame{}, err
	}
	copy(f.Data[len(f.Data)-secure.MACTagSize:], tag[:])

	f.Seal()
	return f, nil
}

// installKey executes osdp_KEYSET on an established session.
//
// The new key takes effect only after the acknowledgement has been sealed with
// the old session -- see emit, which applies it -- because a device that
// switched first would authenticate its own ACK with a key the panel has not
// adopted yet, and the commissioning would fail at the last step with the key
// already installed at one end.
//
// Adopting a key ends the session. Both ends must handshake again to derive
// session keys from the new base key, which is what the panel expects: the bus
// tears its own session down on the same command.
//
// data is read and not retained; the key is copied out of it.
//
// The lock must be held.
func (d *Device) installKey(data []byte) cmd.Message {
	payload, err := cmd.ParseKeySet(data)
	if err != nil {
		return nak(cmd.NAKCommandLength)
	}

	// osdp_KEYSET defines exactly one key type, and a key of the wrong length
	// is not a shorter key -- it is a key neither end can derive from. Taking
	// it would leave a reader holding something that is not an SCBK and no way
	// to say so.
	if payload.Type != cmd.KeySCBK || len(payload.Key) != secure.BlockSize {
		return nak(cmd.NAKUnsupportedInput)
	}

	key := secure.BaseKey(payload.Key)
	d.adoptingKey = &key
	return ack()
}

// adoptKey applies a key install once the reply that confirmed it has been
// sealed. The lock must be held.
func (d *Device) adoptKey(ctx context.Context) {
	if d.adoptingKey == nil {
		return
	}
	key := *d.adoptingKey
	d.adoptingKey = nil

	d.key = key
	d.dropSession()

	// The capability report carries a default-key bit, and a device that has
	// just been given a real key must stop advertising that it is running on
	// the one printed in the specification.
	d.refreshCapabilities()

	if d.cfg.keyInstalled != nil {
		d.cfg.keyInstalled(ctx, key)
	}
}
