// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd

import (
	"context"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/telemetry"
)

// The status replies of SIA OSDP v2.2.2 §6.6 to §6.8: one octet per contact,
// in device order, zero for the normal state and one for the asserted one.

// SetInput records the state of a monitored input and reports the change on the
// next poll.
//
// Both halves matter. The first answers osdp_ISTAT, which a panel asks at
// enrolment to establish a baseline; the second is how a door opening reaches
// the panel at all, because a panel that only learned about contacts when it
// asked would learn about a forced door on its next sweep rather than now.
//
// active is the asserted state as the specification defines it -- contact
// closed. Whether that means the door is open or shut depends on how the
// contact is wired, and that is the panel application's business.
//
// index must be below the input count given to WithContacts, or
// ErrNoSuchContact is returned; a device that accepted an input it does not
// have would report a status payload wider than its own capability report.
// ErrQueueFull means the change was not queued and the panel will not hear
// about it until the next one.
//
// Safe for concurrent use.
func (d *Device) SetInput(ctx context.Context, index int, active bool) error {
	_, span := telemetry.Start(ctx, "osdp.pd.input")
	defer span.End()

	d.mu.Lock()
	defer d.mu.Unlock()

	if index < 0 || index >= len(d.inputs) {
		span.RecordError(ErrNoSuchContact)
		return ErrNoSuchContact
	}
	d.inputs[index] = active

	err := d.coalesce(contactStatus(cmd.IStatR, d.inputs))
	span.RecordError(err)
	return err
}

// SetTamper records the device's tamper switch and reports it on the next poll.
//
// This is the one status change nobody should be able to miss: a reader being
// pulled off a wall is an attack in progress, not a reading. It is reported
// through osdp_LSTATR along with the power state, which is why changing either
// re-sends both -- the reply carries the pair and there is no way to send half
// of it. SIA OSDP v2.2.2 §6.6.
//
// Safe for concurrent use.
func (d *Device) SetTamper(ctx context.Context, active bool) error {
	return d.setLocal(ctx, "osdp.pd.tamper", func() { d.tamper = active })
}

// SetPower records whether the device is running on backup power and reports it
// on the next poll.
//
// A reader on backup power is a reader that is about to stop answering, which
// is worth knowing before it does rather than as an unexplained offline event.
// SIA OSDP v2.2.2 §6.6.
//
// Safe for concurrent use.
func (d *Device) SetPower(ctx context.Context, failure bool) error {
	return d.setLocal(ctx, "osdp.pd.power", func() { d.powerFailure = failure })
}

// setLocal applies a change to the local status pair and queues the report the
// two of them share.
func (d *Device) setLocal(ctx context.Context, span string, apply func()) error {
	_, s := telemetry.Start(ctx, span)
	defer s.End()

	d.mu.Lock()
	defer d.mu.Unlock()

	apply()
	err := d.coalesce(d.localStatus())
	s.RecordError(err)
	return err
}

// localStatus builds osdp_LSTATR: tamper, then power. SIA OSDP v2.2.2 §6.6.
//
// The lock must be held.
func (d *Device) localStatus() cmd.Message {
	return cmd.Message{
		Code: cmd.LStatR,
		Data: []byte{boolOctet(d.tamper), boolOctet(d.powerFailure)},
	}
}

// readerStatus builds osdp_RSTATR: one octet per reader head, zero meaning the
// head is connected and untampered. SIA OSDP v2.2.2 §6.8.
//
// Every head reports well. A reader head fault is a property of hardware this
// package does not model, and a device inventing one would have the panel chase
// a fault that does not exist. An application with real heads to report
// supplies a Handler, or overrides the reply through one.
//
// The lock must be held.
func (d *Device) readerStatus() cmd.Message {
	return cmd.Message{Code: cmd.RStatR, Data: make([]byte, d.cfg.readers)}
}

// contactStatus builds one octet per contact, in device order.
//
// The returned message owns its payload: state is read, never retained. That
// matters because the payload is queued for a later poll while the device keeps
// mutating the slice it was built from.
func contactStatus(code cmd.Code, state []bool) cmd.Message {
	data := make([]byte, len(state))
	for i, active := range state {
		data[i] = boolOctet(active)
	}
	return cmd.Message{Code: code, Data: data}
}

// boolOctet is the wire encoding of a contact state: zero normal, one asserted.
func boolOctet(active bool) byte {
	if active {
		return 1
	}
	return 0
}
