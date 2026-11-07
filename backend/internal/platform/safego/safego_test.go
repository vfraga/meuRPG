package safego_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/PuraFome/meuRPG/backend/internal/platform/safego"
)

func TestRecoverStopsAPanicAndLogsIt(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	failed := false

	func() {
		defer safego.Recover(logger, "test job", func() { failed = true })
		panic("bad data")
	}()

	if !failed {
		t.Error("onPanic did not run")
	}
	for _, want := range []string{"level=ERROR", "test job", "bad data", "stack="} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("log %q misses %q", logs.String(), want)
		}
	}
}

func TestRecoverDoesNothingWithoutAPanic(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	func() { defer safego.Recover(slog.New(slog.NewTextHandler(&logs, nil)), "calm job") }()
	if logs.Len() != 0 {
		t.Errorf("logged %q without a panic", logs.String())
	}
}

func TestGoLogsAPanicAndTheProcessLives(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	failed := make(chan struct{})

	safego.Go(logger, "panicking job", func() { panic("bad data") }, func() { close(failed) })
	<-failed // onPanic runs after the log line, so the log is complete here

	for _, want := range []string{"level=ERROR", "panicking job", "bad data", "stack="} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("log %q misses %q", logs.String(), want)
		}
	}
}

func TestGoRunsAFunctionThatDoesNotPanic(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		var logs bytes.Buffer
		ran, onPanicRan := false, false

		safego.Go(slog.New(slog.NewTextHandler(&logs, nil)), "calm job", func() { ran = true }, func() { onPanicRan = true })
		synctest.Wait() // the goroutine, Recover included, has ended

		if !ran {
			t.Error("fn did not run")
		}
		if onPanicRan || logs.Len() != 0 {
			t.Errorf("onPanic ran or logged %q without a panic", logs.String())
		}
	})
}
