package play

import (
	"maps"
	"slices"
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
)

// "Ver pelos olhos do familiar" (MR-036, Etapa 9, slice 9.10): Pensantus's familiar
// is the owl Nanquim. The fog's side (what the player then sees) is in package maps'
// tests; these are the rules of starting and stopping it. They need the database
// (MEURPG_TEST_DATABASE_URL).

var (
	blockedFamiliar = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_FAMILIAR_SIGHT_BLOCKED
	blindAndDeaf    = []string{"condition:blinded", "condition:deafened"}
)

// sightCall calls StartFamiliarSight or StopFamiliarSight as u.
func (a *armed) sightCall(t *testing.T, u *user, c *charactersv1.Character, start bool) (*playv1.CharacterVitals, *playv1.Encounter, error) {
	t.Helper()
	if start {
		res, err := u.play.StartFamiliarSight(t.Context(), connect.NewRequest(&playv1.StartFamiliarSightRequest{CampaignId: a.campaignID, CharacterId: c.GetId(), IdempotencyKey: newKey()}))
		if err != nil {
			return nil, nil, err
		}
		return res.Msg.GetVitals(), res.Msg.GetEncounter(), nil
	}
	res, err := u.play.StopFamiliarSight(t.Context(), connect.NewRequest(&playv1.StopFamiliarSightRequest{CampaignId: a.campaignID, CharacterId: c.GetId(), IdempotencyKey: newKey()}))
	if err != nil {
		return nil, nil, err
	}
	return res.Msg.GetVitals(), res.Msg.GetEncounter(), nil
}

func (a *armed) mustSight(t *testing.T, u *user, c *charactersv1.Character, start bool) (*playv1.CharacterVitals, *playv1.Encounter) {
	t.Helper()
	v, e, err := a.sightCall(t, u, c, start)
	if err != nil {
		t.Fatalf("familiar sight (start %v) error = %v", start, err)
	}
	return v, e
}

func wantSightBlocked(t *testing.T, err error, want playv1.FamiliarSightBlockedReason) {
	t.Helper()
	b := wantEncounterBlocked(t, err, blockedFamiliar)
	if b.GetFamiliarSightReason() != want {
		t.Errorf("familiar sight reason = %v, want %v", b.GetFamiliarSightReason(), want)
	}
}

// nanquim gives Pensantus his familiar (the ritual) and returns its ID.
func (s *shapers) nanquim(t *testing.T) string {
	t.Helper()
	s.mustCastSummon(t, s.ana, s.pens, findFamiliar, nil, 0, []string{"monster:owl"}, "Nanquim")
	list := s.mustCreatures(t, s.ana, s.pens)
	if len(list) != 1 {
		t.Fatalf("creatures = %v, want Nanquim", list)
	}
	return list[0].GetId()
}

// centerBP is the middle of a square of the test map (20 x 10 squares), in basis
// points.
func centerBP(col, row int) (x, y int) {
	g := link.Grid{Columns: gridColumns, Rows: gridColumns / 2}
	xb, yb := centerOf(g, int32(col), int32(row)) //nolint:gosec // a 20 x 10 test map
	return int(xb), int(yb)
}

// placeCreatureToken puts a creature's token on a square of the map.
func (a *armed) placeCreatureToken(t *testing.T, creatureID string, col, row int) {
	t.Helper()
	x, y := centerBP(col, row)
	if _, err := a.h.pool.Exec(t.Context(), `
		INSERT INTO map_creature_tokens (map_id, creature_id, x_bp, y_bp, updated_at) VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (map_id, creature_id) DO UPDATE SET x_bp = excluded.x_bp, y_bp = excluded.y_bp`, a.mapID, creatureID, x, y); err != nil {
		t.Fatalf("place the creature's token: %v", err)
	}
}

