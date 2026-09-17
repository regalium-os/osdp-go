// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package telemetry

// Span attributes are declared with struct tags rather than written by hand at
// each call site:
//
//	type Frame struct {
//	    Address  Address `telemetry:"trace:osdp.device.address"`
//	    Sequence uint8   `telemetry:"trace:osdp.frame.sequence"`
//	    Data     []byte  // no tag: never recorded
//	}
//
// The format is telemetry-go's, so an application using that SDK gets these
// attributes with no adapter work at all.
//
// # Why tags rather than SetAttribute calls
//
// Declaring the attribute next to the field it describes means the decision
// about what is safe to record lives with the data, not scattered across call
// sites. That matters here: this library handles credentials.
//
// The rule is that an untagged field is never recorded. A card number has no
// tag and therefore cannot leak into a trace by someone adding a well-meaning
// SetAttribute call in a hot path -- there is no such call to add. Record the
// format and the bit count, which are tagged; never the credential.
//
// # Tag syntax
//
//	`telemetry:"trace:osdp.frame.sequence"`
//
// Space-separated parts are allowed; only those prefixed trace: are read as
// span attributes. Nested structs are traversed.
const tagKey = "telemetry"

// TagKey is the struct tag namespace read by adapters.
const TagKey = tagKey

// TracePrefix marks a tag part as a span attribute name.
const TracePrefix = "trace:"
