// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/regalium-os/osdp-go"
)

// tracingPort prints every octet that crosses it, in both directions.
//
// It is the reason -trace costs the library nothing: osdp.Port is an interface
// the consumer may implement, so a diagnostic wrapper goes between the runtime
// and the real port without either knowing. The same seam is how a proprietary
// serial bridge would be added.
//
// It decodes what it can, because a hex dump alone answers "is anything on the
// wire" and not "is it the right thing". A line that fails to decode is printed
// as octets and the reason, which is usually the interesting case: noise, a
// half-duplex turnaround too fast for the converter, or two devices answering
// at once.
type tracingPort struct {
	osdp.Port
}

// Write prints the frame the panel is sending, then sends it.
func (p *tracingPort) Write(b []byte) (int, error) {
	p.dump("TX", b)
	return p.Port.Write(b)
}

// Read prints whatever arrived.
//
// It prints what the read returned rather than a whole frame: a converter hands
// over whatever was in its buffer, and seeing a frame arrive in three pieces is
// itself a useful thing to know about a line.
func (p *tracingPort) Read(b []byte) (int, error) {
	n, err := p.Port.Read(b)
	if n > 0 {
		p.dump("RX", b[:n])
	}
	return n, err
}

// dump prints one direction's octets and whatever sense can be made of them.
func (p *tracingPort) dump(direction string, b []byte) {
	fmt.Printf("%s  %s  % X\n", time.Now().Format("15:04:05.000"), direction, b)

	f, err := osdp.Decode(context.Background(), b)
	if err != nil {
		fmt.Printf("                    %s\n", explain(err))
		return
	}

	kind := "command"
	if f.IsReply {
		kind = "reply"
	}
	secure := ""
	if f.Security != nil {
		secure = fmt.Sprintf(", %s", osdp.SecureBlockType(f.Security.Type))
	}

	fmt.Printf("                    addr %02X, seq %d, %s %s, %d octets%s\n",
		byte(f.Address), f.Control.Sequence(), kind,
		osdp.Code(f.Code).Name(f.IsReply), len(f.Data), secure)
}

// isErr is errors.Is, named for what the switch above is asking.
func isErr(err, target error) bool { return errors.Is(err, target) }

// explain turns a decode failure into the thing to go and check.
//
// The errors are the same ones the codec returns; what a hex dump cannot say is
// which of them means "wait" and which means "your wiring is wrong".
func explain(err error) string {
	switch {
	case isErr(err, osdp.ErrShortBuffer):
		return "partial frame — more octets to come, normal on a converter"
	case isErr(err, osdp.ErrBadCheck):
		return "BAD CHECK — the frame arrived corrupted; suspect termination or noise"
	case isErr(err, osdp.ErrNoStartOfMessage):
		return "no start-of-message — line noise, or a reply to somebody else"
	case isErr(err, osdp.ErrLengthMismatch):
		return "length field disagrees with the frame"
	case isErr(err, osdp.ErrBadSecurityBlock):
		return "malformed security block"
	default:
		return fmt.Sprintf("undecodable: %v", err)
	}
}