// TestMR036_FamiliarSightOutsideACombat: the player starts looking through the
// familiar's eyes when it is on the same map within 30 m, and stops when they want;
// it needs a familiar, both tokens and a grid; only the character's player and the
// master may; and the history has it with ids only.
func TestMR036_FamiliarSightOutsideACombat(t *testing.T) {
	t.Parallel()
	s := newShapers(t)
	a := s.armed

	// No familiar yet.
	_, _, err := a.sightCall(t, s.ana, s.pens, true)
	wantSightBlocked(t, err, playv1.FamiliarSightBlockedReason_FAMILIAR_SIGHT_BLOCKED_REASON_NO_FAMILIAR)
	_, _, err = a.sightCall(t, s.ana, s.pens, false)
	wantSightBlocked(t, err, playv1.FamiliarSightBlockedReason_FAMILIAR_SIGHT_BLOCKED_REASON_NOT_SEEING)

	owl := s.nanquim(t)
	// Another player may not do it for Pensantus; the master may.
	_, _, err = a.sightCall(t, s.caio, s.pens, true)
	wantCode(t, "StartFamiliarSight by another player", err, connect.CodePermissionDenied)

	// Neither token is on the map yet.
	_, _, err = a.sightCall(t, s.ana, s.pens, true)
	wantSightBlocked(t, err, playv1.FamiliarSightBlockedReason_FAMILIAR_SIGHT_BLOCKED_REASON_NOT_ON_MAP)
	x, y := centerBP(0, 0)
	a.h.placeToken(a.mapID, s.pens.GetId(), x, y)
	_, _, err = a.sightCall(t, s.ana, s.pens, true)
	wantSightBlocked(t, err, playv1.FamiliarSightBlockedReason_FAMILIAR_SIGHT_BLOCKED_REASON_NOT_ON_MAP)

	// 21 squares away is more than 30 m (20 squares); 20 squares away is not.
	a.placeCreatureToken(t, owl, 19, 9)
	_, _, err = a.sightCall(t, s.ana, s.pens, true)
	wantSightBlocked(t, err, playv1.FamiliarSightBlockedReason_FAMILIAR_SIGHT_BLOCKED_REASON_TOO_FAR)
	a.placeCreatureToken(t, owl, 12, 9) // 15 squares away
	v, e := a.mustSight(t, s.ana, s.pens, true)
	if e != nil {
		t.Errorf("encounter = %v, want none: there is no combat", e)
	}
	got := v.GetFamiliarSight()
	if got.GetCreatureId() != owl || got.GetInCombat() || len(got.GetConditionsGiven()) != 0 {
		t.Errorf("familiar sight = %v, want the owl, outside a combat, with no conditions given", got)
	}
	// The player's own read has it, and so does the master's.
	for _, u := range []*user{s.ana, s.master} {
		var seen bool
		for _, vit := range u.liveSession(t, s.campaignID).GetVitals() {
			seen = seen || vit.GetFamiliarSight().GetCreatureId() == owl
		}
		if !seen {
			t.Errorf("a live session read lacks the familiar's sight")
		}
	}
	_, _, err = a.sightCall(t, s.ana, s.pens, true)
	wantSightBlocked(t, err, playv1.FamiliarSightBlockedReason_FAMILIAR_SIGHT_BLOCKED_REASON_ALREADY_SEEING)

	// Out of a combat it lasts until the player stops.
	v, _ = a.mustSight(t, s.ana, s.pens, false)
	if v.GetFamiliarSight() != nil {
		t.Errorf("after stopping = %v, want no sight", v.GetFamiliarSight())
	}
	_, _, err = a.sightCall(t, s.ana, s.pens, false)
	wantSightBlocked(t, err, playv1.FamiliarSightBlockedReason_FAMILIAR_SIGHT_BLOCKED_REASON_NOT_SEEING)
	// The master may start it for him too, and a dismissed familiar ends it.
	a.mustSight(t, s.master, s.pens, true)
	if _, err := s.ana.characters.DismissCreature(t.Context(), connect.NewRequest(&charactersv1.DismissCreatureRequest{CampaignId: s.campaignID, CreatureId: owl})); err != nil {
		t.Fatalf("DismissCreature() error = %v", err)
	}
	for _, vit := range s.ana.liveSession(t, s.campaignID).GetVitals() {
		if vit.GetFamiliarSight() != nil {
			t.Errorf("a dismissed familiar leaves the sight = %v, want none", vit.GetFamiliarSight())
		}
	}

	ev := a.lastPayload(t, eventFamiliarSight)
	if ev["sight"] != "start" || ev["creature_id"] != owl || ev["character_id"] != s.pens.GetId() {
		t.Errorf("familiar_sight = %v, want the start with ids", ev)
	}
	if _, has := ev["name"]; has {
		t.Errorf("familiar_sight = %v: no name belongs in an event", ev)
	}
}

