package characters

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// ReviveCharacter, phase by phase: the order of its refusals (role, ID, key, the
// character, its kind, its state), the log line and the stream after a revival only, and
// where the line the table reads goes (the combat's log, the session's, or nowhere).

// reviveHostSpy is a ReviewHost whose ReviveInCombat answers what the test sets: a
// combat's ID, or an error.
type reviveHostSpy struct {
	*hostSpy
	mu         sync.Mutex
	encounter  string
	fail       error
	revived    []string // the characters ReviveInCombat was asked about
	encounters []string // the combats PublishEncounterChanged was told about
}

func (h *reviveHostSpy) ReviveInCombat(_ context.Context, _ pgx.Tx, _, characterID, _ string, _ time.Time) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.revived = append(h.revived, characterID)
	return h.encounter, h.fail
}

func (h *reviveHostSpy) PublishEncounterChanged(_ context.Context, _, encounterID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.encounters = append(h.encounters, encounterID)
}

func (h *reviveHostSpy) reviveSpyCalls() (revived, encounters []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.revived...), append([]string(nil), h.encounters...)
}

// reviveEventSpy is the session's history: it records the events, or fails.
type reviveEventSpy struct {
	CreatureHost
	mu     sync.Mutex
	fail   error
	events []string
}

func (e *reviveEventSpy) AppendEvent(_ context.Context, _ pgx.Tx, _, kind, actor string, payload []byte, _ time.Time) (bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.fail != nil {
		return false, e.fail
	}
	e.events = append(e.events, kind+" by "+actor+" "+string(payload))
	return true, nil
}

func (e *reviveEventSpy) reviveEvents() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.events...)
}

// revivePhasesTable is newReviveTable with the spies and the logs.
type revivePhasesTable struct {
	*reviveTable
	logs   *logBuffer
	spy    *reviveHostSpy
	events *reviveEventSpy
}

func newRevivePhasesTable(t *testing.T) *revivePhasesTable {
	t.Helper()
	tb, logs := newReviewTable(t)
	tb.master.approve(t, tb.pc)
	tb.h.lockSheets(tb.campaign)
	hp := int32(0)
	tb.h.adjust(tb.campaign, tb.pc.GetId(), &playv1.AdjustCharacterVitalsRequest{HitPointsCurrent: &hp})
	r := &revivePhasesTable{reviveTable: &reviveTable{reviewTable: tb, dead: tb.master.markDead(t, tb.pc)}, logs: logs}
	r.spy = &reviveHostSpy{hostSpy: tb.host}
	r.events = &reviveEventSpy{}
	tb.h.svc.SetReviewHost(r.spy)
	tb.h.svc.SetCreatureHost(r.events)
	return r
}

// reviveLogLines counts the log lines of the revival of a character.
func (r *revivePhasesTable) reviveLogLines(characterID string) int {
	return strings.Count(r.logs.String(), `"event":"character.revived","character_id":"`+characterID+`"`)
}

