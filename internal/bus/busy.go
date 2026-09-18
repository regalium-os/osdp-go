// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package bus

import (
	"context"

	"github.com/regalium-os/osdp-go/internal/cmd"
)

// onBusy handles osdp_BUSY: the device received the command and could not act
// on it yet. SIA OSDP v2.2.2 §6.4.
//
// # Not a refusal and not a delivery
//
// This is the reply that is easiest to get wrong, because it looks like an
// answer and is not one. An osdp_NAK means the device considered the command
// and declined it, so retrying produces the same refusal forever. An osdp_BUSY
// means it never got that far -- it was writing to flash, or finishing a
// cryptographic operation for the credential somebody just presented -- and the
// command is still owed. Treating it as success loses the command silently,
// which for a door release is a door that does not open and a panel reporting
// that it did.
//
// # Why a poll comes next
//
// The command goes back on the queue, but the following exchange with this
// device is deliberately a poll rather than an immediate retry.
//
// A device that answers busy once will often answer busy again a millisecond
// later, and a bus that re-sends the same command every cycle would never poll
// it -- so a card presented at that reader during the busy period would never
// be reported. The poll both gives the device the moment it asked for and keeps
// events flowing. On a multidrop line the next cycle is already a full pass
// away; this makes the same true of a line with one device on it.
func (b *Bus) onBusy(ctx context.Context, d *Device) (Event, error) {
	// The device answered, so the exchange completed -- but what it said is
	// that the command was not acted on. It goes back on the queue.
	//
	// Not retransmit: that repeats the sequence number, which is right for a
	// reply lost on the wire and wrong here. A busy device cached no answer to
	// replay, so repeating the number would have it send the same osdp_BUSY
	// back forever.
	d.reclaim()
	d.deferCommands = true

	return b.event(ctx, Event{Kind: KindBusy, Device: d}), nil
}

// deferring reports whether the next exchange with this device must be a poll,
// and clears the flag.
//
// It is consumed rather than merely read: the backoff is one exchange, not a
// state the device sits in. A device that is still busy will say so again.
func (d *Device) deferring() bool {
	if !d.deferCommands {
		return false
	}
	d.deferCommands = false
	return true
}

// pollMessage is what the bus sends when it has nothing to say, or has been
// asked to wait.
func pollMessage() cmd.Message { return cmd.Message{Code: cmd.Poll} }
