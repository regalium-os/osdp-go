// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package frame

// Wire constants from SIA OSDP v2.2.2 §5.
const (
	// SOM is the start-of-message octet that opens every frame.
	SOM byte = 0x53

	// Mark is the optional octet some devices emit before SOM to let a
	// receiver find the edge of a frame on a line that has been idle. It is
	// not counted by the length field and is not covered by the error check.
	Mark byte = 0xFF

	// HeaderSize is SOM, address, the two length octets and the control
	// octet: the fixed part every frame carries before any security block.
	HeaderSize = 5

	// BroadcastAddress reaches every peripheral device on the line. It is the
	// configuration address, used before a device has been given its own.
	BroadcastAddress Address = 0x7F

	// replyFlag is set in the address octet of a message sent by a peripheral
	// device, distinguishing a reply from the command that prompted it.
	replyFlag byte = 0x80
)

// Address is an OSDP peripheral device address.
//
// The wire octet carries the address in its low seven bits and a reply flag in
// the high bit; Address holds only the address, and Frame.IsReply carries the
// flag separately. Valid addresses are 0x00 to 0x7E, plus BroadcastAddress.
type Address byte

// Valid reports whether a is addressable, which includes the broadcast address.
func (a Address) Valid() bool { return a <= BroadcastAddress }

// Control is the control octet: sequence number, check scheme, and whether a
// security block follows the header. SIA OSDP v2.2.2 §5.5.
//
// Bits 4 to 7 are reserved. They are preserved verbatim through a decode and
// re-encode so that a frame from a device setting them is reproduced exactly
// rather than silently normalised.
type Control byte

const (
	ctrlSequenceMask byte = 0x03
	ctrlCRCFlag      byte = 0x04
	ctrlSecurityFlag byte = 0x08
)

// Sequence returns the frame sequence number, 0 to 3.
//
// Zero is not part of the normal rotation: a device uses it to signal that it
// has lost synchronisation and requires the exchange to restart. The rotation
// proper is 1, 2, 3, 1.
func (c Control) Sequence() uint8 { return uint8(c) & ctrlSequenceMask }

// Scheme reports which error check the frame carries.
func (c Control) Scheme() Scheme {
	if byte(c)&ctrlCRCFlag != 0 {
		return SchemeCRC16
	}
	return SchemeChecksum
}

// HasSecurityBlock reports whether a security block follows the header.
func (c Control) HasSecurityBlock() bool { return byte(c)&ctrlSecurityFlag != 0 }

// NewControl builds a control octet. Reserved bits are left clear; to preserve
// reserved bits observed on the wire, keep the decoded Control instead.
func NewControl(seq uint8, scheme Scheme, security bool) Control {
	c := seq & ctrlSequenceMask
	if scheme == SchemeCRC16 {
		c |= ctrlCRCFlag
	}
	if security {
		c |= ctrlSecurityFlag
	}
	return Control(c)
}

// SecurityBlock is the optional block between the control octet and the command
// or reply code. Its contents are interpreted by the secure package; frame
// treats it as a length-prefixed envelope and nothing more.
type SecurityBlock struct {
	// Type identifies the block, and with it the phase of the secure channel
	// handshake or the mode of an established session.
	Type byte

	// Data is the remainder of the block after the length and type octets.
	//
	// It aliases the buffer passed to Decode. See Frame.Data.
	Data []byte
}

// MaxSecurityBlockSize is the largest security block the wire can describe,
// the leading length octet included. SIA OSDP v2.2.2 §5.8.
const MaxSecurityBlockSize = 0xFF

// MaxFrameSize is the largest frame the two-octet length field can describe.
//
// The specification's own payload ceiling is far lower, and a device's real
// one lower still -- osdp_CAP_RECEIVE_BUFFERSIZE is how a panel finds out. This
// is not that limit: it is the point past which the header cannot say how long
// the frame is, which is a property of the format rather than of any device.
const MaxFrameSize = 0xFFFF

// wireLen is the block's length as the leading length octet reports it: the
// length octet itself, the type octet, and the data.
func (s *SecurityBlock) wireLen() int {
	if s == nil {
		return 0
	}
	return 2 + len(s.Data)
}
