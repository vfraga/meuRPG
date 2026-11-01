package play

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1/mapsv1connect"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/maps"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
	"github.com/PuraFome/meuRPG/backend/internal/platform/httpserver"
	"github.com/PuraFome/meuRPG/backend/internal/platform/reqlimit"
)

// The character's creatures (MR-037): the familiar, the
// summoned animals and undead, the master's gift, and the creatures in a combat.
// These tests need the database (MEURPG_TEST_DATABASE_URL). The fixture is the
// party of the designs: Toren, a level 5 fighter; Pensantus, a wizard
// who knows Convocar Familiar and Animar Mortos (level 9, so he has
// 5th-circle slots); and Sálvia, a level 5 druid with Conjurar Animais.

const (
	findFamiliar   = "spell:find-familiar"
	animateDead    = "spell:animate-dead"
	conjureAnimals = "spell:conjure-animals"
	direWolf       = "monster:dire-wolf"
	direWolfBite   = "monster:dire-wolf#bite"
)

// newSummoners is the party of this file.
func newSummoners(t *testing.T) *armed {
	t.Helper()
	return newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 5,
			&rulesv1.AbilityScores{Strength: 16, Dexterity: 13, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{battleaxe}, nil)
		a.pens = a.ana.caster(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 9,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8}, nil, []string{fireBolt},
			[]string{findFamiliar, animateDead, magicMissileSpell}, []string{findFamiliar, animateDead, magicMissileSpell})
		a.bri = a.bia.caster(t, a.campaignID, "Sálvia", "class:druid", "race:human", 5,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 12, Constitution: 14, Intelligence: 10, Wisdom: 16, Charisma: 8}, nil, nil, nil,
			[]string{conjureAnimals})
	})
}

