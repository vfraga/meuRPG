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
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	progressionv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/progression/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/progression/v1/progressionv1connect"
	"github.com/PuraFome/meuRPG/backend/internal/platform/httpserver"
	"github.com/PuraFome/meuRPG/backend/internal/progression/link"
)

// The rig of the award characterization tests: a table whose progression
// service logs into a buffer, counts what it tells the streams, and can fail
// any call it makes to another module once, so each phase of an award and of
// its undo (the checks, the writes, the event, the log line, the publishes)
// is frozen as it is.

// errAwardInjected is the failure an armed fault returns.
var errAwardInjected = errors.New("injected failure")

// awardFaults arms one-shot failures by name, and holds a hook that runs the
// first time the service reads the clock: inside an award's transaction, after
// every read and right before the award is written.
type awardFaults struct {
	mu          sync.Mutex
	armed       map[string]error
	beforeWrite func()
	published   map[string]int // PublishXPChanged calls by campaign
}

// now runs the hook, once, and reads the clock.
func (f *awardFaults) now(clock func() time.Time) func() time.Time {
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

// arm makes the next call named name fail with err.
func (f *awardFaults) arm(name string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.armed == nil {
		f.armed = map[string]error{}
	}
	f.armed[name] = err
}

// take returns the armed failure of name, once.
func (f *awardFaults) take(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	err := f.armed[name]
	delete(f.armed, name)
	return err
}

func (f *awardFaults) xpChanged(campaignID string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.published[campaignID]
}

type awardFaultParty struct {
	inner Party
	f     *awardFaults
}

func (p awardFaultParty) Names(ctx context.Context, campaignID string, ids []string) (map[string]string, error) {
	return p.inner.Names(ctx, campaignID, ids)
}

func (p awardFaultParty) Party(ctx context.Context, tx pgx.Tx, campaignID string) ([]link.Member, error) {
	if err := p.f.take("party"); err != nil {
		return nil, err
	}
	return p.inner.Party(ctx, tx, campaignID)
}

func (p awardFaultParty) AddExperience(ctx context.Context, tx pgx.Tx, campaignID, characterID string, delta int32, at time.Time) (int32, int32, error) {
	if err := p.f.take("add_experience"); err != nil {
		return 0, 0, err
	}
	return p.inner.AddExperience(ctx, tx, campaignID, characterID, delta, at)
}

type awardFaultCombats struct {
	Combats
	f *awardFaults
}

func (c awardFaultCombats) CampaignEncounter(ctx context.Context, tx pgx.Tx, campaignID, encounterID string) (link.Encounter, error) {
	if err := c.f.take("encounter"); err != nil {
		return link.Encounter{}, err
	}
	return c.Combats.CampaignEncounter(ctx, tx, campaignID, encounterID)
}

type awardFaultLog struct {
	SessionLog
	f *awardFaults
}

func (l awardFaultLog) AppendEvent(ctx context.Context, tx pgx.Tx, campaignID, kind, actorUserID string, payload []byte, at time.Time) (bool, error) {
	if err := l.f.take("append_event"); err != nil {
		return false, err
	}
	return l.SessionLog.AppendEvent(ctx, tx, campaignID, kind, actorUserID, payload, at)
}

func (l awardFaultLog) PublishXPChanged(campaignID string) {
	l.f.mu.Lock()
	if l.f.published == nil {
		l.f.published = map[string]int{}
	}
	l.f.published[campaignID]++
	l.f.mu.Unlock()
	l.SessionLog.PublishXPChanged(campaignID)
}

type awardFaultTreasures struct {
	Treasures
	f *awardFaults
}

func (s awardFaultTreasures) LockForConversion(ctx context.Context, tx pgx.Tx, campaignID string, pointIDs []string) ([]link.Treasure, error) {
	if err := s.f.take("lock_treasures"); err != nil {
		return nil, err
	}
	return s.Treasures.LockForConversion(ctx, tx, campaignID, pointIDs)
}

func (s awardFaultTreasures) MarkConverted(ctx context.Context, tx pgx.Tx, awardID string, pointIDs []string) error {
	if err := s.f.take("mark_converted"); err != nil {
		return err
	}
	return s.Treasures.MarkConverted(ctx, tx, awardID, pointIDs)
}

func (s awardFaultTreasures) Release(ctx context.Context, tx pgx.Tx, awardID string) ([]string, error) {
	if err := s.f.take("release"); err != nil {
		return nil, err
	}
	return s.Treasures.Release(ctx, tx, awardID)
}

type awardFaultCampaigns struct {
	Campaigns
	f *awardFaults
}

func (c awardFaultCampaigns) CampaignXPMode(ctx context.Context, tx pgx.Tx, campaignID string) (campaignsv1.XpMode, error) {
	name := "xp_mode"
	if tx != nil {
		name = "xp_mode_in_tx"
	}
	if err := c.f.take(name); err != nil {
		return 0, err
	}
	return c.Campaigns.CampaignXPMode(ctx, tx, campaignID)
}

// awardLogBuffer is the JSON log of the rig's service.
type awardLogBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *awardLogBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

// awardRig is a table served by a progression service of its own.
type awardRig struct {
	*table
	faults *awardFaults
	logs   *awardLogBuffer
	// labels names the IDs in the goldens; unnamed UUIDs are numbered.
	labels  map[string]string
	unnamed map[string]string
	events  int // the session events already taken
}

// newAwardRig is newTable whose members call a progression service that logs
// at debug level into the rig and goes through the rig's faults.
func newAwardRig(t *testing.T, mode campaignsv1.XpMode, n int) *awardRig {
	t.Helper()
	tb := newTable(t, mode, n)
	rig := &awardRig{table: tb, faults: &awardFaults{}, logs: &awardLogBuffer{}, labels: map[string]string{}, unnamed: map[string]string{}}
	cfg := tb.h.cfg
	cfg.Logger = slog.New(slog.NewJSONHandler(rig.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cfg.Party = awardFaultParty{inner: cfg.Party, f: rig.faults}
	cfg.Combats = awardFaultCombats{Combats: cfg.Combats, f: rig.faults}
	cfg.Log = awardFaultLog{SessionLog: cfg.Log, f: rig.faults}
	cfg.Treasures = awardFaultTreasures{Treasures: cfg.Treasures, f: rig.faults}
	cfg.Campaigns = awardFaultCampaigns{Campaigns: cfg.Campaigns, f: rig.faults}
	cfg.Now = rig.faults.now(cfg.Now)
	svc, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	srv := httpserver.New(httpserver.Config{Logger: slog.New(slog.DiscardHandler)})
	var sessions Sessions = fakeSessions{}
	svc.Mount(srv.Handle, sessions, tb.h.camps, connect.WithRequireConnectProtocolHeader())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	for _, u := range append([]*user{tb.master}, tb.players...) {
		u.xp = progressionv1connect.NewProgressionServiceClient(&http.Client{Transport: userTransport{userID: u.id, next: ts.Client().Transport}}, ts.URL)
	}
	rig.label(tb.campaign, "campaign")
	rig.label(tb.master.id, "master")
	for i, pc := range tb.pcs {
		rig.label(pc.GetId(), strings.ToLower(pc.GetName()))
		rig.label(tb.players[i].id, "player"+fmt.Sprint(i))
	}
	return rig
}

// label names an ID in the goldens.
func (rig *awardRig) label(id, name string) { rig.labels[id] = "<" + name + ">" }

var awardUUID = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// redact puts the labels in place of the IDs, and numbers the others in the
// order the rig first meets them.
func (rig *awardRig) redact(s string) string {
	return awardUUID.ReplaceAllStringFunc(s, func(id string) string {
		if l, ok := rig.labels[id]; ok {
			return l
		}
		if l, ok := rig.unnamed[id]; ok {
			return l
		}
		l := fmt.Sprintf("<uuid%d>", len(rig.unnamed)+1)
		rig.unnamed[id] = l
		return l
	})
}

// takeLogs returns the log lines written since the last call, but the
// authorization's: the level, the message and the attributes sorted, without
// the time.
func (rig *awardRig) takeLogs(t *testing.T) []string {
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
		if msg != "event" && !strings.HasPrefix(msg, "progression:") {
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

// takeEvents returns the award events the session got since the last call:
// the kind, the actor and the payload with its keys sorted.
func (rig *awardRig) takeEvents(t *testing.T) []string {
	t.Helper()
	rows, err := rig.h.pool.Query(t.Context(), `
		SELECT e.kind, coalesce(e.actor_user_id::STRING, ''), e.payload::STRING
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
func (rig *awardRig) awardRow(t *testing.T, id string) string {
	t.Helper()
	ctx := t.Context()
	var mode, reason, givenBy, encounter, milestone, undoneBy string
	var gold *int32
	var total int32
	var again, hashed, undone, undoKey bool
	if err := rig.h.pool.QueryRow(ctx, `
		SELECT mode, reason, coalesce(given_by::STRING, ''), coalesce(encounter_id::STRING, ''), gold, total_xp,
		       coalesce(milestone_id::STRING, ''), milestone_again, idempotency_hash IS NOT NULL,
		       undone_at IS NOT NULL, coalesce(undone_by::STRING, ''), undo_key IS NOT NULL
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
	shares, err := rig.h.pool.Query(ctx, `SELECT character_id::STRING, xp, level_at_mark FROM xp_award_shares WHERE award_id = $1`, id)
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
	treasures, err := rig.h.pool.Query(ctx, `SELECT point_id::STRING, value_po FROM xp_award_treasures WHERE award_id = $1`, id)
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

// redactPublished is what the maps were told so far, a line per call with
// its maps sorted by label.
func (rig *awardRig) redactPublished() []string {
	var out []string
	for _, p := range rig.h.treasures.published() {
		maps := make([]string, 0, len(p.mapIDs))
		for _, id := range p.mapIDs {
			maps = append(maps, rig.redact(id))
		}
		sort.Strings(maps)
		out = append(out, rig.redact(p.campaignID)+" "+strings.Join(maps, ","))
	}
	return out
}

// awardRefusal is a refusal as the app reads it: the code, the message and
// the XPBlocked detail, when there is one.
func awardRefusal(err error) string {
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
func (rig *awardRig) wantAwardRefusal(t *testing.T, call string, err error, want string) {
	t.Helper()
	if got := rig.redact(awardRefusal(err)); got != want {
		t.Errorf("%s:\n got  %s\n want %s", call, got, want)
	}
}

// wantAwardLines fails the test unless got is want, line for line, and prints
// got as Go so a reader sees the whole difference.
func wantAwardLines(t *testing.T, what string, got, want []string) {
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

// awardEach is the master's AwardXP through the rig with a fixed key.
func (rig *awardRig) awardWith(key string, edit func(*progressionv1.AwardXPRequest)) (*progressionv1.AwardXPResponse, error) {
	return rig.master.award(rig.campaign, func(r *progressionv1.AwardXPRequest) {
		edit(r)
		r.IdempotencyKey = key
	})
}

// undoWith is the master's UndoLastXPAward with an expected award.
func (rig *awardRig) undoWith(key, expected string) (*progressionv1.XPAward, error) {
	res, err := rig.master.xp.UndoLastXPAward(context.Background(), connect.NewRequest(&progressionv1.UndoLastXPAwardRequest{
		CampaignId: rig.campaign, IdempotencyKey: key, ExpectedAwardId: expected,
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetAward(), nil
}
