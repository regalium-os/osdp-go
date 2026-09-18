# Roadmap

Where `osdp-go` is, what is left in it, and what has to exist around it before a
phone in somebody's pocket opens a door.

The last part is the one worth reading carefully, because most of it is not this
repository and some of it cannot be bought with engineering time.

---

## 1. Where we are

Everything below is implemented and gated in CI. "Gated" means a test fails the
build if it regresses, not that somebody checked once.

| Area | State | Evidence |
| --- | --- | --- |
| Wire framing — CRC-16/AUG-CCITT, checksum, mark octets, security blocks | **done** | 18-record hex corpus, byte-exact round trip, 4 fuzz targets |
| Length-field safety — refuses frames the wire cannot describe | **done** | found by fuzzing; failing input committed as a seed |
| Command / reply codec | **done** | per-payload tests, spec clauses cited |
| Capability negotiation (`osdp_CAP`) + vendor reconciliation | **done** | 14-record corpus, claimed-vs-believed kept separate |
| Poll cycle — enrolment, online/offline, resync | **done** | table tests on a fake clock, no hardware |
| Retransmission — an unanswered command is retried, not lost | **done** | repeats the sequence number, so a device cannot act twice |
| `osdp_BUSY` — a device asking for a moment | **done** | command reclaimed, one-cycle backoff so polling continues |
| `MaxMessageSize` — refuse what a device cannot receive | **done** | estimate pinned against real frames, secure overhead included |
| `osdp_COMSET` — commission a device onto its own address | **done** | bus follows the device; duplicate addresses refused |
| Secure Channel — handshake, authenticated + encrypted traffic | **done** | driven against a real `RolePD` session, not a mock |
| Key install (`osdp_KEYSET`) — commission off SCBK-D | **done** | refuses to transmit without a channel, twice |
| Commands — output, LED, buzzer, text, status requests | **done** | wire layouts asserted against the spec |
| Events — card, keypad, status change, NAK, vendor, lifecycle | **done** | status reports become *changes*, not readings |
| Runtime — `Run`/`Close` lifecycle, backpressure, cancellation | **done** | no goroutine outlives `Run` |
| Protobuf domain schema + FlatBuffers mirror + drift gate | **done** | three-way check: proto, ordinal ledger, mirror |
| Conformance gates — layering, purity, file size, licence, cgo | **done** | `internal/arch`, fails loudly on an unreadable tree |
| A runnable panel — demo mode, wire tracing, door release, commissioning | **done** | `examples/panel`; `-demo` needs no hardware |
| Peripheral-device state machine with reply caching | **done** | `internal/pd`; plaintext only |
| `EventService` — Get, List, pagination, bounded store | **done** | `protobuf/service`; in-memory only |

148 hand-written Go files, 60 test files, 4 fuzz targets, 3 fixture corpora,
three modules all built and tested in CI.

---

## 2. Left in this library

### 2.1 Correctness

Both items here are done. Left in place because what they were is worth knowing:
`osdp_BUSY` was read as success, and a device's reported receive buffer was
negotiated and never consulted — an oversized command retried against a silence
forever, looking like a reader that answered polls and nothing else.

### 2.2 Reaching real hardware

| | Notes |
| --- | --- |
| **RS-485 serial driver** | Most readers are RS-485. A serial-to-Ethernet converter works **today** via `DialTCP` — that is the viable path without writing this. Doing it properly means termios ioctls by hand to keep the no-cgo rule, and belongs in a separate opt-in module. |
| ~~`osdp_COMSET`~~ | **done** — moves a device's address and baud, follows the reply rather than the request, refuses a duplicate address. |
| ~~A runnable example~~ | **done** — `examples/panel`, with `-demo` for no hardware, `-trace` for the octets, `-unlock` for the full loop. |

### 2.3 Protocol surface not yet needed

Deliberately unbuilt. Each is real work and none blocks a door opening.

- **File transfer** (`osdp_FILETRANSFER`) and multi-part message reassembly — firmware updates over the bus.
- **Biometrics** (`osdp_BIOREAD` / `osdp_BIOMATCH`).
- **PIV / advanced auth** (`osdp_PIVDATA`, `GENAUTH`, `CRAUTH`) — federal deployments.
- **Peripheral-device runtime** — **substantially done**: `internal/pd` implements the reader's state machine, including the reply cache §5.7 requires so a repeated sequence number replays rather than re-executes. Still to do: the Secure Channel path (a documented seam, not a stub) and wiring it to a port so it can answer a real line.

