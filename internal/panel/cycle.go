// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package panel

import (
	"context"
	"errors"
	"time"

	"github.com/regalium-os/osdp-go/internal/bus"
	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/internal/transport"
	"github.com/regalium-os/osdp-go/telemetry"
)

// cycle runs one transaction: ask the bus what to send, send it, wait for the
// answer, and tell the bus what came back.
//
// The order is the protocol's and none of it is negotiable. The turnaround wait
// sits between the write and the read because on a half-duplex line the panel
// must stop driving before the device can answer, and reading too early reads
// back the tail of its own transmission.
func (p *Panel) cycle(ctx context.Context, r *reader) error {
	ctx, span := telemetry.Start(ctx, "osdp.panel.cycle")
	defer span.End()

	step, ok := p.bus.Next(ctx)
	if !ok {
		return ErrNoDevices
	}

	wire, err := step.Frame.Append(ctx, nil)
	if err != nil {
		span.RecordError(err)
		return err
	}
	if err = p.send(wire, step.Timeout); err != nil {
		span.RecordError(err)
		return err
	}

	if err = p.clock.Sleep(ctx, p.line().Turnaround); err != nil {
		return err
	}

	ev, err := p.await(ctx, r, step)
	if err != nil {
		span.RecordError(err)
		return err
	}
	return p.emit(ctx, ev)
}

// await reads the answer to step and turns it into an event.
//
// Three things can happen and only one of them is an error. The device answers,
// and the bus says what the answer meant. The device stays silent, and the bus
// counts a miss -- which after enough of them is an offline event, not a
// failure. Or the port itself breaks, which is the caller's problem to fix.
func (p *Panel) await(ctx context.Context, r *reader, step bus.Step) (bus.Event, error) {
	if err := p.port.SetReadDeadline(p.clock.Now().Add(step.Timeout)); err != nil {
		return bus.Event{}, err
	}

	f, err := r.next(ctx)
	switch {
	case err == nil:
		// The bus decides what the reply means, including whether it came from
		// the right device and whether it authenticates.
		ev, rErr := p.bus.Reply(ctx, step.Device, f, p.clock.Now())
		if rErr != nil {
			// A reply the bus could not attribute to this exchange is line
			// noise that happened to have a valid check: another panel on the
			// same wire, or two devices sharing an address. It is not this
			// transaction's answer, so the device is treated as silent.
			//
			// The error stops here rather than being returned, because it says
			// nothing is wrong with this panel -- and a runtime that gave up on
			// a line whenever something else transmitted on it would be useless
			// on the multidrop bus this protocol exists for. It reaches the
			// span instead, which is where somebody tracing a duplicated
			// address will look.
			return p.unattributed(ctx, step, rErr), nil
		}
		return ev, nil

	case errors.Is(err, transport.ErrTimeout):
		return p.bus.Timeout(ctx, step.Device), nil

	case errors.Is(err, frame.ErrBadCheck), errors.Is(err, frame.ErrLengthMismatch),
		errors.Is(err, frame.ErrNoStartOfMessage), errors.Is(err, frame.ErrBadSecurityBlock):
		// A frame the line corrupted. The device did answer, but not with
		// anything that can be acted on, so it counts as silence: the bus will
		// re-poll it, and a run of these becomes an offline event the operator
		// can trace back to the wiring.
		return p.bus.Timeout(ctx, step.Device), nil

	default:
		return bus.Event{}, err
	}
}

// unattributed records a reply that belongs to no exchange of this panel's and
// treats the device as having stayed silent.
//
// Two devices answering to one address, or a second panel transmitting on the
// same wire, produce well-formed frames that are not answers to this command.
// The device is re-polled; a run of these becomes an offline event, and the
// span says why.
func (p *Panel) unattributed(ctx context.Context, step bus.Step, cause error) bus.Event {
	ctx, span := telemetry.Start(ctx, "osdp.panel.unattributed", step.Device.Trace())
	span.RecordError(cause)
	defer span.End()

	return p.bus.Timeout(ctx, step.Device)
}

// send puts a frame on the line, bounded when the port allows it.
//
// The bound is what keeps Run cancellable. A write to a socket whose peer has
// stopped reading never returns on its own, and a context cannot interrupt a
// blocked syscall -- so without a deadline the only way out is closing the
// port, which is a shutdown rather than a cancellation. A port that cannot set
// one still works; it just cannot be interrupted here.
//
// The same bound the device gets to answer is reused deliberately. A line too
// slow to accept a frame within a reply timeout is not a line that is about to
// carry a reply.
func (p *Panel) send(wire []byte, timeout time.Duration) error {
	if dl, ok := p.port.(transport.WriteDeadliner); ok {
		if err := dl.SetWriteDeadline(p.clock.Now().Add(timeout)); err != nil {
			return err
		}
	}

	_, err := p.port.Write(wire)
	return err
}

// line is the bus's line parameters, which carry the turnaround.
func (p *Panel) line() transport.Line { return p.bus.Line() }
