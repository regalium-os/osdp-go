// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"errors"
	"testing"
	"time"

	"github.com/regalium-os/osdp-go/internal/driver"
	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/internal/transport"
)

// TestPipeCarriesAFrame runs a real frame through a real Port, so the transport
// contract is exercised rather than asserted.
func TestPipeCarriesAFrame(t *testing.T) {
	panel, device := driver.Pipe()
	defer panel.Close()
	defer device.Close()

	f := frame.Frame{
		Address: 0x00,
		Control: frame.NewControl(1, frame.SchemeCRC16, false),
		Code:    0x60,
	}
	f.Seal()
	wire, err := f.Append(t.Context(), nil)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	go func() {
		_, _ = panel.Write(wire)
	}()

	buf := make([]byte, len(wire))
	if dlErr := device.SetReadDeadline(time.Now().Add(2 * time.Second)); dlErr != nil {
		t.Fatalf("SetReadDeadline: %v", dlErr)
	}
	n, err := device.Read(buf)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	got, err := frame.Decode(t.Context(), buf[:n])
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Code != f.Code || got.Address != f.Address {
		t.Errorf("frame did not survive the port: %+v", got)
	}
}

// TestReadDeadlineReportsTransportTimeout: a device not answering is the
// ordinary case, and the core must see it in its own vocabulary rather than as
// an opaque net.Error.
func TestReadDeadlineReportsTransportTimeout(t *testing.T) {
	panel, device := driver.Pipe()
	defer panel.Close()
	defer device.Close()

	if err := device.SetReadDeadline(time.Now().Add(20 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	_, err := device.Read(make([]byte, 8))
	if !errors.Is(err, transport.ErrTimeout) {
		t.Errorf("Read past the deadline = %v, want transport.ErrTimeout", err)
	}
}

func TestClosedPortReportsTransportClosed(t *testing.T) {
	panel, device := driver.Pipe()
	_ = device.Close()
	defer panel.Close()

	if _, err := device.Read(make([]byte, 4)); !errors.Is(err, transport.ErrClosed) {
		t.Errorf("Read on a closed port = %v, want transport.ErrClosed", err)
	}
}

// TestCloseIsIdempotent: Close is documented as safe to call more than once,
// which matters because a deferred Close and an explicit one often both run.
func TestCloseIsIdempotent(t *testing.T) {
	panel, device := driver.Pipe()
	defer device.Close()

	if err := panel.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := panel.Close(); err != nil {
		t.Errorf("second Close: %v, want nil", err)
	}
}
