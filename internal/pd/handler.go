// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd

import (
	"context"

	"github.com/regalium-os/osdp-go/internal/cmd"
)

// Handler answers a command this package does not implement.
//
// It is the extension point for the families deliberately left out of the
// dispatch table -- osdp_MFG above all, but equally biometrics, file transfer
// and PIV. Vendor behaviour belongs in an osdp_MFG body per the architecture
// note in the repository's conventions, and this is where a device supplies it
// without this package growing a per-vendor fork of its own parser.
//
// It is consulted last, not first: every command in the dispatch table is
// answered before the handler is asked, so a handler cannot change what
// osdp_POLL or osdp_ID mean. Returning false declines the command, and the
// device answers osdp_NAK with reason 0x03, command not implemented -- which is
// the correct answer and the one a handler should fall back to rather than
// inventing a reply it is unsure of.
//
// # Ownership
//
// m.Data aliases the command frame, which aliases the runtime's read buffer.
// A handler that needs the payload beyond the call must copy it; cmd.Message
// has Clone for exactly this.
//
// The returned message's Data is copied before the reply reaches the wire, so a
// handler may return a slice it intends to reuse. Code is taken as given and is
// not checked against the reply range: a vendor answering osdp_MFG with
// osdp_MFGREP and a vendor answering it with osdp_ACK are both doing something
// real, and this package is not the place that decides which.
//
// # Concurrency
//
// A handler is called with the device's lock held, so it runs one at a time and
// must not call back into the Device -- reporting a card read from inside a
// handler would deadlock. Queue the work and report it from the caller's own
// goroutine.
type Handler func(ctx context.Context, m cmd.Message) (cmd.Message, bool)

// WithHandler supplies the Handler consulted for commands the dispatch table
// does not cover. A nil handler is ignored, which leaves the osdp_NAK default.
func WithHandler(h Handler) Option {
	return func(s *settings) {
		if h != nil {
			s.handler = h
		}
	}
}
