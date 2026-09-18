// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// statusError translates a storage or naming error into the gRPC status a
// client can act on.
//
// The mapping is exhaustive over this package's sentinels on purpose. A caller
// distinguishing "no such event" from "malformed name" must not have to match
// strings, which is the same reason the errors are sentinels rather than
// fmt.Errorf values -- and it is why a Store implementation should wrap these
// rather than invent its own.
//
// A nil error maps to nil, so a caller may pass a result unconditionally.
func statusError(err error) error {
	switch {
	case err == nil:
		return nil

	case errors.Is(err, ErrNotFound):
		return status.Error(codes.NotFound, err.Error())

	case errors.Is(err, ErrInvalidName),
		errors.Is(err, ErrInvalidParent),
		errors.Is(err, ErrInvalidPageToken):
		return status.Error(codes.InvalidArgument, err.Error())

	// Cancellation is the client hanging up or its deadline expiring, which is
	// an expected outcome on a long listing rather than a fault of the panel's.
	// Reporting it as Internal would put it in the wrong alert.
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, err.Error())
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, err.Error())

	default:
		// Deliberately not err.Error(). An error this package does not
		// recognise came from a Store it did not write, and its text may carry
		// a connection string, a file path or a row it failed on. The RPC
		// boundary is where that stops; the span the handler opened carries the
		// error itself for whoever is operating the panel.
		return status.Error(codes.Internal, "the event store failed")
	}
}
