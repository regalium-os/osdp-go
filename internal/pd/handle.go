// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd

import (
	"context"
	"time"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/telemetry"
)

// Handle answers one command. It is the whole device.
//
// # What it returns
//
// A reply frame, sealed and ready for the wire, or an error. Three of those
// errors -- ErrNotAddressed, ErrNotCommand and ErrCorrupt -- mean "say nothing"
// rather than "something is wrong"; they all satisfy errors.Is(err, ErrNoReply)
// and a runtime seeing one should go back to reading the port. Anything else is
// a configuration fault, because a peripheral device that has been addressed
// always answers something, osdp_NAK included.
//
// # Sequence numbers, and why this is not a switch statement
//
// Per SIA OSDP v2.2.2 §5.7 the panel numbers its commands 1, 2, 3, 1, and
// repeats a number to mean "I did not hear you, say again". The device must
// answer a repeat by re-sending the reply it already gave, without acting on
// the command a second time. That single rule is what stops a door unlocking
// twice: an osdp_OUT that releases a strike, whose osdp_ACK is then lost to
// noise, arrives again with the same sequence number -- and a device that
// treated it as new would release the strike again, several seconds after the
// cardholder has walked through. The panel half of this repository relies on
// this; see internal/bus/sequence_test.go, which asserts that a retry repeats
// the number rather than advancing it.
//
// The ordering matters more than it looks on an established Secure Channel. A
// repeat is detected and replayed before the message authentication code is
// consulted, and it has to be: this end already verified that command once and
// advanced its command chain, so verifying the repeat would fail and tear down
// a session that is working perfectly. Replaying the cached frame is also
// correct for the other end, because a panel that never received the reply
// never advanced its reply chain either.
//
// Sequence zero is not part of the rotation. The panel uses it to restart the
// exchange -- after its own restart, or after losing track of a device -- and
// the device answers at zero, forgets its cached reply, and begins again. A
// zero exchange is never cached, because a second zero is a second restart and
// not a repeat; re-executing it is safe, as the only commands a panel sends at
// zero are the idempotent ones it starts a conversation with.
//
// # Ownership
//
// command is read and not retained. Its Data may alias the runtime's read
// buffer; nothing here keeps a reference to it past the call, including the
// cached reply, which is cloned.
//
// The returned frame does not alias command, which is the guarantee a runtime
// reusing one read buffer actually needs: the reply survives the next read.
//
// It does share storage with the reply cache, and deliberately. The cached
// reply and the returned one are the same octets, because a retransmission must
// reproduce them exactly; copying per exchange to guard against a caller that
// writes into a frame it was handed would be the same wrong trade frame.Frame
// declines to make on the decode path.
//
// So treat it as read-only. Encode it and let it go. Nothing in this package
// writes into a frame it has already returned -- a later exchange replaces the
// cache entry rather than modifying it -- so holding it, or reading it from
// another goroutine, is safe; mutating it corrupts what the next retransmission
// will send. Call Clone to get a copy that is yours.
//
// # Concurrency
//
// Safe for concurrent use, and serialised: one command is processed at a time.
func (d *Device) Handle(ctx context.Context, command frame.Frame) (frame.Frame, error) {
	ctx, span := telemetry.Start(ctx, "osdp.pd.exchange", command.Trace())
	defer span.End()

	if err := screen(command); err != nil {
		span.RecordError(err)
		return frame.Frame{}, err
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.addressed(command.Address) {
		span.RecordError(ErrNotAddressed)
		return frame.Frame{}, ErrNotAddressed
	}
	d.observe(d.cfg.clock.Now())

	seq := command.Control.Sequence()
	view := exchangeView{
		Address:  int(d.address),
		Sequence: int(seq),
		Command:  cmd.Code(command.Code).Name(false),
		Secure:   command.Security != nil,
	}

	switch {
	case seq == 0:
		d.restart()
	case d.cachedValid && seq == d.cachedSeq:
		view.Replayed = true
		view.Reply = cmd.Code(d.cached.Code).Name(true)
		telemetry.Apply(span, view)
		return d.cached, nil
	}

	m, block := d.answerFor(ctx, command, seq)
	reply, err := d.emit(ctx, command, seq, m, block)
	if err != nil {
		span.RecordError(err)
		return frame.Frame{}, err
	}

	view.Reply = cmd.Code(reply.Code).Name(true)
	telemetry.Apply(span, view)
	return reply, nil
}

// screen rejects the frames a device must not answer at all, in the order the
// decisions can be trusted.
//
// The direction test comes first because it is the only one that cannot be
// wrong: the reply flag is one bit and it either says another device is
// talking or it does not. The error check comes next, and it comes before the
// address test on purpose -- the address octet of a frame that failed its check
// is no more trustworthy than the rest of it, so there is nothing to be gained
// by asking who a corrupt frame claims to be for.
func screen(f frame.Frame) error {
	if f.IsReply {
		return ErrNotCommand
	}
	if !f.CheckOK() {
		return ErrCorrupt
	}
	return nil
}

// addressed reports whether this device should answer a. The lock must be held.
func (d *Device) addressed(a frame.Address) bool {
	return a == d.address || (d.cfg.configAddr && a == frame.BroadcastAddress)
}

// observe records that a command arrived, restarting the exchange first if the
// line has been silent for longer than WithCommunicationTimeout allows.
//
// A device has no loop, so it can only notice silence at the moment it ends --
// which is the moment that matters, because the command breaking the silence is
// almost certainly the opening of a new conversation with a panel that has been
// restarted. Restarting here rather than waiting for the panel to send sequence
// zero means a panel that resumes mid-rotation cannot be answered from a cache
// filled during the previous conversation.
//
// The lock must be held.
func (d *Device) observe(now time.Time) {
	if d.seen && d.cfg.timeout > 0 && now.Sub(d.lastSeen) > d.cfg.timeout {
		d.restart()
	}
	d.seen, d.lastSeen = true, now
}

// answerFor picks the message a command deserves, sequence discipline included,
// and the security block it travels under.
//
// The strict check lives here rather than in Handle because a sequence error is
// an answer like any other: it is emitted at the received sequence number, so
// the panel can actually read it, and it is cached like any other reply, so
// repeating the bad number repeats the NAK instead of producing a second one.
//
// A sequence error is refused in the clear even on an established session. The
// frame that provoked it has not been authenticated -- it cannot be, because
// checking the sequence is what happens before the message authentication code
// is consulted -- so sealing a reply to it would advance the chain on the word
// of something that may not be the panel at all.
//
// The lock must be held.
func (d *Device) answerFor(
	ctx context.Context, f frame.Frame, seq uint8,
) (cmd.Message, *frame.SecurityBlock) {
	if d.cfg.strict && d.cachedValid && seq != nextSequence(d.cachedSeq) {
		return nak(cmd.NAKSequenceError), nil
	}
	return d.answer(ctx, f)
}
