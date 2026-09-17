package secure_test

import (
	"errors"
	"testing"

	"github.com/regalium-os/osdp-go/internal/secure"
)

// The tests here cover what the MAC chain is for: rejecting a frame that was
// authentic once, or authentic in the other direction, or altered in flight.
// They share the handshake helper in session_test.go.

// TestReplayIsRejected is the property the MAC chain exists to provide.
//
// A frame captured from the line and injected again must fail, even though it
// was authentic when it was sent. Without chaining, a recorded "unlock" is a
// working key for anyone with a pair of alligator clips.
func TestReplayIsRejected(t *testing.T) {
	cp, pd := handshake(t, secure.DefaultBaseKey)

	// The bus alternates strictly: a command, then the reply it prompted. The
	// chain only advances through that alternation, so the replay has to be
	// attempted after a complete exchange -- which is exactly when an attacker
	// who recorded the line would attempt it.
	cmd := []byte{0x69, 0x01}
	cmdTag, err := cp.Authenticate(cmd, true)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if vErr := pd.Verify(cmd, cmdTag[:], true); vErr != nil {
		t.Fatalf("first delivery rejected: %v", vErr)
	}

	reply := []byte{0x40}
	replyTag, err := pd.Authenticate(reply, false)
	if err != nil {
		t.Fatalf("Authenticate reply: %v", err)
	}
	if err := cp.Verify(reply, replyTag[:], false); err != nil {
		t.Fatalf("reply rejected: %v", err)
	}

	// The recorded command, injected again.
	if err := pd.Verify(cmd, cmdTag[:], true); !errors.Is(err, secure.ErrMACMismatch) {
		t.Errorf("a replayed frame was accepted: %v", err)
	}
}

// TestFailedVerifyDoesNotAdvanceTheChain: a rejected frame must not desynchronise
// the session, or an attacker could knock a bus offline by injecting noise.
func TestFailedVerifyDoesNotAdvanceTheChain(t *testing.T) {
	cp, pd := handshake(t, secure.DefaultBaseKey)

	msg := []byte{0x60}
	tag, _ := cp.Authenticate(msg, true)

	forged := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	if err := pd.Verify(msg, forged, true); !errors.Is(err, secure.ErrMACMismatch) {
		t.Fatalf("a forged tag was accepted: %v", err)
	}

	// The genuine frame must still verify afterwards.
	if err := pd.Verify(msg, tag[:], true); err != nil {
		t.Errorf("a rejected forgery desynchronised the chain: %v", err)
	}
}

// TestTamperedPayloadIsRejected closes the loop left open by
// TestEncryptionAloneDoesNotAuthenticate: CBC will happily decrypt altered
// ciphertext, and this is what stops it mattering.
func TestTamperedPayloadIsRejected(t *testing.T) {
	cp, pd := handshake(t, secure.DefaultBaseKey)

	sealed, err := cp.Seal([]byte("unlock the east door"), true)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	tag, err := cp.Authenticate(sealed, true)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}

	sealed[0] ^= 0x01 // a single bit, on the wire
	if err := pd.Verify(sealed, tag[:], true); !errors.Is(err, secure.ErrMACMismatch) {
		t.Errorf("a tampered payload was accepted: %v", err)
	}
}

// TestDirectionsChainSeparately: a command's MAC must not verify as a reply.
func TestDirectionsChainSeparately(t *testing.T) {
	cp, pd := handshake(t, secure.DefaultBaseKey)

	msg := []byte{0x60}
	tag, _ := cp.Authenticate(msg, true)

	if err := pd.Verify(msg, tag[:], false); !errors.Is(err, secure.ErrMACMismatch) {
		t.Error("a command authenticated as a reply; the two chains are not separate")
	}
}

func TestTeardownClearsTheSession(t *testing.T) {
	cp, _ := handshake(t, secure.DefaultBaseKey)
	cp.Teardown()

	if cp.Established() || cp.State() != secure.StateIdle {
		t.Errorf("after Teardown: established=%v state=%v", cp.Established(), cp.State())
	}
	if _, err := cp.Seal([]byte{0x60}, true); !errors.Is(err, secure.ErrNotEstablished) {
		t.Errorf("Seal after Teardown = %v, want ErrNotEstablished", err)
	}
}

func TestDefaultKeySessionIsFlagged(t *testing.T) {
	cp, _ := handshake(t, secure.DefaultBaseKey)
	if !cp.UsingDefaultKey() {
		t.Error("a session on SCBK-D does not report itself as such")
	}

	var installed secure.BaseKey
	copy(installed[:], secure.DefaultBaseKey[:])
	installed[0] ^= 0x5A
	installedSession, _ := handshake(t, installed)
	if installedSession.UsingDefaultKey() {
		t.Error("a session on an installed key reports the default")
	}
}
