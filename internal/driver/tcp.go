// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"context"
	"net"

	"github.com/regalium-os/osdp-go/internal/transport"
)

// DialTCP opens an OSDP connection over TCP.
//
// Serial-to-Ethernet converters are how most installed OSDP buses reach a panel
// that is not in the same cupboard, so this is a first-class transport rather
// than a testing convenience. The framing is identical; only the medium differs.
//
// The returned Port must be closed by the caller.
func DialTCP(ctx context.Context, address string) (transport.Port, error) {
	var d net.Dialer
	c, err := d.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	// Nagle would coalesce a command with whatever follows it, and OSDP is a
	// request/response protocol on a line where turnaround timing is the whole
	// budget. Send each frame when it is ready.
	if tcp, ok := c.(*net.TCPConn); ok {
		if err := tcp.SetNoDelay(true); err != nil {
			_ = c.Close()
			return nil, err
		}
	}
	return &conn{c: c, name: address}, nil
}

// ListenTCP accepts an OSDP connection, for a peripheral device or a simulator
// that a panel dials into.
func ListenTCP(ctx context.Context, address string) (net.Listener, error) {
	var lc net.ListenConfig
	return lc.Listen(ctx, "tcp", address)
}

// Accept wraps an accepted connection as a Port.
func Accept(l net.Listener) (transport.Port, error) {
	c, err := l.Accept()
	if err != nil {
		return nil, err
	}
	if tcp, ok := c.(*net.TCPConn); ok {
		if err := tcp.SetNoDelay(true); err != nil {
			_ = c.Close()
			return nil, err
		}
	}
	return &conn{c: c, name: c.RemoteAddr().String()}, nil
}
