// Package telemetry holds the OpenTelemetry conventions shared by every layer:
// tracer acquisition, span naming, and the attribute keys that describe OSDP
// traffic.
//
// Observability for this project goes through the the-protobuf-project
// telemetry SDK (telemetry-go), never through an OpenTelemetry import. This
// package is the whole seam: Tracer and Span define what the layers need, and
// Bind attaches the SDK to it without importing anything.
//
// # Why this exists at commit one
//
// Spans at layer boundaries are a design constraint, not an observability
// feature bolted on later. A span that wraps frame.Decode forces Decode to have
// a context parameter, which forces the caller to have one, which is what keeps
// cancellation and deadlines flowing through a protocol stack that talks to
// hardware. Retrofitting that later means changing every signature in the core.
//
// # Contract
//
// Nothing here is a package-level global tracer. Tracers are resolved from the
// TracerProvider on the context, so a library consumer who configures nothing
// pays for a no-op recorder and a library consumer who configures a pipeline
// gets full traces without touching osdp-go.
//
// Attribute keys live here so that osdp.device.address means the same thing in a
// driver span and a bus span.
package telemetry
