package tasks

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/PuraFome/meuRPG/backend/internal/platform/safego"
)

// The endpoints of the two services the runner talks to. The fields that hold them are
// unexported on purpose, as in platform/blob: only a test in this package can point the
// runner somewhere else, so no environment variable can send the access token to another host.
const (
	cloudTasksAPIBase      = "https://cloudtasks.googleapis.com"
	cloudTasksMetadataBase = "http://metadata.google.internal"
	cloudTasksTokenPath    = "/computeMetadata/v1/instance/service-accounts/default/token"
)

// EndpointPrefix is where the endpoint serves the tasks: EndpointPrefix + kind.
const EndpointPrefix = "/internal/tasks/"

// Cloud Tasks accepts a dispatch deadline of 15 seconds to 30 minutes for an HTTP target
// (https://cloud.google.com/tasks/docs/reference/rest/v2/projects.locations.queues.tasks, field
// dispatchDeadline). Cloud Run's request timeout is longer (35 minutes), but no task can ask
// for more than this, so a kind's Deadline is capped at it.
const (
	MaxDeadline         = 30 * time.Minute
	minDispatchDeadline = 15 * time.Second
)

const (
	cloudTasksMetadataTimeout = 5 * time.Second
	cloudTasksCallTimeout     = 15 * time.Second
	cloudTasksTLSTimeout      = 10 * time.Second
	cloudTasksIdleTimeout     = 90 * time.Second
	cloudTasksTokenMargin     = time.Minute
	cloudTasksMaxTokenBody    = 64 << 10
	cloudTasksRetryPause      = 150 * time.Millisecond
	cloudTasksAttempts        = 3
	maxTaskNameLen            = 500
	maxPayloadBytes           = 64 << 10
)

