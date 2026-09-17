package bus_test

import (
	"context"
	"testing"
	"time"

	"github.com/regalium-os/osdp-go/internal/bus"
	"github.com/regalium-os/osdp-go/internal/cmd"
	"github.com/regalium-os/osdp-go/internal/frame"
	"github.com/regalium-os/osdp-go/internal/transport"
)

func newBus(addrs ...frame.Address) *bus.Bus {
	line := transport.Line{Name: "test", Baud: 9600, ReplyTimeout: 200 * time.Millisecond}
	return bus.New(line, addrs, frame.SchemeCRC16)
}

// reply builds a sealed reply frame from a device.
func reply(addr frame.Address, seq uint8, code cmd.Code, data []byte) frame.Frame {
	f := frame.Frame{
		IsReply: true,
		Address: addr,
		Control: frame.NewControl(seq, frame.SchemeCRC16, false),
		Code:    byte(code),
		Data:    data,
	}
	f.Seal()
	return f
}

// TestSequenceRotation: the sequence number cycles 1, 2, 3 and back to 1.
// Zero is reserved for resynchronisation and must never appear in the rotation.
func TestSequenceRotation(t *testing.T) {
	ctx := context.Background()
	b := newBus(0x00)

	var got []uint8
	for range 5 {
		step, ok := b.Next(ctx)
		if !ok {
			t.Fatal("Next returned no step")
		}
		got = append(got, step.Frame.Control.Sequence())
	}

	want := []uint8{1, 2, 3, 1, 2}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sequence rotation = %v, want %v", got, want)
		}
	}
}

// TestPollCycleVisitsEveryDevice: a device that never answers must not stall
// the ones behind it.
func TestPollCycleVisitsEveryDevice(t *testing.T) {
	ctx := context.Background()
	b := newBus(0x00, 0x01, 0x02)

	seen := map[frame.Address]int{}
	for range 6 {
		step, _ := b.Next(ctx)
		seen[step.Device.Address]++
	}
	for _, a := range []frame.Address{0x00, 0x01, 0x02} {
		if seen[a] != 2 {
			t.Errorf("address 0x%02X polled %d times in two cycles, want 2", byte(a), seen[a])
		}
	}
}
