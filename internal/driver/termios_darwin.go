// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"fmt"
	"os"
	"syscall"
)

// The ioctl numbers come from the standard library's syscall package for this
// platform -- syscall.TIOCGETA and syscall.TIOCSETA -- rather than from
// hand-copied header values. They differ from Linux's TCGETS/TCSETS, which is
// the reason this file exists at all.

// getTermios reads the current line settings.
func getTermios(file *os.File) (*syscall.Termios, error) {
	var t syscall.Termios
	if err := ioctlTermios(file, syscall.TIOCGETA, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// setTermios applies line settings immediately.
//
// TIOCSETA rather than TIOCSETAW or TIOCSETAF: the port has just been opened,
// so there is nothing to drain or flush, and the other two block.
func setTermios(file *os.File, t *syscall.Termios) error {
	return ioctlTermios(file, syscall.TIOCSETA, t)
}

// setSpeed sets both rates, which OSDP always keeps equal.
//
// Darwin stores the speed as the rate itself rather than as a flag -- B9600 is
// 9600 -- so there is no table to look up and no field in Cflag to clear. The
// validation still happens, because an unlisted rate is a caller mistake
// wherever it is caught.
func setSpeed(t *syscall.Termios, baud int) error {
	for _, rate := range SerialBaudRates {
		if rate == baud {
			t.Ispeed = uint64(baud)
			t.Ospeed = uint64(baud)
			return nil
		}
	}
	return fmt.Errorf("%w: %d", ErrUnsupportedBaud, baud)
}
