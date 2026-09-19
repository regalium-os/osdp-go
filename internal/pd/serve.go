// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd

import (
	"context"
	"errors"

	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/internal/transport"
)

// The serving loop, and the two port operations it is made of. The Server it
// belongs to, and that type's lifecycle, are server.go.

// Run answers commands until ctx is cancelled or the port fails.
//
// # What stops it and what does not
//
// A cancelled context is not an error: Run returns nil. A port that failed --
// closed underneath it, a write that did not complete -- is returned as it came.
//
// Nothing else stops it, and that is the design. Noise, a collision between two
// other devices on the line, a frame with a broken error check, a command for
// somebody else, a command this device has never heard of: none of them are
// reasons for a reader to stop reading. A device that fell off the bus because
// of one corrupt frame is a door that stops working for a reason nobody can see
// from the panel, which reports only that the reader went quiet.
//
// # How quickly it stops
//
// Cancellation is observed between commands and, at worst, one idle period
// after it happens. It cannot be observed inside a blocked read, because no
// context interrupts a blocked syscall -- which is why reads are bounded by a
// deadline rather than left to wait for a panel that may never speak again. See
// WithIdleTimeout.
func (s *Server) Run(ctx context.Context) error {
	if s.started {
		return ErrAlreadyRun
	}
	s.started = true

	scan := newScanner(s.port)
	out := make([]byte, 0, maxFrame)

	for {
		// Cancellation is tested on the channel rather than by comparing
		// ctx.Err(), which is how every other cancellation point in this
		// repository is written.
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		command, err := s.read(ctx, scan)
		if errors.Is(err, transport.ErrTimeout) {
			// The panel has said nothing this period. That is not a fault; it
			// is most of the life of a reader on a quiet door.
			continue
		}
		if err != nil {
			return stopped(ctx, err)
		}

		// A frame that failed its error check is handed over exactly as it
		// arrived. Handle refuses it -- see ErrCorrupt -- and the refusal is
		// silence, which is the only safe answer when the address octet is as
		// suspect as the rest of the frame.
		reply, err := s.device.Handle(ctx, command)
		if errors.Is(err, ErrNoReply) {
			continue
		}
		if err != nil {
			return err
		}

		out, err = reply.Append(ctx, out[:0])
		if err != nil {
			return err
		}
		if err := s.write(out); err != nil {
			return stopped(ctx, err)
		}
	}
}

// stopped decides whether an error that ended the loop is a fault or a
// shutdown.
//
// Cancellation wins. Stopping usually makes the far end stop reading, so the
// last read or write can fail as a consequence of the stop rather than as
// anything worth reporting -- and a runtime that returned an error every time
// it was asked to shut down would teach its caller to ignore the error.
func stopped(ctx context.Context, err error) error {
	select {
	case <-ctx.Done():
		return nil
	default:
		return err
	}
}

// read bounds one scan by the idle timeout so that cancellation is observed.
//
// A frame whose error check failed comes back with a nil error, deliberately.
// It is still a frame, and deciding what to do with one the line corrupted
// belongs to Handle rather than to the thing that read it. Only the port
// failing is an error here.
func (s *Server) read(ctx context.Context, scan *scanner) (frame.Frame, error) {
	if err := s.port.SetReadDeadline(s.clock.Now().Add(s.idle)); err != nil {
		return frame.Frame{}, err
	}

	f, err := scan.next(ctx)
	if err != nil && !errors.Is(err, frame.ErrBadCheck) {
		return frame.Frame{}, err
	}
	return f, nil
}

// write puts the reply on the line, bounding it when the port allows.
//
// A port with no write deadline is not refused: transport.WriteDeadliner is
// deliberately optional, and a transport that cannot time out a write is still
// perfectly usable. Closing it is then the only way to interrupt a wedged
// write, which is blunter but still a shutdown.
func (s *Server) write(wire []byte) error {
	if d, ok := s.port.(transport.WriteDeadliner); ok {
		if err := d.SetWriteDeadline(s.clock.Now().Add(s.idle)); err != nil {
			return err
		}
	}

	n, err := s.port.Write(wire)
	if err != nil {
		return err
	}
	if n != len(wire) {
		// Half a frame on the wire desynchronises every device sharing the
		// line, so a short write is a failure rather than a partial success.
		return transport.ErrShortWrite
	}
	return nil
}