// castSummon calls CastSummon as u, with a key of its own.
func (a *armed) castSummon(t *testing.T, u *user, c *charactersv1.Character, spell string, slot *playv1.SpellSlot, option int32, keys []string, names ...string) (*playv1.CastSummonResponse, error) {
	t.Helper()
	res, err := u.play.CastSummon(t.Context(), connect.NewRequest(&playv1.CastSummonRequest{
		CampaignId: a.campaignID, CharacterId: c.GetId(), SpellKey: spell, Ritual: slot == nil, Slot: slot, IdempotencyKey: newKey(),
		Summon: &playv1.SummonChoice{Option: option, CreatureKeys: keys, Names: names},
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (a *armed) mustCastSummon(t *testing.T, u *user, c *charactersv1.Character, spell string, slot *playv1.SpellSlot, option int32, keys []string, names ...string) *playv1.CastSummonResponse {
	t.Helper()
	res, err := a.castSummon(t, u, c, spell, slot, option, keys, names...)
	if err != nil {
		t.Fatalf("CastSummon(%s) error = %v", spell, err)
	}
	return res
}

// creatures lists a character's creatures as u.
func (a *armed) creatures(t *testing.T, u *user, c *charactersv1.Character) ([]*charactersv1.CharacterCreature, error) {
	t.Helper()
	res, err := u.characters.ListCharacterCreatures(t.Context(), connect.NewRequest(&charactersv1.ListCharacterCreaturesRequest{CampaignId: a.campaignID, CharacterId: c.GetId()}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetCreatures(), nil
}

func (a *armed) mustCreatures(t *testing.T, u *user, c *charactersv1.Character) []*charactersv1.CharacterCreature {
	t.Helper()
	out, err := a.creatures(t, u, c)
	if err != nil {
		t.Fatalf("ListCharacterCreatures(%s) error = %v", c.GetName(), err)
	}
	return out
}

func creatureNames(cs []*charactersv1.CharacterCreature) []string {
	var out []string
	for _, c := range cs {
		out = append(out, c.GetName())
	}
	return out
}

// give gives a creature to a character as the master.
func (a *armed) give(t *testing.T, c *charactersv1.Character, key, name string) *charactersv1.CharacterCreature {
	t.Helper()
	res, err := a.master.characters.GiveCreature(t.Context(), connect.NewRequest(&charactersv1.GiveCreatureRequest{
		CampaignId: a.campaignID, CharacterId: c.GetId(), MonsterKey: key, Name: name,
	}))
	if err != nil {
		t.Fatalf("GiveCreature(%s) error = %v", key, err)
	}
	return res.Msg.GetCreature()
}

// TestGiveCreatureLocksTheSessionBeforeTheOwner: a gift waits for the open
// session's lock before it locks the character, the order every combat write
// uses (the session, then what it changes). Locking the character first and the
// session later, when the gift writes its event, lets a gift and a combat start
// each hold what the other wants.
func TestGiveCreatureLocksTheSessionBeforeTheOwner(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	ctx := t.Context()
	holder, err := dbtest.SideConnection(t, a.h.pool).Begin(ctx)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	defer holder.Rollback(ctx) //nolint:errcheck // the test never commits
	if _, err := holder.Exec(ctx, `SELECT id FROM game_sessions WHERE campaign_id = $1 AND ended_at IS NULL FOR UPDATE`, a.campaignID); err != nil {
		t.Fatalf("lock the open session: %v", err)
	}

	given := make(chan error, 1)
	go func() {
		_, err := a.master.characters.GiveCreature(ctx, connect.NewRequest(&charactersv1.GiveCreatureRequest{
			CampaignId: a.campaignID, CharacterId: a.toren.GetId(), MonsterKey: "monster:wolf", IdempotencyKey: newKey(),
		}))
		given <- err
	}()

	// Wait, on a condition and not on a clock, until the gift is stopped at the
	// session's lock.
	observer := dbtest.SideConnection(t, a.h.pool)
	for waiting := false; !waiting; {
		n, err := dbtest.WaitingStatements(ctx, observer, `%FROM game_sessions%FOR UPDATE%`)
		if err != nil {
			t.Fatalf("look for the waiting gift: %v", err)
		}
		waiting = n > 0
		select {
		case err := <-given:
			t.Fatalf("GiveCreature() returned while the session was locked: %v", err)
		default:
		}
	}
	tx, err := observer.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // the test never commits
	if _, err := tx.Exec(ctx, `SELECT id FROM characters WHERE id = $1 FOR UPDATE NOWAIT`, a.toren.GetId()); err != nil {
		t.Errorf("the character is locked while the gift waits for the session: %v", err)
	}
	_ = tx.Rollback(ctx)

	if err := holder.Rollback(ctx); err != nil {
		t.Fatalf("release the session: %v", err)
	}
	if err := <-given; err != nil {
		t.Errorf("GiveCreature() error = %v once the session was free", err)
	}
}

// eventCount counts the session's events of a kind.
func (a *armed) eventCount(t *testing.T, kind string) int {
	t.Helper()
	var n int
	if err := a.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM session_events WHERE kind = $1`, kind).Scan(&n); err != nil {
		t.Fatalf("count the %s events: %v", kind, err)
	}
	return n
}

// summonersFight starts a combat of the summoners and the goblin: Pensantus
// first, then Sálvia, then Toren, with everyone on the map. It is Pensantus's
// turn.
func (a *armed) summonersFight(t *testing.T) *playv1.Encounter {
	t.Helper()
	return a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId()}},
		npcRolls: []int{2},
		players:  map[string]int32{"Pensantus": 20, "Sálvia": 15, "Toren": 12},
		reveal:   []string{"Goblin"},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Pensantus": {10, 3}, "Sálvia": {6, 5}},
	})
}

// summonWolves casts Conjurar Animais in a combat as the druid's player: two
// dire wolves (the option of two creatures of challenge rating 1), with the
// given d20 for their initiative.
func (a *armed) summonWolves(t *testing.T, u *user, e *playv1.Encounter, face int32) (*playv1.CastSpellResponse, error) {
	t.Helper()
	return a.cast(t, u, e, "Sálvia", conjureAnimals, slotOfLevel(3), nil, func(r *playv1.CastSpellRequest) {
		r.Roll = &playv1.CastSpellRequest_D20Face{D20Face: face}
		r.Summon = &playv1.SummonChoice{Option: 1, CreatureKeys: []string{direWolf, direWolf}}
	})
}

// toSalvia passes Pensantus's turn, so it is Sálvia's.
func (a *armed) toSalvia(t *testing.T, e *playv1.Encounter) *playv1.Encounter {
	t.Helper()
	out := a.mustEndTurn(t, a.ana, e)
	if cur := byLabel(t, out, "Sálvia"); out.GetCurrentCombatantId() != cur.GetId() {
		t.Fatalf("current = %s, want Sálvia's turn", out.GetCurrentCombatantId())
	}
	return out
}

// TestMR037_TheRitualFamiliarSpendsNoSlotAndANewOneReplacesTheOld: Encontrar
// Familiar takes 1 hour and is a ritual, so it is cast outside a combat with no
// slot; the choice is checked by the rules; one familiar at a time.
func TestMR037_TheRitualFamiliarSpendsNoSlotAndANewOneReplacesTheOld(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	before := usedSlots(a.vitals(t, a.pens), 1)

	res := a.mustCastSummon(t, a.ana, a.pens, findFamiliar, nil, 0, []string{"monster:owl"}, "Nanquim")
	if len(res.GetCreatureIds()) != 1 || res.GetVitals() != nil || len(res.GetDismissedCreatureIds()) != 0 {
		t.Fatalf("CastSummon() = %v, want one creature, no vitals (a ritual spends no slot), nothing dismissed", res)
	}
	if got := usedSlots(a.vitals(t, a.pens), 1); got != before {
		t.Errorf("1st-circle slots used = %d, want %d: a ritual spends none", got, before)
	}
	list := a.mustCreatures(t, a.ana, a.pens)
	if len(list) != 1 {
		t.Fatalf("creatures = %v, want Nanquim", list)
	}
	nanquim := list[0]
	if nanquim.GetName() != "Nanquim" || nanquim.GetMonsterKey() != "monster:owl" || nanquim.GetMonsterNamePt() != "Coruja" ||
		nanquim.GetSource() != charactersv1.CreatureSource_CREATURE_SOURCE_FAMILIAR || nanquim.GetAttack() != 1 ||
		nanquim.GetHitPointsCurrent() != 1 || nanquim.GetHitPointsMax() != 1 || nanquim.GetCharacterId() != a.pens.GetId() || nanquim.GetDependsOnConcentration() {
		t.Errorf("creature = %v", nanquim)
	}
	// Only the master and the owner's player read it: for another player the
	// creature, like the character's sheet, is the master's and the owner's (RN-20).
	wantCode(t, "ListCharacterCreatures as another player", func() error { _, err := a.creatures(t, a.caio, a.pens); return err }(), connect.CodeNotFound)
	if got := a.mustCreatures(t, a.master, a.pens); len(got) != 1 {
		t.Errorf("the master reads %v, want the familiar", got)
	}

	// The choice is the rules': an imp is the Pact of the Chain's, not a wizard's.
	_, err := a.castSummon(t, a.ana, a.pens, findFamiliar, nil, 0, []string{"monster:imp"})
	wantBlockedBy(t, "CastSummon(imp)", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_SUMMON_CHOICE_INVALID)
	if _, err := a.castSummon(t, a.ana, a.pens, findFamiliar, nil, 1, []string{"monster:owl"}); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("CastSummon(option 1) error = %v, want failed_precondition", err)
	}
	// Another player's character is not theirs to cast for.
	_, err = a.castSummon(t, a.caio, a.pens, findFamiliar, nil, 0, []string{"monster:owl"})
	wantCode(t, "CastSummon for another player's character", err, connect.CodePermissionDenied)

	// A new familiar replaces the old: one at a time. Cast with a slot this
	// time, which is spent.
	res = a.mustCastSummon(t, a.ana, a.pens, findFamiliar, slotOfLevel(1), 0, []string{"monster:raven"}, "Corvo")
	if len(res.GetDismissedCreatureIds()) != 1 || res.GetDismissedCreatureIds()[0] != nanquim.GetId() {
		t.Errorf("dismissed = %v, want Nanquim", res.GetDismissedCreatureIds())
	}
	if got := usedSlots(res.GetVitals(), 1); got != before+1 {
		t.Errorf("1st-circle slots used = %d, want %d: a casting with a slot spends it", got, before+1)
	}
	if got := creatureNames(a.mustCreatures(t, a.ana, a.pens)); len(got) != 1 || got[0] != "Corvo" {
		t.Errorf("creatures = %v, want only Corvo: a new familiar replaces the old one", got)
	}
	if n := a.eventCount(t, "creature_summoned"); n != 2 {
		t.Errorf("creature_summoned events = %d, want 2", n)
	}
	var payload string
	if err := a.h.pool.QueryRow(t.Context(), `SELECT payload::TEXT FROM session_events WHERE kind = 'creature_summoned' ORDER BY seq LIMIT 1`).Scan(&payload); err != nil {
		t.Fatalf("read the event: %v", err)
	}
	if strings.Contains(payload, "Nanquim") || !strings.Contains(payload, "monster:owl") {
		t.Errorf("payload = %s, want ids and keys only (never the name a player typed)", payload)
	}
}

// TestMR037_AnimateDeadAtTheThirdAndFifthCircles: Animar Mortos takes 1
// minute and the slot; 1 creature at the 3rd circle, 2 more for each circle
// above it.
func TestMR037_AnimateDeadAtTheThirdAndFifthCircles(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)

	_, err := a.castSummon(t, a.ana, a.pens, animateDead, slotOfLevel(3), 0, []string{"monster:skeleton", "monster:zombie"})
	wantBlockedBy(t, "CastSummon(two at the 3rd circle)", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_SUMMON_CHOICE_INVALID)
	if got := usedSlots(a.vitals(t, a.pens), 3); got != 0 {
		t.Fatalf("3rd-circle slots used = %d after a refused casting, want 0", got)
	}
	// A ritual is only for a ritual spell.
	if _, err := a.castSummon(t, a.ana, a.pens, animateDead, nil, 0, []string{"monster:skeleton"}); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("CastSummon(ritual Animar Mortos) error = %v, want invalid_argument", err)
	}

	res := a.mustCastSummon(t, a.ana, a.pens, animateDead, slotOfLevel(3), 0, []string{"monster:skeleton"})
	if len(res.GetCreatureIds()) != 1 || usedSlots(res.GetVitals(), 3) != 1 {
		t.Fatalf("3rd circle: %v, want 1 creature and the slot spent", res)
	}
	res = a.mustCastSummon(t, a.ana, a.pens, animateDead, slotOfLevel(5), 0, []string{"monster:skeleton", "monster:zombie", "monster:zombie", "monster:skeleton", "monster:skeleton"})
	if len(res.GetCreatureIds()) != 5 || usedSlots(res.GetVitals(), 5) != 1 {
		t.Fatalf("5th circle: %v, want 5 creatures and the slot spent", res)
	}
	list := a.mustCreatures(t, a.ana, a.pens)
	if len(list) != 6 {
		t.Fatalf("creatures = %v, want 1 + 5", creatureNames(list))
	}
	groups := map[string]int{}
	for _, c := range list {
		groups[c.GetSummonGroupId()]++
		if c.GetSource() != charactersv1.CreatureSource_CREATURE_SOURCE_ANIMATE_DEAD || c.GetAttack() != 3 {
			t.Errorf("creature = %v, want an Animar Mortos creature that attacks", c)
		}
	}
	if len(groups) != 2 {
		t.Errorf("groups = %v, want one for each casting", groups)
	}
	// The same names read "Esqueleto 1"..., numbered when a casting has several of a kind.
	if got := creatureNames(list[1:]); got[0] != "Esqueleto 1" || got[1] != "Zumbi 1" || got[2] != "Zumbi 2" {
		t.Errorf("names = %v, want the kinds numbered within the casting", got)
	}
	// No 5th-circle slot is left.
	_, err = a.castSummon(t, a.ana, a.pens, animateDead, slotOfLevel(5), 0, []string{"monster:skeleton", "monster:skeleton", "monster:skeleton", "monster:skeleton", "monster:skeleton"})
	wantCode(t, "CastSummon without a free slot", err, connect.CodeInvalidArgument) // as CastSpell: a slot the caster has none free of
}

// TestMR037_ConjureAnimalsInCombat: Conjurar Animais (1 action, concentration) in
// a combat: two dire wolves join as combatants with one initiative roll for the
// group, and a group with the same total as another combatant takes a joint
// turn. A choice the rules refuse, a spell that takes an hour and a casting with
// no initiative roll are refused with a reason the screen can say.
func TestMR037_ConjureAnimalsInCombat(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	e := a.summonersFight(t)

	// Pensantus's turn: Convocar Familiar takes an hour (a combat has no room
	// for it).
	_, err := a.cast(t, a.ana, e, "Pensantus", findFamiliar, slotOfLevel(1), nil, func(r *playv1.CastSpellRequest) {
		r.Roll = &playv1.CastSpellRequest_D20Face{D20Face: 10}
		r.Summon = &playv1.SummonChoice{CreatureKeys: []string{"monster:owl"}}
	})
	wantBlockedBy(t, "CastSpell(Convocar Familiar)", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_CASTING_TIME_TOO_LONG)
	e = a.toSalvia(t, e)

	// The choices the rules refuse: a creature above the challenge rating, the
	// wrong number, no initiative roll.
	_, err = a.cast(t, a.bia, e, "Sálvia", conjureAnimals, slotOfLevel(3), nil, func(r *playv1.CastSpellRequest) {
		r.Roll = &playv1.CastSpellRequest_D20Face{D20Face: 11}
		r.Summon = &playv1.SummonChoice{Option: 1, CreatureKeys: []string{direWolf, "monster:giant-boar"}} // a boar is challenge rating 2
	})
	wantBlockedBy(t, "CastSpell(a boar in the option of CR 1)", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_SUMMON_CHOICE_INVALID)
	_, err = a.cast(t, a.bia, e, "Sálvia", conjureAnimals, slotOfLevel(3), nil, func(r *playv1.CastSpellRequest) {
		r.Roll = &playv1.CastSpellRequest_D20Face{D20Face: 11}
		r.Summon = &playv1.SummonChoice{Option: 1, CreatureKeys: []string{direWolf}}
	})
	wantBlockedBy(t, "CastSpell(one wolf for the option of two)", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_SUMMON_CHOICE_INVALID)
	_, err = a.cast(t, a.bia, e, "Sálvia", conjureAnimals, slotOfLevel(3), nil, func(r *playv1.CastSpellRequest) {
		r.Summon = &playv1.SummonChoice{Option: 1, CreatureKeys: []string{direWolf, direWolf}}
	})
	wantBlockedBy(t, "CastSpell(no initiative roll)", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_SUMMON_NEEDS_INITIATIVE)
	if got := usedSlots(a.vitals(t, a.bri), 3); got != 0 {
		t.Fatalf("3rd-circle slots used = %d after refused castings, want 0", got)
	}
	// A summoning spell takes no targets, and any other spell no summon.
	_, err = a.cast(t, a.bia, e, "Sálvia", conjureAnimals, slotOfLevel(3), a.at(t, "Goblin"), func(r *playv1.CastSpellRequest) {
		r.Roll = &playv1.CastSpellRequest_D20Face{D20Face: 11}
		r.Summon = &playv1.SummonChoice{Option: 1, CreatureKeys: []string{direWolf, direWolf}}
	})
	wantCode(t, "CastSpell(with a target)", err, connect.CodeInvalidArgument)

	// The cast: face 12 + the dire wolf's Dexterity modifier (+2) = 14, the total
	// Toren has (face 12 + 2), so the wolves and Toren take a joint turn.
	res, err := a.summonWolves(t, a.bia, e, 12)
	if err != nil {
		t.Fatalf("CastSpell(Conjurar Animais) error = %v", err)
	}
	if len(res.GetSummonedCombatantIds()) != 2 || usedSlots(a.vitals(t, a.bri), 3) != 1 {
		t.Fatalf("CastSpell() = %v, want two combatants and the 3rd-circle slot spent", res)
	}
	e = a.get(t, a.bia)
	w1, w2 := byLabel(t, e, "Lobo atroz 1"), byLabel(t, e, "Lobo atroz 2")
	for _, w := range []*playv1.Combatant{w1, w2} {
		if w.GetKind() != playv1.CombatantKind_COMBATANT_KIND_CREATURE || w.GetMine() || !w.GetControlledByMe() ||
			w.GetOwnerCharacterId() != a.bri.GetId() || w.GetCharacterId() != "" || w.GetMonsterKey() != direWolf || w.GetMonsterNamePt() != "Lobo atroz" ||
			w.GetCreatureAttack() != playv1.CreatureAttack_CREATURE_ATTACK_FULL || w.GetCreatureId() == "" || w.GetInitiative() != 14 || w.GetInitiativeFace() != 12 {
			t.Errorf("wolf = %v", w)
		}
		if w.GetHitPointsCurrent() != 37 || w.GetHitPointsMax() != 37 {
			t.Errorf("the owner reads %d/%d hit points, want 37/37", w.GetHitPointsCurrent(), w.GetHitPointsMax())
		}
		if !w.GetPlaced() {
			t.Errorf("%s has no square: a creature appears next to its owner", w.GetLabel())
		}
	}
	if w1.GetSummonGroupId() == "" || w1.GetSummonGroupId() != w2.GetSummonGroupId() || w1.GetCreatureId() == w2.GetCreatureId() {
		t.Errorf("the wolves came from one casting: groups %q and %q, creatures %q and %q", w1.GetSummonGroupId(), w2.GetSummonGroupId(), w1.GetCreatureId(), w2.GetCreatureId())
	}
	if salvia := byLabel(t, e, "Sálvia"); !salvia.GetMine() || !salvia.GetControlledByMe() || salvia.GetConcentrationSpell() != conjureAnimals {
		t.Errorf("Sálvia = %v, want mine, controlled and concentrating on Conjurar Animais", salvia)
	}
	list := a.mustCreatures(t, a.bia, a.bri)
	if len(list) != 2 || !list[0].GetDependsOnConcentration() || list[0].GetSource() != charactersv1.CreatureSource_CREATURE_SOURCE_CONJURE_ANIMALS {
		t.Errorf("Sálvia's creatures = %v, want the two wolves, on her concentration", list)
	}
	if n := a.eventCount(t, "creature_summoned"); n != 1 {
		t.Errorf("creature_summoned events = %d, want 1", n)
	}

	// The turn passes from Sálvia to the group of the same total: the wolves and
	// Toren act together.
	out := a.mustEndTurn(t, a.bia, e)
	ids := out.GetTurnGroupIds()
	if len(ids) != 3 || !contains(ids, w1.GetId()) || !contains(ids, w2.GetId()) || !contains(ids, byLabel(t, out, "Toren").GetId()) {
		t.Errorf("turn group = %v, want the two wolves and Toren (the same total of 14)", ids)
	}
	// Each wolf has an economy of its own, played by the owner's player.
	opts := a.mustOptions(t, a.bia, out, "Lobo atroz 1")
	if attackOption(opts, direWolfBite) == nil || !attackOption(opts, direWolfBite).GetEnabled() || standardOption(opts, "standard:attack") == nil {
		t.Errorf("a wolf's options = %v, want its Bite", opts.GetOptions())
	}
	if opts.GetOptions().GetEconomy().GetMovement().GetSpeedFt() != 50 {
		t.Errorf("a wolf's speed = %d ft, want 50", opts.GetOptions().GetEconomy().GetMovement().GetSpeedFt())
	}
}

// mustAttackAsReaction is the master's opportunity attack of an NPC (a reaction,
// so it needs no turn of its own), rolled in the app.
func (a *armed) mustAttackAsReaction(t *testing.T, e *playv1.Encounter, attacker, key, target string) *playv1.RollAttackResponse {
	t.Helper()
	res, err := a.attackAs(t, a.master, e, attacker, key, target, inAppRoll, true)
	if err != nil {
		t.Fatalf("RollAttack(%s -> %s as a reaction) error = %v", attacker, target, err)
	}
	return res
}

func contains(ids []string, id string) bool {
	return slices.Contains(ids, id)
}

// TestMR037_TheMastersGiftRenameAndDismiss: the master gives any SRD creature to
// a character; the owner or the master renames and dismisses it; hit points are
// the master's (RN-02) and outside a combat.
func TestMR037_TheMastersGiftRenameAndDismiss(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)

	wolf := a.give(t, a.toren, "monster:wolf", "Presa")
	if wolf.GetName() != "Presa" || wolf.GetSource() != charactersv1.CreatureSource_CREATURE_SOURCE_MASTER || wolf.GetAttack() != 3 ||
		wolf.GetHitPointsMax() != 11 || wolf.GetHitPointsCurrent() != 11 {
		t.Fatalf("gift = %v", wolf)
	}
	if def := a.give(t, a.toren, "monster:wolf", ""); def.GetName() != "Lobo" {
		t.Errorf("a gift with no name is called %q, want the kind's name", def.GetName())
	}
	// A player does not give, and the creature is not for another player's eyes.
	_, err := a.caio.characters.GiveCreature(t.Context(), connect.NewRequest(&charactersv1.GiveCreatureRequest{CampaignId: a.campaignID, CharacterId: a.toren.GetId(), MonsterKey: "monster:wolf"}))
	wantCode(t, "GiveCreature as a player", err, connect.CodePermissionDenied)
	_, err = a.master.characters.GiveCreature(t.Context(), connect.NewRequest(&charactersv1.GiveCreatureRequest{CampaignId: a.campaignID, CharacterId: a.toren.GetId(), MonsterKey: "monster:dragon-of-the-moon"}))
	wantCode(t, "GiveCreature(unknown creature)", err, connect.CodeInvalidArgument)
	_, err = a.master.characters.GiveCreature(t.Context(), connect.NewRequest(&charactersv1.GiveCreatureRequest{CampaignId: a.campaignID, CharacterId: a.goblin.GetId(), MonsterKey: "monster:wolf"}))
	wantCode(t, "GiveCreature to an NPC", err, connect.CodeInvalidArgument) // only a player's character has creatures

	rename := func(u *user, name string) (*charactersv1.RenameCreatureResponse, error) {
		res, err := u.characters.RenameCreature(t.Context(), connect.NewRequest(&charactersv1.RenameCreatureRequest{CampaignId: a.campaignID, CreatureId: wolf.GetId(), Name: name}))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	}
	if res, err := rename(a.caio, "Fúria"); err != nil || res.GetCreature().GetName() != "Fúria" {
		t.Fatalf("RenameCreature by the owner = %v, %v", res, err)
	}
	if res, err := rename(a.master, "Sombra"); err != nil || res.GetCreature().GetName() != "Sombra" {
		t.Fatalf("RenameCreature by the master = %v, %v", res, err)
	}
	_, err = rename(a.ana, "Roubado")
	wantCode(t, "RenameCreature by another player", err, connect.CodeNotFound)
	_, err = rename(a.caio, strings.Repeat("a", 41))
	wantCode(t, "RenameCreature(41 characters)", err, connect.CodeInvalidArgument)
	_, err = rename(a.caio, "Duas\nlinhas")
	wantCode(t, "RenameCreature(two lines)", err, connect.CodeInvalidArgument)

	// The hit points are the master's.
	adjust := func(u *user, edit func(*charactersv1.AdjustCreatureHitPointsRequest)) (*charactersv1.AdjustCreatureHitPointsResponse, error) {
		req := &charactersv1.AdjustCreatureHitPointsRequest{CampaignId: a.campaignID, CreatureId: wolf.GetId()}
		edit(req)
		res, err := u.characters.AdjustCreatureHitPoints(t.Context(), connect.NewRequest(req))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	}
	damage := func(n int32) func(*charactersv1.AdjustCreatureHitPointsRequest) {
		return func(r *charactersv1.AdjustCreatureHitPointsRequest) {
			r.Change = &charactersv1.AdjustCreatureHitPointsRequest_Damage{Damage: n}
		}
	}
	_, err = adjust(a.caio, damage(3))
	wantCode(t, "AdjustCreatureHitPoints by a player", err, connect.CodePermissionDenied)
	if res, err := adjust(a.master, damage(3)); err != nil || res.GetCreature().GetHitPointsCurrent() != 8 {
		t.Fatalf("AdjustCreatureHitPoints(damage 3) = %v, %v, want 8 hit points", res, err)
	}
	_, err = adjust(a.master, func(r *charactersv1.AdjustCreatureHitPointsRequest) {
		r.Change = &charactersv1.AdjustCreatureHitPointsRequest_HitPoints{HitPoints: 12}
	})
	wantCode(t, "AdjustCreatureHitPoints(above the maximum)", err, connect.CodeInvalidArgument)
	if res, err := adjust(a.master, func(r *charactersv1.AdjustCreatureHitPointsRequest) {
		r.Change = &charactersv1.AdjustCreatureHitPointsRequest_Heal{Heal: 100}
	}); err != nil || res.GetCreature().GetHitPointsCurrent() != 11 {
		t.Fatalf("AdjustCreatureHitPoints(heal) = %v, %v, want back at the maximum", res, err)
	}
	// At 0 it is out of the fight, so it is dismissed.
	if res, err := adjust(a.master, damage(50)); err != nil || res.GetCreature() != nil {
		t.Fatalf("AdjustCreatureHitPoints(damage 50) = %v, %v, want the creature gone", res, err)
	}
	if got := creatureNames(a.mustCreatures(t, a.caio, a.toren)); len(got) != 1 || got[0] != "Lobo" {
		t.Errorf("Toren's creatures = %v, want the gift with no name only", got)
	}

	// The owner dismisses; a second dismissal changes nothing; another player's
	// creature is not theirs to see.
	other := a.mustCreatures(t, a.caio, a.toren)[0]
	dismiss := func(u *user, id string) error {
		_, err := u.characters.DismissCreature(t.Context(), connect.NewRequest(&charactersv1.DismissCreatureRequest{CampaignId: a.campaignID, CreatureId: id}))
		return err
	}
	wantCode(t, "DismissCreature by another player", dismiss(a.ana, other.GetId()), connect.CodeNotFound)
	if err := dismiss(a.caio, other.GetId()); err != nil {
		t.Fatalf("DismissCreature() error = %v", err)
	}
	if err := dismiss(a.caio, other.GetId()); err != nil {
		t.Errorf("DismissCreature() again error = %v, want no change", err)
	}
	if got := a.mustCreatures(t, a.master, a.toren); len(got) != 0 {
		t.Errorf("creatures = %v, want none", creatureNames(got))
	}
	if n := a.eventCount(t, "creature_dismissed"); n != 2 { // the one at 0 and the owner's dismissal
		t.Errorf("creature_dismissed events = %d, want 2", n)
	}
}

// TestMR037_ConcentrationEndingDismissesTheCastingsCreatures: ending the
// concentration, in any way the table has, dismisses the creatures of the casting,
// with an event for each, and the master's undo brings them back.
func TestMR037_ConcentrationEndingDismissesTheCastingsCreatures(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	e := a.summonersFight(t)
	e = a.toSalvia(t, e)
	if _, err := a.summonWolves(t, a.bia, e, 11); err != nil {
		t.Fatalf("CastSpell() error = %v", err)
	}
	endConcentration := func(u *user, label string) (*playv1.Encounter, error) {
		res, err := u.combat.EndConcentration(t.Context(), connect.NewRequest(&playv1.EndConcentrationRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, label), IdempotencyKey: newKey(),
		}))
		if err != nil {
			return nil, err
		}
		return res.Msg.GetEncounter(), nil
	}

	// Not Toren's player's to end.
	_, err := endConcentration(a.caio, "Sálvia")
	wantCode(t, "EndConcentration by another player", err, connect.CodePermissionDenied)
	// A creature does not concentrate.
	_, err = endConcentration(a.master, "Lobo atroz 1")
	wantCode(t, "EndConcentration on a creature", err, connect.CodeInvalidArgument)

	out, err := endConcentration(a.bia, "Sálvia")
	if err != nil {
		t.Fatalf("EndConcentration() error = %v", err)
	}
	if got := labels(out); len(got) != 4 || slicesContain(got, "Lobo atroz 1") || byLabel(t, out, "Sálvia").GetConcentrationSpell() != "" {
		t.Errorf("combatants = %v, want the wolves gone and Sálvia no longer concentrating", got)
	}
	if got := a.mustCreatures(t, a.bia, a.bri); len(got) != 0 {
		t.Errorf("Sálvia's creatures = %v, want none: the casting depended on her concentration", creatureNames(got))
	}
	if n := a.eventCount(t, "creature_dismissed"); n != 2 {
		t.Errorf("creature_dismissed events = %d, want one for each wolf", n)
	}
	// Ending a concentration that is not there changes nothing.
	if _, err := endConcentration(a.bia, "Sálvia"); err != nil {
		t.Errorf("EndConcentration() again error = %v", err)
	}
	if n := a.eventCount(t, "creature_dismissed"); n != 2 {
		t.Errorf("creature_dismissed events = %d after a second call, want still 2", n)
	}

	// The master's undo gives it back: the concentration, the creatures and their
	// initiative.
	last := a.log(t, a.master, e).GetUndoableEventId()
	if err := a.undo(t, a.master, e, last); err != nil {
		t.Fatalf("UndoLastAction() error = %v", err)
	}
	back := a.get(t, a.bia)
	w1 := byLabel(t, back, "Lobo atroz 1")
	if w1.GetInitiative() != 13 || byLabel(t, back, "Sálvia").GetConcentrationSpell() != conjureAnimals || len(a.mustCreatures(t, a.bia, a.bri)) != 2 {
		t.Errorf("after the undo: wolf = %v, creatures = %d, want the wolves back with their initiative and the concentration", w1, len(a.mustCreatures(t, a.bia, a.bri)))
	}

	// The same through the condition call, which also ends a concentration.
	if _, err := a.conditions(t, a.bia, e, "Sálvia", nil, false, true); err != nil {
		t.Fatalf("SetCombatantConditions(end_concentration) error = %v", err)
	}
	if got := a.mustCreatures(t, a.bia, a.bri); len(got) != 0 {
		t.Errorf("creatures = %v, want none after SetCombatantConditions(end_concentration) too", creatureNames(got))
	}
	if err := a.undo(t, a.master, e, a.log(t, a.master, e).GetUndoableEventId()); err != nil {
		t.Fatalf("UndoLastAction() error = %v", err)
	}

	// Casting another concentration spell ends it too: the master casts Conjurar
	// Animais again (the master has the last word on the economy), and the first
	// casting's wolves go, the new ones come.
	first := byLabel(t, a.get(t, a.master), "Lobo atroz 1").GetCreatureId()
	if _, err := a.summonWolves(t, a.master, e, 4); err != nil {
		t.Fatalf("CastSpell() again error = %v", err)
	}
	list := a.mustCreatures(t, a.bia, a.bri)
	if len(list) != 2 || list[0].GetId() == first || list[1].GetId() == first {
		t.Errorf("creatures = %v, want only the second casting's wolves", creatureNames(list))
	}
	if got := a.get(t, a.master).GetCombatants(); len(got) != 6 {
		t.Errorf("combatants = %v, want the four of the fight and the second casting's two wolves", labels(a.get(t, a.master)))
	}

	// Dismissing a creature takes it out of the combat.
	if _, err := a.caio.characters.DismissCreature(t.Context(), connect.NewRequest(&charactersv1.DismissCreatureRequest{CampaignId: a.campaignID, CreatureId: list[0].GetId()})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("DismissCreature by another player error = %v, want not_found", err)
	}
	if _, err := a.bia.characters.DismissCreature(t.Context(), connect.NewRequest(&charactersv1.DismissCreatureRequest{CampaignId: a.campaignID, CreatureId: list[0].GetId()})); err != nil {
		t.Fatalf("DismissCreature() error = %v", err)
	}
	if got := labels(a.get(t, a.master)); len(got) != 5 {
		t.Errorf("combatants = %v, want one wolf less after the owner dismissed it", got)
	}
}

func slicesContain(ss []string, s string) bool { return contains(ss, s) }

// TestMR037_UndoOfAConjuringTakesTheCreaturesAway: undoing the cast gives the slot
// and the concentration back and removes the creatures.
func TestMR037_UndoOfAConjuringTakesTheCreaturesAway(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	e := a.summonersFight(t)
	e = a.toSalvia(t, e)
	if _, err := a.summonWolves(t, a.bia, e, 11); err != nil {
		t.Fatalf("CastSpell() error = %v", err)
	}
	if err := a.undo(t, a.master, e, a.log(t, a.master, e).GetUndoableEventId()); err != nil {
		t.Fatalf("UndoLastAction() error = %v", err)
	}
	if got := labels(a.get(t, a.master)); len(got) != 4 {
		t.Errorf("combatants = %v, want the wolves gone", got)
	}
	if got := a.mustCreatures(t, a.bia, a.bri); len(got) != 0 {
		t.Errorf("creatures = %v, want none", creatureNames(got))
	}
	if usedSlots(a.vitals(t, a.bri), 3) != 0 || byLabel(t, a.get(t, a.master), "Sálvia").GetConcentrationSpell() != "" {
		t.Error("the undo must give the slot and the concentration back")
	}
	// And the cast can be made again.
	if _, err := a.summonWolves(t, a.bia, a.get(t, a.master), 11); err != nil {
		t.Fatalf("CastSpell() after the undo error = %v", err)
	}
}

// TestMR037_ACreatureAtZeroLeavesAndTheHitPointsGoBack: a creature's damage lands
// at once, as an NPC's; at 0 it is defeated and dismissed, an undo or a heal
// brings it back, and its hit points are written back when the combat ends.
func TestMR037_ACreatureAtZeroLeavesAndTheHitPointsGoBack(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	e := a.summonersFight(t)
	e = a.toSalvia(t, e)
	if _, err := a.summonWolves(t, a.bia, e, 11); err != nil {
		t.Fatalf("CastSpell() error = %v", err)
	}

	// The goblin bites a wolf: the damage lands at once, with no waiting for the
	// master (a creature is not a character: RN-02 is for characters' hit points).
	a.h.roller.queue(15)
	hit := a.mustAttackAsReaction(t, e, "Goblin", sword, "Lobo atroz 1")
	if hit.GetRoll().GetOutcome() == playv1.AttackOutcome_ATTACK_OUTCOME_MISS || hit.GetPendingDamage() == nil {
		t.Fatalf("attack = %v, want a hit (15 + 4 against a wolf's armor class of 14)", hit)
	}
	dmg := a.mustDamage(t, a.master, e, hit.GetPendingDamage().GetId(), typedDamage(4)) // 1d6 + 2: 6 damage
	if dmg.GetPendingDamage().GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED {
		t.Fatalf("damage = %v, want applied at once", dmg.GetPendingDamage())
	}
	if cur, _, _ := a.hp(t, "Lobo atroz 1"); cur != 31 {
		t.Errorf("the wolf has %d hit points, want 31", cur)
	}
	// The undo of the damage puts the hit points back.
	if err := a.undo(t, a.master, e, a.log(t, a.master, e).GetUndoableEventId()); err != nil {
		t.Fatalf("UndoLastAction(damage) error = %v", err)
	}
	if cur, _, _ := a.hp(t, "Lobo atroz 1"); cur != 37 {
		t.Errorf("after the undo the wolf has %d hit points, want 37", cur)
	}

	// At 0 it is defeated and leaves its owner's list; an undo brings it back.
	if _, err := a.adjustHP(t, e, "Lobo atroz 1", damageHP(500)); err != nil {
		t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
	}
	if _, _, defeated := a.hp(t, "Lobo atroz 1"); !defeated {
		t.Error("the wolf at 0 hit points is not defeated")
	}
	if got := creatureNames(a.mustCreatures(t, a.bia, a.bri)); len(got) != 1 || got[0] != "Lobo atroz 2" {
		t.Errorf("Sálvia's creatures = %v, want only the wolf still standing", got)
	}
	if err := a.undo(t, a.master, e, a.log(t, a.master, e).GetUndoableEventId()); err != nil {
		t.Fatalf("UndoLastAction(correction) error = %v", err)
	}
	if got := a.mustCreatures(t, a.bia, a.bri); len(got) != 2 || got[0].GetHitPointsCurrent() != 37 {
		t.Errorf("Sálvia's creatures = %v, want both wolves again after the undo, at full hit points", creatureNames(got))
	}
	// A heal brings it back too.
	if _, err := a.adjustHP(t, e, "Lobo atroz 2", damageHP(500)); err != nil {
		t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
	}
	if len(a.mustCreatures(t, a.bia, a.bri)) != 1 {
		t.Fatal("the second wolf at 0 hit points must leave the list")
	}
	if _, err := a.adjustHP(t, e, "Lobo atroz 2", healHP(5)); err != nil {
		t.Fatalf("AdjustCombatantHitPoints(heal) error = %v", err)
	}
	if got := a.mustCreatures(t, a.bia, a.bri); len(got) != 2 || got[1].GetHitPointsCurrent() != 5 {
		t.Errorf("Sálvia's creatures = %v, want the healed wolf back with 5 hit points", creatureNames(got))
	}

	// The hit points are written back when the combat ends, and the hit points of a
	// creature are the master's outside a combat only.
	if _, err := a.adjustHP(t, e, "Lobo atroz 1", damageHP(20)); err != nil {
		t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
	}
	_, err := a.master.characters.AdjustCreatureHitPoints(t.Context(), connect.NewRequest(&charactersv1.AdjustCreatureHitPointsRequest{
		CampaignId: a.campaignID, CreatureId: a.mustCreatures(t, a.master, a.bri)[0].GetId(),
		Change: &charactersv1.AdjustCreatureHitPointsRequest_Damage{Damage: 1},
	}))
	if err == nil {
		t.Fatal("AdjustCreatureHitPoints in a combat succeeded, want failed_precondition")
	}
	if detail := charactersBlocked(t, err); detail != charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_CREATURE_IN_COMBAT {
		t.Errorf("reason = %v, want CREATURE_IN_COMBAT", detail)
	}
	if _, err := a.master.combat.EndEncounter(t.Context(), connect.NewRequest(&playv1.EndEncounterRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey()})); err != nil {
		t.Fatalf("EndEncounter() error = %v", err)
	}
	got := a.mustCreatures(t, a.bia, a.bri)
	if len(got) != 2 || got[0].GetHitPointsCurrent() != 17 || got[1].GetHitPointsCurrent() != 5 {
		t.Errorf("after the combat the wolves have %d and %d hit points, want 17 and 5", got[0].GetHitPointsCurrent(), got[1].GetHitPointsCurrent())
	}
}

// charactersBlocked reads the CharacterBlocked reason of a failed_precondition.
func charactersBlocked(t *testing.T, err error) charactersv1.CharacterBlockedReason {
	t.Helper()
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("error = %v, want failed_precondition", err)
	}
	if ce, ok := errors.AsType[*connect.Error](err); ok {
		for _, d := range ce.Details() {
			if v, derr := d.Value(); derr == nil {
				if b, ok := v.(*charactersv1.CharacterBlocked); ok {
					return b.GetReason()
				}
			}
		}
	}
	t.Fatalf("error %v has no CharacterBlocked detail", err)
	return 0
}

// TestRN20_CreatureHitPointsOnlyToOwnerAndMaster: another player's responses and
// stream, read as the app's JSON, never carry a creature's hit points; and `mine`
// stays false for creatures.
func TestRN20_CreatureHitPointsOnlyToOwnerAndMaster(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	e := a.summonersFight(t)
	e = a.toSalvia(t, e)
	other := a.ana.watch(t, a.campaignID)
	other.ready(t)
	if _, err := a.summonWolves(t, a.bia, e, 11); err != nil {
		t.Fatalf("CastSpell() error = %v", err)
	}
	a.h.roller.queue(15)
	hit := a.mustAttackAsReaction(t, e, "Goblin", sword, "Lobo atroz 1")
	a.mustDamage(t, a.master, e, hit.GetPendingDamage().GetId(), typedDamage(6)) // 1d6 + 2: 8 damage

	hasHP := func(c *playv1.Combatant) bool {
		return c.HitPointsCurrent != nil || c.HitPointsMax != nil || c.HitPointsTemporary != nil
	}
	for name, u := range map[string]*user{"another player": a.ana, "Toren's player": a.caio} {
		enc := a.get(t, u)
		for _, c := range enc.GetCombatants() {
			if c.GetKind() != playv1.CombatantKind_COMBATANT_KIND_CREATURE {
				continue
			}
			if hasHP(c) || c.GetMine() || c.GetControlledByMe() || c.GetCharacterId() != "" || c.GetCreatureId() != "" || c.GetInitiativeBonus() != 0 {
				t.Errorf("%s reads %s as %v, want its state word only and no owner's numbers", name, c.GetLabel(), c)
			}
			if c.GetOwnerCharacterId() != a.bri.GetId() || c.GetMonsterKey() != direWolf {
				t.Errorf("%s reads %s as %v, want the owner and the kind (the party knows them)", name, c.GetLabel(), c)
			}
		}
		if st := byLabel(t, enc, "Lobo atroz 1").GetState(); st != playv1.CombatantState_COMBATANT_STATE_HURT {
			t.Errorf("%s reads a state of %v for the bitten wolf, want hurt", name, st)
		}
		if st := byLabel(t, enc, "Lobo atroz 2").GetState(); st != playv1.CombatantState_COMBATANT_STATE_UNHURT {
			t.Errorf("%s reads a state of %v for the other wolf, want unhurt", name, st)
		}
		if js := asJSON(t, enc); strings.Contains(js, `"hitPointsCurrent"`) || strings.Contains(js, `"hitPointsMax"`) {
			t.Errorf("%s: the combat as JSON has hit points:\n%s", name, js)
		}
		log := a.log(t, u, e)
		if js := asJSON(t, log); strings.Contains(js, "hitPointsAfter") {
			t.Errorf("%s: the combat log has hit points:\n%s", name, js)
		}
		_, err := u.combat.GetTurnOptions(t.Context(), connect.NewRequest(&playv1.GetTurnOptionsRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: byLabel(t, enc, "Lobo atroz 1").GetId()}))
		wantCode(t, name+": GetTurnOptions of the wolf", err, connect.CodePermissionDenied)
		_, err = a.creatures(t, u, a.bri)
		wantCode(t, name+": ListCharacterCreatures", err, connect.CodeNotFound)
	}
	// The owner and the master do read the numbers, and only Sálvia is "mine".
	for name, u := range map[string]*user{"the owner": a.bia, "the master": a.master} {
		enc := a.get(t, u)
		w := byLabel(t, enc, "Lobo atroz 1")
		if w.GetHitPointsCurrent() != 29 || w.GetHitPointsMax() != 37 || w.GetMine() {
			t.Errorf("%s reads the wolf as %v, want 29/37 hit points and not mine", name, w)
		}
	}
	if enc := a.get(t, a.bia); !byLabel(t, enc, "Sálvia").GetMine() || !byLabel(t, enc, "Lobo atroz 2").GetControlledByMe() {
		t.Error("the druid's player must read Sálvia as mine and the wolves as controlled by them")
	}

	// The stream: what the other player's stream got, since the cast, has no
	// creature's numbers either.
	var sawEncounter bool
	for range 20 {
		ev := other.nextChange(t)
		js := protojson.Format(ev)
		if strings.Contains(js, "hitPoints") || strings.Contains(js, "monster") {
			t.Fatalf("the other player's stream carries a creature's numbers: %s", js)
		}
		if ev.GetEncounterChanged() != nil {
			sawEncounter = true
		}
		if ev.GetCombatLogChanged() != nil && sawEncounter {
			break
		}
	}
	if !sawEncounter {
		t.Error("the other player's stream got no encounter_changed")
	}
}

// TestRN20_TableCreatureHitPointsOnlyToOwnerAndMaster: the same for a table creature (issue #578), a skeleton
// of the master's with 30 hit points and armor class 15 given to Toren. Its owner's player and the master
// read its hit points; the master alone reads its armor class; another player's responses and stream carry
// neither, and the key of a table creature has no "monster" in it.
func TestRN20_TableCreatureHitPointsOnlyToOwnerAndMaster(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	if _, err := a.master.table.CreateTableEntry(t.Context(), connect.NewRequest(&rulesv1.CreateTableEntryRequest{
		CampaignId: a.campaignID, Body: &rulesv1.CreateTableEntryRequest_TableCreature{TableCreature: &rulesv1.TableCreature{
			NamePt: "Guarda Ossuda", BaseKey: "monster:skeleton", HitPoints: 30, ArmorClass: 15,
		}},
	})); err != nil {
		t.Fatalf("CreateTableEntry(creature) error = %v", err)
	}
	if strings.Contains(tableSkeletonKey, "monster") {
		t.Fatalf("the table creature's key %q has \"monster\" in it, which the stream check below relies on", tableSkeletonKey)
	}
	given := a.give(t, a.toren, tableSkeletonKey, "Guarda")
	if given.GetHitPointsMax() != 30 || given.GetArmorClass() != 15 {
		t.Fatalf("the gift = hp %d, armor class %d; want 30 and 15", given.GetHitPointsMax(), given.GetArmorClass())
	}
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId()}},
		npcRolls: []int{2},
		players:  map[string]int32{"Pensantus": 20, "Sálvia": 15, "Toren": 12, "Guarda": 10},
		reveal:   []string{"Goblin"},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Pensantus": {10, 3}, "Sálvia": {6, 5}, "Guarda": {5, 3}},
	})
	other := a.ana.watch(t, a.campaignID)
	other.ready(t)
	a.h.roller.queue(15)
	hit := a.mustAttackAsReaction(t, e, "Goblin", sword, "Guarda")
	a.mustDamage(t, a.master, e, hit.GetPendingDamage().GetId(), typedDamage(6)) // 1d6 + 2: 8 damage, 30 - 8 = 22

	hasNumbers := func(c *playv1.Combatant) bool {
		return c.HitPointsCurrent != nil || c.HitPointsMax != nil || c.HitPointsTemporary != nil || c.ArmorClass != nil
	}
	for name, u := range map[string]*user{"another player": a.ana, "a third player": a.bia} {
		enc := a.get(t, u)
		guard := byLabel(t, enc, "Guarda")
		if guard.GetKind() != playv1.CombatantKind_COMBATANT_KIND_CREATURE {
			t.Fatalf("%s reads Guarda as %v, want a creature", name, guard.GetKind())
		}
		if hasNumbers(guard) || guard.GetMine() || guard.GetControlledByMe() || guard.GetCharacterId() != "" || guard.GetCreatureId() != "" || guard.GetInitiativeBonus() != 0 {
			t.Errorf("%s reads Guarda as %v, want its state word only and no owner's numbers", name, guard)
		}
		if guard.GetOwnerCharacterId() != a.toren.GetId() || guard.GetMonsterKey() != tableSkeletonKey {
			t.Errorf("%s reads Guarda as %v, want the owner and the kind (the party knows them)", name, guard)
		}
		if guard.GetState() != playv1.CombatantState_COMBATANT_STATE_HURT {
			t.Errorf("%s reads a state of %v for the hurt skeleton, want hurt", name, guard.GetState())
		}
		if js := asJSON(t, enc); strings.Contains(js, `"hitPointsCurrent"`) || strings.Contains(js, `"hitPointsMax"`) {
			t.Errorf("%s: the combat as JSON has hit points:\n%s", name, js)
		}
		if js := asJSON(t, a.log(t, u, e)); strings.Contains(js, "hitPointsAfter") {
			t.Errorf("%s: the combat log has hit points:\n%s", name, js)
		}
		_, err := u.combat.GetTurnOptions(t.Context(), connect.NewRequest(&playv1.GetTurnOptionsRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: guard.GetId()}))
		wantCode(t, name+": GetTurnOptions of the skeleton", err, connect.CodePermissionDenied)
		_, err = a.creatures(t, u, a.toren)
		wantCode(t, name+": ListCharacterCreatures", err, connect.CodeNotFound)
	}
	// The owner's player and the master read the hit points; only the master reads the armor class.
	for name, u := range map[string]*user{"the owner": a.caio, "the master": a.master} {
		guard := byLabel(t, a.get(t, u), "Guarda")
		if guard.GetHitPointsCurrent() != 22 || guard.GetHitPointsMax() != 30 || guard.GetMine() {
			t.Errorf("%s reads Guarda as %v, want 22/30 hit points and not mine", name, guard)
		}
		if (u == a.master) != (guard.ArmorClass != nil) || (u == a.master && guard.GetArmorClass() != 15) {
			t.Errorf("%s reads armor class %v for Guarda, want 15 for the master alone", name, guard.ArmorClass)
		}
	}
	if enc := a.get(t, a.caio); !byLabel(t, enc, "Toren").GetMine() || !byLabel(t, enc, "Guarda").GetControlledByMe() {
		t.Error("Toren's player must read Toren as mine and Guarda as controlled by them")
	}
	mine, err := a.creatures(t, a.caio, a.toren)
	if err != nil || len(mine) != 1 || mine[0].GetHitPointsMax() != 30 || mine[0].GetArmorClass() != 15 {
		t.Errorf("the owner's ListCharacterCreatures = %v, %v; want 30 hit points at most and armor class 15", mine, err)
	}

	// The stream: what the other player's stream got, since the hit, has no numbers and no "monster".
	var sawEncounter bool
	for range 20 {
		ev := other.nextChange(t)
		js := protojson.Format(ev)
		if strings.Contains(js, "hitPoints") || strings.Contains(js, "armorClass") || strings.Contains(js, "monster") {
			t.Fatalf("the other player's stream carries a creature's numbers: %s", js)
		}
		if ev.GetEncounterChanged() != nil {
			sawEncounter = true
		}
		if ev.GetCombatLogChanged() != nil && sawEncounter {
			break
		}
	}
	if !sawEncounter {
		t.Error("the other player's stream got no encounter_changed")
	}
}

// submitFor rolls the initiative of a combatant as u, from a physical die.
func (a *armed) submitFor(t *testing.T, u *user, e *playv1.Encounter, label string, face int32) (*playv1.Encounter, error) {
	t.Helper()
	res, err := u.combat.SubmitInitiative(t.Context(), connect.NewRequest(&playv1.SubmitInitiativeRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: byLabel(t, a.get(t, a.master), label).GetId(), IdempotencyKey: newKey(),
		Roll: &playv1.SubmitInitiativeRequest_D20Face{D20Face: face},
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetEncounter(), nil
}

// TestMR037_ExistingCreaturesJoinACombatAndAGroupSharesOneRoll: the creatures a
// character already has join a combat with it, each without an initiative until
// its owner's player rolls: one roll for the creatures of one casting (Animar os
// Mortos here, ours as Conjurar Animais is the SRD's), and the familiar rolls its
// own. The master can take one out.
func TestMR037_ExistingCreaturesJoinACombatAndAGroupSharesOneRoll(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	a.mustCastSummon(t, a.ana, a.pens, findFamiliar, nil, 0, []string{"monster:owl"}, "Nanquim")
	a.mustCastSummon(t, a.ana, a.pens, animateDead, slotOfLevel(4), 0, []string{"monster:skeleton", "monster:skeleton", "monster:zombie"})
	a.give(t, a.toren, "monster:wolf", "Presa") // Toren's, but he is not in this fight's party? he is: the party joins whole

	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId()}},
		npcRolls: []int{2},
		players:  map[string]int32{"Pensantus": 20, "Sálvia": 15, "Toren": 12},
		reveal:   []string{"Goblin"},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Pensantus": {10, 3}, "Sálvia": {6, 5}},
		setup:    true,
	})
	want := []string{"Nanquim", "Esqueleto 1", "Esqueleto 2", "Zumbi", "Presa"}
	for _, label := range want {
		c := byLabel(t, e, label)
		if c.GetKind() != playv1.CombatantKind_COMBATANT_KIND_CREATURE || c.Initiative != nil {
			t.Errorf("%s = %v, want a creature with no initiative yet", label, c)
		}
	}
	// Combat cannot begin without their rolls, and the error names them.
	_, err := a.master.combat.BeginCombat(t.Context(), connect.NewRequest(&playv1.BeginCombatRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey()}))
	blocked := wantBlockedBy(t, "BeginCombat", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_INITIATIVE_MISSING)
	if len(blocked.GetCombatantIds()) != 5 {
		t.Errorf("missing = %v, want the five creatures", blocked.GetCombatantIds())
	}

	// Only the owner's player (or the master) rolls for a creature.
	_, err = a.submitFor(t, a.caio, e, "Nanquim", 10)
	wantCode(t, "SubmitInitiative for another player's creature", err, connect.CodePermissionDenied)
	if _, err := a.submitFor(t, a.ana, e, "Nanquim", 10); err != nil {
		t.Fatalf("SubmitInitiative(Nanquim) error = %v", err)
	}
	// One roll for the casting's three: the owl is not in it (a familiar rolls its own).
	// The bonus is the first creature's of the casting (the skeleton's +2), whichever
	// member the player rolls for (the zombie has -2).
	got, err := a.submitFor(t, a.ana, e, "Zumbi", 8)
	if err != nil {
		t.Fatalf("SubmitInitiative(Zumbi) error = %v", err)
	}
	if total := byLabel(t, got, "Esqueleto 1").GetInitiative(); total != 10 {
		t.Errorf("the group's total = %d, want 8 + 2", total)
	}
	for _, label := range []string{"Esqueleto 1", "Esqueleto 2", "Zumbi"} {
		if c := byLabel(t, got, label); c.GetInitiativeFace() != 8 || c.Initiative == nil || c.GetInitiative() != byLabel(t, got, "Esqueleto 1").GetInitiative() {
			t.Errorf("%s = %v, want the group's one roll (face 8, one total)", label, c)
		}
	}
	if byLabel(t, got, "Nanquim").GetInitiativeFace() != 10 {
		t.Error("the familiar kept its own roll")
	}
	_, err = a.submitFor(t, a.ana, e, "Zumbi", 15)
	wantBlockedBy(t, "SubmitInitiative(again)", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_INITIATIVE_ALREADY_SET)

	// The master takes a creature out of the fight, and it stays on its owner's list.
	if _, err := a.master.combat.RemoveCombatant(t.Context(), connect.NewRequest(&playv1.RemoveCombatantRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: byLabel(t, got, "Presa").GetId(), IdempotencyKey: newKey(),
	})); err != nil {
		t.Fatalf("RemoveCombatant(Presa) error = %v", err)
	}
	if n := len(a.mustCreatures(t, a.caio, a.toren)); n != 1 {
		t.Errorf("Toren's creatures = %d, want the gift still his", n)
	}
	if _, err := a.master.combat.BeginCombat(t.Context(), connect.NewRequest(&playv1.BeginCombatRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey()})); err != nil {
		t.Fatalf("BeginCombat() error = %v", err)
	}
	// A player's character that leaves a combat in SETUP takes its creatures with it.
}

// TestMR037_TheKindAudit: what each kind of combatant does where a rule could
// ask only "player or NPC": a creature takes an attack and a damage like an NPC
// (its hit points are on the combatant), never makes a death save, never moves
// the owner's vitals, is not a character's token at the end, and the spell that
// brought it says what it may do: a familiar never takes the Attack action.
func TestMR037_TheKindAudit(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	a.mustCastSummon(t, a.ana, a.pens, findFamiliar, nil, 0, []string{"monster:owl"}, "Nanquim")
	a.mustCastSummon(t, a.ana, a.pens, animateDead, slotOfLevel(3), 0, []string{"monster:skeleton"})
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId()}},
		npcRolls: []int{2},
		players:  map[string]int32{"Pensantus": 20, "Sálvia": 15, "Toren": 12, "Nanquim": 10, "Esqueleto": 8},
		reveal:   []string{"Goblin"},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Pensantus": {10, 3}, "Sálvia": {6, 5}, "Nanquim": {11, 3}, "Esqueleto": {5, 3}},
	})
	pensHP := a.vitals(t, a.pens).GetHitPointsCurrent()

	// The familiar's options: it moves and takes the other standard actions, never
	// the Attack action, and it has no attacks.
	opts := a.mustOptions(t, a.ana, e, "Nanquim")
	if len(opts.GetOptions().GetAttacks()) != 0 || standardOption(opts, "standard:attack") != nil || standardOption(opts, "standard:dodge") == nil ||
		standardOption(opts, "standard:cast-a-spell") != nil {
		t.Errorf("the familiar's options = %v, want no attacks and no Attack or Cast a Spell action", opts.GetOptions())
	}
	if opts.GetOptions().GetEconomy().GetMovement().GetSpeedFt() != 60 { // an owl flies 60 ft
		t.Errorf("the familiar's speed = %d ft, want 60 (its best)", opts.GetOptions().GetEconomy().GetMovement().GetSpeedFt())
	}
	e = a.passTo(t, e, "Nanquim")
	_, err := a.attack(t, a.ana, e, "Nanquim", "monster:owl#talons", "Goblin", inAppRoll)
	wantBlockedBy(t, "RollAttack by the familiar", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_CREATURE_CANNOT_ATTACK)
	_, err = a.attack(t, a.master, e, "Nanquim", "monster:owl#talons", "Goblin", inAppRoll)
	wantBlockedBy(t, "RollAttack by the familiar as the master", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_CREATURE_CANNOT_ATTACK)
	if out, err := a.action(t, a.ana, e, "Nanquim", "standard:dodge"); err != nil {
		t.Errorf("TakeAction(Esquivar) by the familiar error = %v", err)
	} else if !byLabel(t, out, "Nanquim").GetActionUsed() {
		t.Error("the familiar's Dodge must spend its action")
	}
	// A creature makes no death save, and it is never confirmed dead.
	_, err = a.deathSave(t, a.ana, e, "Nanquim", func(r *playv1.RollDeathSaveRequest) {
		r.Roll = &playv1.RollDeathSaveRequest_RollInApp{RollInApp: true}
	})
	wantCode(t, "RollDeathSave by a creature", err, connect.CodeInvalidArgument)
	_, err = a.master.combat.ConfirmDeath(t.Context(), connect.NewRequest(&playv1.ConfirmDeathRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Nanquim"), IdempotencyKey: newKey()}))
	wantCode(t, "ConfirmDeath of a creature", err, connect.CodeInvalidArgument)

	// The undead's turn: it attacks with its shortsword, the owner's player rolls
	// and the damage lands on the goblin at once; another player may not act for it.
	e = a.passTo(t, e, "Esqueleto")
	_, err = a.attack(t, a.caio, e, "Esqueleto", "monster:skeleton#shortsword", "Goblin", d20(15))
	wantCode(t, "RollAttack by another player for a creature", err, connect.CodePermissionDenied)
	hit, err := a.attack(t, a.ana, e, "Esqueleto", "monster:skeleton#shortsword", "Goblin", d20(15))
	if err != nil {
		t.Fatalf("RollAttack by the skeleton error = %v", err)
	}
	if hit.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_HIT || hit.GetRoll().GetTargetArmorClass() != 0 {
		t.Fatalf("attack = %v, want a hit, with no armor class to the owner's player", hit)
	}
	if hit.GetPendingDamage() == nil || hit.GetPendingDamage().GetAttackerId() != a.id(t, "Esqueleto") {
		t.Fatalf("pending damage = %v, want the skeleton's", hit.GetPendingDamage())
	}
	a.mustDamage(t, a.ana, e, hit.GetPendingDamage().GetId(), typedDamage(3)) // 1d6 + 2: 5 damage
	if cur, _, _ := a.hp(t, "Goblin"); cur != 2 {
		t.Errorf("the goblin has %d hit points, want 7 - 5 = 2", cur)
	}
	if !byLabel(t, a.get(t, a.ana), "Esqueleto").GetActionUsed() {
		t.Error("the skeleton's attack must spend its action")
	}
	if got := a.vitals(t, a.pens).GetHitPointsCurrent(); got != pensHP {
		t.Errorf("Pensantus has %d hit points, want %d: a creature's rolls never touch its owner's vitals", got, pensHP)
	}
	// The goblin hits the skeleton: the damage lands at once (a creature's hit
	// points are on its combatant) and Pensantus's vitals stay as they were.
	a.h.roller.queue(18)
	back := a.mustAttackAsReaction(t, e, "Goblin", sword, "Esqueleto")
	a.mustDamage(t, a.master, e, back.GetPendingDamage().GetId(), typedDamage(2)) // 1d6 + 2: 4 damage
	if cur, _, _ := a.hp(t, "Esqueleto"); cur != 9 {
		t.Errorf("the skeleton has %d hit points, want 13 - 4 = 9", cur)
	}
	if got := a.vitals(t, a.pens).GetHitPointsCurrent(); got != pensHP {
		t.Errorf("Pensantus has %d hit points, want %d: a hit on his skeleton is not a hit on him", got, pensHP)
	}
	// The combat's end puts the characters' tokens where they stood; a creature
	// has no token of its own (the maps slice gives it one), so nothing of it is
	// left on the map.
	if _, err := a.master.combat.EndEncounter(t.Context(), connect.NewRequest(&playv1.EndEncounterRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey()})); err != nil {
		t.Fatalf("EndEncounter() error = %v", err)
	}
	var tokens int
	if err := a.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM map_tokens WHERE map_id = $1`, a.mapID).Scan(&tokens); err != nil {
		t.Fatalf("count the tokens: %v", err)
	}
	if tokens != 3 {
		t.Errorf("tokens = %d, want the three characters' (a creature stands for none)", tokens)
	}
	for _, c := range a.mustCreatures(t, a.ana, a.pens) {
		if c.GetName() == "Esqueleto" && c.GetHitPointsCurrent() != 9 {
			t.Errorf("the skeleton has %d hit points on its owner's list, want 9", c.GetHitPointsCurrent())
		}
	}
}

// TestMR037_TheFamiliarHelps (SRD 5.1, Find Familiar and Help): a familiar can't attack, but
// it can take other actions as normal, Help among them. Nanquim helps Toren against the goblin
// next to it: Toren's attack has advantage, "Ajuda de Nanquim". A Help for Pensantus is his
// alone: his skeleton, which carries his character, attacks the same goblin without it.
func TestMR037_TheFamiliarHelps(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	a.mustCastSummon(t, a.ana, a.pens, findFamiliar, nil, 0, []string{"monster:owl"}, "Nanquim")
	a.mustCastSummon(t, a.ana, a.pens, animateDead, slotOfLevel(3), 0, []string{"monster:skeleton"})
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId()}},
		npcRolls: []int{2},
		players:  map[string]int32{"Nanquim": 20, "Sálvia": 18, "Pensantus": 15, "Toren": 12, "Esqueleto": 8},
		reveal:   []string{"Goblin"},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Nanquim": {5, 3}, "Sálvia": {5, 4}, "Pensantus": {10, 3}, "Esqueleto": {4, 4}},
	})
	help := func(u *user, helper, ally string) (*playv1.HelpResponse, error) {
		res, err := u.contests.Help(t.Context(), connect.NewRequest(&playv1.HelpRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey(), CombatantId: a.id(t, helper),
			Kind: playv1.HelpKind_HELP_KIND_ATTACK, AllyId: a.id(t, ally), TargetId: a.id(t, "Goblin"),
		}))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	}

	e = a.passTo(t, e, "Nanquim")
	given, err := help(a.ana, "Nanquim", "Toren")
	if err != nil {
		t.Fatalf("Help by the familiar error = %v", err)
	}
	if h := given.GetHelp(); h.GetHelperId() != a.id(t, "Nanquim") || h.GetAllyId() != a.id(t, "Toren") {
		t.Fatalf("help = %v, want Nanquim's Help to Toren", h)
	}
	if !byLabel(t, given.GetEncounter(), "Nanquim").GetActionUsed() || byLabel(t, given.GetEncounter(), "Pensantus").GetActionUsed() {
		t.Error("the familiar's Help must spend its own action, not Pensantus's")
	}

	e = a.passTo(t, e, "Sálvia")
	if _, err := help(a.bia, "Sálvia", "Pensantus"); err != nil {
		t.Fatalf("Help by Sálvia error = %v", err)
	}

	e = a.passTo(t, e, "Toren")
	tg := targetOf(a.mustOptions(t, a.caio, e, "Toren"), battleaxe, "Goblin")
	if tg == nil || tg.GetRollMode() != playv1.RollMode_ROLL_MODE_ADVANTAGE || len(tg.GetSources()) != 1 ||
		tg.GetSources()[0].GetKind() != playv1.AdvantageSourceKind_ADVANTAGE_SOURCE_KIND_HELP || !strings.Contains(tg.GetSources()[0].GetTextPt(), "Nanquim") {
		t.Fatalf("the goblin as Toren's target = %v, want advantage from \"Ajuda de Nanquim\"", tg)
	}

	e = a.passTo(t, e, "Esqueleto")
	if tg := targetOf(a.mustOptions(t, a.ana, e, "Esqueleto"), "monster:skeleton#shortsword", "Goblin"); tg == nil || tg.GetRollMode() != playv1.RollMode_ROLL_MODE_NORMAL {
		t.Errorf("the goblin as the skeleton's target = %v, want a normal roll: Sálvia helped Pensantus, not his skeleton", tg)
	}
}

