package arch

import (
	"path/filepath"
	"slices"
	"testing"
)

// ioPackages are standard-library packages whose presence in a pure layer means
// that layer has grown a side effect.
//
// Logging is included alongside the obvious ones: a protocol codec that logs has
// made a policy decision on its caller's behalf, and the caller is the one who
// knows whether a malformed frame is routine noise or an incident. The core
// returns errors and spans; it does not narrate.
var ioPackages = []string{
	"net",
	"net/http",
	"os",
	"os/exec",
	"os/signal",
	"syscall",
	"bufio",
	"log",
	"log/slog",
}

// TestPureLayersDoNoIO asserts that the core computes rather than acts.
func TestPureLayersDoNoIO(t *testing.T) {
	root := repoRoot(t)

	for name, spec := range layers {
		if !spec.pure {
			continue
		}
		t.Run(name, func(t *testing.T) {
			for _, file := range goFiles(t, filepath.Join(root, name)) {
				for _, imp := range imports(t, file) {
					if slices.Contains(ioPackages, imp) {
						t.Errorf(
							"%s imports %q\n  %s is a pure layer: it computes over its\n"+
								"  arguments and returns a decision. Move the effect into\n"+
								"  driver or provider, or inject it through an interface.",
							relPath(root, file), imp, name,
						)
					}
				}
			}
		})
	}
}

// TestStandardCipherSuiteIsReachable guards the interoperability requirement.
//
// Secure Channel's AES-128 suite is what every third-party reader in the field
// speaks. A CipherSuite interface exists so another suite can be added beside
// it, and this test exists so that "beside" never quietly becomes "instead of".
func TestStandardCipherSuiteIsReachable(t *testing.T) {
	root := repoRoot(t)
	if len(goFiles(t, filepath.Join(root, "secure"))) <= 1 {
		t.Skip("secure not implemented yet; enabled in Phase 2")
	}
	t.Error("Phase 2 must replace this with an assertion that the standard " +
		"AES-128 suite is registered and selectable by default")
}
