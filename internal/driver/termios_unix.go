// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

//go:build linux || darwin

package driver

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// rawTermios strips every piece of terminal processing from t.
//
// The flags are the ones cfmakeraw sets, and each is here because a tty layer
// that helps would corrupt a binary protocol:
//
//   - IGNBRK/BRKINT/PARMRK: a break on the line must arrive as octets, not as
//     a signal or a three-byte escape sequence spliced into the frame.
//   - ISTRIP: OSDP octets use all eight bits; stripping the high one would
//     silently halve the alphabet.
//   - INLCR/IGNCR/ICRNL: a CRC byte that happens to be 0x0D must not become
//     0x0A or vanish.
//   - IXON/IXOFF: 0x11 and 0x13 inside a payload are data, not flow control.
//   - OPOST: no output translation, for the same reason.
//   - ECHO/ECHONL: a shared RS-485 pair echoing input would put the panel's
//     own command back on the wire as though a device had sent it.
//   - ICANON: deliver octets as they arrive rather than at a newline. A frame
//     that never contains 0x0A would otherwise never be delivered at all.
//   - ISIG: 0x03 in a payload is data, not an interrupt.
//   - CSIZE/PARENB then CS8: eight data bits, no parity -- the 8N1 OSDP
//     specifies.
//   - CLOCAL: ignore modem control lines. An RS-485 adapter never asserts
//     carrier detect, and without this every read would block waiting for it.
//   - CREAD: enable the receiver, without which the port is write-only.
func rawTermios(t *syscall.Termios) {
	t.Iflag &^= syscall.IGNBRK | syscall.BRKINT | syscall.PARMRK | syscall.ISTRIP |
		syscall.INLCR | syscall.IGNCR | syscall.ICRNL | syscall.IXON | syscall.IXOFF
	t.Oflag &^= syscall.OPOST
	t.Lflag &^= syscall.ECHO | syscall.ECHONL | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
	t.Cflag &^= syscall.CSIZE | syscall.PARENB | syscall.CSTOPB
	t.Cflag |= syscall.CS8 | syscall.CLOCAL | syscall.CREAD

	// VMIN 1, VTIME 0: a read is satisfied by the first octet to arrive, and
	// waits for it.
	//
	// The zero case is a trap worth naming. With VMIN 0 a read returns
	// immediately whether or not anything has arrived, so it succeeds with
	// zero octets -- which Go reports as io.EOF. A panel would then spin
	// through its poll loop at the speed of the CPU rather than the wire,
	// burning a core on an idle bus, and no deadline would ever be reached
	// because no read would ever block. This was not theoretical: it is what
	// this driver did until TestAReadDeadlineActuallyInterrupts caught it.
	//
	// Waiting does not cost cancellability. The descriptor is non-blocking, so
	// an empty read returns EAGAIN rather than sleeping in the kernel, and the
	// runtime poller parks the goroutine until either an octet arrives or the
	// deadline passes.
	t.Cc[syscall.VMIN] = 1
	t.Cc[syscall.VTIME] = 0
}

// ioctlTermios issues one termios ioctl against file.
//
// It runs through RawConn so the descriptor stays registered with the runtime
// poller: taking the fd out with File.Fd() would put it back into blocking mode
// and cost the deadlines this driver is built on.
func ioctlTermios(file *os.File, request uintptr, t *syscall.Termios) error {
	raw, err := file.SyscallConn()
	if err != nil {
		return err
	}

	var ioctlErr syscall.Errno
	if err := raw.Control(func(fd uintptr) {
		// #nosec G103 -- an ioctl takes the address of a struct the kernel
		// fills; there is no way to issue one without unsafe.Pointer. t is a
		// live local whose lifetime spans the call, which is what the rule is
		// actually protecting against.
		_, _, ioctlErr = syscall.Syscall(syscall.SYS_IOCTL, fd, request,
			uintptr(unsafe.Pointer(t)))
	}); err != nil {
		return err
	}
	if ioctlErr != 0 {
		return fmt.Errorf("ioctl %#x: %w", request, ioctlErr)
	}
	return nil
}