// pensantusFirst starts the fight with Pensantus first and his familiar next to him.
func (s *shapers) pensantusFirst(t *testing.T, at map[string][2]int32) *playv1.Encounter {
	t.Helper()
	places := map[string][2]int32{
		"Pensantus": {10, 3}, "Nanquim": {12, 3}, "Sálvia": {6, 5}, "Irmã": {7, 5}, "Toren": {2, 2}, "Capitão Goblin": {5, 8}, "Goblin": {6, 8},
	}
	maps.Copy(places, at)
	return s.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: s.capitao.GetId()}, {CharacterId: s.goblin.GetId()}},
		npcRolls: []int{1, 1},
		players:  map[string]int32{"Pensantus": 20, "Sálvia": 15, "Irmã": 12, "Toren": 10, "Nanquim": 8},
		reveal:   []string{"Capitão Goblin", "Goblin"},
		at:       places,
	})
}

// TestMR036_FamiliarSightInCombat: in a combat it costs the action, makes the
// character blind and deaf for the master's reminder, and ends at the start of the
// character's next turn; stopping early refunds nothing; the master's undo of the start
// gives the action back.
func TestMR036_FamiliarSightInCombat(t *testing.T) {
	t.Parallel()
	s := newShapers(t)
	a := s.armed
	s.nanquim(t)
	e := s.pensantusFirst(t, nil)
	pens := byLabel(t, e, "Pensantus")
	if e.GetCurrentCombatantId() != pens.GetId() || byLabel(t, e, "Nanquim").GetOwnerCharacterId() != s.pens.GetId() {
		t.Fatalf("setup: current %s, want Pensantus's turn with Nanquim in the fight", e.GetCurrentCombatantId())
	}

	// On his turn only: Toren's player may not, and it is not Sálvia's player's call.
	_, _, err := a.sightCall(t, s.caio, s.pens, true)
	wantCode(t, "StartFamiliarSight by another player", err, connect.CodePermissionDenied)

	v, enc := a.mustSight(t, s.ana, s.pens, true)
	if got := v.GetFamiliarSight(); !got.GetInCombat() || !slices.Equal(got.GetConditionsGiven(), blindAndDeaf) {
		t.Fatalf("familiar sight = %v, want in a combat and the two conditions given", got)
	}
	got := byLabel(t, enc, "Pensantus")
	if !got.GetActionUsed() || !slices.Equal(got.GetConditions(), blindAndDeaf) || got.GetFamiliarSightCreatureId() == "" {
		t.Errorf("combatant = action %v, conditions %v, sight %q; want the action spent, blinded and deafened, the owl", got.GetActionUsed(), got.GetConditions(), got.GetFamiliarSightCreatureId())
	}
	// Who is told which creature: the owner and the master; the others see the conditions only.
	if other := byLabel(t, a.get(t, s.caio), "Pensantus"); other.GetFamiliarSightCreatureId() != "" || !slices.Equal(other.GetConditions(), blindAndDeaf) {
		t.Errorf("another player's copy = sight %q, conditions %v, want no creature ID", other.GetFamiliarSightCreatureId(), other.GetConditions())
	}
	if byLabel(t, a.get(t, s.master), "Pensantus").GetFamiliarSightCreatureId() == "" {
		t.Error("the master's copy lacks the familiar's ID")
	}

	// Stopping early takes the conditions away and gives nothing back.
	v, enc = a.mustSight(t, s.ana, s.pens, false)
	got = byLabel(t, enc, "Pensantus")
	if v.GetFamiliarSight() != nil || len(got.GetConditions()) != 0 || !got.GetActionUsed() {
		t.Errorf("after stopping: sight %v, conditions %v, action used %v; want none, none, still spent", v.GetFamiliarSight(), got.GetConditions(), got.GetActionUsed())
	}
	_, _, err = a.sightCall(t, s.ana, s.pens, true)
	wantEncounterBlocked(t, err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ACTION_USED)
	// The master's undo of the stop is none: only the start can be undone, and the
	// last change was a stop.
	if id := a.log(t, a.master, enc).GetUndoableEventId(); id != "" {
		t.Errorf("undoable event after a stop = %s, want none", id)
	}

	// Next round: a new turn, a new action; start again, end the turn, and the sight
	// ends with the next turn.
	enc = a.passTo(t, a.mustEndTurn(t, s.ana, enc), "Pensantus")
	if byLabel(t, enc, "Pensantus").GetActionUsed() {
		t.Fatal("the action did not come back in the new round")
	}
	a.mustSight(t, s.ana, s.pens, true)
	enc = a.mustEndTurn(t, s.ana, a.get(t, s.ana))
	if v := a.vitals(t, s.pens); v.GetFamiliarSight() == nil {
		t.Fatalf("the sight ended when the turn did, want it to last until the next turn starts: %v", v)
	}
	enc = a.passTo(t, enc, "Pensantus")
	got = byLabel(t, enc, "Pensantus")
	if v := a.vitals(t, s.pens); v.GetFamiliarSight() != nil || len(got.GetConditions()) != 0 {
		t.Errorf("at the start of his next turn: sight %v, conditions %v; want none", v.GetFamiliarSight(), got.GetConditions())
	}
	if ev := a.lastPayload(t, eventFamiliarSight); ev["sight"] != "stop" || ev["reason"] != sightTurnStart {
		t.Errorf("familiar_sight = %v, want the stop at the turn's start", ev)
	}

	// The undo of a start: the action is back and nobody is blind.
	a.mustSight(t, s.ana, s.pens, true)
	a.undoLast(t, a.get(t, s.master))
	got = byLabel(t, a.get(t, s.master), "Pensantus")
	if a.vitals(t, s.pens).GetFamiliarSight() != nil || got.GetActionUsed() || len(got.GetConditions()) != 0 {
		t.Errorf("after undoing the start: sight %v, action used %v, conditions %v; want nothing changed", a.vitals(t, s.pens).GetFamiliarSight(), got.GetActionUsed(), got.GetConditions())
	}
}

