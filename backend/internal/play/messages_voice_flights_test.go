package play

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// Readers who ask for the same line together share one making of its voice.
func TestVoiceFlightsShareOneMaking(t *testing.T) {
	t.Parallel()
	var v voiceFlights
	var calls atomic.Int32
	release := make(chan struct{})
	work := func(context.Context) error {
		calls.Add(1)
		<-release
		return nil
	}
	errs := make(chan error, 2)
	for range 2 {
		go func() { errs <- v.wait(t.Context(), "line", time.Minute, work) }()
	}
	waitFor(t, "both readers wait", func() bool {
		v.mu.Lock()
		defer v.mu.Unlock()
		f := v.m["line"]
		return f != nil && f.waiters == 2
	})
	close(release)
	for range 2 {
		if err := <-errs; err != nil {
			t.Errorf("wait() error = %v, want nil", err)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("the voice was made %d times, want 1", got)
	}
}

// The detached making has a time of its own: when it is up the work is told to stop and the reader
// who waits gets its answer.
func TestVoiceFlightsTailHasATimeout(t *testing.T) {
	t.Parallel()
	var v voiceFlights
	err := v.wait(t.Context(), "line", 30*time.Millisecond, func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("wait() error = %v, want the deadline of the making", err)
	}
}

// The making goes on while one reader still waits and is canceled when the last one leaves.
func TestVoiceFlightsLastReaderCancelsTheTail(t *testing.T) {
	t.Parallel()
	var v voiceFlights
	started, stopped := make(chan struct{}), make(chan struct{})
	work := func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		close(stopped)
		return ctx.Err()
	}
	first, cancelFirst := context.WithCancel(t.Context())
	second, cancelSecond := context.WithCancel(t.Context())
	defer cancelSecond()
	firstDone, secondDone := make(chan error, 1), make(chan error, 1)
	go func() { firstDone <- v.wait(first, "line", time.Minute, work) }()
	<-started
	go func() { secondDone <- v.wait(second, "line", time.Minute, work) }()
	waitFor(t, "the second reader joined", func() bool {
		v.mu.Lock()
		defer v.mu.Unlock()
		return v.m["line"] != nil && v.m["line"].waiters == 2
	})
	cancelFirst()
	if err := <-firstDone; !errors.Is(err, context.Canceled) {
		t.Errorf("the first reader's error = %v, want canceled", err)
	}
	select {
	case <-stopped:
		t.Fatal("the making stopped while a reader still waited")
	case <-time.After(100 * time.Millisecond):
	}
	cancelSecond()
	if err := <-secondDone; !errors.Is(err, context.Canceled) {
		t.Errorf("the second reader's error = %v, want canceled", err)
	}
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("the making was not canceled when the last reader left")
	}
}

// A reader who comes after the last one left starts a new making; it does not join the one that was
// called off.
func TestVoiceFlightsANewReaderAfterTheCancelStartsAgain(t *testing.T) {
	t.Parallel()
	var v voiceFlights
	var calls atomic.Int32
	hold := make(chan struct{})
	work := func(ctx context.Context) error {
		if calls.Add(1) == 1 {
			<-ctx.Done()
			<-hold // winding down slowly
			return ctx.Err()
		}
		return nil
	}
	gone, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- v.wait(gone, "line", time.Minute, work) }()
	waitFor(t, "the first making started", func() bool { return calls.Load() == 1 })
	cancel()
	<-done
	if err := v.wait(t.Context(), "line", time.Minute, work); err != nil {
		t.Errorf("the new reader's error = %v, want nil", err)
	}
	close(hold)
	if got := calls.Load(); got != 2 {
		t.Errorf("the voice was made %d times, want 2 (the called-off one and the new one)", got)
	}
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting: %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestVoiceFlightsAPanickingWorkReturnsAnErrorToTheWaiter(t *testing.T) {
	t.Parallel()
	var v voiceFlights
	got := make(chan error, 1)
	go func() {
		got <- v.wait(context.Background(), "line-1", time.Minute, func(context.Context) error { panic("bad voice") })
	}()
	select {
	case err := <-got:
		if !errors.Is(err, errVoicePanicked) {
			t.Errorf("err = %v, want errVoicePanicked", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the waiter hangs after the work panicked")
	}
	// The flight is forgotten: a new reader starts a new one.
	if err := v.wait(context.Background(), "line-1", time.Minute, func(context.Context) error { return nil }); err != nil {
		t.Errorf("next flight err = %v, want nil", err)
	}
}
