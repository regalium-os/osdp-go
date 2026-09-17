// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package frame

// Scheme identifies the trailing error check a frame carries. Which one is in
// use is declared by bit 2 of the control byte, per SIA OSDP v2.2.2 §5.6.
type Scheme uint8

const (
	// SchemeChecksum is the single-octet checksum used when control bit 2 is
	// clear. Supported for interoperability with older devices; new
	// deployments should negotiate CRC.
	SchemeChecksum Scheme = iota

	// SchemeCRC16 is the two-octet CRC used when control bit 2 is set.
	SchemeCRC16
)

// String implements fmt.Stringer, and supplies the value of the
// osdp.frame.check_scheme span attribute.
func (s Scheme) String() string {
	if s == SchemeCRC16 {
		return "crc16"
	}
	return "checksum"
}

// Size is the number of octets the check occupies on the wire.
func (s Scheme) Size() int {
	if s == SchemeCRC16 {
		return 2
	}
	return 1
}

// crcInit is the CRC-16 seed OSDP specifies.
//
// This is CRC-16/AUG-CCITT (also catalogued as CRC-16/SPI-FUJITSU): polynomial
// 0x1021, initial value 0x1D0F, input and output not reflected, no final XOR.
// Its published check value -- the CRC of the ASCII string "123456789" -- is
// 0xE5CC, which TestCRCMatchesPublishedCheckValue asserts.
//
// The seed is worth stating explicitly because 0x1E79 circulates as an OSDP CRC
// initial value and is wrong; it yields 0xF84F for the same input.
const crcInit uint16 = 0x1D0F

// CRC16 computes the OSDP CRC over b.
//
// b must be the frame from the start-of-message octet through the end of the
// data block: the mark byte, if any, is excluded, and the check octets
// themselves are not part of the input.
//
// b is read and never retained or modified.
func CRC16(b []byte) uint16 {
	crc := crcInit
	for _, c := range b {
		crc = (crc >> 8) | (crc << 8)
		crc ^= uint16(c)
		crc ^= (crc & 0xff) >> 4
		crc ^= crc << 12
		crc ^= (crc & 0xff) << 5
	}
	return crc
}

// Checksum computes the OSDP single-octet checksum over b: the two's complement
// of the low octet of the running sum.
//
// The same input rules as CRC16 apply. b is read and never retained or
// modified.
func Checksum(b []byte) uint8 {
	var sum uint8
	for _, c := range b {
		sum += c
	}
	return ^sum + 1
}