// TestMR037_TheChainFamiliarAttacksOnlyWithItsReaction: the familiar of a warlock
// with the Pact of the Chain may be an imp (it is not a wizard's form), and it
// attacks only with its reaction: its attacks are disabled as an action, the
// Attack action is not offered, and an attack on its own turn is refused; an
// opportunity attack with the reaction works once.
func TestMR037_TheChainFamiliarAttacksOnlyWithItsReaction(t *testing.T) {
	t.Parallel()
	a := newArmedWith(t, func(a *armed) {
		scores := &rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 16}
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 2, scores, []string{battleaxe}, nil)
		a.pens = a.ana.hero(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 1, scores, nil, []string{fireBolt})
		sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
			BaseScores: scores, RaceKey: "race:human", Classes: []*charactersv1.ClassLevel{{ClassKey: "class:warlock", Level: 3}},
			FeatureChoiceKeys: []string{"feature:pact-of-the-chain"},
		}}}
		res, err := a.bia.characters.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
			CampaignId: a.campaignID, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Brisa", Sheet: sheet,
		}))
		if err != nil {
			t.Fatalf("CreateCharacter(Brisa) error = %v", err)
		}
		a.bri = res.Msg.GetCharacter()
	})
	// The pact gives the ritual with no spell on the sheet; a wizard's form still works.
	a.mustCastSummon(t, a.bia, a.bri, findFamiliar, nil, 0, []string{"monster:imp"}, "Fagulha")
	list := a.mustCreatures(t, a.bia, a.bri)
	if len(list) != 1 || list[0].GetAttack() != 2 || list[0].GetMonsterNamePt() == "" {
		t.Fatalf("creatures = %v, want the imp, which attacks with its reaction", list)
	}

	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId()}},
		npcRolls: []int{3},
		players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 5, "Fagulha": 2},
		reveal:   []string{"Goblin"},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Pensantus": {10, 3}, "Brisa": {8, 8}, "Fagulha": {5, 3}},
	})
	// The imp's attack is the first the sheet lists; off its turn the options say so.
	key := a.mustOptions(t, a.bia, e, "Fagulha").GetOptions().GetAttacks()[0].GetAttack().GetKey()

	// Off its turn, the reaction attack works, and only once.
	hit, err := a.attackAs(t, a.bia, e, "Fagulha", key, "Goblin", d20(15), true)
	if err != nil {
		t.Fatalf("RollAttack(as a reaction) error = %v", err)
	}
	if hit.GetRoll().GetOutcome() == playv1.AttackOutcome_ATTACK_OUTCOME_MISS {
		t.Errorf("attack = %v, want a hit", hit.GetRoll())
	}
	if !byLabel(t, a.get(t, a.bia), "Fagulha").GetReactionUsed() {
		t.Error("the opportunity attack must spend the imp's reaction")
	}
	_, err = a.attackAs(t, a.bia, e, "Fagulha", key, "Goblin", d20(15), true)
	wantBlockedBy(t, "a second reaction attack", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_REACTION_USED)
	// On its own turn it never takes the Attack action: the options disable its
	// attacks (only the reaction does) and offer no Attack action, and an attack is refused.
	e = a.passTo(t, e, "Fagulha")
	opts := a.mustOptions(t, a.bia, e, "Fagulha")
	if standardOption(opts, "standard:attack") != nil || len(opts.GetOptions().GetAttacks()) == 0 {
		t.Fatalf("options = %v, want its attacks and no Attack action", opts.GetOptions())
	}
	if sting := opts.GetOptions().GetAttacks()[0]; sting.GetEnabled() || sting.GetReason().GetCode() != rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_REACTION_ONLY {
		t.Errorf("the imp's attack = %v, want disabled as an action (REACTION_ONLY)", sting)
	}
	_, err = a.attackAs(t, a.bia, e, "Fagulha", key, "Goblin", d20(15), false)
	wantBlockedBy(t, "RollAttack on its own turn", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_CREATURE_CANNOT_ATTACK)
}

