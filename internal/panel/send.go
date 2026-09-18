// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package panel

import (
	"context"
	"errors"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/internal/secure"
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
//
// A key installation travels the same path as a command, because it must happen
// on the same goroutine: it inspects the device's secure session, and the bus
// is not safe to touch from anywhere else.
type request struct {
	addr frame.Address
	msg  cmd.Message
	key  *secure.BaseKey
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
// Send blocks until the run loop accepts the command, and returns ctx.Err() if
// the context is done first. It returns ErrNotRunning once Run has returned,
// and ErrUnknownDevice for an address this panel does not poll.
//
// # It blocks for longer than the queue being full
//
// The run loop accepts commands between transactions, so anything that stops
// the poll cycle stops Send with it -- and the likeliest such thing is not a
// full request queue but an event consumer that has stopped reading. The
// runtime halts the cycle when the event buffer fills, by design (see Events),
// and a halted cycle never gets back to the requests.
//
// A caller that reads one event and then stops, while another goroutine sends,
// therefore deadlocks: each is waiting for the other, and Send has no deadline
// of its own -- only the context ends it. **Drain Events continuously for as
// long as Run is executing.** It is the runtime's one real obligation on a
// caller, and it is easy to meet by accident and easy to break by accident.
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
		if d.Address != req.addr {
			continue
		}
		if req.key != nil {
			return p.bus.InstallKey(d, *req.key)
		}
		p.bus.Send(d, req.msg)
		return nil
	}
	return ErrUnknownDevice
}

// InstallKey moves a device onto a new Secure Channel base key.
//
// The device must already have an established Secure Channel: the command's
// payload is the key, so on an unencrypted line this would publish the site key
// to anyone on the wire. That is refused here and again before the frame goes
// out, in case the channel drops in between.
//
// Like Send, it returns once the runtime has accepted the request. The device's
// acknowledgement arrives on the event stream as EventKeyInstalled -- persist
// the key then, because the bus adopts it immediately but a later run of this
// process starts from whatever the application's keyring says.
//
// key is copied. Zero the caller's array once it has been persisted.
func (p *Panel) InstallKey(ctx context.Context, addr frame.Address, key secure.BaseKey) error {
	req := request{addr: addr, key: &key, done: make(chan error, 1)}

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
