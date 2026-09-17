# CLAUDE.md

Conventions for working in this repository. These are enforced by
`internal/arch` and by CI, not left to reviewer memory.

`osdp-go` is a pure-Go implementation of SIA OSDP v2.2.2 / IEC 60839-11-5. It is
a **library and a runtime**, not an application: it is imported by control-panel
software that must keep a bus alive for years. Design for that caller.

## Non-negotiables

Each of these fails CI. Run `just arch` before you think you are done.

| Rule | Enforced by |
| --- | --- |
| No hand-written Go file exceeds 200 lines | `internal/arch/size_test.go` |
| Imports only ever point inward | `internal/arch/arch_test.go` |
| The pure core performs no I/O | `internal/arch/purity_test.go` |
| The standard AES-128 suite stays reachable | `internal/arch/purity_test.go` |
| `.fbs` has not drifted from the `.proto` it mirrors | `tools/schemadrift` |
| `.proto` satisfies Google AIP, strictly | `.github/workflows/api-lint.yml` |
| BUILD files match their sources | `.github/workflows/ci.yml` |

## The 200-line limit

The limit is about comprehension. A file that fits in two screens can be held in
the head while reading it, and a protocol codec nobody can hold in their head is
a codec where the off-by-one lives.

When a file approaches the ceiling, split it along a seam that already exists:
encode from decode, one command family from another, the state machine from its
transition table. Name the halves for what they contain.

**Never reclaim lines by deleting documentation.** If the choice is a 210-line
file with its comments or a 190-line file without them, the file was already
doing two jobs — split it.

Generated code under `protobuf/generated/` and `flatbuffers/generated/` is
exempt. Test files are not.

## Documentation

Every package has a `doc.go` stating its **layer contract**: what it owns, what
it must not do, which layers it may import, and which spans it opens. Read the
existing ones before adding a package; match their shape.

Every exported identifier has a doc comment. For a protocol library that means:

- **Cite the specification.** `// Per SIA OSDP v2.2.2 §5.7, the sequence number
  wraps 1→2→3→1; zero is reserved for a device that has lost sync.` A reader
  fixing a bug at 3am needs the clause, not a paraphrase.
- **Say why, not what.** `crc = 0x1D0F` is self-evident; *why that initial value
  and not 0xFFFF* is not.
- **Document byte-slice ownership.** Whether a function retains, copies or
  mutates the slice it is handed is the single most common source of protocol
  bugs. State it on every function that takes `[]byte`.
- **Document concurrency** on every exported type: safe for concurrent use, or
  not, and what the caller must do.
- **Record real-world quirks with a fixture.** A comment saying a vendor
  misreports a capability must point at the fixture that proves it.

## Library API

For everything an importer calls directly.

- `context.Context` is the first parameter of anything that can block, fail, or
  open a span. That is nearly everything — it is what keeps cancellation flowing
  through a stack talking to hardware.
- **No package-level mutable state. No `init()` side effects. No global
  registries.** A process driving two buses with different configurations must
  not have them interfere.
- Constructors take required arguments positionally and optional ones as
  functional options: `New(addr Address, opts ...Option) (*T, error)`. Adding an
  option must never break a caller.
- Return sentinel errors matched with `errors.Is`, or typed errors carrying the
  offending bytes. A caller distinguishing "bad CRC" from "short read" must not
  resort to string matching.
- Accept interfaces, return concrete types. Interfaces are declared by the
  consumer at the boundary — that is why `transport` holds the port definition
  and `driver` merely satisfies it.
- Make zero values useful where it costs nothing, and say so.
- **Never panic.** Not on malformed input, not on a short buffer. Malformed
  input is the normal case on a wire shared with noise.

## Runtime API

For the long-lived components: `bus`, `driver`, `provider`.

- Lifecycle is explicit: `New(...)` constructs, `Run(ctx) error` blocks until the
  context is cancelled or a fatal error occurs. **`New` never starts a
  goroutine.** A caller must be able to construct, inspect and discard.
- If a component spawns goroutines, it owns their shutdown, and `Run` does not
  return until they have stopped. No orphans.
- `Close` is idempotent and safe to call after a failed `Run`.
- Time is injected. The core never calls `time.Now` or `time.Sleep`; it returns
  a decision and a deadline, and the driver acts on it. This is what lets a full
  online/offline/resync scenario run as a table test with a fake clock.
- Channel direction is part of the signature, and the doc comment says who
  closes it and what happens when the reader is slow. Backpressure is a design
  decision, never an accident.
- A device going offline is an expected event, not an error return. Report it
  through the event stream; reserve errors for conditions the caller must fix.

## Layering

```
frame ← cmd ← bus → transport ← driver
  ↑      ↑                        ↑
secure ──┘              provider ─┘
```

`telemetry` is importable everywhere. Everything else obeys the table in
`internal/arch/arch_test.go`, which is the authority — update it there, never by
working around it.

Vendor differences live **only** in `osdp_MFG` extension commands and `osdp_CAP`
negotiation. A provider never reimplements framing. If a vendor appears to need
its own frame codec, that is a quirk flag in `frame` with a fixture proving it.

## Telemetry

Spans open at layer boundaries and are named `osdp.<layer>.<operation>` —
`osdp.frame.decode`, `osdp.secure.seal`, `osdp.driver.write`. Tracers resolve
from the context; there is no package-level tracer.

**Never record key material, session keys, or challenge nonces as span
attributes.** Card numbers are personal data — record the format and bit count,
not the credential.

## Schemas

`protobuf/` is the source of truth: resource-oriented per Google AIP, standard
methods, hierarchical names like `devices/{device}/events/{event}`. House style
is `{device}`, not `{device_id}`.

`flatbuffers/` mirrors only the poll-cycle payloads. A mirrored table declares
its source in the schema itself so the two cannot come apart:

```
/// mirrors: osdp.poll.v1.CardRead
table CardRead { ... }
```

Adding a mirrored table without implementing its comparison turns CI red on
purpose. That is the design.

## Tests and phase gates

Every phase ends with a fixture corpus: hex-dumped OSDP traffic decoded byte for
byte. **Do not start a phase before the previous corpus passes.**

Decoding must be lossless: decode then re-encode reproduces the input exactly,
including a wrong CRC, so malformed traffic can be inspected rather than
silently normalised.

A test that cannot see what it is meant to check must **fail**, not pass. Every
corpus walker asserts it found files. This repository has already shipped one
test that passed on an empty directory; do not ship the second.

## Commands

```sh
just arch       # architecture, purity and file-size conformance — run this
just test       # go test ./...
just fixtures   # the phase gate
just tidy       # regenerate BUILD files after moving code
just gen all    # codegen, then the drift gate
```

## Do not

- Add a dependency to the root `go.mod`. The library's dependency footprint is a
  feature; tooling belongs in `tools/`, which is a separate module.
- Weaken a test to make it pass. Fix the code, or change the rule in
  `internal/arch` deliberately and say why in the commit.
- Replace the standard AES-128 cipher suite. A second suite is registered
  *beside* it, never instead of it.
- Commit or push unless asked.