// undoTwice undoes the last action and then the one before it, as the master does.
func (a *armed) undoLast(t *testing.T, e *playv1.Encounter) {
	t.Helper()
	id := a.log(t, a.master, e).GetUndoableEventId()
	if id == "" {
		t.Fatal("nothing to undo, want an action")
	}
	if err := a.undo(t, a.master, e, id); err != nil {
		t.Fatalf("UndoLastAction() error = %v", err)
	}
}

// TestMR037_UndoChainSurvivesTheCreaturesEvents: the creature_summoned and
// creature_dismissed events sit before the change's own, and never close the undo:
// after undoing a cast, the action before it can be undone too.
func TestMR037_UndoChainSurvivesTheCreaturesEvents(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	e := a.summonersFight(t)
	e = a.toSalvia(t, e)
	if _, err := a.adjustHP(t, e, "Goblin", damageHP(3)); err != nil {
		t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
	}
	if _, err := a.summonWolves(t, a.bia, e, 11); err != nil {
		t.Fatalf("CastSpell() error = %v", err)
	}
	a.undoLast(t, e) // the cast
	a.undoLast(t, e) // the goblin's damage, behind the creature_summoned event
	if cur, _, _ := a.hp(t, "Goblin"); cur != 7 {
		t.Errorf("the goblin has %d hit points after two undos, want 7", cur)
	}

	// After an EndConcentration.
	if _, err := a.adjustHP(t, e, "Goblin", damageHP(3)); err != nil {
		t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
	}
	if _, err := a.summonWolves(t, a.bia, e, 11); err != nil {
		t.Fatalf("CastSpell() error = %v", err)
	}
	if _, err := a.conditions(t, a.bia, e, "Sálvia", nil, false, true); err != nil {
		t.Fatalf("end concentration error = %v", err)
	}
	a.undoLast(t, e) // the end of the concentration
	a.undoLast(t, e) // the cast
	a.undoLast(t, e) // the damage
	if cur, _, _ := a.hp(t, "Goblin"); cur != 7 || len(a.mustCreatures(t, a.bia, a.bri)) != 0 {
		t.Errorf("after three undos: goblin %d hit points, creatures %d, want 7 and none", cur, len(a.mustCreatures(t, a.bia, a.bri)))
	}

	// After a recast that dismissed creatures.
	if _, err := a.adjustHP(t, e, "Goblin", damageHP(3)); err != nil {
		t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
	}
	if _, err := a.summonWolves(t, a.bia, e, 11); err != nil {
		t.Fatalf("CastSpell() error = %v", err)
	}
	if _, err := a.summonWolves(t, a.master, e, 4); err != nil {
		t.Fatalf("CastSpell() again error = %v", err)
	}
	a.undoLast(t, e)
	a.undoLast(t, e)
	a.undoLast(t, e)
	if cur, _, _ := a.hp(t, "Goblin"); cur != 7 || len(a.get(t, a.master).GetCombatants()) != 4 {
		t.Errorf("after the recast's undos: goblin %d hit points, %d combatants, want 7 and 4", cur, len(a.get(t, a.master).GetCombatants()))
	}
}

