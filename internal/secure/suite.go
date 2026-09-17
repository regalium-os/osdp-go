// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package secure

// StandardSuiteName is the cipher suite SIA OSDP v2.2.2 mandates.
//
// Every third-party reader in the field -- HID, Gallagher, Salto, generic wall
// readers -- speaks this and only this. Interoperability depends on it being
// present and reachable, not merely compiled in.
const StandardSuiteName = "aes-128"

// CipherSuite is the set of primitives a secure channel session needs.
//
// It exists so that an additional suite can be registered *beside* the standard
// one -- a deployment with its own cryptographic requirements, a future
// revision of the specification. It is an extension point, never a
// replacement: see Registry, which will not let the standard suite be
// displaced, and the conformance test that asserts as much.
//
// Implementations must be safe for concurrent use and must not retain any of
// the key material passed to them.
type CipherSuite interface {
	// Name identifies the suite in capability negotiation and in traces.
	Name() string

	// DeriveSessionKeys produces the session keys from the device's base key
	// and the control panel's random. Only the first six octets of rndA
	// participate, which is what the specification prescribes.
	DeriveSessionKeys(base BaseKey, rndA [8]byte) (SessionKeys, error)

	// ClientCryptogram is the proof a peripheral device returns in
	// osdp_CCRYPT, demonstrating possession of the base key.
	ClientCryptogram(k SessionKeys, rndA, rndB [8]byte) ([BlockSize]byte, error)

	// ServerCryptogram is the matching proof the control panel returns in
	// osdp_SCRYPT.
	ServerCryptogram(k SessionKeys, rndA, rndB [8]byte) ([BlockSize]byte, error)

	// InitialRMAC seeds the reply MAC chain from the server cryptogram.
	InitialRMAC(k SessionKeys, serverCryptogram [BlockSize]byte) ([BlockSize]byte, error)

	// Seal pads and enciphers plaintext. chain is the MAC the initialisation
	// vector is derived from; see the implementation for how.
	Seal(k SessionKeys, chain [BlockSize]byte, plaintext []byte) ([]byte, error)

	// Open deciphers and unpads ciphertext, which must be block aligned.
	Open(k SessionKeys, chain [BlockSize]byte, ciphertext []byte) ([]byte, error)

	// MAC computes the authentication code over data, continuing the chain.
	MAC(k SessionKeys, chain [BlockSize]byte, data []byte) ([BlockSize]byte, error)
}

// Registry holds the cipher suites a session may negotiate.
//
// The standard suite is always present and cannot be removed or overridden.
// That is the whole point of the type: a registry is the natural place for
// "and now replace the default", and here that must not be possible.
type Registry struct {
	standard CipherSuite
	extra    map[string]CipherSuite
}

// NewRegistry returns a registry holding the standard suite, plus any
// additional suites supplied.
//
// Registering a suite whose name collides with the standard one returns
// ErrSuiteReserved rather than quietly winning.
func NewRegistry(extra ...CipherSuite) (*Registry, error) {
	r := &Registry{standard: AES128{}, extra: make(map[string]CipherSuite, len(extra))}
	for _, s := range extra {
		if err := r.register(s); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *Registry) register(s CipherSuite) error {
	if s.Name() == StandardSuiteName {
		return ErrSuiteReserved
	}
	r.extra[s.Name()] = s
	return nil
}

// Standard returns the specification-mandated suite. It is never nil.
func (r *Registry) Standard() CipherSuite { return r.standard }

// Lookup returns the suite registered under name.
func (r *Registry) Lookup(name string) (CipherSuite, error) {
	if name == StandardSuiteName {
		return r.standard, nil
	}
	if s, ok := r.extra[name]; ok {
		return s, nil
	}
	return nil, ErrSuiteUnavailable
}

// Names lists every registered suite, the standard one first. A peer that
// advertises none of the others still negotiates successfully.
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.extra)+1)
	out = append(out, StandardSuiteName)
	for name := range r.extra {
		out = append(out, name)
	}
	return out
}
