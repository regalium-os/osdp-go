// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package telemetry

import "context"

// ScopeName identifies this library in emitted telemetry.
const ScopeName = "github.com/regalium-os/osdp-go"

// Span is the span surface this library uses.
//
// It is deliberately three methods wide. Defining the seam here, rather than
// importing a tracing library into the core, is what keeps osdp-go free of
// dependencies and lets the application own the observability stack. It also
// accommodates an SDK whose span type is unexported, which telemetry-go's is.
// The core layers only ever see this interface.
type Span interface {
	// End closes the span. Calling End more than once must be harmless.
	End()

	// SetAttribute records one attribute on the span.
	//
	// Never pass key material, session keys, challenge nonces, or a raw
	// credential. Record the format and bit count of a card read, not the
	// card number.
	SetAttribute(key string, value any)

	// RecordError marks the span as failed and attaches err. A nil err is
	// ignored, so callers may pass the result of an operation unconditionally.
	RecordError(err error)
}

// Tracer opens spans. A nil Tracer is never valid; use Nop instead.
type Tracer interface {
	// Start opens a span and returns a context carrying it. The caller must
	// call End on the returned Span, conventionally with defer.
	//
	// Any values passed as data are scanned for `telemetry:"trace:<name>"`
	// struct tags, and each tagged field becomes a span attribute. See the
	// package documentation for why the tag is the only way a field is
	// recorded.
	Start(ctx context.Context, name string, data ...any) (context.Context, Span)
}

// Nop is a Tracer that records nothing. It is the zero-cost default: a consumer
// who configures no tracing pays one interface call and no allocation.
type Nop struct{}

// Start returns ctx unchanged and a span that discards everything.
func (Nop) Start(ctx context.Context, _ string, _ ...any) (context.Context, Span) {
	return ctx, nopSpan{}
}

type nopSpan struct{}

func (nopSpan) End()                     {}
func (nopSpan) SetAttribute(string, any) {}
func (nopSpan) RecordError(error)        {}

type tracerKey struct{}

// ContextWithTracer returns a context that carries t.
//
// Tracers travel on the context rather than in a package-level variable so that
// a process driving two buses with different telemetry configurations does not
// have them interfere, and so that a library consumer is never required to
// mutate global state to get traces.
func ContextWithTracer(ctx context.Context, t Tracer) context.Context {
	if t == nil {
		return ctx
	}
	return context.WithValue(ctx, tracerKey{}, t)
}

// FromContext returns the Tracer carried by ctx, or Nop if there is none.
func FromContext(ctx context.Context) Tracer {
	if t, ok := ctx.Value(tracerKey{}).(Tracer); ok {
		return t
	}
	return Nop{}
}

// Start opens a span on the Tracer carried by ctx. It is the call every layer
// boundary makes:
//
//	ctx, span := telemetry.Start(ctx, "osdp.frame.decode", f)
//	defer span.End()
func Start(ctx context.Context, name string, data ...any) (context.Context, Span) {
	return FromContext(ctx).Start(ctx, name, data...)
}
