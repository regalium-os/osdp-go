// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package panel

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/internal/transport"
)

// scriptedPort hands back a prepared sequence of reads, which is how a line
// that fragments frames is reproduced without owning one.
type scriptedPort struct {
	chunks [][]byte
	reads  int
}

func (p *scriptedPort) Read(b []byte) (int, error) {
	if p.reads >= len(p.chunks) {
		return 0, transport.ErrTimeout
	}
	n := copy(b, p.chunks[p.reads])
	p.reads++
	return n, nil
}

func (p *scriptedPort) Write(b []byte) (int, error)     { return len(b), nil }
func (p *scriptedPort) SetReadDeadline(time.Time) error { return nil }
func (p *scriptedPort) Close() error                    { return nil }

// poll is osdp_POLL to address 0x00, CRC, sequence 1 -- the Phase 1 corpus
// vector, so the octets here are the same ones the frame tests trust.
var poll = []byte{0x53, 0x00, 0x08, 0x00, 0x05, 0x60, 0xDA, 0x99}

// ack is osdp_ACK from that device.
var ack = []byte{0x53, 0x80, 0x08, 0x00, 0x05, 0x40, 0x68, 0x9F}

func TestReaderAssemblesASplitFrame(t *testing.T) {
	// A converter that hands over three octets at a time is not hypothetical;
	// it is what a USB serial adapter does under load.
	r := newReader(&scriptedPort{chunks: [][]byte{poll[:3], poll[3:5], poll[5:]}})

	f, err := r.next(context.Background())
	if err != nil {
		t.Fatalf("next: %v", err)
	}
	if f.Code != 0x60 {
		t.Errorf("code = 0x%02X, want 0x60", f.Code)
	}
}

func TestReaderReturnsTwoFramesFromOneRead(t *testing.T) {
	// The second frame must survive in the buffer: discarding the remainder of
	// a read is how a busy line quietly loses every other reply.
	both := append(append([]byte{}, poll...), ack...)
	r := newReader(&scriptedPort{chunks: [][]byte{both}})
	ctx := context.Background()

	first, err := r.next(ctx)
	if err != nil {
		t.Fatalf("first frame: %v", err)
	}
	second, err := r.next(ctx)
	if err != nil {
		t.Fatalf("second frame: %v", err)
	}
	if first.IsReply || !second.IsReply {
		t.Errorf("got command=%v reply=%v, want the command then the reply",
			first.IsReply, second.IsReply)
	}
}

// TestReaderResynchronisesPastRubbish: a line shared with noise, or one joined
// mid-transmission, starts with octets that are not a frame. The frame behind
// them must still be found.
func TestReaderResynchronisesPastRubbish(t *testing.T) {
	noisy := append([]byte{0x11, 0x22, 0x33}, poll...)
	r := newReader(&scriptedPort{chunks: [][]byte{noisy}})

	f, err := r.next(context.Background())
	if err != nil {
		t.Fatalf("next: %v", err)
	}
	if f.Code != 0x60 {
		t.Errorf("code = 0x%02X, want the frame behind the noise", f.Code)
	}
}

// TestReaderSurfacesACorruptFrame: a frame whose check fails is evidence, not
// litter. It comes back with the error rather than being silently dropped.
func TestReaderSurfacesACorruptFrame(t *testing.T) {
	bad := append([]byte{}, poll...)
	bad[len(bad)-1] ^= 0xFF

	r := newReader(&scriptedPort{chunks: [][]byte{bad}})
	f, err := r.next(context.Background())

	if !errors.Is(err, frame.ErrBadCheck) {
		t.Fatalf("err = %v, want ErrBadCheck", err)
	}
	if f.Code != 0x60 {
		t.Error("the corrupt frame was not returned alongside the error")
	}
}

// TestReaderRefusesAnImplausibleLength: the length field arrives from the line
// and cannot be trusted. A frame claiming more than the buffer holds must fail
// rather than grow it.
func TestReaderRefusesAnImplausibleLength(t *testing.T) {
	huge := []byte{0x53, 0x00, 0xFF, 0xFF, 0x05, 0x60}
	filler := make([]byte, maxFrame)
	copy(filler, huge)

	r := newReader(&scriptedPort{chunks: [][]byte{filler}})
	if _, err := r.next(context.Background()); !errors.Is(err, frame.ErrLengthMismatch) {
		t.Errorf("err = %v, want ErrLengthMismatch", err)
	}
}

// TestReaderReportsSilence: no octets at all is a timeout, which the runtime
// turns into a missed poll rather than a failure.
func TestReaderReportsSilence(t *testing.T) {
	r := newReader(&scriptedPort{})
	if _, err := r.next(context.Background()); !errors.Is(err, transport.ErrTimeout) {
		t.Errorf("err = %v, want ErrTimeout", err)
	}
}
