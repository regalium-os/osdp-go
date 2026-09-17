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
| **No direct OpenTelemetry import, anywhere** | `internal/arch/deps_test.go` |
| **No cgo, anywhere** | `internal/arch/deps_test.go` + CI `purity` job |
| Clean under the race detector | CI `race` job |
| Clean under golangci-lint | `.golangci.yml`, CI `lint` job |
| Imports only ever point inward | `internal/arch/arch_test.go` |
| The pure core performs no I/O | `internal/arch/purity_test.go` |
| The standard AES-128 suite stays reachable | `internal/secure/suite_test.go` |
| `.fbs` has not drifted from the `.proto` it mirrors | `tools/schemadrift` |
| `.proto` satisfies Google AIP, strictly | `.github/workflows/api-lint.yml` |
| BUILD files match their sources | `.github/workflows/ci.yml` |
| Every source file carries the Apache-2.0 notice | `internal/arch/license_test.go` |

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

Generated code under `protobuf/generated/` is exempt — the mirrored schemas and
their Go live there too. Test files are not exempt.

## Licensing

The project is Apache-2.0. Every hand-written source file opens with the notice
and its SPDX identifier, in that file kind's comment syntax:

```go
// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0
```

Leave a blank line between the notice and whatever follows. In Go a comment
block touching `package x` **becomes** that package's doc comment, and a
copyright header is not documentation.

This covers `.go`, `.proto`, `BUILD.bazel`, `.yaml`, `.yml`, `.just`, `justfile`,
`.bazelrc`, and the `.hex` and `.kat` fixture corpora. It does not cover
generated output, lock files, or `go.mod` — nothing there is authored here.
`internal/arch/license_test.go` is the authority; add a file kind there rather
than working around it.

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
internal/frame ← internal/cmd ← internal/bus → internal/transport ← internal/driver
       ↑                ↑                                                  ↑
