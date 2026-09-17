// Package secure implements OSDP Secure Channel: session establishment, key
// derivation, message encryption, and the message authentication code that
// covers every secured frame.
//
// # Layer contract
//
// secure sits above frame and beside cmd. It owns the Secure Channel Session
// (SCS) state machine -- CHLNG/CCRYPT, SCRYPT/RMAC_I, and the transition into
// the encrypted SCS_15/SCS_16 and plaintext-with-MAC SCS_17/SCS_18 modes -- plus
// the sequence and MAC chaining that make replay detectable.
//
// # Cipher suites
//
// The AES-128 suite mandated by SIA OSDP v2.2.2 is REQUIRED and is always
// registered. Every third-party reader in the field -- HID, Gallagher, Salto,
// generic wall readers -- speaks it and only it, so interoperability depends on
// it being present and reachable, not merely compiled in.
//
// CipherSuite exists so an additional suite can be registered alongside the
// standard one. It is an extension point, never a replacement: a build that
// cannot negotiate the standard suite is a broken build, and the suite registry
// test asserts exactly that. Any custom suite must be opt-in per session and
// must fall back rather than fail when a peer does not advertise it.
//
// secure performs no I/O and holds no keys at package scope; key material is
// owned by the caller and passed in explicitly so it can be zeroed on session
// teardown.
//
// # Allowed imports
//
//	stdlib (incl. crypto/aes, crypto/subtle), frame, telemetry
//
// # Tracing
//
// Spans: osdp.secure.handshake, osdp.secure.seal, osdp.secure.open. Span
// attributes carry the SCS state and suite identifier. Key material, session
// keys and the RNG challenge are NEVER recorded as attributes.
package secure
