package telemetry

import (
	"reflect"
	"strings"
)

// Apply reads `telemetry:"trace:<name>"` tags from v and sets each tagged field
// on span.
//
// Start already does this for values passed at span creation, by handing them to
// the SDK. Apply exists for the case where the values are not known until after
// the span has begun -- a decoder cannot describe a frame it has not parsed yet,
// and starting the span afterwards would exclude the parse from its duration.
//
// Using the same tags for both paths keeps one declaration of what is safe to
// record. An untagged field is not recorded here either.
//
// Nested structs are traversed. A nil v, a nil pointer, or a non-struct is
// ignored rather than treated as an error: telemetry must never be the reason a
// protocol operation fails.
func Apply(span Span, v any) {
	if span == nil || v == nil {
		return
	}
	apply(span, reflect.ValueOf(v), 0)
}

// maxDepth bounds traversal so a cyclic structure cannot hang a decode.
const maxDepth = 8

func apply(span Span, rv reflect.Value, depth int) {
	if depth > maxDepth {
		return
	}
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return
	}

	rt := rv.Type()
	for i := range rt.NumField() {
		field, value := rt.Field(i), rv.Field(i)
		if !value.CanInterface() {
			continue
		}

		for _, part := range strings.Fields(field.Tag.Get(TagKey)) {
			if name, found := strings.CutPrefix(part, TracePrefix); found {
				span.SetAttribute(name, value.Interface())
			}
		}

		switch value.Kind() {
		case reflect.Struct:
			apply(span, value, depth+1)
		case reflect.Pointer:
			if !value.IsNil() && value.Elem().Kind() == reflect.Struct {
				apply(span, value, depth+1)
			}
		}
	}
}
