// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

// Package cmd implements the OSDP command and reply codec: the CMND/REPLY code
// and the payload that follows it, carried inside a frame.
//
// # Layer contract
//
// cmd sits directly above frame. It owns the meaning of the command byte and the
// structure of each payload -- osdp_POLL, osdp_ID, osdp_CAP, osdp_LSTAT,
// osdp_RAW, osdp_KEYPAD, osdp_MFG and the rest -- and the symmetric reply set.
//
// The .proto tree is the source of truth for field names and semantics, but cmd
// does not import the types generated from it: that would put
// google.golang.org/protobuf in the dependency graph of every consumer, and
// this module has no dependencies by design. Conversion lives in the schema
// module, in protobuf/record, which imports this one.
//
// Vendor-specific behaviour does NOT live here. osdp_MFG is decoded only as far
// as its OUI and vendor-defined body; interpreting that body is the job of a
// provider. This is what keeps HID, Gallagher and Salto from each growing their
// own fork of the codec.
//
// cmd does not frame, does not encrypt, and performs no I/O.
//
// # Allowed imports
//
//	stdlib, frame, telemetry
//
// # Tracing
//
// Spans are named osdp.cmd.encode and osdp.cmd.decode, attributed with the
// command code so a trace distinguishes a poll storm from a card read.
package cmd
