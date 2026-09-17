// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var (
	fbsNamespace = regexp.MustCompile(`^\s*namespace\s+([A-Za-z0-9_.]+)\s*;`)
	fbsTable     = regexp.MustCompile(`^table\s+([A-Za-z_][A-Za-z0-9_]*)\s*\{`)
	fbsField     = regexp.MustCompile(`^\s+([a-z_][a-z0-9_]*)\s*:\s*([A-Za-z0-9_\[\]]+)\s*\(\s*id\s*:\s*(\d+)\s*\)`)

	// source names the .proto a generated schema was produced from. The
	// generators emit it, which is what makes the mirror self-declaring: the
	// claim travels in the file it is a claim about.
	fbsSource = regexp.MustCompile(`^//\s*source:\s*(\S+\.proto)\s*$`)

	// mirrors is the hand-written form, for a schema somebody wrote rather
	// than generated. Both are accepted; a file needs one of them.
	fbsMirrors = regexp.MustCompile(`^\s*(?://+|/\*+)?\s*mirrors:\s*([A-Za-z0-9_.]+)\s*$`)
)

// Schema is one parsed .fbs file.
type Schema struct {
	// Path is the file it came from.
	Path string

	// Namespace is the FlatBuffers namespace, which the generators keep equal
	// to the proto package.
	Namespace string

	// Source is the .proto this schema mirrors, as the generator recorded it,
	// relative to the protobuf module root. Empty for a schema that declares
	// its mirrors individually instead.
	Source string

	// Tables are the tables declared, in file order.
	Tables []Message

	// Declared maps a table name to the fully qualified proto message a
	// hand-written `mirrors:` comment claims it mirrors.
	Declared map[string]string
}

// ParseFBS reads a FlatBuffers schema.
//
// Enums are skipped: they carry values rather than wire slots, and a mismatched
// enum value is caught by the generator refusing to assign a different ordinal,
// which is what buffers.lock records.
func ParseFBS(path string) (*Schema, error) {
	// #nosec G304 -- reading a schema the operator pointed this gate at is what
	// the gate is for; there is no untrusted input and no server here.
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	s := &Schema{Path: path, Declared: map[string]string{}}

	var current *Message
	var pendingMirror string
	for i, line := range strings.Split(string(content), "\n") {
		switch {
		case fbsSource.MatchString(line):
			s.Source = fbsSource.FindStringSubmatch(line)[1]

		case fbsNamespace.MatchString(line):
			s.Namespace = fbsNamespace.FindStringSubmatch(line)[1]

		case fbsMirrors.MatchString(line):
			pendingMirror = fbsMirrors.FindStringSubmatch(line)[1]

		case fbsTable.MatchString(line):
			name := fbsTable.FindStringSubmatch(line)[1]
			current = &Message{Name: name, File: path, Line: i + 1}
			if pendingMirror != "" {
				s.Declared[name], pendingMirror = pendingMirror, ""
			}

		case strings.HasPrefix(line, "}"):
			if current != nil {
				s.Tables = append(s.Tables, *current)
				current = nil
			}

		case current != nil:
			if f, ok := parseFBSField(line); ok {
				current.Fields = append(current.Fields, f)
			}
		}
	}
	if current != nil {
		return nil, fmt.Errorf("%s: table %q is not closed", path, current.Name)
	}
	if pendingMirror != "" {
		return nil, fmt.Errorf("%s: `mirrors: %s` is not followed by a table", path, pendingMirror)
	}
	return s, nil
}

// parseFBSField reads one field declaration, normalising its id to the proto
// field numbering so nothing downstream has to hold both conventions.
func parseFBSField(line string) (Field, bool) {
	m := fbsField.FindStringSubmatch(line)
	if m == nil {
		return Field{}, false
	}

	id, err := strconv.Atoi(m[3])
	if err != nil {
		return Field{}, false
	}
	return Field{Name: m[1], Type: m[2], Slot: protoNumber(id)}, true
}
