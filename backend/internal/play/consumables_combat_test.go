package play

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// A consumable used in a running combat (ChargesService.UseConsumable, ListConsumableTargets): drinking or giving costs the
// user's action on its turn, a gift reaches only a player's character within 5 feet of the giver, and the line of the log tells the
// table the item and the owners the numbers. These tests need the database.

var blockedNotAdjacent = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TARGET_NOT_ADJACENT

// potionFight is Toren (Caio's, first on the turn), Pensantus (Ana's, next to Toren) and Brisa (Bia's, far) in a combat
// with a goblin far away; each of the three holds the potions.
type potionFight struct {
	*stocked
	e *playv1.Encounter
}

func newPotionFight(t *testing.T, keys ...string) *potionFight {
	t.Helper()
	s := newStocked(t, keys...)
	s.bri = s.withItems(t, s.bri, keys...)
	e := s.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: s.goblin.GetId()}},
		npcRolls: []int{3},
		players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1},
		reveal:   []string{"Goblin"},
		at:       map[string][2]int32{"Toren": {3, 3}, "Pensantus": {4, 4}, "Brisa": {9, 8}, "Goblin": {12, 8}},
	})
	return &potionFight{stocked: s, e: e}
}

// line is the consumable line a user reads in the log, nil when it reads none.
func (f *potionFight) line(t *testing.T, u *user) *playv1.CombatLogEntry {
	t.Helper()
	for _, r := range f.log(t, u, f.e).GetRounds() {
		for _, en := range r.GetEntries() {
			if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_CONSUMABLE_USED {
				return en
			}
		}
	}
	return nil
}

// targets is the list of receivers u reads for the potion of who's sheet.
func (s *stocked) targets(t *testing.T, u *user, who *charactersv1.Character, key string) []*playv1.ConsumableTarget {
	t.Helper()
	res, err := u.charges.ListConsumableTargets(t.Context(), connect.NewRequest(&playv1.ListConsumableTargetsRequest{
		CampaignId: s.campaignID, CharacterId: who.GetId(), EntryId: s.entryOf(t, who, key),
	}))
	if err != nil {
		t.Fatalf("ListConsumableTargets() error = %v", err)
	}
	return res.Msg.GetTargets()
}

func TestDrinkingInACombatCostsTheActionOnYourTurn(t *testing.T) {
	t.Parallel()
	f := newPotionFight(t, potionCommon, potionGreater)
	f.hurt(t, f.toren, 1)
	_, err := f.use(t, f.ana, f.pens, potionCommon)
	wantBlockedBy(t, "a drink off turn", err, blockedNotYourTurn)
	if !f.holds(t, f.pens, potionCommon) {
		t.Error("the refused drink off turn spent the potion")
	}
	res := f.mustUse(t, f.caio, f.toren, potionCommon)
	if res.GetHealed() == 0 || f.holds(t, f.toren, potionCommon) {
		t.Errorf("the drink = %v, want the unit spent and a heal", res)
	}
	_, err = f.use(t, f.caio, f.toren, potionGreater)
	wantBlockedBy(t, "a second drink in the turn", err, blockedActionUsed)
	if !f.holds(t, f.toren, potionGreater) {
		t.Error("the refused drink spent the potion")
	}
}

func TestGivingAPotionNextToTheGiverHealsTheReceiverAndSpendsTheGiversAction(t *testing.T) {
	t.Parallel()
	f := newPotionFight(t, potionCommon, potionGreater)
	f.hurt(t, f.pens, 1)
	res := f.mustUse(t, f.caio, f.toren, potionCommon, giveTo(f.pens))
	if res.GetHealed() == 0 || res.GetTargetCharacterId() != f.pens.GetId() || f.holds(t, f.toren, potionCommon) || !f.holds(t, f.pens, potionCommon) {
		t.Errorf("the gift = %v, want Pensantus healed by Toren's potion", res)
	}
	if len(res.GetVitals()) != 0 {
		t.Errorf("the giver was told the receiver's vitals: %v", res.GetVitals())
	}
	_, err := f.use(t, f.caio, f.toren, potionGreater)
	wantBlockedBy(t, "a drink after the gift", err, blockedActionUsed)
}

func TestAPotionIsNotGivenOutOfReach(t *testing.T) {
	t.Parallel()
	f := newPotionFight(t, potionCommon)
	_, err := f.use(t, f.caio, f.toren, potionCommon, giveTo(f.bri))
	wantBlockedBy(t, "a gift out of reach", err, blockedNotAdjacent)
	if !f.holds(t, f.toren, potionCommon) {
		t.Error("a refused gift spent the potion")
	}
	f.mustUse(t, f.caio, f.toren, potionCommon, giveTo(f.pens)) // the refusals spent no action
}

func TestTheMasterUsesAPotionForACharacterBoundByItsTurn(t *testing.T) {
	t.Parallel()
	f := newPotionFight(t, potionCommon)
	f.hurt(t, f.pens, 1)
	_, err := f.use(t, f.master, f.pens, potionCommon)
	wantBlockedBy(t, "the master acting for a character off turn", err, blockedNotYourTurn)
	if res := f.mustUse(t, f.master, f.toren, potionCommon, giveTo(f.pens)); res.GetHealed() == 0 || len(res.GetVitals()) != 1 {
		t.Errorf("the master's gift = %v, want a heal and the receiver's vitals", res)
	}
}

func TestAReceiverAtZeroHitPointsWakesAndAnUnconsciousUserCannotAct(t *testing.T) {
	t.Parallel()
	f := newPotionFight(t, potionCommon, potionSupreme)
	f.hurt(t, f.pens, 0)
	f.mustUse(t, f.caio, f.toren, potionCommon, giveTo(f.pens))
	if got := f.vitals(t, f.pens).GetHitPointsCurrent(); got == 0 {
		t.Error("Pensantus is still at 0 hit points")
	}
	if c := byLabel(t, f.get(t, f.master), "Pensantus"); c.GetDeathSuccesses() != 0 || c.GetDeathFailures() != 0 {
		t.Errorf("the death saves = %d/%d, want them reset", c.GetDeathSuccesses(), c.GetDeathFailures())
	}
	f.hurt(t, f.toren, 0)
	_, err := f.use(t, f.caio, f.toren, potionSupreme)
	wantBlockedBy(t, "the unconscious user", err, blockedDown)
}

