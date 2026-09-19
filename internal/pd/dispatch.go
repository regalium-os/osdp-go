// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd

import (
	"context"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
)

// answer returns the message this command deserves and the security block it
// travels under, nil for plaintext.
//
// It never fails. A peripheral device that has been addressed owes the panel a
// reply, and every way this can go wrong has a reply of its own: osdp_NAK with
// a reason from SIA OSDP v2.2.2 §6.3. Returning an error instead would leave
// the runtime holding a command it could neither answer nor ignore.
//
// f.Data is read and not retained. The payloads built below are freshly
// allocated.
//
// The lock must be held.
func (d *Device) answer(ctx context.Context, f frame.Frame) (cmd.Message, *frame.SecurityBlock) {
	if f.Security != nil {
		return d.answerSecure(ctx, f)
	}

	// Plaintext arriving on an established session. Per SIA OSDP v2.2.2 §7.4
	// this must not be answered as though the channel were never there: the
	// reply to a poll can be a credential, and handing one back in the clear
	// because somebody injected an unauthenticated frame is precisely the
	// downgrade the channel exists to prevent.
	//
	// So it is refused once, and the session is torn down with it. That second
	// half is what stops the refusal becoming a wedge: the panel has evidently
	// abandoned the channel -- it restarted, or its own session failed -- and
	// from the next command on this device is an ordinary plaintext reader
	// again, free to be challenged afresh whenever the panel is ready.
	if d.session != nil && d.session.Established() {
		d.dropSession()
		return nak(cmd.NAKSecureRequired), nil
	}

	return d.dispatch(ctx, f, false), nil
}

// dispatch is the command table, over a payload already in the clear.
//
// secured says whether the command arrived inside an established session. Only
// osdp_KEYSET consults it, because it is the only command whose meaning depends
// on how it travelled rather than on what it says.
//
// The lock must be held.
func (d *Device) dispatch(ctx context.Context, f frame.Frame, secured bool) cmd.Message {
	switch cmd.Code(f.Code) {
	case cmd.Poll:
		return d.pollReply()
	case cmd.ID:
		return cmd.Message{Code: cmd.PDID, Data: appendDeviceID(nil, d.cfg.identity)}
	case cmd.Cap:
		return cmd.Message{Code: cmd.PDCap, Data: d.capabilities.Append(nil)}
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

	// A handshake command with no security block around it. A device that can
	// speak Secure Channel says so -- put it in a block; one that cannot says
	// that instead.
	case cmd.Chlng, cmd.SCrypt:
		if d.secureEnabled() {
			return nak(cmd.NAKSecureRequired)
		}
		return nak(cmd.NAKEncryptionUnsup)

	// osdp_KEYSET is the one command that must never be obeyed in the clear.
	// The payload is the base key: a panel that sent it unencrypted has handed
	// the site key to anyone with a pair of probes and a cupboard door, and
	// nothing afterwards takes it back. Per SIA OSDP v2.2.2 §6.16 a key install
	// belongs inside an established session, and reason 0x05 says exactly that
	// -- which is a different fact from "I cannot encrypt", and a panel acts on
	// the two differently.
	case cmd.KeySet:
		if !secured {
			return nak(cmd.NAKSecureRequired)
		}
		return d.installKey(f.Data)
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
