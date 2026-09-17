package secure

import "crypto/cipher"

// eom is the end-of-message marker that terminates a padded payload.
const eom byte = 0x80

// Seal pads plaintext and enciphers it under S-ENC in CBC mode.
//
// Padding is the marker octet 0x80 followed by zeros to the next block
// boundary. The marker is always appended, so a payload that is already block
// aligned grows by a whole block -- unpadding would otherwise be ambiguous.
//
// plaintext is read and not retained. The returned slice is freshly allocated.
func (AES128) Seal(k SessionKeys, chain [BlockSize]byte, plaintext []byte) ([]byte, error) {
	padded := make([]byte, paddedLen(len(plaintext)))
	copy(padded, plaintext)
	padded[len(plaintext)] = eom

	block, iv, err := cbc(k.Enc, chainIV(chain))
	if err != nil {
		return nil, err
	}
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(padded, padded)
	return padded, nil
}

// Open deciphers ciphertext and strips its padding.
//
// The returned slice aliases a fresh buffer, never the input. ciphertext is
// read and not modified.
//
// A malformed payload returns ErrBadPadding, which a caller must treat exactly
// as it treats a MAC failure: discard the frame and tear the session down. The
// two are deliberately not distinguishable to a peer.
func (AES128) Open(k SessionKeys, chain [BlockSize]byte, ciphertext []byte) ([]byte, error) {
	if len(ciphertext) == 0 || len(ciphertext)%BlockSize != 0 {
		return nil, ErrNotBlockAligned
	}

	block, iv, err := cbc(k.Enc, chainIV(chain))
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(out, ciphertext)

	n, err := unpad(out)
	if err != nil {
		return nil, err
	}
	return out[:n], nil
}

// paddedLen is the enciphered length of a payload of n octets, marker included.
func paddedLen(n int) int { return (n + 1 + BlockSize - 1) &^ (BlockSize - 1) }

// unpad returns the payload length, having verified the marker.
//
// It scans back over the zero fill to the marker. The scan runs over at most
// one block's worth of padding and does not branch on payload content, so it
// leaks nothing beyond the padded length, which is already public.
func unpad(b []byte) (int, error) {
	i := len(b) - 1
	for i > 0 && b[i] == 0x00 {
		i--
	}
	if b[i] != eom {
		return 0, ErrBadPadding
	}
	return i, nil
}
