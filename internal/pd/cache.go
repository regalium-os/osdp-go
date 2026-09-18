// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd

import (
	"context"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
)

// The reply cache of SIA OSDP v2.2.2 §5.7, and the sequence rotation it is
// keyed on. Handle documents why it exists; this is how it works.

// sequenceRotation is the highest number in the panel's 1, 2, 3, 1 rotation.
const sequenceRotation = 3

// nextSequence returns the number that should follow s.
//
// Zero is absent from the rotation by design: it means "restart", so a device
// that expected zero to follow three would treat an ordinary wrap as a request
// to forget everything. The panel half computes the same rotation in
// internal/bus/sequence.go, and the two must agree or every third command looks
// like a sequence error.
func nextSequence(s uint8) uint8 {
	if s >= sequenceRotation {
		return 1
	}
	return s + 1
}

// emit encodes m as this device's reply, caches it, and returns it.
//
// # Why one cached reply and not four
//
// The cache holds a single reply and the sequence number it was given for,
// rather than a slot per number. That is not an economy, it is the correctness
// requirement. A panel only ever repeats the number of the command it is still
// waiting on, and it advances only once a reply arrives; so a four-slot cache
// keyed by sequence would be identical for one rotation and wrong on the next,
// replaying the answer given three commands ago to a genuinely new command
// carrying the same number. One slot cannot make that mistake: after 1, 2, 3
// the slot holds 3, and the 1 that follows is correctly seen as new.
//
// # Why the frame is cloned
//
// The reply must be replayable octet for octet, and cmd.Encode does not copy
// the payload it is handed. Cloning here means the cached frame aliases neither
// the command's read buffer nor any slice a Handler returned, so a replay months
// of polls later reproduces exactly what the panel was told the first time.
//
// A sequence-zero exchange invalidates the cache instead of filling it; see
// Handle for why a repeated zero must re-execute.
//
// The lock must be held.
func (d *Device) emit(
	ctx context.Context, command frame.Frame, seq uint8, m cmd.Message,
) (frame.Frame, error) {
	m.IsReply = true

	// The reply carries the sequence number and the check scheme of the
	// command that prompted it. Answering CRC-16 to a checksum panel, or the
	// reverse, is a reply the panel cannot verify and will retry forever.
	f, err := cmd.Encode(ctx, m, d.address, seq, command.Control.Scheme())
	if err != nil {
		return frame.Frame{}, err
	}
	reply := f.Clone()

	if seq == 0 {
		d.cachedValid = false
	} else {
		d.cached, d.cachedSeq, d.cachedValid = reply, seq, true
	}

	// osdp_COMSET takes effect only once the confirmation has been built, so
	// the panel is answered at the address it used. See applyCommunication.
	if d.adopting != nil {
		d.address, d.adopting = *d.adopting, nil
	}
	return reply, nil
}

// restart forgets everything belonging to one run of the exchange.
//
// Queued events deliberately survive. A card presented while the panel was
// rebooting is still a card that was presented, and the cardholder is still
// standing at the door; discarding it would turn a panel restart into a
// credential that was read and never reported.
//
// The lock must be held.
func (d *Device) restart() {
	d.cached, d.cachedSeq, d.cachedValid = frame.Frame{}, 0, false
}

// exchangeView is the projection of one exchange that may appear in a span.
//
// It is a separate type for the same reason frame.traceView is: what is not in
// this struct cannot be traced, and that is a boundary a reader can check in
// ten seconds. No payload field appears here, permanently -- a card read is a
// credential, and the reply that carries one is named but never described.
type exchangeView struct {
	Address  int    `telemetry:"trace:osdp.device.address"`
	Sequence int    `telemetry:"trace:osdp.frame.sequence"`
	Command  string `telemetry:"trace:osdp.command.name"`
	Reply    string `telemetry:"trace:osdp.pd.reply"`

	// Replayed marks a cached reply re-sent for a repeated sequence number.
	// It is the one attribute here worth alerting on: a line where replays are
	// routine is a line losing replies.
	Replayed bool `telemetry:"trace:osdp.pd.replayed"`
}
