// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package arch

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// spdxIdentifier is the licence every source file declares.
//
// The machine-readable identifier is the point: a downstream compliance scan
// reads this line, not the prose in LICENSE. A file without it is a file some
// tool will report as unlicensed, which for a library imported into commercial
// control-panel software is a procurement problem rather than a style one.
const spdxIdentifier = "SPDX-License-Identifier: Apache-2.0"

// licensedExtensions are the file kinds the notice applies to, with the comment
// syntax each one uses.
var licensedExtensions = map[string]string{
	".go":    "//",
	".proto": "//",
	".bazel": "#",
	".yaml":  "#",
	".yml":   "#",
	".just":  "#",
	".hex":   "#",
	".kat":   "#",
}

// licensedNames are the files with no extension to go by.
var licensedNames = map[string]string{
	"justfile": "#",
	".bazelrc": "#",
}

// unlicensedTrees hold files the notice does not apply to.
//
// Generated output is excluded because it is rewritten wholesale on every run:
// the notice belongs on the schema it is generated from, and protoc carries a
// .proto file's leading comment block into the Go it emits.
var unlicensedTrees = []string{
	"protobuf/generated", "flatbuffers/generated", ".git",
}

// TestEverySourceFileDeclaresItsLicence.
//
// Adding a file and forgetting the notice is the easiest omission in this
// repository to make and the hardest to notice, because nothing about the build
// or the tests depends on it. So it is asserted here rather than left to a
// reviewer to spot in a diff of ninety files.
func TestEverySourceFileDeclaresItsLicence(t *testing.T) {
	root := repoRoot(t)

	var checked int
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := relPath(root, path)
		if d.IsDir() {
			if isUnlicensed(rel) {
				return fs.SkipDir
			}
			return nil
		}

		prefix, ok := commentPrefix(d.Name())
		if !ok {
			return nil
		}
		checked++

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !declaresLicence(string(content), prefix) {
			t.Errorf("%s carries no licence notice\n"+
				"  the first two lines must be:\n"+
				"    %s Copyright 2026 RegaliumOS™.\n"+
				"    %s %s", rel, prefix, prefix, spdxIdentifier)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the source tree: %v", err)
	}

	if checked == 0 {
		t.Error("no source files were examined; this test is checking nothing")
	}
	t.Logf("%d source files declare Apache-2.0", checked)
}

// declaresLicence reports whether the notice opens the file, in the comment
// syntax that file kind uses. Only the opening lines are examined: an
// identifier buried halfway down is not a licence notice, and a scanner
// looking for one will not find it there either.
func declaresLicence(content, prefix string) bool {
	for _, line := range strings.SplitN(content, "\n", 5)[:4] {
		if strings.HasPrefix(line, prefix) && strings.Contains(line, spdxIdentifier) {
			return true
		}
	}
	return false
}

// commentPrefix returns the comment syntax for a file the notice applies to.
func commentPrefix(name string) (string, bool) {
	if prefix, ok := licensedNames[name]; ok {
		return prefix, true
	}
	prefix, ok := licensedExtensions[filepath.Ext(name)]
	return prefix, ok
}

// isUnlicensed reports whether a directory is outside the notice's scope.
//
// The match is on whole path elements. A bare prefix test would exclude
// .github along with .git, and quietly stop checking the CI workflows -- which
// is exactly the class of silent skip the corpus walkers guard against
// elsewhere in this package.
func isUnlicensed(rel string) bool {
	// bazel-bin, bazel-out and friends are symlinks into Bazel's cache that it
	// drops in the workspace root. They are not part of the source tree.
	if strings.HasPrefix(rel, "bazel-") {
		return true
	}
	for _, tree := range unlicensedTrees {
		if rel == tree || strings.HasPrefix(rel, tree+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
