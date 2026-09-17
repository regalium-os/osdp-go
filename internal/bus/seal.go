// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package bus

import (
	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/internal/secure"
)

// blockForCommand chooses the security block an established session's command
// travels under.
//
// A command with a payload is enciphered (SCS_17); one without is authenticated
// only (SCS_15), because there is nothing to encipher and padding an empty
// payload would put a whole block of cipher on the wire for no gain. An
// osdp_POLL, which is most of what a bus carries, is the second case.
func blockForCommand(dataLen int) frame.SecurityBlock {
	if dataLen > 0 {
		return frame.SecurityBlock{Type: byte(secure.SCS17)}
	}
	return frame.SecurityBlock{Type: byte(secure.SCS15)}
}

// authenticate appends the message authentication code to a frame's data.
//
// The code covers the finished frame, which is why the space for it is
// reserved before the body is taken: the length field inside the header must
// already report the code and the error check that follow it, or the receiver
// authenticates a different octet string than the sender did and every frame
// fails. SIA OSDP v2.2.2 §7.
//
// f.Data is replaced with a fresh slice, so the caller's payload is neither
// extended nor aliased.
func authenticate(s *secure.Session, f *frame.Frame, isCommand bool) error {
	data := make([]byte, len(f.Data)+secure.MACTagSize)
	copy(data, f.Data)
	f.Data = data

	body := f.AppendBody(nil)
	tag, err := s.Authenticate(body[:len(body)-secure.MACTagSize], isCommand)
	if err != nil {
		return err
	}

	copy(f.Data[len(f.Data)-secure.MACTagSize:], tag[:])
	return nil
}

// openReply verifies a reply from an established session and returns its
// payload, deciphered when the security block says it was enciphered.
//
// A reply that arrives without a security block is not merely unexpected: an
// established device that sends plaintext is either not the device we
// authenticated or is being spoken for by something else on the line. It is
// treated exactly as a MAC failure, and deliberately not distinguished from one
// in what is returned to the peer.
//
// The returned payload aliases f.Data when the block is authenticated only, and
// is freshly allocated when it was enciphered.
func openReply(s *secure.Session, f frame.Frame) ([]byte, error) {
	if f.Security == nil || len(f.Data) < secure.MACTagSize {
		return nil, secure.ErrMACMismatch
	}

	body := f.AppendBody(nil)
	split := len(f.Data) - secure.MACTagSize

	if err := s.Verify(body[:len(body)-secure.MACTagSize], f.Data[split:], false); err != nil {
		return nil, err
	}

	payload := f.Data[:split]
	if !secure.BlockType(f.Security.Type).Encrypted() {
		return payload, nil
	}
	return s.Open(payload, false)
}
