// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package telemetry

import "context"

// spanAPI is the span surface the telemetry-go SDK provides.
//
// It is declared structurally, not by importing the SDK, because the SDK's span
// type lives in an internal package and cannot be named from outside it. That
// turns out to be a feature: matching the shape instead of the type means this
// package keeps its zero dependencies, and any tracer with the same three
// methods binds just as well.
type spanAPI interface {
	End()
	SetAttribute(key string, value any)
	SetError(err error)
}

// Bind adapts a telemetry-go tracer to this package's Tracer.
//
// Pass the Start method value rather than the SDK object, so the compiler can
// infer the span type that the SDK does not export:
//
//	p, err := tel.New().WithService("panel", "1.0.0").WithTracing().Build()
//	if err != nil {
//	    return err
//	}
//	defer p.Close()
//
//	ctx = telemetry.ContextWithTracer(ctx, telemetry.Bind(p.Tracing.Start))
//
// From that point every layer boundary in osdp-go opens spans on the
// application's pipeline, and the `telemetry:"trace:..."` struct tags on the
// protocol types become span attributes without another line of wiring.
func Bind[S spanAPI](
	start func(context.Context, string, ...any) (context.Context, S),
) Tracer {
	return bound[S]{start}
}

type bound[S spanAPI] struct {
	start func(context.Context, string, ...any) (context.Context, S)
}

// Start forwards data unchanged, which is what makes the struct tags work: the
// SDK scans each value for tagged fields and turns them into span attributes,
// so an attribute is never named twice.
func (b bound[S]) Start(
	ctx context.Context, name string, data ...any,
) (context.Context, Span) {
	ctx, span := b.start(ctx, name, data...)
	return ctx, boundSpan{span}
}

type boundSpan struct{ span spanAPI }

func (s boundSpan) End()                           { s.span.End() }
func (s boundSpan) SetAttribute(key string, v any) { s.span.SetAttribute(key, v) }

// RecordError forwards to SetError, which both records the error and sets the
// span status. A nil error is ignored, so callers may pass a result directly.
func (s boundSpan) RecordError(err error) {
	if err == nil {
		return
	}
	s.span.SetError(err)
}
