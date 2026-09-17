// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package main

import "fmt"

// Field is one field of a message or table, as declared in whichever schema it
// came from.
type Field struct {
	// Name is the field name, which the generators keep identical across
	// targets so that a human comparing two schemas is comparing like for like.
	Name string

	// Type is the declared type in that schema's own vocabulary: int32 in a
	// .proto, int in a .fbs.
	Type string

	// Slot is where the field sits on the wire. A .proto records it as a field
	// number counting from one; a .fbs records it as an id counting from zero.
	// Both are normalised to the proto numbering here, so a comparison never
	// has to remember which convention it is holding.
	Slot int
}

// Message is a protobuf message or the FlatBuffers table mirroring it.
type Message struct {
	Name   string
	Fields []Field

	// File and Line locate the declaration, so a drift report points at
	// something a reader can open.
	File string
	Line int
}

// Field returns the named field, and whether the message declares one.
func (m Message) Field(name string) (Field, bool) {
	for _, f := range m.Fields {
		if f.Name == name {
			return f, true
		}
	}
	return Field{}, false
}

// String identifies the message for a report.
func (m Message) String() string { return fmt.Sprintf("%s (%s:%d)", m.Name, m.File, m.Line) }

// fbsOrdinal converts a proto field number to the FlatBuffers id the
// generators assign it.
//
// The offset is not arbitrary and not a detail to be rediscovered at a call
// site: protobuf numbers fields from one because zero is reserved, and
// FlatBuffers indexes slots from zero because it is an array offset. One is the
// other minus one, everywhere, forever.
func fbsOrdinal(protoNumber int) int { return protoNumber - 1 }

// protoNumber is the inverse of fbsOrdinal.
func protoNumber(fbsID int) int { return fbsID + 1 }
