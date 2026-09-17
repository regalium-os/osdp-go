package secure_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/regalium-os/osdp-go/internal/secure"
)

func TestFixtureSeal(t *testing.T) {
	kat := loadKAT(t)
	keys := sessionFromKAT(t, kat)
	chain := key16(t, kat["rmac_i"])

	for _, tt := range []struct{ name, pt, ct string }{
		{"short payload", "seal_plaintext_short", "seal_ciphertext_short"},
		{"block aligned payload", "seal_plaintext_aligned", "seal_ciphertext_aligned"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := aes128.Seal(keys, chain, kat[tt.pt])
			if err != nil {
				t.Fatalf("Seal: %v", err)
			}
			if !bytes.Equal(got, kat[tt.ct]) {
				t.Errorf("Seal = %x\n want %x", got, kat[tt.ct])
			}
		})
	}
}

// TestFixtureSealGrowsBlockAlignedPayloads guards the padding rule that is easy
// to get wrong: the marker is appended unconditionally, so a payload that
// already fills a block gains a whole block of padding. Omitting it would make
// unpadding ambiguous for any payload ending in 0x80.
func TestFixtureSealGrowsBlockAlignedPayloads(t *testing.T) {
	kat := loadKAT(t)
	keys := sessionFromKAT(t, kat)

	sealed, err := aes128.Seal(keys, key16(t, kat["rmac_i"]), kat["seal_plaintext_aligned"])
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if want := len(kat["seal_plaintext_aligned"]) + secure.BlockSize; len(sealed) != want {
		t.Errorf("sealed length = %d, want %d", len(sealed), want)
	}
}

func TestSealOpenRoundTrip(t *testing.T) {
	kat := loadKAT(t)
	keys := sessionFromKAT(t, kat)
	chain := key16(t, kat["rmac_i"])

	for _, n := range []int{0, 1, 15, 16, 17, 31, 32, 128} {
		payload := make([]byte, n)
		for i := range payload {
			payload[i] = byte(i * 7)
		}

		sealed, err := aes128.Seal(keys, chain, payload)
		if err != nil {
			t.Fatalf("Seal(%d octets): %v", n, err)
		}
		if len(sealed)%secure.BlockSize != 0 {
			t.Errorf("sealed length %d is not block aligned", len(sealed))
		}

		opened, err := aes128.Open(keys, chain, sealed)
		if err != nil {
			t.Fatalf("Open(%d octets): %v", n, err)
		}
		if !bytes.Equal(opened, payload) {
			t.Errorf("round trip of %d octets: got %x, want %x", n, opened, payload)
		}
	}
}

func TestOpenRejectsMalformedInput(t *testing.T) {
	kat := loadKAT(t)
	keys := sessionFromKAT(t, kat)
	chain := key16(t, kat["rmac_i"])

	t.Run("not block aligned", func(t *testing.T) {
		_, err := aes128.Open(keys, chain, make([]byte, 17))
		if !errors.Is(err, secure.ErrNotBlockAligned) {
			t.Errorf("Open = %v, want ErrNotBlockAligned", err)
		}
	})

	t.Run("empty", func(t *testing.T) {
		_, err := aes128.Open(keys, chain, nil)
		if !errors.Is(err, secure.ErrNotBlockAligned) {
			t.Errorf("Open = %v, want ErrNotBlockAligned", err)
		}
	})

	t.Run("padding marker absent", func(t *testing.T) {
		// A block that deciphers to something without the 0x80 marker.
		sealed, err := aes128.Seal(keys, chain, []byte{0x60})
		if err != nil {
			t.Fatalf("Seal: %v", err)
		}
		sealed[0] ^= 0xFF // corrupt the ciphertext

		if _, err := aes128.Open(keys, chain, sealed); !errors.Is(err, secure.ErrBadPadding) {
			t.Errorf("Open of corrupted ciphertext = %v, want ErrBadPadding", err)
		}
	})
}

// TestEncryptionAloneDoesNotAuthenticate documents why the MAC is mandatory
// rather than an optional extra.
//
// CBC is malleable: flipping a bit in the initialisation vector flips exactly
// that bit of the first plaintext block and leaves everything else, padding
// marker included, intact. Open therefore succeeds and returns *altered*
// plaintext without complaint. Nothing in this file detects that; only the MAC
// does.
//
// The test asserts the malleability deliberately. If a future change made Open
// appear to reject tampering, that would be a false sense of integrity and a
// reason to look hard at what it was actually checking.
func TestEncryptionAloneDoesNotAuthenticate(t *testing.T) {
	kat := loadKAT(t)
	keys := sessionFromKAT(t, kat)
	chain := key16(t, kat["rmac_i"])

	plaintext := []byte("unlock the east door")
	sealed, err := aes128.Seal(keys, chain, plaintext)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	// A single bit of the chain, and therefore of the derived IV.
	tampered := chain
	tampered[0] ^= 0x01

	opened, err := aes128.Open(keys, tampered, sealed)
	if err != nil {
		t.Fatalf("Open with a tampered IV failed; CBC should still unpad: %v", err)
	}
	if bytes.Equal(opened, plaintext) {
		t.Fatal("a tampered IV produced identical plaintext, which AES-CBC cannot do")
	}
	if len(opened) != len(plaintext) {
		t.Errorf("tampered plaintext length = %d, want %d: only the first block should change",
			len(opened), len(plaintext))
	}

	// The MAC is what rejects it.
	honest, err := aes128.MAC(keys, chain, plaintext)
	if err != nil {
		t.Fatalf("MAC: %v", err)
	}
	forged, err := aes128.MAC(keys, chain, opened)
	if err != nil {
		t.Fatalf("MAC: %v", err)
	}
	if bytes.Equal(honest[:], forged[:]) {
		t.Error("tampered plaintext authenticates identically; the MAC is not binding the payload")
	}
}
