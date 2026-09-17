// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package panel

import (
	"context"
	"errors"

	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/internal/transport"
)

// maxFrame is the largest frame this runtime will assemble.
//
// The specification's ceiling is 1440 octets of payload; this allows for that
// plus the header, a security block and the error check, rounded up. The number
// matters because the length field is attacker-controlled on a shared line: a
// reader must not be talked into allocating whatever a noise burst claims.
const maxFrame = 1536

// reader assembles whole frames from a byte-oriented port.
//
// This is the difference between a test and a line. A pipe hands over exactly
// one frame per Read; an RS-485 converter hands over whatever happened to be in
// its buffer, which may be half a frame, three frames, or a frame preceded by
// the tail of a collision. Everything above this type gets to assume a frame.
//
// A reader is not safe for concurrent use. One goroutine drives a line.
type reader struct {
	port transport.Port
	buf  []byte
	n    int
}

// newReader returns a reader over port, with the buffer allocated once.
func newReader(port transport.Port) *reader {
	return &reader{port: port, buf: make([]byte, maxFrame)}
}

// next returns the next whole frame, reading until one arrives or the port's
// deadline passes.
//
// The returned frame aliases the reader's buffer and is only valid until the
// next call. Callers keeping a frame must Clone it -- and the bus does not, so
// an event carrying payload octets must be consumed before the next cycle.
//
// A frame whose error check fails is returned with frame.ErrBadCheck rather
// than swallowed: on a shared line a corrupt frame is the thing an operator
// most needs to see.
func (r *reader) next(ctx context.Context) (frame.Frame, error) {
	for {
		if r.n > 0 {
			f, err := frame.Decode(ctx, r.buf[:r.n])
			switch {
			case err == nil || errors.Is(err, frame.ErrBadCheck):
				r.consume(f.WireLen())
				return f, err

			case errors.Is(err, frame.ErrShortBuffer):
				// Wait for more octets; the frame has not all arrived.

			default:
				// Not a frame at all. Drop to the next plausible start and try
				// again without reading, because the buffer may already hold a
				// good frame behind the rubbish.
				if !r.resync() {
					return frame.Frame{}, err
				}
				continue
			}
		}

		if r.n == len(r.buf) {
			// A full buffer that still will not decode is a length field
			// nobody should believe. Discard and resynchronise.
			r.n = 0
			return frame.Frame{}, frame.ErrLengthMismatch
		}

		read, err := r.port.Read(r.buf[r.n:])
		if err != nil {
			return frame.Frame{}, err
		}
		if read == 0 {
			return frame.Frame{}, transport.ErrTimeout
		}
		r.n += read
	}
}

// consume drops n octets from the front of the buffer, keeping whatever came
// after them: a single read often carries the next frame as well.
func (r *reader) consume(n int) {
	if n <= 0 || n > r.n {
		r.n = 0
		return
	}
	r.n = copy(r.buf, r.buf[n:r.n])
}

// resync drops octets up to the next start of message and reports whether one
// was found. A mark octet is a start too: it legitimately precedes one.
func (r *reader) resync() bool {
	for i := 1; i < r.n; i++ {
		if r.buf[i] == frame.SOM || r.buf[i] == frame.Mark {
			r.consume(i)
			return true
		}
	}
	r.n = 0
	return false
}