func TestASupremePotionIsCutAtTheMaximumInACombat(t *testing.T) {
	t.Parallel()
	f := newPotionFight(t, potionSupreme)
	full := f.maxHP(t, f.toren)
	f.hurt(t, f.toren, full-3)
	if res := f.mustUse(t, f.caio, f.toren, potionSupreme); res.GetHealed() != 3 || f.vitals(t, f.toren).GetHitPointsCurrent() != full {
		t.Errorf("the supreme potion healed %d, want 3 up to %d", res.GetHealed(), full)
	}
}

func TestAResistancePotionInACombatIsAnEffectOfTheReceiverForTheHourInRounds(t *testing.T) {
	t.Parallel()
	f := newPotionFight(t, potionFire)
	f.mustUse(t, f.caio, f.toren, potionFire, giveTo(f.pens))
	var ends int32
	if err := f.h.pool.QueryRow(t.Context(), `SELECT ends_round FROM combatant_states WHERE combatant_id = $1 AND source_key = $2`, f.id(t, "Pensantus"), potionFire).Scan(&ends); err != nil {
		t.Fatalf("read the effect: %v", err)
	}
	if want := f.e.GetRound() + 600; ends != want {
		t.Errorf("the effect ends in round %d, want %d", ends, want)
	}
}

func TestAConsumableEventClosesTheUndoChainAndItsRetryIsTheSameUse(t *testing.T) {
	t.Parallel()
	f := newPotionFight(t, potionCommon)
	f.hurt(t, f.toren, 1)
	if _, err := f.adjustHP(t, f.e, "Goblin", healHP(1)); err != nil {
		t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
	}
	if f.log(t, f.master, f.e).GetUndoableEventId() == "" {
		t.Fatal("the heal of the goblin is not undoable")
	}
	key, entry := newKey(), f.entryOf(t, f.toren, potionCommon)
	same := func(r *playv1.UseConsumableRequest) { r.IdempotencyKey, r.EntryId = key, entry }
	first := f.mustUse(t, f.caio, f.toren, potionCommon, same)
	again := f.mustUse(t, f.caio, f.toren, potionCommon, same)
	if again.GetHealed() != first.GetHealed() || again.GetEntryId() != first.GetEntryId() || f.countEvents(t, eventConsumableUsed) != 1 {
		t.Errorf("the retry answered %v after %v", again, first)
	}
	if got := f.log(t, f.master, f.e).GetUndoableEventId(); got != "" {
		t.Errorf("the undoable event = %q after the drink, want none", got)
	}
}

// The table reads who used which item on whom; the dice and the hit points are the master's and the two owners'.
func TestTheLineOfAGiftTellsTheTableTheItemAndTheOwnersTheNumbers(t *testing.T) {
	t.Parallel()
	f := newPotionFight(t, potionCommon)
	f.hurt(t, f.pens, 1)
	f.mustUse(t, f.caio, f.toren, potionCommon, giveTo(f.pens))
	for name, u := range map[string]*user{"the master": f.master, "the giver's owner": f.caio, "the receiver's owner": f.ana} {
		l := f.line(t, u).GetConsumableUse()
		if l == nil || l.GetHealed() == 0 || l.GetDiceText() == "" || len(l.GetFaces()) == 0 || l.GetRollTotal() == 0 || l.GetTargetLabel() != "Pensantus" {
			t.Errorf("%s reads %v, want the numbers and the receiver", name, l)
		}
	}
	l := f.line(t, f.bia).GetConsumableUse()
	if l == nil || l.GetItemNamePt() == "" || l.GetTargetLabel() != "Pensantus" {
		t.Fatalf("another player reads %v, want the item and the receiver", l)
	}
	if l.GetHealed() != 0 || l.GetDiceText() != "" || len(l.GetFaces()) != 0 || l.GetRollTotal() != 0 {
		t.Errorf("another player reads numbers: %v", l)
	}
	if l.GetKind() != charactersv1.ConsumableKind_CONSUMABLE_KIND_POTION || l.GetEffectDurationTextPt() != "" {
		t.Errorf("another player reads kind %v and length %q of a heal, want the potion and no length", l.GetKind(), l.GetEffectDurationTextPt())
	}
}

// The line of a resistance potion says the kind and the length of the effect to the whole table, from the catalog; an oil says its kind.
func TestTheLineOfAResistancePotionSaysItsKindAndLengthToTheTable(t *testing.T) {
	t.Parallel()
	f := newPotionFight(t, potionFire, oilSharpness)
	f.mustUse(t, f.caio, f.toren, potionFire, giveTo(f.pens))
	for name, u := range map[string]*user{"the master": f.master, "another player": f.bia} {
		l := f.line(t, u).GetConsumableUse()
		if l.GetKind() != charactersv1.ConsumableKind_CONSUMABLE_KIND_POTION || l.GetEffectDurationTextPt() != "por 1 hora" {
			t.Errorf("%s reads kind %v and length %q, want the potion and %q", name, l.GetKind(), l.GetEffectDurationTextPt(), "por 1 hora")
		}
	}
	f.mustUse(t, f.caio, f.toren, oilSharpness)
	if l := f.line(t, f.bia).GetConsumableUse(); l.GetKind() != charactersv1.ConsumableKind_CONSUMABLE_KIND_OIL || l.GetEffectDurationTextPt() != "" {
		t.Errorf("the oil's line reads kind %v and length %q, want the oil and none", l.GetKind(), l.GetEffectDurationTextPt())
	}
}

