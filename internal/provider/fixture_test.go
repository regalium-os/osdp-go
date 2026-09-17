package provider_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/provider"
)

// corpusPath is the Phase 3 fixture corpus.
const corpusPath = "testdata/vendor.hex"

// fixtureProviders is the table the corpus's :vendor directive names.
//
// A record names its provider rather than resolving one by OUI, because an
// osdp_PDCAP reply does not carry a vendor code: by the time a panel asks what
// a device can do, it has already read osdp_PDID and chosen. The fixture states
// the same choice.
func fixtureProviders() map[string]provider.Provider {
	return map[string]provider.Provider{
		"generic": provider.Generic{},
		"hid":     provider.HID(),

		// quirked is the fixture backing QuirkNoSecureChannel and
		// QuirkSmallMessages. CLAUDE.md requires a quirk to point at a record
		// that proves it; cap-quirked-vendor is that record, and it shows a
		// device claiming AES-128 and a 1024-octet buffer being disbelieved on
		// both counts. The OUI is not a real registration -- nothing resolves
		// this provider by it.
		"quirked": provider.Vendor{
			VendorName:     "quirked",
			VendorOUI:      0x0F0F0F,
			Quirks:         provider.QuirkNoSecureChannel | provider.QuirkSmallMessages,
			MaxMessageSize: 128,
		},
	}
}

// providerFor resolves the record's :vendor directive.
func providerFor(t *testing.T, fx fixture) provider.Provider {
	t.Helper()
	name := fx.value(t, "vendor")
	p, ok := fixtureProviders()[name]
	if !ok {
		t.Fatalf("fixture %q names provider %q, which the table does not hold", fx.name, name)
	}
	return p
}

// TestFixtureCapabilities is half of the Phase 3 exit criterion: every
// osdp_PDCAP record in the corpus parses, re-encodes to the identical payload,
// and produces the capability set the record states -- after the device's
// provider has had its say.
func TestFixtureCapabilities(t *testing.T) {
	var checked int

	for _, fx := range loadCorpus(t, corpusPath) {
		_, m := fx.decode(t)
		if m.Code != cmd.PDCap {
			continue
		}
		checked++

		t.Run(fx.name, func(t *testing.T) {
			report, err := cmd.ParseCapabilities(m.Data)
			if want, bad := fx.want["error"]; bad {
				if want != "short-payload" || !errors.Is(err, cmd.ErrShortPayload) {
					t.Fatalf("ParseCapabilities = %v, want %s", err, want)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseCapabilities: %v\n  %s", err, fx.desc)
			}

			// Lossless: a report a panel read and wrote back must be the reply
			// the device sent, unknown function codes included.
			if got := report.Append(nil); !bytes.Equal(got, m.Data) {
				t.Errorf("re-encoded report is not byte-exact\n  want % X\n  got  % X",
					m.Data, got)
			}
			if want := fx.flag(t, "defaultkey"); provider.UsesDefaultKey(report) != want {
				t.Errorf("UsesDefaultKey = %v, want %v\n  %s",
					!want, want, fx.desc)
			}

			assertCapabilities(t, fx, providerFor(t, fx).Reconcile(
				cmd.DeviceID{}, provider.FromReport(report),
			))
		})
	}

	if checked == 0 {
		t.Fatalf("%s yielded no osdp_PDCAP records; this test is checking nothing", corpusPath)
	}
	t.Logf("%d capability records", checked)
}

// assertCapabilities compares the reconciled capability set against the record.
func assertCapabilities(t *testing.T, fx fixture, got provider.Capabilities) {
	t.Helper()

	for _, c := range []struct {
		key string
		got int
	}{
		{"readers", got.Readers},
		{"leds", got.LEDs},
		{"buzzers", got.Buzzers},
		{"displays", got.Displays},
		{"inputs", got.Inputs},
		{"outputs", got.Outputs},
		{"maxmessage", got.MaxMessageSize},
	} {
		if want := fx.number(t, c.key); c.got != want {
			t.Errorf("%s = %d, want %d\n  %s", c.key, c.got, want, fx.desc)
		}
	}

	if want := fx.flag(t, "secure"); got.SecureChannel != want {
		t.Errorf("secure channel = %v, want %v\n  %s", got.SecureChannel, want, fx.desc)
	}
}
