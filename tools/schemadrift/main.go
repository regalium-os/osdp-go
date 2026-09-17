// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

// Command schemadrift verifies that the FlatBuffers poll-cycle schema still
// mirrors the protobuf messages it claims to mirror.
//
// # Why this exists
//
// The .proto tree is the source of truth for the OSDP domain. A small subset of
// it -- the payloads exchanged on every poll, thousands of times a minute -- is
// additionally expressed in FlatBuffers because decoding those on the hot path
// should not allocate. Two schemas describing one thing is a standing invitation
// to drift, and drift here is not a compile error: it is a field that silently
// decodes to the wrong offset on a live access-control bus.
//
// So the mirror is declared in the schema itself and checked on every CI run.
//
// # Declaring a mirror
//
// A mirrored table names its source message in a comment directly above it:
//
//	/// mirrors: osdp.poll.v1.CardRead
//	table CardRead {
//	    ...
//	}
//
// The declaration lives in the .fbs rather than in a side-car config so that the
// two cannot drift apart on their own.
package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// mirrorDecl marks a FlatBuffers table as mirroring a protobuf message.
var mirrorDecl = regexp.MustCompile(`^\s*(?://+|/\*+)?\s*mirrors:\s*([A-Za-z0-9_.]+)\s*$`)

type mirror struct {
	protoMessage string // fully qualified, e.g. osdp.poll.v1.CardRead
	fbsTable     string
	file         string
	line         int
}

func main() {
	fbsDir := flag.String("fbs", "flatbuffers", "root of the FlatBuffers schema tree")
	flag.Parse()

	mirrors, err := scan(*fbsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "schemadrift: %v\n", err)
		os.Exit(2)
	}

	if len(mirrors) == 0 {
		fmt.Println("schemadrift: no mirrored tables declared; nothing to verify.")
		fmt.Println("  The mirror is populated in the phase that introduces the")
		fmt.Println("  poll-cycle payload types. Until then this gate is a no-op.")
		return
	}

	// Deliberate forcing function: the moment a mirror is declared, this gate
	// goes red until the descriptor comparison below is implemented. A drift
	// check that silently passes is worse than no drift check, because it is
	// believed.
	fmt.Fprintf(os.Stderr, "schemadrift: %d mirrored table(s) declared but the\n", len(mirrors))
	fmt.Fprintf(os.Stderr, "  descriptor comparison is not implemented yet.\n\n")
	for _, m := range mirrors {
		fmt.Fprintf(os.Stderr, "  %s:%d  table %s  mirrors  %s\n",
			m.file, m.line, m.fbsTable, m.protoMessage)
	}
	fmt.Fprintf(os.Stderr, "\n  Implement the comparison before landing a mirrored type:\n")
	fmt.Fprintf(os.Stderr, "    buf build -o - --as-file-descriptor-set   (proto side)\n")
	fmt.Fprintf(os.Stderr, "    flatc --bfbs                              (fbs side)\n")
	fmt.Fprintf(os.Stderr, "  then compare field name, id and type for every mirrored pair.\n")
	os.Exit(1)
}

// scan walks the schema tree and collects every mirror declaration.
func scan(root string) ([]mirror, error) {
	var found []mirror

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".fbs") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		var pending string
		var pendingLine int
		for i, line := range strings.Split(string(content), "\n") {
			if m := mirrorDecl.FindStringSubmatch(line); m != nil {
				pending, pendingLine = m[1], i+1
				continue
			}
			if pending == "" {
				continue
			}
			// The declaration applies to the next table it precedes.
			if table, ok := tableName(line); ok {
				found = append(found, mirror{
					protoMessage: pending,
					fbsTable:     table,
					file:         path,
					line:         pendingLine,
				})
				pending = ""
			}
		}
		if pending != "" {
			return fmt.Errorf("%s:%d: `mirrors:` declaration is not followed by a table", path, pendingLine)
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return found, nil
}

var tableDecl = regexp.MustCompile(`^\s*table\s+([A-Za-z_][A-Za-z0-9_]*)`)

func tableName(line string) (string, bool) {
	m := tableDecl.FindStringSubmatch(line)
	if m == nil {
		return "", false
	}
	return m[1], true
}
