// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd

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

// scanner assembles whole frames from a byte-oriented port.
//
// This is the difference between a test and a line. A pipe hands over exactly
// one frame per Read; an RS-485 converter hands over whatever happened to be in
// its buffer, which may be half a frame, three frames, or a frame preceded by
// the tail of a collision.
//
// It is named for what it does rather than called a reader, because in this
// package a reader is the thing a card is presented to.
//
// A scanner is not safe for concurrent use. One goroutine drives a line.
type scanner struct {
	port transport.Port
	buf  []byte
	n    int
}

// newScanner returns a scanner over port, with the buffer allocated once.
func newScanner(port transport.Port) *scanner {
	return &scanner{port: port, buf: make([]byte, maxFrame)}
}

// next returns the next frame addressed at this line, reading until one arrives
// or the port fails.
//
// # What it does not return
//
// Garbage is not an error here, and that is the difference between this and the
// panel's equivalent. A control panel that cannot parse what came back has a
// decision to make about the device it was talking to; a peripheral device has
// no such decision, because it did not ask for anything. Noise, a collision
// between two other devices, the tail of a frame that began before this device
// powered up -- all of it is resynchronised past and none of it is worth
// stopping for. Only the port failing ends the scan.
//
// A frame whose error check fails IS returned, together with frame.ErrBadCheck.
// Deciding what to do with it belongs to Device.Handle, which stays silent; see
// ErrCorrupt for why silence rather than osdp_NAK.
//
// # Ownership
//
// The returned frame aliases the scanner's buffer and is valid only until the
// next call. Device.Handle does not retain it.
func (s *scanner) next(ctx context.Context) (frame.Frame, error) {
	for {
		if s.n > 0 {
			f, err := frame.Decode(ctx, s.buf[:s.n])
			switch {
			case err == nil || errors.Is(err, frame.ErrBadCheck):
				s.consume(f.WireLen())
				return f, err

			case errors.Is(err, frame.ErrShortBuffer):
				// Wait for more octets; the frame has not all arrived.

			default:
				// Not a frame at all. Drop to the next plausible start and try
				// again without reading, because the buffer may already hold a
				// good frame behind the rubbish.
				s.resync()
				continue
			}
		}

		if s.n == len(s.buf) {
			// A full buffer that still will not decode is a length field
			// nobody should believe. Discard it and listen again rather than
			// reporting a fault: the panel is still out there, and a device
			// that stopped listening after one noise burst is a dead reader.
			s.n = 0
		}

		read, err := s.port.Read(s.buf[s.n:])
		if err != nil {
			return frame.Frame{}, err
		}
		if read == 0 {
			return frame.Frame{}, transport.ErrTimeout
		}
		s.n += read
	}
}

// consume drops n octets from the front of the buffer, keeping whatever came
// after them: a single read often carries the next command as well.
func (s *scanner) consume(n int) {
	if n <= 0 || n > s.n {
		s.n = 0
		return
	}
	s.n = copy(s.buf, s.buf[n:s.n])
}

// resync drops octets up to the next plausible start of message.
//
// A mark octet counts as a start: it legitimately precedes one. Finding
// nothing empties the buffer, which is the right answer for a device that has
// been listening to a burst of noise.
func (s *scanner) resync() {
	for i := 1; i < s.n; i++ {
		if s.buf[i] == frame.SOM || s.buf[i] == frame.Mark {
			s.consume(i)
			return
		}
	}
	s.n = 0
}
