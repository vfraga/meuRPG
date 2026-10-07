package maps

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/PuraFome/meuRPG/backend/internal/maps/mapsdb"
)

// Finding U5-2: a waiter on the fog scene's single flight inherits the first caller's canceled context.

// blockLayers is a mapsdb.DBTX whose first read of map_layers never reaches the
// database: it signals started and waits for the statement's ctx to end.
type blockLayers struct {
	mapsdb.DBTX
	once    sync.Once
	started chan struct{}
}

func (b *blockLayers) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if strings.Contains(sql, "FROM map_layers") {
		first := false
		b.once.Do(func() { first = true })
		if first {
			close(b.started)
			<-ctx.Done()
			return errRow{ctx.Err()}
		}
	}
	return b.DBTX.QueryRow(ctx, sql, args...)
}

type errRow struct{ err error }

func (r errRow) Scan(...any) error { return r.err }

func TestReview5_LitSingleFlightCancel(t *testing.T) {
	c := newCave(t)
	svc := c.h.svc
	ctx := t.Context()
	in, _, ok, err := svc.fogRow(ctx, c.campaign, c.mapID)
	if err != nil || !ok {
		t.Fatalf("fogRow: ok=%v err=%v", ok, err)
	}
	points, err := svc.queries.ListMapPoints(ctx, c.mapID)
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := svc.queries.ListMapTokens(ctx, c.mapID)
	if err != nil {
		t.Fatal(err)
	}
	svc.lits.forget(c.mapID)
	bl := &blockLayers{DBTX: c.h.pool, started: make(chan struct{})}
	svc.queries = mapsdb.New(bl)

	ctxA, cancelA := context.WithCancel(ctx)
	defer cancelA()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = svc.newSight(ctxA, nil, in, points, tokens)
	}()
	select {
	case <-bl.started:
	case <-time.After(10 * time.Second):
		t.Fatal("first compile never started")
	}
	var errB error
	go func() {
		defer wg.Done()
		_, errB = svc.newSight(ctx, nil, in, points, tokens)
	}()
	time.Sleep(300 * time.Millisecond)
	cancelA()
	wg.Wait()
	if errB != nil {
		t.Fatalf("a waiter with a live context got %v (canceled=%v); want a valid scene", errB, errors.Is(errB, context.Canceled))
	}
}