// TestReviveCharacterRefusalOrder: the role first, then the ID, then the key, then the
// character, its kind and its state, each with its code and message.
func TestReviveCharacterRefusalOrder(t *testing.T) {
	t.Parallel()
	r := newRevivePhasesTable(t)
	call := func(u *user, campaign, character, key string) error {
		_, err := u.api.ReviveCharacter(t.Context(), connect.NewRequest(&charactersv1.ReviveCharacterRequest{
			CampaignId: campaign, CharacterId: character, IdempotencyKey: key,
		}))
		return err
	}
	nobody := "00000000-0000-4000-8000-000000000001"
	npc := r.master.create(t, r.campaign, charactersv1.CharacterKind_CHARACTER_KIND_ENEMY, "Bandido", enemySheet())
	living := r.other.createPensantus(t, r.campaign)
	cases := []struct {
		name               string
		u                  *user
		campaign, id, key  string
		code               connect.Code
		message, fieldName string
	}{
		{"a player, with a bad ID and no key", r.other, r.campaign, "x", "", connect.CodePermissionDenied, "permission_denied: only the campaign's master can do this", ""},
		{"the owner", r.lia, r.campaign, r.pc.GetId(), key(), connect.CodePermissionDenied, "permission_denied: only the campaign's master can do this", ""},
		{"a bad ID before the key", r.master, r.campaign, "x", "", connect.CodeNotFound, "not_found: character not found", ""},
		{"no key before the character", r.master, r.campaign, nobody, "", connect.CodeInvalidArgument, "invalid_argument: idempotency_key is required", ""},
		{"a key need not be a UUID", r.master, r.campaign, nobody, "not-a-uuid", connect.CodeNotFound, "not_found: character not found", ""},
		{"nobody", r.master, r.campaign, nobody, key(), connect.CodeNotFound, "not_found: character not found", ""},
		{"an NPC", r.master, r.campaign, npc.GetId(), key(), connect.CodeInvalidArgument, "invalid_argument: character_id is an NPC: only player characters die", "character_id"},
		{"a living character", r.master, r.campaign, living.GetId(), key(), connect.CodeFailedPrecondition, "failed_precondition", ""},
	}
	for _, c := range cases {
		err := call(c.u, c.campaign, c.id, c.key)
		if connect.CodeOf(err) != c.code {
			t.Errorf("%s: error = %v, want %v", c.name, err, c.code)
			continue
		}
		if err.Error() != c.message {
			// Finding: NOT_DEAD has no message in errBlocked's table, so the refusal of a
			// living character carries an empty one; frozen as it is.
			t.Errorf("%s: error = %q, want %q", c.name, err.Error(), c.message)
		}
		if c.fieldName != "" {
			if got := invalidField(t, c.name, err); got != c.fieldName {
				t.Errorf("%s: InvalidField = %q, want %q", c.name, got, c.fieldName)
			}
		}
	}
	if d := blocked(t, "a living character", call(r.master, r.campaign, living.GetId(), key())); d.GetReason() != charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_NOT_DEAD || d.GetCharacterId() != living.GetId() {
		t.Errorf("CharacterBlocked = %v, want NOT_DEAD naming the living character", d)
	}
	if revived, _ := r.spy.reviveSpyCalls(); len(revived) != 0 || r.reviveLogLines(r.pc.GetId()) != 0 || len(r.events.reviveEvents()) != 0 {
		t.Errorf("after the refusals: ReviveInCombat %v, %d log lines, events %v; want nothing", revived, r.reviveLogLines(r.pc.GetId()), r.events.reviveEvents())
	}
	if got := r.master.get(t, r.campaign, r.pc.GetId()); got.GetState() != charactersv1.CharacterState_CHARACTER_STATE_DEAD {
		t.Errorf("the dead character after the refusals = %v, want dead", got.GetState())
	}
}

// TestReviveCharacterOutsideACombat: no combat holds the character, so the line goes to the
// session's history; the log line and the stream come once, and a retry repeats neither.
func TestReviveCharacterOutsideACombat(t *testing.T) {
	t.Parallel()
	r := newRevivePhasesTable(t)
	k := key()
	revived, err := r.master.revive(r.dead, k)
	if err != nil {
		t.Fatalf("ReviveCharacter() error = %v", err)
	}
	if revived.GetId() != r.pc.GetId() || revived.GetRevivedAt() == nil {
		t.Errorf("ReviveCharacter() = %v, want the revived character", revived)
	}
	want := []string{"character_revived by " + r.master.id + ` {"character_id":"` + r.pc.GetId() + `"}`}
	if got := r.events.reviveEvents(); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("session events = %v, want %v", got, want)
	}
	asked, encounters := r.spy.reviveSpyCalls()
	if strings.Join(asked, ",") != r.pc.GetId() || len(encounters) != 0 {
		t.Errorf("ReviveInCombat asked about %v, PublishEncounterChanged %v; want the character once and no combat", asked, encounters)
	}
	if n := r.reviveLogLines(r.pc.GetId()); n != 1 {
		t.Errorf("log lines of the revival = %d, want 1", n)
	}
	if got := r.host.told(); len(got) == 0 || got[len(got)-1] != "revived:"+r.pc.GetId() {
		t.Errorf("the stream was told %v, want the revival last", got)
	}
	told := len(r.host.told())
	again, err := r.master.revive(r.dead, k)
	if err != nil || again.GetId() != r.pc.GetId() {
		t.Fatalf("retry = %v, %v; want the character", again, err)
	}
	asked, _ = r.spy.reviveSpyCalls()
	if len(r.events.reviveEvents()) != 1 || r.reviveLogLines(r.pc.GetId()) != 1 || len(r.host.told()) != told || len(asked) != 1 {
		t.Errorf("a retry wrote %d events, %d log lines, told the stream %d times, asked the combat %d times; want 1, 1, %d, 1",
			len(r.events.reviveEvents()), r.reviveLogLines(r.pc.GetId()), len(r.host.told()), len(asked), told)
	}
}

