package cmd_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/regalium-os/osdp-go/internal/cmd"
)

// report is a capability payload built from function/compliance/item triples.
func report(t *testing.T, triples ...byte) cmd.CapabilityReport {
	t.Helper()
	r, err := cmd.ParseCapabilities(triples)
	if err != nil {
		t.Fatalf("ParseCapabilities: %v", err)
	}
	return r
}

// TestPartialEntryIsRejected: a reply cut mid-entry is ordinary corruption on a
// shared line, and guessing at the missing octets would invent a capability.
func TestPartialEntryIsRejected(t *testing.T) {
	for _, n := range []int{1, 2, 4, 5, 7} {
		if _, err := cmd.ParseCapabilities(make([]byte, n)); !errors.Is(err, cmd.ErrShortPayload) {
			t.Errorf("%d octets: err = %v, want ErrShortPayload", n, err)
		}
	}
}

// TestEmptyReportIsNotAnError: a device may answer osdp_CAP with nothing.
func TestEmptyReportIsNotAnError(t *testing.T) {
	r, err := cmd.ParseCapabilities(nil)
	if err != nil {
		t.Fatalf("ParseCapabilities(nil): %v", err)
	}
	if len(r) != 0 {
		t.Errorf("report has %d entries, want none", len(r))
	}
	if n := r.ReceiveBufferSize(); n != 0 {
		t.Errorf("ReceiveBufferSize = %d, want 0 when unreported", n)
	}
}

// TestDuplicateFunctionTakesTheFirst pins the documented resolution. Devices do
// repeat entries, and a lookup that silently took the last one would make the
// answer depend on where in the reply the panel happened to look.
func TestDuplicateFunctionTakesTheFirst(t *testing.T) {
	r := report(t, 0x0D, 0x01, 0x02, 0x0D, 0x01, 0x08)

	if got := r.ItemsOf(cmd.FuncReaders); got != 2 {
		t.Errorf("readers = %d, want 2 -- the first entry", got)
	}
}

// TestSupportsDistinguishesAbsentFromDeclined: a device reporting a function at
// compliance zero is saying no, which is not the same as omitting it -- but to
// a caller asking "can it?", both answers are no.
func TestSupportsDistinguishesAbsentFromDeclined(t *testing.T) {
	declined := report(t, byte(cmd.FuncSmartCard), 0x00, 0x00)

	if _, ok := declined.Get(cmd.FuncSmartCard); !ok {
		t.Error("Get lost an entry the device did report")
	}
	if declined.Supports(cmd.FuncSmartCard) {
		t.Error("Supports believes a function declared at compliance zero")
	}
	if report(t).Supports(cmd.FuncSmartCard) {
		t.Error("Supports invented a function the device never mentioned")
	}
}

// TestSecureChannelReadsBothOctets: capability and commissioning state are
// different questions, carried in different octets of the same entry.
func TestSecureChannelReadsBothOctets(t *testing.T) {
	for _, tc := range []struct {
		name                string
		compliance, items   byte
		capable, defaultKey bool
	}{
		{"absent", 0x00, 0x00, false, false},
		{"aes128-site-key", 0x01, 0x00, true, false},
		{"aes128-default-key", 0x01, 0x01, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := report(t, byte(cmd.FuncCommSecurity), tc.compliance, tc.items)
			capable, defaultKey := r.SecureChannel()
			if capable != tc.capable || defaultKey != tc.defaultKey {
				t.Errorf("SecureChannel = (%v, %v), want (%v, %v)",
					capable, defaultKey, tc.capable, tc.defaultKey)
			}
		})
	}

	// A device that never mentions the function must not be handshaken with:
	// silence is not consent to attempt one.
	if capable, _ := report(t).SecureChannel(); capable {
		t.Error("a silent device was reported as Secure Channel capable")
	}
}

// TestAppendExtendsRatherThanReplaces: Append follows the stdlib convention, so
// a caller assembling a reply into a reused buffer keeps what was already there.
func TestAppendExtendsRatherThanReplaces(t *testing.T) {
	prefix := []byte{0xAA, 0xBB}
	r := report(t, 0x0D, 0x01, 0x01)

	got := r.Append(prefix)
	if want := []byte{0xAA, 0xBB, 0x0D, 0x01, 0x01}; !bytes.Equal(got, want) {
		t.Errorf("Append = % X, want % X", got, want)
	}
}

// TestUnknownFunctionIsNamedNotHidden: an unrecognised code must still print as
// something a human can act on at 3am.
func TestUnknownFunctionIsNamedNotHidden(t *testing.T) {
	if got := cmd.Function(0x7F).Name(); got != "osdp_CAP_UNKNOWN" {
		t.Errorf("Name = %q, want osdp_CAP_UNKNOWN", got)
	}
	if got := cmd.FuncReaders.String(); got != "osdp_CAP_READERS" {
		t.Errorf("String = %q, want osdp_CAP_READERS", got)
	}
}
