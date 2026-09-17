package osdp

import "github.com/regalium-os/osdp-go/internal/secure"

// Secure channel types.
//
// These are re-exported because the cipher suite is an extension point. A
// deployment with its own cryptographic requirements registers a suite beside
// the mandated one; without these aliases it could not, and the extension point
// would exist in name only.
type (
	// CipherSuite is the set of primitives a secure session needs. Implement it
	// to add a suite alongside the standard one -- never to replace it.
	CipherSuite = secure.CipherSuite

	// Registry holds the negotiable cipher suites. The standard suite is
	// always present and cannot be displaced.
	Registry = secure.Registry

	// BaseKey is the secure channel base key (SCBK) shared with one device.
	BaseKey = secure.BaseKey

	// SessionKeys are the three keys derived for a single session.
	SessionKeys = secure.SessionKeys

	// SecureBlockType identifies a security block and the protection applied
	// to the frame carrying it.
	SecureBlockType = secure.BlockType

	// AES128 is the cipher suite the specification mandates.
	AES128 = secure.AES128
)

// StandardSuiteName is the name of the mandated AES-128 suite. Every
// third-party reader speaks it; interoperability depends on it staying
// reachable.
const StandardSuiteName = secure.StandardSuiteName

// Security block types, SIA OSDP v2.2.2 §7.
const (
	// SCS11 through SCS14 carry the handshake.
	SCS11 = secure.SCS11
	SCS12 = secure.SCS12
	SCS13 = secure.SCS13
	SCS14 = secure.SCS14

	// SCS15 and SCS16 mark an established session, authenticated only.
	SCS15 = secure.SCS15
	SCS16 = secure.SCS16

	// SCS17 and SCS18 mark an established session, encrypted and authenticated.
	SCS17 = secure.SCS17
	SCS18 = secure.SCS18
)

// DefaultBaseKey is SCBK-D, the specification's well-known default.
//
// It is public knowledge and provides no confidentiality. It exists so a panel
// can reach a factory-fresh reader long enough to install a real key; check
// BaseKey.IsDefault and refuse to leave a device running on it.
var DefaultBaseKey = secure.DefaultBaseKey

// NewRegistry returns a cipher suite registry holding the standard suite plus
// any additional suites supplied. Supplying one named StandardSuiteName returns
// ErrSuiteReserved rather than displacing it.
func NewRegistry(extra ...CipherSuite) (*Registry, error) {
	return secure.NewRegistry(extra...)
}

// Secure channel errors, matched with errors.Is.
var (
	// ErrMACMismatch means a message failed authentication. Discard it and
	// tear the session down.
	ErrMACMismatch = secure.ErrMACMismatch

	// ErrBadPadding means a payload was malformed. Treat it exactly as
	// ErrMACMismatch; the two are deliberately not distinguishable.
	ErrBadPadding = secure.ErrBadPadding

	// ErrNotBlockAligned means ciphertext was not a multiple of the AES block.
	ErrNotBlockAligned = secure.ErrNotBlockAligned

	// ErrCryptogramMismatch means the handshake failed: the peer could not
	// prove possession of the base key.
	ErrCryptogramMismatch = secure.ErrCryptogramMismatch

	// ErrSuiteUnavailable means the named cipher suite is not registered.
	ErrSuiteUnavailable = secure.ErrSuiteUnavailable

	// ErrSuiteReserved means an attempt was made to register a suite under the
	// standard suite's name.
	ErrSuiteReserved = secure.ErrSuiteReserved
)