var (
	// queueName is the shape of a queue's full name.
	queueName = regexp.MustCompile(`^projects/[a-z][a-z0-9:.-]{3,60}/locations/[a-z0-9-]{3,40}/queues/[A-Za-z0-9_-]{1,100}$`)
	// kindName is the shape of a Kind: it is part of the task's URL.
	kindName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	// taskID is what Cloud Tasks allows in the id of a task.
	taskID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,500}$`)
	// serviceAccountEmail is the loose shape of an e-mail address.
	serviceAccountEmail = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
)

// Config of the Cloud Tasks runner: the three TASKS_* variables.
type Config struct {
	// Queue is the full name of the queue: projects/P/locations/L/queues/Q.
	Queue string
	// TargetURL is the service's own https address (scheme and host, no path): where Cloud Tasks
	// POSTs, and the audience of the OIDC token.
	TargetURL string
	// ServiceAccount is the e-mail of the account that signs the OIDC token: the service's own.
	ServiceAccount string
}

// CloudTasks is the Runner for production: Enqueue creates a task on a Cloud Tasks queue, over
// its REST API with net/http (one call does not justify the client library and its
// dependencies), and Cloud Tasks calls the service back at EndpointPrefix + kind with an OIDC
// token, which Handler checks before it runs the handler of the kind. The access token comes
// from the metadata server. Neither a token nor a response body ever goes into an error or a log.
//
// # Sweeps
//
// Every(kind, interval) is a chain of named tasks, one per slot of interval: the task of slot N
// is named "sweep-<kind>-N" (N = unix seconds / interval) and is due at the start of slot N.
// Start enqueues the next slot; when a delivery of a scheduled kind arrives, the endpoint
// enqueues the slot after the current one first and then runs the handler, so a failing handler
// does not break the chain. ALREADY_EXISTS counts as success: any number of instances may start
// or receive a delivery, and each slot still has exactly one task. The name carries the slot
// because Cloud Tasks refuses a task id again for about an hour after the task ran. A scheduled
// handler therefore runs at most one interval after the previous one, plus the queue's latency.
type CloudTasks struct {
	cfg    Config
	logger *slog.Logger

	apiBase      string
	metadataBase string
	client       *http.Client
	metadata     *http.Client
	pause        time.Duration
	now          func() time.Time

	check *tokenChecker

	base   context.Context
	cancel context.CancelFunc

	running atomic.Int64
	wg      sync.WaitGroup

	mu       sync.Mutex
	handlers map[Kind]entry
	every    []Kind
	started  bool

	tokenMu sync.Mutex // guards token and expires
	token   string
	expires time.Time
}

type entry struct {
	// kind is the kind as the code registered it. After the lookup the endpoint uses it, never the
	// path it read: what reaches a log line or a task name is then a constant of the code.
	kind     Kind
	h        Handler
	opts     Options
	interval time.Duration // > 0 for a scheduled kind
}

// The compiler checks that CloudTasks is a Runner.
var _ Runner = (*CloudTasks)(nil)

// NewCloudTasks returns the runner of cfg. It makes no call. The OIDC check of the endpoint
// trusts tokens issued by accounts.google.com for TargetURL and ServiceAccount.
func NewCloudTasks(cfg Config, logger *slog.Logger) (*CloudTasks, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if logger == nil {
		logger = slog.Default()
	}
	cfg.TargetURL = strings.TrimSuffix(cfg.TargetURL, "/")
	api := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   cloudTasksTLSTimeout,
		IdleConnTimeout:       cloudTasksIdleTimeout,
		ResponseHeaderTimeout: cloudTasksCallTimeout,
	}
	base, cancel := context.WithCancel(context.Background())
	c := &CloudTasks{
		cfg:          cfg,
		logger:       logger,
		apiBase:      cloudTasksAPIBase,
		metadataBase: cloudTasksMetadataBase,
		client:       &http.Client{Transport: api},
		metadata:     &http.Client{Transport: &http.Transport{DisableKeepAlives: true}},
		pause:        cloudTasksRetryPause,
		now:          time.Now,
		base:         base,
		cancel:       cancel,
		handlers:     map[Kind]entry{},
	}
	c.check = newTokenChecker(googleIssuer, googleJWKS, cfg.TargetURL, cfg.ServiceAccount, nil, c.now)
	return c, nil
}

func (cfg Config) validate() error {
	var missing []string
	for _, f := range [...]struct{ name, value string }{
		{"queue", cfg.Queue}, {"target URL", cfg.TargetURL}, {"service account", cfg.ServiceAccount},
	} {
		if f.value == "" {
			missing = append(missing, f.name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("tasks: the Cloud Tasks configuration lacks the %s", strings.Join(missing, ", "))
	}
	if !queueName.MatchString(cfg.Queue) {
		return errors.New("tasks: the queue is not a full queue name (projects/P/locations/L/queues/Q)")
	}
	if err := checkTarget(cfg.TargetURL); err != nil {
		return err
	}
	if !serviceAccountEmail.MatchString(cfg.ServiceAccount) {
		return errors.New("tasks: the service account is not an e-mail address")
	}
	return nil
}

func checkTarget(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return errors.New("tasks: the target URL must be a scheme and a host, without path or query")
	}
	if u.Scheme == "https" || (u.Scheme == "http" && isLoopback(u.Hostname())) {
		return nil
	}
	return errors.New("tasks: the target URL must be https")
}

func isLoopback(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1" || strings.HasSuffix(host, ".localhost")
}

// Cancel ends the context of every running handler: the shutdown.
func (c *CloudTasks) Cancel() { c.cancel() }

// Register adds the handler of a kind. The kind must be lowercase letters, digits and dashes.
// A Deadline above MaxDeadline is capped at it, which Cloud Tasks does not exceed.
func (c *CloudTasks) Register(kind Kind, h Handler, opts Options) {
	c.register(kind, h, opts, 0)
}

func (c *CloudTasks) register(kind Kind, h Handler, opts Options, interval time.Duration) {
	if !kindName.MatchString(string(kind)) {
		panic(fmt.Sprintf("tasks: the kind %q is not lowercase letters, digits and dashes", kind))
	}
	if opts.Deadline <= 0 {
		opts.Deadline = defaultDeadline
	}
	opts.Deadline = min(opts.Deadline, MaxDeadline)
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, dup := c.handlers[kind]; dup {
		panic(fmt.Sprintf("tasks: the kind %q is registered twice", kind))
	}
	c.handlers[kind] = entry{kind: kind, h: h, opts: opts, interval: interval}
}

func (c *CloudTasks) lookup(kind Kind) (entry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.handlers[kind]
	return e, ok
}

// Every registers h on kind as a chain of named tasks, one per interval (see the type's
// documentation). The interval is a whole number of seconds, one second at least.
func (c *CloudTasks) Every(kind Kind, interval time.Duration, h Handler, opts Options) {
	if interval < time.Second {
		panic(fmt.Sprintf("tasks: the interval of %q is under a second", kind))
	}
	interval = interval.Truncate(time.Second)
	c.register(kind, h, opts, interval)
	c.mu.Lock()
	c.every = append(c.every, kind)
	c.mu.Unlock()
}

// Start enqueues the next slot of every scheduled kind, in the background, and returns. A slot
// that already has its task (another instance started first) is fine.
func (c *CloudTasks) Start(ctx context.Context) {
	c.mu.Lock()
	if c.started {
		c.mu.Unlock()
		return
	}
	c.started = true
	kinds := append([]Kind(nil), c.every...)
	c.mu.Unlock()
	for _, kind := range kinds {
		c.wg.Go(func() {
			defer safego.Recover(c.logger, "schedule "+string(kind))
			ctx, cancel := context.WithTimeout(ctx, 4*cloudTasksCallTimeout)
			defer cancel()
			if err := c.enqueueNext(ctx, kind); err != nil {
				c.logger.WarnContext(ctx, "tasks: the next sweep could not be scheduled; it is scheduled again by the next instance that starts", "kind", string(kind), "error", err)
			}
		})
	}
}

// slot returns the name and the due time of the task of the slot after now.
func slot(kind Kind, interval time.Duration, now time.Time) (string, time.Time) {
	secs := int64(interval / time.Second)
	next := now.Unix()/secs + 1
	return fmt.Sprintf("sweep-%s-%d", kind, next), time.Unix(next*secs, 0)
}

// enqueueNext creates the task of the slot after now for a scheduled kind.
func (c *CloudTasks) enqueueNext(ctx context.Context, kind Kind) error {
	e, ok := c.lookup(kind)
	if !ok || e.interval <= 0 {
		return fmt.Errorf("tasks: %q is not a scheduled kind", kind)
	}
	name, at := slot(kind, e.interval, c.now())
	err := c.Enqueue(ctx, Job{Kind: kind, Payload: struct{}{}, Name: name, At: at})
	if errors.Is(err, ErrExists) {
		return nil
	}
	return err
}

// Wait waits for the handlers running in this process, up to ctx.
func (c *CloudTasks) Wait(ctx context.Context) {
	done := make(chan struct{})
	safego.Go(c.logger, "wait for the task handlers", func() {
		defer close(done)
		c.wg.Wait()
		for c.running.Load() > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(10 * time.Millisecond):
			}
		}
	})
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// createRequest is the body of tasks.create.
type createRequest struct {
	Task taskResource `json:"task"`
}

type taskResource struct {
	Name             string      `json:"name,omitempty"`
	ScheduleTime     string      `json:"scheduleTime,omitempty"`
	DispatchDeadline string      `json:"dispatchDeadline"`
	HTTPRequest      httpRequest `json:"httpRequest"`
}

type httpRequest struct {
	URL        string            `json:"url"`
	HTTPMethod string            `json:"httpMethod"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
	OIDCToken  oidcToken         `json:"oidcToken"`
}

