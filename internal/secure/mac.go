// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package secure

import "crypto/aes"

// MAC computes the message authentication code over data, continuing the chain.
//
// The construction is a two-key CBC-MAC, per SIA OSDP v2.2.2 §7.5: every block
// but the last is chained under S-MAC1, and the final block is enciphered under
// S-MAC2 to produce the code. The chain starts from the opposite direction's
// previous MAC -- a command's MAC continues from the last R-MAC and a reply's
// from the last C-MAC -- which is what binds each message to the exchange
// before it and makes a replayed frame detectable.
//
// The final block is padded with 0x80 and zeros only when data does not end on
// a block boundary. A block-aligned message contributes its last block as it
// stands, and an empty message contributes a block of padding alone.
//
// data is read and not retained.
func (AES128) MAC(k SessionKeys, chain [BlockSize]byte, data []byte) ([BlockSize]byte, error) {
	mac1, err := aes.NewCipher(k.MAC1[:])
	if err != nil {
		return [BlockSize]byte{}, err
	}
	mac2, err := aes.NewCipher(k.MAC2[:])
	if err != nil {
		return [BlockSize]byte{}, err
	}

	full, rem := len(data)/BlockSize, len(data)%BlockSize
	blocks := full
	if rem != 0 {
		blocks++
	}
	if blocks == 0 {
		blocks = 1
	}

	iv, rest := chain, data
	var block [BlockSize]byte

	// B[1] .. B[N-1] under S-MAC1, in CBC fashion.
	for range blocks - 1 {
		copy(block[:], rest[:BlockSize])
		xorInto(&block, iv)
		mac1.Encrypt(block[:], block[:])
		iv, rest = block, rest[BlockSize:]
	}

	// B[N] under S-MAC2 is the MAC.
	block = [BlockSize]byte{}
	if rem == 0 && full > 0 {
		copy(block[:], rest[:BlockSize])
	} else {
		copy(block[:], rest[:rem])
		block[rem] = eom
	}
	xorInto(&block, iv)
	mac2.Encrypt(block[:], block[:])
	return block, nil
}

func xorInto(dst *[BlockSize]byte, src [BlockSize]byte) {
	for i := range dst {
		dst[i] ^= src[i]
	}
}
