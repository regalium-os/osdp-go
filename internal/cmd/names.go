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
