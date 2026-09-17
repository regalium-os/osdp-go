// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package bus

import (
	"context"

	"github.com/regalium-os/osdp-go/internal/cmd"
)

// contacts is the last state a device reported for one kind of contact.
type contacts struct {
	// known distinguishes "every input is inactive" from "no input has ever
	// been reported". Without it the first report after enrolment looks
	// identical to no change at all, and a panel coming up to a door that is
	// already standing open would never be told.
	known bool
	state []bool
}

// statusChanges turns a status report into the subset that is news.
//
// A device reports the state of every contact every time, but a panel wants
// transitions: a door that has been shut for a week is not an event, and
// treating it as one buries the door that just opened.
//
// The first report is the exception and is returned whole. A panel needs a
// complete baseline before it can call anything a change, and a contact that
// was already abnormal when the panel started is exactly the thing an operator
// needs to be told about at boot.
func (d *Device) statusChanges(reported []cmd.StatusChange) []cmd.StatusChange {
	if d.status == nil {
		d.status = map[cmd.StatusKind]*contacts{}
	}

	var changed []cmd.StatusChange
	for _, r := range reported {
		c, ok := d.status[r.Kind]
		if !ok {
			c = &contacts{}
			d.status[r.Kind] = c
		}

		for len(c.state) <= r.Index {
			c.state = append(c.state, false)
		}

		if !c.known || c.state[r.Index] != r.Active {
			changed = append(changed, r)
		}
		c.state[r.Index] = r.Active
	}

	// Mark every kind this report covered as established, after the whole
	// report is applied: a report covering inputs says nothing about outputs.
	for _, r := range reported {
		d.status[r.Kind].known = true
	}
	return changed
}

// Status reports the last known state of one kind of contact, and whether the
// device has ever reported it.
//
// The slice is a copy: a caller holding it cannot alter what the bus believes,
// and the bus overwriting it on the next report cannot alter what the caller
// read.
func (d *Device) Status(kind cmd.StatusKind) ([]bool, bool) {
	c, ok := d.status[kind]
	if !ok || !c.known {
		return nil, false
	}
	return append([]bool(nil), c.state...), true
}

// onStatus turns a status reply into an event, or into nothing at all.
//
// A report in which nothing moved produces KindNone rather than an event with
// an empty payload. On a bus polling hundreds of devices a second, an event per
// unchanged report is how an event stream becomes unreadable.
func (b *Bus) onStatus(
	ctx context.Context, d *Device,
	parse func([]byte) ([]cmd.StatusChange, error), data []byte,
) (Event, error) {
	reported, err := parse(data)
	if err != nil {
		return Event{}, err
	}

	changed := d.statusChanges(reported)
	if len(changed) == 0 {
		return b.event(ctx, Event{Kind: KindNone, Device: d}), nil
	}
	return b.event(ctx, Event{Kind: KindStatusChange, Device: d, Status: changed}), nil
}
