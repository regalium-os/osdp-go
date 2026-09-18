// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package bus

import (
	"errors"
	"fmt"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/internal/secure"
)

// ErrMessageTooLarge reports a command bigger than the device said it can
// receive.
//
// It carries the two numbers, because "too large" without them sends a caller
// to the specification to find out by how much.
var ErrMessageTooLarge = errors.New("osdp/bus: command larger than the device accepts")

// oversizedError is ErrMessageTooLarge with the measurements attached.
type oversizedError struct {
	code     cmd.Code
	need     int
	accepted int
}

func (e oversizedError) Error() string {
	return fmt.Sprintf("osdp/bus: %s needs %d octets, device accepts %d",
		e.code.Name(false), e.need, e.accepted)
}

func (e oversizedError) Is(target error) bool { return target == ErrMessageTooLarge }

// checkSize reports whether a command will fit in what the device said it can
// receive.
//
// # Why this is worth checking rather than finding out
//
// A device that cannot receive a frame does not reply to it. The panel sees a
// silence, which is indistinguishable from a reader with a cut wire, so it
// retries -- and the retry is the same oversized frame, so it fails the same
// way. The command sits at the head of that device's queue forever, and the
// only symptom is a reader that appears to be answering polls but never
// anything else. That is a bad afternoon.
//
// Refusing at the call site turns it into an error at the moment the mistake is
// made, which is the only moment anybody can do anything about it.
//
// A device that reported no buffer size is taken at its word: nothing is
// checked, because a limit nobody stated is not a limit this package may
// invent.
func (b *Bus) checkSize(d *Device, m cmd.Message) error {
	accepted := d.Caps.ReceiveBufferSize()
	if accepted == 0 {
		return nil
	}

	need := b.wireLength(d, m)
	if need > accepted {
		return oversizedError{code: m.Code, need: need, accepted: accepted}
	}
	return nil
}

// wireLength is how many octets this command will occupy once framed for this
// device, secure channel included.
//
// It is computed rather than measured because the frame does not exist yet --
// the point is to refuse before building one. The arithmetic mirrors what
// compose does, and the two agreeing is asserted by a test rather than by
// hoping.
func (b *Bus) wireLength(d *Device, m cmd.Message) int {
	payload := len(m.Data)
	security := 0

	// An established session pads the payload to the cipher's block size,
	// appends a message authentication code, and carries a security block.
	// All three are on the wire and all three count against the device's
	// buffer.
	if d.secureEstablished() {
		security = securityBlockOverhead
		if payload > 0 {
			payload = paddedLength(payload)
		}
		payload += secure.MACTagSize
	}

	return frame.HeaderSize + security + 1 + payload + b.scheme.Size()
}

// securityBlockOverhead is the length and type octets a security block adds.
// The blocks this package sends carry no data of their own.
const securityBlockOverhead = 2

// paddedLength is what Seal will grow a payload to: the end-of-message marker
// plus zero fill to the next block boundary.
//
// The marker is always appended, so a payload that is already block aligned
// grows by a whole block. Mirrors secure.paddedLen, which is unexported.
func paddedLength(n int) int {
	return (n + 1 + secure.BlockSize - 1) &^ (secure.BlockSize - 1)
}
