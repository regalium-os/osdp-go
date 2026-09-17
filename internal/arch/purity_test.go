// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

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
			for _, file := range goFiles(t, filepath.Join(root, spec.dir)) {
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
