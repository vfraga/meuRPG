package play

import (
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
)

// Find Steed: a paladin casts it outside a combat and gets a steed that stays bonded to the character.

const (
	findSteed = "spell:find-steed"
	warhorse  = "monster:warhorse"
	elk       = "monster:elk"

	divineFavor = "spell:divine-favor"
)

// newRiders is the party of this file: Aurélio, a level 9 paladin (three 2nd circle slots) who has Find Steed prepared, and Toren.
func newRiders(t *testing.T) *armed {
	t.Helper()
	return newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 5,
			&rulesv1.AbilityScores{Strength: 16, Dexterity: 13, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{battleaxe}, nil)
		a.pens = a.ana.caster(t, a.campaignID, "Aurélio", "class:paladin", "race:human", 9,
			&rulesv1.AbilityScores{Strength: 16, Dexterity: 10, Constitution: 14, Intelligence: 10, Wisdom: 12, Charisma: 16}, nil, nil,
			[]string{findSteed, divineFavor, cureWounds}, []string{findSteed, divineFavor, cureWounds})
	})
}

// castSteed casts Find Steed as the paladin's player with a 2nd circle slot.
func (a *armed) castSteed(t *testing.T, form, kind string, names ...string) (*playv1.CastSummonResponse, error) {
	t.Helper()
	return a.castSteedAt(t, 2, form, kind, names...)
}

