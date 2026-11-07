// Package slowtest is the client that the tests of the routes reading a body
// under slowclient.ReadBody share: one that sends a body a byte at a time.
package slowtest

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/PuraFome/meuRPG/backend/internal/platform/safego"
)

// interval is the time between the bytes of the trickle.
const interval = 20 * time.Millisecond

// AnswerWhileTrickling sends head and first as the start of a chunked request
// (head is the request line and the headers, each ending in CRLF), goes on
// sending a byte every 20 ms and returns the status line of the answer. It fails
// when no answer comes within wait.
//
// net/http discards what is left of a body the handler did not read before it
// answers, so a connection with no read deadline left holds the answer back for
// as long as the client trickles: the answer is what the deadline buys.
func AnswerWhileTrickling(t *testing.T, addr, head, first string, wait time.Duration) string {
	t.Helper()
	conn, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if _, err := conn.Write([]byte(head + "Transfer-Encoding: chunked\r\n\r\n" + chunk(first))); err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithCancel(t.Context())
	t.Cleanup(stop)
	safego.Go(nil, "slow answer", func() {
		tick := time.NewTicker(interval)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				if _, err := conn.Write([]byte(chunk("x"))); err != nil {
					return
				}
			}
		}
	})
	_ = conn.SetReadDeadline(time.Now().Add(wait))
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("no answer within %v while the client kept trickling (the connection has no read deadline left): %v", wait, err)
	}
	return line
}

func chunk(s string) string { return fmt.Sprintf("%x\r\n%s\r\n", len(s), s) }
