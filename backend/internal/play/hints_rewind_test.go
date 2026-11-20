package play

import (
	"context"
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/play/live"
)

// A savepoint that is rolled back takes the hints its statements collected with it: hints.rewind puts back what mark kept.
func TestHintsRewindForgetsWhatWasToldAfterTheMark(t *testing.T) {
	t.Parallel()
	var h hints
	h.tell("c1", live.Event{})
	h.after(func(context.Context, func(string, live.Event)) {})
	h.heldTold = true
	m := h.mark()
	h.tell("c2", live.Event{})
	h.after(func(context.Context, func(string, live.Event)) {})
	h.touch()
	h.heldTold = false
	h.replayed = true
	h.rewind(m)
	if len(h.events) != 1 || h.events[0].campaignID != "c1" || len(h.afters) != 1 {
		t.Errorf("after the rewind: %d events (%v), %d afters; want the first event and the first step only", len(h.events), h.events, len(h.afters))
	}
	if h.touched || !h.heldTold || h.replayed {
		t.Errorf("after the rewind: touched %v, heldTold %v, replayed %v; want false, true, false", h.touched, h.heldTold, h.replayed)
	}
}
