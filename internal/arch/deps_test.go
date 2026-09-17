package arch

import (
	"path/filepath"
	"strings"
	"testing"
)

// bannedImports are module paths no code in this repository may import, with
// the reason stated so a failure explains itself.
//
// OpenTelemetry is banned as a direct dependency. Observability goes through
// the the-protobuf-project telemetry SDK, which owns the OpenTelemetry
// integration, the exporter lifecycle and the struct-tag attribute convention.
// Importing the tracing API here as well would mean two things deciding how a
// span is made, and the core would acquire a dependency it does not need: the
// pure layers speak only to telemetry.Tracer, which is three methods of
// standard library.
var bannedImports = map[string]string{
	"go.opentelemetry.io/": "use the telemetry package and adapters/telemetrygo; " +
		"OpenTelemetry is reached through the telemetry-go SDK, never imported directly",
}

// forbiddenExact are import paths banned by exact match rather than prefix.
//
// "C" is the cgo pseudo-package. Banning it here is what makes the pure-Go
// claim structural rather than a CI environment variable someone can flip: a
// file that imports it fails this test whatever CGO_ENABLED says. Pure Go is
// what lets the library cross-compile to the ARM panels it runs on with one
// flag, and what keeps a static binary static.
var forbiddenExact = map[string]string{
	"C": "cgo is not permitted anywhere in this repository; the library is pure Go",
}

// depsDirs are the trees scanned. Every hand-written Go file in the repository
// is covered, across all four modules in the workspace.
var depsDirs = []string{
	"internal", "telemetry", "tools",
}

// TestNoBannedImports keeps the dependency rules in CLAUDE.md honest.
func TestNoBannedImports(t *testing.T) {
	root := repoRoot(t)

	var checked int
	files := rootGoFiles(t, root)
	for _, dir := range depsDirs {
		files = append(files, allGoFiles(t, filepath.Join(root, dir))...)
	}

	{
		for _, file := range files {
			checked++
			for _, imp := range imports(t, file) {
				for prefix, reason := range bannedImports {
					if strings.HasPrefix(imp, prefix) {
						t.Errorf("%s imports %q\n  %s",
							relPath(root, file), imp, reason)
					}
				}
				if reason, banned := forbiddenExact[imp]; banned {
					t.Errorf("%s imports %q\n  %s", relPath(root, file), imp, reason)
				}
			}
		}
	}

	if checked == 0 {
		t.Error("no Go files were scanned; this test is checking nothing")
	}
	t.Logf("%d Go files free of banned imports", checked)
}