// TestMR036_FamiliarSightKeepsAConditionTheCharacterHad: a condition the character
// already had is never given by the sight, so ending the sight never takes it away.
func TestMR036_FamiliarSightKeepsAConditionTheCharacterHad(t *testing.T) {
	t.Parallel()
	s := newShapers(t)
	a := s.armed
	s.nanquim(t)
	e := s.pensantusFirst(t, nil)
	a.setConditions(t, e, "Pensantus", "condition:deafened")
	v, _ := a.mustSight(t, s.ana, s.pens, true)
	if got := v.GetFamiliarSight().GetConditionsGiven(); !slices.Equal(got, []string{"condition:blinded"}) {
		t.Fatalf("conditions given = %v, want only the blinded: he was deafened already", got)
	}
	_, enc := a.mustSight(t, s.ana, s.pens, false)
	if got := byLabel(t, enc, "Pensantus").GetConditions(); !slices.Equal(got, []string{"condition:deafened"}) {
		t.Errorf("conditions after stopping = %v, want the deafened he had", got)
	}
}

// TestMR036_FamiliarSightNeedsTheFamiliarWithin30m: in a combat the squares are the
// combatants'.
func TestMR036_FamiliarSightNeedsTheFamiliarWithin30m(t *testing.T) {
	t.Parallel()
	s := newShapers(t)
	a := s.armed
	s.nanquim(t)
	s.pensantusFirst(t, map[string][2]int32{"Pensantus": {0, 0}, "Nanquim": {19, 9}})
	_, _, err := a.sightCall(t, s.ana, s.pens, true)
	wantSightBlocked(t, err, playv1.FamiliarSightBlockedReason_FAMILIAR_SIGHT_BLOCKED_REASON_TOO_FAR)
	if got := a.vitals(t, s.pens); got.GetFamiliarSight() != nil || byLabel(t, a.get(t, s.master), "Pensantus").GetActionUsed() {
		t.Errorf("a refused start changed something: %v", got)
	}
}

