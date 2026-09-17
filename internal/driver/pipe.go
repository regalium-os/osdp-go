package driver

import (
	"net"

	"github.com/regalium-os/osdp-go/internal/transport"
)

// Pipe returns two Ports connected to each other, in memory.
//
// It is how a panel and a device are exercised together without hardware: the
// full stack runs end to end, framing, secure channel and poll cycle included,
// with nothing mocked. What it does not reproduce is the half-duplex turnaround
// of a real RS-485 line, where a panel reading too early hears its own
// transmission -- that failure mode needs the real thing, or a driver that
// simulates it deliberately.
func Pipe() (panel, device transport.Port) {
	a, b := net.Pipe()
	return &conn{c: a, name: "pipe:panel"}, &conn{c: b, name: "pipe:device"}
}
