// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package panel_test

import (
	"sync"
	"testing"
	"time"

	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
)

// A device that remembers what it was sent, so a test can ask what actually
// reached the line rather than what was queued.

// recorder is a device that notes every command it was sent, so a test can ask
// what actually reached the line rather than what was queued.
type recorder struct {
	mu   sync.Mutex
	seen []cmd.Message
}

func (r *recorder) answer(command frame.Frame) frame.Frame {
	r.mu.Lock()
	r.seen = append(r.seen, cmd.Message{
		Code: cmd.Code(command.Code),
		Data: append([]byte(nil), command.Data...),
	})
	r.mu.Unlock()

	return cardReadOnPoll(command)
}

// swallowOne returns an answer function that ignores the first command of the
// given code, reproducing a reply lost on the wire.
func (r *recorder) swallowOne(code cmd.Code) func(frame.Frame) frame.Frame {
	var swallowed bool
	return func(command frame.Frame) frame.Frame {
		// Record first. The device received this frame; what it chose to do
		// about it is the next question, and a helper that forgot the
		// swallowed attempt would make a working retry look like a single
		// delivery.
		got := r.answer(command)
		if cmd.Code(command.Code) == code && !swallowed {
			swallowed = true
			return frame.Frame{} // not a reply: the device says nothing
		}
		return got
	}
}

// busyOnce answers the first command of the given code with osdp_BUSY, as a
// reader mid-cryptogram does, and behaves normally thereafter.
func (r *recorder) busyOnce(code cmd.Code) func(frame.Frame) frame.Frame {
	var refused bool
	return func(command frame.Frame) frame.Frame {
		got := r.answer(command)
		if cmd.Code(command.Code) == code && !refused {
			refused = true
			return reply(command.Address, command.Control.Sequence(), cmd.Busy, nil)
		}
		return got
	}
}

// waitForCount blocks until the device has been sent code at least n times.
func (r *recorder) waitForCount(t *testing.T, code cmd.Code, n int) {
	t.Helper()

	deadline := time.Now().Add(eventWait)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		var seen int
		for _, m := range r.seen {
			if m.Code == code {
				seen++
			}
		}
		r.mu.Unlock()
		if seen >= n {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("%s was sent fewer than %d times", code.Name(false), n)
}

// waitForCommand blocks until the device has been sent code, and returns it.
func (r *recorder) waitForCommand(t *testing.T, code cmd.Code) cmd.Message {
	t.Helper()

	deadline := time.Now().Add(eventWait)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		for _, m := range r.seen {
			if m.Code == code {
				r.mu.Unlock()
				return m
			}
		}
		r.mu.Unlock()
		time.Sleep(time.Millisecond)
	}

	t.Fatalf("the device was never sent %s", code.Name(false))
	return cmd.Message{}
}
