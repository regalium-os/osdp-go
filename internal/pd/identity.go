// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd

import (
	"encoding/binary"

	"github.com/regalium-os/osdp-go/internal/cmd"
)

// osdpVersion is the compliance value of the osdp_CAP_OSDP_VERSION entry.
//
// Per SIA OSDP v2.2.2 §6.6 the codes are 0 for unspecified, 1 for
// IEC 60839-11-5 and 2 for SIA OSDP 2.2. This package implements the latter, so
// a device built here says so rather than leaving a panel to guess.
const osdpVersion byte = 0x02

// defaultReceiveBuffer is how large a single command this device says it
// accepts, in octets.
//
// 128 is the figure the specification uses for a device that has nothing larger
// to offer, and it is what the panel half of this repository assumes when a
// device reports nothing at all. Claiming more than the application can
// actually buffer is how a panel comes to send a command that arrives truncated
// and is NAKed forever, so the conservative number is the default and
// WithCapabilities is how a device that knows better says so.
const defaultReceiveBuffer = 128

// appendDeviceID encodes an osdp_PDID payload onto dst and returns the extended
// slice. SIA OSDP v2.2.2 §6.4.
//
// The two orderings in one payload are the specification's, not a
// transcription error: the vendor code is an IEEE OUI, written most significant
// octet first everywhere it appears, while the serial number is an OSDP integer
// and every one of those is little-endian. Getting this backwards yields a
// device that identifies itself as a different vendor, which a provider
// registry then keys on.
//
// dst may be nil and is not retained. cmd.ParseDeviceID reverses this exactly;
// identity_test.go asserts the round trip.
func appendDeviceID(dst []byte, id cmd.DeviceID) []byte {
	dst = append(dst, id.VendorCode[0], id.VendorCode[1], id.VendorCode[2])
	dst = append(dst, id.Model, id.Version)
	dst = binary.LittleEndian.AppendUint32(dst, id.Serial)
	return append(dst, id.Firmware[0], id.Firmware[1], id.Firmware[2])
}

// defaultCapabilities is the osdp_PDCAP a device reports when the application
// has not supplied one of its own.
//
// It is derived from the contact counts rather than fixed, because a capability
// report that disagrees with the status replies is worse than no report at all:
// a panel told there are four inputs and then handed two octets of osdp_ISTATR
// has no way to decide which of the two to believe, and both are this device.
//
// osdp_CAP_COMMUNICATION_SECURITY is reported at compliance zero deliberately.
// This package does not implement the device half of the Secure Channel, and a
// device that claimed AES-128 would be challenged and would have to NAK the
// challenge -- which at the panel reads as a reader that is broken rather than
// one that is merely plain. See the package documentation for the seam.
func defaultCapabilities(inputs, outputs, readers int) cmd.CapabilityReport {
	return cmd.CapabilityReport{
		{Function: cmd.FuncContactStatus, Compliance: 0x01, Items: byte(inputs)},
		{Function: cmd.FuncOutputControl, Compliance: 0x01, Items: byte(outputs)},

		// Compliance 1 is the bit-array format: this device reports what the
		// reader read, without transcribing it into BCD characters.
		{Function: cmd.FuncCardDataFormat, Compliance: 0x01, Items: 0x00},

		// CRC-16 is supported. A device that only claimed the checksum would
		// be answered in kind by any sensible panel, and the checksum catches
		// far less on a line shared with a motor.
		{Function: cmd.FuncCheckCharacter, Compliance: 0x01, Items: 0x00},

		{Function: cmd.FuncCommSecurity, Compliance: 0x00, Items: 0x00},

		// The receive buffer entry is the one whose compliance octet is not a
		// compliance level: the pair is a little-endian octet count, least
		// significant first. See cmd.CapabilityReport.ReceiveBufferSize.
		{
			Function:   cmd.FuncReceiveBuffer,
			Compliance: byte(defaultReceiveBuffer),
			Items:      byte(defaultReceiveBuffer >> 8),
		},

		{Function: cmd.FuncReaders, Compliance: 0x01, Items: byte(readers)},
		{Function: cmd.FuncOSDPVersion, Compliance: osdpVersion, Items: 0x00},
	}
}
