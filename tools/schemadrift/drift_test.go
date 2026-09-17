// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The fixtures below are a matching triple -- proto, mirror and ledger -- which
// each test then damages in one specific way. Damaging a good pair is the only
// way to know the gate reads what it claims to: a gate tested solely against
// correct input reports success for both the case where it works and the case
// where it does nothing.

const goodProto = `syntax = "proto3";
package osdp.event.v1;

message CardRead {
  int32 reader = 1;
  int32 format = 2;
  bytes data = 3;
}

enum StatusKind {
  STATUS_KIND_UNSPECIFIED = 0;
}
`

const goodFBS = `// source: osdp/event/v1/payloads.proto
namespace osdp.event.v1;

table CardRead {
  reader:int (id: 0);
  format:int (id: 1);
  data:[ubyte] (id: 2);
}
`

const goodLedger = `version: 1
messages:
  - message: osdp.event.v1.CardRead
    fields:
      - number: 1
        ordinal: 0
        name: reader
      - number: 2
        ordinal: 1
        name: format
      - number: 3
        ordinal: 2
        name: data
enums:
  - enum: osdp.event.v1.StatusKind
    values:
      - number: 0
        ordinal: 0
        name: STATUS_KIND_UNSPECIFIED
`

// tree writes a proto/mirror/ledger triple and returns the roots to check.
func tree(t *testing.T, proto, fbs, ledger string) (fbsRoot, protoRoot, ledgerPath string) {
	t.Helper()
	root := t.TempDir()

	protoRoot = filepath.Join(root, "protobuf")
	protoDir := filepath.Join(protoRoot, "osdp", "event", "v1")
	fbsRoot = filepath.Join(root, "mirror")

	for _, dir := range []string{protoDir, fbsRoot} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("preparing %s: %v", dir, err)
		}
	}

	ledgerPath = filepath.Join(root, "buffers.lock")
	write(t, filepath.Join(protoDir, "payloads.proto"), proto)
	write(t, filepath.Join(fbsRoot, "payloads.fbs"), fbs)
	write(t, ledgerPath, ledger)

	return fbsRoot, protoRoot, ledgerPath
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

// check runs the gate over a triple and returns what it reported.
func check(t *testing.T, proto, fbs, ledger string) error {
	t.Helper()
	return run(tree(t, proto, fbs, ledger))
}

func TestAMatchingMirrorPasses(t *testing.T) {
	if err := check(t, goodProto, goodFBS, goodLedger); err != nil {
		t.Errorf("a matching triple was reported as drift: %v", err)
	}
}

// TestAnEmptyTreeFails is the guard that matters most.
//
// A gate reporting success because it could not find the tree it was meant to
// check is worse than no gate, because it is believed. This repository has
// shipped that mistake once; the point of this test is that it does not ship a
// second time.
func TestAnEmptyTreeFails(t *testing.T) {
	empty := t.TempDir()

	err := run(empty, empty, filepath.Join(empty, "buffers.lock"))
	if err == nil {
		t.Fatal("a tree with no schemas passed; the gate verified nothing and said so was fine")
	}
	if !strings.Contains(err.Error(), "no .fbs schemas found") {
		t.Errorf("error = %v, want it to name the missing schemas", err)
	}
}

// TestAMissingTreeFails: a path that does not exist at all is the same failure
// as an empty one, and must not be mistaken for a clean run.
func TestAMissingTreeFails(t *testing.T) {
	if err := run(filepath.Join(t.TempDir(), "nowhere"), t.TempDir(), "buffers.lock"); err == nil {
		t.Error("a nonexistent schema tree passed")
	}
}
