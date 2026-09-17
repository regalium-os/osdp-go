// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package provider_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/internal/provider"
)

// TestFixtureManufacturerMessages is the second half of the Phase 3 exit
// criterion, TestFixtureCapabilities being the first: every osdp_MFG record
// decodes to the vendor and body the record states, and re-encoding the result
// reproduces the original frame octet for octet -- including a message the
// provider did not recognise.
//
// The round trip is the assertion that matters. A panel forwarding a vendor
// message it cannot read must put back on the wire exactly what it was handed;
// anything else is a panel quietly editing traffic between a reader and the
// software that understands it.
func TestFixtureManufacturerMessages(t *testing.T) {
	ctx := context.Background()
	var checked int

	for _, fx := range loadCorpus(t, corpusPath) {
		f, m := fx.decode(t)
		if m.Code != cmd.MFG && m.Code != cmd.MFGReply {
			continue
		}
		checked++

		t.Run(fx.name, func(t *testing.T) {
			p := providerFor(t, fx)

			mm, err := cmd.ParseManufacturerMessage(m.Data)
			if want, bad := fx.want["error"]; bad {
				if want != "short-payload" || !errors.Is(err, cmd.ErrShortPayload) {
					t.Fatalf("ParseManufacturerMessage = %v, want %s", err, want)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseManufacturerMessage: %v\n  %s", err, fx.desc)
			}

			ext, err := p.DecodeExtension(ctx, mm)
			if err != nil {
				t.Fatalf("DecodeExtension: %v", err)
			}
			if want := ouiOf(fx.octets(t, "oui")); ext.OUI != want {
				t.Errorf("OUI = %06X, want %06X", ext.OUI, want)
			}
			if want := fx.flag(t, "recognised"); ext.Recognised != want {
				t.Errorf("recognised = %v, want %v", ext.Recognised, want)
			}
			if want := fx.octets(t, "body"); !bytes.Equal(ext.Payload, want) {
				t.Errorf("payload = % X, want % X", ext.Payload, want)
			}

			assertFrameRoundTrip(t, fx, f, p, ext)
		})
	}

	if checked == 0 {
		t.Fatalf("%s yielded no osdp_MFG records; this test is checking nothing", corpusPath)
	}
	t.Logf("%d manufacturer-message records", checked)
}

// assertFrameRoundTrip re-encodes the extension through the provider and back
// onto the wire, and requires the result to equal the octets the record holds.
//
// The frame is rebuilt from the decoded one's own address, sequence and check
// scheme rather than from constants: those belong to the bus, and borrowing
// them from the fixture is what makes the comparison a statement about the
// vendor path alone.
func assertFrameRoundTrip(
	t *testing.T, fx fixture, f frame.Frame, p provider.Provider, ext provider.Extension,
) {
	t.Helper()
	ctx := context.Background()

	back, err := p.EncodeExtension(ctx, ext)
	if err != nil {
		t.Fatalf("EncodeExtension: %v", err)
	}

	out, err := cmd.Encode(ctx,
		cmd.Message{Code: cmd.Code(f.Code), IsReply: f.IsReply, Data: back.Append(nil)},
		f.Address, f.Control.Sequence(), f.Control.Scheme(),
	)
	if err != nil {
		t.Fatalf("cmd.Encode: %v", err)
	}

	wire, err := out.Append(ctx, nil)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if !bytes.Equal(wire, fx.raw) {
		t.Errorf("round trip through the provider is not byte-exact\n  %s\n  want % X\n  got  % X",
			fx.desc, fx.raw, wire)
	}
}

// ouiOf assembles the three octets of an OUI expectation into the integer form
// providers register under.
func ouiOf(b []byte) uint32 {
	if len(b) != 3 {
		return 0
	}
	return uint32(b[0])<<16 | uint32(b[1])<<8 | uint32(b[2])
}
