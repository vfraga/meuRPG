package play

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
)

// Helpers of the characterization of AddCharacterEffect and AddLastingEffect: the party, the
// way the rows and events are read back as text, and who hears what.

// newEffectsParty is newCastingParty with Toren in leather armor, so that Mage Armor takes no
// hold of him (SRD 5.1: the target is a willing creature who is not wearing armor).
func newEffectsParty(t *testing.T) *armed {
	t.Helper()
	return newArmedWith(t, func(a *armed) {
		a.toren = a.caio.castingHero(t, a.campaignID, "Toren", classLevel("class:fighter", 5, ""),
			&rulesv1.AbilityScores{Strength: 16, Dexterity: 13, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, nil, nil, nil, leatherArmorKey)
		a.pens = a.ana.castingHero(t, a.campaignID, "Pensantus", classLevel("class:wizard", 5, ""),
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8}, []string{fireBolt},
			[]string{mageArmorKey, holdPerson, webSpell}, []string{mageArmorKey}, "")
		a.bri = a.bia.castingHero(t, a.campaignID, "Brisa", classLevel("class:cleric", 3, "subclass:life"),
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 16, Constitution: 14, Intelligence: 10, Wisdom: 16, Charisma: 8}, []string{sacredFlame}, nil,
			[]string{cureWounds, blessKey}, "")
	})
}

// atText finds a timestamp field; uuidText is shared with the undo snapshot tests.
var atText = regexp.MustCompile(`"at":"[^"]*",\s*`)

// plain writes the text without what changes from one run to the next: the ids the party
// knows are named, any other id is numbered by its first appearance, and the timestamps go.
func plain(text string, names map[string]string) string {
	text = atText.ReplaceAllString(text, "")
	seen := map[string]string{}
	return uuidText.ReplaceAllStringFunc(text, func(id string) string {
		if n, ok := names[id]; ok {
			return n
		}
		if n, ok := seen[id]; ok {
			return n
		}
		n := fmt.Sprintf("<id%d>", len(seen)+1)
		seen[id] = n
		return n
	})
}

// partyNames names the characters, and the combatants of the encounter when given.
func (a *armed) partyNames(t *testing.T, e *playv1.Encounter) map[string]string {
	t.Helper()
	names := map[string]string{
		a.toren.GetId(): "toren", a.pens.GetId(): "pens", a.bri.GetId(): "bri", a.goblin.GetId(): "goblin-char", a.capitao.GetId(): "capitao-char",
	}
	for _, c := range e.GetCombatants() {
		names[c.GetId()] = "@" + c.GetLabel()
	}
	if e != nil {
		names[e.GetId()] = "encounter"
	}
	return names
}

// characterEffectRows are the character_effects rows of the campaign, one text line each, in
// the order of their source key and character.
func (a *armed) characterEffectRows(t *testing.T) []string {
	t.Helper()
	rows, err := a.h.pool.Query(t.Context(), `
		SELECT jsonb_strip_nulls(to_jsonb(x))::TEXT FROM (
			SELECT character_id, source_key, source_kind, condition_keys, duration_kind, seconds_left, player_visible, audience, player_label,
				end_save_ability, start_save_ability, on_fail_effect,
				(SELECT string_agg(m->>'kind' || coalesce(':' || (m->>'value'), '') || coalesce((m->'abilities')::TEXT, ''), ',') FROM jsonb_array_elements(modifiers) m) AS mods
			FROM character_effects WHERE campaign_id = $1 ORDER BY source_key, character_id
		) x`, a.campaignID)
	if err != nil {
		t.Fatalf("read the character effects: %v", err)
	}
	defer rows.Close()
	names := a.partyNames(t, nil)
	var out []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, plain(dbtest.SortJSONKeys(line), names))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read the character effects: %v", err)
	}
	slices.Sort(out) // the characters' ids are random: the order is by the text
	return out
}

