// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd_test

import (
	"io"
	"sync"
	"testing"
	"time"

	"github.com/regalium-os/osdp-go/internal/transport"
)

// fakePort is a transport.Port a test drives both ends of.
//
// It is written here rather than borrowed from the driver package because pd
// may not import driver -- the layer contract points the other way, and a
// consumer declaring the interface it needs at its own boundary is the whole
// reason transport.Port exists. Forty lines is a cheap price for the
// architecture staying honest.
//
// It is safe for concurrent use: the test goroutine feeds octets in while the
// server reads them out.
type fakePort struct {
	mu     sync.Mutex
	inbox  []byte
	sent   []byte
	closed bool

	// deadline is honoured only to the extent a test needs: a read with
	// nothing to return reports ErrTimeout rather than blocking, so no test
	// ever waits on wall-clock time.
	deadlineSet bool

	// idle carries one signal per read that found nothing. It is how a test
	// waits for the server to finish rather than for the clock: an idle read
	// can only happen after every octet fed has been scanned, handled, and any
	// reply written. Buffered and sent to without blocking, so the server is
	// never held up by a test that has stopped listening.
	idle chan struct{}
}

// newFakePort returns a port with its progress signal ready.
func newFakePort() *fakePort {
	return &fakePort{idle: make(chan struct{}, 16)}
}

// feed queues octets for the server to read, as a panel would put them on the
// line.
func (p *fakePort) feed(b []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.inbox = append(p.inbox, b...)
}

// written returns everything the server has transmitted.
func (p *fakePort) written() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]byte(nil), p.sent...)
}

// Read implements transport.Port.
func (p *fakePort) Read(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return 0, transport.ErrClosed
	}
	if len(p.inbox) == 0 {
		// An idle line. The server treats this as "nothing said this period",
		// which is most of the life of a reader on a quiet door.
		select {
		case p.idle <- struct{}{}:
		default:
		}
		return 0, transport.ErrTimeout
	}

	n := copy(b, p.inbox)
	p.inbox = p.inbox[n:]
	return n, nil
}

// Write implements transport.Port.
func (p *fakePort) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return 0, transport.ErrClosed
	}
	p.sent = append(p.sent, b...)
	return len(b), nil
}

// SetReadDeadline implements transport.Port.
func (p *fakePort) SetReadDeadline(time.Time) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return transport.ErrClosed
	}
	p.deadlineSet = true
	return nil
}

// Close implements transport.Port. It is idempotent, as the interface requires.
func (p *fakePort) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	return nil
}

// awaitIdle blocks until the server has read an empty line twice, which means
// it has processed everything fed to it and written whatever it meant to.
func (p *fakePort) awaitIdle(t *testing.T) {
	t.Helper()

	for range 2 {
		select {
		case <-p.idle:
		case <-time.After(5 * time.Second):
			t.Fatal("the server never came back for more octets")
		}
	}
}

// failingPort is a port whose reads fail outright, standing in for a cable
// pulled out rather than a line that has gone quiet.
type failingPort struct{ fakePort }

// Read always fails.
func (p *failingPort) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
