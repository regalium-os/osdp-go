// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd_test

import (
	"testing"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
)

// How a reply is framed, as distinct from what it says. The panel half keeps
// the same separation, in internal/bus/framing_test.go.

// TestTheReplyUsesTheCommandsCheckScheme.
//
// A panel speaking the checksum cannot verify a CRC-16 reply, and will retry
// what it reads as a corrupt frame until it gives up on the device.
func TestTheReplyUsesTheCommandsCheckScheme(t *testing.T) {
	for _, scheme := range []frame.Scheme{frame.SchemeCRC16, frame.SchemeChecksum} {
		t.Run(scheme.String(), func(t *testing.T) {
			d := newDevice(t)

			sent := frame.Frame{
				Address: testAddress,
				Control: frame.NewControl(1, scheme, false),
				Code:    byte(cmd.Poll),
			}
			sent.Seal()

			reply := answer(t, d, sent)
			if got := reply.Control.Scheme(); got != scheme {
				t.Errorf("answered a %s command with %s", scheme, got)
			}
		})
	}
}
