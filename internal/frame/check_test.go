// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package frame_test

import (
	"testing"

	"github.com/regalium-os/osdp-go/internal/frame"
)

// TestCRCMatchesPublishedCheckValue anchors the CRC to something outside this
// repository.
//
// Every other CRC test here compares our output against our own fixtures, which
// would agree perfectly even if the seed were wrong. The catalogue check value
// for CRC-16/AUG-CCITT -- the CRC of the ASCII digits "123456789" -- is the one
// assertion that cannot be satisfied by a self-consistent mistake.
func TestCRCMatchesPublishedCheckValue(t *testing.T) {
	const published = 0xE5CC
	if got := frame.CRC16([]byte("123456789")); got != published {
		t.Errorf("CRC16(\"123456789\") = 0x%04X, want 0x%04X\n"+
			"  this is CRC-16/AUG-CCITT: poly 0x1021, init 0x1D0F, not reflected.\n"+
			"  0x1E79 circulates as an OSDP seed and is wrong; it yields 0xF84F.",
			got, published)
	}
}

func TestCRCRejectsTheWrongSeed(t *testing.T) {
	// Guards against someone "fixing" the seed to the value found in several
	// online OSDP summaries.
	if frame.CRC16([]byte("123456789")) == 0xF84F {
		t.Error("CRC16 is seeded 0x1E79; the specification requires 0x1D0F")
	}
}

func TestChecksumIsTwosComplement(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want uint8
	}{
		{"empty sums to zero", nil, 0x00},
		{"single octet", []byte{0x01}, 0xFF},
		{"wraps past a byte", []byte{0xFF, 0x01}, 0x00},
		{"poll frame body", []byte{0x53, 0x01, 0x07, 0x00, 0x02, 0x60}, 0x43},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := frame.Checksum(tt.in); got != tt.want {
				t.Errorf("Checksum(% X) = 0x%02X, want 0x%02X", tt.in, got, tt.want)
			}
		})
	}
}

func TestSchemeSizeAndName(t *testing.T) {
	if frame.SchemeCRC16.Size() != 2 || frame.SchemeCRC16.String() != "crc16" {
		t.Error("SchemeCRC16 must be two octets named crc16")
	}
	if frame.SchemeChecksum.Size() != 1 || frame.SchemeChecksum.String() != "checksum" {
		t.Error("SchemeChecksum must be one octet named checksum")
	}
}