// TestMR037_UndoOfEndingAConcentrationBringsTheSameWolvesBack: the dismissed
// combatants are hidden, not deleted: their ids, hit points and conditions return.
func TestMR037_UndoOfEndingAConcentrationBringsTheSameWolvesBack(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	e := a.summonersFight(t)
	e = a.toSalvia(t, e)
	if _, err := a.summonWolves(t, a.bia, e, 11); err != nil {
		t.Fatalf("CastSpell() error = %v", err)
	}
	before := byLabel(t, a.get(t, a.master), "Lobo atroz 1").GetId()
	if _, err := a.adjustHP(t, e, "Lobo atroz 1", damageHP(34)); err != nil {
		t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
	}
	if _, err := a.conditions(t, a.master, e, "Lobo atroz 1", []string{"condition:poisoned"}, true, false); err != nil {
		t.Fatalf("SetCombatantConditions() error = %v", err)
	}
	if _, err := a.conditions(t, a.bia, e, "Sálvia", nil, false, true); err != nil {
		t.Fatalf("end concentration error = %v", err)
	}
	if got := labels(a.get(t, a.master)); len(got) != 4 {
		t.Fatalf("combatants = %v, want the wolves out", got)
	}
	a.undoLast(t, e)
	w := byLabel(t, a.get(t, a.master), "Lobo atroz 1")
	if w.GetId() != before || w.GetHitPointsCurrent() != 3 || len(w.GetConditions()) != 1 || w.GetInitiative() != 13 {
		t.Errorf("wolf = %v, want the same combatant at 3/37 with its condition", w)
	}
	if got := a.mustCreatures(t, a.bia, a.bri); len(got) != 2 {
		t.Errorf("creatures = %d, want 2", len(got))
	}
}

