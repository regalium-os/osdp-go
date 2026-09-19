// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"fmt"
	"os"
	"syscall"
)

// The ioctl numbers and the baud constants below all come from the standard
// library's syscall package for this platform, not from hand-copied header
// values: syscall.TCGETS, syscall.TCSETS and syscall.B9600 through
// syscall.B230400 are generated from the kernel headers by the Go build and
// are correct for every architecture this library targets. A number written
// out here would be a number to get wrong on arm.

// getTermios reads the current line settings.
func getTermios(file *os.File) (*syscall.Termios, error) {
	var t syscall.Termios
	if err := ioctlTermios(file, syscall.TCGETS, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// setTermios applies line settings immediately, discarding nothing.
//
// TCSETS rather than TCSETSW or TCSETSF: the port has just been opened, so
// there is no pending output worth draining and no input worth flushing, and
// the draining variants block.
func setTermios(file *os.File, t *syscall.Termios) error {
	return ioctlTermios(file, syscall.TCSETS, t)
}

// linuxBaud maps a line speed to the constant that encodes it.
//
// Linux carries the speed as a flag in the control word rather than as a
// number, so it has to be looked up rather than assigned.
var linuxBaud = map[int]uint32{
	9600:   syscall.B9600,
	19200:  syscall.B19200,
	38400:  syscall.B38400,
	57600:  syscall.B57600,
	115200: syscall.B115200,
	230400: syscall.B230400,
}

// speedMask is the region of Cflag the line speed occupies.
//
// Linux calls it CBAUD, which the standard library does not expose -- so it is
// derived here rather than copied from a header as a magic number: the union of
// every speed the platform can represent is exactly the field those speeds live
// in. Every constant below is one syscall names, so this stays correct on any
// architecture where the values differ.
const speedMask = syscall.B0 | syscall.B50 | syscall.B75 | syscall.B110 |
	syscall.B134 | syscall.B150 | syscall.B200 | syscall.B300 |
	syscall.B600 | syscall.B1200 | syscall.B1800 | syscall.B2400 |
	syscall.B4800 | syscall.B9600 | syscall.B19200 | syscall.B38400 |
	syscall.B57600 | syscall.B115200 | syscall.B230400 | syscall.B460800 |
	syscall.B500000 | syscall.B576000 | syscall.B921600 | syscall.B1000000 |
	syscall.B1152000 | syscall.B1500000 | syscall.B2000000 | syscall.B2500000 |
	syscall.B3000000 | syscall.B3500000 | syscall.B4000000

// setSpeed sets both the input and output rates, which OSDP always keeps equal.
func setSpeed(t *syscall.Termios, baud int) error {
	encoded, ok := linuxBaud[baud]
	if !ok {
		return fmt.Errorf("%w: %d", ErrUnsupportedBaud, baud)
	}

	// Clear the whole speed field first: the rates are flag patterns rather
	// than numbers, so a previous setting's bits would otherwise survive
	// underneath the new one and name a third speed entirely.
	t.Cflag &^= speedMask
	t.Cflag |= encoded
	t.Ispeed = encoded
	t.Ospeed = encoded
	return nil
}