func TestTheTargetsOfAGiftAreTheAdjacentCharacters(t *testing.T) {
	t.Parallel()
	f := newPotionFight(t, potionCommon)
	f.hurt(t, f.pens, 0)
	list := func(u *user, who string) ([]*playv1.ConsumableTarget, error) {
		res, err := u.charges.ListConsumableTargets(t.Context(), connect.NewRequest(&playv1.ListConsumableTargetsRequest{
			CampaignId: f.campaignID, CharacterId: who, EntryId: f.entryOf(t, f.toren, potionCommon),
		}))
		if err != nil {
			return nil, err
		}
		return res.Msg.GetTargets(), nil
	}
	for _, u := range []*user{f.caio, f.master} {
		got, err := list(u, f.toren.GetId())
		if err != nil || len(got) != 1 || got[0].GetCharacterId() != f.pens.GetId() || got[0].GetDistanceFt() != 5 ||
			got[0].GetState() != playv1.ConsumableTargetState_CONSUMABLE_TARGET_STATE_DOWN || !got[0].GetMayReceive() {
			t.Errorf("targets = %v, %v; want Pensantus, 5 ft, down", got, err)
		}
	}
	if _, err := list(f.bia, f.toren.GetId()); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("another player's list = %v, want permission_denied", err)
	}
}

func TestTheTargetsOutOfACombatAreTheOtherCharactersOfTheParty(t *testing.T) {
	t.Parallel()
	s := newStocked(t, potionCommon)
	s.hurt(t, s.pens, 0)
	res, err := s.caio.charges.ListConsumableTargets(t.Context(), connect.NewRequest(&playv1.ListConsumableTargetsRequest{
		CampaignId: s.campaignID, CharacterId: s.toren.GetId(), EntryId: s.entryOf(t, s.toren, potionCommon),
	}))
	if err != nil {
		t.Fatalf("ListConsumableTargets() error = %v", err)
	}
	if got := targetIDs(res.Msg.GetTargets()); !sameIDs(got, s.pens.GetId(), s.bri.GetId()) {
		t.Fatalf("targets = %v; want Pensantus %s and Brisa %s", got, s.pens.GetId(), s.bri.GetId())
	}
	for _, g := range res.Msg.GetTargets() {
		if g.GetCharacterId() == s.toren.GetId() || g.GetDistanceFt() != 0 || !g.GetMayReceive() {
			t.Errorf("target %v: want another character, no distance", g)
		}
		if g.GetState() != playv1.ConsumableTargetState_CONSUMABLE_TARGET_STATE_UNSPECIFIED {
			t.Errorf("target %v: the hit points of another character are no one's to read outside a combat (RN-20)", g)
		}
	}
	_, err = s.caio.charges.ListConsumableTargets(t.Context(), connect.NewRequest(&playv1.ListConsumableTargetsRequest{
		CampaignId: s.campaignID, CharacterId: s.toren.GetId(), EntryId: newKey(),
	}))
	wantBlockedBy(t, "an entry that is gone", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ITEM_GONE)
}

// Activating a potion takes an action, which is not the Use an Object of Haste's extra action: with the action used a hasted
// creature is refused, and the extra action stays free.
func TestAPotionUnderHasteIsNotPaidByTheExtraAction(t *testing.T) {
	t.Parallel()
	f := newPotionFight(t, potionCommon, potionGreater)
	f.mustAddEffect(t, f.e, "spell:haste", []string{"Toren"}, f.rounds(t, 3, "Toren"))
	f.mustUse(t, f.caio, f.toren, potionCommon) // the action
	_, err := f.use(t, f.caio, f.toren, potionGreater, giveTo(f.pens))
	wantBlockedBy(t, "a second potion under Haste", err, blockedActionUsed)
	var action, extra bool
	if err := f.h.pool.QueryRow(t.Context(), `SELECT action_used, extra_action_used FROM combatants WHERE id = $1`, f.id(t, "Toren")).Scan(&action, &extra); err != nil {
		t.Fatalf("read the economy of Toren: %v", err)
	}
	if !action || extra {
		t.Errorf("action used = %v, extra = %v; want the action spent and the extra one free", action, extra)
	}
	if !f.holds(t, f.toren, potionGreater) {
		t.Error("the refused potion was spent")
	}
}

// A receiver of the campaign that does not fight in the active combat is out of the giver's reach: nothing is spent, no action either.
func TestAPotionIsNotGivenToACharacterThatDoesNotFightInTheCombat(t *testing.T) {
	t.Parallel()
	s := newStocked(t, potionCommon)
	e := s.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: s.goblin.GetId()}},
		npcRolls: []int{3},
		players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1},
		reveal:   []string{"Goblin"},
		at:       map[string][2]int32{"Toren": {3, 3}, "Pensantus": {4, 4}, "Brisa": {9, 8}, "Goblin": {12, 8}},
		setup:    true,
	})
	// Brisa leaves the combat before it begins: she is still a character of the campaign, but no combatant of the fight.
	if _, err := s.master.combat.RemoveCombatant(t.Context(), connect.NewRequest(&playv1.RemoveCombatantRequest{
		CampaignId: s.campaignID, EncounterId: e.GetId(), CombatantId: s.id(t, "Brisa"), IdempotencyKey: newKey(),
	})); err != nil {
		t.Fatalf("RemoveCombatant(Brisa) error = %v", err)
	}
	if _, err := s.master.combat.BeginCombat(t.Context(), connect.NewRequest(&playv1.BeginCombatRequest{CampaignId: s.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey()})); err != nil {
		t.Fatalf("BeginCombat() error = %v", err)
	}
	f := &potionFight{stocked: s, e: e}
	_, err := f.use(t, f.caio, f.toren, potionCommon, giveTo(f.bri))
	wantBlockedBy(t, "a gift to a character outside the combat", err, blockedNotAdjacent)
	if !f.holds(t, f.toren, potionCommon) {
		t.Error("the refused gift spent the potion")
	}
	f.mustUse(t, f.caio, f.toren, potionCommon, giveTo(f.pens)) // the refusal spent no action
}

