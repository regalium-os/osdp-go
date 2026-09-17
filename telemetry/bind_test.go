package telemetry_test

import (
	"context"
	"errors"
	"testing"

	"github.com/regalium-os/osdp-go/telemetry"
)

// fakeSpan mirrors the telemetry-go SDK's tracing.Span: a concrete, unexported
// type reached only through its methods.
type fakeSpan struct {
	name  string
	attrs map[string]any
	err   error
	ended bool
}

func (s *fakeSpan) End()                           { s.ended = true }
func (s *fakeSpan) SetAttribute(key string, v any) { s.attrs[key] = v }
func (s *fakeSpan) SetError(err error)             { s.err = err }

// fakeTracing mirrors the SDK's *tracing.Tracing, including the variadic data
// parameter that carries tagged structs.
type fakeTracing struct{ spans []*fakeSpan }

func (t *fakeTracing) Start(
	ctx context.Context, name string, data ...any,
) (context.Context, *fakeSpan) {
	s := &fakeSpan{name: name, attrs: map[string]any{}}
	for i, d := range data {
		s.attrs["data"] = d
		_ = i
	}
	t.spans = append(t.spans, s)
	return ctx, s
}

// TestBindInfersTheSDKSpanType is a compile-time assertion as much as a runtime
// one: if Bind's type inference stops working against a Start method value
// shaped like the SDK's, this file fails to build.
func TestBindInfersTheSDKSpanType(t *testing.T) {
	sdk := &fakeTracing{}
	tracer := telemetry.Bind(sdk.Start)

	ctx := telemetry.ContextWithTracer(context.Background(), tracer)

	type payload struct {
		Address uint8 `telemetry:"trace:osdp.device.address"`
	}
	_, span := telemetry.Start(ctx, "osdp.frame.decode", payload{Address: 0x7F})
	span.SetAttribute(telemetry.AttrSequence, 2)
	span.RecordError(nil) // must be ignored
	span.End()

	if len(sdk.spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(sdk.spans))
	}
	got := sdk.spans[0]
	if got.name != "osdp.frame.decode" {
		t.Errorf("span name = %q, want %q", got.name, "osdp.frame.decode")
	}
	if got.attrs[telemetry.AttrSequence] != 2 {
		t.Errorf("sequence attribute = %v, want 2", got.attrs[telemetry.AttrSequence])
	}
	if _, ok := got.attrs["data"]; !ok {
		t.Error("tagged payload was not forwarded to the SDK")
	}
	if got.err != nil {
		t.Errorf("RecordError(nil) recorded %v, want nothing", got.err)
	}
	if !got.ended {
		t.Error("span was not ended")
	}
}

func TestBindForwardsRealErrors(t *testing.T) {
	sdk := &fakeTracing{}
	ctx := telemetry.ContextWithTracer(context.Background(), telemetry.Bind(sdk.Start))

	_, span := telemetry.Start(ctx, "osdp.frame.decode")
	want := errors.New("bad crc")
	span.RecordError(want)
	span.End()

	if !errors.Is(sdk.spans[0].err, want) {
		t.Errorf("recorded %v, want %v", sdk.spans[0].err, want)
	}
}

// TestNopIsTheDefault: a consumer who configures nothing must pay nothing and
// must not crash.
func TestNopIsTheDefault(t *testing.T) {
	ctx, span := telemetry.Start(context.Background(), "osdp.frame.decode")
	if ctx == nil {
		t.Fatal("Start returned a nil context")
	}
	span.SetAttribute("k", "v")
	span.RecordError(errors.New("ignored"))
	span.End()
	span.End() // End must be safe to call twice
}
