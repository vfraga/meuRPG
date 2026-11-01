package play

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// The characterization of the core of the effects that last (exhaustion, the rows of the
// effects, their clock, the dice a roll spends, game time, the armor class of Mage Armor and the
// effects of a spell). These tests freeze what the calls do today, scenario by scenario, before
// the big functions are split: what the call answers, what the combatants and the effect rows
// hold afterwards and the exact payload of every event written. A scenario's state is a text
// file in testdata/lasting_core (ids replaced by labels); a change here is a change of
// behavior, not of structure. Run with -update-lasting to write the files again after a
// change of behavior that was meant, and read the diff.

// coreNames replaces the ids of the combatants and the users in a text by their labels, and any
// other id by <uuid>, so that the text is the same in every run.
func (a *armed) coreNames(t *testing.T, text string) string {
	t.Helper()
	rows, err := a.h.pool.Query(t.Context(), `SELECT id::TEXT, label FROM combatants`)
	if err != nil {
		t.Fatalf("read the labels: %v", err)
	}
	labels := map[string]string{}
	for rows.Next() {
		var id, label string
		if err := rows.Scan(&id, &label); err != nil {
			t.Fatalf("scan a label: %v", err)
		}
		labels[id] = label
	}
	rows.Close() // the pool has one connection
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for id, label := range labels {
		text = strings.ReplaceAll(text, id, "<"+label+">")
	}
	for name, u := range map[string]*user{"master": a.master, "caio": a.caio, "ana": a.ana, "bia": a.bia} {
		text = strings.ReplaceAll(text, u.id, "<user:"+name+">")
	}
	for name, c := range map[string]string{"toren": a.toren.GetId(), "pens": a.pens.GetId(), "bri": a.bri.GetId(), "goblin": a.goblin.GetId(), "capitao": a.capitao.GetId()} {
		if c != "" { // a fixture without that hero
			text = strings.ReplaceAll(text, c, "<char:"+name+">")
		}
	}
	return uuidText.ReplaceAllString(text, "<uuid>")
}

