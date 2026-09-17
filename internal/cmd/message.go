package cmd

import (
	"context"

	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/telemetry"
)

// Message is one command or reply: the code and the payload that follows it.
//
// It is the frame with the wire mechanics removed. Where frame.Frame carries
// the length, the check and the reserved bits, a Message carries only what the
// two ends are actually saying to each other.
//
// Data aliases the frame it came from, which aliases the read buffer. See
// frame.Frame for the ownership rules; Clone escapes them.
type Message struct {
	// Code is the command or reply code.
	Code Code

	// IsReply distinguishes the two, which matters because osdp_CHLNG and
	// osdp_CCRYPT share the value 0x76.
	IsReply bool

	// Data is the payload after the code. It may be empty.
	Data []byte
}

// Name returns the specification mnemonic for this message.
func (m Message) Name() string { return m.Code.Name(m.IsReply) }

// Clone returns a copy that no longer aliases the decode buffer.
func (m Message) Clone() Message {
	out := m
	if m.Data != nil {
		out.Data = make([]byte, len(m.Data))
		copy(out.Data, m.Data)
	}
	return out
}

// traceView is the projection of a Message that may appear in a span. The
// payload is absent and stays absent: it carries credentials.
type traceView struct {
	Code    int    `telemetry:"trace:osdp.command.code"`
	Name    string `telemetry:"trace:osdp.command.name"`
	IsReply bool   `telemetry:"trace:osdp.frame.is_reply"`
	Length  int    `telemetry:"trace:osdp.command.payload_length"`
}

// Trace returns the span attributes for this message.
func (m Message) Trace() any {
	return traceView{
		Code:    int(m.Code),
		Name:    m.Name(),
		IsReply: m.IsReply,
		Length:  len(m.Data),
	}
}

// Decode extracts the message from a decoded frame.
//
// It does not validate the payload against the code; that is the job of the
// typed accessors in payloads.go, which a caller reaches for only when it cares.
// A control panel forwarding an unrecognised osdp_MFG body to a provider should
// not have to parse it first.
func Decode(ctx context.Context, f frame.Frame) (Message, error) {
	m := Message{Code: Code(f.Code), IsReply: f.IsReply, Data: f.Data}

	_, span := telemetry.Start(ctx, "osdp.cmd.decode", m.Trace())
	defer span.End()

	return m, nil
}

// Encode places the message into a frame addressed to addr.
//
// The frame is sealed: its error check is computed. The caller supplies the
// sequence number and check scheme, which belong to the bus, not the message.
func Encode(
	ctx context.Context, m Message, addr frame.Address, seq uint8, scheme frame.Scheme,
) (frame.Frame, error) {
	_, span := telemetry.Start(ctx, "osdp.cmd.encode", m.Trace())
	defer span.End()

	if !addr.Valid() {
		span.RecordError(frame.ErrInvalidAddress)
		return frame.Frame{}, frame.ErrInvalidAddress
	}

	f := frame.Frame{
		IsReply: m.IsReply,
		Address: addr,
		Control: frame.NewControl(seq, scheme, false),
		Code:    byte(m.Code),
		Data:    m.Data,
	}
	f.Seal()
	return f, nil
}
