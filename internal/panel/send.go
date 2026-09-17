// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package panel

import (
	"context"
	"errors"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
)

// Command errors, matched with errors.Is.
var (
	// ErrUnknownDevice reports a command addressed to a device this panel does
	// not poll. It is a caller mistake rather than a line fault: the address
	// list is fixed when the bus is built, so an address not on it will never
	// become valid by waiting.
	ErrUnknownDevice = errors.New("osdp/panel: no such device on this line")

	// ErrNotRunning reports a command submitted to a panel whose Run has
	// returned. Queueing it would be a promise the runtime cannot keep.
	ErrNotRunning = errors.New("osdp/panel: the panel is not running")
)

// request is one application command waiting to reach the run loop.
type request struct {
	addr frame.Address
	msg  cmd.Message
	done chan error
}

// Send queues a command for one device and returns once the runtime has
// accepted it -- not once the device has acted on it.
//
// # Why acceptance and not delivery
//
// A line carries one exchange at a time, so a command waits for the cycle to
// reach its device: up to one full pass over the address list. Blocking until
// delivery would make an application's control flow hostage to the slowest
// reader on the bus, and blocking until the device acknowledged would make it
// hostage to a reader that has stopped answering. What actually happened
// arrives on the event stream, where an osdp_NAK is reported as one.
//
// # Concurrency
//
// Safe to call from any goroutine, including several at once, while Run is
// executing. This is the seam that keeps the bus itself single-threaded: the
// request crosses to the run loop here, and the bus is only ever touched by it.
//
// Send blocks while the runtime's queue is full, and returns ctx.Err() if the
// context is done first. It returns ErrNotRunning once Run has returned, and
// ErrUnknownDevice for an address this panel does not poll.
//
// m.Data is retained until the command is sent. Do not modify it afterwards.
func (p *Panel) Send(ctx context.Context, addr frame.Address, m cmd.Message) error {
	req := request{addr: addr, msg: m, done: make(chan error, 1)}

	select {
	case p.requests <- req:
	case <-p.stopped:
		return ErrNotRunning
	case <-ctx.Done():
		return ctx.Err()
	}

	select {
	case err := <-req.done:
		return err
	case <-p.stopped:
		return ErrNotRunning
	case <-ctx.Done():
		return ctx.Err()
	}
}

// drainRequests moves everything the application has submitted onto the bus.
//
// It runs on the run loop's goroutine and nowhere else, which is the whole
// point: the bus is documented as unsafe for concurrent use, and this is how
// that stays true while Send is called from anywhere.
//
// It never blocks. Whatever has arrived by now is queued; the rest waits for
// the next cycle, which is at most one transaction away.
func (p *Panel) drainRequests() {
	for {
		select {
		case req := <-p.requests:
			req.done <- p.queue(req)
		default:
			return
		}
	}
}

// queue puts one request on its device, or says why it cannot.
func (p *Panel) queue(req request) error {
	for _, d := range p.bus.Devices() {
		if d.Address == req.addr {
			p.bus.Send(d, req.msg)
			return nil
		}
	}
	return ErrUnknownDevice
}