internal/secure ────────┘                              internal/provider ──┘
```

Every protocol layer is under `internal/`. The public surface is `package osdp`
at the repository root, plus `telemetry`, and nothing else.

`telemetry` is importable everywhere. Everything else obeys the table in
`internal/arch/arch_test.go`, which is the authority — update it there, never by
working around it.

### Re-export by alias, never by wrapper

The root facade re-exports internal types with `type Frame = frame.Frame`. Keep
it that way. An alias is the same type, so a consumer's `Transport` or
`CipherSuite` still satisfies the interfaces declared internally. A wrapper type
would compile and then silently close every extension point — third parties
could no longer implement a transport or add a cipher suite, which is most of
what the hexagon is for.

Adding to the public surface is a deliberate act. If something does not need to
be public, leave it internal.

Vendor differences live **only** in `osdp_MFG` extension commands and `osdp_CAP`
negotiation. A provider never reimplements framing. If a vendor appears to need
its own frame codec, that is a quirk flag in `frame` with a fixture proving it.

## Pure Go, no cgo

**Never `import "C"`. Never add a dependency that pulls cgo transitively.**

This is not preference. These panels are ARM, and pure Go is what makes
`GOOS=linux GOARCH=arm go build` a one-flag operation and keeps a static binary
static. CI builds for linux/arm, linux/arm64, linux/amd64, windows/amd64 and
darwin/arm64 on every run.

Two gates enforce it, because they catch different things:
`internal/arch/deps_test.go` rejects an `import "C"` in the source, and the CI
`purity` job — which runs with cgo off — rejects `runtime/cgo` appearing in the
dependency graph, which no amount of reading the source would reveal.

`CGO_ENABLED=0` everywhere, with exactly one exception: the `race` job, and
`just race` to match it. Go's race detector builds without cgo only on darwin;
on linux/amd64 and linux/arm64 the toolchain refuses with `-race requires cgo`,
and on linux/arm the detector does not exist at all. **Know this before trusting
a local run**: `CGO_ENABLED=0 go test -race ./...` passes on a Mac and cannot
pass on the Linux runner, which is exactly how this reached CI red.

The exception costs nothing, because the ban is about what ships rather than how
a test binary is instrumented, and neither gate above runs inside that job. A
dependency needing cgo still fails in `purity`.

If something seems to need cgo — a serial library, say — the answer is a
separate module the application opts into, not a dependency here.

## Telemetry

**Never import `go.opentelemetry.io/...`. Not in the core, not in an adapter,
not in a test, not "just for the attribute type".** This is enforced by
`internal/arch/deps_test.go` and is not a style preference.

Observability goes through the the-protobuf-project telemetry SDK
(`telemetry-go`), which owns the OpenTelemetry integration, the exporter
lifecycle and the attribute convention. Two things deciding how a span is made
is how instrumentation rots.

The seam is the `telemetry` package — `Tracer` and `Span` are three methods of
standard library, and `telemetry.Bind` attaches the SDK to them:

```go
ctx = telemetry.ContextWithTracer(ctx, telemetry.Bind(p.Tracing.Start))
```

`Bind` takes the SDK's `Start` method value rather than the SDK object, because
the SDK's span type is in an internal package and cannot be named. Matching the
shape structurally instead of importing the type is why `telemetry` — and the
whole root module — still has **zero dependencies**.

Spans open at layer boundaries and are named `osdp.<layer>.<operation>` —
`osdp.frame.decode`, `osdp.secure.seal`, `osdp.driver.write`. Tracers travel on
the context; there is no package-level tracer.

### Attributes are struct tags, not calls

Declare an attribute next to the field it describes:

```go
type Frame struct {
    Address  Address `telemetry:"trace:osdp.device.address"`
    Sequence uint8   `telemetry:"trace:osdp.frame.sequence"`
    Data     []byte  // no tag: never recorded
}
```

Then pass the value: `telemetry.Start(ctx, "osdp.frame.decode", f)`.

A struct tag is an inert string, so this costs the dependency-free core nothing
while giving a telemetry-go application full attributes automatically.

**An untagged field is never recorded, and that is the safety mechanism.** This
library handles credentials. A card number has no tag, so it cannot leak into a
trace through a well-meaning `SetAttribute` call added in a hot path — there is
no such call to add. Record the format and the bit count. Never the credential,
the key material, the session key, or the challenge nonce.

## Schemas

`protobuf/` is the source of truth: resource-oriented per Google AIP, standard
methods, hierarchical names like `devices/{device}/events/{event}`. House style
is `{device}`, not `{device_id}`.

`protobuf/generated/flatbuffers/` mirrors only the poll-cycle payloads, and is
generated by `buffers` from the same descriptor set as the Go types — never
hand-written. The mirror declares its source in the file itself, so the two
cannot come apart:

```
// source: osdp/event/v1/payloads.proto
namespace osdp.event.v1;
```

`tools/schemadrift` checks **three** artefacts against each other on every CI
run, not two: the `.proto` says what a field is and which number it holds,
`buffers.lock` says which target ordinal that number was committed to, and the
`.fbs` says where the mirror put it. Any two agreeing while the third differs
tells you which one moved.

It fails when it finds no schemas at all. A gate that reports success because it
could not find what it was meant to inspect is worse than no gate, because it is
believed — and this repository shipped exactly that once already.

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
just lint       # golangci-lint, as CI runs it
just race       # the suite under the race detector
just purity     # confirm nothing pulled cgo into the graph
just test       # go test ./...
just fixtures   # the phase gate
just tidy       # regenerate BUILD files after moving code
just gen all    # codegen, then the drift gate
```

## Do not

- Add a dependency to the root `go.mod`. **It currently has none, and that is
  the target state.** The core is standard library only; tooling belongs in
  `tools/`, a separate module.
- Import OpenTelemetry directly. See Telemetry above; use `telemetry.Tracer`.
- Weaken a test to make it pass. Fix the code, or change the rule in
  `internal/arch` deliberately and say why in the commit.
- Replace the standard AES-128 cipher suite. A second suite is registered
  *beside* it, never instead of it.
- Commit or push unless asked.