// TestMR037_ConjureAnimalsAgainOutsideACombatDismissesTheFirst: a new summon that
// needs concentration dismisses the caster's previous concentration creatures.
func TestMR037_ConjureAnimalsAgainOutsideACombatDismissesTheFirst(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	a.mustCastSummon(t, a.bia, a.bri, conjureAnimals, slotOfLevel(3), 1, []string{direWolf, direWolf})
	first := a.mustCreatures(t, a.bia, a.bri)
	res := a.mustCastSummon(t, a.bia, a.bri, conjureAnimals, slotOfLevel(3), 1, []string{direWolf, direWolf})
	list := a.mustCreatures(t, a.bia, a.bri)
	if len(list) != 2 || len(res.GetDismissedCreatureIds()) != 2 || list[0].GetId() == first[0].GetId() {
		t.Errorf("creatures = %v, dismissed = %v, want only the second casting's wolves", creatureNames(list), res.GetDismissedCreatureIds())
	}
	if n := a.eventCount(t, "creature_dismissed"); n != 2 {
		t.Errorf("creature_dismissed events = %d, want 2", n)
	}
}

// TestMR037_ACombatWithNoRoomRefusesBeforeSpendingTheSlot: a combat holds 40
// combatants; the summon is refused with a reason, and nothing is spent.
func TestMR037_ACombatWithNoRoomRefusesBeforeSpendingTheSlot(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	e := a.start(t, plan{
		npcs:    []*playv1.Participant{{CharacterId: a.goblin.GetId(), Count: 10}, {CharacterId: a.capitao.GetId(), Count: 10}},
		players: map[string]int32{"Pensantus": 20, "Sálvia": 15, "Toren": 12},
	})
	add := func(c *charactersv1.Character, n int32) {
		if _, err := a.master.combat.AddCombatants(t.Context(), connect.NewRequest(&playv1.AddCombatantsRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey(), Participants: []*playv1.Participant{{CharacterId: c.GetId(), Count: n}},
		})); err != nil {
			t.Fatalf("AddCombatants() error = %v", err)
		}
	}
	add(a.goblin, 10)
	add(a.capitao, 6) // 3 + 10 + 10 + 10 + 6 = 39 combatants
	e = a.toSalvia(t, a.get(t, a.master))
	_, err := a.summonWolves(t, a.bia, e, 11)
	wantBlockedBy(t, "CastSpell with no room", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TOO_MANY_COMBATANTS)
	if usedSlots(a.vitals(t, a.bri), 3) != 0 || len(a.mustCreatures(t, a.bia, a.bri)) != 0 {
		t.Error("a refused summon must spend nothing and make nothing")
	}
}

