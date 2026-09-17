package secure

import "errors"

// Secure channel errors, matched with errors.Is.
var (
	// ErrNotBlockAligned reports ciphertext whose length is not a multiple of
	// the AES block size.
	ErrNotBlockAligned = errors.New("osdp/secure: ciphertext is not a multiple of the block size")

	// ErrBadPadding reports a plaintext that does not end in the 0x80 marker
	// the specification requires.
	//
	// It is deliberately indistinguishable, to a caller, from a MAC failure:
	// both mean the frame is rejected. Reporting which one failed, and how
	// quickly, is how padding oracles are built.
	ErrBadPadding = errors.New("osdp/secure: malformed padding")

	// ErrMACMismatch reports a message whose authentication code does not
	// match. The message must be discarded and the session torn down.
	ErrMACMismatch = errors.New("osdp/secure: message authentication failed")

	// ErrCryptogramMismatch reports a failed handshake: the peer could not
	// prove possession of the base key. The session must not proceed.
	ErrCryptogramMismatch = errors.New("osdp/secure: cryptogram verification failed")

	// ErrSuiteUnavailable reports a cipher suite that is not registered.
	ErrSuiteUnavailable = errors.New("osdp/secure: cipher suite not registered")

	// ErrSuiteReserved reports an attempt to register a suite under the name
	// of the specification-mandated one. The standard suite is what every
	// third-party reader speaks; it cannot be displaced.
	ErrSuiteReserved = errors.New("osdp/secure: the standard suite name is reserved")
)
