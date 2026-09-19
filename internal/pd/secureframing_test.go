// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd_test

import (
	"testing"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/internal/secure"
)

// How the panel end puts a secure frame together and takes one apart. The
// controller itself, and the handshake it drives, are securesupport_test.go.
//
// None of this calls into anything the device shares. The two ends of a secure
// channel must agree on exactly which octets the message authentication code
// covers, and an agreement between a function and itself proves nothing.

// send builds an established-session command, choosing the block the way the
// bus does: enciphered when there is a payload, authenticated only when there
// is not.
func (c *controller) send(t *testing.T, m cmd.Message) frame.Frame {
	t.Helper()

	block := secure.SCS15
	if len(m.Data) > 0 {
		block = secure.SCS17
	}
	return c.command(t, m, block)
}

// command assembles one frame under block, in the specification's order: the
// payload is enciphered, the frame is built with room for the code reserved so
// that the length field is already final, the code covers every octet before
// itself, and the error check is written last. SIA OSDP v2.2.2 §7.
func (c *controller) command(t *testing.T, m cmd.Message, block secure.BlockType) frame.Frame {
	t.Helper()
	return c.commandAt(t, m, block, c.next())
}

// commandAt is command with the sequence number chosen, for the exchanges a
// panel drives outside the 1, 2, 3 rotation -- sequence zero above all.
func (c *controller) commandAt(
	t *testing.T, m cmd.Message, block secure.BlockType, seq uint8,
) frame.Frame {
	t.Helper()

	payload := m.Data
	if block.Encrypted() {
		ciphertext, err := c.session.Seal(payload, true)
		if err != nil {
			t.Fatalf("panel could not encipher %s: %v", m.Name(), err)
		}
		payload = ciphertext
	}

	data := payload
	if block.Established() {
		data = make([]byte, len(payload)+secure.MACTagSize)
		copy(data, payload)
	}

	f := frame.Frame{
		Address:  testAddress,
		Control:  frame.NewControl(seq, frame.SchemeCRC16, true),
		Security: &frame.SecurityBlock{Type: byte(block)},
		Code:     byte(m.Code),
		Data:     data,
	}

	if block.Established() {
		body := f.AppendBody(nil)
		tag, err := c.session.Authenticate(body[:len(body)-secure.MACTagSize], true)
		if err != nil {
			t.Fatalf("panel could not authenticate %s: %v", m.Name(), err)
		}
		copy(f.Data[len(f.Data)-secure.MACTagSize:], tag[:])
	}

	f.Seal()
	return throughWire(t, f)
}

// open verifies a reply and returns its payload in the clear, failing the test
// if the device's own code does not check out against the panel's chain.
func (c *controller) open(t *testing.T, f frame.Frame) []byte {
	t.Helper()

	if f.Security == nil {
		t.Fatalf("device answered in the clear on an established session: %s",
			cmd.Code(f.Code).Name(true))
	}

	body := f.AppendBody(nil)
	split := len(f.Data) - secure.MACTagSize
	if split < 0 {
		t.Fatal("device answered with no room for a message authentication code")
	}

	if err := c.session.Verify(body[:len(body)-secure.MACTagSize], f.Data[split:], false); err != nil {
		t.Fatalf("the device's reply did not authenticate: %v", err)
	}

	payload := f.Data[:split]
	if !secure.BlockType(f.Security.Type).Encrypted() {
		return payload
	}

	plain, err := c.session.Open(payload, false)
	if err != nil {
		t.Fatalf("the device's reply would not decipher: %v", err)
	}
	return plain
}
