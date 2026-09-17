// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package bus

import (
	"context"
	"errors"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/secure"
)

// Key installation errors, matched with errors.Is.
var (
	// ErrNotSecure reports an attempt to install a key on a device that has no
	// Secure Channel. It is refused rather than deferred because the command
	// carries the key as its payload: sending it unencrypted would publish the
	// site key to anyone on the wire, and there is no taking that back.
	ErrNotSecure = errors.New("osdp/bus: a key cannot be installed without a secure channel")
)

// InstallKey queues an osdp_KEYSET that moves a device onto a new base key.
//
// # What this is for
//
// A factory-fresh reader answers to SCBK-D, the default key printed in the
// specification. A channel built on it is authenticated against public
// knowledge, which is to say not authenticated at all; it exists so a panel can
// reach the device long enough to do exactly this. A deployment that leaves
// devices on SCBK-D has encryption and no security.
//
// # What it refuses
//
// The device must already have an established Secure Channel, because the
// command's payload is the key itself. On an unencrypted line this is a
// broadcast of the site key, and the mistake is unrecoverable -- the key is out,
// and every device it was installed on must be re-keyed. So it is an error at
// the call site rather than a decision the poll cycle makes later, and the
// check runs again before the frame goes out in case the channel dropped in
// between.
//
// # What happens next
//
// The command is queued like any other and sent when the cycle reaches the
// device, enciphered under the current session. When the device acknowledges
// it, the bus tears the session down -- both ends must now derive from the new
// key -- and re-handshakes with it on the following cycle, emitting
// KindKeyInstalled.
//
// The new key is adopted by this bus immediately, without waiting for the
// application to act on the event. That removes a race the application could
// not win: the next handshake can begin microseconds after the acknowledgement,
// long before a consumer has read from a channel. The event is the durability
// signal -- persist the key when it arrives, or the next run of the process
// will not be able to talk to the device.
//
// There is no length check because there cannot be a wrong length:
// secure.BaseKey is a fixed-size array, so the type system has already made the
// only mistake available here impossible to express.
//
// key is copied. The caller's array is not retained and should be zeroed once
// it has been persisted.
func (b *Bus) InstallKey(d *Device, key secure.BaseKey) error {
	if !d.secureEstablished() {
		return ErrNotSecure
	}

	pending := key
	d.pendingKey = &pending
	d.enqueue(cmd.KeySetCommand(key[:]))
	return nil
}

// secureEstablished reports whether this device has a Secure Channel that has
// completed its handshake. A session mid-challenge has proved nothing and
// enciphers nothing.
func (d *Device) secureEstablished() bool {
	return d.session != nil && d.session.Established()
}

// adoptPendingKey promotes the key a device has just acknowledged.
func (d *Device) adoptPendingKey() {
	if d.pendingKey == nil {
		return
	}
	adopted := *d.pendingKey
	d.installedKey = &adopted
	d.clearPendingKey()
}

// clearPendingKey discards the key in flight and zeroes it.
//
// Zeroing is defence in depth rather than a guarantee -- Go may have copied the
// array already, and nothing stops the compiler eliding the write. It shortens
// the window in which a core dump or a swapped page yields a site key, and
// costs nothing.
func (d *Device) clearPendingKey() {
	if d.pendingKey == nil {
		return
	}
	clear(d.pendingKey[:])
	d.pendingKey = nil
}

// baseKey returns the key to handshake with, and whether one is available.
//
// A key installed during this process wins over the application's keyring: the
// device is using it, so it is the only key that will work, whether or not the
// application has caught up.
func (d *Device) baseKey(lookup KeyFor) (secure.BaseKey, bool) {
	if d.installedKey != nil {
		return *d.installedKey, true
	}
	if lookup == nil {
		return secure.BaseKey{}, false
	}
	return lookup(d.Address)
}

// onKeyInstalled handles the acknowledgement of an osdp_KEYSET.
//
// The session is torn down rather than continued: it was derived from the key
// the device has just replaced, and the device has already stopped believing in
// it. Rebuilding from the new key is the only thing either end can do.
func (b *Bus) onKeyInstalled(ctx context.Context, d *Device) (Event, error) {
	installed := *d.pendingKey
	d.adoptPendingKey()

	// Back to the handshake, with the new key. Not resync: the device is
	// answering perfectly well and its sequence numbering is intact -- it is
	// only the session that has to be rebuilt.
	d.dropSession()
	d.State = Identifying

	return b.event(ctx, Event{
		Kind:       KindKeyInstalled,
		Device:     d,
		DefaultKey: installed.IsDefault(),
	}), nil
}

// keySetRefused handles a device declining the new key.
//
// The pending key is discarded and the session left alone. A device that
// refuses is still running on the key it had, so the channel it is talking on
// remains valid -- and a panel that tore the session down here would lose a
// working channel over a command that changed nothing.
func (b *Bus) keySetRefused(d *Device) {
	d.clearPendingKey()
}

// awaitingKeySet reports whether the reply being handled acknowledges an
// osdp_KEYSET.
//
// Both halves are needed. A pending key alone would match the acknowledgement
// of a poll that happened to be in flight when the key was queued; the code
// alone would match a device that was never asked to change keys.
func (d *Device) awaitingKeySet() bool {
	return d.pendingKey != nil && d.acknowledged == cmd.KeySet
}
