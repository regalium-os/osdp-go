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
    go test ./...

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

# Google AIP lint over the protobuf module, matching CI's strictness.
[group('lint')]
api-lint:
    cd protobuf && api-linter --set-exit-status $(find . -name '*.proto')