// TestMR037_ACreatureTokenFollowsItsCombatantWhenTheFightEnds: the master gave Nanquim a
// token; when the combat ends it stands where its combatant ended, and a creature with
// no token never gets one from the fight.
func TestMR037_ACreatureTokenFollowsItsCombatantWhenTheFightEnds(t *testing.T) {
	t.Parallel()
	s := newShapers(t)
	a := s.armed
	owl := s.nanquim(t)
	a.placeCreatureToken(t, owl, 1, 1)
	e := s.pensantusFirst(t, nil) // Nanquim at (12, 3)
	a.endEncounter(t, e)
	var x, y int
	if err := a.h.pool.QueryRow(t.Context(), `SELECT x_bp, y_bp FROM map_creature_tokens WHERE map_id = $1 AND creature_id = $2`, a.mapID, owl).Scan(&x, &y); err != nil {
		t.Fatalf("read the owl's token: %v", err)
	}
	if wantX, wantY := centerBP(12, 3); x != wantX || y != wantY {
		t.Errorf("the owl's token = (%d, %d), want the center of (12, 3): (%d, %d)", x, y, wantX, wantY)
	}
	var n int
	if err := a.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM map_creature_tokens WHERE map_id = $1`, a.mapID).Scan(&n); err != nil || n != 1 {
		t.Errorf("creature tokens = %d, %v, want the one the master placed", n, err)
	}
}

// TestMR036_FamiliarSightEndsWithTheCombat: a sight begun in a fight has no next turn to
// end it once the fight is over, so it ends with the combat.
func TestMR036_FamiliarSightEndsWithTheCombat(t *testing.T) {
	t.Parallel()
	s := newShapers(t)
	a := s.armed
	s.nanquim(t)
	e := s.pensantusFirst(t, nil)
	a.mustSight(t, s.ana, s.pens, true)
	a.endEncounter(t, e)
	if v := a.vitals(t, s.pens); v.GetFamiliarSight() != nil {
		t.Errorf("after the combat ended: %v, want no sight", v.GetFamiliarSight())
	}
	if got := byLabel(t, a.get(t, a.master), "Pensantus"); len(got.GetConditions()) != 0 {
		t.Errorf("conditions after the combat ended = %v, want none", got.GetConditions())
	}
}

// A familiar dismissed while its eyes are on in a combat leaves the sight in place until it ends (the
// owner's next turn, or the combat), and ending it takes back the conditions it gave.
func TestMR036_FamiliarSightEndsConditionsWhenTheFamiliarIsDismissed(t *testing.T) {
	t.Parallel()
	s := newShapers(t)
	a := s.armed
	owl := s.nanquim(t)
	s.pensantusFirst(t, nil)
	a.mustSight(t, s.ana, s.pens, true)
	if _, err := s.ana.characters.DismissCreature(t.Context(), connect.NewRequest(&charactersv1.DismissCreatureRequest{CampaignId: s.campaignID, CreatureId: owl})); err != nil {
		t.Fatalf("DismissCreature() error = %v", err)
	}
	// His next turn starts.
	a.passTo(t, a.mustEndTurn(t, s.ana, a.get(t, s.ana)), "Pensantus")
	if got := byLabel(t, a.get(t, s.master), "Pensantus").GetConditions(); len(got) != 0 {
		t.Errorf("conditions at the start of his next turn after the familiar was dismissed = %v, want none (the sight's blinded/deafened)", got)
	}
	// And ending the combat must not leave them either.
	a.endEncounter(t, a.get(t, s.master))
	if got := byLabel(t, a.get(t, s.master), "Pensantus").GetConditions(); len(got) != 0 {
		t.Errorf("conditions after the combat ended = %v, want none", got)
	}
}
