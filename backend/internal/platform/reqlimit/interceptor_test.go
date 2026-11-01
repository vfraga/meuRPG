package reqlimit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	systemv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/system/v1"
)

type refusal struct {
	reason systemv1.RequestRefusedReason
	field  string
}

// refusalOf is what the client reads from an error: nil for none.
func refusalOf(t *testing.T, err error) *refusal {
	t.Helper()
	if err == nil {
		return nil
	}
	ce, ok := errors.AsType[*connect.Error](err)
	if !ok {
		t.Fatalf("not a Connect error: %v", err)
	}
	if ce.Code() != connect.CodeInvalidArgument {
		t.Fatalf("code = %v, want invalid_argument", ce.Code())
	}
	for _, d := range ce.Details() {
		v, verr := d.Value()
		if verr != nil {
			t.Fatal(verr)
		}
		if r, ok := v.(*systemv1.RequestRefused); ok {
			return &refusal{r.GetReason(), r.GetField()}
		}
	}
	t.Fatalf("no RequestRefused detail in %v", err)
	return nil
}

func TestCheckDefaults(t *testing.T) {
	s := newSchema(t)
	const (
		list = systemv1.RequestRefusedReason_REQUEST_REFUSED_REASON_LIST_TOO_LONG
		long = systemv1.RequestRefusedReason_REQUEST_REFUSED_REASON_TEXT_TOO_LONG
	)
	tests := []struct {
		name  string
		build func() *dynamicpb.Message
		want  *refusal
	}{
		{"empty request", s.newReq, nil},
		{"list at the limit", func() *dynamicpb.Message { m := s.newReq(); s.setStrings(m, "names", DefaultMaxItems, "x"); return m }, nil},
		{"list over the limit", func() *dynamicpb.Message { m := s.newReq(); s.setStrings(m, "names", DefaultMaxItems+1, "x"); return m }, &refusal{list, "names"}},
		{"element at the limit", func() *dynamicpb.Message {
			m := s.newReq()
			s.setStrings(m, "names", 2, text(DefaultMaxString))
			return m
		}, nil},
		{"element over the limit", func() *dynamicpb.Message {
			m := s.newReq()
			s.setStrings(m, "names", 2, text(DefaultMaxString+1))
			return m
		}, &refusal{long, "names"}},
		{"string at the limit", func() *dynamicpb.Message { m := s.newReq(); s.set(m, "word", text(DefaultMaxString)); return m }, nil},
		{"oneof string over the limit", func() *dynamicpb.Message { m := s.newReq(); s.set(m, "word", text(DefaultMaxString+1)); return m }, &refusal{long, "word"}},
		{"nested message string over", func() *dynamicpb.Message {
			m := s.newReq()
			s.setLeaves(m, 3, "ok")
			s.setLeaves(m, 1, text(DefaultMaxString+1))
			return m
		}, &refusal{long, "leaves.text"}},
		{"nested list over the limit", func() *dynamicpb.Message { m := s.newReq(); s.setLeaves(m, DefaultMaxItems+1, "ok"); return m }, &refusal{list, "leaves"}},
		{"list is checked before its elements", func() *dynamicpb.Message {
			m := s.newReq()
			s.setLeaves(m, DefaultMaxItems+1, text(DefaultMaxString+1))
			return m
		}, &refusal{list, "leaves"}},
		{"oneof message over", func() *dynamicpb.Message {
			m := s.newReq()
			m.Set(s.fieldOf(s.req, "chosen"), protoreflect.ValueOfMessage(s.leafWith(text(DefaultMaxString+1))))
			return m
		}, &refusal{long, "chosen.text"}},
		{"bytes at the limit", func() *dynamicpb.Message { m := s.newReq(); s.setBlob(m, DefaultMaxBytes); return m }, nil},
		{"bytes over the limit", func() *dynamicpb.Message { m := s.newReq(); s.setBlob(m, DefaultMaxBytes+1); return m }, &refusal{long, "leaves.blob"}},
		{"map over the limit", func() *dynamicpb.Message { m := s.newReq(); s.setLabels(m, DefaultMaxItems+1, "k", "v"); return m }, &refusal{list, "labels"}},
		{"map at the limit", func() *dynamicpb.Message { m := s.newReq(); s.setLabels(m, DefaultMaxItems, "k", "v"); return m }, nil},
		{"map value over", func() *dynamicpb.Message { m := s.newReq(); s.setLabels(m, 1, "k", text(DefaultMaxString+1)); return m }, &refusal{long, "labels"}},
		{"map key over", func() *dynamicpb.Message { m := s.newReq(); s.setLabels(m, 1, text(DefaultMaxString+1), "v"); return m }, &refusal{long, "labels"}},
		{"map message value over", func() *dynamicpb.Message {
			m := s.newReq()
			m.Mutable(s.fieldOf(s.req, "by_key")).Map().Set(protoreflect.ValueOfString("a").MapKey(), protoreflect.ValueOfMessage(s.leafWith(text(DefaultMaxString+1))))
			return m
		}, &refusal{long, "by_key.text"}},
		{"recursive message over, three deep", func() *dynamicpb.Message { m := s.newReq(); s.setTree(m, 3, text(DefaultMaxString+1)); return m }, &refusal{long, "tree.children.children.children.name"}},
		{"recursive message within", func() *dynamicpb.Message { m := s.newReq(); s.setTree(m, 30, "ok"); return m }, nil},
		{"many numbers over the limit", func() *dynamicpb.Message { m := s.newReq(); s.setNums(m, DefaultMaxItems+1); return m }, &refusal{list, "nums"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := refusalOf(t, Check(tt.build()))
			switch {
			case tt.want == nil && got != nil, tt.want != nil && (got == nil || *got != *tt.want):
				t.Errorf("refusal = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckOptionsChangeTheLimit(t *testing.T) {
	s := newSchema(t)
	t.Run("max_items and max_len lower the limit", func(t *testing.T) {
		m := s.newReq()
		s.setStrings(m, "few", 5, "abc")
		if got := Check(m); got != nil {
			t.Fatalf("at the options' limits: %v", got)
		}
		s.setStrings(m, "few", 1, "abc")
		if r := refusalOf(t, Check(m)); r == nil || r.field != "few" || r.reason != systemv1.RequestRefusedReason_REQUEST_REFUSED_REASON_LIST_TOO_LONG {
			t.Errorf("sixth item: %v", r)
		}
		m = s.newReq()
		s.setStrings(m, "few", 1, "abcd")
		if r := refusalOf(t, Check(m)); r == nil || r.reason != systemv1.RequestRefusedReason_REQUEST_REFUSED_REASON_TEXT_TOO_LONG {
			t.Errorf("long element: %v", r)
		}
	})
	t.Run("max_items raises the limit", func(t *testing.T) {
		m := s.newReq()
		s.setStrings(m, "many", 1000, "x")
		if got := Check(m); got != nil {
			t.Errorf("1000 items with max_items 1000: %v", got)
		}
		s.setStrings(m, "many", 1, "x")
		if r := refusalOf(t, Check(m)); r == nil || r.field != "many" {
			t.Errorf("1001 items: %v", r)
		}
	})
	t.Run("max_len raises the limit", func(t *testing.T) {
		m := s.leafWith("")
		m.Set(s.fieldOf(s.leaf, "long_note"), protoreflect.ValueOfString(text(20000)))
		if got := Check(m); got != nil {
			t.Errorf("20000 bytes with max_len 20000: %v", got)
		}
		m.Set(s.fieldOf(s.leaf, "long_note"), protoreflect.ValueOfString(text(20001)))
		if r := refusalOf(t, Check(m)); r == nil || r.field != "long_note" {
			t.Errorf("20001 bytes: %v", r)
		}
		// The same size in a field without the option is over its default.
		m = s.leafWith(text(20000))
		if r := refusalOf(t, Check(m)); r == nil || r.field != "text" {
			t.Errorf("20000 bytes without the option: %v", r)
		}
	})
}

func TestEffectiveLimit(t *testing.T) {
	s := newSchema(t)
	tests := []struct {
		msg   protoreflect.MessageDescriptor
		field string
		want  uint32
	}{
		{s.req, "names", DefaultMaxItems},
		{s.req, "labels", DefaultMaxItems},
		{s.req, "word", DefaultMaxString},
		{s.leaf, "blob", DefaultMaxBytes},
		{s.leaf, "long_note", 20000},
		{s.req, "few", 5},
		{s.req, "many", 1000},
		{s.req, "n", 0},
		{s.req, "tree", 0},
	}
	for _, tt := range tests {
		if got := EffectiveLimit(s.fieldOf(tt.msg, tt.field)); got != tt.want {
			t.Errorf("%s.%s = %d, want %d", tt.msg.Name(), tt.field, got, tt.want)
		}
	}
	for name, want := range map[string]bool{"names": false, "few": false, "many": true, "word": false} {
		if got := Raised(s.fieldOf(s.req, name)); got != want {
			t.Errorf("Raised(%s) = %v, want %v", name, got, want)
		}
	}
	if !Raised(s.fieldOf(s.leaf, "long_note")) {
		t.Error("Raised(long_note) = false, want true")
	}
	if got := LenLimit(s.fieldOf(s.req, "few"), protoreflect.StringKind); got != 3 {
		t.Errorf("LenLimit(few) = %d, want 3", got)
	}
}

// serve mounts a unary and a server-stream handler behind the interceptor.
// Both take a GetServerInfoResponse as the request: it is a generated message
// with two string fields, which is all these tests need.
func serve(t *testing.T, extra func(http.Handler) http.Handler) (url string, called *int) {
	t.Helper()
	calls := new(int)
	mux := http.NewServeMux()
	opts := connect.WithInterceptors(Interceptor())
	mux.Handle("/meurpg.test.v1.Echo/Say", connect.NewUnaryHandler("/meurpg.test.v1.Echo/Say",
		func(_ context.Context, req *connect.Request[systemv1.GetServerInfoResponse]) (*connect.Response[systemv1.GetServerInfoResponse], error) {
			*calls++
			return connect.NewResponse(req.Msg), nil
		}, opts))
	mux.Handle("/meurpg.test.v1.Echo/Watch", connect.NewServerStreamHandler("/meurpg.test.v1.Echo/Watch",
		func(_ context.Context, req *connect.Request[systemv1.GetServerInfoResponse], stream *connect.ServerStream[systemv1.GetServerInfoResponse]) error {
			*calls++
			return stream.Send(req.Msg)
		}, opts))
	var h http.Handler = mux
	if extra != nil {
		h = extra(h)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv.URL, calls
}

func TestInterceptorUnary(t *testing.T) {
	url, called := serve(t, nil)
	client := connect.NewClient[systemv1.GetServerInfoResponse, systemv1.GetServerInfoResponse](http.DefaultClient, url+"/meurpg.test.v1.Echo/Say")
	for _, n := range []int{DefaultMaxString, DefaultMaxString + 1} {
		before := *called
		_, err := client.CallUnary(t.Context(), connect.NewRequest(&systemv1.GetServerInfoResponse{Version: text(n)}))
		got := refusalOf(t, err)
		if n <= DefaultMaxString {
			if got != nil || *called != before+1 {
				t.Errorf("%d bytes: refusal %v, handler calls %d", n, got, *called-before)
			}
			continue
		}
		if got == nil || got.field != "version" || got.reason != systemv1.RequestRefusedReason_REQUEST_REFUSED_REASON_TEXT_TOO_LONG {
			t.Errorf("%d bytes: refusal %v", n, got)
		}
		if *called != before {
			t.Errorf("%d bytes: the handler ran", n)
		}
		if ce, ok := errors.AsType[*connect.Error](err); ok {
			if ce.Message() != "request over its size limit" || ce.Meta().Get("Cache-Control") != "no-store" {
				t.Errorf("message %q, cache-control %q", ce.Message(), ce.Meta().Get("Cache-Control"))
			}
		}
	}
}

func TestInterceptorServerStream(t *testing.T) {
	url, called := serve(t, nil)
	client := connect.NewClient[systemv1.GetServerInfoResponse, systemv1.GetServerInfoResponse](http.DefaultClient, url+"/meurpg.test.v1.Echo/Watch")
	open := func(n int) error {
		stream, err := client.CallServerStream(t.Context(), connect.NewRequest(&systemv1.GetServerInfoResponse{Commit: text(n)}))
		if err != nil {
			return err
		}
		defer stream.Close()
		for stream.Receive() {
		}
		return stream.Err()
	}
	if err := open(DefaultMaxString); err != nil || *called != 1 {
		t.Fatalf("at the limit: %v, handler calls %d", err, *called)
	}
	got := refusalOf(t, open(DefaultMaxString+1))
	if got == nil || got.field != "commit" || got.reason != systemv1.RequestRefusedReason_REQUEST_REFUSED_REASON_TEXT_TOO_LONG {
		t.Errorf("over the limit: refusal %v", got)
	}
	if *called != 1 {
		t.Errorf("the handler ran for a request over the limit")
	}
}

// The unary body is read at ReadLimit(procedure): a body over it is
// `resource_exhausted` to the client, never an `internal` or a dropped connection.
func TestMiddlewareRefusesALargeBodyAsResourceExhausted(t *testing.T) {
	url, called := serve(t, Middleware)
	client := connect.NewClient[systemv1.GetServerInfoResponse, systemv1.GetServerInfoResponse](http.DefaultClient, url+"/meurpg.test.v1.Echo/Say")
	say := func(n int) error {
		_, err := client.CallUnary(t.Context(), connect.NewRequest(&systemv1.GetServerInfoResponse{Version: text(n)}))
		return err
	}
	// Over the string default and under the read limit: the interceptor decides.
	if r := refusalOf(t, say(DefaultReadLimit-100)); r == nil {
		t.Fatal("a string over its limit was not refused by the interceptor")
	}
	err := say(DefaultReadLimit + 1)
	if ce, ok := errors.AsType[*connect.Error](err); !ok || ce.Code() != connect.CodeResourceExhausted {
		t.Fatalf("a body over the read limit: %v, want resource_exhausted", err)
	}
	if *called != 0 {
		t.Errorf("the handler ran %d times", *called)
	}
	// A raised read limit lets the body through to the interceptor again.
	withRaised(t, "/meurpg.test.v1.Echo/Say", DefaultReadLimit*2)
	if r := refusalOf(t, say(DefaultReadLimit+1)); r == nil {
		t.Error("a body under its raised read limit should reach the interceptor")
	}
	if ReadLimit("/meurpg.test.v1.Echo/Say") != DefaultReadLimit*2 || ReadLimit("/meurpg.test.v1.Echo/Other") != DefaultReadLimit {
		t.Error("ReadLimit does not follow the table")
	}
}

// Anything that is not a unary RPC keeps the body it had.
func TestMiddlewareLeavesOtherRequestsAlone(t *testing.T) {
	var read int
	h := Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		b := make([]byte, 1<<20)
		n, _ := r.Body.Read(b)
		read = n
	}))
	for _, c := range []struct{ method, path, contentType string }{
		{http.MethodPost, "/upload", "application/json"},
		{http.MethodPost, "/meurpg.x.v1.S/M", "application/connect+proto"},
		{http.MethodPost, "/meurpg.x.v1.S/M", "multipart/form-data; boundary=x"},
		{http.MethodGet, "/meurpg.x.v1.S/M", "application/json"},
	} {
		read = 0
		path := c.path
		req := httptest.NewRequestWithContext(t.Context(), c.method, path, strings.NewReader(text(DefaultReadLimit*4)))
		req.Header.Set("Content-Type", c.contentType)
		h.ServeHTTP(httptest.NewRecorder(), req)
		if read <= DefaultReadLimit {
			t.Errorf("%s %s: read %d bytes, the body was limited", path, c.contentType, read)
		}
	}
}

// withRaised puts an entry in raisedReadLimits for the test and takes it out
// at the end. The tests that use it are not parallel.
func withRaised(t *testing.T, procedure string, n int64) {
	t.Helper()
	raisedMu.Lock()
	raisedReadLimits[procedure] = n
	raisedMu.Unlock()
	t.Cleanup(func() {
		raisedMu.Lock()
		delete(raisedReadLimits, procedure)
		raisedMu.Unlock()
	})
}

// A stream's request (connect+proto) keeps only the 4 MiB backstop: the
// per-procedure read limit does not touch it, while the same size in a unary
// body is refused.
func TestStreamRequestKeepsOnlyTheBackstop(t *testing.T) {
	// No interceptor here: the field limits would refuse this size first.
	called := new(int)
	mux := http.NewServeMux()
	mux.Handle("/meurpg.test.v1.Echo/Watch", connect.NewServerStreamHandler("/meurpg.test.v1.Echo/Watch",
		func(_ context.Context, req *connect.Request[systemv1.GetServerInfoResponse], stream *connect.ServerStream[systemv1.GetServerInfoResponse]) error {
			*called++
			return stream.Send(&systemv1.GetServerInfoResponse{Version: strings.Repeat("b", len(req.Msg.GetCommit()))})
		}, connect.WithReadMaxBytes(Backstop)))
	srv := httptest.NewServer(Middleware(mux))
	t.Cleanup(srv.Close)
	client := connect.NewClient[systemv1.GetServerInfoResponse, systemv1.GetServerInfoResponse](http.DefaultClient, srv.URL+"/meurpg.test.v1.Echo/Watch")
	open := func(n int) error {
		stream, err := client.CallServerStream(t.Context(), connect.NewRequest(&systemv1.GetServerInfoResponse{Commit: text(n)}))
		if err != nil {
			return err
		}
		defer stream.Close()
		for stream.Receive() {
		}
		return stream.Err()
	}
	if err := open(DefaultReadLimit * 4); err != nil || *called != 1 {
		t.Fatalf("a stream request of %d bytes: %v (handler calls %d)", DefaultReadLimit*4, err, *called)
	}
	ce, ok := errors.AsType[*connect.Error](open(Backstop + 1))
	if !ok || ce.Code() != connect.CodeResourceExhausted {
		t.Errorf("a stream request over the backstop: %v, want resource_exhausted", ce)
	}
}
