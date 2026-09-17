// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

// Package transport defines the port through which OSDP octets leave and enter
// the process. It is an interface package: definitions only, no implementations.
//
// # Layer contract
//
// transport is the boundary of the pure core. It declares what the core needs
// from the outside world -- read these bytes, write these bytes, honour this
// deadline, report a line error -- and nothing about how that is accomplished.
// There is no serial port here, no socket, no file handle.
//
// Concrete transports live in driver. That inversion is the whole point: bus
// depends on this interface, driver satisfies it, and the core never learns
// whether it is speaking to an RS-485 line, a TCP socket, or a byte slice in a
// test.
//
// A package in this directory that opens something is a bug.
//
// # Allowed imports
//
//	stdlib, telemetry
package transport
