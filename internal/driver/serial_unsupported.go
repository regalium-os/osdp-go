// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

//go:build !linux && !darwin

package driver

import (
	"context"
	"fmt"

	"github.com/regalium-os/osdp-go/internal/transport"
)

// DialSerial reports that this platform has no serial implementation.
//
// It exists so that a panel builds for every target this library supports. A
// Windows or FreeBSD binary that cannot open a serial port is more useful than
// one that will not link -- the same program can still drive a line through a
// serial-to-Ethernet converter with DialTCP, which is how most installed buses
// reach a panel that is not in the same cupboard.
//
// Adding a platform means a termios implementation beside the linux and darwin
// ones; nothing above that layer needs to change.
func DialSerial(_ context.Context, device string, _ ...SerialOption) (transport.Port, error) {
	return nil, fmt.Errorf("%w: cannot open %s", ErrSerialUnsupported, device)
}
