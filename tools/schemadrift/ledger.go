// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"regexp"
	"strconv"
	"strings"
)

// Ledger is buffers.lock: the committed record of which proto field number owns
// which target slot.
//
// It exists because the slot is a wire format. A consumer compiled against a
// schema where a field sat at ordinal 5 reads ordinal 5 forever, and a rebuild
// that quietly moved something else there would be reported by nothing -- not
// protoc, not flatc, not the consumer, which simply reads the wrong field. The
// ledger is what turns that into a build failure, and this gate is what checks
// the ledger still matches both schemas.
type Ledger struct {
	// Slots maps a fully qualified message name to its field number -> ordinal
	// assignments.
	Slots map[string]map[int]int

	// Names maps the same to field number -> field name, so a rename that kept
	// the number is reported too.
	Names map[string]map[int]string
}

var (
	ledgerMessage = regexp.MustCompile(`^\s*-\s*message:\s*([A-Za-z0-9_.]+)\s*$`)
	ledgerNumber  = regexp.MustCompile(`^\s*-\s*number:\s*(\d+)\s*$`)
	ledgerOrdinal = regexp.MustCompile(`^\s*ordinal:\s*(\d+)\s*$`)
	ledgerName    = regexp.MustCompile(`^\s*name:\s*(\S+)\s*$`)

	// A new top-level key ends the messages section. Without this the parser
	// reads the enums that follow as though they were the last message's
	// fields, and then reports drift against its own confusion.
	ledgerSection = regexp.MustCompile(`^[a-z][a-z0-9_]*:`)
)

// ParseLedger reads buffers.lock.
//
// It is read line by line rather than through a YAML library so that this gate
// keeps the tools module's dependency list short and cannot be broken by a
// parser upgrade. The file's shape is fixed by the generator that writes it.
func ParseLedger(path string) (*Ledger, error) {
	// #nosec G304 -- reading the ordinal ledger the operator pointed this gate at is what
	// the gate is for; there is no untrusted input and no server here.
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	l := &Ledger{Slots: map[string]map[int]int{}, Names: map[string]map[int]string{}}

	var message string
	var number int
	for _, line := range strings.Split(string(content), "\n") {
		switch {
		case ledgerSection.MatchString(line):
			message = ""

		case ledgerMessage.MatchString(line):
			message = ledgerMessage.FindStringSubmatch(line)[1]
			l.Slots[message] = map[int]int{}
			l.Names[message] = map[int]string{}

		case message == "":
			// Header comments and the version line.

		case ledgerNumber.MatchString(line):
			number, _ = strconv.Atoi(ledgerNumber.FindStringSubmatch(line)[1])

		case ledgerOrdinal.MatchString(line):
			ordinal, _ := strconv.Atoi(ledgerOrdinal.FindStringSubmatch(line)[1])
			l.Slots[message][number] = ordinal

		case ledgerName.MatchString(line):
			l.Names[message][number] = ledgerName.FindStringSubmatch(line)[1]
		}
	}
	return l, nil
}

// Message reports the ledger's record for a message, and whether it has one.
func (l *Ledger) Message(name string) (slots map[int]int, names map[int]string, ok bool) {
	slots, ok = l.Slots[name]
	return slots, l.Names[name], ok
}
