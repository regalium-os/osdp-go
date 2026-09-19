// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"errors"
	"fmt"
)

// Serial configuration errors, matched with errors.Is.
var (
	// ErrUnsupportedBaud reports a line speed OSDP does not define.
	//
	// The rate is not merely unusual: both ends have to agree, and a device
	// only speaks the rates in the specification. Refusing here turns a typo
	// into an error rather than into a line that carries nothing.
	ErrUnsupportedBaud = errors.New("osdp/driver: not an OSDP line speed")

	// ErrSerialUnsupported reports a platform with no serial implementation.
	//
	// It is a runtime error rather than a build failure so that a panel
	// binary still builds for every target this library supports -- a
	// cross-compiled Windows build that cannot open a serial port is more
	// useful than one that will not link.
	ErrSerialUnsupported = errors.New("osdp/driver: serial ports are not supported on this platform")
)

// SerialBaudRates are the line speeds SIA OSDP v2.2.2 defines.
//
// A device is required to support 9600 and negotiates upward with osdp_COMSET.
// Starting anywhere else is a guess about hardware nobody has met yet.
var SerialBaudRates = []int{9600, 19200, 38400, 57600, 115200, 230400}

// serialConfig is what DialSerial was asked for.
type serialConfig struct {
	baud int
}

// SerialOption configures a serial port at open time.
//
// Options exist so that adding a capability never breaks a caller: DialSerial
// keeps its two required arguments and everything else arrives this way.
type SerialOption func(*serialConfig)

// WithBaud sets the line speed. The default is 9600, the only rate every OSDP
// device is required to support.
func WithBaud(baud int) SerialOption {
	return func(c *serialConfig) { c.baud = baud }
}

// defaultSerialConfig is 9600 8N1, which is what a device answers to before
// anybody has told it otherwise.
func defaultSerialConfig() serialConfig {
	return serialConfig{baud: 9600}
}

// validate reports whether this configuration can be applied.
func (c serialConfig) validate() error {
	for _, rate := range SerialBaudRates {
		if rate == c.baud {
			return nil
		}
	}
	return fmt.Errorf("%w: %d (OSDP defines %v)", ErrUnsupportedBaud, c.baud, SerialBaudRates)
}
