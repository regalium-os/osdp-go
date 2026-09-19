// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

//go:build linux || darwin

package driver

import (
	"errors"
	"io"
	"os"
	"time"

	"github.com/regalium-os/osdp-go/internal/transport"
)

// serialPort is an open serial line, satisfying transport.Port and
// transport.WriteDeadliner.
//
// It wraps *os.File rather than reimplementing anything, because the runtime
// poller already provides exactly what a port needs: interruptible reads, real
// deadlines, and a Close that unblocks a goroutine parked in Read.
//
// Safe for one reader and one writer concurrently. Close may be called from
// any goroutine and more than once.
type serialPort struct {
	file *os.File
	name string
}

// Read reads whatever octets have arrived, blocking until at least one does or
// the read deadline passes.
//
// It returns fewer octets than a whole frame routinely: a serial line delivers
// what the UART has buffered, not what the sender meant as a unit. Reassembly
// is the caller's -- internal/panel does it.
//
// p is filled and not retained.
func (p *serialPort) Read(b []byte) (int, error) {
	n, err := p.file.Read(b)
	return n, translateSerial(err)
}

// Write writes b in full.
//
// A short write is an error rather than a count to retry: half a frame on an
// RS-485 line leaves every device reading a length field that will never be
// satisfied, and the whole bus stalls until they time out.
//
// b is read and not retained.
func (p *serialPort) Write(b []byte) (int, error) {
	n, err := p.file.Write(b)
	if err != nil {
		return n, translateSerial(err)
	}
	if n != len(b) {
		return n, transport.ErrShortWrite
	}
	return n, nil
}

// SetReadDeadline bounds how long Read blocks. A zero time clears it.
func (p *serialPort) SetReadDeadline(t time.Time) error {
	return translateSerial(p.file.SetReadDeadline(t))
}

// SetWriteDeadline bounds how long Write blocks. A zero time clears it.
//
// This is what keeps a panel cancellable when a port stops accepting output --
// a USB adapter unplugged mid-frame, or a converter whose buffer has filled.
// See transport.WriteDeadliner.
func (p *serialPort) SetWriteDeadline(t time.Time) error {
	return translateSerial(p.file.SetWriteDeadline(t))
}

// Close releases the port. It is safe to call more than once, and it unblocks
// a goroutine parked in Read.
func (p *serialPort) Close() error {
	err := p.file.Close()
	if errors.Is(err, os.ErrClosed) {
		return nil
	}
	return err
}

// Name identifies the port in logs and traces.
func (p *serialPort) Name() string { return p.name }

// translateSerial maps the errors a serial descriptor produces onto the ones
// the core understands.
//
// A caller must not have to know which transport it was handed to find out
// that a read timed out, which is why this mirrors translate in conn.go rather
// than letting os errors reach the bus.
func translateSerial(err error) error {
	switch {
	case err == nil:
		return nil

	case errors.Is(err, os.ErrDeadlineExceeded):
		return transport.ErrTimeout

	case errors.Is(err, os.ErrClosed):
		return transport.ErrClosed

	// A USB adapter pulled out of the socket reports EOF or ENXIO rather than
	// anything about closure. To the bus it means the same thing: this port is
	// finished, and no amount of retrying will bring it back.
	case errors.Is(err, io.EOF):
		return transport.ErrClosed

	default:
		return err
	}
}
