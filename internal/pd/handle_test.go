// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd_test

import (
	"testing"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/pd"
)

// The dispatch table: which reply each command deserves, and what its payload
// has to say. The sequencing that wraps it is cache_test.go.

// TestEveryCommandIsAnswered walks the commands a panel sends during enrolment
// and an ordinary poll cycle.
func TestEveryCommandIsAnswered(t *testing.T) {
	tests := []struct {
		name    string
		command cmd.Message
		want    cmd.Code
		inspect func(t *testing.T, data []byte)
	}{{
		name:    "a poll with nothing to report",
		command: cmd.Message{Code: cmd.Poll},
		want:    cmd.ACK,
	}, {
		name:    "osdp_ID returns what the device is",
		command: cmd.Message{Code: cmd.ID},
		want:    cmd.PDID,
		inspect: func(t *testing.T, data []byte) {
			got, err := cmd.ParseDeviceID(data)
			if err != nil {
				t.Fatalf("the device's own osdp_PDID would not parse: %v", err)
			}
			if got != testIdentity {
				t.Errorf("identity round-tripped as %+v, want %+v", got, testIdentity)
			}
		},
	}, {
		name:    "osdp_CAP returns what the device can do",
		command: cmd.Message{Code: cmd.Cap},
		want:    cmd.PDCap,
		inspect: func(t *testing.T, data []byte) {
			report, err := cmd.ParseCapabilities(data)
			if err != nil {
				t.Fatalf("the device's own osdp_PDCAP would not parse: %v", err)
			}
			if got := report.ItemsOf(cmd.FuncContactStatus); got != 4 {
				t.Errorf("claimed %d inputs, want 4: the capability report and "+
					"the status replies must describe the same device", got)
			}
			if capable, _ := report.SecureChannel(); capable {
				t.Error("claimed AES-128, which this package does not implement: " +
					"a panel would challenge it and have to be NAKed")
			}
		},
	}, {
		name:    "osdp_LSTAT returns tamper and power",
		command: cmd.LocalStatusCommand(),
		want:    cmd.LStatR,
		inspect: func(t *testing.T, data []byte) {
			changes, err := cmd.ParseLocalStatus(data)
			if err != nil {
				t.Fatalf("osdp_LSTATR would not parse: %v", err)
			}
			for _, c := range changes {
				if c.Active {
					t.Errorf("%s reported active on an untouched device", c.Kind)
				}
			}
		},
	}, {
		name:    "osdp_ISTAT returns one octet per input",
		command: cmd.InputStatusCommand(),
		want:    cmd.IStatR,
		inspect: func(t *testing.T, data []byte) {
			if len(data) != 4 {
				t.Errorf("osdp_ISTATR carried %d octets, want 4", len(data))
			}
		},
	}, {
		name:    "osdp_OSTAT returns one octet per output",
		command: cmd.OutputStatusCommand(),
		want:    cmd.OStatR,
		inspect: func(t *testing.T, data []byte) {
			if len(data) != 2 {
				t.Errorf("osdp_OSTATR carried %d octets, want 2", len(data))
			}
		},
	}, {
		name:    "osdp_RSTAT returns one octet per reader head",
		command: cmd.ReaderStatusCommand(),
		want:    cmd.RStatR,
		inspect: func(t *testing.T, data []byte) {
			if len(data) != 1 || data[0] != 0 {
				t.Errorf("osdp_RSTATR was %v, want one octet reporting a well head", data)
			}
		},
	}, {
		name:    "osdp_LED is acknowledged even with no lamp to light",
		command: cmd.Message{Code: cmd.LED},
		want:    cmd.ACK,
	}, {
		name:    "an unimplemented command is refused, not ignored",
		command: cmd.Message{Code: cmd.BioRead},
		want:    cmd.NAK,
		inspect: expectNAK(cmd.NAKUnknownCommand),
	}, {
		name:    "osdp_CHLNG is refused as unsupported encryption",
		command: cmd.Message{Code: cmd.Chlng, Data: make([]byte, 8)},
		want:    cmd.NAK,
		inspect: expectNAK(cmd.NAKEncryptionUnsup),
	}, {
		name:    "osdp_KEYSET is refused for want of a secure channel",
		command: cmd.KeySetCommand(make([]byte, 16)),
		want:    cmd.NAK,
		inspect: expectNAK(cmd.NAKSecureRequired),
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := newDevice(t, pd.WithIdentity(testIdentity), pd.WithContacts(4, 2, 1))

			reply := answer(t, d, command(t, 1, tc.command))
			replyCode(t, reply, tc.want)
			if tc.inspect != nil {
				tc.inspect(t, reply.Data)
			}
		})
	}
}

// expectNAK builds an inspector asserting the reason a refusal carried. The
// reason is the whole content of a NAK: a panel distinguishes "I cannot do that
// in the clear" from "I have never heard of that" and acts differently on each.
func expectNAK(want cmd.NAKReason) func(t *testing.T, data []byte) {
	return func(t *testing.T, data []byte) {
		t.Helper()

		got, err := cmd.ParseNAK(data)
		if err != nil {
			t.Fatalf("osdp_NAK would not parse: %v", err)
		}
		if got != want {
			t.Errorf("refused with %q, want %q", got, want)
		}
	}
}