// castSteedAt casts it with a slot of the circle.
func (a *armed) castSteedAt(t *testing.T, circle int32, form, kind string, names ...string) (*playv1.CastSummonResponse, error) {
	t.Helper()
	res, err := a.ana.play.CastSummon(t.Context(), connect.NewRequest(&playv1.CastSummonRequest{
		CampaignId: a.campaignID, CharacterId: a.pens.GetId(), SpellKey: findSteed, Slot: slotOfLevel(circle), IdempotencyKey: newKey(),
		Summon: &playv1.SummonChoice{CreatureKeys: []string{form}, CreatureType: kind, Names: names},
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (a *armed) mustCastSteed(t *testing.T, form, kind string, names ...string) *charactersv1.CharacterCreature {
	t.Helper()
	res, err := a.castSteed(t, form, kind, names...)
	if err != nil {
		t.Fatalf("CastSummon(%s, %s) error = %v", form, kind, err)
	}
	if len(res.GetCreatureIds()) != 1 {
		t.Fatalf("CastSummon() made %v, want the steed", res.GetCreatureIds())
	}
	for _, c := range a.mustCreatures(t, a.ana, a.pens) {
		if c.GetId() == res.GetCreatureIds()[0] {
			return c
		}
	}
	t.Fatalf("the steed %s is not on the list", res.GetCreatureIds()[0])
	return nil
}

func (a *armed) dismissCreatureAs(t *testing.T, u *user, id string) {
	t.Helper()
	if _, err := u.characters.DismissCreature(t.Context(), connect.NewRequest(&charactersv1.DismissCreatureRequest{CampaignId: a.campaignID, CreatureId: id})); err != nil {
		t.Fatalf("DismissCreature() error = %v", err)
	}
}

func (a *armed) setCreatureHitPoints(t *testing.T, id string, hp int32) {
	t.Helper()
	req := &charactersv1.AdjustCreatureHitPointsRequest{CampaignId: a.campaignID, CreatureId: id, Change: &charactersv1.AdjustCreatureHitPointsRequest_HitPoints{HitPoints: hp}}
	if _, err := a.master.characters.AdjustCreatureHitPoints(t.Context(), connect.NewRequest(req)); err != nil {
		t.Fatalf("AdjustCreatureHitPoints() error = %v", err)
	}
}

// TestFindSteedMakesABondedSteedOfTheChosenType: the cast spends the slot and the steed is the character's creature with the form, the name and the type
// the player picked, at full hit points, and only the owner and the master read it.
func TestFindSteedMakesABondedSteedOfTheChosenType(t *testing.T) {
	t.Parallel()
	a := newRiders(t)
	before := usedSlots(a.vitals(t, a.pens), 2)
	steed := a.mustCastSteed(t, warhorse, "fey", "Trovão")
	if steed.GetName() != "Trovão" || steed.GetMonsterKey() != warhorse || steed.GetSource() != charactersv1.CreatureSource_CREATURE_SOURCE_FIND_STEED ||
		steed.GetCreatureType() != "fey" || !steed.GetBonded() || steed.GetHitPointsCurrent() != 19 || steed.GetHitPointsMax() != 19 || steed.GetAttack() != 3 {
		t.Errorf("steed = %v", steed)
	}
	if got := usedSlots(a.vitals(t, a.pens), 2); got != before+1 {
		t.Errorf("2nd circle slots used = %d, want %d", got, before+1)
	}
	_, err := a.creatures(t, a.caio, a.pens)
	wantCode(t, "ListCharacterCreatures as another player", err, connect.CodeNotFound)
	if got := a.mustCreatures(t, a.master, a.pens); len(got) != 1 || got[0].GetCreatureType() != "fey" {
		t.Errorf("the master reads %v, want the steed with its type", got)
	}
	var floor *int32
	if err := a.h.pool.QueryRow(t.Context(), `SELECT (stat_block->>'MinInt')::int4 FROM character_creatures WHERE id = $1`, steed.GetId()).Scan(&floor); err != nil || floor == nil || *floor != 6 {
		t.Errorf("the steed keeps the Intelligence floor in its stat block: %v, %v", floor, err)
	}
}

// TestFindSteedNeedsOneOfTheThreeTypes: the type is the player's choice among the spell's three, and a spell that offers none takes none.
func TestFindSteedNeedsOneOfTheThreeTypes(t *testing.T) {
	t.Parallel()
	a := newRiders(t)
	before := usedSlots(a.vitals(t, a.pens), 2)
	for _, kind := range []string{"", "beast", "Fey", "undead"} {
		_, err := a.castSteed(t, warhorse, kind)
		wantBlockedBy(t, "CastSummon(type "+kind+")", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_SUMMON_CHOICE_INVALID)
	}
	_, err := a.castSteed(t, "monster:wolf", "fey")
	wantBlockedBy(t, "CastSummon(a wolf)", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_SUMMON_CHOICE_INVALID)
	if got := usedSlots(a.vitals(t, a.pens), 2); got != before || len(a.mustCreatures(t, a.ana, a.pens)) != 0 {
		t.Errorf("a refused cast spent a slot (%d, want %d) or made a creature", got, before)
	}
	// A medium rider casts it for a form it cannot ride: the cast is allowed.
	if steed := a.mustCastSteed(t, "monster:pony", "celestial"); steed.GetMonsterKey() != "monster:pony" {
		t.Errorf("steed = %v, want the pony", steed)
	}
}

// TestFindSteedCastAgainBringsTheSameSteedBack: a character has one steed. A new cast revives the one it has, with its form,
// name and type, at full hit points; it never makes another and never lets the player pick another form or type.
func TestFindSteedCastAgainBringsTheSameSteedBack(t *testing.T) {
	t.Parallel()
	a := newRiders(t)
	steed := a.mustCastSteed(t, warhorse, "fiend", "Trovão")
	a.setCreatureHitPoints(t, steed.GetId(), 5)
	a.dismissCreatureAs(t, a.ana, steed.GetId())
	if got := a.mustCreatures(t, a.ana, a.pens); len(got) != 0 {
		t.Fatalf("creatures after the dismissal = %v, want none listed", got)
	}
	used := usedSlots(a.vitals(t, a.pens), 2)

	for name, tt := range map[string]struct{ form, kind string }{"another form": {elk, "fiend"}, "another type": {warhorse, "fey"}, "both": {elk, "celestial"}} {
		_, err := a.castSteed(t, tt.form, tt.kind)
		wantBlockedBy(t, "CastSummon("+name+")", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_SUMMON_CHOICE_INVALID)
	}
	if got := usedSlots(a.vitals(t, a.pens), 2); got != used {
		t.Errorf("refused casts spent slots: %d, want %d", got, used)
	}

	back := a.mustCastSteed(t, warhorse, "fiend", "Outro nome")
	if back.GetId() != steed.GetId() || back.GetName() != "Trovão" || back.GetCreatureType() != "fiend" || back.GetHitPointsCurrent() != 19 || !back.GetBonded() {
		t.Errorf("steed back = %v, want the same row at full hit points", back)
	}
	if got := a.mustCreatures(t, a.ana, a.pens); len(got) != 1 {
		t.Errorf("creatures = %v, want the one steed", got)
	}
	if got := usedSlots(a.vitals(t, a.pens), 2); got != used+1 {
		t.Errorf("2nd circle slots used = %d, want %d", got, used+1)
	}
	// A live steed is not made twice either.
	res, err := a.castSteed(t, warhorse, "fiend")
	if err != nil || len(res.GetCreatureIds()) != 1 || res.GetCreatureIds()[0] != steed.GetId() || len(a.mustCreatures(t, a.ana, a.pens)) != 1 {
		t.Errorf("a cast with the steed alive made another: %v, %v", res, err)
	}
}

// TestFindSteedOptionsSayWhatTheSheetOffers: the sheet offers the three types, the Intelligence floor, the mount and the forms the rider is too
// large for, and, once the steed is bonded, the steed to bring back.
func TestFindSteedOptionsSayWhatTheSheetOffers(t *testing.T) {
	t.Parallel()
	a := newRiders(t)
	find := func() *charactersv1.SummonSpellOptions {
		for _, sp := range a.summonOptions(t, a.ana, a.pens).GetSpells() {
			if sp.GetSpellKey() == findSteed {
				return sp
			}
		}
		t.Fatalf("Find Steed is not among the summon spells")
		return nil
	}
	sp := find()
	if got := sp.GetTypeChoices(); len(got) != 3 || got[0] != "celestial" || got[1] != "fey" || got[2] != "fiend" || sp.GetMinInt() != 6 || sp.GetBondedSteed() != nil {
		t.Errorf("options = %v", sp)
	}
	if len(sp.GetCircles()) == 0 || len(sp.GetCircles()[0].GetOptions()) != 1 {
		t.Fatalf("circles = %v", sp.GetCircles())
	}
	opt := sp.GetCircles()[0].GetOptions()[0]
	if !opt.GetMount() || len(opt.GetForms()) != 5 || len(opt.GetTooSmallForYou()) != 2 {
		t.Errorf("option = %v, want a mount of five forms, two too small for a Medium rider", opt)
	}
	steed := a.mustCastSteed(t, warhorse, "celestial")
	if got := find().GetBondedSteed(); got.GetId() != steed.GetId() {
		t.Errorf("bonded steed = %v, want %s", got, steed.GetId())
	}
}

// intOfCreature is the Intelligence score ContentService.GetCreature answers for the creature (empty id: the entry's own).
func (a *armed) intOfCreature(t *testing.T, key, creatureID string) int32 {
	t.Helper()
	res, err := a.ana.content.GetCreature(t.Context(), connect.NewRequest(&rulesv1.GetCreatureRequest{CampaignId: a.campaignID, Key: key, CreatureId: creatureID}))
	if err != nil {
		t.Fatalf("GetCreature(%s) error = %v", key, err)
	}
	for _, s := range res.Msg.GetCreature().GetAbilities() {
		if s.GetAbility() == rulesv1.Ability_ABILITY_INTELLIGENCE {
			return s.GetScore()
		}
	}
	t.Fatalf("GetCreature(%s) has no Intelligence", key)
	return 0
}

// TestFindSteedSheetShowsTheIntelligenceFloor: the owner's page of the steed reads Intelligence 6, the creature's entry keeps its
// own 2, and the page says nothing in the log about an unreadable stat block.
func TestFindSteedSheetShowsTheIntelligenceFloor(t *testing.T) {
	t.Parallel()
	a := newRiders(t)
	steed := a.mustCastSteed(t, warhorse, "fey")
	if got := a.intOfCreature(t, warhorse, steed.GetId()); got != 6 {
		t.Errorf("the steed's Intelligence on its sheet = %d, want 6", got)
	}
	if got := a.intOfCreature(t, warhorse, ""); got != 2 {
		t.Errorf("the warhorse entry's Intelligence = %d, want its own 2", got)
	}
}

// TestFindSteedCombatAndListReadTheIntelligenceFloor: the creature's saving throw (the combat's read, through the stat block of the
// row) uses Intelligence 6, and a warhorse given by the master, with no floor, keeps 2.
func TestFindSteedCombatAndListReadTheIntelligenceFloor(t *testing.T) {
	t.Parallel()
	a := newRiders(t)
	steed := a.mustCastSteed(t, warhorse, "fey")
	plain := a.give(t, a.toren, warhorse, "Sem vínculo")
	saveOf := func(id string) int {
		var bonus int
		err := db.InTx(t.Context(), a.h.pool, func(tx pgx.Tx) error {
			s, err := a.h.chars.CreatureSave(t.Context(), tx, a.campaignID, id, warhorse, "int")
			bonus = s.Bonus
			return err
		})
		if err != nil {
			t.Fatalf("CreatureSave() error = %v", err)
		}
		return bonus
	}
	if got := saveOf(steed.GetId()); got != -2 {
		t.Errorf("the steed's Intelligence save = %d, want -2 (score 6)", got)
	}
	if got := saveOf(plain.GetId()); got != -4 {
		t.Errorf("a warhorse with no floor: Intelligence save = %d, want -4 (score 2)", got)
	}
}

// TestFindSteedOptionsAreRefusedToAnotherPlayer: another player does not read the summon options of a character with a bonded steed,
// the steed it would bring back included.
func TestFindSteedOptionsAreRefusedToAnotherPlayer(t *testing.T) {
	t.Parallel()
	a := newRiders(t)
	a.mustCastSteed(t, warhorse, "fey", "Trovão")
	_, err := a.caio.characters.GetSummonOptions(t.Context(), connect.NewRequest(&charactersv1.GetSummonOptionsRequest{CampaignId: a.campaignID, CharacterId: a.pens.GetId()}))
	wantCode(t, "GetSummonOptions as another player", err, connect.CodeNotFound)
}
