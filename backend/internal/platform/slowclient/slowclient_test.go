package slowclient_test

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/slowclient"
)

const procedure = "/test.Svc/Stream"

// streamServer serves a stream that sends 256 KiB messages until a Send fails,
// and reports how it ended on the returned channel.
func streamServer(t *testing.T, sendTimeout time.Duration) (*httptest.Server, chan error) {
	t.Helper()
	ended := make(chan error, 1)
	big := strings.Repeat("a", 256<<10)
	mux := http.NewServeMux()
	mux.Handle(procedure, connect.NewServerStreamHandler(procedure,
		func(ctx context.Context, _ *connect.Request[campaignsv1.GetCampaignRequest], s *connect.ServerStream[campaignsv1.GetCampaignRequest]) error {
			for {
				if err := s.Send(&campaignsv1.GetCampaignRequest{CampaignId: big}); err != nil {
					ended <- err
					return nil //nolint:nilerr // the send failing is how this test's stream ends; the error is reported on the channel
				}
				select {
				case <-ctx.Done():
					ended <- ctx.Err()
					return nil
				default:
				}
			}
		}, connect.WithInterceptors(slowclient.Interceptor(sendTimeout))))
	srv := httptest.NewServer(slowclient.Middleware(mux))
	t.Cleanup(srv.Close)
	return srv, ended
}

// A client that connects and never reads: the server's writes fill the socket
// buffers, and without a deadline the handler would sit in Send for good.
func TestAStalledReaderEndsTheStream(t *testing.T) {
	t.Parallel()
	srv, ended := streamServer(t, 300*time.Millisecond)

	conn, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	_ = conn.(*net.TCPConn).SetReadBuffer(4096) // so the buffers fill quickly
	// A Connect server-stream request: one empty message in an envelope.
	req := "POST " + procedure + " HTTP/1.1\r\nHost: x\r\nContent-Type: application/connect+proto\r\nContent-Length: 5\r\n\r\n\x00\x00\x00\x00\x00"
	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-ended:
		if err == nil {
			t.Error("the stream ended without an error")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the handler is still blocked in Send: a stalled reader holds the stream")
	}
}

// A client that reads keeps its stream: each Send gets a fresh deadline, and the
// time between messages does not count.
func TestAReaderThatKeepsUpKeepsTheStream(t *testing.T) {
	t.Parallel()
	srv, ended := streamServer(t, 300*time.Millisecond)

	conn, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	req := "POST " + procedure + " HTTP/1.1\r\nHost: x\r\nContent-Type: application/connect+proto\r\nContent-Length: 5\r\n\r\n\x00\x00\x00\x00\x00"
	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatal(err)
	}
	// Read for longer than the deadline, slowly but steadily.
	r := bufio.NewReader(conn)
	buf := make([]byte, 64<<10)
	stop := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(stop) {
		_ = conn.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := r.Read(buf); err != nil {
			t.Fatalf("the stream broke while its reader kept up: %v", err)
		}
	}
	select {
	case err := <-ended:
		t.Fatalf("the server ended a stream whose reader kept up: %v", err)
	default:
	}
}

func TestReadBodyEndsATrickledBody(t *testing.T) {
	t.Parallel()
	done := make(chan error, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slowclient.ReadBody(w, 300*time.Millisecond)
		buf := make([]byte, 1024)
		for {
			if _, err := r.Body.Read(buf); err != nil {
				done <- err
				return
			}
		}
	}))
	t.Cleanup(srv.Close)

	conn, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	// Promises 1 MiB, sends 10 bytes, then nothing.
	_, _ = conn.Write([]byte("POST / HTTP/1.1\r\nHost: x\r\nContent-Length: 1048576\r\n\r\n0123456789"))

	select {
	case err := <-done:
		if err == nil {
			t.Error("the body read ended without an error")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the handler is still waiting for the body")
	}
}

func TestWriteBodyEndsAWriteToAClientThatStoppedReading(t *testing.T) {
	t.Parallel()
	done := make(chan error, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		defer slowclient.WriteBody(w, 300*time.Millisecond)()
		chunk := make([]byte, 64<<10)
		for {
			if _, err := w.Write(chunk); err != nil {
				done <- err
				return
			}
			if err := http.NewResponseController(w).Flush(); err != nil {
				done <- err
				return
			}
		}
	}))
	t.Cleanup(srv.Close)

	conn, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.SetReadBuffer(4 << 10)
	}
	// Asks for the response and never reads it.
	_, _ = conn.Write([]byte("GET / HTTP/1.1\r\nHost: x\r\n\r\n"))

	select {
	case err := <-done:
		if err == nil {
			t.Error("the write ended without an error")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the handler is still blocked writing to a client that stopped reading")
	}
}

func TestWriteBodyLeavesAWriterWithoutDeadlinesAlone(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	slowclient.WriteBody(rec, time.Second)()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}
