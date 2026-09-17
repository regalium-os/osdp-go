// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package cmd

// StatusKind says which kind of contact a status report concerns.
//
// The values match osdp.event.v1.StatusKind so that a record of a change and
// the change itself cannot disagree about what changed.
type StatusKind uint8

const (
	// StatusUnspecified is the zero value and never reported.
	StatusUnspecified StatusKind = 0

	// StatusInput is a monitored input: door position, request-to-exit, a
	// glassbreak. These are what an access-control system is actually watching.
	StatusInput StatusKind = 1

	// StatusOutput is a controlled output, such as a strike relay. Reported
	// back so a panel can tell what a device did with the command it was sent.
	StatusOutput StatusKind = 2

	// StatusTamper is the device's tamper switch, from osdp_LSTATR. A reader
	// being pulled off a wall is the one status change nobody should be able
	// to miss.
	StatusTamper StatusKind = 3

	// StatusPower is the device's power supply, from osdp_LSTATR: a reader
	// running on backup power is one that is about to stop answering.
	StatusPower StatusKind = 4

	// StatusLocal is a reader's own status, from osdp_RSTATR -- whether the
	// reader head is connected and untampered, which is distinct from the
	// device the head is attached to.
	StatusLocal StatusKind = 5
)

// String implements fmt.Stringer.
func (k StatusKind) String() string {
	switch k {
	case StatusInput:
		return "input"
	case StatusOutput:
		return "output"
	case StatusTamper:
		return "tamper"
	case StatusPower:
		return "power"
	case StatusLocal:
		return "local"
	default:
		return "unspecified"
	}
}

// StatusChange is one contact reported at one state.
//
// Active is what the specification calls the abnormal or asserted state: a
// contact closed, a tamper triggered, an output energised, a power supply on
// backup. Which of those is a problem depends on how the door is wired, and
// that is the panel's business rather than this library's.
type StatusChange struct {
	Kind   StatusKind
	Index  int
	Active bool
}

// ParseInputStatus decodes an osdp_ISTATR payload: one octet per monitored
// input, in device order. data is read and not retained.
func ParseInputStatus(data []byte) ([]StatusChange, error) {
	return parseContacts(StatusInput, data)
}

// ParseOutputStatus decodes an osdp_OSTATR payload: one octet per controlled
// output. data is read and not retained.
func ParseOutputStatus(data []byte) ([]StatusChange, error) {
	return parseContacts(StatusOutput, data)
}

// ParseReaderStatus decodes an osdp_RSTATR payload: one octet per reader, zero
// meaning the reader is present and well.
//
// The specification distinguishes "not connected" from "tampered", which this
// reduces to active or not. A panel that needs the distinction has the raw
// reply; what it must not miss is that something is wrong with the reader.
func ParseReaderStatus(data []byte) ([]StatusChange, error) {
	return parseContacts(StatusLocal, data)
}

// parseContacts turns one-octet-per-contact into changes of one kind.
//
// An empty payload is not an error: a device with no inputs answers osdp_ISTAT
// with nothing to say, and that is a true answer.
func parseContacts(kind StatusKind, data []byte) ([]StatusChange, error) {
	out := make([]StatusChange, 0, len(data))
	for i, v := range data {
		out = append(out, StatusChange{Kind: kind, Index: i, Active: v != 0})
	}
	return out, nil
}

// LocalStatusSize is the fixed osdp_LSTATR payload length: tamper, then power.
const LocalStatusSize = 2

// ParseLocalStatus decodes an osdp_LSTATR payload into its two distinct
// concerns, which the wire carries together and a panel acts on separately.
//
// data is read and not retained.
func ParseLocalStatus(data []byte) ([]StatusChange, error) {
	if len(data) < LocalStatusSize {
		return nil, ErrShortPayload
	}
	return []StatusChange{
		{Kind: StatusTamper, Index: 0, Active: data[0] != 0},
		{Kind: StatusPower, Index: 0, Active: data[1] != 0},
	}, nil
}

// Status commands carry no payload: each asks a device to report one kind of
// contact. A device with something to say also reports unsolicited, in answer
// to an ordinary poll, which is the usual way a panel learns a door opened.
func LocalStatusCommand() Message  { return Message{Code: LStat} }
func InputStatusCommand() Message  { return Message{Code: IStat} }
func OutputStatusCommand() Message { return Message{Code: OStat} }
func ReaderStatusCommand() Message { return Message{Code: RStat} }
