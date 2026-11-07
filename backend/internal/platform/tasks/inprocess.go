package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/PuraFome/meuRPG/backend/internal/platform/safego"
)

// defaultDeadline is the deadline of a kind registered without one.
const defaultDeadline = 10 * time.Minute

// InProcess is the Runner that runs each task in a goroutine of this process: the way the
// detached work always ran, used locally, in tests, in CI's e2e and whenever the Cloud Tasks
// configuration is absent. Shutdown waits for the goroutines (Wait). It retries nothing: an
// error is logged, as a failure of detached work always was.
type InProcess struct {
	logger *slog.Logger
	base   context.Context
	cancel context.CancelFunc

	wg sync.WaitGroup

	mu       sync.Mutex
	handlers map[Kind]registered
	names    map[string]struct{}
	every    []scheduled
	started  bool
}

type registered struct {
	h    Handler
	opts Options
}

type scheduled struct {
	kind     Kind
	interval time.Duration
}

// The compiler checks that InProcess is a Runner.
var _ Runner = (*InProcess)(nil)

// NewInProcess returns an in-process Runner. Cancel it (or end the context of Start) at
// shutdown, then Wait.
func NewInProcess(logger *slog.Logger) *InProcess {
	if logger == nil {
		logger = slog.Default()
	}
	base, cancel := context.WithCancel(context.Background())
	return &InProcess{logger: logger, base: base, cancel: cancel, handlers: map[Kind]registered{}, names: map[string]struct{}{}}
}

// Cancel ends the context of every running handler: the shutdown.
func (p *InProcess) Cancel() { p.cancel() }

// Register adds the handler of a kind.
func (p *InProcess) Register(kind Kind, h Handler, opts Options) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, dup := p.handlers[kind]; dup {
		panic(fmt.Sprintf("tasks: the kind %q is registered twice", kind))
	}
	if opts.Deadline <= 0 {
		opts.Deadline = defaultDeadline
	}
	p.handlers[kind] = registered{h: h, opts: opts}
}

// Enqueue runs the task in a goroutine, after Job.At when that is in the future.
func (p *InProcess) Enqueue(_ context.Context, job Job) error {
	payload, err := json.Marshal(job.Payload)
	if err != nil {
		return fmt.Errorf("tasks: encode the payload of %q: %w", job.Kind, err)
	}
	p.mu.Lock()
	reg, ok := p.handlers[job.Kind]
	if !ok {
		p.mu.Unlock()
		return fmt.Errorf("tasks: no handler is registered for %q", job.Kind)
	}
	if job.Name != "" {
		if _, dup := p.names[job.Name]; dup {
			p.mu.Unlock()
			return ErrExists
		}
		p.names[job.Name] = struct{}{}
	}
	p.mu.Unlock()
	p.wg.Add(1)
	safego.Go(p.logger, "task "+string(job.Kind), func() { p.run(job.Kind, reg, payload, job.At) })
	return nil
}

func (p *InProcess) run(kind Kind, reg registered, payload json.RawMessage, at time.Time) {
	defer p.wg.Done()
	if wait := time.Until(at); !at.IsZero() && wait > 0 {
		select {
		case <-time.After(wait):
		case <-p.base.Done():
			return
		}
	}
	ctx, cancel := context.WithTimeout(p.base, reg.opts.Deadline)
	defer cancel()
	p.log(ctx, kind, reg.h(ctx, payload))
}

func (p *InProcess) log(ctx context.Context, kind Kind, err error) {
	switch {
	case err == nil:
	case IsPermanent(err):
		p.logger.InfoContext(ctx, "tasks: a task ended with a failure a retry cannot help", "kind", string(kind), "error", err)
	default:
		p.logger.WarnContext(ctx, "tasks: a task failed", "kind", string(kind), "error", err)
	}
}

// Every registers h on kind and runs it at Start and then every interval.
func (p *InProcess) Every(kind Kind, interval time.Duration, h Handler, opts Options) {
	p.Register(kind, h, opts)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.every = append(p.every, scheduled{kind: kind, interval: interval})
}

// Start begins the tickers of the scheduled kinds, until ctx ends or Cancel is called.
func (p *InProcess) Start(ctx context.Context) {
	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return
	}
	p.started = true
	every := append([]scheduled(nil), p.every...)
	p.mu.Unlock()
	for _, s := range every {
		p.wg.Add(1)
		safego.Go(p.logger, "schedule "+string(s.kind), func() { p.tick(ctx, s) })
	}
}

func (p *InProcess) tick(ctx context.Context, s scheduled) {
	defer p.wg.Done()
	p.mu.Lock()
	reg := p.handlers[s.kind]
	p.mu.Unlock()
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.base.Done():
			return
		case <-timer.C:
		}
		func() {
			run, cancel := context.WithTimeout(p.base, reg.opts.Deadline)
			defer cancel()
			p.log(run, s.kind, reg.h(run, json.RawMessage("{}")))
		}()
		timer.Reset(s.interval)
	}
}

// Wait waits for the running handlers and tickers, up to ctx.
func (p *InProcess) Wait(ctx context.Context) {
	done := make(chan struct{})
	safego.Go(p.logger, "wait for the tasks", func() { defer close(done); p.wg.Wait() })
	select {
	case <-done:
	case <-ctx.Done():
	}
}