// combatEffectRows are the effects of the encounter, one text line each, with the labels of
// the combatants they name.
func (a *armed) combatEffectRows(t *testing.T, e *playv1.Encounter) []string {
	t.Helper()
	rows, err := a.h.pool.Query(t.Context(), `
		SELECT jsonb_strip_nulls(to_jsonb(x))::TEXT FROM (
			SELECT c.label AS target, s.source_key, s.source_kind, s.condition_keys, s.duration_kind, s.started_round, s.ends_round,
				(SELECT label FROM combatants WHERE id = s.ends_combatant_id) AS ends_on, s.ends_phase,
				(SELECT label FROM combatants WHERE id = s.source_id) AS caster, s.concentration, s.player_visible, s.audience, s.player_label,
				s.save_dc, s.end_save_ability, s.start_save_ability, s.on_fail_effect, s.follows_key,
				(SELECT string_agg(m->>'kind' || coalesce(':' || (m->>'value'), '') || coalesce((m->'abilities')::TEXT, ''), ',') FROM jsonb_array_elements(s.modifiers) m) AS mods
			FROM combatant_states s JOIN combatants c ON c.id = s.combatant_id
			WHERE s.encounter_id = $1 AND s.kind = 'effect'
		) x ORDER BY x.target, x.source_key`, e.GetId())
	if err != nil {
		t.Fatalf("read the combat effects: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, dbtest.SortJSONKeys(line))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read the combat effects: %v", err)
	}
	return out
}

// concentrations lists who concentrates on what in the encounter.
func (a *armed) concentrations(t *testing.T, e *playv1.Encounter) string {
	t.Helper()
	rows, err := a.h.pool.Query(t.Context(), `SELECT label || '=' || concentration_spell FROM combatants WHERE encounter_id = $1 AND concentration_spell IS NOT NULL ORDER BY label`, e.GetId())
	if err != nil {
		t.Fatalf("read the concentrations: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, s)
	}
	return strings.Join(out, ",")
}

// effectEvents are the payloads of the lasting_effect_added events of the open session, with
// the idempotency key (named by the caller's keys) and the actor's role.
func (a *armed) effectEvents(t *testing.T, e *playv1.Encounter) []string {
	t.Helper()
	rows, err := a.h.pool.Query(t.Context(), `
		SELECT payload::TEXT || ' key=' || (idempotency_key IS NOT NULL)::TEXT || ' hash=' || (idempotency_hash IS NOT NULL)::TEXT
			|| ' enc=' || (encounter_id IS NOT NULL)::TEXT || ' char=' || (character_id IS NOT NULL)::TEXT || ' actor=' || (actor_user_id IS NOT NULL)::TEXT
		FROM session_events WHERE kind = 'lasting_effect_added' AND game_session_id IN (SELECT id FROM game_sessions WHERE campaign_id = $1) ORDER BY seq`, a.campaignID)
	if err != nil {
		t.Fatalf("read the events: %v", err)
	}
	defer rows.Close()
	names := a.partyNames(t, e)
	var out []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, plain(dbtest.SortJSONKeys(line), names))
	}
	return out
}

// clearEffects removes every effect of the campaign and of the encounter by hand, so that the
// next case of a table starts from the same party.
func (a *armed) clearEffects(t *testing.T, e *playv1.Encounter) {
	t.Helper()
	a.execSQL(t, `DELETE FROM character_effects WHERE campaign_id = $1`, a.campaignID)
	if e != nil {
		a.execSQL(t, `DELETE FROM combatant_states WHERE encounter_id = $1 AND kind = 'effect'`, e.GetId())
		a.execSQL(t, `UPDATE combatants SET concentration_spell = NULL WHERE encounter_id = $1`, e.GetId())
	}
}

// seenBy counts, as "master/owner/other", how many of the character's effects each reads
// outside a combat: the master, the character's own player and another player (RN-10).
func (a *armed) seenBy(t *testing.T, owner, other *user, characterID string) string {
	t.Helper()
	count := func(u *user) int {
		res, err := u.lasting.ListCharacterEffects(t.Context(), connect.NewRequest(&playv1.ListCharacterEffectsRequest{CampaignId: a.campaignID}))
		if err != nil {
			t.Fatalf("ListCharacterEffects() error = %v", err)
		}
		n := 0
		for _, f := range res.Msg.GetEffects() {
			if f.GetCharacterId() == characterID {
				n++
			}
		}
		return n
	}
	return fmt.Sprintf("%d/%d/%d", count(a.master), count(owner), count(other))
}

// seenInCombat counts, as "master/owner/other", the effects each reads on the combatant.
func (a *armed) seenInCombat(t *testing.T, owner, other *user, label string) string {
	t.Helper()
	count := func(u *user) int { return len(byLabel(t, a.get(t, u), label).GetEffects()) }
	return fmt.Sprintf("%d/%d/%d", count(a.master), count(owner), count(other))
}

