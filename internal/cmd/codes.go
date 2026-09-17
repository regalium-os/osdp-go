// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package cmd

// Code is a command or reply code, the octet that follows the header and any
// security block. SIA OSDP v2.2.2 §6.
//
// Commands and replies occupy disjoint ranges, so one type carries both: what
// distinguishes them is the direction of the frame, not the value.
type Code byte

// Commands, sent by the control panel.
const (
	Poll         Code = 0x60 // osdp_POLL
	ID           Code = 0x61 // osdp_ID
	Cap          Code = 0x62 // osdp_CAP
	LStat        Code = 0x64 // osdp_LSTAT
	IStat        Code = 0x65 // osdp_ISTAT
	OStat        Code = 0x66 // osdp_OSTAT
	RStat        Code = 0x67 // osdp_RSTAT
	Out          Code = 0x68 // osdp_OUT
	LED          Code = 0x69 // osdp_LED
	Buz          Code = 0x6A // osdp_BUZ
	Text         Code = 0x6B // osdp_TEXT
	ComSet       Code = 0x6E // osdp_COMSET
	BioRead      Code = 0x73 // osdp_BIOREAD
	BioMatch     Code = 0x74 // osdp_BIOMATCH
	KeySet       Code = 0x75 // osdp_KEYSET
	Chlng        Code = 0x76 // osdp_CHLNG
	SCrypt       Code = 0x77 // osdp_SCRYPT
	ACURxSize    Code = 0x7B // osdp_ACURXSIZE
	FileTransfer Code = 0x7C // osdp_FILETRANSFER
	MFG          Code = 0x80 // osdp_MFG
	Abort        Code = 0xA2 // osdp_ABORT
	PIVData      Code = 0xA3 // osdp_PIVDATA
	GenAuth      Code = 0xA4 // osdp_GENAUTH
	CRAuth       Code = 0xA5 // osdp_CRAUTH
	KeepActive   Code = 0xA7 // osdp_KEEPACTIVE
)

// Replies, sent by a peripheral device.
const (
	ACK      Code = 0x40 // osdp_ACK
	NAK      Code = 0x41 // osdp_NAK
	PDID     Code = 0x45 // osdp_PDID
	PDCap    Code = 0x46 // osdp_PDCAP
	LStatR   Code = 0x48 // osdp_LSTATR
	IStatR   Code = 0x49 // osdp_ISTATR
	OStatR   Code = 0x4A // osdp_OSTATR
	RStatR   Code = 0x4B // osdp_RSTATR
	Raw      Code = 0x50 // osdp_RAW
	Keypad   Code = 0x53 // osdp_KEYPAD
	Com      Code = 0x54 // osdp_COM
	BioReadR Code = 0x57 // osdp_BIOREADR
	CCrypt   Code = 0x76 // osdp_CCRYPT
	RMACI    Code = 0x78 // osdp_RMAC_I
	MFGReply Code = 0x90 // osdp_MFGREP
	Busy     Code = 0x79 // osdp_BUSY
)

// IsCommand reports whether c is in the command range.
//
// osdp_CHLNG and osdp_CCRYPT share the value 0x76. They are told apart by
// direction, which is why this is a range test and not a lookup: a reply frame
// carrying 0x76 is osdp_CCRYPT, and a command frame carrying it is osdp_CHLNG.
func (c Code) IsCommand() bool { return c >= 0x60 }

// Name returns the specification's mnemonic, given the direction.
func (c Code) Name(isReply bool) string {
	if isReply {
		if n, ok := replyNames[c]; ok {
			return n
		}
		return "osdp_UNKNOWN_REPLY"
	}
	if n, ok := commandNames[c]; ok {
		return n
	}
	return "osdp_UNKNOWN_CMD"
}