// TestMR037_RenamingACreatureRenamesItsCombatant, and friendly fire counts nowhere
// in the highlights.
func TestMR037_RenamingACreatureRenamesItsCombatantAndFriendlyFireIsNotAHighlight(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	e := a.summonersFight(t)
	e = a.toSalvia(t, e)
	if _, err := a.summonWolves(t, a.bia, e, 11); err != nil {
		t.Fatalf("CastSpell() error = %v", err)
	}
	wolf := a.mustCreatures(t, a.bia, a.bri)[0]
	if _, err := a.bia.characters.RenameCreature(t.Context(), connect.NewRequest(&charactersv1.RenameCreatureRequest{CampaignId: a.campaignID, CreatureId: wolf.GetId(), Name: "Sombra"})); err != nil {
		t.Fatalf("RenameCreature() error = %v", err)
	}
	if byLabel(t, a.get(t, a.ana), "Sombra").GetMonsterKey() != direWolf {
		t.Error("the combatant must take the creature's new name")
	}
	// Toren hits Sálvia's wolf with a critical: friendly fire, nobody's highlight.
	hit, err := a.attackAs(t, a.master, e, "Toren", "equipment:battleaxe", "Sombra", d20(20), true)
	if err != nil {
		t.Fatalf("RollAttack() error = %v", err)
	}
	a.mustDamage(t, a.master, e, hit.GetPendingDamage().GetId(), typedDamage(8))
	if _, err := a.master.combat.EndEncounter(t.Context(), connect.NewRequest(&playv1.EndEncounterRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey()})); err != nil {
		t.Fatalf("EndEncounter() error = %v", err)
	}
	h, err := a.highlights(t, a.master, e)
	if err != nil || len(h.GetCategories()) != 0 {
		t.Errorf("highlights = %v, %v, want none: friendly fire counts nowhere", h, err)
	}
}

