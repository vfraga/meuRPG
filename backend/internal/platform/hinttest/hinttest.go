// Package hinttest tracks the trailing sends of the services' hint gates, so a test waits for "no hint
// is pending" instead of sleeping past a gate's interval.
package hinttest

import (
	"sync"
	"testing"
	"time"
)

// Tracker counts the sends the play service's gate (After) and the maps layer gate (AfterTimer) have scheduled and not yet
// made, so a test can wait for the state "no trailing hint is pending" instead of sleeping
// past the gate's interval. The sends still run on the real clock.
type Tracker struct {
	mu      sync.Mutex
	pending int
	idle    chan struct{} // closed when pending drops to zero; nil while nothing waits
}

// After is the gate's time.AfterFunc: it counts the send until it has been made.
func (h *Tracker) After(d time.Duration, f func()) {
	h.AfterTimer(d, f)
}

// AfterTimer is After for a gate that wants the timer back (the maps service's).
func (h *Tracker) AfterTimer(d time.Duration, f func()) *time.Timer {
	h.mu.Lock()
	h.pending++
	h.mu.Unlock()
	return time.AfterFunc(d, func() {
		defer h.done()
		f()
	})
}

func (h *Tracker) done() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pending--
	if h.pending == 0 && h.idle != nil {
		close(h.idle)
		h.idle = nil
	}
}

// Settle returns once every scheduled hint send has been made.
func (h *Tracker) Settle(t *testing.T) {
	t.Helper()
	h.mu.Lock()
	if h.pending == 0 {
		h.mu.Unlock()
		return
	}
	if h.idle == nil {
		h.idle = make(chan struct{})
	}
	idle := h.idle
	h.mu.Unlock()
	select {
	case <-idle:
	case <-time.After(30 * time.Second):
		t.Fatal("the hint gate's trailing send never went out")
	}
}
