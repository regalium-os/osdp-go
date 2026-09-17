// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

// Package provider adapts the OSDP core to a specific reader vendor.
//
// # Layer contract
//
// A Provider expresses what differs between an HID reader, a Gallagher reader,
// a Salto lock and a generic wall reader -- and that difference is deliberately
// narrow. It is confined to two things:
//
//   - osdp_MFG extension commands, keyed by the vendor's OUI.
//   - Capability negotiation: how a device answers osdp_CAP, which optional
//     features it truthfully reports, and which quirks must be inferred when it
//     misreports.
//
// A provider does NOT reimplement framing, sequencing, or Secure Channel. If a
// vendor appears to need its own frame codec, the finding belongs in frame as a
// quirk flag with a fixture proving it, not as a second codec here. Vendor
// forks of the wire format are how OSDP stacks rot, and this boundary exists to
// prevent it.
//
// Providers may perform I/O -- some vendor flows require out-of-band steps -- so
// this is an edge layer alongside driver.
//
// # Allowed imports
//
//	stdlib, frame, cmd, secure, bus, transport, driver, telemetry,
//	protobuf/generated/go
//
// # Tracing
//
// Span osdp.provider.<vendor>.<operation>, attributed with the OUI and the
// negotiated capability set.
package provider