// hearers are the streams of the master and of the three players of the party.
type hearers struct{ master, caio, ana, bia *watcher }

// listen opens the four streams and reads each one's first event.
func (a *armed) listen(t *testing.T) hearers {
	t.Helper()
	h := hearers{master: a.master.watch(t, a.campaignID), caio: a.caio.watch(t, a.campaignID), ana: a.ana.watch(t, a.campaignID), bia: a.bia.watch(t, a.campaignID)}
	for _, w := range []*watcher{h.master, h.caio, h.ana, h.bia} {
		w.ready(t)
	}
	return h
}

// heard marks the streams and returns, for each of the master, Caio (Toren's player), Ana
// (Pensantus's) and Bia (Brisa's), the kinds of event that came before the marker.
func (a *armed) heard(t *testing.T, h hearers) string {
	t.Helper()
	a.master.markCurrentMap(t, a.campaignID, a.mapID, h.master, h.caio, h.ana, h.bia)
	kinds := func(w *watcher) string {
		var out []string
		for _, ev := range w.beforeMarker(t) {
			out = append(out, strings.TrimPrefix(strings.TrimSuffix(fmt.Sprintf("%T", ev.GetEvent()), "_"), "*playv1.WatchGameSessionResponse_"))
		}
		return strings.Join(out, ",")
	}
	return fmt.Sprintf("master[%s] caio[%s] ana[%s] bia[%s]", kinds(h.master), kinds(h.caio), kinds(h.ana), kinds(h.bia))
}

// advanceGameTimeKeyed is advanceGameTime with the idempotency key of the caller.
func (a *armed) advanceGameTimeKeyed(t *testing.T, key string, seconds int32) {
	t.Helper()
	if _, err := a.master.lasting.AdvanceGameTime(t.Context(), connect.NewRequest(&playv1.AdvanceGameTimeRequest{CampaignId: a.campaignID, IdempotencyKey: key, Seconds: seconds})); err != nil {
		t.Fatalf("AdvanceGameTime() error = %v", err)
	}
}

// encounterEvents are the lasting_effect_added and conditions_set events of the campaign's
// session, one line each, in order.
func (a *armed) encounterEvents(t *testing.T, e *playv1.Encounter) []string {
	t.Helper()
	rows, err := a.h.pool.Query(t.Context(), `
		SELECT kind || ' ' || payload::TEXT || ' key=' || (idempotency_key IS NOT NULL)::TEXT || ' enc=' || (encounter_id IS NOT NULL)::TEXT
			|| ' char=' || (character_id IS NOT NULL)::TEXT
		FROM session_events WHERE kind IN ('lasting_effect_added', 'conditions_set') AND game_session_id IN (SELECT id FROM game_sessions WHERE campaign_id = $1) ORDER BY seq`, a.campaignID)
	if err != nil {
		t.Fatalf("read the events: %v", err)
	}
	defer rows.Close()
	names := a.partyNames(t, e)
	var out []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, plain(dbtest.SortJSONKeys(line), names))
	}
	return out
}

// capturedLog is what the service logs, as one JSON line each.
type capturedLog struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *capturedLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

// take returns the lines logged since the last call, without the time and with the ids named.
func (l *capturedLog) take(names map[string]string) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := plain(strings.TrimSpace(l.buf.String()), names)
	l.buf.Reset()
	return out
}

// captureLog has the service log its debug lines to the returned buffer from now on.
func (a *armed) captureLog() *capturedLog {
	l := &capturedLog{}
	a.h.svc.logger = slog.New(slog.NewJSONHandler(l, &slog.HandlerOptions{
		Level: slog.LevelDebug,
		ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr {
			if len(groups) == 0 && attr.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return attr
		},
	}))
	return l
}

// logOf is the combat log as the user reads it, as JSON without ids or times.
func (a *armed) logOf(t *testing.T, u *user, e *playv1.Encounter) string {
	t.Helper()
	var compact bytes.Buffer // protojson adds random spaces on purpose
	if err := json.Compact(&compact, []byte(asJSON(t, a.log(t, u, e)))); err != nil {
		t.Fatalf("compact the log: %v", err)
	}
	return plain(compact.String(), a.partyNames(t, e))
}
