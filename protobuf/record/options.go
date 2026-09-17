// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package record

// settings are the options a conversion was given.
type settings struct {
	credentials bool
}

// Option configures how an event is recorded.
type Option func(*settings)

// WithCredentials includes the credential bits and keypad digits in the record.
//
// # Read this before using it
//
// Without it, a CardRead records the reader, the format and the bit count --
// enough to say a 26-bit Wiegand credential was presented at reader 0, and not
// enough to say whose. That is the default because a record leaves the panel:
// it goes over a network, into a database, into a backup, and onto whatever
// reads that backup in five years.
//
// With it, the record contains the number encoded in somebody's badge, and a
// keypad entry contains what is frequently their PIN. There are systems that
// need this -- a panel that makes its access decisions centrally has to send
// the credential somewhere -- and for those it is correct. It is an opt-in so
// that it is a decision somebody made rather than a default nobody noticed.
//
// The schema says the same thing in the field's own documentation: hash or
// truncate before it leaves the panel.
func WithCredentials() Option {
	return func(s *settings) { s.credentials = true }
}
