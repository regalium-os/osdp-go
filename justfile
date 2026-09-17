# Copyright 2026 RegaliumOS™.
# SPDX-License-Identifier: Apache-2.0

default:
    @just --list

# Protobuf/FlatBuffers codegen and the schema-mirror drift gate.
[group('codegen')]
mod gen 'tools/just/gen.just'

# --- Build ---

# Compile every module in the workspace.
#
# `go build ./...` covers the module it is run in and no other. With three
# modules in the workspace that quietly builds a third of the repository, which
# is how the schema module came to ship generated code that did not compile.
#
# The output is discarded because a module whose only command is in a directory
# of the same name cannot write its binary beside it.
[group('build')]
build:
    @just _each "go build -o /dev/null ./..."

# Vet every module.
[group('build')]
vet:
    @just _each "go vet ./..."

# Run a command in each module of the workspace.
[private]
_each command:
    #!/usr/bin/env bash
    set -euo pipefail
    for dir in $(go work edit -json | jq -r '.Use[].DiskPath'); do
      printf '\n--- %s ---\n' "$dir"
      (cd "$dir" && {{command}})
    done

# Format all Go sources.
[group('build')]
fmt:
    gofmt -w .

# --- Test ---

# Run the full Go test suite, in every module.
[group('test')]
test:
    @just _each "go test -count=1 ./..."

# Run the suite under the race detector, as CI does.
#
# cgo is on here and nowhere else. Go's detector builds without it only on
# darwin, so a Linux machine cannot run this with CGO_ENABLED=0 -- and a Mac
# that can will happily hide that from you until CI says otherwise. See the
# race job in .github/workflows/ci.yml.
[group('test')]
race:
    CGO_ENABLED=1 go test -race -count=1 ./...

# Architecture conformance: layering direction and I/O purity.
[group('test')]
arch:
    go test ./internal/arch/ -v

# Decode the hex fixture corpus byte for byte; the gate between phases.
[group('test')]
fixtures:
    go test ./... -run 'TestFixture' -v

# --- Bazel ---

# Regenerate BUILD files from the Go sources.
[group('bazel')]
tidy:
    bazel run //:gazelle
    bazel mod tidy

[group('bazel')]
bazel-test:
    bazel test //...

# --- Lint ---

# Static analysis, matching CI's configuration.
[group('lint')]
lint:
    golangci-lint run ./...

# Confirm nothing has pulled cgo into the graph.
[group('lint')]
purity:
    #!/usr/bin/env bash
    if go list -deps ./... | grep -qx 'runtime/cgo'; then
      echo "runtime/cgo is in the dependency graph"; exit 1
    fi
    echo "pure Go: $(go list -deps ./... | wc -l | tr -d ' ') packages, no cgo"

# Google AIP lint over the protobuf module, matching CI's strictness.
[group('lint')]
api-lint:
    cd protobuf && api-linter --set-exit-status $(find . -name '*.proto')