type oidcToken struct {
	ServiceAccountEmail string `json:"serviceAccountEmail"`
	Audience            string `json:"audience"`
}

// Enqueue creates the task on the queue. A named task that exists (or ran lately) is ErrExists.
func (c *CloudTasks) Enqueue(ctx context.Context, job Job) error {
	e, ok := c.lookup(job.Kind)
	if !ok {
		return fmt.Errorf("tasks: no handler is registered for %q", job.Kind)
	}
	payload, err := json.Marshal(job.Payload)
	if err != nil {
		return fmt.Errorf("tasks: encode the payload of %q: %w", job.Kind, err)
	}
	if len(payload) > maxPayloadBytes {
		return fmt.Errorf("tasks: the payload of %q is too large", job.Kind)
	}
	task := taskResource{
		DispatchDeadline: fmt.Sprintf("%ds", int(max(e.opts.Deadline, minDispatchDeadline)/time.Second)),
		HTTPRequest: httpRequest{
			URL:        c.cfg.TargetURL + EndpointPrefix + string(job.Kind),
			HTTPMethod: http.MethodPost,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       base64.StdEncoding.EncodeToString(payload),
			OIDCToken:  oidcToken{ServiceAccountEmail: c.cfg.ServiceAccount, Audience: c.cfg.TargetURL},
		},
	}
	if job.Name != "" {
		if !taskID.MatchString(job.Name) {
			return errors.New("tasks: the task name is not letters, digits, hyphens and underscores (500 at most)")
		}
		task.Name = c.cfg.Queue + "/tasks/" + job.Name
	}
	if !job.At.IsZero() && job.At.After(c.now()) {
		task.ScheduleTime = job.At.UTC().Format(time.RFC3339)
	}
	body, err := json.Marshal(createRequest{Task: task})
	if err != nil {
		return errors.New("tasks: cloud tasks create: cannot encode the request")
	}
	return c.create(ctx, body)
}

