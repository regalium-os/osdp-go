// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd

import (
	"context"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
)

// answer returns the message this command deserves.
//
// It never fails. A peripheral device that has been addressed owes the panel a
// reply, and every way this can go wrong has a reply of its own: osdp_NAK with
// a reason from SIA OSDP v2.2.2 §6.3. Returning an error instead would leave
// the runtime holding a command it could neither answer nor ignore.
//
// f.Data is read and not retained by anything here. The payloads built below
// are freshly allocated.
//
// The lock must be held.
func (d *Device) answer(ctx context.Context, f frame.Frame) cmd.Message {
	// A security block means the panel is speaking Secure Channel. This
	// package implements the plaintext state machine only, so the honest
	// answer is that encryption is not supported -- which is also what the
	// capability report said, so the panel is not being contradicted. See the
	// package documentation for where the device half of the handshake goes.
	if f.Security != nil {
		return nak(cmd.NAKEncryptionUnsup)
	}

	switch cmd.Code(f.Code) {
	case cmd.Poll:
		return d.pollReply()
	case cmd.ID:
		return cmd.Message{Code: cmd.PDID, Data: appendDeviceID(nil, d.cfg.identity)}
	case cmd.Cap:
		return cmd.Message{Code: cmd.PDCap, Data: d.cfg.capabilities.Append(nil)}
	case cmd.LStat:
		return d.localStatus()
	case cmd.IStat:
		return contactStatus(cmd.IStatR, d.inputs)
	case cmd.OStat:
		return contactStatus(cmd.OStatR, d.outputs)
	case cmd.RStat:
		return d.readerStatus()
	case cmd.Out:
		return d.applyOutputs(f.Data)
	case cmd.ComSet:
		return d.applyCommunication(f.Data)

	// The annunciator and housekeeping commands. A reader acknowledges these
	// whether or not it has the hardware to act on them: what an LED command
	// does to a device with one LED and no colours is the vendor's business,
	// and a panel that sends osdp_LED at enrolment -- most do -- must not
	// conclude the reader is broken because this library has no opinion about
	// lamps. An application that does have one supplies a Handler.
	case cmd.LED, cmd.Buz, cmd.Text, cmd.KeepActive, cmd.Abort, cmd.ACURxSize:
		return ack()

	// The Secure Channel commands, answered rather than ignored so that a
	// panel offering a channel learns immediately that it will not get one.
	case cmd.Chlng, cmd.SCrypt:
		return nak(cmd.NAKEncryptionUnsup)

	// osdp_KEYSET is refused with "secure channel required" and not with
	// "encryption not supported", because the two are different facts and the
	// panel acts on them differently. This device could store a key; what it
	// will not do is accept one that arrived in the clear, where anyone with a
	// pair of probes and a cupboard door now has it too. Per SIA OSDP v2.2.2
	// §6.16 a key install belongs inside an established session.
	case cmd.KeySet:
		return nak(cmd.NAKSecureRequired)
	}

	return d.delegate(ctx, f)
}

// delegate offers an unrecognised command to the application's Handler, and
// falls back to osdp_NAK reason 0x03, command not implemented.
//
// The fallback is the important half. A device that stayed silent on a command
// it did not know would look, at the panel, exactly like a device that had gone
// offline -- and the panel would spend its retry budget on a command no version
// of this device will ever answer.
//
// The lock must be held; see Handler for what that means for the application.
func (d *Device) delegate(ctx context.Context, f frame.Frame) cmd.Message {
	if d.cfg.handler != nil {
		m, ok := d.cfg.handler(ctx, cmd.Message{Code: cmd.Code(f.Code), Data: f.Data})
		if ok {
			return m
		}
	}
	return nak(cmd.NAKUnknownCommand)
}

// ack builds osdp_ACK: the device heard the command and has nothing to add.
func ack() cmd.Message { return cmd.Message{Code: cmd.ACK} }

// nak builds osdp_NAK carrying reason. SIA OSDP v2.2.2 §6.3.
//
// The payload is one octet. The specification allows a second, function-code
// specific octet after it, and this package does not send one: every reason it
// uses is self-contained, and a panel reading a trailing octet that means
// something different per reason is a panel that will eventually read it wrong.
func nak(reason cmd.NAKReason) cmd.Message {
	return cmd.Message{Code: cmd.NAK, Data: []byte{byte(reason)}}
}
