package reqlimit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"

	systemv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/system/v1"
)

// HandlerOption carries the protocol header requirement, the field limits and
// the backstop of one message.
func TestHandlerOptionAppliesTheRequestSizeLimits(t *testing.T) {
	const procedure = "/meurpg.test.v1.Echo/Say"
	handler := connect.NewUnaryHandler(procedure,
		func(_ context.Context, req *connect.Request[systemv1.GetServerInfoResponse]) (*connect.Response[systemv1.GetServerInfoResponse], error) {
			return connect.NewResponse(req.Msg), nil
		}, HandlerOption())
	mux := http.NewServeMux()
	mux.Handle(procedure, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client := connect.NewClient[systemv1.GetServerInfoResponse, systemv1.GetServerInfoResponse](http.DefaultClient, srv.URL+procedure)
	call := func(n int) error {
		_, err := client.CallUnary(t.Context(), connect.NewRequest(&systemv1.GetServerInfoResponse{Version: strings.Repeat("a", n)}))
		return err
	}

	if err := call(DefaultMaxString); err != nil {
		t.Errorf("a field at its limit: %v", err)
	}
	if code := connect.CodeOf(call(DefaultMaxString + 1)); code != connect.CodeInvalidArgument {
		t.Errorf("a field over its limit: code = %v, want invalid_argument", code)
	}
	if code := connect.CodeOf(call(Backstop + 1)); code != connect.CodeResourceExhausted {
		t.Errorf("a message over the backstop: code = %v, want resource_exhausted", code)
	}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+procedure, strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode == http.StatusOK {
		t.Error("a call without the Connect-Protocol-Version header was accepted")
	}
}
