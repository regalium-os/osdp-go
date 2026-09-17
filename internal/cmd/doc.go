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
// It marshals payloads to and from the protobuf domain types generated under
// protobuf/generated/go, which are the source of truth for field names and
// semantics.
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
//	stdlib, frame, telemetry, protobuf/generated/go
//
// # Tracing
//
// Spans are named osdp.cmd.encode and osdp.cmd.decode, attributed with the
// command code so a trace distinguishes a poll storm from a card read.
package cmd
