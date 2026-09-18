// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package cmd

import "encoding/binary"

// Communication is a device's address and line speed, as carried by osdp_COMSET
// and confirmed by osdp_COM. SIA OSDP v2.2.2 §6.14.
type Communication struct {
	// Address is the device's bus address, 0x00 to 0x7E.
	Address byte

	// Baud is the line speed. OSDP defines 9600, 19200, 38400, 57600, 115200
	// and 230400; every device must support 9600, which is why a bus that has
	// lost track of itself can always be recovered there.
	Baud uint32
}

// CommunicationSize is the fixed osdp_COMSET and osdp_COM payload length: the
// address, then four octets of baud rate.
const CommunicationSize = 5

// CommunicationCommand builds an osdp_COMSET.
//
// # This is the command that can lose a device
//
// A device applies the change after replying, so the panel's next frame must
// use the new address and the new speed. Get either wrong and the device is
// still there, still working, and unreachable -- there is no way to ask a
// device what address it is on, because asking requires knowing.
//
// Two consequences worth planning around. Send it to one device at a time: a
// bus of readers fresh from the box all answer to address 0, so a broadcast
// osdp_COMSET sets them all to the same new address and leaves you worse off.
// And change one thing at a time where you can, because a device that took the
// address but not the speed is indistinguishable from one that took neither.
//
// The reply carries what the device actually adopted, which is not always what
// it was asked for. Believe the reply.
func CommunicationCommand(c Communication) Message {
	data := make([]byte, CommunicationSize)
	data[0] = c.Address
	binary.LittleEndian.PutUint32(data[1:], c.Baud)

	return Message{Code: ComSet, Data: data}
}

// ParseCommunication decodes an osdp_COM reply, or an osdp_COMSET payload.
//
// data is read and not retained.
func ParseCommunication(data []byte) (Communication, error) {
	if len(data) < CommunicationSize {
		return Communication{}, ErrShortPayload
	}
	return Communication{
		Address: data[0],
		Baud:    binary.LittleEndian.Uint32(data[1:CommunicationSize]),
	}, nil
}

// StandardBaudRates are the line speeds the specification defines.
//
// A device is required to support 9600 and negotiates upward. They are listed
// so a caller can check before asking for something no device will take, but
// nothing here refuses an unlisted value: a vendor that supports one is
// entitled to, and this package does not know better than the device.
var StandardBaudRates = []uint32{9600, 19200, 38400, 57600, 115200, 230400}

// IsStandardBaud reports whether b is one of the specification's rates.
func IsStandardBaud(b uint32) bool {
	for _, r := range StandardBaudRates {
		if r == b {
			return true
		}
	}
	return false
}
