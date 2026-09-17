package secure_test

import (
	"bytes"
	"testing"

	"github.com/regalium-os/osdp-go/internal/secure"
)

// TestFixtureMAC checks the two-key CBC-MAC against independent vectors across
// the three shapes the construction distinguishes: a short block that needs
// padding, a message spanning several blocks, and one that is exactly block
// aligned and therefore gets no padding at all.
func TestFixtureMAC(t *testing.T) {
	kat := loadKAT(t)
	keys := sessionFromKAT(t, kat)
	chain := key16(t, kat["rmac_i"])

	tests := []struct {
		name  string
		input []byte
		want  []byte
	}{
		{"short block, padded", kat["seal_plaintext_short"], kat["mac_short"]},
		{"two blocks", kat["mac_two_block_input"], kat["mac_two_block"]},
		{"exactly one block, unpadded", kat["seal_plaintext_aligned"], kat["mac_exact_block"]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := aes128.MAC(keys, chain, tt.input)
			if err != nil {
				t.Fatalf("MAC: %v", err)
			}
			if !bytes.Equal(got[:], tt.want) {
				t.Errorf("MAC = %x, want %x", got, tt.want)
			}
		})
	}
}

// TestMACChainsForward: the same message authenticated against a different
// chain value must produce a different code, or a captured frame could be
// replayed later in the session.
func TestMACChainsForward(t *testing.T) {
	kat := loadKAT(t)
	keys := sessionFromKAT(t, kat)

	msg := []byte{0x60}
	first, err := aes128.MAC(keys, key16(t, kat["rmac_i"]), msg)
	if err != nil {
		t.Fatalf("MAC: %v", err)
	}
	second, err := aes128.MAC(keys, first, msg)
	if err != nil {
		t.Fatalf("MAC: %v", err)
	}

	if bytes.Equal(first[:], second[:]) {
		t.Error("the MAC does not advance with the chain; a frame could be replayed verbatim")
	}
}

// TestMACOfEmptyMessage: an empty payload still contributes a terminal padded
// block, so it is authenticated rather than silently accepted.
func TestMACOfEmptyMessage(t *testing.T) {
	kat := loadKAT(t)
	keys := sessionFromKAT(t, kat)
	chain := key16(t, kat["rmac_i"])

	empty, err := aes128.MAC(keys, chain, nil)
	if err != nil {
		t.Fatalf("MAC: %v", err)
	}
	var zero [secure.BlockSize]byte
	if bytes.Equal(empty[:], zero[:]) {
		t.Error("the MAC of an empty message is all zeros")
	}

	other, err := aes128.MAC(keys, chain, []byte{0x00})
	if err != nil {
		t.Fatalf("MAC: %v", err)
	}
	if bytes.Equal(empty[:], other[:]) {
		t.Error("an empty message and a single zero octet authenticate identically")
	}
}
