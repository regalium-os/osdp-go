# Copyright 2026 RegaliumOS™.
# SPDX-License-Identifier: Apache-2.0

# Code generation for the schema layer.
#
# The .proto tree is the source of truth. The .fbs tree mirrors a deliberately
# small subset of it -- the poll-cycle payloads that are encoded thousands of
# times a minute -- and `just gen drift` is what keeps the mirror honest.

default:
    @just --list gen

# Generate Go types from the protobuf module.
[group('codegen')]
go:
    buf generate --template tools/protobuf/buf/go.buf.gen.yaml

# Generate Go types from the FlatBuffers poll-cycle schema.
[group('codegen')]
fbs:
    flatc --go --go-namespace osdpfbs \
        -o flatbuffers/generated/go \
        flatbuffers/osdp/poll/v1/*.fbs

# Verify the .fbs mirror still matches the .proto it mirrors. Fails loudly on
# any divergence; this runs on every CI run, not on demand.
[group('codegen')]
drift:
    go run ./tools/schemadrift -config schema-mirror.yaml

# Everything, in dependency order.
[group('codegen')]
all: go fbs drift