// A creature with three failed death saves is dead, whether or not the master confirmed it: a healing potion does not raise it, the
// refusal spends nothing, and the list of receivers leaves it out.
func TestAHealingPotionDoesNotReviveACreatureWithThreeFailedDeathSaves(t *testing.T) {
	t.Parallel()
	f := newPotionFight(t, potionCommon)
	f.hurt(t, f.pens, 0)
	f.execSQL(t, `UPDATE combatants SET death_failures = 3 WHERE id = $1`, f.id(t, "Pensantus"))
	_, err := f.use(t, f.caio, f.toren, potionCommon, giveTo(f.pens))
	wantBlockedBy(t, "a potion to the dead", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TARGET_DEAD)
	if !f.holds(t, f.toren, potionCommon) || f.vitals(t, f.pens).GetHitPointsCurrent() != 0 || byLabel(t, f.get(t, f.master), "Toren").GetActionUsed() {
		t.Error("the refused potion was spent, healed the dead or used the action")
	}
	got := f.targets(t, f.caio, f.toren, potionCommon)
	if len(got) != 1 || got[0].GetCharacterId() != f.pens.GetId() || got[0].GetState() != playv1.ConsumableTargetState_CONSUMABLE_TARGET_STATE_DOWN {
		t.Errorf("a player's targets = %v, want Pensantus down, the word the order of the combat gives until the master confirms", got)
	}
	if got := f.targets(t, f.master, f.toren, potionCommon); len(got) != 0 {
		t.Errorf("the master's targets = %v, want the creature left out", got)
	}
}

// A creature with three failed death saves is dead whatever the item: a scroll given by a character that does not fight to a
// combatant in that state is refused too, with the scroll still on the giver's sheet and no line in the log.
func TestAScrollGivenByAUserOutsideTheCombatIsRefusedForADeadCombatant(t *testing.T) {
	t.Parallel()
	f := fightWithout(t, []string{scrollFirst}, "Brisa")
	f.hurt(t, f.pens, 0)
	f.execSQL(t, `UPDATE combatants SET death_failures = 3 WHERE id = $1`, f.id(t, "Pensantus"))
	_, err := f.use(t, f.bia, f.bri, scrollFirst, giveTo(f.pens))
	wantBlockedBy(t, "a scroll to the dead", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TARGET_DEAD)
	if !f.holds(t, f.bri, scrollFirst) {
		t.Error("the refused scroll was spent")
	}
	if f.line(t, f.master) != nil {
		t.Error("the refused scroll wrote a line in the combat's log")
	}
}

// A combat in setup has its combatants but no turn: a resistance potion is their effect (the record lives on the combatant, as for
// any character that joined), and nothing is gated.
func TestAResistancePotionInACombatInSetupIsAnEffectOfTheCombatantWithNoTurnAndNoAction(t *testing.T) {
	t.Parallel()
	s := newStocked(t, potionFire)
	e := s.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: s.goblin.GetId()}},
		npcRolls: []int{3},
		players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1},
		reveal:   []string{"Goblin"},
		at:       map[string][2]int32{"Toren": {3, 3}, "Pensantus": {4, 4}, "Brisa": {9, 8}, "Goblin": {12, 8}},
		setup:    true,
	})
	f := &potionFight{stocked: s, e: e}
	f.mustUse(t, f.caio, f.toren, potionFire)                   // the drinker's own
	f.mustUse(t, f.master, f.pens, potionFire, giveTo(f.toren)) // a gift, the master giving Pensantus's potion to Toren
	var onCombatants, onCharacters int
	if err := f.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM combatant_states WHERE source_key = $1`, potionFire).Scan(&onCombatants); err != nil {
		t.Fatalf("read the effects of the combatants: %v", err)
	}
	if err := f.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM character_effects WHERE source_key = $1`, potionFire).Scan(&onCharacters); err != nil {
		t.Fatalf("read the effects of the characters: %v", err)
	}
	if onCombatants != 1 || onCharacters != 0 { // the second potion of the same kind on Toren ends the first
		t.Errorf("effects on combatants = %d, on characters = %d; want 1 and 0", onCombatants, onCharacters)
	}
	var spent bool
	if err := f.h.pool.QueryRow(t.Context(), `SELECT action_used FROM combatants WHERE id = $1`, f.id(t, "Toren")).Scan(&spent); err != nil || spent {
		t.Errorf("the action of Toren is used = %v (%v) in a combat that has not begun", spent, err)
	}
}

// Only a potion costs the turn and the action: any other consumable is spent and logged on any turn, with the action used or not.
// scrollFirst is a scroll, which the app does not cast, and oilSharpness is an oil, which the SRD files with the potions but
// applies in a minute or ten: the master judges its time, and it is no action of the turn.
func TestAConsumableThatIsNoPotionNeedsNeitherTheTurnNorTheAction(t *testing.T) {
	t.Parallel()
	f := newPotionFight(t, potionCommon, scrollFirst, oilSharpness)
	f.mustUse(t, f.caio, f.toren, potionCommon) // the action of Toren's turn
	// the action of the turn is used now, and a scroll or an oil is still spent and logged
	for _, key := range []string{scrollFirst, oilSharpness} {
		if text := f.mustUse(t, f.caio, f.toren, key); text.GetOutcome() != playv1.ConsumableUseOutcome_CONSUMABLE_USE_OUTCOME_TEXT_ONLY || len(text.GetText()) == 0 {
			t.Errorf("%s with the action used = %v, want its text", key, text)
		}
		res := f.mustUse(t, f.ana, f.pens, key, giveTo(f.toren)) // not Pensantus's turn, and Toren is another's
		if res.GetOutcome() != playv1.ConsumableUseOutcome_CONSUMABLE_USE_OUTCOME_TEXT_ONLY || len(res.GetText()) == 0 {
			t.Errorf("%s given off turn = %v, want its text", key, res)
		}
		if f.holds(t, f.toren, key) || f.holds(t, f.pens, key) {
			t.Errorf("a unit of %s was not spent", key)
		}
	}
	if byLabel(t, f.get(t, f.master), "Pensantus").GetActionUsed() {
		t.Error("the scroll or the oil used Pensantus's action")
	}
	l := f.line(t, f.bia).GetConsumableUse()
	if l == nil || l.GetItemNamePt() == "" {
		t.Errorf("the table reads %v, want the line of the use", l)
	}
}

// The list of an oil is the one of any consumable that is no potion: no distance and no turn.
func TestTheTargetsOfAnOilAreTheOtherCharactersOfTheParty(t *testing.T) {
	t.Parallel()
	f := newPotionFight(t, oilSharpness)
	if got := targetIDs(f.targets(t, f.caio, f.toren, oilSharpness)); !sameIDs(got, f.pens.GetId(), f.bri.GetId()) {
		t.Errorf("targets of an oil = %v, want Pensantus %s and Brisa %s", got, f.pens.GetId(), f.bri.GetId())
	}
}

