package driver

import (
	"errors"
	"io"
	"net"
	"time"

	"github.com/regalium-os/osdp-go/internal/transport"
)

// conn adapts a net.Conn to transport.Port.
//
// It is the shared body of every stream transport here. What it adds over the
// net.Conn it wraps is two things the protocol needs and the standard library
// deliberately does not provide: writes that are all-or-nothing, and timeouts
// reported as transport.ErrTimeout rather than as an opaque net.Error.
type conn struct {
	c    net.Conn
	name string
}

// Read reads available octets, mapping a deadline expiry to transport.ErrTimeout.
func (p *conn) Read(b []byte) (int, error) {
	n, err := p.c.Read(b)
	return n, translate(err)
}

// Write writes b in full.
//
// A short write is returned as an error rather than as a count the caller might
// retry. Half a frame on an RS-485 line is not a partial success: every device
// sharing that line is now reading a length field that will never be satisfied,
// and the whole bus stalls until the read times out.
func (p *conn) Write(b []byte) (int, error) {
	n, err := p.c.Write(b)
	if err != nil {
		return n, translate(err)
	}
	if n != len(b) {
		return n, transport.ErrShortWrite
	}
	return n, nil
}

// SetReadDeadline bounds how long Read blocks.
func (p *conn) SetReadDeadline(t time.Time) error { return translate(p.c.SetReadDeadline(t)) }

// Close releases the connection. It is safe to call more than once.
func (p *conn) Close() error {
	err := p.c.Close()
	if errors.Is(err, net.ErrClosed) || errors.Is(err, io.ErrClosedPipe) {
		return nil
	}
	return err
}

// Name identifies the port in traces.
func (p *conn) Name() string { return p.name }

// translate maps net errors onto the transport vocabulary, so the core never
// has to know what a net.Error is.
func translate(err error) error {
	if err == nil {
		return nil
	}
	// net.Pipe reports io.ErrClosedPipe where a socket reports net.ErrClosed.
	// Both mean the same thing to the core, and a caller must not have to know
	// which transport it was handed to find out.
	if errors.Is(err, net.ErrClosed) || errors.Is(err, io.ErrClosedPipe) {
		return transport.ErrClosed
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return transport.ErrTimeout
	}
	return err
}
