default:
    @just --list

# Protobuf/FlatBuffers codegen and the schema-mirror drift gate.
[group('codegen')]
mod gen 'tools/just/gen.just'

# --- Build ---

# Compile every module in the workspace.
[group('build')]
build:
    go build ./...

# Vet every module.
[group('build')]
vet:
    go vet ./...

# Format all Go sources.
[group('build')]
fmt:
    gofmt -w .

# --- Test ---

# Run the full Go test suite.
[group('test')]
test:
    go test -count=1 ./...

# Run the suite under the race detector, as CI does.
[group('test')]
race:
    CGO_ENABLED=0 go test -race -count=1 ./...

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
