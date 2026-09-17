// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// Each case damages the matching triple in one way and asserts the gate says
// so. The want string is the phrase a person reading CI output needs to see;
// matching on it keeps the message itself under test, because a drift report
// nobody can act on is barely better than no report.
func TestTheGateDetectsDrift(t *testing.T) {
	for _, tc := range []struct {
		name               string
		proto, fbs, ledger string
		want               string
	}{
		{
			name:  "a field moved to another slot in the mirror",
			proto: goodProto,
			fbs: strings.Replace(goodFBS,
				"format:int (id: 1);", "format:int (id: 2);", 1),
			ledger: goodLedger,
			want:   "the mirror puts it at id 2",
		},
		{
			name:  "a field the mirror does not have",
			proto: goodProto,
			fbs: strings.Replace(goodFBS,
				"  format:int (id: 1);\n", "", 1),
			ledger: goodLedger,
			want:   "missing from the mirror",
		},
		{
			name:  "a field the mirror grew on its own",
			proto: goodProto,
			fbs: strings.Replace(goodFBS,
				"  data:[ubyte] (id: 2);", "  data:[ubyte] (id: 2);\n  extra:int (id: 3);", 1),
			ledger: goodLedger,
			want:   "not in the proto it mirrors",
		},
		{
			name:  "a type that does not mirror",
			proto: goodProto,
			fbs: strings.Replace(goodFBS,
				"data:[ubyte] (id: 2);", "data:string (id: 2);", 1),
			ledger: goodLedger,
			want:   "should mirror as [ubyte]",
		},
		{
			name:  "the ledger moved a slot",
			proto: goodProto,
			fbs:   goodFBS,
			ledger: strings.Replace(goodLedger,
				"      - number: 2\n        ordinal: 1", "      - number: 2\n        ordinal: 7", 1),
			want: "buffers.lock gives field 2 ordinal 7",
		},
		{
			name:  "a field renamed in place, keeping its slot",
			proto: goodProto,
			fbs:   goodFBS,
			ledger: strings.Replace(goodLedger,
				"        name: format", "        name: card_format", 1),
			want: "buffers.lock records field 2 as \"card_format\"",
		},
		{
			name: "a message the ledger has never recorded",
			proto: strings.Replace(goodProto,
				"message CardRead {", "message Unrecorded {\n  int32 a = 1;\n}\n\nmessage CardRead {", 1),
			fbs: strings.Replace(goodFBS,
				"table CardRead {", "table Unrecorded {\n  a:int (id: 0);\n}\n\ntable CardRead {", 1),
			ledger: goodLedger,
			want:   "no entry in buffers.lock",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			drifts := reportedDrift(t, tc.proto, tc.fbs, tc.ledger)
			if drifts == "" {
				t.Fatal("the gate passed; this schema has drifted and it did not notice")
			}
			if !strings.Contains(drifts, tc.want) {
				t.Errorf("report does not explain the drift\n  want a mention of: %s\n  got:\n%s",
					tc.want, drifts)
			}
		})
	}
}

// reportedDrift runs the comparison and returns what it found, without the
// process exit the command itself performs.
func reportedDrift(t *testing.T, proto, fbs, ledger string) string {
	t.Helper()
	fbsRoot, protoRoot, ledgerPath := tree(t, proto, fbs, ledger)

	parsed, err := ParseLedger(ledgerPath)
	if err != nil {
		t.Fatalf("ParseLedger: %v", err)
	}

	schemas, err := findSchemas(fbsRoot)
	if err != nil || len(schemas) == 0 {
		t.Fatalf("findSchemas(%s) = %v, %v", fbsRoot, schemas, err)
	}

	var all []string
	for _, path := range schemas {
		_, drifts, vErr := verify(path, protoRoot, parsed)
		if vErr != nil {
			return vErr.Error()
		}
		for _, d := range drifts {
			all = append(all, d.String())
		}
	}
	return strings.Join(all, "\n")
}