// A potion costs the action whether or not the app applies it: potionClimb is a potion that is only text, and it is gated like a
// healing potion.
func TestATextOnlyPotionIsRefusedOffTurnAndWithTheActionUsedAndSpentOnTheTurn(t *testing.T) {
	t.Parallel()
	f := newPotionFight(t, potionCommon, potionClimb)
	_, err := f.use(t, f.ana, f.pens, potionClimb)
	wantBlockedBy(t, "a text-only potion off turn", err, blockedNotYourTurn)
	if !f.holds(t, f.pens, potionClimb) {
		t.Error("the refused drink off turn spent the potion")
	}
	f.mustUse(t, f.caio, f.toren, potionCommon) // the action of Toren's turn
	_, err = f.use(t, f.caio, f.toren, potionClimb)
	wantBlockedBy(t, "a text-only potion with the action used", err, blockedActionUsed)
	if !f.holds(t, f.toren, potionClimb) {
		t.Error("the refused drink with the action used spent the potion")
	}
	if f.line(t, f.master).GetConsumableUse().GetOutcome() == playv1.ConsumableUseOutcome_CONSUMABLE_USE_OUTCOME_TEXT_ONLY {
		t.Error("a refused text-only potion wrote a line in the log")
	}
}

func TestATextOnlyPotionOnTheTurnIsSpentLoggedAsTextAndPaysTheAction(t *testing.T) {
	t.Parallel()
	f := newPotionFight(t, potionClimb, potionCommon)
	res := f.mustUse(t, f.caio, f.toren, potionClimb)
	if res.GetOutcome() != playv1.ConsumableUseOutcome_CONSUMABLE_USE_OUTCOME_TEXT_ONLY || len(res.GetText()) == 0 || f.holds(t, f.toren, potionClimb) {
		t.Errorf("the text-only drink = %v, want its text and the unit spent", res)
	}
	if !byLabel(t, f.get(t, f.master), "Toren").GetActionUsed() {
		t.Error("the text-only potion left the action free")
	}
	l := f.line(t, f.bia).GetConsumableUse()
	if l == nil || l.GetItemNamePt() == "" || l.GetOutcome() != playv1.ConsumableUseOutcome_CONSUMABLE_USE_OUTCOME_TEXT_ONLY {
		t.Errorf("the table reads %v, want the text-only line of the use", l)
	}
	_, err := f.use(t, f.caio, f.toren, potionCommon)
	wantBlockedBy(t, "a drink after the text-only one", err, blockedActionUsed)
}

// The list of receivers of a text-only potion is the one its use accepts: the characters within 5 feet of the giver.
func TestTheTargetsOfATextOnlyPotionInACombatAreTheAdjacentCharacters(t *testing.T) {
	t.Parallel()
	f := newPotionFight(t, potionClimb)
	got := f.targets(t, f.caio, f.toren, potionClimb)
	if len(got) != 1 || got[0].GetCharacterId() != f.pens.GetId() {
		t.Errorf("targets = %v, want Pensantus only", got)
	}
}

// The effect of a potion is the item's, not the master's: the record says so, and the card its player reads names the potion.
func TestThePotionsResistanceIsAnEffectOfTheItemWhoseCardNamesThePotion(t *testing.T) {
	t.Parallel()
	f := newPotionFight(t, potionFire)
	f.mustUse(t, f.caio, f.toren, potionFire, giveTo(f.pens))
	var kind string
	if err := f.h.pool.QueryRow(t.Context(), `SELECT source_kind FROM combatant_states WHERE combatant_id = $1 AND source_key = $2`, f.id(t, "Pensantus"), potionFire).Scan(&kind); err != nil || kind != "item" {
		t.Errorf("the effect on the combatant is of the kind %q (%v), want item", kind, err)
	}
	for name, u := range map[string]*user{"the receiver's owner": f.ana, "the master": f.master} {
		cards := byLabel(t, f.get(t, u), "Pensantus").GetEffects()
		if len(cards) != 1 || cards[0].GetSourceKind() != playv1.EffectSourceKind_EFFECT_SOURCE_KIND_ITEM || !strings.Contains(cards[0].GetOriginPt(), cards[0].GetSourceNamePt()) || cards[0].GetSourceNamePt() == "" {
			t.Errorf("%s reads %v, want one card of an item whose origin names the potion", name, cards)
		}
		if len(cards) == 1 && strings.Contains(cards[0].GetOriginPt(), "mestre") {
			t.Errorf("%s reads the origin %q, want the potion's, not the master's", name, cards[0].GetOriginPt())
		}
	}
}

// fightWithout starts a combat of the three players and a goblin, in setup, takes the named characters out of it and begins it:
// they stay characters of the campaign that do not fight.
func fightWithout(t *testing.T, keys []string, out ...string) *potionFight {
	t.Helper()
	s := newStocked(t, keys...)
	s.bri = s.withItems(t, s.bri, keys...)
	e := s.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: s.goblin.GetId()}},
		npcRolls: []int{3},
		players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1},
		reveal:   []string{"Goblin"},
		at:       map[string][2]int32{"Toren": {3, 3}, "Pensantus": {4, 4}, "Brisa": {9, 8}, "Goblin": {12, 8}},
		setup:    true,
	})
	for _, label := range out {
		if _, err := s.master.combat.RemoveCombatant(t.Context(), connect.NewRequest(&playv1.RemoveCombatantRequest{
			CampaignId: s.campaignID, EncounterId: e.GetId(), CombatantId: s.id(t, label), IdempotencyKey: newKey(),
		})); err != nil {
			t.Fatalf("RemoveCombatant(%s) error = %v", label, err)
		}
	}
	if _, err := s.master.combat.BeginCombat(t.Context(), connect.NewRequest(&playv1.BeginCombatRequest{CampaignId: s.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey()})); err != nil {
		t.Fatalf("BeginCombat() error = %v", err)
	}
	return &potionFight{stocked: s, e: e}
}

