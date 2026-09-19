// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd

import (
	"errors"
	"sync"
	"time"

	"github.com/regalium-os/osdp-go/internal/transport"
)

// Server drives a Device against a real line: the peripheral-device mirror of
// the control panel's runtime.
//
// The asymmetry with the panel side is the protocol's, not this package's. A
// panel decides what to send and when, so its runtime owns a poll cycle and a
// clock to pace it. A device decides nothing: it reads, answers, and reads
// again. There is no cycle here because a device has no initiative -- it is the
// server in an exchange the panel always starts.
//
// # Lifecycle
//
// New constructs and starts nothing. Run blocks until the context is cancelled
// or the port fails, and owns everything it started. Close is idempotent and
// safe after a failed Run. No goroutine outlives Run -- in fact none is
// started: the loop runs on the caller's goroutine, because there is nothing
// for a second one to do.
//
// # Concurrency
//
// A Server must be Run once, from one goroutine. The Device it serves stays
// safe for concurrent use, which is the point: the application reports a card
// read from wherever it noticed one while this loop is blocked in a read.
type Server struct {
	device *Device
	port   transport.Port
	clock  Clock
	idle   time.Duration

	started   bool
	closeOnce sync.Once
	closeErr  error
}

// ErrAlreadyRun reports a second call to Run. A Server drives one line once;
// the buffers and the sequence state it accumulated belong to that run.
var ErrAlreadyRun = errors.New("osdp/pd: server has already been run")

// NewServer returns a server that answers for device on port.
//
// It starts nothing and touches nothing -- not even a read deadline -- so a
// server can be constructed, inspected and discarded without an octet reaching
// a line. The port is adopted: Close closes it.
func NewServer(device *Device, port transport.Port, opts ...ServerOption) *Server {
	cfg := serverDefaults()
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return &Server{device: device, port: port, clock: cfg.clock, idle: cfg.idle}
}

// Close releases the port. It is idempotent and safe after a failed Run.
//
// It does not stop Run; cancelling the context does that. Closing the port out
// from under a running server makes Run return the port's error, which reports
// as a failure rather than a clean stop.
func (s *Server) Close() error {
	s.closeOnce.Do(func() { s.closeErr = s.port.Close() })
	return s.closeErr
}
