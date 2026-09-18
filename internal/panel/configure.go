// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package panel

import (
	"context"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/internal/secure"
)

// Reconfiguring a device rather than commanding one: its key and its place on
// the line. Both cross to the run loop the same way a command does, because
// both read state only that goroutine may touch.

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

// SetCommunication moves a device to a new address and line speed.
//
// The device applies the change after replying, so from the acknowledgement
// onward the panel addresses it at its new address and re-enrols it. What it
// actually adopted arrives on the event stream as EventCommunication --
// believe that rather than what was asked for, and persist it, because there
// is no command for asking a device what address it is on.
//
// It refuses an address another device on this line already uses, and the
// broadcast address. Both would put two devices on one address, which the
// protocol has no way to recover from.
//
// Nothing here changes the local port's speed: that belongs to the driver.
func (p *Panel) SetCommunication(
	ctx context.Context, addr frame.Address, to cmd.Communication,
) error {
	req := request{addr: addr, comms: &to, done: make(chan error, 1)}

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
