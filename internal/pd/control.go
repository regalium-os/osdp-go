// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd

import (
	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
)

// The two commands that change the device rather than merely ask it something.
// Both are the reason the reply cache exists: acting twice on either has
// consequences that acting twice on a poll does not.

// outputEntrySize is one entry of an osdp_OUT payload: output number, control
// code, and a two-octet timer. SIA OSDP v2.2.2 §6.9.
const outputEntrySize = 4

// applyOutputs executes osdp_OUT and acknowledges it.
//
// This is the command that opens a door, and the one the retransmission rules
// are written for: an osdp_ACK lost on the way back brings the same command
// again with the same sequence number, and Handle replays the cached reply
// rather than reaching this function a second time. Without that, a strike
// released for five seconds would be released for five seconds twice, the
// second time after the cardholder has gone through and the door has shut.
//
// The permanent state is what is recorded here. A timed operation -- the usual
// way a door is released, so that a panel crashing mid-grant leaves a locked
// door rather than an open one -- needs a clock to expire, and a device with no
// loop has nowhere to run one; the output takes its permanent state and the
// timer is left to the hardware an application wires up behind a Handler.
//
// data is read and not retained.
//
// The lock must be held.
func (d *Device) applyOutputs(data []byte) cmd.Message {
	if len(data) == 0 || len(data)%outputEntrySize != 0 {
		return nak(cmd.NAKCommandLength)
	}

	// Every entry is checked before any is applied, and both ways an entry can
	// be wrong are checked here rather than one here and one in the loop below.
	// A command refused halfway is the worst outcome available: the strike is
	// energised, the panel is told the command failed, and it will not send the
	// off command for a door it believes never opened.
	for i := 0; i < len(data); i += outputEntrySize {
		if int(data[i]) >= len(d.outputs) {
			return nak(cmd.NAKUnsupportedInput)
		}
		if outputActionFor(cmd.OutputControl(data[i+1])) == outputUnknown {
			return nak(cmd.NAKUnsupportedInput)
		}
	}

	for i := 0; i < len(data); i += outputEntrySize {
		switch outputActionFor(cmd.OutputControl(data[i+1])) {
		case outputDeenergise:
			d.outputs[data[i]] = false
		case outputEnergise:
			d.outputs[data[i]] = true
		case outputLeave, outputUnknown:
			// osdp_OUT_NOP, which the validation pass above has already
			// separated from a code nobody defined.
		}
	}
	return ack()
}

// outputAction is what one osdp_OUT entry does to its output point, reduced to
// the three outcomes this device can represent.
type outputAction uint8

const (
	// outputUnknown is a control code the specification does not define. It is
	// deliberately the zero value: a lookup that fell through returns the
	// answer that refuses the command rather than the one that ignores it.
	outputUnknown outputAction = iota
	outputLeave
	outputDeenergise
	outputEnergise
)

// outputActionFor maps a control code to its outcome.
//
// It exists so that deciding whether an entry is valid and deciding what it
// does are the same decision, made once. They were two, and the validity half
// ran a loop later than the effect half -- which is exactly how a command comes
// to be half-applied and then refused.
//
// The permanent state is all that is recorded. The abort-versus-let-the-timer-
// finish distinction needs a running timer to be observable, and a device with
// no loop has nowhere to run one; see applyOutputs.
func outputActionFor(c cmd.OutputControl) outputAction {
	switch c {
	case cmd.OutputNOP:
		return outputLeave
	case cmd.OutputOffAbort, cmd.OutputOffAfterTimer, cmd.OutputTimedOff:
		return outputDeenergise
	case cmd.OutputOnAbort, cmd.OutputOnAfterTimer, cmd.OutputTimedOn:
		return outputEnergise
	default:
		return outputUnknown
	}
}

// applyCommunication executes osdp_COMSET and confirms it with osdp_COM.
// SIA OSDP v2.2.2 §6.14.
//
// # This is the command that can lose a device
//
// The address change takes effect after the reply has been built, so the panel
// is answered at the address it used and moves afterwards -- see emit. That is
// the specified behaviour and it has a hole in it that no implementation can
// close: if the osdp_COM reply is lost, the panel retries at the old address,
// which this device has already left. The panel half of this repository says
// the same thing from the other side; see cmd.CommunicationCommand. Commission
// one device at a time, and believe the reply rather than the request.
//
// The baud rate is echoed and not otherwise acted on. Line speed belongs to the
// transport, which this package does not own; a runtime driving a real port
// reads the confirmed rate out of the reply and reconfigures itself.
//
// data is read and not retained.
//
// The lock must be held.
func (d *Device) applyCommunication(data []byte) cmd.Message {
	want, err := cmd.ParseCommunication(data)
	if err != nil {
		return nak(cmd.NAKCommandLength)
	}

	addr := frame.Address(want.Address)
	if !addr.Valid() || addr == frame.BroadcastAddress {
		return nak(cmd.NAKUnsupportedInput)
	}
	d.adopting = &addr

	// The reply reports what was actually adopted, which is what a panel is
	// told to believe. Echoing the request instead would make a device that
	// took the address but not the speed indistinguishable from one that took
	// neither.
	return cmd.Message{
		Code: cmd.Com,
		Data: cmd.CommunicationCommand(cmd.Communication{
			Address: byte(addr),
			Baud:    want.Baud,
		}).Data,
	}
}
