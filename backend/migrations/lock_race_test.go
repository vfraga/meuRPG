package migrations

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestTryAcquireRacersNeverFail: runs that send the lock statement together
// all get an answer. One wins and the rest hear "held", even when the database
// may abort a racer with 40001.
func TestTryAcquireRacersNeverFail(t *testing.T) {
	t.Parallel()
	db := freshDatabase(t)
	if err := createLockTable(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	const racers = 12
	var ready, start sync.WaitGroup
	start.Add(1)
	var won atomic.Int32
	var wg sync.WaitGroup
	for i := range racers {
		ready.Add(1)
		wg.Go(func() {
			ready.Done()
			start.Wait()
			got, err := tryAcquire(t.Context(), db, fmt.Sprintf("racer-%d", i), time.Minute)
			if err != nil {
				t.Errorf("tryAcquire() error = %v", err)
				return
			}
			if got {
				won.Add(1)
			}
		})
	}
	ready.Wait()
	start.Done()
	wg.Wait()
	if won.Load() != 1 {
		t.Errorf("%d racers won the lock, want 1", won.Load())
	}
}