// sameIDs says whether got holds exactly the wanted ids, in any order.
func sameIDs(got []string, want ...string) bool {
	return slices.Equal(slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(want)))
}

func targetIDs(ts []*playv1.ConsumableTarget) []string {
	var out []string
	for _, g := range ts {
		out = append(out, g.GetCharacterId())
	}
	return out
}

// A user that does not fight in the running combat may give a potion only to a character that does not fight either (the
// combat's receivers are out of its reach), and any other consumable to the whole party: the list offers who the use accepts.
func TestTheTargetsOfAUserOutsideTheRunningCombatAreTheCharactersOutsideIt(t *testing.T) {
	t.Parallel()
	f := fightWithout(t, []string{potionCommon, scrollFirst}, "Brisa")
	for _, u := range []*user{f.bia, f.master} {
		if got := f.targets(t, u, f.bri, potionCommon); len(got) != 0 {
			t.Errorf("targets of a potion = %v, want none: Toren and Pensantus fight", got)
		}
	}
	_, err := f.use(t, f.bia, f.bri, potionCommon, giveTo(f.pens))
	wantBlockedBy(t, "a potion to a fighter", err, blockedNotAdjacent)
	if got := targetIDs(f.targets(t, f.bia, f.bri, scrollFirst)); !sameIDs(got, f.toren.GetId(), f.pens.GetId()) {
		t.Errorf("targets of a scroll = %v, want Toren %s and Pensantus %s: no gate for a text-only consumable", got, f.toren.GetId(), f.pens.GetId())
	}
	f.mustUse(t, f.bia, f.bri, scrollFirst, giveTo(f.pens))
}

func TestTheTargetsOfAPotionFromOutsideTheCombatIncludeWhoDoesNotFightEither(t *testing.T) {
	t.Parallel()
	f := fightWithout(t, []string{potionCommon}, "Pensantus", "Brisa")
	if got := targetIDs(f.targets(t, f.bia, f.bri, potionCommon)); len(got) != 1 || got[0] != f.pens.GetId() {
		t.Errorf("targets = %v, want Pensantus only", got)
	}
	if got := f.targets(t, f.caio, f.toren, potionCommon); len(got) != 0 {
		t.Errorf("Toren's targets = %v, want none: the others do not fight, out of his reach", got)
	}
	f.mustUse(t, f.bia, f.bri, potionCommon, giveTo(f.pens))
}

// An NPC is no player's combatant, so the turn and the action of the combat gate no use of an NPC's potion: the master's own use
// of it is spent and logged on any turn with no effect, and a potion of an NPC given to a player's character in a running combat
// is refused as out of reach. The combat's line and its action economy are the player characters'.
func TestAnNPCsPotionInARunningCombatIsNotGatedByTheTurnNorTheAction(t *testing.T) {
	t.Parallel()
	s := newStocked(t, potionCommon)
	mage := s.withItems(t, s.master.npcWizard(t, s.campaignID, "Mago Rival"), potionCommon, potionCommon)
	e := s.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: mage.GetId()}},
		npcRolls: []int{3},
		players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1},
		reveal:   []string{"Mago Rival"},
		at:       map[string][2]int32{"Toren": {3, 3}, "Pensantus": {4, 4}, "Brisa": {9, 8}, "Mago Rival": {3, 4}},
	})
	f := &potionFight{stocked: s, e: e}
	res, err := f.use(t, f.master, mage, potionCommon)
	if err != nil || res.GetOutcome() != playv1.ConsumableUseOutcome_CONSUMABLE_USE_OUTCOME_TEXT_ONLY {
		t.Fatalf("the NPC's own potion off its turn = %v, %v; want it spent with no effect", res, err)
	}
	if byLabel(t, f.get(t, f.master), "Mago Rival").GetActionUsed() || f.line(t, f.master) != nil {
		t.Error("the NPC's potion used the NPC's action or wrote a line in the combat's log")
	}
	_, err = f.use(t, f.master, mage, potionCommon, giveTo(f.toren))
	wantBlockedBy(t, "an NPC's potion given to a combatant", err, blockedNotAdjacent)
	if !f.holds(t, mage, potionCommon) {
		t.Error("the refused gift spent the potion")
	}
}

// The list follows the word of state the order of the combat gives every player: a creature with three failed death saves that
// the master has not confirmed reads "down" to a player, as any down ally does, and the use is still refused before anything is
// spent. The master's list leaves such a creature out, and a confirmed death is left out of every list.
func TestTheTargetsFollowTheStateWordThePlayerReads(t *testing.T) {
	t.Parallel()
	f := newPotionFight(t, potionCommon)
	f.hurt(t, f.pens, 0)
	f.execSQL(t, `UPDATE combatants SET death_failures = 3 WHERE id = $1`, f.id(t, "Pensantus"))
	for name, u := range map[string]*user{"the giver's player": f.caio, "the master": f.master} {
		got := f.targets(t, u, f.toren, potionCommon)
		if name == "the master" {
			if len(got) != 0 {
				t.Errorf("%s reads %v, want none", name, got)
			}
			continue
		}
		if len(got) != 1 || got[0].GetCharacterId() != f.pens.GetId() || got[0].GetState() != playv1.ConsumableTargetState_CONSUMABLE_TARGET_STATE_DOWN || got[0].GetDistanceFt() != 5 {
			t.Errorf("%s reads %v, want Pensantus, 5 ft, down", name, got)
		}
	}
	_, err := f.use(t, f.caio, f.toren, potionCommon, giveTo(f.pens))
	wantBlockedBy(t, "a potion to a creature the list showed down", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TARGET_DEAD)
	f.execSQL(t, `UPDATE combatants SET defeated = true WHERE id = $1`, f.id(t, "Pensantus"))
	if got := f.targets(t, f.caio, f.toren, potionCommon); len(got) != 0 {
		t.Errorf("after the master confirmed it, a player reads %v, want none", got)
	}
}

