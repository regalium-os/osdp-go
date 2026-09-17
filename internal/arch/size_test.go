package arch

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// maxFileLines is the ceiling for a hand-written Go source file.
//
// The limit is about comprehension, not aesthetics. A file that fits in a couple
// of screens can be held in the head while reading it, and a protocol codec that
// nobody can hold in their head is a codec where the off-by-one lives. When a
// file approaches the ceiling the fix is to split it along a seam that already
// exists -- encode from decode, one command family from another -- never to
// delete the comments that explain why the bytes are what they are.
const maxFileLines = 200

// sizedDirs are the trees the limit applies to. Generated code is excluded by
// omission: protobuf/ and flatbuffers/ output is machine-written and is held to
// schema review instead.
var sizedDirs = []string{
	"frame", "cmd", "secure", "bus", "transport",
	"driver", "provider", "telemetry",
	"internal", "tools",
}

// TestSourceFilesAreWithinLineLimit keeps files readable, tests included.
func TestSourceFilesAreWithinLineLimit(t *testing.T) {
	root := repoRoot(t)

	var checked int
	for _, dir := range sizedDirs {
		for _, file := range allGoFiles(t, filepath.Join(root, dir)) {
			checked++

			content, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("reading %s: %v", file, err)
			}
			lines := bytes.Count(content, []byte("\n"))
			if !bytes.HasSuffix(content, []byte("\n")) && len(content) > 0 {
				lines++
			}

			if lines > maxFileLines {
				t.Errorf("%s is %d lines, limit is %d\n"+
					"  split it along a seam that already exists;\n"+
					"  do not reclaim the lines by deleting documentation",
					relPath(root, file), lines, maxFileLines)
			}
		}
	}

	if checked == 0 {
		t.Error("no Go files were measured; this test is checking nothing")
	}
	t.Logf("%d Go files within the %d-line limit", checked, maxFileLines)
}
