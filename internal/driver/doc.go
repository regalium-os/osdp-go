// Package driver contains the concrete transports that satisfy the transport
// port: RS-485 serial lines, TCP sockets, and the in-memory pipe used by tests.
//
// # Layer contract
//
// driver is an edge layer. Along with provider it is one of only two packages
// permitted to perform I/O, hold an operating-system handle, or consult a real
// clock. It owns port configuration, baud rates, turnaround timing, read
// deadlines, reconnection and backoff.
//
// driver depends on the core; the core never depends on driver. Nothing here is
// importable from frame, cmd, secure, bus or transport, and the architecture
// test enforces it.
//
// # Allowed imports
//
//	stdlib, transport, telemetry, third-party serial libraries
//
// # Tracing
//
// Spans osdp.driver.read and osdp.driver.write are the leaves of a transaction
// trace and carry byte counts and the port identity. They are where real latency
// becomes visible, which is the reason the span boundary sits exactly here.
package driver
