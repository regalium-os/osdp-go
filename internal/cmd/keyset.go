// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package cmd

// KeyType identifies which key an osdp_KEYSET carries. SIA OSDP v2.2.2 §6.16.
type KeyType byte

// KeySCBK is the Secure Channel Base Key: the per-device secret both ends
// derive session keys from. It is the only key type the specification defines,
// and the only one this package will build a command for -- an unrecognised
// type would be a key installed somewhere neither end can name.
const KeySCBK KeyType = 0x01

// KeySetHeaderSize is the fixed part of an osdp_KEYSET payload: the key type
// and the key length.
const KeySetHeaderSize = 2

// KeySetCommand builds an osdp_KEYSET carrying a new Secure Channel Base Key.
//
// # This command must never travel in the clear
//
// The payload is the key. A panel that sends this on an unencrypted channel has
// handed the site key to anyone with a pair of probes and a cupboard door, and
// nothing afterwards can take it back -- the device will happily use it, and so
// will they. The wire format offers no protection of its own: confidentiality
// here is entirely the Secure Channel's job.
//
// This package cannot enforce that, because it does not know what the frame it
// builds will be wrapped in. The bus does, and refuses. See bus.InstallKey.
//
// # Ownership
//
// key is copied into the returned message. The caller's array is not retained,
// and should be zeroed by the caller once it has been persisted.
func KeySetCommand(key []byte) Message {
	data := make([]byte, 0, KeySetHeaderSize+len(key))
	data = append(data, byte(KeySCBK), byte(len(key)))
	return Message{Code: KeySet, Data: append(data, key...)}
}

// KeySetPayload is a decoded osdp_KEYSET body.
//
// It exists for the device side of the protocol and for a panel inspecting what
// it is about to send. Key aliases the message it came from; see ParseKeySet.
type KeySetPayload struct {
	// Type is which key this installs.
	Type KeyType

	// Key is the key material.
	//
	// This is the most sensitive value in the protocol. Do not log it, do not
	// attach it to a span, and zero it once it has been stored. A card number
	// identifies one person; this opens every door on the line.
	Key []byte
}

// ParseKeySet decodes an osdp_KEYSET payload.
//
// Key aliases data and is not copied: the caller is about to install it and
// knows better than this package when the bytes may be released.
func ParseKeySet(data []byte) (KeySetPayload, error) {
	if len(data) < KeySetHeaderSize {
		return KeySetPayload{}, ErrShortPayload
	}

	length := int(data[1])
	if len(data[KeySetHeaderSize:]) < length {
		return KeySetPayload{}, ErrShortPayload
	}

	return KeySetPayload{
		Type: KeyType(data[0]),
		Key:  data[KeySetHeaderSize : KeySetHeaderSize+length],
	}, nil
}
