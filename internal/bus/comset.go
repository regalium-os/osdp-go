// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package bus

import (
	"context"
	"errors"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
)

// Communication configuration errors, matched with errors.Is.
var (
	// ErrAddressInUse reports a requested address another device on this bus
	// already answers to.
	//
	// Two devices sharing an address is not a state the protocol can recover
	// from by itself: both answer every command, their replies collide, and the
	// panel sees corruption it cannot attribute. Refusing is the only moment
	// anybody can prevent it.
	ErrAddressInUse = errors.New("osdp/bus: another device on this line uses that address")

	// ErrInvalidAddress reports an address outside 0x00 to 0x7E.
	//
	// The broadcast address is excluded deliberately: a device configured to
	// answer on it would reply to every command meant for every other device.
	ErrInvalidAddress = errors.New("osdp/bus: not a device address")
)

// SetCommunication queues an osdp_COMSET to move a device to a new address and
// line speed.
//
// # What happens afterwards
//
// The device applies the change after replying, so from the acknowledgement
// onward it is a different device as far as the line is concerned. The bus
// follows it: Device.Address becomes whatever the reply says the device
// adopted, the exchange restarts at sequence zero, and enrolment runs again --
// because a device that has just reconfigured its communications is not a
// device whose session, sequence or capability report can be assumed intact.
//
// It follows the reply rather than the request. A device may clamp a baud rate
// it cannot reach or refuse an address it does not like, and what it says it
// did is the only version that matters; believing the request instead is how a
// panel ends up talking confidently to nobody.
//
// # The baud rate
//
// Nothing here changes the speed of the local port -- that is the driver's, and
// there is no serial driver yet. On a serial-to-Ethernet converter the
// converter's own configuration governs, so changing a device's baud without
// changing the converter's is a way to lose it. Passing the current speed
// leaves it alone.
func (b *Bus) SetCommunication(d *Device, to cmd.Communication) error {
	if frame.Address(to.Address) >= frame.BroadcastAddress {
		return ErrInvalidAddress
	}
	for _, other := range b.devices {
		if other != d && other.Address == frame.Address(to.Address) {
			return ErrAddressInUse
		}
	}

	return b.Send(d, cmd.CommunicationCommand(to))
}

// onCommunication handles osdp_COM: the device reporting what it adopted.
func (b *Bus) onCommunication(ctx context.Context, d *Device, data []byte) (Event, error) {
	adopted, err := cmd.ParseCommunication(data)
	if err != nil {
		return Event{}, err
	}

	previous := d.Address
	d.Address = frame.Address(adopted.Address)

	// Everything the panel believed about this device was learned over the old
	// configuration. The exchange starts again from nothing, which is also
	// what the device does.
	d.resync()

	return b.event(ctx, Event{
		Kind:          KindCommunication,
		Device:        d,
		Communication: adopted,
		PreviousAddr:  previous,
	}), nil
}
