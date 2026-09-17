// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package secure_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/regalium-os/osdp-go/internal/secure"
)

// custom is a stand-in for a second cipher suite someone might add later. It
// does nothing useful; what matters is how the registry treats it.
type custom struct{ name string }

func (c custom) Name() string { return c.name }
func (custom) DeriveSessionKeys(secure.BaseKey, [8]byte) (secure.SessionKeys, error) {
	return secure.SessionKeys{}, nil
}
func (custom) ClientCryptogram(secure.SessionKeys, [8]byte, [8]byte) ([16]byte, error) {
	return [16]byte{}, nil
}
func (custom) ServerCryptogram(secure.SessionKeys, [8]byte, [8]byte) ([16]byte, error) {
	return [16]byte{}, nil
}
func (custom) InitialRMAC(secure.SessionKeys, [16]byte) ([16]byte, error) {
	return [16]byte{}, nil
}
func (custom) Seal(secure.SessionKeys, [16]byte, []byte) ([]byte, error)  { return nil, nil }
func (custom) Open(secure.SessionKeys, [16]byte, []byte) ([]byte, error)  { return nil, nil }
func (custom) MAC(secure.SessionKeys, [16]byte, []byte) ([16]byte, error) { return [16]byte{}, nil }

// TestStandardSuiteIsAlwaysReachable is the interoperability guarantee.
//
// Every third-party reader in the field speaks the specification's AES-128
// suite and nothing else. A build in which it is missing, or has been replaced,
// cannot talk to an HID, Gallagher or Salto reader -- and would fail in the
// field rather than in CI. So it is asserted here.
func TestStandardSuiteIsAlwaysReachable(t *testing.T) {
	t.Run("present in an empty registry", func(t *testing.T) {
		r, err := secure.NewRegistry()
		if err != nil {
			t.Fatalf("NewRegistry: %v", err)
		}
		if r.Standard() == nil {
			t.Fatal("the registry has no standard suite")
		}
		if got := r.Standard().Name(); got != secure.StandardSuiteName {
			t.Errorf("Standard().Name() = %q, want %q", got, secure.StandardSuiteName)
		}
	})

	t.Run("still reachable alongside another suite", func(t *testing.T) {
		r, err := secure.NewRegistry(custom{name: "experimental"})
		if err != nil {
			t.Fatalf("NewRegistry: %v", err)
		}
		s, err := r.Lookup(secure.StandardSuiteName)
		if err != nil {
			t.Fatalf("the standard suite is not reachable once another is registered: %v", err)
		}
		if s.Name() != secure.StandardSuiteName {
			t.Errorf("Lookup returned %q", s.Name())
		}
	})

	t.Run("listed first", func(t *testing.T) {
		r, _ := secure.NewRegistry(custom{name: "experimental"})
		names := r.Names()
		if len(names) == 0 || names[0] != secure.StandardSuiteName {
			t.Errorf("Names() = %v, want the standard suite first", names)
		}
		if !slices.Contains(names, "experimental") {
			t.Error("an additional suite is not advertised")
		}
	})
}

// TestStandardSuiteCannotBeDisplaced is the other half: an extension point that
// permits replacement is not an extension point, it is a footgun.
func TestStandardSuiteCannotBeDisplaced(t *testing.T) {
	_, err := secure.NewRegistry(custom{name: secure.StandardSuiteName})
	if !errors.Is(err, secure.ErrSuiteReserved) {
		t.Fatalf("registering a suite named %q = %v, want ErrSuiteReserved",
			secure.StandardSuiteName, err)
	}

	// And the real suite still answers afterwards.
	r, err := secure.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if _, ok := r.Standard().(secure.AES128); !ok {
		t.Errorf("Standard() is %T, want secure.AES128", r.Standard())
	}
}

func TestLookupUnknownSuite(t *testing.T) {
	r, _ := secure.NewRegistry()
	if _, err := r.Lookup("nonexistent"); !errors.Is(err, secure.ErrSuiteUnavailable) {
		t.Errorf("Lookup of an unregistered suite = %v, want ErrSuiteUnavailable", err)
	}
}

// TestDefaultBaseKeyIsRecognised: SCBK-D is public knowledge, and a deployment
// must be able to detect a device still running on it.
func TestDefaultBaseKeyIsRecognised(t *testing.T) {
	if !secure.DefaultBaseKey.IsDefault() {
		t.Error("DefaultBaseKey does not recognise itself")
	}
	var installed secure.BaseKey
	copy(installed[:], secure.DefaultBaseKey[:])
	installed[0] ^= 0x01
	if installed.IsDefault() {
		t.Error("an install-specific key is reported as the default")
	}
}
