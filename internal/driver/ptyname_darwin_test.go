// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// ptySlaveName returns the path of the slave belonging to a pty master.
//
// Darwin has no TIOCGPTN. It names the slave directly through TIOCPTYGNAME,
// which fills a 128-byte buffer, and grants access with TIOCPTYGRANT and
// TIOCPTYUNLK -- the equivalents of grantpt and unlockpt. All three ioctl
// numbers come from syscall rather than from a header copied by hand.
func ptySlaveName(master *os.File) (string, error) {
	if err := ptyIoctl(master, syscall.TIOCPTYGRANT, nil); err != nil {
		return "", fmt.Errorf("granting the pty: %w", err)
	}
	if err := ptyIoctl(master, syscall.TIOCPTYUNLK, nil); err != nil {
		return "", fmt.Errorf("unlocking the pty: %w", err)
	}

	// The size is the kernel's, not a guess: TIOCPTYGNAME is defined with a
	// 128-byte argument and will write that many.
	var name [128]byte
	if err := ptyIoctl(master, syscall.TIOCPTYGNAME, unsafe.Pointer(&name[0])); err != nil {
		return "", fmt.Errorf("naming the pty: %w", err)
	}

	for i, b := range name {
		if b == 0 {
			return string(name[:i]), nil
		}
	}
	return string(name[:]), nil
}

// ptyIoctl issues one ioctl without taking the descriptor out of the poller.
func ptyIoctl(f *os.File, request uintptr, arg unsafe.Pointer) error {
	raw, err := f.SyscallConn()
	if err != nil {
		return err
	}

	var errno syscall.Errno
	if err := raw.Control(func(fd uintptr) {
		_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, fd, request, uintptr(arg))
	}); err != nil {
		return err
	}
	if errno != 0 {
		return errno
	}
	return nil
}
