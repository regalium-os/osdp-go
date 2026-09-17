package telemetry

// Attribute keys, defined once so that osdp.device.address means the same thing
// in a driver span and a bus span.
//
// The list is deliberately short. An attribute that cannot be acted on in an
// incident is cost without benefit, and cardinality on a bus with hundreds of
// devices is real.
const (
	// AttrDeviceAddress is the OSDP peripheral device address, 0x00-0x7E, or
	// 0x7F for the broadcast configuration address.
	AttrDeviceAddress = "osdp.device.address"

	// AttrIsReply distinguishes a reply from a peripheral device from a
	// command issued by the control panel.
	AttrIsReply = "osdp.frame.is_reply"

	// AttrSequence is the frame sequence number, 0-3.
	AttrSequence = "osdp.frame.sequence"

	// AttrFrameLength is the declared total frame length in octets.
	AttrFrameLength = "osdp.frame.length"

	// AttrCheckScheme is "crc16" or "checksum", per the control byte.
	AttrCheckScheme = "osdp.frame.check_scheme"

	// AttrSecureBlockType is the security block type when one is present.
	AttrSecureBlockType = "osdp.secure.block_type"

	// AttrCommand is the command or reply code carried by the frame.
	AttrCommand = "osdp.command.code"

	// AttrCardFormat and AttrCardBitCount describe a credential without
	// recording it. The card number itself is personal data and is never a
	// span attribute.
	AttrCardFormat   = "osdp.card.format"
	AttrCardBitCount = "osdp.card.bit_count"
)
