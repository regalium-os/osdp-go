package frame

// Frame is one decoded OSDP frame.
//
// A Frame holds the wire fields as they appeared, not a normalised view of
// them: the reserved control bits, the observed error check even when wrong,
// and the presence of a mark octet all survive a decode. That is what lets
// Decode followed by Append reproduce the input octet for octet, which in turn
// is what makes a fixture corpus of real traffic meaningful.
//
// # Buffer ownership
//
// Data and Security.Data alias the buffer passed to Decode. Nothing is copied.
// A Frame is therefore only valid while that buffer is unmodified; a caller
// reusing a read buffer across the poll cycle must call Clone to keep a Frame
// beyond the next read. This is the hot path in a control panel, and copying
// every payload to guard against a mistake the caller is better placed to avoid
// would be the wrong trade.
//
// A Frame is not safe for concurrent modification. Concurrent reads are fine.
type Frame struct {
	// HasMark reports that a mark octet preceded the start of message. It is
	// not covered by the length field or the error check, but it is reproduced
	// by Append because a device that sends one expects to see one.
	HasMark bool

	// IsReply distinguishes a reply from a peripheral device from a command
	// issued by the control panel. It is the high bit of the address octet.
	IsReply bool

	// Address is the peripheral device address, without the reply flag.
	Address Address

	// Control is the control octet, reserved bits included.
	Control Control

	// Security is the security block, or nil when the control octet does not
	// declare one.
	Security *SecurityBlock

	// Code is the command or reply code. Which of the two it is depends on
	// IsReply; the cmd package gives it meaning.
	Code byte

	// Data is the payload between the code and the error check. It aliases the
	// decode buffer; see Buffer ownership above.
	//
	// Deliberately untagged for telemetry: a payload may carry a credential,
	// and an untagged field cannot reach a span.
	Data []byte

	// Check is the error check exactly as it appeared on the wire, whether or
	// not it was correct. For SchemeChecksum only the low octet is used.
	Check uint16
}

// Len returns the total frame length in octets as the length field reports it:
// start of message through the error check, excluding any mark octet.
func (f Frame) Len() int {
	return HeaderSize + f.Security.wireLen() + 1 + len(f.Data) + f.Control.Scheme().Size()
}

// WireLen returns the octets Append will produce, mark octet included.
func (f Frame) WireLen() int {
	if f.HasMark {
		return f.Len() + 1
	}
	return f.Len()
}

// Clone returns a deep copy that no longer aliases the decode buffer. Use it to
// retain a Frame past the next read into a reused buffer.
func (f Frame) Clone() Frame {
	out := f
	if f.Data != nil {
		out.Data = make([]byte, len(f.Data))
		copy(out.Data, f.Data)
	}
	if f.Security != nil {
		sb := *f.Security
		if f.Security.Data != nil {
			sb.Data = make([]byte, len(f.Security.Data))
			copy(sb.Data, f.Security.Data)
		}
		out.Security = &sb
	}
	return out
}

// traceView is the projection of a Frame that may appear in a span.
//
// Telemetry attributes are declared here rather than as tags on Frame itself
// for two reasons. The field types are the ones the telemetry SDK converts
// natively -- int, string, bool -- so an attribute never degrades to a
// reflected placeholder. And, more importantly, it makes the safety boundary a
// single struct one can read in ten seconds: what is not in this type cannot be
// traced. Data and Security.Data are absent, permanently.
type traceView struct {
	Address  int    `telemetry:"trace:osdp.device.address"`
	IsReply  bool   `telemetry:"trace:osdp.frame.is_reply"`
	Sequence int    `telemetry:"trace:osdp.frame.sequence"`
	Scheme   string `telemetry:"trace:osdp.frame.check_scheme"`
	Length   int    `telemetry:"trace:osdp.frame.length"`
	Code     int    `telemetry:"trace:osdp.command.code"`
	Secure   bool   `telemetry:"trace:osdp.secure.present"`
}

// Trace returns the span attributes for this frame. It never exposes payload
// octets; see traceView.
func (f Frame) Trace() any {
	return traceView{
		Address:  int(f.Address),
		IsReply:  f.IsReply,
		Sequence: int(f.Control.Sequence()),
		Scheme:   f.Control.Scheme().String(),
		Length:   f.Len(),
		Code:     int(f.Code),
		Secure:   f.Security != nil,
	}
}