// A consumable that is not a potion is no action of the combat, even while the combat runs: its list is the party, with no state
// word and no distance. A combatant with three failed death saves the master has not confirmed is on a player's list as any other,
// a confirmed death is on nobody's, and the master's list leaves the first out.
func TestTheTargetsOfAScrollInARunningCombatFollowTheStateWordThePlayerReads(t *testing.T) {
	t.Parallel()
	f := newPotionFight(t, scrollFirst)
	f.hurt(t, f.pens, 0)
	f.execSQL(t, `UPDATE combatants SET death_failures = 3 WHERE id = $1`, f.id(t, "Pensantus"))
	f.execSQL(t, `UPDATE combatants SET defeated = true WHERE id = $1`, f.id(t, "Brisa"))
	got := f.targets(t, f.caio, f.toren, scrollFirst)
	if len(got) != 1 || got[0].GetCharacterId() != f.pens.GetId() || got[0].GetState() != playv1.ConsumableTargetState_CONSUMABLE_TARGET_STATE_UNSPECIFIED {
		t.Errorf("a player's targets = %v, want Pensantus alone with no state", got)
	}
	if got := f.targets(t, f.master, f.toren, scrollFirst); len(got) != 0 {
		t.Errorf("the master's targets = %v, want none", got)
	}
	_, err := f.use(t, f.caio, f.toren, scrollFirst, giveTo(f.pens))
	wantBlockedBy(t, "a scroll to a creature the list showed", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TARGET_DEAD)
}

// wantTargets is the exact list a user reads: the characters in order, each at no distance, with no state word (the table does not
// read one outside a running combat) and able to receive.
func wantTargets(t *testing.T, who string, got []*playv1.ConsumableTarget, want ...*charactersv1.Character) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s reads %v, want %d targets", who, got, len(want))
	}
	for i, c := range want {
		g := got[i]
		if g.GetCharacterId() != c.GetId() || g.GetName() != c.GetName() || g.GetDistanceFt() != 0 || !g.GetMayReceive() ||
			g.GetState() != playv1.ConsumableTargetState_CONSUMABLE_TARGET_STATE_UNSPECIFIED {
			t.Errorf("%s reads %v at %d, want %s (%s), 0 ft, no state, may receive", who, g, i, c.GetName(), c.GetId())
		}
	}
}

// fourthHero is a fourth character of the party, a wizard of a fourth player who joins the campaign, made before the combat starts.
func (s *stocked) fourthHero(t *testing.T, name string) *charactersv1.Character {
	t.Helper()
	fourth := s.h.newUser("Duda")
	s.h.join(s.master, s.campaignID, fourth)
	return fourth.hero(t, s.campaignID, name, "class:wizard", "race:gnome", 1,
		&rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8}, nil, []string{fireBolt})
}

// At a table that hides the death saves a potion or a scroll given to a creature with three failed death saves is refused as dead
// for every reader all the same, and spends nothing: trying to heal is how the table learns.
func TestAGiftToThreeFailedDeathSavesIsRefusedAtATableThatHidesThem(t *testing.T) {
	t.Parallel()
	f := newPotionFight(t, potionCommon, scrollFirst)
	f.setRules(t, hiddenDeath)
	f.hurt(t, f.pens, 0)
	f.execSQL(t, `UPDATE combatants SET death_failures = 3 WHERE id = $1`, f.id(t, "Pensantus"))
	for _, key := range []string{potionCommon, scrollFirst} {
		_, err := f.use(t, f.caio, f.toren, key, giveTo(f.pens))
		wantBlockedBy(t, key+" to the creature with three failed death saves", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TARGET_DEAD)
		if !f.holds(t, f.toren, key) {
			t.Errorf("the refused %s was spent", key)
		}
	}
}

// A creature with three failed death saves that is above 0 hit points (an exhaustion of level 6 sets the failures and keeps the hit
// points) is on its feet in the order, so a player's list says so too, and the use is refused as dead.
func TestTheTargetsSayOnItsFeetForThreeFailuresAboveZeroHitPoints(t *testing.T) {
	t.Parallel()
	f := newPotionFight(t, potionCommon)
	f.execSQL(t, `UPDATE combatants SET death_failures = 3 WHERE id = $1`, f.id(t, "Pensantus"))
	got := f.targets(t, f.caio, f.toren, potionCommon)
	if len(got) != 1 || got[0].GetCharacterId() != f.pens.GetId() || got[0].GetState() != playv1.ConsumableTargetState_CONSUMABLE_TARGET_STATE_OK || got[0].GetDistanceFt() != 5 {
		t.Errorf("a player reads %v, want Pensantus, 5 ft, on its feet", got)
	}
	_, err := f.use(t, f.caio, f.toren, potionCommon, giveTo(f.pens))
	wantBlockedBy(t, "a potion to the creature with three failed death saves", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TARGET_DEAD)
}

// While the combat is in setup it has no turn and no distance: the list of an item of Toren is the party's characters that are not
// dead. A creature with three failed death saves (on its combatant) that the master has not confirmed reads as a down ally to a
// player, with no state word as for every target outside a running combat, and the master's list leaves it out; a creature the
// character sheet marks dead is on nobody's list.
func TestTheTargetsInACombatInSetupFollowTheStateWordThePlayerReads(t *testing.T) {
	t.Parallel()
	for _, key := range []string{potionCommon, scrollFirst} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			s := newStocked(t, key)
			fallen := s.fourthHero(t, "Orvalho")
			p := plan{
				npcs:     []*playv1.Participant{{CharacterId: s.goblin.GetId()}},
				npcRolls: []int{3},
				players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1, "Orvalho": 5},
				reveal:   []string{"Goblin"},
				at:       map[string][2]int32{"Toren": {3, 3}, "Pensantus": {4, 4}, "Brisa": {9, 8}, "Orvalho": {5, 5}, "Goblin": {12, 8}},
				setup:    true,
			}
			f := &potionFight{stocked: s, e: s.start(t, p)}
			f.hurt(t, f.pens, 0) // three failed death saves, not confirmed
			f.execSQL(t, `UPDATE combatants SET death_failures = 3 WHERE id = $1`, f.id(t, "Pensantus"))
			f.hurt(t, f.bri, 0) // an ordinary down ally
			f.execSQL(t, `UPDATE characters SET status = 'dead', died_at = now() WHERE id = $1`, fallen.GetId())
			if enc := f.get(t, f.master); enc.GetStatus() != playv1.EncounterStatus_ENCOUNTER_STATUS_SETUP {
				t.Fatalf("the combat is %v, want it in setup", enc.GetStatus())
			}
			wantTargets(t, "a player's list", f.targets(t, f.caio, f.toren, key), f.pens, f.bri)
			wantTargets(t, "the master's list", f.targets(t, f.master, f.toren, key), f.bri)
			if key == potionCommon {
				_, err := f.use(t, f.caio, f.toren, key, giveTo(f.pens))
				wantBlockedBy(t, "a potion to the creature with three failed death saves", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TARGET_DEAD)
				if !f.holds(t, f.toren, key) {
					t.Error("the refused gift spent the potion")
				}
			}
		})
	}
}