// create posts the task, again after a network error, a 429 or a 5xx (three attempts in all).
func (c *CloudTasks) create(ctx context.Context, body []byte) error {
	var lastErr error
	for attempt := range cloudTasksAttempts {
		if attempt > 0 {
			select {
			case <-time.After(c.pause * time.Duration(attempt)):
			case <-ctx.Done():
				return fmt.Errorf("tasks: cloud tasks create: %w", ctx.Err())
			}
		}
		status, err := c.createOnce(ctx, body)
		done, err := judgeCreate(ctx, status, err)
		if done {
			return err
		}
		lastErr = err
	}
	return lastErr
}

// judgeCreate says what one attempt means: done with its answer (nil, ErrExists or a final
// error), or not done, with the error of a failure another attempt may fix.
func judgeCreate(ctx context.Context, status int, err error) (bool, error) {
	switch {
	case err != nil:
		return ctx.Err() != nil, err
	case status == http.StatusOK:
		return true, nil
	case status == http.StatusConflict:
		return true, ErrExists
	default:
		retry := status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
		return !retry, statusError("create", status)
	}
}

func (c *CloudTasks) createOnce(ctx context.Context, body []byte) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, cloudTasksCallTimeout)
	defer cancel()
	token, err := c.accessToken(ctx)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiBase+"/v2/"+c.cfg.Queue+"/tasks", bytes.NewReader(body))
	if err != nil {
		return 0, errors.New("tasks: cloud tasks create: cannot build the request")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return 0, networkError("create", err)
	}
	defer func() { drainAndClose(resp.Body) }()
	if resp.StatusCode == http.StatusUnauthorized {
		c.forgetToken()
	}
	return resp.StatusCode, nil
}

// accessToken returns the service account's token, from the cache while it has more than a
// margin left, else from the metadata server.
func (c *CloudTasks) accessToken(ctx context.Context) (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	if c.token != "" && c.now().Before(c.expires) {
		return c.token, nil
	}
	token, lifetime, err := c.fetchToken(ctx)
	if err != nil {
		return "", err
	}
	margin := min(cloudTasksTokenMargin, lifetime/2)
	c.token, c.expires = token, c.now().Add(lifetime-margin)
	return token, nil
}

func (c *CloudTasks) forgetToken() {
	c.tokenMu.Lock()
	c.token, c.expires = "", time.Time{}
	c.tokenMu.Unlock()
}

// fetchToken reads the token from the metadata server, once more after a network error or a 5xx.
func (c *CloudTasks) fetchToken(ctx context.Context) (string, time.Duration, error) {
	var lastErr error
	for attempt := range 2 {
		if attempt > 0 {
			select {
			case <-time.After(c.pause):
			case <-ctx.Done():
				return "", 0, fmt.Errorf("tasks: access token: %w", ctx.Err())
			}
		}
		token, lifetime, retry, err := c.fetchTokenOnce(ctx)
		if err == nil {
			return token, lifetime, nil
		}
		lastErr = err
		if !retry || ctx.Err() != nil {
			break
		}
	}
	return "", 0, lastErr
}

func (c *CloudTasks) fetchTokenOnce(ctx context.Context) (token string, lifetime time.Duration, retry bool, err error) {
	ctx, cancel := context.WithTimeout(ctx, cloudTasksMetadataTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.metadataBase+cloudTasksTokenPath, nil)
	if err != nil {
		return "", 0, false, errors.New("tasks: access token: cannot build the request")
	}
	req.Header.Set("Metadata-Flavor", "Google")
	resp, err := c.metadata.Do(req)
	if err != nil {
		return "", 0, true, networkError("access token", err)
	}
	defer func() { drainAndClose(resp.Body) }()
	if resp.StatusCode != http.StatusOK {
		return "", 0, resp.StatusCode >= http.StatusInternalServerError, statusError("access token", resp.StatusCode)
	}
	var answer struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, cloudTasksMaxTokenBody)).Decode(&answer); err != nil {
		return "", 0, false, errors.New("tasks: access token: the metadata server's answer cannot be read")
	}
	if answer.AccessToken == "" || answer.ExpiresIn <= 0 {
		return "", 0, false, errors.New("tasks: access token: the metadata server's answer has no token")
	}
	return answer.AccessToken, time.Duration(answer.ExpiresIn) * time.Second, false, nil
}

func networkError(op string, err error) error {
	if ue, ok := errors.AsType[*url.Error](err); ok {
		err = ue.Err
	}
	return fmt.Errorf("tasks: cloud tasks %s: %w", op, err)
}

func statusError(op string, status int) error {
	return fmt.Errorf("tasks: cloud tasks %s: status %d", op, status)
}

func drainAndClose(body io.ReadCloser) {
	_, _ = io.Copy(io.Discard, io.LimitReader(body, 4<<10))
	_ = body.Close()
}

var errPanicked = errors.New("tasks: the handler panicked")
