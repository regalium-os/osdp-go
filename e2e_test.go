package osdp_test

import (
	"context"
	"testing"
	"time"

	"github.com/regalium-os/osdp-go"
)

// TestEndToEndCardRead drives the whole stack through a real port: the bus
// composes a command, the frame codec puts it on the wire, a device answers,
// and the reply comes back out as an event.
//
// Everything here goes through the public API, so it doubles as the worked
// example of what using this library looks like. Nothing is mocked; the only
// thing standing in for hardware is the in-memory port.
func TestEndToEndCardRead(t *testing.T) {
	ctx := context.Background()
	panel, device := osdp.Pipe()
	defer panel.Close()
	defer device.Close()

	line := osdp.Line{Name: "test", Baud: 9600, ReplyTimeout: time.Second}
	bus := osdp.NewBus(line, []osdp.Address{0x00}, osdp.SchemeCRC16)
	pd := bus.Devices()[0]

	// The device: read a frame, answer it. A real reader is this plus hardware.
	answers := make(chan error, 1)
	go func() { answers <- respondWithCardRead(ctx, device) }()

	// The panel asks the first thing it asks an unidentified device.
	step, ok := bus.Next(ctx)
	if !ok {
		t.Fatal("the bus produced no step")
	}
	wire, err := step.Frame.Append(ctx, nil)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if _, err := panel.Write(wire); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// And reads what came back.
	if err := panel.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	buf := make([]byte, 256)
	n, err := panel.Read(buf)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if err := <-answers; err != nil {
		t.Fatalf("device: %v", err)
	}

	reply, err := osdp.Decode(ctx, buf[:n])
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	event, err := bus.Reply(ctx, pd, reply, time.Now())
	if err != nil {
		t.Fatalf("Reply: %v", err)
	}

	if event.Kind != osdp.EventCardRead {
		t.Fatalf("event = %v, want card_read", event.Kind)
	}
	if event.Card.BitCount != 26 {
		t.Errorf("bit count = %d, want 26", event.Card.BitCount)
	}
	if event.Device.Address != 0x00 {
		t.Errorf("event came from address 0x%02X, want 0x00", byte(event.Device.Address))
	}
}

// respondWithCardRead plays the peripheral device: decode whatever the panel
// sent, answer with a credential at the sequence number it used.
func respondWithCardRead(ctx context.Context, port osdp.Port) error {
	if err := port.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return err
	}
	buf := make([]byte, 256)
	n, err := port.Read(buf)
	if err != nil {
		return err
	}

	command, err := osdp.Decode(ctx, buf[:n])
	if err != nil {
		return err
	}

	reply := osdp.Frame{
		IsReply: true,
		Address: command.Address,
		Control: osdp.NewControl(command.Control.Sequence(), osdp.SchemeCRC16, false),
		Code:    0x50, // osdp_RAW
		// Reader 0, format 1, 26 bits, then the credential.
		Data: []byte{0x00, 0x01, 0x1A, 0x00, 0xAB, 0xCD, 0xEF, 0x80},
	}
	reply.Seal()

	wire, err := reply.Append(ctx, nil)
	if err != nil {
		return err
	}
	_, err = port.Write(wire)
	return err
}
