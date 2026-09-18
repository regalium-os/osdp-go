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
)

// testIdentity is what the devices in these tests say they are. The vendor code
// is not zero because it is the OUI a panel's provider registry keys on, and a
// fixture that reports zeros would exercise the generic path only.
var testIdentity = cmd.DeviceID{
	VendorCode: [3]byte{0x00, 0x06, 0x8E},
	Model:      0x02,
	Version:    0x01,
	Serial:     0xDEADBEEF,
	Firmware:   [3]byte{0x01, 0x02, 0x03},
}

// TestNewRefusesAnAddressNoDeviceHas.
//
// The configuration address is refused specifically: it is not an address a
// device has, it is the one a panel uses to reach a device whose address it
// does not know. Answering it is WithConfigurationAddress, and conflating the
// two would put a device on a line where it collides with every other.
func TestNewRefusesAnAddressNoDeviceHas(t *testing.T) {
	for _, addr := range []frame.Address{frame.BroadcastAddress, 0x80, 0xFF} {
		if _, err := pd.New(addr); !errors.Is(err, frame.ErrInvalidAddress) {
			t.Errorf("pd.New(%#x) returned %v, want ErrInvalidAddress", byte(addr), err)
		}
	}
}

// TestNewRefusesAConfigurationTheWireCannotCarry.
func TestNewRefusesAConfigurationTheWireCannotCarry(t *testing.T) {
	tests := []struct {
		name string
		opt  pd.Option
		want error
	}{
		{"more inputs than one octet can report", pd.WithContacts(256, 0, 1), pd.ErrTooManyContacts},
		{"more outputs than one octet can report", pd.WithContacts(0, 300, 1), pd.ErrTooManyContacts},
		{"nowhere to hold a card read", pd.WithEventQueue(0), pd.ErrInvalidQueue},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := pd.New(testAddress, tc.opt); !errors.Is(err, tc.want) {
				t.Errorf("pd.New returned %v, want %v", err, tc.want)
			}
		})
	}
}

// TestCapabilitiesDescribeTheSameDeviceAsTheStatusReplies.
//
// A panel told there are four inputs and then handed two octets of osdp_ISTATR
// has no way to decide which of the two to believe, and both are this device.
func TestCapabilitiesDescribeTheSameDeviceAsTheStatusReplies(t *testing.T) {
	d := newDevice(t, pd.WithContacts(4, 2, 3))
	report := d.Capabilities()

	claimed := map[cmd.Function]int{
		cmd.FuncContactStatus: 4,
		cmd.FuncOutputControl: 2,
		cmd.FuncReaders:       3,
	}
	for fn, want := range claimed {
		if got := report.ItemsOf(fn); got != want {
			t.Errorf("%s reports %d items, want %d", fn, got, want)
		}
	}

	for code, want := range map[cmd.Code]int{cmd.IStat: 4, cmd.OStat: 2, cmd.RStat: 3} {
		reply := answer(t, d, command(t, 1, cmd.Message{Code: code}))
		if got := len(reply.Data); got != want {
			t.Errorf("%s answered with %d octets, want %d", code.Name(false), got, want)
		}
		answer(t, d, poll(t, 2)) // advance off the cached sequence
	}
}

// TestCapabilitiesAreCopied, so a caller cannot reach into the device's own
// answer and change what it claims after the panel has already believed it.
func TestCapabilitiesAreCopied(t *testing.T) {
	d := newDevice(t, pd.WithContacts(4, 0, 1))

	report := d.Capabilities()
	for i := range report {
		report[i].Items = 0xFF
	}

	if got := d.Capabilities().ItemsOf(cmd.FuncContactStatus); got != 4 {
		t.Errorf("the device now claims %d inputs, want 4", got)
	}
}

// TestSuppliedCapabilitiesAreReportedVerbatim, including a function this
// package does not model -- the report is how a device says something this
// library has no type for.
func TestSuppliedCapabilitiesAreReportedVerbatim(t *testing.T) {
	want := cmd.CapabilityReport{
		{Function: cmd.FuncBiometrics, Compliance: 0x01, Items: 0x01},
		{Function: cmd.FuncSmartCard, Compliance: 0x01, Items: 0x00},
	}
	d := newDevice(t, pd.WithCapabilities(want))

	got, err := cmd.ParseCapabilities(answer(t, d, command(t, 1, cmd.Message{Code: cmd.Cap})).Data)
	if err != nil {
		t.Fatalf("osdp_PDCAP would not parse: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("reported %d entries, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d was %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestTheConfigurationAddressIsAnsweredOnlyWhenEnabled. SIA OSDP v2.2.2 §5.3.
func TestTheConfigurationAddressIsAnsweredOnlyWhenEnabled(t *testing.T) {
	d := newDevice(t, pd.WithConfigurationAddress())

	reply := answer(t, d, commandTo(t, frame.BroadcastAddress, 1, cmd.Message{Code: cmd.Poll}))
	replyCode(t, reply, cmd.ACK)

	if got := reply.Address; got != testAddress {
		t.Errorf("answered from address %#x, want %#x: a device commissioned at "+
			"the configuration address must say which address it really is",
			byte(got), byte(testAddress))
	}
}

// TestAHandlerAnswersWhatTheTableDoesNot, and is consulted last, so it cannot
// change what osdp_POLL means.
func TestAHandlerAnswersWhatTheTableDoesNot(t *testing.T) {
	var sawPoll bool
	d := newDevice(t, pd.WithHandler(func(_ context.Context, m cmd.Message) (cmd.Message, bool) {
		if m.Code == cmd.Poll {
			sawPoll = true
		}
		return cmd.Message{Code: cmd.MFGReply, Data: []byte{0x00, 0x06, 0x8E, 0x01}}, true
	}))

	replyCode(t, answer(t, d, poll(t, 1)), cmd.ACK)
	if sawPoll {
		t.Error("the handler was offered osdp_POLL, which the dispatch table owns")
	}

	vendor := cmd.Message{Code: cmd.MFG, Data: []byte{0x00, 0x06, 0x8E, 0x01}}
	replyCode(t, answer(t, d, command(t, 2, vendor)), cmd.MFGReply)
}

// TestADecliningHandlerFallsBackToNAK, so a panel learns immediately rather
// than spending its retry budget on a command no version of this device answers.
func TestADecliningHandlerFallsBackToNAK(t *testing.T) {
	d := newDevice(t, pd.WithHandler(func(_ context.Context, _ cmd.Message) (cmd.Message, bool) {
		return cmd.Message{}, false
	}))

	reply := answer(t, d, command(t, 1, cmd.Message{Code: cmd.PIVData}))
	replyCode(t, reply, cmd.NAK)
	expectNAK(cmd.NAKUnknownCommand)(t, reply.Data)
}
