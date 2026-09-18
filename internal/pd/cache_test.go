// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package pd_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/internal/pd"
)

// The reply cache of SIA OSDP v2.2.2 §5.7, which is the whole reason a device
// runtime is more than a switch statement. The panel half of this repository
// assumes it: internal/bus/sequence_test.go asserts that a retry repeats the
// sequence number rather than advancing it, and this is the other end of that
// contract.

// TestARepeatedSequenceReplaysTheCachedReply, octet for octet.
//
// The comparison is on the encoded frame because that is the only thing the
// panel sees: sequence number, code, payload and error check all have to come
// back identical, or the retry is answered with something the panel will
// reconcile against the wrong command.
//
// The device is loaded with two different credentials on purpose. A reply that
// were merely rebuilt rather than replayed would carry the second one, and the
// first -- the credential actually presented at that moment -- would be gone
// without either end noticing.
func TestARepeatedSequenceReplaysTheCachedReply(t *testing.T) {
	ctx := context.Background()
	d := newDevice(t)

	for _, bits := range []byte{0x11, 0x22} {
		if err := d.ReportCardRead(ctx, cmd.CardRead{Format: 1, BitCount: 8, Data: []byte{bits}}); err != nil {
			t.Fatalf("ReportCardRead: %v", err)
		}
	}

	first := answer(t, d, poll(t, 1))
	again := answer(t, d, poll(t, 1))

	if !bytes.Equal(wire(t, first), wire(t, again)) {
		t.Errorf("a repeated sequence produced\n  %x\nafter\n  %x", wire(t, again), wire(t, first))
	}
}

// TestARepeatedSequenceDoesNotExecuteTheCommandTwice.
//
// This is the property the cache exists for, stated directly. A panel repeats a
// sequence number to mean "I did not hear you"; a device that treated the
// repeat as a new command would release a strike a second time, several seconds
// after the cardholder walked through, and nothing downstream would ever know
// the door opened twice.
func TestARepeatedSequenceDoesNotExecuteTheCommandTwice(t *testing.T) {
	var executed int
	d := newDevice(t, pd.WithHandler(
		func(_ context.Context, _ cmd.Message) (cmd.Message, bool) {
			executed++
			return cmd.Message{Code: cmd.MFGReply}, true
		},
	))

	vendor := cmd.Message{Code: cmd.MFG, Data: []byte{0x00, 0x00, 0x01, 0xFF}}
	answer(t, d, command(t, 2, vendor))
	answer(t, d, command(t, 2, vendor))

	if executed != 1 {
		t.Errorf("the command executed %d times; a repeated sequence number is "+
			"a retransmission, not a second instruction", executed)
	}
}

// TestARepeatedPollDoesNotConsumeASecondEvent.
//
// A poll looks idempotent and is not: it drains the event queue, so the reply
// to one poll is a card read and the reply to the next is a different one.
// Without the cache, a panel that missed the first reply would receive the
// second card read in answer to its retry -- and the first credential, the one
// actually presented at that moment, would be gone.
func TestARepeatedPollDoesNotConsumeASecondEvent(t *testing.T) {
	ctx := context.Background()
	d := newDevice(t)

	first := cmd.CardRead{Format: 1, BitCount: 26, Data: []byte{0x12, 0x34, 0x56, 0x00}}
	second := cmd.CardRead{Format: 1, BitCount: 26, Data: []byte{0xAB, 0xCD, 0xEF, 0x00}}
	for _, c := range []cmd.CardRead{first, second} {
		if err := d.ReportCardRead(ctx, c); err != nil {
			t.Fatalf("ReportCardRead: %v", err)
		}
	}

	reported := answer(t, d, poll(t, 1))
	replyCode(t, reported, cmd.Raw)
	answer(t, d, poll(t, 1))

	if got := d.Pending(); got != 1 {
		t.Errorf("%d events remain after a poll and its retry, want 1: the retry "+
			"consumed a credential the panel never saw", got)
	}
}

// TestAFullRotationIsNotAReplay.
//
// The sequence rotation is 1, 2, 3, 1, so the same number comes round every
// three commands. A cache with a slot per number would be indistinguishable
// from a correct one for exactly one rotation and would then start answering
// new commands with answers three commands stale.
func TestAFullRotationIsNotAReplay(t *testing.T) {
	ctx := context.Background()
	d := newDevice(t)

	for range 2 {
		if err := d.ReportCardRead(ctx, cmd.CardRead{BitCount: 8, Data: []byte{0x01}}); err != nil {
			t.Fatalf("ReportCardRead: %v", err)
		}
	}

	for _, seq := range []uint8{1, 2, 3, 1} {
		answer(t, d, poll(t, seq))
	}

	if got := d.Pending(); got != 0 {
		t.Errorf("%d events remain after a full rotation, want 0: the second "+
			"sequence 1 was answered from the cache instead of being executed", got)
	}
}

// TestTheReplyDoesNotAliasTheCommandBuffer.
//
// A cached reply outlives the read it came from by definition: it is held until
// the next command arrives into the same buffer. A handler that returns a slice
// of the command's own payload -- the natural way to echo an osdp_MFG body --
// would otherwise have the cached reply mutate under the device, and the replay
// would be whatever noise landed in the buffer next.
func TestTheReplyDoesNotAliasTheCommandBuffer(t *testing.T) {
	ctx := context.Background()
	d := newDevice(t, pd.WithHandler(
		func(_ context.Context, m cmd.Message) (cmd.Message, bool) {
			return cmd.Message{Code: cmd.MFGReply, Data: m.Data}, true
		},
	))

	body := []byte{0x00, 0x00, 0x01, 0xDE, 0xAD}
	sent, err := cmd.Encode(ctx, cmd.Message{Code: cmd.MFG, Data: body}, testAddress, 1, frame.SchemeCRC16)
	if err != nil {
		t.Fatalf("cmd.Encode: %v", err)
	}
	buf, err := sent.Append(ctx, nil)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	decoded, err := frame.Decode(ctx, buf)
	if err != nil {
		t.Fatalf("frame.Decode: %v", err)
	}

	reply := answer(t, d, decoded)
	held := bytes.Clone(reply.Data)

	// The runtime reads the next frame into the same buffer.
	for i := range buf {
		buf[i] = 0xAA
	}

	if !bytes.Equal(reply.Data, held) {
		t.Errorf("the reply payload became %x when the read buffer was reused, was %x",
			reply.Data, held)
	}
}