---

## 3. Left around this library

None of this is OSDP. All of it is between a working bus and a working product.

| | Why |
| --- | --- |
| **`EventService`** | **Done** — `protobuf/service`: `GetEvent`, `ListEvents`, AIP-158 pagination, a `Store` port and a bounded in-memory implementation. `DeviceService` is still unimplemented, and there is **no durable store** — `MemoryStore` promises recency, not retention. |
| **Persistence** | The panel is entirely in-memory. Restart it and every device key, capability set and contact baseline is gone — and a device whose installed key was never written down is a device nobody can talk to. `EventKeyInstalled` exists precisely so an application can persist; nothing consumes it yet. |
| **Credential decision logic** | This library reports that 26 bits arrived at reader 0. Whether that opens the door is the panel application's job: cardholder database, schedules, anti-passback, offline behaviour. |
| **Audit storage** | `protobuf/record` converts events to domain records. Where they go is unbuilt. |
| **Scale** | The poll cycle is tested with up to 3 devices. A real bus is dozens, and RS-485 timing at 9600 baud is the constraint nobody models until it bites. |

---

## 4. The Wallet end goal

### 4.1 Where the credential actually lives

```
Phone (Apple / Google Wallet)
  │   NFC · Apple ECP, Seos, DESFire, ISO 18013-5
  │   or QR presented on screen
  ▼
Reader  ─────────────────────────────  the Wallet integration is HERE
  │   OSDP over RS-485
  ▼
Panel (osdp-go)  ──────────────────── this repository
  │   gRPC  (§3, unbuilt)
  ▼
Backend → your mobile app
```

**OSDP never sees the phone.** A phone tap and a plastic card arrive at the panel
as the same `osdp_RAW` reply, which is why `grep -i wallet` over this repo
returns nothing and should keep returning nothing. That is not a gap — it is the
layering working.

The practical consequence: **"support Apple Wallet" is a reader purchase and a
credential-ecosystem relationship, not a feature of this library.** What this
library owes is to carry whatever the reader reports, faithfully. It already
does — credentials of 26, 37, 56, 128, 200, 1024 and 4096 bits all parse
correctly, so Seos and DESFire lengths are covered.

### 4.2 The three channels, by how hard they actually are

| Channel | Difficulty | What gates it |
| --- | --- | --- |
| **RFID / physical cards** | works today | nothing — this is the baseline |
| **QR on screen** | easy | a reader with a scanner. Both wallets issue barcode passes through public APIs. The reader reports the scan as a card read or an `osdp_MFG` body. |
| **NFC, Android HCE** | moderate | your own Android app emulates a card. No gatekeeper, but it is your app rather than Google Wallet proper. |
| **NFC, Google Wallet passes** | hard | Google's partner programme. |
| **NFC, Apple Wallet** | hardest | **Apple approval is mandatory and not purchasable on demand.** Employee Badge / access credentials require Apple's programme, a reader with ECP firmware, and provisioning through an approved credential provider (HID Origo and similar). No amount of work in this repository shortens it. |

### 4.3 The order that actually gets you to a phone tap

1. **QR first.** It is the only channel with no gatekeeper on either wallet, it
   proves the whole chain end to end — pass issued, scanned, decided, door
   opened — and everything after it reuses that chain unchanged.
2. **Android HCE** next, for a real tap without a partner programme.
3. **Google Wallet, then Apple**, in parallel with the commercial conversations,
   because those have lead times measured in months and no engineering
   substitute.

Start the Apple conversation early if it is a real requirement. It is the long
pole and it is not a technical one.

---

## 5. What I would do next

In order, and each is independently useful:

1. ~~`osdp_BUSY`~~ — **done**.
2. ~~A runnable example against a converter~~ — **done**.
3. **A durable store behind `EventService`** — the service and its `Store` port
   exist; `MemoryStore` loses everything on restart, which is not an audit
   trail. This is now the gap, rather than the service itself.
4. **QR end to end** — the shortest honest path to "a phone opened a door".
5. **A serial driver** — if a converter is not acceptable for the deployment.
