// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// ptySlaveName returns the path of the slave belonging to a pty master, and
// unlocks it so it can be opened.
//
// Linux names the slave by its index: TIOCGPTN reports the number, and the path
// is /dev/pts/<n>. TIOCSPTLCK with zero clears the lock the kernel puts on a
// freshly allocated pair. Both ioctl numbers come from syscall rather than from
// a header copied by hand.
func ptySlaveName(master *os.File) (string, error) {
	var unlock int32
	if err := ptyIoctl(master, syscall.TIOCSPTLCK, unsafe.Pointer(&unlock)); err != nil {
		return "", fmt.Errorf("unlocking the pty: %w", err)
	}

	var index uint32
	if err := ptyIoctl(master, syscall.TIOCGPTN, unsafe.Pointer(&index)); err != nil {
		return "", fmt.Errorf("naming the pty: %w", err)
	}
	return fmt.Sprintf("/dev/pts/%d", index), nil
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
