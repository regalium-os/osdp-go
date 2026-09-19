// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

//go:build linux || darwin

package driver_test

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/regalium-os/osdp-go/internal/driver"
	"github.com/regalium-os/osdp-go/internal/transport"
)

// A pty is a real tty: the same termios ioctls apply, the same poller handles
// it, and the same deadlines work. It is not an RS-485 line -- there is no
// transceiver, no turnaround and no noise -- but everything between DialSerial
// and the wire is exercised, which is everything this package owns.
//
// What still needs hardware: that a real adapter turns its transceiver around
// fast enough to hear the reply, and that a device at the far end agrees about
// the line speed. Neither is testable here, and neither is this package's to
// get wrong.

// openPTY returns a master and the path of its slave, or skips.
func openPTY(t *testing.T) (*os.File, string) {
	t.Helper()

	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("no /dev/ptmx on this machine: %v", err)
	}
	t.Cleanup(func() { _ = master.Close() })

	name, err := ptySlaveName(master)
	if err != nil {
		t.Skipf("cannot name the pty slave: %v", err)
	}
	return master, name
}

// TestTheLineSettingsApplyToARealTTY.
//
// The whole termios path -- open, raw mode, speed -- against a descriptor the
// kernel treats exactly as it treats a serial port. A failure here is a wrong
// ioctl number or a struct laid out differently than assumed, which is the
// class of bug that would otherwise surface only on the panel in the cupboard.
func TestTheLineSettingsApplyToARealTTY(t *testing.T) {
	_, slave := openPTY(t)

	port, err := driver.DialSerial(t.Context(), slave, driver.WithBaud(9600))
	if err != nil {
		t.Fatalf("DialSerial on a pty: %v", err)
	}
	defer func() { _ = port.Close() }()
}

// TestEveryOSDPRateCanBeApplied, which is what proves the speed encoding is
// right on this platform rather than merely accepted by the validator.
func TestEveryOSDPRateCanBeApplied(t *testing.T) {
	for _, baud := range driver.SerialBaudRates {
		_, slave := openPTY(t)

		port, err := driver.DialSerial(t.Context(), slave, driver.WithBaud(baud))
		if err != nil {
			t.Errorf("baud %d: %v", baud, err)
			continue
		}
		_ = port.Close()
	}
}

// TestOctetsCrossTheLineIntact.
//
// Raw mode is the point: a tty layer that translated a carriage return, echoed
// input or acted on a 0x03 would corrupt frames in ways indistinguishable from
// line noise. The payload here is chosen to contain exactly those octets.
func TestOctetsCrossTheLineIntact(t *testing.T) {
	master, slave := openPTY(t)

	port, err := driver.DialSerial(t.Context(), slave)
	if err != nil {
		t.Fatalf("DialSerial: %v", err)
	}
	defer func() { _ = port.Close() }()

	// CR, LF, XON, XOFF, ETX, NUL and a high-bit octet: every one of them
	// something a cooked tty would alter.
	want := []byte{0x53, 0x0D, 0x0A, 0x11, 0x13, 0x03, 0x00, 0xFF, 0x80}
	if _, err := master.Write(want); err != nil {
		t.Fatalf("writing to the pty master: %v", err)
	}

	if err := port.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}

	got := make([]byte, len(want))
	read := 0
	for read < len(want) {
		n, err := port.Read(got[read:])
		if err != nil {
			t.Fatalf("Read after %d octets: %v", read, err)
		}
		read += n
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("octet %d = %#x, want %#x — the tty layer altered the data\n"+
				"  sent % X\n  got  % X", i, got[i], want[i], want, got)
		}
	}
}

// TestAReadDeadlineActuallyInterrupts.
//
// The property the whole design rests on. If deadlines did not work on a serial
// descriptor, a panel waiting on a silent line could not be cancelled and the
// only alternative would be busy-polling -- a core burned on an idle bus.
func TestAReadDeadlineActuallyInterrupts(t *testing.T) {
	_, slave := openPTY(t)

	port, err := driver.DialSerial(t.Context(), slave)
	if err != nil {
		t.Fatalf("DialSerial: %v", err)
	}
	defer func() { _ = port.Close() }()

	if dErr := port.SetReadDeadline(time.Now().Add(50 * time.Millisecond)); dErr != nil {
		t.Fatalf("SetReadDeadline: %v", dErr)
	}

	start := time.Now()
	_, err = port.Read(make([]byte, 16))
	elapsed := time.Since(start)

	if !errors.Is(err, transport.ErrTimeout) {
		t.Fatalf("Read = %v, want transport.ErrTimeout", err)
	}
	if elapsed > time.Second {
		t.Errorf("the read took %v to time out; the deadline is not being honoured", elapsed)
	}
}

// TestCloseUnblocksAReaderParkedOnASilentLine, which is what lets a panel shut
// down without waiting out its reply timeout.
func TestCloseUnblocksAReaderParkedOnASilentLine(t *testing.T) {
	_, slave := openPTY(t)

	port, err := driver.DialSerial(t.Context(), slave)
	if err != nil {
		t.Fatalf("DialSerial: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, rErr := port.Read(make([]byte, 16))
		done <- rErr
	}()

	time.Sleep(20 * time.Millisecond) // let the reader park
	_ = port.Close()

	select {
	case err := <-done:
		if !errors.Is(err, transport.ErrClosed) {
			t.Errorf("Read after Close = %v, want transport.ErrClosed", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not unblock a parked reader")
	}
}
