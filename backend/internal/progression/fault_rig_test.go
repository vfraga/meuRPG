package progression

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	progressionv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/progression/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/progression/v1/progressionv1connect"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
	"github.com/PuraFome/meuRPG/backend/internal/platform/goldentest"
	"github.com/PuraFome/meuRPG/backend/internal/platform/httpserver"
	"github.com/PuraFome/meuRPG/backend/internal/progression/link"
)

// The rig of the award, undo, history and milestone tests: a table whose
// members call a progression service of its own, on a pool of its own, that
// logs into a buffer and can fail once any call it makes to another module
// (by name), or any query of progressiondb (by its sqlc name,
// dbtest.QueryFaults), so each branch of a read or a write that can fail is
// reached.

// errInjected is the failure an armed fault of another module returns.
var errInjected = errors.New("injected failure")

// faults arms one-shot failures of the calls to another module by name, holds
// a hook that runs the first time the service reads the clock (inside an
// award's transaction, after every read and right before the award is
// written), and arms the failures of queries.
type faults struct {
	mu          sync.Mutex
	armed       map[string]error
	beforeWrite func()
	queries     *dbtest.QueryFaults
}

// arm makes the next call named name fail with err (errInjected when none).
func (f *faults) arm(name string, err ...error) {
	cause := errInjected
	if len(err) > 0 {
		cause = err[0]
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.armed == nil {
		f.armed = map[string]error{}
	}
	f.armed[name] = cause
}

// armQuery makes the run of the query that comes after skip others fail.
func (f *faults) armQuery(name string, skip int) { f.queries.ArmOnce(name, skip) }

// take returns the armed failure of the call named name, once.
func (f *faults) take(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	err := f.armed[name]
	delete(f.armed, name)
	return err
}

// setBeforeWrite sets the hook of the next clock read.
func (f *faults) setBeforeWrite(hook func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.beforeWrite = hook
}

// now runs the hook, once, and reads the clock.
func (f *faults) now(clock func() time.Time) func() time.Time {
	return func() time.Time {
		f.mu.Lock()
		hook := f.beforeWrite
		f.beforeWrite = nil
		f.mu.Unlock()
		if hook != nil {
			hook()
		}
		return clock()
	}
}

// pending is what is armed and has not fired yet, so a test sees a fault that
// never fired (a branch it thought it reached).
func (f *faults) pending() []string {
	f.mu.Lock()
	var out []string
	for name := range f.armed {
		out = append(out, name)
	}
	f.mu.Unlock()
	for _, name := range f.queries.Pending() {
		out = append(out, "query "+name)
	}
	sort.Strings(out)
	return out
}

// reset forgets everything armed.
func (f *faults) reset() {
	f.mu.Lock()
	f.armed = nil
	f.mu.Unlock()
	f.queries.Disarm()
}

type faultParty struct {
	inner Party
	f     *faults
}

func (p faultParty) Names(ctx context.Context, campaignID string, ids []string) (map[string]string, error) {
	if err := p.f.take("names"); err != nil {
		return nil, err
	}
	return p.inner.Names(ctx, campaignID, ids)
}

func (p faultParty) Party(ctx context.Context, tx pgx.Tx, campaignID string) ([]link.Member, error) {
	if err := p.f.take("party"); err != nil {
		return nil, err
	}
	return p.inner.Party(ctx, tx, campaignID)
}

func (p faultParty) AddExperience(ctx context.Context, tx pgx.Tx, campaignID, characterID string, delta int32, at time.Time) (int32, int32, error) {
	if err := p.f.take("add_experience"); err != nil {
		return 0, 0, err
	}
	return p.inner.AddExperience(ctx, tx, campaignID, characterID, delta, at)
}

type faultCombats struct {
	Combats
	f *faults
}

func (c faultCombats) CampaignEncounter(ctx context.Context, tx pgx.Tx, campaignID, encounterID string) (link.Encounter, error) {
	if err := c.f.take("encounter"); err != nil {
		return link.Encounter{}, err
	}
	return c.Combats.CampaignEncounter(ctx, tx, campaignID, encounterID)
}

func (c faultCombats) EncounterNames(ctx context.Context, campaignID string, ids []string) (map[string]string, error) {
	if err := c.f.take("encounter_names"); err != nil {
		return nil, err
	}
	return c.Combats.EncounterNames(ctx, campaignID, ids)
}

type faultProfiles struct {
	Profiles
	f *faults
}

func (p faultProfiles) DisplayNames(ctx context.Context, userIDs []string) (map[string]string, error) {
	if err := p.f.take("display_names"); err != nil {
		return nil, err
	}
	return p.Profiles.DisplayNames(ctx, userIDs)
}

type faultLog struct {
	SessionLog
	f *faults
}

func (l faultLog) AppendEvent(ctx context.Context, tx pgx.Tx, campaignID, kind, actorUserID string, payload []byte, at time.Time) (bool, error) {
	if err := l.f.take("append_event"); err != nil {
		return false, err
	}
	return l.SessionLog.AppendEvent(ctx, tx, campaignID, kind, actorUserID, payload, at)
}

type faultTreasures struct {
	Treasures
	f *faults
}

func (s faultTreasures) LockForConversion(ctx context.Context, tx pgx.Tx, campaignID string, pointIDs []string) ([]link.Treasure, error) {
	if err := s.f.take("lock_treasures"); err != nil {
		return nil, err
	}
	return s.Treasures.LockForConversion(ctx, tx, campaignID, pointIDs)
}

func (s faultTreasures) MarkConverted(ctx context.Context, tx pgx.Tx, awardID string, pointIDs []string) error {
	if err := s.f.take("mark_converted"); err != nil {
		return err
	}
	return s.Treasures.MarkConverted(ctx, tx, awardID, pointIDs)
}

func (s faultTreasures) Release(ctx context.Context, tx pgx.Tx, awardID string) ([]string, error) {
	if err := s.f.take("release"); err != nil {
		return nil, err
	}
	return s.Treasures.Release(ctx, tx, awardID)
}

type faultCampaigns struct {
	Campaigns
	f *faults
}

func (c faultCampaigns) CampaignXPMode(ctx context.Context, tx pgx.Tx, campaignID string) (campaignsv1.XpMode, error) {
	name := "xp_mode"
	if tx != nil {
		name = "xp_mode_in_tx"
	}
	if err := c.f.take(name); err != nil {
		return 0, err
	}
	return c.Campaigns.CampaignXPMode(ctx, tx, campaignID)
}

// logBuffer is the JSON log of the rig's service.
type logBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *logBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

// faultRig is a table served by a progression service of its own.
type faultRig struct {
	*table
	faults *faults
	logs   *logBuffer
	client func(u *user) progressionv1connect.ProgressionServiceClient
	// names labels the IDs in the goldens; unnamed UUIDs are numbered.
	names  *goldentest.Names
	events int // the session events already taken
}

// newFaultRig is newTable whose master and players call a progression service
// that logs at debug level into the rig and goes through its faults.
func newFaultRig(t *testing.T, mode campaignsv1.XpMode, n int) *faultRig {
	t.Helper()
	tb := newTable(t, mode, n)
	base := tb.h.pool.Config()
	queries := dbtest.NewQueryFaults(base.ConnConfig.Tracer)
	rig := &faultRig{table: tb, faults: &faults{queries: queries}, logs: &logBuffer{}, names: goldentest.NewNames()}
	pool, err := db.NewPoolWith(t.Context(), base.ConnString(), func(cfg *pgxpool.Config) {
		cfg.MaxConns = max(base.MaxConns, 2)
		cfg.ConnConfig.Tracer = queries
	})
	if err != nil {
		t.Fatalf("NewPoolWith() error = %v", err)
	}
	t.Cleanup(pool.Close)
	cfg := tb.h.cfg
	cfg.Pool = pool
	cfg.Logger = slog.New(slog.NewJSONHandler(rig.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cfg.Party = faultParty{inner: cfg.Party, f: rig.faults}
	cfg.Combats = faultCombats{Combats: cfg.Combats, f: rig.faults}
	cfg.Profiles = faultProfiles{Profiles: cfg.Profiles, f: rig.faults}
	cfg.Campaigns = faultCampaigns{Campaigns: cfg.Campaigns, f: rig.faults}
	cfg.Log = faultLog{SessionLog: cfg.Log, f: rig.faults}
	cfg.Treasures = faultTreasures{Treasures: cfg.Treasures, f: rig.faults}
	cfg.Now = rig.faults.now(cfg.Now)
	svc, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	srv := httpserver.New(httpserver.Config{Logger: slog.New(slog.DiscardHandler)})
	svc.Mount(srv.Handle, fakeSessions{}, tb.h.camps, connect.WithRequireConnectProtocolHeader())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	rig.client = func(u *user) progressionv1connect.ProgressionServiceClient {
		if u.id == "" {
			return progressionv1connect.NewProgressionServiceClient(ts.Client(), ts.URL)
		}
		return progressionv1connect.NewProgressionServiceClient(&http.Client{Transport: userTransport{userID: u.id, next: ts.Client().Transport}}, ts.URL)
	}
	for _, u := range append([]*user{tb.master}, tb.players...) {
		u.xp = rig.client(u)
	}
	rig.label(tb.campaign, "campaign")
	rig.label(tb.master.id, "master")
	for i, pc := range tb.pcs {
		rig.label(pc.GetId(), strings.ToLower(pc.GetName()))
		rig.label(tb.players[i].id, "player-"+strings.ToLower(pc.GetName()))
	}
	return rig
}

// label names an ID in the goldens.
func (rig *faultRig) label(id, name string) { rig.names.Label(id, name) }

// redact puts the labels in place of the IDs, and numbers the others in the
// order the rig first meets them.
func (rig *faultRig) redact(s string) string { return rig.names.Redact(s) }

// historyRead is one read of a golden: the answer as JSON, or the refusal.
func (rig *faultRig) historyRead(t *testing.T, msg proto.Message, err error) any {
	t.Helper()
	if err != nil {
		return rig.redact(refusal(err))
	}
	raw, merr := protojson.Marshal(msg)
	if merr != nil {
		t.Fatal(merr)
	}
	var tree any
	if err := json.Unmarshal([]byte(rig.redact(string(raw))), &tree); err != nil {
		t.Fatal(err)
	}
	return tree
}

// wantHistoryGolden compares what a viewer read (or what a test's steps did)
// to its file in testdata, the IDs replaced by labels and the times by their
// rank.
func (rig *faultRig) wantHistoryGolden(t *testing.T, name string, reads map[string]any) {
	t.Helper()
	raw, err := json.Marshal(reads)
	if err != nil {
		t.Fatal(err)
	}
	var tree any
	if err := json.Unmarshal([]byte(goldentest.MaskTimes(t, rig.redact(string(raw)))), &tree); err != nil {
		t.Fatal(err)
	}
	goldentest.JSON(t, "testdata/"+name+".golden.json", tree)
}

// takeLogs returns the service's log lines written since the last call, but
// the authorization's: the level, the message and the attributes sorted,
// without the time.
func (rig *faultRig) takeLogs(t *testing.T) []string {
	t.Helper()
	rig.logs.mu.Lock()
	raw := rig.logs.b.String()
	rig.logs.b.Reset()
	rig.logs.mu.Unlock()
	var out []string
	for line := range strings.SplitSeq(strings.TrimSpace(raw), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("log line %q: %v", line, err)
		}
		msg, _ := rec["msg"].(string)
		if !strings.HasPrefix(msg, "progression:") {
			continue
		}
		keys := make([]string, 0, len(rec))
		for k := range rec {
			if k != "time" && k != "level" && k != "msg" {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		parts := []string{fmt.Sprint(rec["level"]), msg}
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s=%v", k, rec[k]))
		}
		out = append(out, rig.redact(strings.Join(parts, " ")))
	}
	return out
}

// wantFailureLogged fails the test unless the service logged an ERROR since
// the last call: a failure nobody can fix is told to the operator.
func (rig *faultRig) wantFailureLogged(t *testing.T, call string) {
	t.Helper()
	for _, line := range rig.takeLogs(t) {
		if strings.HasPrefix(line, "ERROR ") {
			return
		}
	}
	t.Errorf("%s: the failure was not logged at ERROR", call)
}

// refusal is a refusal as the app reads it: the code, the message and the
// XPBlocked detail, when there is one.
func refusal(err error) string {
	if err == nil {
		return "ok"
	}
	ce, ok := errors.AsType[*connect.Error](err)
	if !ok {
		return "not a connect error: " + err.Error()
	}
	out := ce.Code().String() + ": " + ce.Message()
	for _, d := range ce.Details() {
		v, derr := d.Value()
		if derr != nil {
			continue
		}
		if b, ok := v.(*progressionv1.XPBlocked); ok {
			out += fmt.Sprintf(" [reason=%s character=%s xp_mode=%s treasure=%s]", b.GetReason(), b.GetCharacterId(), b.GetXpMode(), b.GetTreasurePointId())
		}
	}
	return out
}

// wantAwardRefusal fails the test unless err is the refusal, its IDs replaced by labels.
func (rig *faultRig) wantAwardRefusal(t *testing.T, call string, err error, want string) {
	t.Helper()
	if got := rig.redact(refusal(err)); got != want {
		t.Errorf("%s:\n got  %s\n want %s", call, got, want)
	}
}

// wantLines fails the test unless got is want, line for line, and prints got
// as Go so a reader sees the whole difference.
func wantLines(t *testing.T, what string, got, want []string) {
	t.Helper()
	if slices.Equal(got, want) {
		return
	}
	var b strings.Builder
	for _, g := range got {
		fmt.Fprintf(&b, "\t%q,\n", g)
	}
	t.Errorf("%s:\n got:\n%s want:\n%s", what, b.String(), strings.Join(want, "\n"))
}

// wantNoPendingHistoryFault fails the test if an armed fault never fired.
func (rig *faultRig) wantNoPendingHistoryFault(t *testing.T, call string) {
	t.Helper()
	if p := rig.faults.pending(); len(p) > 0 {
		t.Errorf("%s: armed faults that never fired: %v", call, p)
		rig.faults.reset()
	}
}

// takeEvents returns the award events the session got since the last call:
// the kind, the actor and the payload with its keys sorted.
func (rig *faultRig) takeEvents(t *testing.T) []string {
	t.Helper()
	rows, err := rig.h.pool.Query(t.Context(), `
		SELECT e.kind, coalesce(e.actor_user_id::TEXT, ''), e.payload::TEXT
		FROM session_events e JOIN game_sessions s ON s.id = e.game_session_id
		WHERE s.campaign_id = $1 AND e.kind IN ('xp_awarded', 'xp_award_undone', 'milestone_marked')
		ORDER BY e.created_at, e.seq`, rig.campaign)
	if err != nil {
		t.Fatalf("read the events: %v", err)
	}
	defer rows.Close()
	var all []string
	for rows.Next() {
		var kind, actor, payload string
		if err := rows.Scan(&kind, &actor, &payload); err != nil {
			t.Fatalf("scan an event: %v", err)
		}
		var doc any
		if err := json.Unmarshal([]byte(payload), &doc); err != nil {
			t.Fatalf("payload %s: %v", payload, err)
		}
		// An undo lists the treasures by point ID, a random order here: by
		// label instead.
		if m, ok := doc.(map[string]any); ok && kind == eventXPAwardUndone {
			if ts, ok := m["treasures"].([]any); ok {
				sort.Slice(ts, func(i, j int) bool { return rig.redact(fmt.Sprint(ts[i])) < rig.redact(fmt.Sprint(ts[j])) })
			}
		}
		sorted, _ := json.Marshal(doc)
		all = append(all, rig.redact(kind+" by "+actor+" "+string(sorted)))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read the events: %v", err)
	}
	out := all[min(rig.events, len(all)):]
	rig.events = len(all)
	return out
}

// awardRow is what the award's rows hold: the award, its shares and its
// treasures, the shares and treasures sorted by label.
func (rig *faultRig) awardRow(t *testing.T, id string) string {
	t.Helper()
	ctx := t.Context()
	var mode, reason, givenBy, encounter, milestone, undoneBy string
	var gold *int32
	var total int32
	var again, hashed, undone, undoKey bool
	if err := rig.h.pool.QueryRow(ctx, `
		SELECT mode, reason, coalesce(given_by::TEXT, ''), coalesce(encounter_id::TEXT, ''), gold, total_xp,
		       coalesce(milestone_id::TEXT, ''), milestone_again, idempotency_hash IS NOT NULL,
		       undone_at IS NOT NULL, coalesce(undone_by::TEXT, ''), undo_key IS NOT NULL
		FROM xp_awards WHERE id = $1`, id).Scan(&mode, &reason, &givenBy, &encounter, &gold, &total, &milestone, &again, &hashed, &undone, &undoneBy, &undoKey); err != nil {
		t.Fatalf("read the award %s: %v", id, err)
	}
	goldText := "null"
	if gold != nil {
		goldText = fmt.Sprint(*gold)
	}
	head := fmt.Sprintf("award %s mode=%s reason=%q given_by=%s encounter=%s gold=%s total=%d milestone=%s again=%v hashed=%v undone=%v undone_by=%s undo_key=%v",
		id, mode, reason, givenBy, encounter, goldText, total, milestone, again, hashed, undone, undoneBy, undoKey)
	var parts []string
	shares, err := rig.h.pool.Query(ctx, `SELECT character_id::TEXT, xp, level_at_mark FROM xp_award_shares WHERE award_id = $1`, id)
	if err != nil {
		t.Fatalf("read the shares: %v", err)
	}
	for shares.Next() {
		var who string
		var xp int32
		var level *int32
		if err := shares.Scan(&who, &xp, &level); err != nil {
			t.Fatalf("scan a share: %v", err)
		}
		levelText := "null"
		if level != nil {
			levelText = fmt.Sprint(*level)
		}
		parts = append(parts, rig.redact(fmt.Sprintf("share %s xp=%d level_at_mark=%s", who, xp, levelText)))
	}
	shares.Close()
	treasures, err := rig.h.pool.Query(ctx, `SELECT point_id::TEXT, value_po FROM xp_award_treasures WHERE award_id = $1`, id)
	if err != nil {
		t.Fatalf("read the treasures: %v", err)
	}
	for treasures.Next() {
		var point string
		var po int32
		if err := treasures.Scan(&point, &po); err != nil {
			t.Fatalf("scan a treasure: %v", err)
		}
		parts = append(parts, rig.redact(fmt.Sprintf("treasure %s po=%d", point, po)))
	}
	treasures.Close()
	sort.Strings(parts)
	return rig.redact(strings.Join(append([]string{head}, parts...), "; "))
}

// awardWith is the master's AwardXP through the rig with a fixed key.
func (rig *faultRig) awardWith(key string, edit func(*progressionv1.AwardXPRequest)) (*progressionv1.AwardXPResponse, error) {
	return rig.master.award(rig.campaign, func(r *progressionv1.AwardXPRequest) {
		edit(r)
		r.IdempotencyKey = key
	})
}

// undoWith is the master's UndoLastXPAward with an expected award.
func (rig *faultRig) undoWith(key, expected string) (*progressionv1.XPAward, error) {
	res, err := rig.master.xp.UndoLastXPAward(context.Background(), connect.NewRequest(&progressionv1.UndoLastXPAwardRequest{
		CampaignId: rig.campaign, IdempotencyKey: key, ExpectedAwardId: expected,
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetAward(), nil
}

// plannedRows is the campaign's planned_milestones as they are stored, by
// position: the position, the text, and whether a key and a hash were kept.
func (rig *faultRig) plannedRows(t *testing.T) []string {
	t.Helper()
	rows, err := rig.h.pool.Query(t.Context(), `
		SELECT position, text, create_key IS NOT NULL, create_hash IS NOT NULL
		FROM planned_milestones WHERE campaign_id = $1 ORDER BY position, id`, rig.campaign)
	if err != nil {
		t.Fatalf("read the planned milestones: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var position int32
		var text string
		var key, hash bool
		if err := rows.Scan(&position, &text, &key, &hash); err != nil {
			t.Fatalf("scan a planned milestone: %v", err)
		}
		out = append(out, fmt.Sprintf("%d %s key=%v hash=%v", position, text, key, hash))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read the planned milestones: %v", err)
	}
	return out
}

// campaignEvents is the number of session events of the campaign.
func (rig *faultRig) campaignEvents(t *testing.T) int {
	t.Helper()
	var n int
	if err := rig.h.pool.QueryRow(t.Context(), `
		SELECT count(*) FROM session_events e JOIN game_sessions s ON s.id = e.game_session_id WHERE s.campaign_id = $1`, rig.campaign).Scan(&n); err != nil {
		t.Fatalf("count the session events: %v", err)
	}
	return n
}
