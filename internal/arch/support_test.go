// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package arch

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// repoRoot walks up from the working directory to the go.work marker.
//
// It fails rather than guessing. These tests are only meaningful when they can
// read the real source tree, so a runner that hides the tree -- a sandbox
// without the sources declared as inputs, for one -- must produce a failure and
// not a green tick.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("resolving working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.work found above %q\n"+
				"  these tests read the source tree directly;\n"+
				"  run them with `go test ./internal/arch/` or `just arch`", dir)
		}
		dir = parent
	}
}

func relPath(root, path string) string {
	if r, err := filepath.Rel(root, path); err == nil {
		return r
	}
	return path
}

// goFiles returns the non-test Go files under dir, recursively.
//
// Test files are excluded on purpose: a fixture test legitimately reads files
// from disk, and that says nothing about whether the layer performs I/O.
func goFiles(t *testing.T, dir string) []string {
	t.Helper()
	return collect(t, dir, func(name string) bool {
		return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")
	})
}

// allGoFiles returns every Go file under dir, tests included. The line limit
// applies to test code too -- an unreadable test is still unreadable.
func allGoFiles(t *testing.T, dir string) []string {
	t.Helper()
	return collect(t, dir, func(name string) bool {
		return strings.HasSuffix(name, ".go")
	})
}

// collect walks dir and returns files whose base name satisfies match.
// Generated trees are skipped: machine output is not held to a line limit.
func collect(t *testing.T, dir string, match func(string) bool) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}

	var out []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			if name == "generated" || name == "testdata" {
				continue
			}
			out = append(out, collect(t, filepath.Join(dir, name), match)...)
			continue
		}
		if match(name) {
			out = append(out, filepath.Join(dir, name))
		}
	}
	return out
}

// rootGoFiles lists the Go files directly in the repository root -- the public
// facade package -- without recursing into the module's other trees.
func rootGoFiles(t *testing.T, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("reading %s: %v", root, err)
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
			out = append(out, filepath.Join(root, e.Name()))
		}
	}
	return out
}

// imports returns the import paths of a single Go file.
func imports(t *testing.T, file string) []string {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parsing %s: %v", file, err)
	}

	out := make([]string, 0, len(parsed.Imports))
	for _, spec := range parsed.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			t.Fatalf("unquoting import in %s: %v", file, err)
		}
		out = append(out, path)
	}
	return out
}
