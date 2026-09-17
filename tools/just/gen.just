# Copyright 2026 RegaliumOS™.
# SPDX-License-Identifier: Apache-2.0

# Code generation for the schema layer.
#
# The .proto tree is the source of truth. The .fbs tree mirrors a deliberately
# small subset of it -- the poll-cycle payloads that are encoded thousands of
# times a minute -- and `just gen drift` is what keeps the mirror honest.

# Every path below is written from the repository root, because that is where
# the schemas, the generator config and the ordinal ledger all live. Without
# this a module's recipes run in the module's own directory and every one of
# them fails looking for a file one level up from where it is.
set working-directory := '../..'

default:
    @just --list gen

# Generate Go types from the protobuf module.
[group('codegen')]
go:
    buf generate --template tools/protobuf/buf/go.buf.gen.yaml

# buffers.yaml owns the layout and buffers.lock the ordinal assignments, so the
# mirror is derived from the same descriptor set as the Go types rather than
# maintained alongside them. Nobody hand-writes a .fbs here.
#
# Generate the mirrored FlatBuffers and Cap'n Proto schemas, and their Go.
[group('codegen')]
fbs:
    buffers generate

# Runs on every CI run rather than on demand. A field at the wrong ordinal is
# not a compile error anywhere in the chain: it is a value decoding as another
# field on a live bus, which is why this is a gate and not a lint.
#
# Check the .proto, its mirrors and the ordinal ledger all still agree.
[group('codegen')]
drift:
    go run ./tools/schemadrift

# Everything, in dependency order.
[group('codegen')]
all: go fbs drift
