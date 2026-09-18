// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"fmt"

	eventpbv1 "github.com/regalium-os/osdp-go/protobuf/generated/go/event/v1/eventpbv1"
	"github.com/regalium-os/osdp-go/telemetry"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ListEvents returns a page of a device's events, newest first, or of every
// device's when parent is "devices/-".
//
// # Paging
//
// page_size is advisory: zero takes the service default, more than the cap
// takes the cap, and the response says how many actually came back. Per
// AIP-158 that is a cap rather than a refusal, because a client that asked for
// too many has no way to act on an error and every way to act on a
// next_page_token.
//
// A negative page_size is refused. It is not a client asking for a large page,
// it is a client with a bug, and the schema's own bound says gte 0.
//
// next_page_token is empty exactly when the page just returned was the last
// one. A client that keeps paging while a token comes back will terminate; one
// that pages until it sees fewer events than it asked for may not, because a
// store is entitled to return fewer.
func (s *EventService) ListEvents(
	ctx context.Context, req *eventpbv1.ListEventsRequest,
) (*eventpbv1.ListEventsResponse, error) {
	device, err := ParseParent(req.GetParent())
	if err != nil {
		return nil, statusError(fmt.Errorf("%w: parent %q", err, req.GetParent()))
	}
	if refusal := checkUnsupported(req); refusal != nil {
		return nil, refusal
	}
	size, err := s.pageSize(req.GetPageSize())
	if err != nil {
		return nil, err
	}

	ctx, span := telemetry.Start(ctx, "osdp.service.list_events",
		listTrace{Device: device, PageSize: size})
	defer span.End()

	page, err := s.store.List(ctx, Query{
		Parent:    req.GetParent(),
		PageSize:  size,
		PageToken: req.GetPageToken(),
	})
	if err != nil {
		span.RecordError(err)
		return nil, statusError(err)
	}

	return &eventpbv1.ListEventsResponse{
		Events:        page.Events,
		NextPageToken: page.NextPageToken,
	}, nil
}

// pageSize resolves the requested size against this service's default and cap.
func (s *EventService) pageSize(requested int32) (int, error) {
	switch {
	case requested < 0:
		return 0, status.Errorf(codes.InvalidArgument,
			"page_size must not be negative, got %d", requested)
	case requested == 0:
		return s.defaultPageSize, nil
	case int(requested) > s.maxPageSize:
		return s.maxPageSize, nil
	default:
		return int(requested), nil
	}
}

// checkUnsupported refuses the request fields no Store here implements.
//
// Unimplemented rather than InvalidArgument: the request is well formed, and
// reporting it as malformed would send somebody hunting for a syntax error that
// is not there. Refusing at all rather than ignoring, because a client filtering
// for security violations and receiving every card read instead has been given
// a wrong answer that looks like a right one -- which on an audit API is worse
// than an error.
func checkUnsupported(req *eventpbv1.ListEventsRequest) error {
	if f := req.GetFilter(); f != "" {
		return status.Errorf(codes.Unimplemented,
			"filter is not implemented; got %q, only the empty filter is served", f)
	}
	if o := req.GetOrderBy(); o != "" && o != defaultOrderBy {
		return status.Errorf(codes.Unimplemented,
			"order_by is not implemented; got %q, only %q is served", o, defaultOrderBy)
	}
	return nil
}

// listTrace is what the list_events span records.
//
// The resolved page size, not the requested one: what the panel was asked to
// assemble is the number that explains the latency. No field here can reach an
// event payload, so no credential can reach a trace through it.
type listTrace struct {
	Device   string `telemetry:"trace:osdp.device.name"`
	PageSize int    `telemetry:"trace:osdp.page.size"`
}
