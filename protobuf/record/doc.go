// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

// Package record turns what the runtime observed into the domain records the
// schema defines.
//
// # Why this is not in the library
//
// The osdp-go module has no dependencies and that is deliberate: it is imported
// into control-panel software that must keep a bus alive for years, and every
// dependency is something that can break a build somebody cannot postpone.
// Converting to protobuf needs google.golang.org/protobuf, so it lives here, in
// the schema module, which already has it.
//
// The direction is what makes that work. This module imports osdp-go; osdp-go
// imports nothing. An application that wants records opts into this module and
// pays for it; one that only wants to drive a bus does not.
//
// # What it produces
//
// An osdp.Event says what happened. An eventpbv1.Event is the record of it:
// named, timestamped, and carrying whether the exchange that produced it was
// authenticated -- because a credential that arrived in the clear is a
// different piece of evidence from one that arrived over a secure channel, and
// after the fact there is no way to tell them apart unless it was written down
// at the time.
//
// # Credentials
//
// A card number is not included unless the caller asks for it. See
// WithCredentials, and read it before using it.
package record
