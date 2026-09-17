// Package frame implements the OSDP wire frame: the octets between the start-of-
// message marker and the trailing error check, exactly as they appear on the bus.
//
// # Layer contract
//
// frame is the innermost pure layer. It owns framing and nothing else:
// start-of-message detection, address and control-byte parsing, the declared
// length, the security block envelope when present, and checksum/CRC validation
// over the exact bytes received. It converts []byte to a Frame and back with no
// loss: decoding a frame and re-encoding it MUST reproduce the input byte for
// byte, including a wrong error check, so malformed traffic can be inspected
// rather than silently normalised.
//
// frame does not interpret the command or reply code, does not decrypt, does not
// know a device exists, and never touches a transport. It has no I/O and no
// ambient state; every exported function is safe for concurrent use and depends
// only on its arguments.
//
// # Allowed imports
//
//	stdlib, github.com/regalium-os/osdp-go/telemetry
//
// Importing cmd, secure, bus, transport, driver, or provider from here is a
// layering violation and is enforced by the architecture test in
// arch_test.go at the repository root.
//
// # Tracing
//
// Decode and Encode open spans named osdp.frame.decode and osdp.frame.encode.
// The tracer arrives through a context, never through a package-level global, so
// a caller that supplies no tracer pays only a no-op.
package frame
