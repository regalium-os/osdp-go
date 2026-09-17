// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package transport

import (
	"errors"
	"time"
)

// Port is the byte-oriented connection the core speaks through.
//
// It is modelled on net.Conn deliberately: a deadline rather than a context,
// because an RS-485 turnaround is measured in milliseconds and the cost of a
// context per read matters on a bus polling hundreds of devices a second.
//
// Implementations live in the driver package. Nothing here opens anything.
//
// A Port must be safe for one reader and one writer concurrently, which is how
// a bus uses it. It need not support concurrent readers.
type Port interface {
	// Read reads available octets into p, blocking until at least one arrives
	// or the read deadline passes. A deadline expiry returns ErrTimeout.
	Read(p []byte) (int, error)

	// Write writes p in full. A short write is an error, not a partial
	// success: half a frame on the wire desynchronises every device sharing
	// the line.
	Write(p []byte) (int, error)

	// SetReadDeadline bounds how long Read blocks. A zero time clears it.
	SetReadDeadline(t time.Time) error

	// Close releases the port. It is safe to call more than once.
	Close() error
}

// WriteDeadliner is the optional half of a Port: a transport that can also
// bound how long a write blocks.
//
// It is deliberately not part of Port. Port is what every transport must
// satisfy, including one wrapping something that cannot time out a write at
// all, and widening it would break every implementation written against it.
//
// The runtime uses it when a port provides it, and it matters: a socket whose
// peer has stopped reading blocks a write indefinitely, and a runtime with no
// way out of that cannot honour a cancelled context. A port without it is still
// perfectly usable -- closing it is then the only way to interrupt a wedged
// write, which is a blunter shutdown but still a shutdown.
//
// Every transport in the driver package implements it, because all of them are
// built on net.Conn.
type WriteDeadliner interface {
	// SetWriteDeadline bounds how long Write blocks. A zero time clears it.
	SetWriteDeadline(t time.Time) error
}

// Line describes the physical parameters of a bus.
//
// It is data, not configuration to be acted on here: the driver interprets it.
// The core carries it so that a trace can say which line a frame came from
// without the core knowing what a baud rate is.
type Line struct {
	// Name identifies the line in logs and traces, for example "/dev/ttyUSB0"
	// or "panel-3:4001".
	Name string

	// Baud is the signalling rate. OSDP defines 9600, 19200, 38400, 57600,
	// 115200 and 230400; a device is required to support 9600 and negotiates
	// upward with osdp_COMSET.
	Baud int

	// Turnaround is how long to wait after transmitting before expecting a
	// reply. On a half-duplex RS-485 line the driver must stop driving before
	// the device can answer, and reading too early reads the tail of the
	// transmission back.
	Turnaround time.Duration

	// ReplyTimeout bounds how long a device has to answer before it is
	// considered offline for this cycle.
	ReplyTimeout time.Duration
}

// Transport errors.
var (
	// ErrTimeout reports that a read deadline passed with no octets. It is the
	// ordinary signal that a device did not answer, not a fault.
	ErrTimeout = errors.New("osdp/transport: read deadline exceeded")

	// ErrClosed reports use of a port that has been closed.
	ErrClosed = errors.New("osdp/transport: port is closed")

	// ErrShortWrite reports that a port accepted only part of a frame.
	ErrShortWrite = errors.New("osdp/transport: short write")
)
