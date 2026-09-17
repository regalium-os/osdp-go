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
	protoPackage = regexp.MustCompile(`^\s*package\s+([A-Za-z0-9_.]+)\s*;`)
	protoMessage = regexp.MustCompile(`^message\s+([A-Za-z_][A-Za-z0-9_]*)\s*\{`)
	protoEnum    = regexp.MustCompile(`^enum\s+([A-Za-z_][A-Za-z0-9_]*)\s*\{`)

	// A field line: an optional label, a type, a name, its number, and
	// whatever options follow. Options are ignored -- a field_behavior
	// annotation has no bearing on where the field sits on the wire.
	protoField = regexp.MustCompile(
		`^\s+(?:(repeated|optional)\s+)?([A-Za-z0-9_.]+)\s+([a-z_][a-z0-9_]*)\s*=\s*(\d+)\s*[\[;]`)
)

// ParseProto reads the top-level messages of a .proto file.
//
// It is a deliberately small parser rather than a protobuf descriptor read,
// because this gate must run with nothing installed but Go. A descriptor set
// would need buf or protoc present, and a check that only runs where its
// toolchain is installed is a check that stops running.
//
// Nested messages, oneofs and maps are not parsed. If a mirrored message ever
// grows one, the comparison reports the field it cannot account for rather than
// passing over it -- see Compare.
func ParseProto(path string) (*ProtoFile, error) {
	// #nosec G304 -- reading a .proto the operator pointed this gate at is what
	// the gate is for; there is no untrusted input and no server here.
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	f := &ProtoFile{Path: path, Enums: map[string]bool{}}

	var current *Message
	for i, line := range strings.Split(string(content), "\n") {
		switch {
		case protoPackage.MatchString(line):
			f.Package = protoPackage.FindStringSubmatch(line)[1]

		case protoEnum.MatchString(line):
			f.Enums[protoEnum.FindStringSubmatch(line)[1]] = true

		case protoMessage.MatchString(line):
			if current != nil {
				f.Messages = append(f.Messages, *current)
			}
			current = &Message{
				Name: protoMessage.FindStringSubmatch(line)[1],
				File: path,
				Line: i + 1,
			}

		case strings.HasPrefix(line, "}"):
			if current != nil {
				f.Messages = append(f.Messages, *current)
				current = nil
			}

		case current != nil:
			if field, ok := parseProtoField(line); ok {
				current.Fields = append(current.Fields, field)
			}
		}
	}
	if current != nil {
		return nil, fmt.Errorf("%s: message %q is not closed", path, current.Name)
	}
	return f, nil
}

// ProtoFile is one parsed .proto.
type ProtoFile struct {
	Path     string
	Package  string
	Messages []Message

	// Enums names the enums this file declares. A field of an enum type
	// mirrors as a field of the same type name, which is only knowable by
	// having read the declaration -- a type name alone does not say whether it
	// is an enum, a message, or a typo.
	Enums map[string]bool
}

// parseProtoField reads one field declaration.
func parseProtoField(line string) (Field, bool) {
	m := protoField.FindStringSubmatch(line)
	if m == nil {
		return Field{}, false
	}

	number, err := strconv.Atoi(m[4])
	if err != nil {
		return Field{}, false
	}

	kind := m[2]
	if m[1] == "repeated" {
		kind = "repeated " + kind
	}
	return Field{Name: m[3], Type: kind, Slot: number}, true
}
