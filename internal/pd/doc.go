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
// direction-agnostic, and secure implements RolePD in full -- so what this
// package adds is sequencing and the loop around it: which reply a command
// deserves, what to do with a sequence number it has seen before, when a
// session must be torn down, and how to find a frame in a stream of octets that
// is mostly not one.
//
// # Using it
//
// A Device is constructed, not run. It starts no goroutine, owns no port and
// has no loop: a device is a function from a command frame to a reply frame,
// over the state that makes the same command twice mean either two things or
// one. That split is deliberate -- it is what lets a full enrolment, a secure
// handshake and a retransmission run as table tests with no port at all.
//
// Server is the other half, and the mirror of the panel runtime: it owns a
// transport.Port, assembles frames off a byte stream, and serves Handle until
// its context is cancelled.
//
//	server := pd.NewServer(device, port)
//	defer server.Close()
//	return server.Run(ctx) // nil when ctx is cancelled
//
// Drive Handle yourself instead when the octets arrive some other way -- a
// vendor bridge, a replayed capture, a test.
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
// A device that must also be secure adds a key and a source of randomness:
//
//	device, err := pd.New(0x01,
//	    pd.WithIdentity(id),
//	    pd.WithSecureChannel(registry.Standard(), key, randomNonce),
//	    pd.WithKeyInstalled(persist),
//	)
//
// # Secure Channel
//
// A device given WithSecureChannel speaks the AES-128 Secure Channel of SIA
// OSDP v2.2.2 §7: the four-message handshake, authenticated and enciphered
// traffic afterwards, and osdp_KEYSET to commission itself off the default key.
// Every octet of key material, cryptogram and message authentication code is
// secure's; what lives here is which message is due, which block carries it,
// and what each way of failing leaves behind.
//
// Without the option the device is a plaintext reader, which is a complete
// device and what most of the installed base is. It says so coherently in both
// directions: osdp_CAP_COMMUNICATION_SECURITY at compliance zero, so a panel
// never offers a channel that would have to be refused, and osdp_NAK reason
// 0x06 for an osdp_CHLNG that arrives anyway.
//
// Three rules are worth knowing before changing anything here.
//
// A retransmission replays, it does not reseal. The cached reply is returned
// before the message authentication code is consulted, and it has to be: this
// end verified that command once already and advanced its command chain, so
// verifying the repeat would fail and tear down a working session. The panel
// that lost the reply never advanced its reply chain either, so the two stay in
// step precisely because nothing was recomputed.
//
// A command chains from the last reply's code and a reply from the last
// command's. Skipping either direction desynchronises both ends, and because
// the enciphering initialisation vector is drawn from the same chain, the
// symptom is a frame that fails to authenticate two exchanges later rather than
// where the mistake was made.
//
// Plaintext on an established session is refused once and takes the session
// with it. The refusal is the security half -- a poll can be answered with a
// credential, and handing one back in the clear because somebody injected an
// unauthenticated frame is the downgrade the channel exists to prevent. The
// teardown is the interoperability half: from the next command the device is an
// ordinary plaintext reader again, so a panel that merely restarted is not
// locked out.
//
// # Allowed imports
//
//	stdlib, frame, cmd, secure, transport, telemetry
//
// # Tracing
//
// Span osdp.pd.exchange wraps one command and its reply, and carries whether
// the reply was replayed from the cache and whether the exchange was secured.
// The application's reporting calls open osdp.pd.card, osdp.pd.keypad,
// osdp.pd.input, osdp.pd.tamper and osdp.pd.power.
//
// No span here ever carries a credential, a key, a session key or a challenge
// nonce. osdp.pd.card records the format and
// the bit count, which is what an engineer debugging a reader needs;
// osdp.pd.keypad records nothing at all, because the length of a PIN is worth
// knowing to somebody guessing it.
package pd
