// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package secure_test

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/regalium-os/osdp-go/internal/secure"
)

// aes128 is the suite under test. It is stateless, so one value serves every
// test in the package.
var aes128 secure.AES128

// loadKAT reads the known-answer vectors. Each line is 'name = hex  # comment'.
func loadKAT(t *testing.T) map[string][]byte {
	t.Helper()

	f, err := os.Open("testdata/aes128.kat")
	if err != nil {
		t.Fatalf("opening vectors: %v", err)
	}
	defer f.Close()

	out := map[string][]byte{}
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if i := strings.Index(line, "#"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		name, value, ok := strings.Cut(line, "=")
		if !ok {
			t.Fatalf("malformed vector line: %q", line)
		}
		b, err := hex.DecodeString(strings.TrimSpace(value))
		if err != nil {
			t.Fatalf("vector %s: %v", name, err)
		}
		out[strings.TrimSpace(name)] = b
	}
	if len(out) == 0 {
		t.Fatal("no vectors loaded; this test is checking nothing")
	}
	return out
}

func key16(t *testing.T, b []byte) [16]byte {
	t.Helper()
	if len(b) != 16 {
		t.Fatalf("expected 16 octets, got %d", len(b))
	}
	return [16]byte(b)
}

func rnd8(t *testing.T, b []byte) [8]byte {
	t.Helper()
	if len(b) != 8 {
		t.Fatalf("expected 8 octets, got %d", len(b))
	}
	return [8]byte(b)
}

// sessionFromKAT rebuilds the session keys the vectors describe.
func sessionFromKAT(t *testing.T, kat map[string][]byte) secure.SessionKeys {
	t.Helper()
	keys, err := aes128.DeriveSessionKeys(
		secure.BaseKey(key16(t, kat["scbk"])), rnd8(t, kat["rnd_a"]))
	if err != nil {
		t.Fatalf("DeriveSessionKeys: %v", err)
	}
	return keys
}

// TestFixtureSessionKeyDerivation checks the three derived keys against vectors
// computed by an independent AES implementation.
//
// Getting the derivation wrong is not a loud failure: two peers that derive
// differently simply fail every MAC, which looks like a wiring fault. The
// vectors turn it into an immediate, specific failure.
func TestFixtureSessionKeyDerivation(t *testing.T) {
	kat := loadKAT(t)
	keys := sessionFromKAT(t, kat)

	for _, tt := range []struct {
		name string
		got  [16]byte
	}{
		{"s_enc", keys.Enc},
		{"s_mac1", keys.MAC1},
		{"s_mac2", keys.MAC2},
	} {
		if !bytes.Equal(tt.got[:], kat[tt.name]) {
			t.Errorf("%s = %x, want %x", tt.name, tt.got, kat[tt.name])
		}
	}
}

// TestFixtureCryptograms checks both proofs, and that they differ.
func TestFixtureCryptograms(t *testing.T) {
	kat := loadKAT(t)
	keys := sessionFromKAT(t, kat)
	rndA, rndB := rnd8(t, kat["rnd_a"]), rnd8(t, kat["rnd_b"])

	client, err := aes128.ClientCryptogram(keys, rndA, rndB)
	if err != nil {
		t.Fatalf("ClientCryptogram: %v", err)
	}
	if !bytes.Equal(client[:], kat["client_cryptogram"]) {
		t.Errorf("client cryptogram = %x, want %x", client, kat["client_cryptogram"])
	}

	server, err := aes128.ServerCryptogram(keys, rndA, rndB)
	if err != nil {
		t.Fatalf("ServerCryptogram: %v", err)
	}
	if !bytes.Equal(server[:], kat["server_cryptogram"]) {
		t.Errorf("server cryptogram = %x, want %x", server, kat["server_cryptogram"])
	}

	// The randoms are concatenated in opposite orders precisely so neither
	// side can replay the other's proof. If these ever match, that property
	// is gone and the handshake authenticates nothing.
	if bytes.Equal(client[:], server[:]) {
		t.Error("client and server cryptograms are identical; each side could replay the other")
	}
}

func TestFixtureInitialRMAC(t *testing.T) {
	kat := loadKAT(t)
	keys := sessionFromKAT(t, kat)

	got, err := aes128.InitialRMAC(keys, key16(t, kat["server_cryptogram"]))
	if err != nil {
		t.Fatalf("InitialRMAC: %v", err)
	}
	if !bytes.Equal(got[:], kat["rmac_i"]) {
		t.Errorf("rmac_i = %x, want %x", got, kat["rmac_i"])
	}
}