// TestReviveCharacterInACombat: the combat that holds the character writes its own line,
// so the session gets none, and the combat is told after the commit.
func TestReviveCharacterInACombat(t *testing.T) {
	t.Parallel()
	r := newRevivePhasesTable(t)
	r.spy.encounter = "encounter-1"
	if _, err := r.master.revive(r.dead, key()); err != nil {
		t.Fatalf("ReviveCharacter() error = %v", err)
	}
	if got := r.events.reviveEvents(); len(got) != 0 {
		t.Errorf("session events = %v, want none: the combat's log has the line", got)
	}
	if _, encounters := r.spy.reviveSpyCalls(); strings.Join(encounters, ",") != "encounter-1" {
		t.Errorf("PublishEncounterChanged = %v, want encounter-1 once", encounters)
	}
}

// TestReviveCharacterFailures: an error from the combat or from the session's history
// undoes the whole revival; nothing is logged or streamed.
func TestReviveCharacterFailures(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		setup func(r *revivePhasesTable)
	}{
		{"the combat fails", func(r *revivePhasesTable) { r.spy.fail = errors.New("combat down") }},
		{"the history fails", func(r *revivePhasesTable) { r.events.fail = errors.New("history down") }},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := newRevivePhasesTable(t)
			c.setup(r)
			told := len(r.host.told())
			_, err := r.master.revive(r.dead, key())
			if connect.CodeOf(err) != connect.CodeInternal {
				t.Fatalf("ReviveCharacter() error = %v, want internal", err)
			}
			if !strings.Contains(r.logs.String(), `"msg":"characters: cannot revive a character"`) {
				t.Errorf("logs = %s, want the failure logged", r.logs.String())
			}
			if got := r.master.get(t, r.campaign, r.pc.GetId()); got.GetState() != charactersv1.CharacterState_CHARACTER_STATE_DEAD {
				t.Errorf("the character after the failure = %v, want still dead", got.GetState())
			}
			v, err := r.h.svc.getVitals(t.Context(), nil, r.campaign, r.pc.GetId(), true)
			if err != nil || v.GetHitPointsCurrent() != 0 {
				t.Errorf("vitals after the failure = %v, %v; want 0 hit points", v, err)
			}
			if r.reviveLogLines(r.pc.GetId()) != 0 || len(r.host.told()) != told {
				t.Errorf("a failed revival logged %d lines and told the stream %v", r.reviveLogLines(r.pc.GetId()), r.host.told())
			}
		})
	}
}

// TestReviveCharacterWithoutPlay: with no play module connected, the revival happens and
// no line is written anywhere, even with a history to write to.
func TestReviveCharacterWithoutPlay(t *testing.T) {
	t.Parallel()
	r := newRevivePhasesTable(t)
	r.h.svc.SetReviewHost(nil)
	if _, err := r.master.revive(r.dead, key()); err != nil {
		t.Fatalf("ReviveCharacter() error = %v", err)
	}
	if got := r.events.reviveEvents(); len(got) != 0 {
		t.Errorf("session events = %v, want none", got)
	}
	if n := r.reviveLogLines(r.pc.GetId()); n != 1 {
		t.Errorf("log lines of the revival = %d, want 1", n)
	}
}
