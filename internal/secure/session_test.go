package secure_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/regalium-os/osdp-go/internal/secure"
)

// handshake runs a full exchange between a panel and a device sharing base, and
// returns both sessions established.
func handshake(t *testing.T, base secure.BaseKey) (cp, pd *secure.Session) {
	t.Helper()

	rndA := [8]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	rndB := [8]byte{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18}
	uid := [8]byte{0x00, 0x06, 0x8E, 0x01, 0x0A, 0x0B, 0x0C, 0x0D}

	cp = secure.NewSession(aes128, secure.RoleCP, base)
	pd = secure.NewSession(aes128, secure.RolePD, base)

	chlng, err := cp.BeginChallenge(rndA)
	if err != nil {
		t.Fatalf("BeginChallenge: %v", err)
	}
	ccrypt, err := pd.AnswerChallenge(chlng, rndB, uid)
	if err != nil {
		t.Fatalf("AnswerChallenge: %v", err)
	}
	scrypt, err := cp.AnswerCryptogram(ccrypt)
	if err != nil {
		t.Fatalf("AnswerCryptogram: %v", err)
	}
	rmacI, err := pd.AnswerServerCryptogram(scrypt)
	if err != nil {
		t.Fatalf("AnswerServerCryptogram: %v", err)
	}
	if err := cp.CompleteHandshake(rmacI); err != nil {
		t.Fatalf("CompleteHandshake: %v", err)
	}
	return cp, pd
}

func TestFixtureHandshakeEstablishesBothSides(t *testing.T) {
	cp, pd := handshake(t, secure.DefaultBaseKey)

	if !cp.Established() || !pd.Established() {
		t.Fatalf("handshake left cp=%v pd=%v", cp.State(), pd.State())
	}
	// Both ends must agree on the chain, or the first MAC fails.
	tag, err := cp.Authenticate([]byte{0x60}, true)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if err := pd.Verify([]byte{0x60}, tag[:], true); err != nil {
		t.Errorf("the device rejected the panel's first authenticated command: %v", err)
	}
}

// TestHandshakeRejectsTheWrongKey: a device with a different base key must fail
// the handshake, not merely produce unreadable traffic later.
func TestHandshakeRejectsTheWrongKey(t *testing.T) {
	var other secure.BaseKey
	copy(other[:], secure.DefaultBaseKey[:])
	other[0] ^= 0xFF

	rndA := [8]byte{1, 2, 3, 4, 5, 6, 7, 8}
	rndB := [8]byte{9, 10, 11, 12, 13, 14, 15, 16}

	cp := secure.NewSession(aes128, secure.RoleCP, secure.DefaultBaseKey)
	pd := secure.NewSession(aes128, secure.RolePD, other)

	chlng, _ := cp.BeginChallenge(rndA)
	ccrypt, _ := pd.AnswerChallenge(chlng, rndB, [8]byte{})

	if _, err := cp.AnswerCryptogram(ccrypt); !errors.Is(err, secure.ErrCryptogramMismatch) {
		t.Fatalf("AnswerCryptogram with a mismatched key = %v, want ErrCryptogramMismatch", err)
	}
	if cp.State() != secure.StateFailed {
		t.Errorf("state after a failed cryptogram = %v, want failed", cp.State())
	}
	if cp.Established() {
		t.Error("a session that failed authentication reports itself established")
	}
}

func TestRoleEnforcement(t *testing.T) {
	pd := secure.NewSession(aes128, secure.RolePD, secure.DefaultBaseKey)
	if _, err := pd.BeginChallenge([8]byte{}); !errors.Is(err, secure.ErrWrongRole) {
		t.Errorf("a device issuing a challenge = %v, want ErrWrongRole", err)
	}

	cp := secure.NewSession(aes128, secure.RoleCP, secure.DefaultBaseKey)
	if _, err := cp.AnswerChallenge(make([]byte, 8), [8]byte{}, [8]byte{}); !errors.Is(err, secure.ErrWrongRole) {
		t.Errorf("a panel answering a challenge = %v, want ErrWrongRole", err)
	}
}

func TestOperationsRequireAnEstablishedSession(t *testing.T) {
	s := secure.NewSession(aes128, secure.RoleCP, secure.DefaultBaseKey)

	if _, err := s.Seal([]byte{0x60}, true); !errors.Is(err, secure.ErrNotEstablished) {
		t.Errorf("Seal before the handshake = %v, want ErrNotEstablished", err)
	}
	if _, err := s.Authenticate([]byte{0x60}, true); !errors.Is(err, secure.ErrNotEstablished) {
		t.Errorf("Authenticate before the handshake = %v, want ErrNotEstablished", err)
	}
}

// TestSealOpenAcrossTheBus is the end-to-end property: what the panel seals,
// the device opens, and the MAC binds it.
func TestSealOpenAcrossTheBus(t *testing.T) {
	cp, pd := handshake(t, secure.DefaultBaseKey)

	plaintext := []byte{0x69, 0x00, 0x00, 0x01}
	sealed, err := cp.Seal(plaintext, true)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if bytes.Contains(sealed, plaintext) {
		t.Error("the ciphertext contains the plaintext verbatim")
	}

	tag, err := cp.Authenticate(sealed, true)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if vErr := pd.Verify(sealed, tag[:], true); vErr != nil {
		t.Fatalf("Verify: %v", vErr)
	}

	opened, err := pd.Open(sealed, true)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !bytes.Equal(opened, plaintext) {
		t.Errorf("opened %x, want %x", opened, plaintext)
	}
}
