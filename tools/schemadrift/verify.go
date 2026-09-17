// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"path/filepath"
)

// verify checks every mirrored table in one schema file, returning how many it
// checked and what disagreed.
func verify(fbsPath, protoRoot string, ledger *Ledger) (int, []Drift, error) {
	schema, err := ParseFBS(fbsPath)
	if err != nil {
		return 0, nil, err
	}
	if schema.Source == "" && len(schema.Declared) == 0 {
		return 0, nil, nil
	}

	// A file-level source declaration mirrors every table in the file; a
	// per-table one names its own message. Both resolve to the same question:
	// which .proto message is this table claiming to be.
	source, err := protoSource(schema, protoRoot)
	if err != nil {
		return 0, nil, err
	}

	byName := make(map[string]Message, len(source.Messages))
	for _, m := range source.Messages {
		byName[m.Name] = m
	}

	var drifts []Drift
	var checked int
	for _, table := range schema.Tables {
		qualified := schema.Namespace + "." + table.Name
		if declared, ok := schema.Declared[table.Name]; ok {
			qualified = declared
		}

		message, ok := byName[table.Name]
		if !ok {
			drifts = append(drifts, Drift{Message: qualified,
				Detail: fmt.Sprintf("table at %s:%d mirrors nothing: no message of that name "+
					"in %s", table.File, table.Line, schema.Source)})
			continue
		}

		checked++
		drifts = append(drifts, Compare(message, table, qualified, ledger, source.Enums)...)
	}
	return checked, drifts, nil
}

// protoSource loads the .proto a schema mirrors.
func protoSource(schema *Schema, protoRoot string) (*ProtoFile, error) {
	if schema.Source == "" {
		return nil, fmt.Errorf("%s: per-table `mirrors:` declarations need a source file; "+
			"add the generator's `// source:` header", schema.Path)
	}

	path := filepath.Join(protoRoot, schema.Source)
	source, err := ParseProto(path)
	if err != nil {
		return nil, fmt.Errorf("%s claims to mirror %s: %w", schema.Path, path, err)
	}

	// The namespace and the package name are kept equal by the generators. If
	// they ever differ, a table resolves to a message in another package and
	// the comparison would be nonsense.
	if schema.Namespace != "" && source.Package != "" && schema.Namespace != source.Package {
		return nil, fmt.Errorf("%s: namespace %q mirrors %s, whose package is %q",
			schema.Path, schema.Namespace, path, source.Package)
	}
	return source, nil
}