// coreCombatants prints what the database keeps of each combatant of the combat that the
// effects and the exhaustion touch (only the labels named, when there are some).
func (a *armed) coreCombatants(t *testing.T, encounterID string, only ...string) string {
	t.Helper()
	rows, err := a.h.pool.Query(t.Context(), `SELECT label, COALESCE(hp_current::TEXT, '-'), COALESCE(hp_max::TEXT, '-'), COALESCE(hp_max_base::TEXT, '-'),
			COALESCE(hp_temp::TEXT, '-'), defeated, exhaustion_level, death_failures, array_to_string(conditions, ','), array_to_string(effect_conditions, ','),
			effect_ac_bonus, effect_speed_pct, effect_speed_add_ft, effect_no_action, effect_no_reaction, effect_no_move, COALESCE(mage_armor_ac::TEXT, '-'),
			COALESCE(concentration_spell, '-'), turn_state
		FROM combatants WHERE encounter_id = $1 ORDER BY label`, encounterID)
	if err != nil {
		t.Fatalf("read the combatants: %v", err)
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var label, hp, hpMax, hpBase, temp, conds, derived, mage, conc, turn string
		var defeated, noAction, noReaction, noMove bool
		var exh, fails, ac, pct, add int32
		if err := rows.Scan(&label, &hp, &hpMax, &hpBase, &temp, &defeated, &exh, &fails, &conds, &derived, &ac, &pct, &add, &noAction, &noReaction, &noMove, &mage, &conc, &turn); err != nil {
			t.Fatalf("scan a combatant: %v", err)
		}
		if len(only) > 0 && !slices.Contains(only, label) {
			continue
		}
		fmt.Fprintf(&b, "%s hp %s/%s base %s temp %s defeated=%v exh=%d fails=%d conds=[%s] derived=[%s] ac%+d speed %d%% %+dft noAction=%v noReaction=%v noMove=%v mage=%s conc=%s turn=%s\n",
			label, hp, hpMax, hpBase, temp, defeated, exh, fails, conds, derived, ac, pct, add, noAction, noReaction, noMove, mage, conc, turn)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// coreEffects prints the effect rows of a combat.
func (a *armed) coreEffects(t *testing.T, encounterID string) string {
	t.Helper()
	rows, err := a.h.pool.Query(t.Context(), `SELECT c.label, s.source_key, COALESCE(s.source_kind, '-'), s.concentration, COALESCE(s.duration_kind, '-'),
			COALESCE(s.ends_round::TEXT, '-'), COALESCE(e.label, '-'), COALESCE(s.ends_phase, '-'), s.spent_at IS NOT NULL, s.triggers_fired,
			COALESCE(s.follows_key, '-'), COALESCE(src.label, '-'), s.player_visible, s.audience, COALESCE(s.player_label, '-'), COALESCE(s.save_dc::TEXT, '-'),
			array_to_string(s.condition_keys, ','), s.modifiers::TEXT, s.started_round
		FROM combatant_states s JOIN combatants c ON c.id = s.combatant_id
		LEFT JOIN combatants e ON e.id = s.ends_combatant_id LEFT JOIN combatants src ON src.id = s.source_id
		WHERE s.encounter_id = $1 AND s.kind = 'effect' ORDER BY c.label, s.source_key, s.created_at, s.id`, encounterID)
	if err != nil {
		t.Fatalf("read the effects: %v", err)
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var who, key, kind, dur, endsRound, endsWho, phase, follows, caster, audience, label, dc, conds, mods string
		var conc, spent, visible bool
		var fired, started int32
		if err := rows.Scan(&who, &key, &kind, &conc, &dur, &endsRound, &endsWho, &phase, &spent, &fired, &follows, &caster, &visible, &audience, &label, &dc, &conds, &mods, &started); err != nil {
			t.Fatalf("scan an effect: %v", err)
		}
		fmt.Fprintf(&b, "%s %s kind=%s conc=%v dur=%s ends=r%s/%s/%s spent=%v fired=%d follows=%s caster=%s visible=%v audience=%s label=%s dc=%s conds=[%s] mods=%s started=r%d\n",
			who, key, kind, conc, dur, endsRound, endsWho, phase, spent, fired, follows, caster, visible, audience, label, dc, conds, dbtest.SortJSONKeys(mods), started)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// coreEvents prints the events of the open session of the kinds, oldest first.
func (a *armed) coreEvents(t *testing.T, kinds ...string) string {
	t.Helper()
	rows, err := a.h.pool.Query(t.Context(), `SELECT kind, payload::TEXT FROM session_events WHERE kind = ANY($1) ORDER BY seq`, kinds)
	if err != nil {
		t.Fatalf("read the events: %v", err)
	}
	var lines [][2]string
	for rows.Next() {
		var kind, payload string
		if err := rows.Scan(&kind, &payload); err != nil {
			t.Fatalf("scan an event: %v", err)
		}
		lines = append(lines, [2]string{kind, payload})
	}
	rows.Close() // the pool has one connection: free it before the names are read
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, l := range lines {
		fmt.Fprintf(&b, "%s %s\n", l[0], a.coreCanonical(t, a.coreNames(t, l[1])))
	}
	return b.String()
}

// coreCanonical prints a payload with the lists of ids in order: the rows of one casting are made
// at the same instant, so the order of its targets is the order of random ids.
func (a *armed) coreCanonical(t *testing.T, payload string) string {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(payload), &v); err != nil {
		t.Fatalf("read a payload: %v", err)
	}
	// The target a line names is the first row of the casting, in a random order for the rows made
	// at one instant: with several targets it reads as one of them.
	if m, ok := v.(map[string]any); ok {
		if l, ok := m["lasting"].(map[string]any); ok {
			if ts, ok := l["targets"].([]any); ok && len(ts) > 1 && slices.Contains(ts, m["target_id"]) {
				m["target_id"] = "<one of the targets>"
			}
			// The line of the change itself (the master ended an effect) names a row of the casting.
			if _, listed := l["targets"]; !listed && l["change"] == "ended" && m["target_id"] != nil {
				m["target_id"] = "<a row of the casting>"
				delete(m, "secret") // read from that row too
			}
		}
	}
	var sorted func(x any) any
	sorted = func(x any) any {
		switch x := x.(type) {
		case map[string]any:
			for k, e := range x {
				x[k] = sorted(e)
			}
		case []any:
			for i, e := range x {
				x[i] = sorted(e)
			}
			if strs := make([]string, 0, len(x)); len(x) > 1 {
				for _, e := range x {
					if str, ok := e.(string); ok {
						strs = append(strs, str)
					}
				}
				if len(strs) == len(x) {
					slices.Sort(strs)
					for i, str := range strs {
						x[i] = str
					}
				}
			}
		}
		return x
	}
	var out strings.Builder
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(sorted(v)); err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(out.String())
}

// coreRefusal prints the whole of a refusal: the code and the message.
func coreRefusal(err error) string {
	if err == nil {
		return "no error"
	}
	var ce *connect.Error
	if !errors.As(err, &ce) {
		return fmt.Sprintf("not a connect error: %v", err)
	}
	return fmt.Sprintf("%s: %s", ce.Code(), ce.Message())
}

// coreLog collects the lines of a scenario: what each step answered and, at the end, the state.
type coreLog struct {
	b strings.Builder
}

func (l *coreLog) say(format string, args ...any) {
	fmt.Fprintf(&l.b, format+"\n", args...)
}

func (l *coreLog) String() string { return l.b.String() }

// coreVitals prints the vitals the panels read of a character.
func (a *armed) coreVitals(t *testing.T, label string, c *charactersv1.Character) string {
	t.Helper()
	v := a.vitals(t, c)
	return fmt.Sprintf("%s vitals hp %d/%d temp %d exh %d acBase %d", label, v.GetHitPointsCurrent(), v.GetHitPointsMax(), v.GetHitPointsTemporary(), v.GetExhaustionLevel(), v.GetArmorClassBase())
}

// coreInTx runs fn in a transaction the way a change of the master's does: in the open session,
// with the combat loaded when encounterID is given. The pool has one connection: fn reads and
// writes through c alone.
func (a *armed) coreInTx(t *testing.T, encounterID string, fn func(ctx context.Context, c *combatTx) error) {
	t.Helper()
	err := db.InTx(t.Context(), a.h.pool, func(tx pgx.Tx) error {
		ctx := t.Context()
		q := a.h.svc.queries.WithTx(tx)
		session, err := q.GetOpenGameSessionForUpdate(ctx, a.campaignID)
		if err != nil {
			return err
		}
		var enc playdb.Encounter
		if encounterID != "" {
			if enc, err = q.GetEncounterByID(ctx, encounterID); err != nil {
				return err
			}
		}
		c, err := a.h.svc.openTx(ctx, combatTx{tx: tx, q: q, session: session, enc: enc, now: a.h.svc.now(), svc: a.h.svc, master: true, actorUserID: a.master.id})
		if err != nil {
			return err
		}
		return fn(ctx, c)
	})
	if err != nil {
		t.Fatalf("the transaction failed: %v", err)
	}
}

// coreCharacterEffects prints the rows of character_effects of a character, the effects out of a combat.
func (a *armed) coreCharacterEffects(t *testing.T, characterID string) string {
	t.Helper()
	rows, err := a.h.pool.Query(t.Context(), `SELECT source_key, source_kind, duration_kind, COALESCE(seconds_left::TEXT, '-'), modifiers::TEXT, array_to_string(condition_keys, ','), player_visible
		FROM character_effects WHERE character_id = $1 ORDER BY source_key, modifiers::TEXT, id`, characterID)
	if err != nil {
		t.Fatalf("read the character's effects: %v", err)
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var key, kind, dur, left, mods, conds string
		var visible bool
		if err := rows.Scan(&key, &kind, &dur, &left, &mods, &conds, &visible); err != nil {
			t.Fatalf("scan a character's effect: %v", err)
		}
		fmt.Fprintf(&b, "%s kind=%s dur=%s left=%s mods=%s conds=[%s] visible=%v\n", key, kind, dur, left, dbtest.SortJSONKeys(mods), conds, visible)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return b.String()
}
