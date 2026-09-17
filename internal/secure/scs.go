// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package secure

// BlockType identifies a security block, and with it the phase of the handshake
// or the protection applied to an established session's payload.
// SIA OSDP v2.2.2 §7.
type BlockType byte

const (
	// SCS11 carries the control panel's challenge, osdp_CHLNG.
	SCS11 BlockType = 0x11
	// SCS12 carries the device's reply, osdp_CCRYPT.
	SCS12 BlockType = 0x12
	// SCS13 carries the control panel's server cryptogram, osdp_SCRYPT.
	SCS13 BlockType = 0x13
	// SCS14 carries the device's initial R-MAC, osdp_RMAC_I.
	SCS14 BlockType = 0x14

	// SCS15 marks an established session, panel to device, authenticated but
	// not encrypted.
	SCS15 BlockType = 0x15
	// SCS16 is the same from the device.
	SCS16 BlockType = 0x16

	// SCS17 marks an established session, panel to device, both encrypted and
	// authenticated.
	SCS17 BlockType = 0x17
	// SCS18 is the same from the device.
	SCS18 BlockType = 0x18
)

// Handshake reports whether the block belongs to session establishment rather
// than to an established session.
func (b BlockType) Handshake() bool { return b >= SCS11 && b <= SCS14 }

// Established reports whether the block marks a running secure session.
func (b BlockType) Established() bool { return b >= SCS15 && b <= SCS18 }

// Encrypted reports whether the payload of a frame carrying this block is
// enciphered. Blocks SCS15 and SCS16 are authenticated only.
func (b BlockType) Encrypted() bool { return b == SCS17 || b == SCS18 }

// FromPD reports whether the block travels from a peripheral device to the
// control panel. The two directions alternate through the handshake and remain
// distinct afterwards, which is what keeps the two MAC chains separate.
func (b BlockType) FromPD() bool {
	switch b {
	case SCS12, SCS14, SCS16, SCS18:
		return true
	default:
		return false
	}
}

// String implements fmt.Stringer and supplies the osdp.secure.block_type span
// attribute.
func (b BlockType) String() string {
	switch b {
	case SCS11:
		return "SCS_11"
	case SCS12:
		return "SCS_12"
	case SCS13:
		return "SCS_13"
	case SCS14:
		return "SCS_14"
	case SCS15:
		return "SCS_15"
	case SCS16:
		return "SCS_16"
	case SCS17:
		return "SCS_17"
	case SCS18:
		return "SCS_18"
	default:
		return "SCS_unknown"
	}
}
