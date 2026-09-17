// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package main

import "fmt"

// fbsTypeFor maps a protobuf type to the FlatBuffers type the generators emit
// for it.
//
// An unmapped type is not assumed compatible. A mirrored message that grows a
// map, a oneof or a nested message reports an unknown mapping and turns the
// gate red, which is the right answer: nobody has decided yet what that field
// looks like on the other side, and a gate that guessed would be believed.
var fbsTypeFor = map[string]string{
	"double":          "double",
	"float":           "float",
	"int32":           "int",
	"int64":           "long",
	"uint32":          "uint",
	"uint64":          "ulong",
	"sint32":          "int",
	"sint64":          "long",
	"fixed32":         "uint",
	"fixed64":         "ulong",
	"bool":            "bool",
	"string":          "string",
	"bytes":           "[ubyte]",
	"repeated bytes":  "[[ubyte]]",
	"repeated string": "[string]",
	"repeated int32":  "[int]",
}

// Drift is one disagreement between the schemas.
type Drift struct {
	Message string
	Field   string
	Detail  string
}

func (d Drift) String() string {
	if d.Field == "" {
		return fmt.Sprintf("%s: %s", d.Message, d.Detail)
	}
	return fmt.Sprintf("%s.%s: %s", d.Message, d.Field, d.Detail)
}

// Compare checks one mirrored pair against each other and against the ledger.
//
// All three must agree. The proto says what the field is, the ledger says which
// slot it was given and has been committed to, and the .fbs says where the
// mirror actually put it. Two of the three agreeing is the interesting case:
// it means the third moved.
func Compare(proto, fbs Message, qualified string, ledger *Ledger, enums map[string]bool) []Drift {
	var drifts []Drift

	slots, names, known := ledger.Message(qualified)
	if !known {
		drifts = append(drifts, Drift{Message: qualified,
			Detail: "no entry in buffers.lock; the ledger has not recorded this message's slots"})
	}

	for _, want := range proto.Fields {
		got, ok := fbs.Field(want.Name)
		if !ok {
			drifts = append(drifts, Drift{Message: qualified, Field: want.Name,
				Detail: fmt.Sprintf("in %s but missing from the mirror", proto.File)})
			continue
		}
		drifts = append(drifts, compareField(qualified, want, got, enums)...)

		if known {
			drifts = append(drifts, compareLedger(qualified, want, slots, names)...)
		}
	}

	// A table with a field the proto does not have is drift in the other
	// direction: a mirror that has grown something of its own.
	for _, got := range fbs.Fields {
		if _, ok := proto.Field(got.Name); !ok {
			drifts = append(drifts, Drift{Message: qualified, Field: got.Name,
				Detail: fmt.Sprintf("in %s but not in the proto it mirrors", fbs.File)})
		}
	}
	return drifts
}

// compareField checks one field's slot and type across the two schemas.
func compareField(message string, want, got Field, enums map[string]bool) []Drift {
	var drifts []Drift

	if want.Slot != got.Slot {
		drifts = append(drifts, Drift{Message: message, Field: want.Name,
			Detail: fmt.Sprintf(
				"proto field %d maps to FlatBuffers id %d, but the mirror puts it at id %d",
				want.Slot, fbsOrdinal(want.Slot), fbsOrdinal(got.Slot))})
	}

	// An enum mirrors as itself: FlatBuffers takes the same type name, which
	// is why the declaration has to be read rather than inferred from the
	// spelling of the field's type.
	expected, mapped := fbsTypeFor[want.Type]
	if enums[want.Type] {
		expected, mapped = want.Type, true
	}

	switch {
	case !mapped:
		drifts = append(drifts, Drift{Message: message, Field: want.Name,
			Detail: fmt.Sprintf("no known FlatBuffers mapping for proto type %q", want.Type)})
	case expected != got.Type:
		drifts = append(drifts, Drift{Message: message, Field: want.Name,
			Detail: fmt.Sprintf("proto %s should mirror as %s, but the schema says %s",
				want.Type, expected, got.Type)})
	}
	return drifts
}

// compareLedger checks the field against the committed ordinal record.
func compareLedger(message string, want Field, slots map[int]int, names map[int]string) []Drift {
	var drifts []Drift

	ordinal, recorded := slots[want.Slot]
	if !recorded {
		return append(drifts, Drift{Message: message, Field: want.Name,
			Detail: fmt.Sprintf("field %d is not in buffers.lock; regenerate before shipping it",
				want.Slot)})
	}
	if ordinal != fbsOrdinal(want.Slot) {
		drifts = append(drifts, Drift{Message: message, Field: want.Name,
			Detail: fmt.Sprintf("buffers.lock gives field %d ordinal %d, not %d",
				want.Slot, ordinal, fbsOrdinal(want.Slot))})
	}
	if named, ok := names[want.Slot]; ok && named != want.Name {
		drifts = append(drifts, Drift{Message: message, Field: want.Name,
			Detail: fmt.Sprintf("buffers.lock records field %d as %q; renaming a field in place "+
				"keeps the slot and changes its meaning", want.Slot, named)})
	}
	return drifts
}
