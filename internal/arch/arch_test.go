// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

// Package arch holds the repository's conformance tests.
//
// The hexagonal layering here is a constraint, not a convention, so it is
// asserted mechanically. A layering mistake fails CI at the point it is
// introduced rather than being discovered a year later, when the core has
// quietly grown a dependency on a serial port.
//
// The tests are split across files so that each stays within the line limit
// they themselves enforce:
//
//	arch_test.go     the layer map and permitted import directions
//	purity_test.go   the pure core performs no I/O
//	size_test.go     no source file outgrows its reader
//	support_test.go  shared file-walking helpers
package arch

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const modulePath = "github.com/regalium-os/osdp-go"

// layer describes one package's permitted dependencies.
type layer struct {
	// dir is the package path relative to the repository root. The protocol
	// layers live under internal/ so that the public API is exactly what the
	// root osdp package chooses to re-export, and nothing more.
	dir string
	// allowed lists the sibling layers this layer may import. Layers absent
	// from the list are forbidden; telemetry is implicitly allowed everywhere.
	allowed []string
	// pure marks a layer that must not perform I/O. Pure layers may not reach
	// for the operating system, the network, or the wall clock.
	pure bool
}

// layers is the architecture, stated once.
//
// The two edge layers -- driver and provider -- are the only packages permitted
// to touch the outside world. Everything above them is a function of its inputs.
var layers = map[string]layer{
	"telemetry": {dir: "telemetry", allowed: nil, pure: true},
	"frame":     {dir: "internal/frame", allowed: nil, pure: true},
	"transport": {dir: "internal/transport", allowed: nil, pure: true},
	"secure":    {dir: "internal/secure", allowed: []string{"frame"}, pure: true},
	"cmd":       {dir: "internal/cmd", allowed: []string{"frame", "protobuf"}, pure: true},
	"bus": {
		dir:     "internal/bus",
		allowed: []string{"frame", "cmd", "secure", "transport", "protobuf"},
		pure:    true,
	},
	"driver": {dir: "internal/driver", allowed: []string{"transport"}, pure: false},

	// panel is the runtime: the one component that owns a port, consults a
	// clock and turns the bus's decisions into traffic. It is an edge layer
	// for exactly that reason, and it depends on the core rather than the
	// other way round -- bus has no idea it exists.
	"panel": {
		dir:     "internal/panel",
		allowed: []string{"frame", "bus", "transport"},
		pure:    false,
	},
	"provider": {
		dir:     "internal/provider",
		allowed: []string{"frame", "cmd", "secure", "bus", "transport", "driver", "protobuf"},
		pure:    false,
	},
}

// layerOf maps a module-relative import path to the layer that owns it, or ""
// for a path outside the layered core.
func layerOf(localPath string) string {
	for name, spec := range layers {
		if localPath == spec.dir || strings.HasPrefix(localPath, spec.dir+"/") {
			return name
		}
	}
	// protobuf is a sibling module, referenced by name in the allow lists.
	if strings.HasPrefix(localPath, "protobuf/") {
		return "protobuf"
	}
	return ""
}

// TestEveryLayerIsPresent is the guard against a silently blind test run.
//
// The other tests iterate over files; if a layer yields no files they pass
// trivially and the architecture goes unchecked. Asserting that every declared
// layer contributes at least one source file turns that failure mode into a
// visible one.
func TestEveryLayerIsPresent(t *testing.T) {
	root := repoRoot(t)

	for name, spec := range layers {
		dir := filepath.Join(root, spec.dir)
		if len(goFiles(t, dir)) == 0 {
			t.Errorf("layer %q contributed no Go files (looked in %s)\n"+
				"  either the layer was deleted, or this test cannot see the\n"+
				"  source tree and is checking nothing at all", name, dir)
		}
	}
}

// TestLayerDependencies asserts that imports only ever point inward.
func TestLayerDependencies(t *testing.T) {
	root := repoRoot(t)

	for name, spec := range layers {
		t.Run(name, func(t *testing.T) {
			for _, file := range goFiles(t, filepath.Join(root, spec.dir)) {
				for _, imp := range imports(t, file) {
					localPath, isLocal := strings.CutPrefix(imp, modulePath+"/")
					if !isLocal {
						continue
					}
					dep := layerOf(localPath)
					if dep == "" {
						continue
					}

					// telemetry is the one package every layer may import: it
					// is definitions and no-op tracers, with no direction.
					if dep == "telemetry" || dep == name {
						continue
					}
					if !slices.Contains(spec.allowed, dep) {
						t.Errorf(
							"%s imports %q\n  %s may import: %v\n"+
								"  a dependency in this direction inverts the hexagon",
							relPath(root, file), imp, name,
							append(slices.Clone(spec.allowed), "telemetry"),
						)
					}
				}
			}
		})
	}
}
