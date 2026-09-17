// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package cmd

import "encoding/binary"

// DeviceID is the osdp_PDID reply: what a device says it is.
//
// It is the first thing a panel asks for and the basis of every vendor
// decision that follows, because VendorCode is the OUI a provider keys on.
type DeviceID struct {
	// VendorCode is the IEEE OUI, three octets, most significant first.
	VendorCode [3]byte
	// Model and Version are vendor-defined.
	Model   byte
	Version byte
	// Serial identifies the unit, little-endian.
	Serial uint32
	// Firmware is major, minor, build.
	Firmware [3]byte
}

// DeviceIDSize is the fixed osdp_PDID payload length.
const DeviceIDSize = 12

// ParseDeviceID decodes an osdp_PDID payload. data is read and not retained.
func ParseDeviceID(data []byte) (DeviceID, error) {
	if len(data) < DeviceIDSize {
		return DeviceID{}, ErrShortPayload
	}
	return DeviceID{
		VendorCode: [3]byte(data[0:3]),
		Model:      data[3],
		Version:    data[4],
		Serial:     binary.LittleEndian.Uint32(data[5:9]),
		Firmware:   [3]byte(data[9:12]),
	}, nil
}

// OUI returns the vendor code as a single integer, which is how providers
// register themselves.
func (d DeviceID) OUI() uint32 {
	return uint32(d.VendorCode[0])<<16 | uint32(d.VendorCode[1])<<8 | uint32(d.VendorCode[2])
}

// CardRead is the osdp_RAW reply: a credential presented at a reader.
type CardRead struct {
	// Reader is the reader number on the device, usually zero.
	Reader byte
	// Format is the format code; zero means the raw bit stream.
	Format byte
	// BitCount is the number of significant bits in Data, which is not
	// necessarily a multiple of eight -- a 26-bit Wiegand credential is the
	// commonest case in the field.
	BitCount uint16
	// Data holds the credential bits, most significant first, zero padded to
	// the next octet.
	//
	// This is personal data. It must never reach a log or a span.
	Data []byte
}

// ParseCardRead decodes an osdp_RAW payload. Data aliases the input.
func ParseCardRead(data []byte) (CardRead, error) {
	const header = 4
	if len(data) < header {
		return CardRead{}, ErrShortPayload
	}
	c := CardRead{
		Reader:   data[0],
		Format:   data[1],
		BitCount: binary.LittleEndian.Uint16(data[2:4]),
		Data:     data[header:],
	}
	if need := (int(c.BitCount) + 7) / 8; len(c.Data) < need {
		return CardRead{}, ErrShortPayload
	}
	return c, nil
}

// KeypadEntry is the osdp_KEYPAD reply: digits entered at a keypad.
type KeypadEntry struct {
	Reader byte
	// Keys are the raw key codes. Like a credential, these are personal data:
	// they are frequently a PIN.
	Keys []byte
}

// ParseKeypadEntry decodes an osdp_KEYPAD payload. Keys aliases the input.
func ParseKeypadEntry(data []byte) (KeypadEntry, error) {
	const header = 2
	if len(data) < header {
		return KeypadEntry{}, ErrShortPayload
	}
	count := int(data[1])
	if len(data[header:]) < count {
		return KeypadEntry{}, ErrShortPayload
	}
	return KeypadEntry{Reader: data[0], Keys: data[header : header+count]}, nil
}

// ManufacturerMessage is the osdp_MFG body: an OUI and a vendor-defined
// remainder that only that vendor's provider understands.
//
// frame and cmd deliberately stop here. Interpreting Body is what a Provider is
// for, and pulling it into the codec is how a protocol library acquires a
// per-vendor fork of its own parser.
type ManufacturerMessage struct {
	OUI  uint32
	Body []byte
}

// ParseManufacturerMessage decodes an osdp_MFG payload. Body aliases the input.
func ParseManufacturerMessage(data []byte) (ManufacturerMessage, error) {
	const header = 3
	if len(data) < header {
		return ManufacturerMessage{}, ErrShortPayload
	}
	return ManufacturerMessage{
		OUI:  uint32(data[0])<<16 | uint32(data[1])<<8 | uint32(data[2]),
		Body: data[header:],
	}, nil
}

// Append encodes the message onto dst and returns the extended slice: the OUI
// most significant octet first, then the vendor-defined body verbatim.
//
// The body is copied into dst and not retained. A provider that round-trips a
// message it did not recognise reproduces the original payload octet for octet,
// which is what lets a panel forward a vendor message it cannot read.
func (m ManufacturerMessage) Append(dst []byte) []byte {
	dst = append(dst, byte(m.OUI>>16), byte(m.OUI>>8), byte(m.OUI))
	return append(dst, m.Body...)
}
