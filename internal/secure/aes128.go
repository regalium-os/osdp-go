// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package secure

import (
	"crypto/aes"
	"crypto/cipher"
)

// AES128 is the cipher suite SIA OSDP v2.2.2 §7 mandates: AES-128 in ECB for
// key derivation and the cryptograms, in CBC for payload confidentiality, and a
// two-key CBC-MAC construction for authentication.
//
// It is stateless and safe for concurrent use. It retains no key material: keys
// arrive by value on every call and leave with the caller.
type AES128 struct{}

// Name returns StandardSuiteName.
func (AES128) Name() string { return StandardSuiteName }

// Key derivation tags. The leading 0x01 and these second octets are the
// specification's domain separation between the three derived keys; without it
// the same base key and random would yield one key used for three purposes.
const (
	tagENC  byte = 0x82
	tagMAC1 byte = 0x01
	tagMAC2 byte = 0x02
)

// DeriveSessionKeys derives S-ENC, S-MAC1 and S-MAC2.
//
// Each is a single ECB block: 0x01, the tag, the first six octets of the
// panel's random, then zeros. Only six of the eight random octets take part --
// that is what the specification says, not an oversight here, and it is the
// reason session key entropy is bounded at 48 bits regardless of how good the
// random source is. The remaining two octets of RND.A still enter the session
// through the cryptograms.
func (AES128) DeriveSessionKeys(base BaseKey, rndA [8]byte) (SessionKeys, error) {
	block, err := aes.NewCipher(base[:])
	if err != nil {
		return SessionKeys{}, err
	}

	derive := func(tag byte, dst *[BlockSize]byte) {
		var in [BlockSize]byte
		in[0], in[1] = 0x01, tag
		copy(in[2:8], rndA[:6])
		block.Encrypt(dst[:], in[:])
	}

	var keys SessionKeys
	derive(tagENC, &keys.Enc)
	derive(tagMAC1, &keys.MAC1)
	derive(tagMAC2, &keys.MAC2)
	return keys, nil
}

// ClientCryptogram is AES-ECB(S-ENC, RND.A || RND.B): the proof a peripheral
// device returns in osdp_CCRYPT.
func (AES128) ClientCryptogram(k SessionKeys, rndA, rndB [8]byte) ([BlockSize]byte, error) {
	return ecb(k.Enc, join(rndA, rndB))
}

// ServerCryptogram is AES-ECB(S-ENC, RND.B || RND.A): the panel's answer in
// osdp_SCRYPT. The randoms are in the opposite order to the client cryptogram,
// which is what stops either side replaying the other's proof.
func (AES128) ServerCryptogram(k SessionKeys, rndA, rndB [8]byte) ([BlockSize]byte, error) {
	return ecb(k.Enc, join(rndB, rndA))
}

// InitialRMAC seeds the reply MAC chain: AES-ECB(S-MAC2, AES-ECB(S-MAC1, sc)).
func (AES128) InitialRMAC(k SessionKeys, sc [BlockSize]byte) ([BlockSize]byte, error) {
	first, err := ecb(k.MAC1, sc)
	if err != nil {
		return [BlockSize]byte{}, err
	}
	return ecb(k.MAC2, first)
}

// ecb enciphers exactly one block.
func ecb(key, in [BlockSize]byte) ([BlockSize]byte, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return [BlockSize]byte{}, err
	}
	var out [BlockSize]byte
	block.Encrypt(out[:], in[:])
	return out, nil
}

func join(a, b [8]byte) [BlockSize]byte {
	var out [BlockSize]byte
	copy(out[0:8], a[:])
	copy(out[8:16], b[:])
	return out
}

// chainIV turns a MAC into the initialisation vector for the next payload.
//
// The specification uses the bitwise complement of the previous MAC. The
// complement matters: reusing the MAC unchanged would make the IV a value the
// peer has already seen in the clear on the wire.
func chainIV(chain [BlockSize]byte) [BlockSize]byte {
	var iv [BlockSize]byte
	for i, b := range chain {
		iv[i] = ^b
	}
	return iv
}

func cbc(key [BlockSize]byte, iv [BlockSize]byte) (cipher.Block, []byte, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, nil, err
	}
	return block, iv[:], nil
}
