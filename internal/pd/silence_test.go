// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd_test

import (
	"context"
	"errors"
	"testing"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/internal/pd"
	"github.com/regalium-os/osdp-go/internal/secure"
)

// When a device says nothing at all, and when it refuses out loud. A reader
// answering a frame that was not addressed to it is how a multidrop line turns
// into a collision. The helpers are in support_test.go.

// TestTheDeviceStaysSilent covers every frame a device must not answer.
//
// Silence is the correct answer on a multidrop line, and it is not the same as
// success: a device that replied to another device's command would transmit
// over the reply the panel was waiting for, and the panel would decode neither.
func TestTheDeviceStaysSilent(t *testing.T) {
	tests := []struct {
		name  string
		build func(t *testing.T) frame.Frame
		want  error
	}{{
		name:  "a command for another address",
		build: func(t *testing.T) frame.Frame { return commandTo(t, 0x02, 1, cmd.Message{Code: cmd.Poll}) },
		want:  pd.ErrNotAddressed,
	}, {
		name: "the configuration address, which is off by default",
		build: func(t *testing.T) frame.Frame {
			return commandTo(t, frame.BroadcastAddress, 1, cmd.Message{Code: cmd.Poll})
		},
		want: pd.ErrNotAddressed,
	}, {
		name: "another device's reply, seen on the shared line",
		build: func(t *testing.T) frame.Frame {
			f := command(t, 1, cmd.Message{Code: cmd.Poll})
			f.IsReply = true
			f.Seal()
			return f
		},
		want: pd.ErrNotCommand,
	}, {
		name: "a frame whose error check does not match",
		build: func(t *testing.T) frame.Frame {
			f := command(t, 1, cmd.Message{Code: cmd.Poll})
			f.Check ^= 0xFFFF
			return f
		},
		want: pd.ErrCorrupt,
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := newDevice(t)

			reply, err := d.Handle(context.Background(), tc.build(t))
			if !errors.Is(err, tc.want) {
				t.Errorf("Handle returned %v, want %v", err, tc.want)
			}
			if !errors.Is(err, pd.ErrNoReply) {
				t.Errorf("%v does not satisfy errors.Is(err, ErrNoReply); a runtime "+
					"matching the class would put this on the wire", err)
			}
			if reply.IsReply || reply.Code != 0 || len(reply.Data) != 0 {
				t.Error("a declined command produced a frame, which would collide " +
					"with the device that really was addressed")
			}
		})
	}
}

// TestASecureFrameIsRefusedRatherThanIgnored.
//
// The device reports no Secure Channel capability, so a well-behaved panel
// never offers one. A panel that offers it anyway -- because it was configured
// by hand, or because it is talking to the wrong address -- learns immediately
// instead of spending its retry budget on silence.
func TestASecureFrameIsRefusedRatherThanIgnored(t *testing.T) {
	d := newDevice(t)

	f := command(t, 1, cmd.Message{Code: cmd.Poll})
	f.Control = frame.NewControl(1, frame.SchemeCRC16, true)
	f.Security = &frame.SecurityBlock{Type: byte(secure.SCS15)}
	f.Seal()

	reply := answer(t, d, f)
	replyCode(t, reply, cmd.NAK)
	expectNAK(cmd.NAKEncryptionUnsup)(t, reply.Data)
}
