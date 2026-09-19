// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/regalium-os/osdp-go/internal/driver"
)

// What can be checked without a serial port: configuration, refusals, and the
// error a missing device produces. The line settings themselves need hardware
// or a pty, and serial_pty_test.go covers as much of that as a pty can.

// TestOnlyOSDPLineSpeedsAreAccepted.
//
// Both ends have to agree on the rate and a device only speaks the ones the
// specification defines, so a typo here produces a line that carries nothing --
// which on a bus looks exactly like a reader that is not plugged in.
func TestOnlyOSDPLineSpeedsAreAccepted(t *testing.T) {
	for _, baud := range driver.SerialBaudRates {
		_, err := driver.DialSerial(context.Background(), "/nonexistent", driver.WithBaud(baud))
		if errors.Is(err, driver.ErrUnsupportedBaud) {
			t.Errorf("%d is an OSDP rate and was refused", baud)
		}
	}

	for _, baud := range []int{0, 300, 14400, 250000, -9600} {
		_, err := driver.DialSerial(context.Background(), "/nonexistent", driver.WithBaud(baud))
		if !errors.Is(err, driver.ErrUnsupportedBaud) {
			t.Errorf("baud %d = %v, want ErrUnsupportedBaud", baud, err)
		}
	}
}

// TestTheRateIsRefusedBeforeTheDeviceIsOpened.
//
// Ordering worth pinning: a bad rate must be reported as a bad rate, not as
// whatever the open happened to fail with. Otherwise a typo in a config file
// surfaces as "no such file or directory" against a device that exists.
func TestTheRateIsRefusedBeforeTheDeviceIsOpened(t *testing.T) {
	_, err := driver.DialSerial(context.Background(), "/nonexistent", driver.WithBaud(12345))

	if !errors.Is(err, driver.ErrUnsupportedBaud) {
		t.Fatalf("err = %v, want ErrUnsupportedBaud", err)
	}
	if strings.Contains(err.Error(), "/nonexistent") {
		t.Error("the open was attempted before the rate was checked")
	}
}

// TestADefaultOfNineThousandSixHundred, the only rate every OSDP device is
// required to support and therefore the only safe thing to assume about
// hardware nobody has met yet.
func TestADefaultOfNineThousandSixHundred(t *testing.T) {
	_, err := driver.DialSerial(context.Background(), "/nonexistent")

	if errors.Is(err, driver.ErrUnsupportedBaud) {
		t.Error("the default rate was refused as unsupported")
	}
}

// TestAMissingDeviceNamesItself: a panel that cannot find its port has to say
// which port, because a site has several.
func TestAMissingDeviceNamesItself(t *testing.T) {
	const device = "/dev/definitely-not-a-serial-port"

	_, err := driver.DialSerial(context.Background(), device)
	if err == nil {
		t.Fatal("opening a nonexistent device succeeded")
	}
	if !strings.Contains(err.Error(), device) {
		t.Errorf("err = %q, want it to name %s", err, device)
	}
}

// TestACancelledContextStopsTheOpen. ctx bounds opening the port; once open the
// port outlives it and Close is what ends it.
func TestACancelledContextStopsTheOpen(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := driver.DialSerial(ctx, "/nonexistent"); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}
