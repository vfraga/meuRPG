package voice

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/PuraFome/meuRPG/backend/internal/platform/safego"
)

// TailTimeout is the longest the detached making of a voice may run: the call to the model
// (at most 2 minutes), the store and the mark, with room. Without it a voice nobody waits for any
// more would run as long as the model answers, holding the gate every other voice queues behind.
const TailTimeout = 3 * time.Minute

// Flights makes a line's voice once for the readers who ask for it at the same time. The work
// is not tied to the first reader (the others who wait for it still want it), but it is tied to all
// of them: it has a timeout of its own, and it is canceled when the last reader leaves, since nobody
// is left to hear it. A reader who asks again later starts a new flight.
type Flights struct {
	mu sync.Mutex
	m  map[string]*flight
}

type flight struct {
	cancel  context.CancelFunc
	waiters int
	done    chan struct{}
	err     error
}

// ErrPanicked is what the readers of a flight get when its work panics.
var ErrPanicked = errors.New("voice: making the voice failed")

// Wait joins the flight of id, starting it with work (which runs on its own context, detached from
// ctx and bounded by timeout) when there is none, and returns what the flight returned, or ctx's
// error when ctx ends first. The last reader to leave cancels the flight.
func (v *Flights) Wait(ctx context.Context, id string, timeout time.Duration, work func(context.Context) error) error {
	v.mu.Lock()
	f := v.m[id]
	if f == nil {
		if v.m == nil {
			v.m = map[string]*flight{}
		}
		runCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
		f = &flight{cancel: cancel, done: make(chan struct{})}
		v.m[id] = f
		safego.Go(nil, "make a voice", func() { v.run(runCtx, id, f, work) })
	}
	f.waiters++
	v.mu.Unlock()
	select {
	case <-f.done:
		return f.err
	case <-ctx.Done():
		v.leave(id, f)
		return ctx.Err()
	}
}

func (v *Flights) run(ctx context.Context, id string, f *flight, work func(context.Context) error) {
	defer f.cancel()
	err := ErrPanicked
	// The flight ends even when work panics: the readers get an error, not a hang.
	defer func() {
		v.mu.Lock()
		f.err = err
		if v.m[id] == f {
			delete(v.m, id)
		}
		v.mu.Unlock()
		close(f.done)
	}()
	err = work(ctx)
}

// leave takes one reader off the flight; the last one cancels it and forgets it, so a reader who
// comes next starts a new flight instead of joining one that was called off.
func (v *Flights) leave(id string, f *flight) {
	v.mu.Lock()
	defer v.mu.Unlock()
	f.waiters--
	if f.waiters > 0 {
		return
	}
	f.cancel()
	if v.m[id] == f {
		delete(v.m, id)
	}
}
