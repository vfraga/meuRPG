package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"

	systemv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/system/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/reqlimit"
)

// connectOptions carries the request size limits: a field over its limit is
// refused with a RequestRefused detail before the handler runs, and a body over
// the procedure's read limit is refused by the transport.
func TestConnectOptionsApplyRequestSizeLimits(t *testing.T) {
	const procedure = "/meurpg.test.v1.Echo/Say"
	handlerRan := false
	handler := connect.NewUnaryHandler(procedure,
		func(_ context.Context, req *connect.Request[systemv1.GetServerInfoResponse]) (*connect.Response[systemv1.GetServerInfoResponse], error) {
			handlerRan = true
			return connect.NewResponse(req.Msg), nil
		}, connectOptions(slog.New(slog.DiscardHandler), nil)...)
	mux := http.NewServeMux()
	mux.Handle(procedure, handler)
	srv := httptest.NewServer(reqlimit.Middleware(mux))
	t.Cleanup(srv.Close)
	client := connect.NewClient[systemv1.GetServerInfoResponse, systemv1.GetServerInfoResponse](http.DefaultClient, srv.URL+procedure, connect.WithProtoJSON())
	// The handlers require the protocol header, which the client sends.

	call := func(n int) error {
		_, err := client.CallUnary(t.Context(), connect.NewRequest(&systemv1.GetServerInfoResponse{Version: strings.Repeat("a", n)}))
		return err
	}
	if err := call(reqlimit.DefaultMaxString); err != nil || !handlerRan {
		t.Fatalf("a field at its limit: %v (handler ran: %v)", err, handlerRan)
	}

	handlerRan = false
	ce, ok := errors.AsType[*connect.Error](call(reqlimit.DefaultMaxString + 1))
	if !ok || ce.Code() != connect.CodeInvalidArgument || len(ce.Details()) != 1 || handlerRan {
		t.Fatalf("a field over its limit: %v (handler ran: %v)", ce, handlerRan)
	}
	v, err := ce.Details()[0].Value()
	if err != nil {
		t.Fatal(err)
	}
	if r, ok := v.(*systemv1.RequestRefused); !ok || r.GetField() != "version" || r.GetReason() != systemv1.RequestRefusedReason_REQUEST_REFUSED_REASON_TEXT_TOO_LONG {
		t.Errorf("detail = %v", v)
	}

	ce, ok = errors.AsType[*connect.Error](call(reqlimit.DefaultReadLimit + 1))
	if !ok || ce.Code() != connect.CodeResourceExhausted || handlerRan {
		t.Errorf("a body over the read limit: %v (handler ran: %v)", ce, handlerRan)
	}
}
