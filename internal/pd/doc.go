// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

// Package pd implements the peripheral-device side of OSDP: what a reader does.
//
// # Layer contract
//
// Everything else in this repository is the control panel's half of the
// conversation. This is the other half: the state machine that waits to be
// polled, answers what it is asked, and never speaks first.
//
// It depends on the same codec the panel uses -- frame, cmd and secure are all
// direction-agnostic, and secure implements RolePD in full -- so this package
// owns sequencing and nothing else: which reply a command deserves, what to do
// with a sequence number it has seen before, and when a session must be torn
// down.
//
// # Using it
//
// A Device is constructed, not run. It starts no goroutine, owns no port and
// has no loop, because the panel supplies all of those: a device is a function
// from a command frame to a reply frame, over the state that makes the same
// command twice mean either two things or one.
//
//	device, err := pd.New(0x01,
//	    pd.WithIdentity(id),
//	    pd.WithContacts(4, 2, 1),
//	)
//	...
//	reply, err := device.Handle(ctx, command)
//	switch {
//	case errors.Is(err, pd.ErrNoReply):
//	    // not ours, or corrupt. Say nothing and read again.
//	case err != nil:
//	    return err
//	default:
//	    // encode reply onto the line
//	}
//
// The application's own half is ReportCardRead: a credential presented at the
// reader waits in a queue and is reported in place of the osdp_ACK the next
// poll would have got. SetInput, SetTamper and SetPower do the same for
// contacts.
//
// # Secure Channel: a seam, not an implementation
//
// This package implements the plaintext state machine only. That is a complete
// device -- most of the installed base runs exactly this -- but it is half of
// what the specification allows, and the omission is deliberate rather than
// pending.
//
// The device is coherent about it in both directions, which is the part that
// matters. Its default osdp_PDCAP reports osdp_CAP_COMMUNICATION_SECURITY at
// compliance zero, so a panel never offers a channel it would have to refuse;
// and an osdp_CHLNG that arrives anyway is answered with osdp_NAK reason 0x06,
// encryption not supported, rather than with silence. osdp_KEYSET is refused
// with reason 0x05, secure channel required, because a base key that arrives in
// the clear is a base key anyone with a pair of probes now holds.
//
// Everything needed to close the seam is already here. secure implements RolePD
// in full -- AnswerChallenge and AnswerServerCryptogram -- and a working device
// session driven against a real panel session lives in internal/bus's
// peripheral_test.go and handshake_test.go. What is missing is the sequencing
// around it: which security block a reply carries in each phase, where the
// message authentication code sits relative to the error check, and when a
// failed verification must tear the session down. The reply cache complicates
// it further, because a replayed reply must not advance the MAC chain. A half
// implementation of that is worse than none, so there is none.
//
// # Allowed imports
//
//	stdlib, frame, cmd, secure, transport, telemetry
//
// # Tracing
//
// Span osdp.pd.exchange wraps one command and its reply, and carries whether
// the reply was replayed from the cache. The application's reporting calls open
// osdp.pd.card, osdp.pd.keypad, osdp.pd.input, osdp.pd.tamper and
// osdp.pd.power.
//
// No span here ever carries a credential. osdp.pd.card records the format and
// the bit count, which is what an engineer debugging a reader needs;
// osdp.pd.keypad records nothing at all, because the length of a PIN is worth
// knowing to somebody guessing it.
package pd
