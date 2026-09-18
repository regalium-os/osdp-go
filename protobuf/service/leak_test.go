// Copyright 2026 RegaliumOS™.
// SPDX-License-Identifier: Apache-2.0

package service_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	eventpbv1 "github.com/regalium-os/osdp-go/protobuf/generated/go/event/v1/eventpbv1"
	"github.com/regalium-os/osdp-go/protobuf/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// errStore is a Store that only fails, so the handlers' error paths can be
// driven without a database that has to be made to break on purpose.
type errStore struct{ err error }

func (s errStore) Append(
	context.Context, string, *eventpbv1.Event,
) (*eventpbv1.Event, error) {
	return nil, s.err
}

func (s errStore) Get(context.Context, string) (*eventpbv1.Event, error) {
	return nil, s.err
}

func (s errStore) List(context.Context, service.Query) (service.Page, error) {
	return service.Page{}, s.err
}

// secret stands for everything a storage error drags along with it: a password,
// a connection string, a row it failed on. The RPC boundary is where that
// stops.
const secret = "hunter2-kR7xQ"

// TestAnUnrecognisedStoreErrorDoesNotReachTheClient.
//
// An error this package does not recognise came from a Store it did not write,
// and it cannot vouch for what the text carries. The operator gets it through
// the span the handler opened; the peer gets a code and nothing else.
func TestAnUnrecognisedStoreErrorDoesNotReachTheClient(t *testing.T) {
	store := errStore{err: fmt.Errorf(
		"dial postgres://panel:%s@audit.internal/events: connection refused", secret)}
	svc := newService(t, store)

	calls := map[string]func() error{
		"GetEvent": func() error {
			_, err := svc.GetEvent(context.Background(),
				&eventpbv1.GetEventRequest{Name: "devices/3/events/0000000000000001"})
			return err
		},
		"ListEvents": func() error {
			_, err := svc.ListEvents(context.Background(),
				&eventpbv1.ListEventsRequest{Parent: "devices/3"})
			return err
		},
	}

	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			err := call()
			if code := status.Code(err); code != codes.Internal {
				t.Fatalf("code = %v, want Internal", code)
			}

			msg := status.Convert(err).Message()
			for _, leaked := range []string{secret, "postgres", "audit.internal"} {
				if strings.Contains(msg, leaked) {
					t.Errorf("the status message %q carries %q out of the panel", msg, leaked)
				}
			}
		})
	}
}

// TestStoreErrorsMapToTheCodeAClientCanActOn.
//
// The sentinels exist so that a caller telling "no such event" from "bad token"
// never has to match strings. This is the assertion that the mapping is
// complete for every sentinel this package exports.
func TestStoreErrorsMapToTheCodeAClientCanActOn(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want codes.Code
	}{
		{name: "missing", err: fmt.Errorf("%w: gone", service.ErrNotFound), want: codes.NotFound},
		{name: "bad token", err: service.ErrInvalidPageToken, want: codes.InvalidArgument},
		{name: "bad name", err: service.ErrInvalidName, want: codes.InvalidArgument},
		{name: "bad parent", err: service.ErrInvalidParent, want: codes.InvalidArgument},

		// A client that hung up, or whose deadline expired, is an expected
		// outcome on a long listing rather than a fault of the panel's.
		// Reporting it as Internal would put it in the wrong alert.
		{name: "client gone", err: context.Canceled, want: codes.Canceled},
		{name: "deadline", err: context.DeadlineExceeded, want: codes.DeadlineExceeded},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newService(t, errStore{err: tt.err})

			_, err := svc.ListEvents(context.Background(),
				&eventpbv1.ListEventsRequest{Parent: "devices/3"})
			if code := status.Code(err); code != tt.want {
				t.Fatalf("code = %v (%v), want %v", code, err, tt.want)
			}
		})
	}
}

// TestACredentialNeverAppearsInAnError.
//
// The credential reaches the store because record was asked for it; that is the
// application's decision and the payload is where it belongs. What must never
// happen is the same bytes arriving in a status message, where they would land
// in a client's log, a proxy's access log and an error tracker, none of which
// anybody chose.
func TestACredentialNeverAppearsInAnError(t *testing.T) {
	store := newStore(t, 4)
	names := appendTo(t, store, "devices/3", 1)
	svc := newService(t, store)

	// The payload does carry it, because the caller opted in.
	got, err := svc.GetEvent(context.Background(), &eventpbv1.GetEventRequest{Name: names[0]})
	if err != nil {
		t.Fatalf("GetEvent: %v", err)
	}
	if !strings.Contains(string(got.GetCardRead().GetData()), string(credential)) {
		t.Fatal("the credential the caller opted into was not stored; this test is checking nothing")
	}

	// No error path may.
	for _, name := range []string{
		"devices/3/events/00000000deadbeef", "devices/3/events", "", "devices/-/events/1",
	} {
		_, err := svc.GetEvent(context.Background(), &eventpbv1.GetEventRequest{Name: name})
		if err == nil {
			continue
		}
		if strings.Contains(status.Convert(err).Message(), string(credential)) {
			t.Errorf("the error for %q carries the credential", name)
		}
	}
}
