// Package bus implements the OSDP poll cycle: the sequencing discipline a
// control panel runs against the peripheral devices sharing a multidrop line.
//
// # Layer contract
//
// bus owns time-free protocol logic -- which device is addressed next, what the
// sequence number should be, when a reply counts as timed out, when a device is
// considered offline, and when a secure session must be torn down and rebuilt.
// It is a state machine over events, not a loop over a serial port: it decides
// what should happen and returns that decision. The driver decides when.
//
// This split is what makes the poll cycle testable. A full online/offline/
// resync scenario runs as a table test with a fake clock and no hardware.
//
// bus does not sleep, does not read a real clock directly, and performs no I/O.
// Time enters through an injected clock.
//
// # Allowed imports
//
//	stdlib, frame, cmd, secure, transport, telemetry, protobuf/generated/go
//
// # Tracing
//
// Span osdp.bus.cycle wraps one pass over the address list; osdp.bus.transaction
// wraps a single command/reply exchange with a device and is the natural parent
// for the frame, cmd and secure spans beneath it.
package bus
