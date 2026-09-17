// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package cmd

// Mnemonics, kept beside the codes they name so a new command cannot be added
// without one. They appear in traces and in error messages, where "osdp_LED"
// saves a reader a trip to the specification that "0x69" does not.
var commandNames = map[Code]string{
	Poll: "osdp_POLL", ID: "osdp_ID", Cap: "osdp_CAP",
	LStat: "osdp_LSTAT", IStat: "osdp_ISTAT", OStat: "osdp_OSTAT",
	RStat: "osdp_RSTAT", Out: "osdp_OUT", LED: "osdp_LED",
	Buz: "osdp_BUZ", Text: "osdp_TEXT", ComSet: "osdp_COMSET",
	BioRead: "osdp_BIOREAD", BioMatch: "osdp_BIOMATCH", KeySet: "osdp_KEYSET",
	Chlng: "osdp_CHLNG", SCrypt: "osdp_SCRYPT", ACURxSize: "osdp_ACURXSIZE",
	FileTransfer: "osdp_FILETRANSFER", MFG: "osdp_MFG", Abort: "osdp_ABORT",
	PIVData: "osdp_PIVDATA", GenAuth: "osdp_GENAUTH", CRAuth: "osdp_CRAUTH",
	KeepActive: "osdp_KEEPACTIVE",
}

var replyNames = map[Code]string{
	ACK: "osdp_ACK", NAK: "osdp_NAK", PDID: "osdp_PDID",
	PDCap: "osdp_PDCAP", LStatR: "osdp_LSTATR", IStatR: "osdp_ISTATR",
	OStatR: "osdp_OSTATR", RStatR: "osdp_RSTATR", Raw: "osdp_RAW",
	Keypad: "osdp_KEYPAD", Com: "osdp_COM", BioReadR: "osdp_BIOREADR",
	CCrypt: "osdp_CCRYPT", RMACI: "osdp_RMAC_I", MFGReply: "osdp_MFGREP",
	Busy: "osdp_BUSY",
}

// Capability mnemonics, kept beside the function codes for the same reason: a
// capability report printed during enrolment is read by a human deciding
// whether the device on the bench is the one on the drawing.
var functionNames = map[Function]string{
	FuncContactStatus:  "osdp_CAP_CONTACT_STATUS_MONITORING",
	FuncOutputControl:  "osdp_CAP_OUTPUT_CONTROL",
	FuncCardDataFormat: "osdp_CAP_CARD_DATA_FORMAT",
	FuncReaderLED:      "osdp_CAP_READER_LED_CONTROL",
	FuncReaderAudible:  "osdp_CAP_READER_AUDIBLE_OUTPUT",
	FuncReaderText:     "osdp_CAP_READER_TEXT_OUTPUT",
	FuncTimeKeeping:    "osdp_CAP_TIME_KEEPING",
	FuncCheckCharacter: "osdp_CAP_CHECK_CHARACTER_SUPPORT",
	FuncCommSecurity:   "osdp_CAP_COMMUNICATION_SECURITY",
	FuncReceiveBuffer:  "osdp_CAP_RECEIVE_BUFFERSIZE",

	FuncCombinedMessage: "osdp_CAP_LARGEST_COMBINED_MESSAGE_SIZE",
	FuncSmartCard:       "osdp_CAP_SMART_CARD_SUPPORT",
	FuncReaders:         "osdp_CAP_READERS",
	FuncBiometrics:      "osdp_CAP_BIOMETRICS",
	FuncSecurePINEntry:  "osdp_CAP_SECURE_PIN_ENTRY",
	FuncOSDPVersion:     "osdp_CAP_OSDP_VERSION",
}
