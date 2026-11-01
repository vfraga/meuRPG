package migrations

import (
	"context"
	"errors"
	"net"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PuraFome/meuRPG/backend/internal/platform/testenv"
)

// serializeUpTo is how many migrations the two runs of TestMigrateUpRunsSerialize
// share: a few are enough to show that the second run waits and then finds
// nothing to do, and each one costs seconds of DDL.
const serializeUpTo = 8

// TestMigrateUpRunsSerialize: two `migrate up` runs started together, as two
// deploys at once, never overlap. Each takes the lock, runs the first real migrations
// and releases it; the second finds nothing left to apply.
func TestMigrateUpRunsSerialize(t *testing.T) {
	t.Parallel()
	db := freshDatabase(t)

	var inside, overlaps atomic.Int32
	var applied atomic.Int32
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			ctx, release, err := AcquireLock(t.Context(), db, LockOptions{TTL: 6 * time.Second, Poll: 50 * time.Millisecond})
			if err != nil {
				t.Errorf("AcquireLock() error = %v", err)
				return
			}
			defer release()
			if inside.Add(1) > 1 {
				overlaps.Add(1)
			}
			defer inside.Add(-1)
			provider, err := NewTestProvider(t.Context(), db)
			if err != nil {
				t.Errorf("NewProvider() error = %v", err)
				return
			}
			results, err := provider.UpTo(ctx, serializeUpTo)
			if err != nil {
				t.Errorf("Up() error = %v", err)
				return
			}
			applied.Add(int32(len(results))) //nolint:gosec // G115: a few hundred migrations
		})
	}
	wg.Wait()

	if overlaps.Load() != 0 {
		t.Errorf("%d runs held the lock while another one did", overlaps.Load())
	}
	if got, want := int(applied.Load()), serializeUpTo; got != want {
		t.Errorf("the two runs applied %d migrations in all, want %d (each once)", got, want)
	}
	var rows int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM migration_lock`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Errorf("migration_lock has %d rows after both runs, want 0 (released)", rows)
	}
}

// TestLockTimesOutWithAClearError: while a live run holds the lock, a second one
// waits for its Wait and then says so.
func TestLockTimesOutWithAClearError(t *testing.T) {
	t.Parallel()
	db := freshDatabase(t)
	_, release, err := AcquireLock(t.Context(), db, LockOptions{TTL: 30 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	_, _, err = AcquireLock(t.Context(), db, LockOptions{TTL: 30 * time.Second, Wait: 300 * time.Millisecond, Poll: 50 * time.Millisecond})
	if !errors.Is(err, ErrLockTimeout) {
		t.Errorf("second AcquireLock() error = %v, want ErrLockTimeout", err)
	}
}

// TestLockOfACrashedRunExpires: a run that died leaves its row; once the lease
// has passed, the next run takes over without anyone cleaning up by hand.
func TestLockOfACrashedRunExpires(t *testing.T) {
	t.Parallel()
	db := freshDatabase(t)
	// A first run that never heartbeats nor releases: a short lease, no release call.
	if err := createLockTable(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	if got, err := tryAcquire(t.Context(), db, "crashed", 300*time.Millisecond); err != nil || !got {
		t.Fatalf("tryAcquire(crashed) = %v, %v; want true", got, err)
	}
	if got, _ := tryAcquire(t.Context(), db, "second", time.Minute); got {
		t.Fatal("a live lease was taken over")
	}

	_, release, err := AcquireLock(t.Context(), db, LockOptions{TTL: time.Minute, Wait: 10 * time.Second, Poll: 100 * time.Millisecond})
	if err != nil {
		t.Fatalf("AcquireLock() after the lease expired error = %v", err)
	}
	release()
}

// TestLostLeaseStopsTheRun: when another holder has the row, the heartbeat finds
// out and cancels the run's context, so goose stops between statements.
func TestLostLeaseStopsTheRun(t *testing.T) {
	t.Parallel()
	db := freshDatabase(t)
	ctx, release, err := AcquireLock(t.Context(), db, LockOptions{TTL: 600 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := db.ExecContext(t.Context(), `UPDATE migration_lock SET holder = 'someone else'`); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
		if cause := context.Cause(ctx); cause == nil || errors.Is(cause, context.Canceled) {
			t.Errorf("cause = %v, want the lost-lease reason", cause)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the run was not stopped after losing the lease")
	}
}

// freezable is a TCP proxy to the test database whose traffic can be stopped
// without closing anything: what a black-holed network does to a connection,
// which never fails by itself.
type freezable struct {
	addr   string
	frozen atomic.Bool
}

func newFreezable(t *testing.T, target string) *freezable {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &freezable{addr: ln.Addr().String()}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			client, err := ln.Accept()
			if err != nil {
				return
			}
			server, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", target)
			if err != nil {
				_ = client.Close()
				continue
			}
			go f.pipe(server, client)
			go f.pipe(client, server)
		}
	}()
	return f
}

// pipe copies from src to dst, holding each chunk back while the proxy is frozen.
func (f *freezable) pipe(dst, src net.Conn) {
	defer func() { _ = dst.Close(); _ = src.Close() }()
	buf := make([]byte, 32<<10)
	for {
		n, err := src.Read(buf)
		for f.frozen.Load() {
			time.Sleep(5 * time.Millisecond)
		}
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// TestRunStopsBeforeTheLeaseEndsWhenTheDatabaseStopsAnswering: a heartbeat that
// cannot reach the database, and gets no error either, cancels the run before
// the lease runs out, so a second runner never starts on top of it. While the
// database answers, the same lease is renewed past its first term.
func TestRunStopsBeforeTheLeaseEndsWhenTheDatabaseStopsAnswering(t *testing.T) {
	t.Parallel()
	direct := freshDatabase(t)
	u, err := url.Parse(testenv.DatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	var name string
	if err := direct.QueryRowContext(t.Context(), `SELECT current_database()`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	proxy := newFreezable(t, u.Host)
	u.Host, u.Path = proxy.addr, "/"+name
	viaProxy := open(t, u.String())

	const ttl = 900 * time.Millisecond
	ctx, release, err := AcquireLock(t.Context(), viaProxy, LockOptions{TTL: ttl})
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	canceled := make(chan time.Time, 1)
	go func() {
		<-ctx.Done()
		canceled <- time.Now()
	}()

	// Positive control: with the database answering, the lease outlives its first term.
	select {
	case <-canceled:
		t.Fatalf("the run was canceled while the heartbeat could renew: %v", context.Cause(ctx))
	case <-time.After(2 * ttl):
	}

	proxy.frozen.Store(true)
	time.Sleep(200 * time.Millisecond) // anything already sent has landed or is held back
	var expires, dbNow time.Time
	if err := direct.QueryRowContext(t.Context(), `SELECT expires_at, now() FROM migration_lock WHERE id = 1`).Scan(&expires, &dbNow); err != nil {
		t.Fatal(err)
	}
	readAt := time.Now()

	select {
	case at := <-canceled:
		// On the database's clock: its now() at the read, plus the time since.
		if late := dbNow.Add(at.Sub(readAt)).Sub(expires); late > 0 {
			t.Errorf("the run was canceled %v after its lease ended, want before", late)
		}
	case <-time.After(3 * ttl):
		t.Fatal("the run was not canceled although the database stopped answering")
	}
}
