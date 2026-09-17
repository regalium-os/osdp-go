package cmd

// Function is an osdp_PDCAP capability function code: the first octet of each
// three-octet entry in a capability report. SIA OSDP v2.2.2 §6.6.
type Function byte

// Capability function codes, SIA OSDP v2.2.2 §6.6.
//
// What the compliance and item octets mean is defined per function and shares
// no common scheme: for osdp_CAP_READER_LED_CONTROL the item octet counts LEDs,
// for osdp_CAP_RECEIVE_BUFFERSIZE the pair is a little-endian octet count, and
// for osdp_CAP_COMMUNICATION_SECURITY both are bit fields. That is why
// CapabilityReport keeps the raw entries: a panel that understands a function
// this library does not must still be able to read it.
const (
	FuncContactStatus   Function = 0x01 // monitored input points
	FuncOutputControl   Function = 0x02 // controlled output points
	FuncCardDataFormat  Function = 0x03 // 1 = bit array, 2 = BCD characters
	FuncReaderLED       Function = 0x04 // LEDs per reader
	FuncReaderAudible   Function = 0x05 // audible annunciators per reader
	FuncReaderText      Function = 0x06 // text output rows per reader
	FuncTimeKeeping     Function = 0x07 // withdrawn in v2.2.2; devices still report it
	FuncCheckCharacter  Function = 0x08 // 1 = CRC-16 supported, 0 = checksum only
	FuncCommSecurity    Function = 0x09 // Secure Channel support
	FuncReceiveBuffer   Function = 0x0A // largest single message accepted
	FuncCombinedMessage Function = 0x0B // largest multi-part message accepted
	FuncSmartCard       Function = 0x0C // transparent smart-card transport
	FuncReaders         Function = 0x0D // card readers attached
	FuncBiometrics      Function = 0x0E // biometric sensor type
	FuncSecurePINEntry  Function = 0x0F // PIN entered at the reader, not the panel
	FuncOSDPVersion     Function = 0x10 // specification revision the device claims
)

// Bit fields of the osdp_CAP_COMMUNICATION_SECURITY entry, SIA OSDP v2.2.2
// §6.6. The two octets carry different questions: whether the device can speak
// Secure Channel at all, and whether it is still holding the default key.
const (
	// SecurityAES128 is bit 0 of the compliance octet: the device implements
	// the mandated AES-128 suite.
	SecurityAES128 byte = 0x01

	// SecurityDefaultKey is bit 0 of the item octet: the device is still using
	// SCBK-D, the well-known default key from the specification. A device
	// reporting this is installable but not yet secure, and a panel should say
	// so rather than treat the channel as trustworthy.
	SecurityDefaultKey byte = 0x01
)

// CapabilitySize is the width of one capability entry on the wire.
const CapabilitySize = 3

// Capability is one entry of an osdp_PDCAP reply.
//
// The entry is kept verbatim rather than interpreted, because interpretation
// depends on the function code and this package does not pretend to know every
// function a device may report. The typed accessors on CapabilityReport cover
// the ones it does.
type Capability struct {
	// Function is the capability being reported.
	Function Function
	// Compliance is the function-specific compliance level.
	Compliance byte
	// Items is the function-specific item count.
	Items byte
}

// CapabilityReport is the decoded osdp_PDCAP payload: everything a device says
// it can do, in the order it said it.
//
// Order is preserved because re-encoding must reproduce the reply octet for
// octet, and devices do not agree on an ordering. A report may repeat a
// function; the accessors below return the first occurrence, which is what a
// device that duplicates an entry means by it.
type CapabilityReport []Capability

// ParseCapabilities decodes an osdp_PDCAP payload.
//
// Entries are copied into the returned slice, so data is neither retained nor
// aliased and the report outlives the read buffer. This differs from the other
// parsers in this package on purpose: a capability report is read once at
// enrolment and then held for the lifetime of the device, whereas a card read
// is consumed immediately.
//
// A payload whose length is not a multiple of CapabilitySize is a truncated
// reply, which on a shared line is ordinary corruption rather than a bug.
func ParseCapabilities(data []byte) (CapabilityReport, error) {
	if len(data)%CapabilitySize != 0 {
		return nil, ErrShortPayload
	}
	if len(data) == 0 {
		return CapabilityReport{}, nil
	}

	out := make(CapabilityReport, 0, len(data)/CapabilitySize)
	for i := 0; i+CapabilitySize <= len(data); i += CapabilitySize {
		out = append(out, Capability{
			Function:   Function(data[i]),
			Compliance: data[i+1],
			Items:      data[i+2],
		})
	}
	return out, nil
}

// Append encodes the report onto dst and returns the extended slice.
//
// Entries are emitted in the order held, so a report decoded from a device and
// appended again reproduces the original payload exactly -- including a
// function code this library does not recognise. dst is not retained.
func (r CapabilityReport) Append(dst []byte) []byte {
	for _, c := range r {
		dst = append(dst, byte(c.Function), c.Compliance, c.Items)
	}
	return dst
}

// Get returns the first entry for fn, and whether the device reported it.
func (r CapabilityReport) Get(fn Function) (Capability, bool) {
	for _, c := range r {
		if c.Function == fn {
			return c, true
		}
	}
	return Capability{}, false
}

// Supports reports whether the device claims fn with a non-zero compliance
// level. A function reported at compliance zero is a device explicitly saying
// it does not have the capability, which is not the same as omitting it -- but
// for a caller asking "can it?", both answers are no.
func (r CapabilityReport) Supports(fn Function) bool {
	c, ok := r.Get(fn)
	return ok && c.Compliance != 0
}

// ItemsOf returns the item count reported for fn, or zero when the device did
// not report the function at all.
func (r CapabilityReport) ItemsOf(fn Function) int {
	c, ok := r.Get(fn)
	if !ok {
		return 0
	}
	return int(c.Items)
}

// SecureChannel reports whether the device claims the mandated AES-128 suite,
// and whether it is still holding the default key SCBK-D.
//
// A device that omits osdp_CAP_COMMUNICATION_SECURITY entirely is reported as
// not capable: silence is not consent to attempt a handshake.
func (r CapabilityReport) SecureChannel() (capable, defaultKey bool) {
	c, ok := r.Get(FuncCommSecurity)
	if !ok {
		return false, false
	}
	return c.Compliance&SecurityAES128 != 0, c.Items&SecurityDefaultKey != 0
}

// ReceiveBufferSize returns the largest single message the device says it
// accepts, in octets, or zero when it did not report one.
//
// The size is split across the two octets of the entry, least significant
// first: osdp_CAP_RECEIVE_BUFFERSIZE is the one capability whose compliance
// octet is not a compliance level at all.
func (r CapabilityReport) ReceiveBufferSize() int {
	c, ok := r.Get(FuncReceiveBuffer)
	if !ok {
		return 0
	}
	return int(c.Items)<<8 | int(c.Compliance)
}

// Name returns the specification's mnemonic for the function code.
func (f Function) Name() string {
	if n, ok := functionNames[f]; ok {
		return n
	}
	return "osdp_CAP_UNKNOWN"
}

// String implements fmt.Stringer.
func (f Function) String() string { return f.Name() }