// With no combat at all the list of an item is the party's characters that are not dead, whoever asks, with no state word: the
// death saves live only on the combatants of a combat (the combatants table), so there is no creature with three failed death
// saves to leave out of the master's list, and a character at 0 hit points is on every list. The code reads no death saves here:
// nothing but the sheet's status (dead is out of the party) tells a down character from a dead one.
func TestTheTargetsWithNoCombatAreThePartyNotDeadForEveryUser(t *testing.T) {
	t.Parallel()
	for _, key := range []string{potionCommon, scrollFirst} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			s := newStocked(t, key)
			fallen := s.fourthHero(t, "Orvalho")
			s.hurt(t, s.pens, 0)
			s.hurt(t, s.bri, 0)
			s.execSQL(t, `UPDATE characters SET status = 'dead', died_at = now() WHERE id = $1`, fallen.GetId())
			var combats int
			if err := s.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM encounters e JOIN game_sessions g ON g.id = e.game_session_id WHERE g.campaign_id = $1`, s.campaignID).Scan(&combats); err != nil || combats != 0 {
				t.Fatalf("encounters = %d, %v; want none", combats, err)
			}
			wantTargets(t, "a player's list", s.targets(t, s.caio, s.toren, key), s.pens, s.bri)
			wantTargets(t, "the master's list", s.targets(t, s.master, s.toren, key), s.pens, s.bri)
		})
	}
}

// failOpenSession is a database that answers every read of the open session with an error.
type failOpenSession struct{ playdb.DBTX }

func (f failOpenSession) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if strings.Contains(sql, "-- name: GetOpenGameSession :one") {
		return errRow{errors.New("the database is gone")}
	}
	return f.DBTX.QueryRow(ctx, sql, args...)
}

type errRow struct{ err error }

func (r errRow) Scan(...any) error { return r.err }

// A read of the open session that fails is an error of the call, not a campaign with no combat.
func TestTheTargetsAreAnErrorWhenTheOpenSessionCannotBeRead(t *testing.T) {
	t.Parallel()
	f := newPotionFight(t, potionCommon)
	entry := f.entryOf(t, f.toren, potionCommon)
	was := f.h.svc.queries
	f.h.svc.queries = playdb.New(failOpenSession{f.h.pool})
	t.Cleanup(func() { f.h.svc.queries = was })
	_, err := f.caio.charges.ListConsumableTargets(t.Context(), connect.NewRequest(&playv1.ListConsumableTargetsRequest{
		CampaignId: f.campaignID, CharacterId: f.toren.GetId(), EntryId: entry,
	}))
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Errorf("ListConsumableTargets() error = %v, want internal", err)
	}
}

// setupFight is the combat of potionFight before it begins: the combatants are placed and no turn is running.
func setupFight(t *testing.T, npc *charactersv1.Character, s *stocked) *potionFight {
	t.Helper()
	p := plan{
		npcs:     []*playv1.Participant{{CharacterId: s.goblin.GetId()}},
		npcRolls: []int{3},
		players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1},
		reveal:   []string{"Goblin"},
		at:       map[string][2]int32{"Toren": {3, 3}, "Pensantus": {4, 4}, "Brisa": {9, 8}, "Goblin": {12, 8}},
		setup:    true,
	}
	if npc != nil {
		p.npcs, p.reveal = []*playv1.Participant{{CharacterId: npc.GetId()}}, []string{npc.GetName()}
		delete(p.at, "Goblin")
		p.at[npc.GetName()] = [2]int32{3, 4}
	}
	return &potionFight{stocked: s, e: s.start(t, p)}
}

// A combat in setup has no turn: a healing potion heals the combatant it is given to, the unit is spent, and no action is used.
func TestAHealingPotionInACombatInSetupHealsTheCombatantAndSpendsNoAction(t *testing.T) {
	t.Parallel()
	f := setupFight(t, nil, newStocked(t, potionCommon))
	f.hurt(t, f.pens, 1)
	res := f.mustUse(t, f.caio, f.toren, potionCommon, giveTo(f.pens))
	if res.GetHealed() == 0 || f.holds(t, f.toren, potionCommon) || f.vitals(t, f.pens).GetHitPointsCurrent() <= 1 {
		t.Errorf("the gift = %v, want Pensantus healed and Toren's unit spent", res)
	}
	if byLabel(t, f.get(t, f.master), "Toren").GetActionUsed() {
		t.Error("a combat that has not begun had the action spent")
	}
}

// An NPC giving its potion to a player's character in a combat that has not begun is not out of reach: there is no turn and no
// distance, and the potion heals the receiver.
func TestAnNPCsPotionGivenInACombatInSetupHealsTheReceiver(t *testing.T) {
	t.Parallel()
	s := newStocked(t, potionCommon)
	mage := s.withItems(t, s.master.npcWizard(t, s.campaignID, "Mago Rival"), potionCommon)
	f := setupFight(t, mage, s)
	f.hurt(t, f.toren, 1)
	res, err := f.use(t, f.master, mage, potionCommon, giveTo(f.toren))
	if err != nil || res.GetHealed() == 0 || f.holds(t, mage, potionCommon) || f.vitals(t, f.toren).GetHitPointsCurrent() <= 1 {
		t.Errorf("the NPC's gift = %v, %v; want Toren healed and the NPC's unit spent", res, err)
	}
}