// wolfFight starts the cave fight with Toren's wolf "Presa" (the master's gift,
// 40 ft of speed) on (7, 7), and passes Toren's turn so it is the wolf's.
func (c *cave) wolfFight(t *testing.T) *playv1.Encounter {
	t.Helper()
	c.give(t, c.toren, "monster:wolf", "Presa")
	e := c.start(t, plan{
		npcs: []*playv1.Participant{
			{CharacterId: c.capitao.GetId()}, {CharacterId: c.goblins.GetId(), Count: 3}, {CharacterId: c.ogre.GetId()}, {CharacterId: c.squire.GetId()},
		},
		npcRolls: []int{2, 2, 2, 2, 2, 2},
		players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1, "Presa": 15},
		reveal:   []string{"Capitão Goblin", "Goblin 1", "Goblin 2", "Goblin 3", "Ogro", "Escudeiro"},
		at: map[string][2]int32{
			"Toren": {6, 7}, "Pensantus": {5, 8}, "Brisa": {4, 7}, "Escudeiro": {3, 8}, "Presa": {7, 7},
			"Goblin 1": {18, 5}, "Goblin 2": {20, 7}, "Goblin 3": {21, 3}, "Capitão Goblin": {12, 13}, "Ogro": {22, 9},
		},
	})
	return c.mustEndTurn(t, c.caio, e)
}

// TestMR037_ACreatureMovesLikeAnyCombatant: the creature joins the party's side
// with its stat block's size, fly speed and jumps; its owner's player moves it on
// its turn and the master always; another player is refused. It crosses painted
// rubble at a cost, an enemy in the way stops it, an undo gives the movement back,
// and an attack breaks its running start.
func TestMR037_ACreatureMovesLikeAnyCombatant(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	content, err := testRules()
	if err != nil {
		t.Fatalf("rules.LoadSRD() error = %v", err)
	}
	msvc, err := maps.New(maps.Config{Pool: c.h.pool, Characters: c.h.chars, Live: c.h.svc, Rules: content, Combats: c.h.svc, Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("maps.New() error = %v", err)
	}
	c.h.svc.SetTerrain(msvc)
	srv := httpserver.New(httpserver.Config{Logger: slog.New(slog.DiscardHandler)})
	msvc.Mount(srv.Handle, mapsSessions{testSessions}, c.h.camps, reqlimit.HandlerOption())
	server := httptest.NewServer(srv.Handler())
	t.Cleanup(server.Close)
	paint := mapsv1connect.NewMapServiceClient(&http.Client{Transport: userTransport{userID: c.master.id, next: server.Client().Transport}}, server.URL)

	e := c.wolfFight(t)
	wolf := byLabel(t, e, "Presa")
	if wolf.GetSide() != playv1.CombatantSide_COMBATANT_SIDE_PARTY || wolf.GetSize() != rulesv1.CreatureSize_CREATURE_SIZE_MEDIUM {
		t.Errorf("wolf = %v, want the party's side and a Medium size", wolf)
	}
	if e.GetCurrentCombatantId() != wolf.GetId() {
		t.Fatalf("current = %s, want the wolf's turn", e.GetCurrentCombatantId())
	}

	// Another player may not move it; its owner and the master may.
	_, err = c.move(t, c.ana, "Presa", 8, 7)
	wantCode(t, "MoveCombatant by another player", err, connect.CodePermissionDenied)
	_, err = c.options(t, c.ana, "Presa")
	wantCode(t, "GetMoveOptions by another player", err, connect.CodePermissionDenied)
	if opts, err := c.options(t, c.caio, "Presa"); err != nil || len(opts.GetReachable()) == 0 {
		t.Errorf("GetMoveOptions by the owner = %v, %v, want squares", opts, err)
	}

	// An enemy in the way stops it (a Small goblin is not two sizes apart).
	c.mustMove(t, c.master, "Goblin 1", 9, 7)
	_, err = c.move(t, c.caio, "Presa", 11, 7)
	wantEncounterBlocked(t, err, reasonEnemy)
	c.mustMove(t, c.master, "Goblin 1", 18, 5)

	// Painted rubble costs 5 ft more: 10 ft of line and 5 ft of rubble.
	if _, err := paint.PaintMapCells(t.Context(), connect.NewRequest(&mapsv1.PaintMapCellsRequest{
		CampaignId: c.campaignID, MapId: c.mapID, Layer: mapsv1.MapLayer_MAP_LAYER_DIFFICULT_TERRAIN, Value: 1, Squares: []*mapsv1.MapSquare{{Col: 8, Row: 7}},
	})); err != nil {
		t.Fatalf("PaintMapCells() error = %v", err)
	}
	c.mustMove(t, c.caio, "Presa", 9, 7)
	if got := c.who(t, c.caio, "Presa"); got.GetMovementUsedDft() != 150 {
		t.Errorf("through rubble = %d dft used, want 150", got.GetMovementUsedDft())
	}
	c.undoLast(t)
	if got := c.who(t, c.caio, "Presa"); got.GetMovementUsedDft() != 0 || got.GetCol() != 7 {
		t.Errorf("after the undo: %d dft used on column %d, want 0 and 7", got.GetMovementUsedDft(), got.GetCol())
	}

	// Run 15 ft, then a bite breaks the running start: the 10 ft jump (a wolf's
	// Strength 12 gives 12 ft running, 6 ft standing) is then refused.
	c.mustMove(t, c.master, "Goblin 1", 10, 6)
	c.mustMove(t, c.caio, "Presa", 10, 7)
	hit, err := c.attack(t, c.caio, c.get(t, c.master), "Presa", "monster:wolf#bite", "Goblin 1", d20(15))
	if err != nil || hit.GetRoll().GetOutcome() == playv1.AttackOutcome_ATTACK_OUTCOME_MISS {
		t.Fatalf("the wolf's bite = %v, %v, want a hit", hit, err)
	}
	_, err = c.move(t, c.caio, "Presa", 12, 7, jumpTo(playv1.JumpKind_JUMP_KIND_LONG))
	wantEncounterBlocked(t, err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TOO_FAR)
}

// TestMR037_ADismissedCreatureBlocksNoSquare: the combatant of a creature whose
// concentration ended is hidden from the movement: its square is free.
func TestMR037_ADismissedCreatureBlocksNoSquare(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	e := a.summonersFight(t)
	e = a.toSalvia(t, e)
	if _, err := a.summonWolves(t, a.bia, e, 11); err != nil {
		t.Fatalf("CastSpell() error = %v", err)
	}
	w := byLabel(t, a.get(t, a.master), "Lobo atroz 1")
	col, row := w.GetCol(), w.GetRow()
	// Alive, the wolf's square is taken (Toren cannot end his move there): the master
	// moves Toren next to it first, as Toren's turn is not now.
	if _, err := a.conditions(t, a.bia, e, "Sálvia", nil, false, true); err != nil {
		t.Fatalf("end concentration error = %v", err)
	}
	a.mustEndTurn(t, a.bia, e) // Sálvia's turn ends: Toren's
	if _, err := a.master.combat.MoveCombatant(t.Context(), connect.NewRequest(&playv1.MoveCombatantRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Goblin"), IdempotencyKey: newKey(), Col: 0, Row: 0,
	})); err != nil { // the goblin stands on Toren's line
		t.Fatalf("MoveCombatant(Goblin) error = %v", err)
	}
	_, err := a.caio.combat.MoveCombatant(t.Context(), connect.NewRequest(&playv1.MoveCombatantRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Toren"), IdempotencyKey: newKey(), Col: col, Row: row,
	}))
	if connect.CodeOf(err) == connect.CodeFailedPrecondition {
		t.Errorf("MoveCombatant onto a dismissed wolf's square error = %v, want it free", err)
	}
}

// TestMR037_ADismissedCreatureTakesNoWaitingDamageSilently: dismissing a creature
// whose attack or whose wound still waits for the master writes the dropped damage
// in the log, as passing the turn over it does, instead of letting the attack
// vanish with the combatant.
func TestMR037_ADismissedCreatureTakesNoWaitingDamageSilently(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	e := a.summonersFight(t)
	e = a.toSalvia(t, e)
	if _, err := a.summonWolves(t, a.bia, e, 11); err != nil {
		t.Fatalf("CastSpell() error = %v", err)
	}
	a.h.roller.queue(15)
	hit := a.mustAttackAsReaction(t, e, "Goblin", sword, "Lobo atroz 1")
	if hit.GetPendingDamage().GetId() == "" {
		t.Fatalf("attack = %v, want a hit that leaves its damage waiting", hit)
	}
	wolf := byLabel(t, a.get(t, a.master), "Lobo atroz 1").GetCreatureId()
	before := a.eventCount(t, eventDamageDiscarded)
	if _, err := a.bia.characters.DismissCreature(t.Context(), connect.NewRequest(&charactersv1.DismissCreatureRequest{CampaignId: a.campaignID, CreatureId: wolf})); err != nil {
		t.Fatalf("DismissCreature() error = %v", err)
	}
	if got := a.eventCount(t, eventDamageDiscarded); got != before+1 {
		t.Errorf("damage_discarded events = %d after the dismissal, want %d: the waiting damage must be written as dropped", got, before+1)
	}
	var entry *playv1.CombatLogEntry
	for _, r := range a.log(t, a.master, e).GetRounds() {
		for _, en := range r.GetEntries() {
			if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_ATTACK && en.GetActorLabel() == "Goblin" {
				entry = en
			}
		}
	}
	if entry == nil || entry.GetDamage().GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_DISCARDED {
		t.Errorf("the attack's log entry = %v, want it with its damage discarded", entry)
	}
}
