package play

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The spells, the reactions, the fallen, the conditions and the class features
// (MR-012, MR-014, RN-02, RN-03, RN-18, RN-20, RN-22, Etapa 6, slice 6.4b).
// These tests need the database (MEURPG_TEST_DATABASE_URL). The fixture is the
// party of the canonical fight: Toren, a level 5 fighter (Extra Attack, Retomar
// o fôlego, Surto de ação); Pensantus, a level 3 wizard with four 1st-level
// slots and two 2nd-level ones; Brisa, a level 3 cleric with the same slots; the
// Capitão Goblin (27 PV, CA 18) and a goblin (7 PV, CA 12).

// spellRPCs are the CombatService methods of this slice; the authorization
// matrix of the encounter (combat_test.go) leaves them to this file's.
var spellRPCs = []string{"CastSpell", "UseReaction", "DeclineReaction", "RollDeathSave", "ConfirmDeath", "SetCombatantConditions", "EndConcentration"}

const (
	magicMissileSpell = "spell:magic-missile"
	shieldSpell       = "spell:shield"
	sleepSpell        = "spell:sleep"
	burningHands      = "spell:burning-hands"
	holdPerson        = "spell:hold-person"
	webSpell          = "spell:web"
	cureWounds        = "spell:cure-wounds"
	healingWord       = "spell:healing-word"
	guidingBolt       = "spell:guiding-bolt"
	sacredFlame       = "spell:sacred-flame"
	secondWindKey     = "feature:second-wind"
	actionSurgeKey    = "feature:action-surge-1-use"
	maceKey           = "equipment:mace"
)

// caster is a hero with spells: the sheet of a spellcaster.
func (u *user) caster(t *testing.T, campaignID, name, class, race string, level int32, scores *rulesv1.AbilityScores, weapons, cantrips, known, prepared []string) *charactersv1.Character {
	t.Helper()
	sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores: scores, RaceKey: race, Classes: []*charactersv1.ClassLevel{{ClassKey: class, Level: level}},
		WeaponKeys: weapons, CantripKeys: cantrips, KnownSpellKeys: known, PreparedSpellKeys: prepared,
	}}}
	res, err := u.characters.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: campaignID, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: name, Sheet: sheet,
	}))
	if err != nil {
		t.Fatalf("CreateCharacter(%s) error = %v", name, err)
	}
	return res.Msg.GetCharacter()
}

// newCasters is the fixture of this file: see the comment at the top.
func newCasters(t *testing.T) *armed { return newCastersAt(t, 3) }

// newCastersAt is newCasters with Pensantus at the given wizard level.
func newCastersAt(t *testing.T, wizardLevel int32) *armed {
	t.Helper()
	t.Helper()
	return newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 5,
			&rulesv1.AbilityScores{Strength: 16, Dexterity: 13, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{battleaxe}, nil)
		a.pens = a.ana.caster(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", wizardLevel,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8}, nil, []string{fireBolt},
			[]string{magicMissileSpell, shieldSpell, sleepSpell, burningHands, holdPerson, webSpell},
			[]string{magicMissileSpell, shieldSpell, sleepSpell, burningHands, holdPerson, webSpell})
		a.bri = a.bia.caster(t, a.campaignID, "Brisa", "class:cleric", "race:human", 3,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 16, Constitution: 14, Intelligence: 10, Wisdom: 16, Charisma: 8}, []string{maceKey}, []string{sacredFlame}, nil,
			[]string{cureWounds, healingWord, guidingBolt})
	})
}

func slotOfLevel(level int32) *playv1.SpellSlot { return &playv1.SpellSlot{Level: level} }

// at lists spell targets by label (a dart count after the label, if any).
func (a *armed) at(t *testing.T, labels ...string) []*playv1.SpellTarget {
	t.Helper()
	var out []*playv1.SpellTarget
	for _, l := range labels {
		out = append(out, &playv1.SpellTarget{CombatantId: a.id(t, l)})
	}
	return out
}

func darts(a *armed, t *testing.T, label string, n int32) *playv1.SpellTarget {
	t.Helper()
	return &playv1.SpellTarget{CombatantId: a.id(t, label), Darts: n}
}

func noCastRoll(*playv1.CastSpellRequest) {}

// cast calls CastSpell as u, with a key of its own.
func (a *armed) cast(t *testing.T, u *user, e *playv1.Encounter, caster, spell string, slot *playv1.SpellSlot, targets []*playv1.SpellTarget, roll func(*playv1.CastSpellRequest)) (*playv1.CastSpellResponse, error) {
	t.Helper()
	return a.castKey(t, u, e, caster, spell, slot, targets, roll, newKey())
}

func (a *armed) castKey(t *testing.T, u *user, e *playv1.Encounter, caster, spell string, slot *playv1.SpellSlot, targets []*playv1.SpellTarget, roll func(*playv1.CastSpellRequest), key string) (*playv1.CastSpellResponse, error) {
	t.Helper()
	req := &playv1.CastSpellRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CasterId: a.id(t, caster), SpellKey: spell, Slot: slot, Targets: targets, IdempotencyKey: key,
	}
	roll(req)
	res, err := u.combat.CastSpell(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (a *armed) mustCast(t *testing.T, u *user, e *playv1.Encounter, caster, spell string, slot *playv1.SpellSlot, targets []*playv1.SpellTarget, roll func(*playv1.CastSpellRequest)) *playv1.CastSpellResponse {
	t.Helper()
	res, err := a.cast(t, u, e, caster, spell, slot, targets, roll)
	if err != nil {
		t.Fatalf("CastSpell(%s, %s) error = %v", caster, spell, err)
	}
	return res
}

// correct is the master's correction of a character's vitals.
func (a *armed) correct(t *testing.T, c *charactersv1.Character, edit func(*playv1.AdjustCharacterVitalsRequest)) *playv1.CharacterVitals {
	t.Helper()
	req := &playv1.AdjustCharacterVitalsRequest{CampaignId: a.campaignID, CharacterId: c.GetId(), IdempotencyKey: newKey()}
	edit(req)
	res, err := a.master.play.AdjustCharacterVitals(t.Context(), connect.NewRequest(req))
	if err != nil {
		t.Fatalf("AdjustCharacterVitals() error = %v", err)
	}
	return res.Msg.GetVitals()
}

func hpIs(n int32) func(*playv1.AdjustCharacterVitalsRequest) {
	return func(r *playv1.AdjustCharacterVitalsRequest) { r.HitPointsCurrent = &n }
}

// usedSlots is how many slots of a level the character has spent.
func usedSlots(v *playv1.CharacterVitals, level int32) int32 {
	for _, s := range v.GetSpellSlots() {
		if s.GetLevel() == level {
			return s.GetUsed()
		}
	}
	return -1
}

func resourceUsed(v *playv1.CharacterVitals, key string) (used, total int32) {
	for _, r := range v.GetResources() {
		if r.GetKey() == key {
			return r.GetUsed(), r.GetTotal()
		}
	}
	return -1, -1
}

func spellOption(o *playv1.GetTurnOptionsResponse, key string) *rulesv1.SpellOption {
	for _, s := range o.GetOptions().GetSpells() {
		if s.GetSpell().GetKey() == key {
			return s
		}
	}
	return nil
}

func spellTargetsOf(o *playv1.GetTurnOptionsResponse, key string) *playv1.SpellTargets {
	for _, s := range o.GetSpellTargets() {
		if s.GetSpellKey() == key {
			return s
		}
	}
	return nil
}

func combatantState(e *playv1.Encounter, label string) playv1.CombatantState {
	for _, c := range e.GetCombatants() {
		if c.GetLabel() == label {
			return c.GetState()
		}
	}
	return playv1.CombatantState_COMBATANT_STATE_UNSPECIFIED
}

// castersFight starts the combat of this file: Pensantus plays first (the one
// that casts), then Toren, Brisa, the goblins and the Capitão; everybody is on
// the map, the NPCs revealed. The NPC rolls are low, so they play last.
func (a *armed) castersFight(t *testing.T, goblins int32) *playv1.Encounter {
	t.Helper()
	npcs := []*playv1.Participant{{CharacterId: a.goblin.GetId(), Count: goblins}, {CharacterId: a.capitao.GetId()}}
	rolls := make([]int, 0, int(goblins)+1)
	for range goblins + 1 {
		rolls = append(rolls, 1)
	}
	reveal := []string{"Capitão Goblin"}
	// The Capitão stands next to Pensantus, with nobody between them: a creature
	// on the line between two others is half cover (D4), and these tests count on
	// the armor classes and the saves as they are.
	at := map[string][2]int32{"Pensantus": {5, 5}, "Toren": {6, 5}, "Brisa": {5, 6}, "Capitão Goblin": {4, 4}}
	if goblins != 1 {
		at["Toren"] = [2]int32{6, 6} // off Pensantus's line to the goblins: no cover for their saves
	}
	if goblins == 1 {
		reveal, at["Goblin"] = append(reveal, "Goblin"), [2]int32{7, 5}
	} else {
		for i := range goblins {
			label := fmt.Sprintf("Goblin %d", i+1)
			reveal, at[label] = append(reveal, label), [2]int32{7 + i/5, 3 + i%5}
		}
	}
	return a.start(t, plan{
		npcs: npcs, npcRolls: rolls, reveal: reveal, at: at,
		players: map[string]int32{"Pensantus": 20, "Toren": 15, "Brisa": 10},
	})
}

func TestMR014_CastingSpendsTheSlot(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)

	// Pensantus is on turn, with four 1st-level slots and two 2nd-level ones.
	opts := a.mustOptions(t, a.ana, e, "Pensantus")
	sleep := spellOption(opts, sleepSpell)
	if sleep == nil || !sleep.GetEnabled() || len(sleep.GetSlots()) != 2 || sleep.GetSlots()[0].GetFree() != 4 {
		t.Fatalf("Sono's option = %v, want enabled with two slot levels and four free 1st-level slots", sleep)
	}
	if st := spellTargetsOf(opts, sleepSpell); st == nil || st.GetMaxTargets() != 0 {
		t.Errorf("Sono's targets = %v, want an area (any number of targets)", st)
	}
	if shield := spellOption(opts, shieldSpell); shield == nil || shield.GetEnabled() || shield.GetReason().GetCode() != rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_REACTION_ONLY_WHEN_HIT {
		t.Errorf("Escudo's option = %v, want it disabled: only when hit", shield)
	}

	// The cast spends the slot and the action at once, and not again on a retry.
	key, before := newKey(), a.events(t)
	first, err := a.castKey(t, a.ana, e, "Pensantus", sleepSpell, slotOfLevel(1), a.at(t, "Goblin"), poolInApp, key)
	if err != nil {
		t.Fatalf("CastSpell(Sono) error = %v", err)
	}
	if got := usedSlots(a.vitals(t, a.pens), 1); got != 1 {
		t.Errorf("1st-level slots used = %d after the cast, want 1", got)
	}
	if c := byLabel(t, first.GetEncounter(), "Pensantus"); !c.GetActionUsed() || c.GetBonusActionUsed() {
		t.Errorf("economy after Sono = action %v, bonus %v; want the action spent", c.GetActionUsed(), c.GetBonusActionUsed())
	}
	if a.events(t) != before+1 {
		t.Fatalf("the cast wrote %d events, want 1", a.events(t)-before)
	}
	again, err := a.castKey(t, a.ana, e, "Pensantus", sleepSpell, slotOfLevel(1), a.at(t, "Goblin"), poolInApp, key)
	if err != nil {
		t.Fatalf("CastSpell(Sono) retry error = %v", err)
	}
	if got := usedSlots(a.vitals(t, a.pens), 1); got != 1 || a.events(t) != before+1 || again.GetCast().GetCastId() != first.GetCast().GetCastId() {
		t.Errorf("after the retry: %d slots used, %d events, cast %q (first %q); want nothing spent twice and the same answer",
			got, a.events(t)-before, again.GetCast().GetCastId(), first.GetCast().GetCastId())
	}
	// The one action is gone: another spell this turn is refused by the economy.
	_, err = a.cast(t, a.ana, e, "Pensantus", magicMissileSpell, slotOfLevel(1), []*playv1.SpellTarget{darts(a, t, "Goblin", 3)}, noCastRoll)
	wantBlockedBy(t, "a second cast in the same action", err, blockedActionUsed)
	if got := usedSlots(a.vitals(t, a.pens), 1); got != 1 {
		t.Errorf("a refused cast spent a slot: %d used", got)
	}

	// A cantrip with a saving throw is cast with no slot; a leveled spell needs one.
	a.mustEndTurn(t, a.ana, e) // Toren
	a.mustEndTurn(t, a.caio, e)
	// Brisa: Palavra de Cura is a bonus action: the action stays free.
	a.correct(t, a.bri, hpIs(10))
	heal := a.mustCast(t, a.bia, e, "Brisa", healingWord, slotOfLevel(2), a.at(t, "Toren"), noCastRoll)
	if c := byLabel(t, heal.GetEncounter(), "Brisa"); c.GetActionUsed() || !c.GetBonusActionUsed() {
		t.Errorf("economy after a bonus action spell = action %v, bonus %v; want only the bonus action spent", c.GetActionUsed(), c.GetBonusActionUsed())
	}
	_, err = a.cast(t, a.bia, e, "Brisa", healingWord, slotOfLevel(1), a.at(t, "Toren"), noCastRoll)
	wantBlockedBy(t, "a second bonus action", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_BONUS_ACTION_USED)
	_, err = a.cast(t, a.bia, e, "Brisa", sacredFlame, slotOfLevel(1), a.at(t, "Goblin"), noCastRoll)
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a cantrip with a slot: %v, want invalid_argument", err)
	}
	flame := a.mustCast(t, a.bia, e, "Brisa", sacredFlame, nil, a.at(t, "Goblin"), noCastRoll) // a cantrip: no slot, the action
	if got := usedSlots(a.vitals(t, a.bri), 2); got != 1 {
		t.Errorf("Brisa's 2nd-level slots used = %d, want 1 (the cantrip spent none)", got)
	}

	// The heal and the cantrip's damage wait for their rolls, which hold the turn.
	_, err = a.endTurn(t, a.bia, e, false)
	wantBlockedBy(t, "EndTurn with a damage to roll", err, blockedPendingDamage)
	a.h.roller.queue(3, 3, 3) // the 2d4 of the heal (the 2nd-level slot) and the 1d8 of the flame: the goblin survives
	healed := a.mustDamage(t, a.bia, e, heal.GetCast().GetPendingDamages()[0].GetId(), inAppDamage).GetPendingDamage()
	if healed.GetAmount() != 9 || !healed.GetHealing() || healed.GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED {
		t.Errorf("Palavra de Cura's heal = %v, want 2d4 (3, 3) + 3 = 9, applied at once", healed)
	}
	a.mustDamage(t, a.bia, e, flame.GetCast().GetPendingDamages()[0].GetId(), inAppDamage)

	// No free slot: NO_SLOT with the lowest level that would do.
	a.mustEndTurn(t, a.bia, e) // the goblin
	a.mustEndTurn(t, a.master, e)
	a.mustEndTurn(t, a.master, e) // the Capitão; round 2: Pensantus
	a.correct(t, a.pens, func(r *playv1.AdjustCharacterVitalsRequest) {
		r.SpellSlotsUsed = []*playv1.SpellSlotsUsed{{Level: 1, Used: 4}, {Level: 2, Used: 2}}
	})
	_, err = a.cast(t, a.ana, e, "Pensantus", burningHands, slotOfLevel(1), a.at(t, "Goblin"), noCastRoll)
	b := wantBlockedBy(t, "Mãos Flamejantes with every slot spent", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_SLOT)
	if b.GetMinLevel() != 1 {
		t.Errorf("NO_SLOT min_level = %d, want 1", b.GetMinLevel())
	}
	if o := spellOption(a.mustOptions(t, a.ana, e, "Pensantus"), burningHands); o.GetEnabled() || o.GetReason().GetCode() != rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_NO_SLOT {
		t.Errorf("the option with no slot = %v, want it disabled: no slot", o)
	}
}

// TestTimelineRound3MagicMissile: Pensantus's Mísseis Mágicos at the 1st level
// makes three darts, shared out as he wants; each target's damage is its own
// roll of 1d4 + 1 for each dart; it spends his last 1st-level slot
// ("0 livres de 4").
func TestTimelineRound3MagicMissile(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 2)
	a.correct(t, a.pens, func(r *playv1.AdjustCharacterVitalsRequest) {
		r.SpellSlotsUsed = []*playv1.SpellSlotsUsed{{Level: 1, Used: 3}}
	})
	opts := a.mustOptions(t, a.ana, e, "Pensantus")
	st := spellTargetsOf(opts, magicMissileSpell)
	if st == nil || len(st.GetDarts()) != 2 || st.GetDarts()[0].GetSlotLevel() != 1 || st.GetDarts()[0].GetDarts() != 3 || st.GetDarts()[1].GetDarts() != 4 || st.GetMaxTargets() != 3 {
		t.Fatalf("Mísseis Mágicos' targets = %v, want 3 darts with the 1st-level slot and 4 with the 2nd, one target for each", st)
	}
	if tg := targetOf2(st, "Goblin 1"); tg == nil || tg.GetDistanceFt() != 10 || tg.GetTooFar() {
		t.Errorf("Goblin 1 as a target = %v, want 10 ft away, in range", tg)
	}

	// The darts must add up: 3 darts, none with fewer than 1.
	for name, targets := range map[string][]*playv1.SpellTarget{
		"too few":      {darts(a, t, "Goblin 1", 1), darts(a, t, "Goblin 2", 1)},
		"too many":     {darts(a, t, "Goblin 1", 2), darts(a, t, "Goblin 2", 2)},
		"a zero":       {darts(a, t, "Goblin 1", 3), darts(a, t, "Goblin 2", 0)},
		"no darts":     a.at(t, "Goblin 1"),
		"no target":    nil,
		"darts on one": {darts(a, t, "Goblin 1", 4)},
	} {
		_, err := a.cast(t, a.ana, e, "Pensantus", magicMissileSpell, slotOfLevel(1), targets, noCastRoll)
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("Mísseis Mágicos with %s: %v, want invalid_argument", name, err)
		}
	}
	if got := usedSlots(a.vitals(t, a.pens), 1); got != 3 {
		t.Fatalf("slots used after the refused casts = %d, want 3 (nothing spent)", got)
	}

	// Two darts to Goblin 1 and one to Goblin 2; the master sets who they are.
	cast := a.mustCast(t, a.ana, e, "Pensantus", magicMissileSpell, slotOfLevel(1), []*playv1.SpellTarget{darts(a, t, "Goblin 1", 2), darts(a, t, "Goblin 2", 1)}, noCastRoll)
	if got := usedSlots(a.vitals(t, a.pens), 1); got != 4 {
		t.Errorf("1st-level slots used = %d, want 4: 0 free of 4", got)
	}
	if len(cast.GetCast().GetTargets()) != 2 || len(cast.GetCast().GetPendingDamages()) != 2 {
		t.Fatalf("the cast = %v, want two targets with a pending damage each", cast.GetCast())
	}
	p1, p2 := cast.GetCast().GetPendingDamages()[0], cast.GetCast().GetPendingDamages()[1]
	if p1.GetDiceCount() != 2 || p1.GetDiceSides() != 4 || p1.GetBonus() != 2 || p1.GetDamageTypeKey() != "damage-type:force" || p2.GetDiceCount() != 1 || p2.GetBonus() != 1 {
		t.Errorf("the pending damages = %v, %v; want 2d4+2 and 1d4+1 of force", p1, p2)
	}
	// Dice: 1d4 + 1 each dart: (3, 4) + 2 = 9 on Goblin 1 (7 PV: defeated), (2) + 1 = 3 on Goblin 2.
	a.h.roller.queue(3, 4, 2)
	r1 := a.mustDamage(t, a.ana, e, p1.GetId(), inAppDamage)
	if d := r1.GetPendingDamage(); d.GetAmount() != 9 || d.GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED || !d.GetTargetDefeated() {
		t.Errorf("Goblin 1's damage = %v, want 9 applied and defeated", d)
	}
	r2 := a.mustDamage(t, a.ana, e, p2.GetId(), inAppDamage)
	if d := r2.GetPendingDamage(); d.GetAmount() != 3 || d.GetTargetDefeated() {
		t.Errorf("Goblin 2's damage = %v, want 3, not defeated", d)
	}
	if cur, _, _ := a.hp(t, "Goblin 2"); cur != 4 {
		t.Errorf("Goblin 2 = %d PV, want 4", cur)
	}

	// All three darts on one target, in the next round: 3d4 + 3 on a Capitão with
	// 10 PV (2, 3, 2 + 3 = 10) defeats him; the 1st-level slots are 0 free of 4.
	a.mustEndTurn(t, a.ana, e) // Toren
	a.mustEndTurn(t, a.caio, e)
	a.mustEndTurn(t, a.bia, e) // Goblin 2 (Goblin 1 is defeated: the turns skip it)
	a.mustEndTurn(t, a.master, e)
	e = a.mustEndTurn(t, a.master, e) // the Capitão; round 2: Pensantus
	if e.GetRound() != 2 || e.GetCurrentCombatantId() != a.id(t, "Pensantus") {
		t.Fatalf("round %d, on turn %s; want Pensantus in round 2", e.GetRound(), e.GetCurrentCombatantId())
	}
	a.correct(t, a.pens, func(r *playv1.AdjustCharacterVitalsRequest) {
		r.SpellSlotsUsed = []*playv1.SpellSlotsUsed{{Level: 1, Used: 3}}
	})
	if _, err := a.adjustHP(t, e, "Capitão Goblin", func(r *playv1.AdjustCombatantHitPointsRequest) {
		r.Change = &playv1.AdjustCombatantHitPointsRequest_HitPoints{HitPoints: 10}
	}); err != nil {
		t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
	}
	cast = a.mustCast(t, a.ana, e, "Pensantus", magicMissileSpell, slotOfLevel(1), []*playv1.SpellTarget{darts(a, t, "Capitão Goblin", 3)}, noCastRoll)
	if d := cast.GetCast().GetPendingDamages()[0]; d.GetDiceCount() != 3 || d.GetBonus() != 3 {
		t.Fatalf("the pending damage = %v, want 3d4+3", d)
	}
	a.h.roller.queue(2, 3, 2)
	if d := a.mustDamage(t, a.ana, e, cast.GetCast().GetPendingDamages()[0].GetId(), inAppDamage).GetPendingDamage(); d.GetAmount() != 10 || !d.GetTargetDefeated() {
		t.Errorf("the Capitão's damage = %v, want 10 and defeated", d)
	}
	if got := usedSlots(a.vitals(t, a.pens), 1); got != 4 {
		t.Errorf("1st-level slots used = %d, want 4 (0 free of 4)", got)
	}
	if o := spellOption(a.mustOptions(t, a.ana, a.get(t, a.master), "Pensantus"), magicMissileSpell); o.GetEnabled() {
		t.Errorf("Mísseis Mágicos with 2nd-level slots free = %v, want it still castable with them", o)
	}
}

func targetOf2(st *playv1.SpellTargets, label string) *playv1.TargetInReach {
	for _, tg := range st.GetTargets() {
		if tg.GetLabel() == label {
			return tg
		}
	}
	return nil
}

// castersFightNPCFirst starts the combat with the Capitão Goblin first (so the
// master can attack with him), then Pensantus, Toren and Brisa, and the goblin
// last; everybody is on the map, the NPCs revealed.
func (a *armed) castersFightNPCFirst(t *testing.T) *playv1.Encounter {
	t.Helper()
	return a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.capitao.GetId()}, {CharacterId: a.goblin.GetId()}},
		npcRolls: []int{20, 1},
		players:  map[string]int32{"Pensantus": 10, "Toren": 9, "Brisa": 8},
		reveal:   []string{"Capitão Goblin", "Goblin"},
		at:       map[string][2]int32{"Pensantus": {5, 5}, "Toren": {6, 5}, "Brisa": {5, 6}, "Capitão Goblin": {4, 4}, "Goblin": {9, 5}}, // nobody between the Capitão and Pensantus: no cover (D4)
	})
}

// TestSaveSpellRollsOnceForTheCast: Mãos Flamejantes on three NPCs: the server
// rolls each one's save, and the damage is one roll of 3d6 for the whole cast: all
// of it for the ones that fail, half (rounded down) for the one that saved. A
// basic-sheet NPC has no save bonus, and the master is told.
func TestSaveSpellRollsOnceForTheCast(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 2)

	// Saves (DC 14 = 8 + 2 + 4: the gnome has Intelligence 18): Goblin 1 rolls 5, Goblin 2 rolls 18 and saves, the
	// Capitão rolls 12; the NPCs have no bonus.
	a.h.roller.queue(5, 18, 12)
	cast := a.mustCast(t, a.ana, e, "Pensantus", burningHands, slotOfLevel(1), a.at(t, "Goblin 1", "Goblin 2", "Capitão Goblin"), noCastRoll)
	saves := map[string]*playv1.SaveResult{}
	for _, tg := range cast.GetCast().GetTargets() {
		saves[tg.GetCombatantId()] = tg.GetSave()
	}
	g1, g2, boss := saves[a.id(t, "Goblin 1")], saves[a.id(t, "Goblin 2")], saves[a.id(t, "Capitão Goblin")]
	if g1.GetOutcome() != playv1.SaveOutcome_SAVE_OUTCOME_FAILED || g2.GetOutcome() != playv1.SaveOutcome_SAVE_OUTCOME_SAVED || boss.GetOutcome() != playv1.SaveOutcome_SAVE_OUTCOME_FAILED {
		t.Fatalf("the saves = %v, %v, %v; want failed, saved, failed", g1, g2, boss)
	}
	// The caster's player gets the DC and the outcome, never an NPC's dice (RN-20).
	for _, sv := range saves {
		if sv.GetRoll() != nil || sv.GetDc() != 14 || sv.GetBonusKnown() {
			t.Errorf("a save as the caster's player = %v, want the outcome and the DC only", sv)
		}
	}
	if len(cast.GetCast().GetPendingDamages()) != 3 {
		t.Fatalf("pending damages = %d, want one for each target (the one that saved takes half)", len(cast.GetCast().GetPendingDamages()))
	}
	var halves int
	for _, p := range cast.GetCast().GetPendingDamages() {
		if p.GetCastId() != cast.GetCast().GetCastId() || p.GetDiceCount() != 3 || p.GetDiceSides() != 6 || p.GetDamageTypeKey() != "damage-type:fire" {
			t.Errorf("pending damage = %v, want 3d6 of fire in the cast", p)
		}
		if p.GetHalf() {
			halves++
		}
	}
	if halves != 1 {
		t.Errorf("%d pending damages are half, want 1 (the goblin that saved)", halves)
	}

	// One roll for the whole cast: 3d6 = 3 + 3 + 3 = 9. Goblin 1 takes 9 (defeated,
	// 7 PV), Goblin 2 takes 4 (half of 9, rounded down), the Capitão 9.
	a.h.roller.queue(3, 3, 3)
	first := cast.GetCast().GetPendingDamages()[0]
	res := a.mustDamage(t, a.ana, e, first.GetId(), inAppDamage)
	if len(res.GetCastPendingDamages()) != 2 {
		t.Fatalf("the roll settled %d other pending damages, want 2", len(res.GetCastPendingDamages()))
	}
	for _, p := range append([]*playv1.PendingDamage{res.GetPendingDamage()}, res.GetCastPendingDamages()...) {
		want := int32(9)
		if p.GetHalf() {
			want = 4
		}
		if p.GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED || p.GetAmount() != want || p.GetRoll().GetTotal() != 9 || len(p.GetRoll().GetFaces()) != 3 {
			t.Errorf("settled damage = %v, want %d applied from one roll of 9", p, want)
		}
	}
	if cur, _, defeated := a.hp(t, "Goblin 1"); cur != 0 || !defeated {
		t.Errorf("Goblin 1 = %d PV, defeated %v; want 0, defeated", cur, defeated)
	}
	if cur, _, _ := a.hp(t, "Goblin 2"); cur != 3 {
		t.Errorf("Goblin 2 = %d PV, want 3 (half of 9 is 4)", cur)
	}
	if cur, _, _ := a.hp(t, "Capitão Goblin"); cur != 18 {
		t.Errorf("Capitão = %d PV, want 18", cur)
	}
	if again, err := a.damage(t, a.ana, e, res.GetCastPendingDamages()[0].GetId(), inAppDamage); err == nil {
		t.Errorf("rolling a damage the cast already settled worked: %v", again)
	} else {
		wantBlockedBy(t, "RollDamage on a settled pending damage", err, blockedDamageResolved)
	}

	// The master's log has the dice and the DC with the bonus flagged; the player's has
	// the words.
	entry := func(l *playv1.ListCombatLogResponse) *playv1.CombatLogEntry {
		for _, r := range l.GetRounds() {
			for _, en := range r.GetEntries() {
				if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_SPELL_CAST {
					return en
				}
			}
		}
		return nil
	}
	m := entry(a.log(t, a.master, e))
	if m == nil || len(m.GetSpell().GetTargets()) != 3 || m.GetSpell().GetSlot().GetLevel() != 1 || m.GetKey() != burningHands || m.GetKeyNamePt() == "" {
		t.Fatalf("the master's SPELL_CAST entry = %v, want three targets, slot 1 and the spell's name", m)
	}
	for _, tg := range m.GetSpell().GetTargets() {
		if tg.GetSave().GetRoll() == nil || tg.GetSave().GetBonusKnown() || tg.GetSave().GetRoll().GetModifier() != 0 || tg.GetDamage().GetRoll() == nil {
			t.Errorf("the master's target = %v, want the save dice (no bonus, flagged) and the damage dice", tg)
		}
	}
	// A player who is not the caster: the words only, and no NPC dice.
	other := entry(a.log(t, a.caio, e))
	if other == nil {
		t.Fatalf("a player does not get the cast of a visible spell")
	}
	for _, tg := range other.GetSpell().GetTargets() {
		if tg.GetSave().GetRoll() != nil || tg.GetSave().GetDc() != 0 || tg.GetSave().GetOutcome() == playv1.SaveOutcome_SAVE_OUTCOME_UNSPECIFIED {
			t.Errorf("another player's target = %v, want the save outcome as a word, no dice, no DC", tg)
		}
		if tg.GetDamage().GetRoll() != nil || tg.GetDamage().GetAmount() == 0 {
			t.Errorf("another player's damage = %v, want the amount without the dice", tg.GetDamage())
		}
	}
}

// TestHealingSpellRevivesAndResetsDeathSaves: a heal reaches a character at 0 hit
// points at once, through the vitals: it gets up and the death saves reset (RN-03).
func TestHealingSpellRevivesAndResetsDeathSaves(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	a.correct(t, a.toren, hpIs(0))
	if got := combatantState(a.get(t, a.caio), "Toren"); got != playv1.CombatantState_COMBATANT_STATE_DOWN {
		t.Fatalf("Toren's state at 0 PV = %v, want DOWN", got)
	}
	a.mustEndTurn(t, a.ana, e) // Toren's turn: a death save is due

	// A failure (a 5), then Brisa's turn.
	a.h.roller.queue(5)
	if _, err := a.deathSave(t, a.caio, e, "Toren", rollApp); err != nil {
		t.Fatalf("RollDeathSave() error = %v", err)
	}
	a.mustEndTurn(t, a.caio, e)
	if c := byLabel(t, a.get(t, a.caio), "Toren"); c.GetDeathFailures() != 1 || c.GetDeathSuccesses() != 0 {
		t.Fatalf("Toren's death saves = %d successes, %d failures; want 0 and 1", c.GetDeathSuccesses(), c.GetDeathFailures())
	}

	// Brisa heals him with Curar Ferimentos (touch: he is next to her): 1d8 (4) + 3.
	cast := a.mustCast(t, a.bia, e, "Brisa", cureWounds, slotOfLevel(1), a.at(t, "Toren"), noCastRoll)
	pending := cast.GetCast().GetPendingDamages()[0]
	if !pending.GetHealing() || pending.GetDiceCount() != 1 || pending.GetDiceSides() != 8 || pending.GetBonus() != 3 {
		t.Fatalf("the heal = %v, want 1d8+3", pending)
	}
	a.h.roller.queue(4)
	healed := a.mustDamage(t, a.bia, e, pending.GetId(), inAppDamage).GetPendingDamage()
	if healed.GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED || healed.GetAmount() != 7 {
		t.Errorf("the heal = %v, want 7 applied at once", healed)
	}
	if got := a.vitals(t, a.toren).GetHitPointsCurrent(); got != 7 {
		t.Errorf("Toren = %d PV, want 7", got)
	}
	c := byLabel(t, a.get(t, a.caio), "Toren")
	if c.GetDeathFailures() != 0 || c.GetDeathSuccesses() != 0 || c.GetState() == playv1.CombatantState_COMBATANT_STATE_DOWN {
		t.Errorf("Toren after the heal = %d failures, state %v; want the death saves reset and not down", c.GetDeathFailures(), c.GetState())
	}
}

var rollApp = func(r *playv1.RollDeathSaveRequest) { r.Roll = &playv1.RollDeathSaveRequest_RollInApp{RollInApp: true} }

func deathFace(face int32) func(*playv1.RollDeathSaveRequest) {
	return func(r *playv1.RollDeathSaveRequest) { r.Roll = &playv1.RollDeathSaveRequest_D20Face{D20Face: face} }
}

// deathSave calls RollDeathSave as u, by label.
func (a *armed) deathSave(t *testing.T, u *user, e *playv1.Encounter, label string, roll func(*playv1.RollDeathSaveRequest)) (*playv1.RollDeathSaveResponse, error) {
	t.Helper()
	req := &playv1.RollDeathSaveRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, label), IdempotencyKey: newKey()}
	roll(req)
	res, err := u.combat.RollDeathSave(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// useReaction calls UseReaction as u.
func (a *armed) useReaction(t *testing.T, u *user, e *playv1.Encounter, pendingID string, slot *playv1.SpellSlot) (*playv1.UseReactionResponse, error) {
	t.Helper()
	res, err := u.combat.UseReaction(t.Context(), connect.NewRequest(&playv1.UseReactionRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), PendingDamageId: pendingID, Slot: slot, IdempotencyKey: newKey(),
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// declineReaction calls DeclineReaction as u.
func (a *armed) declineReaction(t *testing.T, u *user, e *playv1.Encounter, pendingID string) (*playv1.DeclineReactionResponse, error) {
	t.Helper()
	res, err := u.combat.DeclineReaction(t.Context(), connect.NewRequest(&playv1.DeclineReactionRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), PendingDamageId: pendingID, IdempotencyKey: newKey(),
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// TestShieldTurnsAHitIntoAMiss: when an attack hits a player's character that can
// cast Escudo, the hit waits for the reaction (timeline decision 5): the target's
// player and the master get a prompt, nobody else; Escudo gives +5 armor class
// until the start of the caster's next turn and the attack is compared again.
func TestShieldTurnsAHitIntoAMiss(t *testing.T) {
	t.Parallel()
	// Pensantus has AC 12 (10 + Dexterity 2); with Escudo, 17. The Capitão attacks
	// with +4.
	t.Run("the new armor class stops the attack", func(t *testing.T) {
		a := newCasters(t)
		e := a.castersFightNPCFirst(t)
		hit := a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Pensantus", d20(9)) // 9 + 4 = 13: a hit
		pending := hit.GetPendingDamage()
		if hit.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_HIT || pending.GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_AWAITING_REACTION {
			t.Fatalf("the attack = %v / %v, want a hit that waits for the reaction", hit.GetRoll(), pending)
		}
		// Who sees the prompt: the target's player and the master.
		if prompts := a.get(t, a.ana).GetReactionPrompts(); len(prompts) != 1 || prompts[0].GetPendingDamageId() != pending.GetId() || prompts[0].GetAttackerId() != "" ||
			prompts[0].GetTargetId() != a.id(t, "Pensantus") || prompts[0].GetSpellKey() != shieldSpell || len(prompts[0].GetSlots()) != 2 {
			t.Errorf("Ana's prompts = %v, want the hit on Pensantus, with two slot levels and no attacker", a.get(t, a.ana).GetReactionPrompts())
		}
		if prompts := a.get(t, a.master).GetReactionPrompts(); len(prompts) != 1 || prompts[0].GetAttackerId() != a.id(t, "Capitão Goblin") {
			t.Errorf("the master's prompts = %v, want the hit, with the attacker", prompts)
		}
		for name, u := range map[string]*user{"Toren's player": a.caio, "Brisa's player": a.bia} {
			if p := a.get(t, u).GetReactionPrompts(); len(p) != 0 {
				t.Errorf("%s got the prompt %v: it is the target's and the master's", name, p)
			}
		}
		// The attacker's side waits; the damage cannot be rolled, and the turn is held.
		_, err := a.damage(t, a.master, e, pending.GetId(), inAppDamage)
		wantBlockedBy(t, "RollDamage while the hit waits", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_REACTION_PENDING)
		_, err = a.endTurn(t, a.master, e, false)
		wantBlockedBy(t, "EndTurn while the hit waits", err, blockedPendingDamage)
		_, err = a.settle(t, a.master, e, pending.GetId(), true)
		wantBlockedBy(t, "ApplyPendingDamage while the hit waits", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_REACTION_PENDING)

		// Only the target's player or the master answers.
		_, err = a.useReaction(t, a.caio, e, pending.GetId(), slotOfLevel(1))
		wantCode(t, "UseReaction by another player", err, connect.CodePermissionDenied)
		_, err = a.declineReaction(t, a.bia, e, pending.GetId())
		wantCode(t, "DeclineReaction by another player", err, connect.CodePermissionDenied)
		_, err = a.useReaction(t, a.ana, e, pending.GetId(), slotOfLevel(3))
		wantCode(t, "Escudo with a slot she does not have free", err, connect.CodeInvalidArgument)

		res, err := a.useReaction(t, a.ana, e, pending.GetId(), slotOfLevel(1))
		if err != nil {
			t.Fatalf("UseReaction() error = %v", err)
		}
		if res.GetOutcome() != playv1.ReactionOutcome_REACTION_OUTCOME_STOPPED || res.GetPendingDamage() != nil {
			t.Errorf("UseReaction() = %v, want the attack stopped, and nothing about the pending damage for the player", res)
		}
		c := byLabel(t, res.GetEncounter(), "Pensantus")
		if !c.GetReactionUsed() || c.GetArmorClassBonus() != 5 || len(res.GetEncounter().GetReactionPrompts()) != 0 {
			t.Errorf("Pensantus after Escudo = reaction used %v, bonus %d, prompts %v; want the reaction used, +5 and no prompt left", c.GetReactionUsed(), c.GetArmorClassBonus(), res.GetEncounter().GetReactionPrompts())
		}
		if got := usedSlots(a.vitals(t, a.pens), 1); got != 1 {
			t.Errorf("1st-level slots used = %d, want 1", got)
		}
		if cb := byLabel(t, a.get(t, a.caio), "Pensantus"); cb.GetArmorClassBonus() != 0 {
			t.Errorf("another player sees Pensantus's armor class bonus %d, want it hidden", cb.GetArmorClassBonus())
		}
		// The pending damage was stopped, and the log says Escudo did it.
		var attack, reaction *playv1.CombatLogEntry
		for _, en := range a.log(t, a.caio, e).GetRounds()[0].GetEntries() {
			switch en.GetKind() {
			case playv1.CombatLogKind_COMBAT_LOG_KIND_ATTACK:
				attack = en
			case playv1.CombatLogKind_COMBAT_LOG_KIND_REACTION:
				reaction = en
			}
		}
		if attack == nil || attack.GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_MISS || !attack.GetStoppedByReaction() || attack.GetDamage() != nil {
			t.Errorf("the attack's entry = %v, want a miss stopped by the reaction, with no damage", attack)
		}
		if reaction == nil || reaction.GetActorLabel() != "Pensantus" || reaction.GetKey() != shieldSpell || reaction.GetSpell().GetSlot().GetLevel() != 1 || reaction.GetTargetId() != "" {
			t.Errorf("the reaction's entry = %v, want Pensantus's Escudo with its slot and no attacker", reaction)
		}

		// Every attack meanwhile uses the +5: a second hit with the same total (13)
		// misses at once, and no prompt rises (the reaction is used).
		again := a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Pensantus", d20(9))
		if again.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_MISS || again.GetPendingDamage() != nil {
			t.Errorf("the second attack = %v, want a miss against the armor class of 17", again.GetRoll())
		}
		// At the start of Pensantus's next turn the bonus ends and the reaction is back.
		e = a.mustEndTurn(t, a.master, e)
		if c := byLabel(t, e, "Pensantus"); c.GetArmorClassBonus() != 0 || c.GetReactionUsed() {
			t.Errorf("Pensantus at the start of his turn = bonus %d, reaction used %v; want 0 and available", c.GetArmorClassBonus(), c.GetReactionUsed())
		}
	})

	t.Run("a total that still reaches the new armor class is still a hit", func(t *testing.T) {
		a := newCasters(t)
		e := a.castersFightNPCFirst(t)
		hit := a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Pensantus", d20(14)) // 18 >= 17
		res, err := a.useReaction(t, a.ana, e, hit.GetPendingDamage().GetId(), slotOfLevel(2))
		if err != nil {
			t.Fatalf("UseReaction() error = %v", err)
		}
		if res.GetOutcome() != playv1.ReactionOutcome_REACTION_OUTCOME_STILL_HIT {
			t.Errorf("outcome = %v, want still hit", res.GetOutcome())
		}
		if got := usedSlots(a.vitals(t, a.pens), 2); got != 1 {
			t.Errorf("2nd-level slots used = %d, want 1 (the slot is spent either way)", got)
		}
		// The damage goes on: the master rolls it, and it waits for him to apply it.
		a.h.roller.queue(3)
		if d := a.mustDamage(t, a.master, e, hit.GetPendingDamage().GetId(), inAppDamage).GetPendingDamage(); d.GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_ROLLED || d.GetAmount() != 5 {
			t.Errorf("the damage = %v, want 5 rolled, waiting for the master", d)
		}
		// With the reaction spent, Escudo cannot be used again, and a master's try is refused too.
		again := a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Pensantus", d20(18))
		if again.GetPendingDamage().GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_AWAITING_ROLL {
			t.Errorf("a hit with the reaction used = %v, want it to wait for its roll, with no prompt", again.GetPendingDamage())
		}
	})

	t.Run("declining lets the hit go, and a critical hit has no prompt", func(t *testing.T) {
		a := newCasters(t)
		e := a.castersFightNPCFirst(t)
		crit := a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Pensantus", d20(20))
		if crit.GetPendingDamage().GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_AWAITING_ROLL || len(a.get(t, a.ana).GetReactionPrompts()) != 0 {
			t.Errorf("a critical hit = %v, want no prompt: it goes straight to its damage", crit.GetPendingDamage())
		}
		if _, err := a.settle(t, a.master, e, crit.GetPendingDamage().GetId(), false); err != nil {
			t.Fatalf("DiscardPendingDamage() error = %v", err)
		}
		hit := a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Pensantus", d20(9))
		if _, err := a.declineReaction(t, a.ana, e, hit.GetPendingDamage().GetId()); err != nil {
			t.Fatalf("DeclineReaction() error = %v", err)
		}
		if c := byLabel(t, a.get(t, a.ana), "Pensantus"); c.GetReactionUsed() || len(a.get(t, a.ana).GetReactionPrompts()) != 0 {
			t.Errorf("Pensantus after declining = reaction used %v; want it available", c.GetReactionUsed())
		}
		if got := usedSlots(a.vitals(t, a.pens), 1); got != 0 {
			t.Errorf("slots used after declining = %d, want 0", got)
		}
		if d := a.mustDamage(t, a.master, e, hit.GetPendingDamage().GetId(), inAppDamage).GetPendingDamage(); d.GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_ROLLED {
			t.Errorf("the declined hit's damage = %v, want it rolled", d)
		}
		// Answering twice: the hit no longer waits.
		_, err := a.declineReaction(t, a.ana, e, hit.GetPendingDamage().GetId())
		wantBlockedBy(t, "DeclineReaction after the answer", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_AWAITING_REACTION)
		_, err = a.useReaction(t, a.ana, e, hit.GetPendingDamage().GetId(), slotOfLevel(1))
		wantBlockedBy(t, "UseReaction after the answer", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_AWAITING_REACTION)
	})
}

// TestOpportunityAttackSpendsTheReaction: a melee attack off turn, with
// as_reaction, spends the reaction instead of the action.
func TestOpportunityAttackSpendsTheReaction(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1) // Pensantus is on turn; Toren is next to the goblin (6,5) and (7,5)

	// Off turn, an ordinary attack is refused; as a reaction it is an opportunity attack.
	_, err := a.attack(t, a.caio, e, "Toren", battleaxe, "Goblin", d20(15))
	wantBlockedBy(t, "an attack off turn", err, blockedNotYourTurn)
	opp, err := a.attackAs(t, a.caio, e, "Toren", battleaxe, "Goblin", d20(15), true)
	if err != nil {
		t.Fatalf("RollAttack(as_reaction) error = %v", err)
	}
	if opp.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_HIT {
		t.Errorf("the opportunity attack = %v, want a hit (15 + 6 against 12)", opp.GetRoll())
	}
	toren := byLabel(t, opp.GetEncounter(), "Toren")
	if !toren.GetReactionUsed() || toren.GetActionUsed() {
		t.Errorf("Toren after it = reaction used %v, action used %v; want the reaction spent and the action free", toren.GetReactionUsed(), toren.GetActionUsed())
	}
	_, err = a.attackAs(t, a.caio, e, "Toren", battleaxe, "Goblin", d20(15), true)
	wantBlockedBy(t, "a second opportunity attack", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_REACTION_USED)
	// It is the attack's entry in the log, marked as a reaction.
	if en := a.log(t, a.master, e).GetRounds()[0].GetEntries()[0]; en.GetKind() != playv1.CombatLogKind_COMBAT_LOG_KIND_ATTACK || !en.GetAsReaction() {
		t.Errorf("the log's last entry = %v, want the attack as a reaction", en)
	}
	// On its own turn an attack spends the action: as_reaction is refused there;
	// and a ranged attack is no opportunity attack.
	_, err = a.attackAs(t, a.ana, e, "Pensantus", fireBolt, "Goblin", d20(15), true)
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("as_reaction on its own turn = %v, want invalid_argument", err)
	}
	a.h.roller.queue(2)
	a.mustDamage(t, a.caio, e, opp.GetPendingDamage().GetId(), inAppDamage)
	a.mustEndTurn(t, a.ana, e) // Toren's turn: the reaction is back
	if c := byLabel(t, a.get(t, a.caio), "Toren"); c.GetReactionUsed() {
		t.Errorf("Toren's reaction at the start of his turn is still used")
	}
	a.mustEndTurn(t, a.caio, e) // Brisa
	_, err = a.attackAs(t, a.ana, e, "Pensantus", fireBolt, "Goblin", d20(15), true)
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a ranged opportunity attack = %v, want invalid_argument", err)
	}
}

// attackAs is attack with the reaction flag.
func (a *armed) attackAs(t *testing.T, u *user, e *playv1.Encounter, attacker, key, target string, roll func(*playv1.RollAttackRequest), asReaction bool) (*playv1.RollAttackResponse, error) {
	t.Helper()
	req := &playv1.RollAttackRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), AttackerId: a.id(t, attacker), AttackKey: key, TargetId: a.id(t, target), IdempotencyKey: newKey(), AsReaction: asReaction,
	}
	roll(req)
	res, err := u.combat.RollAttack(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

var blockedNotYourTurn = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_YOUR_TURN

// passTo ends turns, as the master, until the combatant is on turn; it fails
// after a few rounds so a wrong label does not loop.
func (a *armed) passTo(t *testing.T, e *playv1.Encounter, label string) *playv1.Encounter {
	t.Helper()
	for range 12 {
		cur := a.get(t, a.master)
		if cur.GetCurrentCombatantId() == a.id(t, label) {
			return cur
		}
		var err error
		if _, err = a.endTurn(t, a.master, e, true); err != nil {
			t.Fatalf("EndTurn() while passing to %s: %v", label, err)
		}
	}
	t.Fatalf("%s never came on turn", label)
	return nil
}

// TestRN03_DeathSavesAndTheMasterConfirms follows the timeline's Rodada 3 and 4:
// Toren falls to 0 hit points; at the start of his turn a death save is due and
// the turn waits for it; a damage that hits him at 0 is a failure (two for a
// critical hit); three failures make him dying and the master confirms the
// death, which marks the character dead and takes it out of the order. A player
// never sees "morrendo" or "morto" before the confirmation.
func TestRN03_DeathSavesAndTheMasterConfirms(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1) // Pensantus, Toren, Brisa, Goblin, Capitão
	a.correct(t, a.toren, hpIs(0))
	for who, u := range map[string]*user{"master": a.master, "Toren's player": a.caio, "Pensantus's player": a.ana} {
		if got := combatantState(a.get(t, u), "Toren"); got != playv1.CombatantState_COMBATANT_STATE_DOWN {
			t.Errorf("Toren's state for %s = %v, want DOWN (\"Caído\")", who, got)
		}
	}

	// His turn starts with a death save due: for him and the master, not for the others.
	a.mustEndTurn(t, a.ana, e)
	if !byLabel(t, a.get(t, a.master), "Toren").GetDeathSaveDue() || !byLabel(t, a.get(t, a.caio), "Toren").GetDeathSaveDue() || byLabel(t, a.get(t, a.ana), "Toren").GetDeathSaveDue() {
		t.Fatalf("death_save_due: master %v, Toren %v, Pensantus's player %v; want true, true, false",
			byLabel(t, a.get(t, a.master), "Toren").GetDeathSaveDue(), byLabel(t, a.get(t, a.caio), "Toren").GetDeathSaveDue(), byLabel(t, a.get(t, a.ana), "Toren").GetDeathSaveDue())
	}
	_, err := a.endTurn(t, a.caio, e, false)
	wantBlockedBy(t, "EndTurn with a death save due", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_DEATH_SAVE_DUE)
	_, err = a.attack(t, a.caio, e, "Toren", battleaxe, "Goblin", d20(15))
	wantBlockedBy(t, "an attack while down", err, blockedDown)
	_, err = a.deathSave(t, a.ana, e, "Toren", rollApp)
	wantCode(t, "RollDeathSave by another player", err, connect.CodePermissionDenied)

	// A 14 is a success.
	a.h.roller.queue(14)
	res, err := a.deathSave(t, a.caio, e, "Toren", rollApp)
	if err != nil {
		t.Fatalf("RollDeathSave() error = %v", err)
	}
	if ds := res.GetDeathSave(); ds.GetOutcome() != playv1.DeathSaveOutcome_DEATH_SAVE_OUTCOME_SUCCESS || ds.GetSuccesses() != 1 || ds.GetFailures() != 0 ||
		ds.GetRoll().GetFaces()[0] != 14 || ds.GetStable() || ds.GetDying() {
		t.Errorf("the death save = %v, want a success (14), 1 and 0", ds)
	}
	if byLabel(t, res.GetEncounter(), "Toren").GetDeathSaveDue() {
		t.Error("the death save is still due after it was rolled")
	}
	_, err = a.deathSave(t, a.caio, e, "Toren", rollApp)
	wantBlockedBy(t, "a second death save in the turn", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_DEATH_SAVE_NOT_DUE)
	a.mustEndTurn(t, a.caio, e) // Brisa
	a.mustEndTurn(t, a.bia, e)  // the goblin

	// Damage while down is a failure, and the hit points stay at 0: the goblin hits him
	// (15 + 4 against 12), the master rolls 1d6 (3) + 2 and applies it.
	a.h.roller.queue(15, 3)
	hit := a.mustAttack(t, a.master, e, "Goblin", sword, "Toren", inAppRoll)
	id := hit.GetPendingDamage().GetId()
	a.mustDamage(t, a.master, e, id, inAppDamage)
	applied, err := a.settle(t, a.master, e, id, true)
	if err != nil {
		t.Fatalf("ApplyPendingDamage() error = %v", err)
	}
	if applied.GetDeathFailuresAdded() != 1 || a.vitals(t, a.toren).GetHitPointsCurrent() != 0 {
		t.Errorf("damage at 0 = %v, Toren at %d PV; want 1 failure added and 0 PV", applied, a.vitals(t, a.toren).GetHitPointsCurrent())
	}
	if c := byLabel(t, a.get(t, a.caio), "Toren"); c.GetDeathSuccesses() != 1 || c.GetDeathFailures() != 1 {
		t.Errorf("Toren's counts = %d and %d, want 1 and 1", c.GetDeathSuccesses(), c.GetDeathFailures())
	}
	a.mustEndTurn(t, a.master, e) // the Capitão

	// A critical hit is two failures: three in all, and he is dying for the master;
	// for the players the word is still "Caído" (never "morrendo").
	a.h.roller.queue(2, 2)
	crit := a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Toren", d20(20))
	a.mustDamage(t, a.master, e, crit.GetPendingDamage().GetId(), inAppDamage)
	applied, err = a.settle(t, a.master, e, crit.GetPendingDamage().GetId(), true)
	if err != nil || applied.GetDeathFailuresAdded() != 2 {
		t.Fatalf("the critical hit at 0 = %v, %v; want 2 failures added", applied, err)
	}
	for who, want := range map[string]playv1.CombatantState{
		"master": playv1.CombatantState_COMBATANT_STATE_DYING, "Toren's player": playv1.CombatantState_COMBATANT_STATE_DOWN, "Pensantus's player": playv1.CombatantState_COMBATANT_STATE_DOWN,
	} {
		u := map[string]*user{"master": a.master, "Toren's player": a.caio, "Pensantus's player": a.ana}[who]
		if got := combatantState(a.get(t, u), "Toren"); got != want {
			t.Errorf("Toren's state for %s = %v, want %v", who, got, want)
		}
	}
	if c := byLabel(t, a.get(t, a.ana), "Toren"); c.GetDeathFailures() != 3 {
		t.Errorf("the failures a player sees = %d, want 3 (the counts are public)", c.GetDeathFailures())
	}
	if text := asJSON(t, a.get(t, a.caio)) + asJSON(t, a.log(t, a.caio, e)) + asJSON(t, a.log(t, a.ana, e)); strings.Contains(text, "DYING") || strings.Contains(text, "DEAD") || strings.Contains(text, "dying\":true") {
		t.Errorf("a player's messages claim a death before the master confirms it: %s", text)
	}

	// His next turn: nothing to roll (he is dying), and the master decides.
	e = a.passTo(t, e, "Toren")
	if byLabel(t, a.get(t, a.caio), "Toren").GetDeathSaveDue() {
		t.Error("a dying character owes a death save")
	}
	_, err = a.deathSave(t, a.caio, e, "Toren", rollApp)
	wantBlockedBy(t, "a death save while dying", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_DEATH_SAVE_NOT_DUE)
	confirm := func(u *user, key string) (*playv1.ConfirmDeathResponse, error) {
		res, err := u.combat.ConfirmDeath(t.Context(), connect.NewRequest(&playv1.ConfirmDeathRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Toren"), IdempotencyKey: key}))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	}
	_, err = confirm(a.caio, newKey())
	wantCode(t, "ConfirmDeath by the player", err, connect.CodePermissionDenied)
	key := newKey()
	done, err := confirm(a.master, key)
	if err != nil {
		t.Fatalf("ConfirmDeath() error = %v", err)
	}
	for who, u := range map[string]*user{"master": a.master, "Toren's player": a.caio, "Pensantus's player": a.ana} {
		c := byLabel(t, a.get(t, u), "Toren")
		if c.GetState() != playv1.CombatantState_COMBATANT_STATE_DEAD || !c.GetDefeated() {
			t.Errorf("Toren for %s = state %v, defeated %v; want DEAD and out of the order", who, c.GetState(), c.GetDefeated())
		}
	}
	if done.GetEncounter().GetCurrentCombatantId() != a.id(t, "Brisa") {
		t.Errorf("on turn after the confirmation = %s, want Brisa (the turn passed on)", done.GetEncounter().GetCurrentCombatantId())
	}
	if got := a.caio.character(t, a.toren).GetState(); got != charactersv1.CharacterState_CHARACTER_STATE_DEAD {
		t.Errorf("Toren's character state = %v, want DEAD (MarkCharacterDead's effect)", got)
	}
	events := a.events(t)
	if _, err := confirm(a.master, key); err != nil || a.events(t) != events {
		t.Errorf("the retry of ConfirmDeath = %v, %d new events; want none", err, a.events(t)-events)
	}
	_, err = confirm(a.master, newKey())
	wantBlockedBy(t, "ConfirmDeath twice", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_DYING)
	// It cannot be undone: the character is dead in the characters module.
	if err := a.undo(t, a.master, e, newKey()); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("UndoLastAction after ConfirmDeath = %v, want nothing to undo", err)
	}
	var confirmed bool
	for _, r := range a.log(t, a.ana, e).GetRounds() {
		for _, en := range r.GetEntries() {
			confirmed = confirmed || en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_DEATH_CONFIRMED && en.GetTargetLabel() == "Toren"
		}
	}
	if !confirmed {
		t.Error("the players' log has no line for the confirmed death")
	}
}

// TestRN03_NaturalOneTwentyAndStable: a natural 1 is two failures, a natural 20
// brings the character back with 1 hit point and resets both counts, and three
// successes make it stable ("Estável").
func TestRN03_NaturalOneTwentyAndStable(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	a.correct(t, a.toren, hpIs(0))
	a.mustEndTurn(t, a.ana, e) // Toren's turn
	res, err := a.deathSave(t, a.caio, e, "Toren", deathFace(1))
	if err != nil {
		t.Fatalf("RollDeathSave(1) error = %v", err)
	}
	if ds := res.GetDeathSave(); ds.GetOutcome() != playv1.DeathSaveOutcome_DEATH_SAVE_OUTCOME_CRITICAL_FAILURE || ds.GetFailures() != 2 || !ds.GetRoll().GetPhysical() {
		t.Errorf("a natural 1 = %v, want two failures", ds)
	}

	// Next round: a natural 20 gets him up with 1 PV and the counts reset; he can act.
	e = a.passTo(t, e, "Pensantus")
	a.mustEndTurn(t, a.ana, e)
	res, err = a.deathSave(t, a.caio, e, "Toren", deathFace(20))
	if err != nil {
		t.Fatalf("RollDeathSave(20) error = %v", err)
	}
	c := byLabel(t, res.GetEncounter(), "Toren")
	if ds := res.GetDeathSave(); ds.GetOutcome() != playv1.DeathSaveOutcome_DEATH_SAVE_OUTCOME_REVIVED || ds.GetSuccesses() != 0 || ds.GetFailures() != 0 {
		t.Errorf("a natural 20 = %v, want revived with the counts reset", ds)
	}
	if got := a.vitals(t, a.toren).GetHitPointsCurrent(); got != 1 || c.GetState() == playv1.CombatantState_COMBATANT_STATE_DOWN || c.GetDeathFailures() != 0 {
		t.Errorf("Toren after a 20 = %d PV, state %v, %d failures; want 1 PV, up, no failures", got, c.GetState(), c.GetDeathFailures())
	}
	a.h.roller.queue(15)
	if _, err := a.attack(t, a.caio, e, "Toren", battleaxe, "Goblin", inAppRoll); err != nil {
		t.Errorf("an attack after getting up: %v, want it allowed", err)
	}

	// Three successes: stable, and nothing more to roll. Toren falls again.
	if _, err := a.endTurn(t, a.master, e, true); err != nil { // his attack's damage is dropped
		t.Fatalf("EndTurn() error = %v", err)
	}
	a.correct(t, a.toren, hpIs(0))
	for range 3 {
		e = a.passTo(t, e, "Toren")
		if _, err := a.deathSave(t, a.caio, e, "Toren", deathFace(12)); err != nil {
			t.Fatalf("RollDeathSave(12) error = %v", err)
		}
		a.mustEndTurn(t, a.caio, e)
	}
	for who, u := range map[string]*user{"master": a.master, "Toren's player": a.caio, "Pensantus's player": a.ana} {
		if got := combatantState(a.get(t, u), "Toren"); got != playv1.CombatantState_COMBATANT_STATE_STABLE {
			t.Errorf("Toren's state for %s after three successes = %v, want STABLE", who, got)
		}
	}
	e = a.passTo(t, e, "Toren")
	if byLabel(t, a.get(t, a.caio), "Toren").GetDeathSaveDue() {
		t.Error("a stable character owes a death save")
	}
	// A damage that hits a stable character starts the saves again: a failure.
	a.mustEndTurn(t, a.caio, e)
	a.h.roller.queue(15, 3)
	e = a.passTo(t, e, "Goblin")
	hit := a.mustAttack(t, a.master, e, "Goblin", sword, "Toren", inAppRoll)
	a.mustDamage(t, a.master, e, hit.GetPendingDamage().GetId(), inAppDamage)
	if _, err := a.settle(t, a.master, e, hit.GetPendingDamage().GetId(), true); err != nil {
		t.Fatalf("ApplyPendingDamage() error = %v", err)
	}
	if c := byLabel(t, a.get(t, a.caio), "Toren"); c.GetDeathSuccesses() != 0 || c.GetDeathFailures() != 1 || c.GetState() != playv1.CombatantState_COMBATANT_STATE_DOWN {
		t.Errorf("Toren after damage while stable = %d successes, %d failures, %v; want 0, 1 and DOWN", c.GetDeathSuccesses(), c.GetDeathFailures(), c.GetState())
	}
}

// conditions calls SetCombatantConditions as u.
func (a *armed) conditions(t *testing.T, u *user, e *playv1.Encounter, label string, keys []string, set, endConcentration bool) (*playv1.Encounter, error) {
	t.Helper()
	req := &playv1.SetCombatantConditionsRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, label), IdempotencyKey: newKey(), EndConcentration: endConcentration,
	}
	if set {
		req.Conditions = &playv1.ConditionList{Keys: keys}
	}
	res, err := u.combat.SetCombatantConditions(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetEncounter(), nil
}

// TestRN22_ConditionsAndTheConcentrationReminder: conditions are labels the
// master marks (a hidden combatant's never reach a player); a concentration spell
// is set by the cast and replaced by the next one; the damage that reaches a
// concentrating combatant carries the DC to keep it.
func TestRN22_ConditionsAndTheConcentrationReminder(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFightNPCFirst(t)

	// Labels: the master marks them, a player sees them with their names.
	e, err := a.conditions(t, a.master, e, "Goblin", []string{"condition:poisoned", "condition:prone"}, true, false)
	if err != nil {
		t.Fatalf("SetCombatantConditions() error = %v", err)
	}
	if g := byLabel(t, a.get(t, a.caio), "Goblin"); !slices.Equal(g.GetConditions(), []string{"condition:poisoned", "condition:prone"}) || !slices.Equal(g.GetConditionNamesPt(), []string{"Envenenado", "Derrubado"}) {
		t.Errorf("a player sees the goblin's conditions as %v / %v, want the keys and the Portuguese names", g.GetConditions(), g.GetConditionNamesPt())
	}
	for name, call := range map[string]func() error{
		"a player setting conditions": func() error {
			_, err := a.conditions(t, a.ana, e, "Pensantus", []string{"condition:poisoned"}, true, false)
			return err
		},
		"a player ending another's concentration": func() error { _, err := a.conditions(t, a.caio, e, "Pensantus", nil, false, true); return err },
	} {
		wantCode(t, name, call(), connect.CodePermissionDenied)
	}
	for name, keys := range map[string][]string{"an unknown key": {"condition:sleepy"}, "a repeated one": {"condition:prone", "condition:prone"}} {
		_, err := a.conditions(t, a.master, e, "Goblin", keys, true, false)
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("%s: %v, want invalid_argument", name, err)
		}
	}
	if _, err := a.conditions(t, a.master, e, "Goblin", nil, false, false); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("nothing to change: %v, want invalid_argument", err)
	}
	// A hidden combatant's conditions never reach a player.
	if _, err := a.master.combat.SetCombatantHidden(t.Context(), connect.NewRequest(&playv1.SetCombatantHiddenRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Goblin"), IdempotencyKey: newKey(), Hidden: true,
	})); err != nil {
		t.Fatalf("SetCombatantHidden() error = %v", err)
	}
	if _, err := a.conditions(t, a.master, e, "Goblin", []string{"condition:frightened"}, true, false); err != nil {
		t.Fatalf("SetCombatantConditions(hidden) error = %v", err)
	}
	for _, text := range []string{asJSON(t, a.get(t, a.caio)), asJSON(t, a.log(t, a.caio, e))} {
		if strings.Contains(text, "frightened") || strings.Contains(text, "Amedrontado") || strings.Contains(text, a.id(t, "Goblin")) {
			t.Errorf("a hidden goblin's condition reached a player: %s", text)
		}
	}

	// Concentration: Pensantus's turn. Hold Person (2nd level) sets it.
	e = a.mustEndTurn(t, a.master, e)
	held := a.mustCast(t, a.ana, e, "Pensantus", holdPerson, slotOfLevel(2), a.at(t, "Capitão Goblin"), noCastRoll)
	if !held.GetCast().GetConcentrating() || held.GetCast().GetConcentrationEndedSpellKey() != "" || byLabel(t, held.GetEncounter(), "Pensantus").GetConcentrationSpell() != holdPerson {
		t.Fatalf("Hold Person's cast = %v, want Pensantus concentrating on it", held.GetCast())
	}
	// A second concentration spell replaces it (the master casts again for him: the action is used).
	web := a.mustCast(t, a.master, e, "Pensantus", webSpell, slotOfLevel(2), a.at(t, "Capitão Goblin"), noCastRoll)
	if web.GetCast().GetConcentrationEndedSpellKey() != holdPerson || byLabel(t, web.GetEncounter(), "Pensantus").GetConcentrationSpell() != webSpell {
		t.Errorf("Teia's cast = %v, concentration %q; want Hold Person ended and Teia on", web.GetCast(), byLabel(t, web.GetEncounter(), "Pensantus").GetConcentrationSpell())
	}
	var ended *playv1.CombatLogEntry
	for _, en := range a.log(t, a.caio, e).GetRounds()[0].GetEntries() {
		if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_SPELL_CAST && en.GetKey() == webSpell {
			ended = en
		}
	}
	if ended == nil || ended.GetSpell().GetConcentrationEndedKey() != holdPerson || !ended.GetSpell().GetConcentrating() {
		t.Errorf("the log's entry for Teia = %v, want it to say Hold Person ended", ended)
	}

	// The reminder: the Capitão's hit on Pensantus. The master applies the rolled 8: DC 10 (the
	// minimum). Applying 30 instead (his last word): DC 15.
	e = a.passTo(t, e, "Capitão Goblin")
	hit := a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Pensantus", d20(9))
	if _, err := a.declineReaction(t, a.ana, e, hit.GetPendingDamage().GetId()); err != nil {
		t.Fatalf("DeclineReaction() error = %v", err)
	}
	a.h.roller.queue(6)
	a.mustDamage(t, a.master, e, hit.GetPendingDamage().GetId(), inAppDamage)
	applied, err := a.settle(t, a.master, e, hit.GetPendingDamage().GetId(), true)
	if err != nil || applied.GetConcentrationDc() != 10 {
		t.Fatalf("the damage on a concentrating Pensantus = %v, %v; want the DC 10 reminder", applied, err)
	}
	hit = a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Pensantus", d20(9))
	if _, err := a.declineReaction(t, a.ana, e, hit.GetPendingDamage().GetId()); err != nil {
		t.Fatalf("DeclineReaction() error = %v", err)
	}
	a.h.roller.queue(6)
	a.mustDamage(t, a.master, e, hit.GetPendingDamage().GetId(), inAppDamage)
	big := int32(30)
	res, err := a.master.combat.ApplyPendingDamage(t.Context(), connect.NewRequest(&playv1.ApplyPendingDamageRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), PendingDamageId: hit.GetPendingDamage().GetId(), IdempotencyKey: newKey(), Amount: &big,
	}))
	if err != nil || res.Msg.GetPendingDamage().GetConcentrationDc() != 15 {
		t.Fatalf("the damage of 30 on Pensantus = %v, %v; want the DC 15 reminder (half of 30)", res, err)
	}
	// The reminder is in the log for the master and the target's own player, not for others.
	dcOf := func(u *user) (dc int32, found bool) {
		for _, r := range a.log(t, u, e).GetRounds() {
			for _, en := range r.GetEntries() {
				if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_ATTACK && en.GetDamage().ConcentrationDc != nil && en.GetDamage().GetStatus() == playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED && en.GetDamage().GetAmount() == 30 {
					return en.GetDamage().GetConcentrationDc(), true
				}
			}
		}
		return 0, false
	}
	if dc, ok := dcOf(a.master); !ok || dc != 15 {
		t.Errorf("the master's log DC = %d, %v; want 15", dc, ok)
	}
	if dc, ok := dcOf(a.ana); !ok || dc != 15 {
		t.Errorf("Pensantus's player's log DC = %d, %v; want 15", dc, ok)
	}
	if _, ok := dcOf(a.caio); ok {
		t.Error("another player's log has Pensantus's concentration DC")
	}

	// His player ends the concentration; the log says which spell ended; the undo puts it back.
	if _, err := a.conditions(t, a.ana, e, "Pensantus", nil, false, true); err != nil {
		t.Fatalf("SetCombatantConditions(end_concentration) error = %v", err)
	}
	if got := byLabel(t, a.get(t, a.caio), "Pensantus").GetConcentrationSpell(); got != "" {
		t.Errorf("concentration after ending it = %q, want none", got)
	}
	var line *playv1.CombatLogEntry
	for _, en := range a.log(t, a.caio, e).GetRounds()[0].GetEntries() {
		if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_CONDITIONS_CHANGED {
			line = en
		}
	}
	if line == nil || line.GetConcentrationEndedKey() != webSpell || line.GetTargetLabel() != "Pensantus" {
		t.Errorf("the log's line = %v, want Teia ended on Pensantus", line)
	}
	if err := a.undo(t, a.master, e, a.log(t, a.master, e).GetUndoableEventId()); err != nil {
		t.Fatalf("UndoLastAction() error = %v", err)
	}
	if got := byLabel(t, a.get(t, a.caio), "Pensantus").GetConcentrationSpell(); got != webSpell {
		t.Errorf("concentration after the undo = %q, want Teia again", got)
	}
}

func featureOption(o *playv1.GetTurnOptionsResponse, key string) *rulesv1.ActionOption {
	for _, f := range o.GetOptions().GetFeatureActions() {
		if f.GetAction().GetKey() == key {
			return f
		}
	}
	return nil
}

// feature calls TakeAction with a roll, as u.
func (a *armed) feature(t *testing.T, u *user, e *playv1.Encounter, label, key string, roll func(*playv1.TakeActionRequest)) (*playv1.TakeActionResponse, error) {
	t.Helper()
	req := &playv1.TakeActionRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, label), ActionKey: key, IdempotencyKey: newKey()}
	roll(req)
	res, err := u.combat.TakeAction(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func takeInApp(r *playv1.TakeActionRequest) {
	r.Roll = &playv1.TakeActionRequest_RollInApp{RollInApp: true}
}
func noTakeRoll(*playv1.TakeActionRequest) {}

// TestExtraAttackAllowsTwoAttacks: a level 5 fighter makes two attacks with the
// Attack action: the first spends the action, the second is still allowed, the
// third is refused; a fighter of level 2 has one.
func TestExtraAttackAllowsTwoAttacks(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	a.mustEndTurn(t, a.ana, e) // Toren's turn
	o := a.mustOptions(t, a.caio, e, "Toren")
	if eco := o.GetOptions().GetEconomy(); eco.GetAttacksPerAction() != 2 || eco.GetAttacksLeft() != 2 || !attackOption(o, battleaxe).GetEnabled() {
		t.Fatalf("Toren's economy = %v, want two attacks per action and both left", eco)
	}
	first := a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", d20(15))
	if c := byLabel(t, first.GetEncounter(), "Toren"); !c.GetActionUsed() {
		t.Error("the first attack did not spend the action")
	}
	o = a.mustOptions(t, a.caio, e, "Toren")
	if eco := o.GetOptions().GetEconomy(); eco.GetAttacksLeft() != 1 || !attackOption(o, battleaxe).GetEnabled() {
		t.Errorf("after one attack: attacks left %d, option %v; want 1 and still enabled", eco.GetAttacksLeft(), attackOption(o, battleaxe))
	}
	if opt := standardOption(o, "standard:dash"); opt.GetEnabled() || opt.GetReason().GetCode() != rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_ACTION_USED {
		t.Errorf("Disparada after an attack = %v, want the action used", opt)
	}
	a.h.roller.queue(2)
	a.mustDamage(t, a.caio, e, first.GetPendingDamage().GetId(), inAppDamage) // the goblin has 7 PV: 2 + 3 = 5
	second := a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", d20(15))
	if second.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_HIT {
		t.Errorf("the second attack = %v, want a hit", second.GetRoll())
	}
	_, err := a.attack(t, a.caio, e, "Toren", battleaxe, "Goblin", d20(15))
	wantBlockedBy(t, "a third attack", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ATTACKS_USED)
	if o := a.mustOptions(t, a.caio, e, "Toren"); o.GetOptions().GetEconomy().GetAttacksLeft() != 0 || attackOption(o, battleaxe).GetReason().GetCode() != rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_ATTACKS_USED {
		t.Errorf("after both attacks: %v, want none left, ATTACKS_USED", attackOption(o, battleaxe))
	}

	// The master may attack again, and a single-attack character gets ACTION_USED.
	if _, err := a.attack(t, a.master, e, "Toren", battleaxe, "Goblin", d20(15)); err != nil {
		t.Errorf("the master's third attack for Toren: %v, want it allowed", err)
	}
	if _, err := a.endTurn(t, a.master, e, true); err != nil { // the pending damages of Toren's drop with the turn
		t.Errorf("EndTurn(discard) error = %v", err)
	}
}

// TestSecondWindAndActionSurge: the feature actions with a use each: Retomar o
// fôlego heals 1d10 + the fighter's level and spends the bonus action and the
// use; Surto de ação gives the action back, with the attacks of the Attack
// action; a spent resource is NO_USES with its recharge, and the master's
// correction brings a use back.
func TestSecondWindAndActionSurge(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	a.correct(t, a.toren, hpIs(20))
	a.mustEndTurn(t, a.ana, e) // Toren's turn

	o := a.mustOptions(t, a.caio, e, "Toren")
	if sw := featureOption(o, secondWindKey); sw == nil || !sw.GetEnabled() || sw.GetUsesLeft() != 1 || sw.GetAction().GetEconomy() != rulesv1.ActionEconomy_ACTION_ECONOMY_BONUS_ACTION {
		t.Fatalf("Retomar o fôlego = %v, want a bonus action with one use", sw)
	}
	if surge := featureOption(o, actionSurgeKey); surge == nil || !surge.GetEnabled() || surge.GetUsesLeft() != 1 || surge.GetAction().GetEconomy() != rulesv1.ActionEconomy_ACTION_ECONOMY_FREE {
		t.Fatalf("Surto de ação = %v, want a free action with one use", surge)
	}

	// Retomar o fôlego: 1d10 (6) + 5 = 11.
	_, err := a.feature(t, a.caio, e, "Toren", secondWindKey, noTakeRoll)
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("Retomar o fôlego without a roll = %v, want invalid_argument", err)
	}
	a.h.roller.queue(6)
	res, err := a.feature(t, a.caio, e, "Toren", secondWindKey, takeInApp)
	if err != nil {
		t.Fatalf("TakeAction(second wind) error = %v", err)
	}
	if res.GetHealed() != 11 || res.GetRoll().GetFaces()[0] != 6 || res.GetRoll().GetModifier() != 5 || res.GetRoll().GetTotal() != 11 {
		t.Errorf("Retomar o fôlego = healed %d, roll %v; want 1d10 (6) + 5 = 11", res.GetHealed(), res.GetRoll())
	}
	if used, total := resourceUsed(a.vitals(t, a.toren), "second_wind"); used != 1 || total != 1 {
		t.Errorf("second_wind used = %d of %d, want 1 of 1", used, total)
	}
	if got := a.vitals(t, a.toren).GetHitPointsCurrent(); got != 31 {
		t.Errorf("Toren = %d PV, want 31", got)
	}
	if c := byLabel(t, res.GetEncounter(), "Toren"); !c.GetBonusActionUsed() || c.GetActionUsed() {
		t.Errorf("economy after it = bonus %v, action %v; want only the bonus action spent", c.GetBonusActionUsed(), c.GetActionUsed())
	}
	_, err = a.feature(t, a.caio, e, "Toren", secondWindKey, takeInApp)
	wantBlockedBy(t, "Retomar o fôlego with the bonus action used", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_BONUS_ACTION_USED)
	// It cannot go above the maximum, and it is capped (heal 11 more at 40 of 44).

	// Surto de ação: two attacks, then the surge brings the action and the attacks back.
	a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", d20(15))
	a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", d20(3))
	_, err = a.attack(t, a.caio, e, "Toren", battleaxe, "Goblin", d20(15))
	wantBlockedBy(t, "a third attack", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ATTACKS_USED)
	if _, err := a.feature(t, a.caio, e, "Toren", actionSurgeKey, noTakeRoll); err != nil {
		t.Fatalf("TakeAction(action surge) error = %v", err)
	}
	o = a.mustOptions(t, a.caio, e, "Toren")
	if c := byLabel(t, a.get(t, a.caio), "Toren"); c.GetActionUsed() || o.GetOptions().GetEconomy().GetAttacksLeft() != 2 {
		t.Errorf("after Surto de ação: action used %v, attacks left %d; want the action back with two attacks", c.GetActionUsed(), o.GetOptions().GetEconomy().GetAttacksLeft())
	}
	if opt := featureOption(o, actionSurgeKey); opt.GetEnabled() || opt.GetReason().GetCode() != rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_NO_USES || opt.GetReason().GetRecharge() != rulesv1.Recharge_RECHARGE_SHORT_REST {
		t.Errorf("Surto de ação again = %v, want NO_USES, back on a short rest", opt)
	}
	_, err = a.feature(t, a.caio, e, "Toren", actionSurgeKey, noTakeRoll)
	b := wantBlockedBy(t, "a second Surto de ação", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_USES)
	if b.GetRecharge() != rulesv1.Recharge_RECHARGE_SHORT_REST {
		t.Errorf("NO_USES recharge = %v, want short rest", b.GetRecharge())
	}
	a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", d20(15)) // the extra action: two more attacks

	// The master's correction brings the uses back (rests come later).
	a.correct(t, a.toren, func(r *playv1.AdjustCharacterVitalsRequest) {
		r.ResourcesUsed = []*playv1.ResourceUsed{{Key: "action_surge", Used: 0}, {Key: "second_wind", Used: 0}}
	})
	if used, _ := resourceUsed(a.vitals(t, a.toren), "action_surge"); used != 0 {
		t.Errorf("action_surge used after the correction = %d, want 0", used)
	}
	req := &playv1.AdjustCharacterVitalsRequest{CampaignId: a.campaignID, CharacterId: a.toren.GetId(), IdempotencyKey: newKey(), ResourcesUsed: []*playv1.ResourceUsed{{Key: "rage", Used: 1}}}
	if _, err := a.master.play.AdjustCharacterVitals(t.Context(), connect.NewRequest(req)); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a correction of a resource the character does not have: %v, want invalid_argument", err)
	}
}

// TestMasterAppliesADifferentAmount: the master applies another number than the
// rolled one (a save he overrules, a resistance); the log keeps both for him, the
// player sees what they took, and the undo gives the hit points back.
func TestMasterAppliesADifferentAmount(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFightNPCFirst(t) // the Capitão first
	a.h.roller.queue(4)            // 1d6 (4) + 2 = 6
	hit := a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Toren", d20(15))
	pid := hit.GetPendingDamage().GetId()
	a.mustDamage(t, a.master, e, pid, inAppDamage)

	apply := func(u *user, amount *int32) (*playv1.ApplyPendingDamageResponse, error) {
		res, err := u.combat.ApplyPendingDamage(t.Context(), connect.NewRequest(&playv1.ApplyPendingDamageRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), PendingDamageId: pid, IdempotencyKey: newKey(), Amount: amount,
		}))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	}
	for _, bad := range []int32{-1, 10000} {
		if _, err := apply(a.master, &bad); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("ApplyPendingDamage(amount %d) = %v, want invalid_argument", bad, err)
		}
	}
	two := int32(2)
	if _, err := apply(a.caio, &two); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("a player applying: %v, want permission_denied", err)
	}
	before := a.vitals(t, a.toren).GetHitPointsCurrent()
	res, err := apply(a.master, &two)
	if err != nil {
		t.Fatalf("ApplyPendingDamage(2) error = %v", err)
	}
	if d := res.GetPendingDamage(); d.GetAmount() != 6 || d.GetAppliedAmount() != 2 {
		t.Errorf("the pending damage = %v, want rolled 6 and applied 2", d)
	}
	if got := a.vitals(t, a.toren).GetHitPointsCurrent(); got != before-2 {
		t.Errorf("Toren = %d PV, want %d (2 applied, not 6)", got, before-2)
	}
	find := func(u *user) *playv1.CombatLogDamage {
		for _, r := range a.log(t, u, e).GetRounds() {
			for _, en := range r.GetEntries() {
				if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_ATTACK && en.GetDamage().GetStatus() == playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED {
					return en.GetDamage()
				}
			}
		}
		return nil
	}
	if d := find(a.master); d == nil || d.GetAmount() != 2 || d.RolledAmount == nil || d.GetRolledAmount() != 6 {
		t.Errorf("the master's log damage = %v, want 2 applied and the rolled 6", d)
	}
	for who, u := range map[string]*user{"Toren's player": a.caio, "Pensantus's player": a.ana} {
		if d := find(u); d == nil || d.GetAmount() != 2 || d.RolledAmount != nil {
			t.Errorf("%s's log damage = %v, want only the 2 they took", who, d)
		}
	}
	if err := a.undo(t, a.master, e, a.log(t, a.master, e).GetUndoableEventId()); err != nil {
		t.Fatalf("UndoLastAction() error = %v", err)
	}
	if got := a.vitals(t, a.toren).GetHitPointsCurrent(); got != before {
		t.Errorf("Toren after the undo = %d PV, want %d", got, before)
	}
	// Applying the rolled amount again, and 0 (a resisted damage), both fine.
	zero := int32(0)
	if res, err := apply(a.master, &zero); err != nil || res.GetPendingDamage().GetAppliedAmount() != 0 || a.vitals(t, a.toren).GetHitPointsCurrent() != before {
		t.Errorf("ApplyPendingDamage(0) = %v, %v; want it applied with no damage", res, err)
	}
}

// snapshot is everything an action changes and an undo must give back: the
// combat as the master sees it (without its revision), the three characters'
// vitals (without theirs) and the pending damages.
func (a *armed) snapshot(t *testing.T) string {
	t.Helper()
	enc := a.get(t, a.master)
	enc.Revision = 0
	out := asJSON(t, enc)
	for _, c := range []*charactersv1.Character{a.toren, a.pens, a.bri} {
		v := a.vitals(t, c)
		v.Revision, v.UpdatedAt = 0, nil
		out += asJSON(t, v)
	}
	rows, err := a.h.pool.Query(t.Context(), `SELECT id, status, COALESCE(amount, -1), COALESCE(applied_amount, -1), COALESCE(roll_total, -1), faces::TEXT, COALESCE(cast_id::TEXT, ''), half, healing FROM pending_damages ORDER BY created_at, id`)
	if err != nil {
		t.Fatalf("read the pending damages: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, status, faces, cast string
		var amount, applied, total int32
		var half, healing bool
		if err := rows.Scan(&id, &status, &amount, &applied, &total, &faces, &cast, &half, &healing); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out += "|" + id + status + faces + cast + fmt.Sprint(amount, applied, total) + map[bool]string{true: "h", false: "-"}[half] + map[bool]string{true: "H", false: "-"}[healing]
	}
	return out
}

// undoes runs an action, takes it back with the master's Desfazer and checks the
// combat is exactly as before.
func (a *armed) undoes(t *testing.T, name string, do func()) {
	t.Helper()
	before := a.snapshot(t)
	do()
	e := a.get(t, a.master)
	id := a.log(t, a.master, e).GetUndoableEventId()
	if id == "" {
		t.Fatalf("%s: nothing to undo after it", name)
	}
	if after := a.snapshot(t); after == before {
		t.Fatalf("%s changed nothing", name)
	}
	if err := a.undo(t, a.master, e, id); err != nil {
		t.Fatalf("%s: UndoLastAction() error = %v", name, err)
	}
	if after := a.snapshot(t); after != before {
		t.Errorf("%s: the undo left the combat different:\nbefore %s\nafter  %s", name, before, after)
	}
}

// TestCombatUndoTakesBackEveryNewAction: the undo of a spell (its slot, its
// action, its concentration, its pending damages), of a damage roll that settled
// a whole cast, of a heal, a reaction used or declined, a death save, the
// conditions, a feature action and an attack of the Attack action puts every
// number back exactly.
func TestCombatUndoTakesBackEveryNewAction(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	both := a.at(t, "Goblin", "Capitão Goblin")

	// Pensantus's turn.
	a.undoes(t, "Sono", func() {
		a.mustCast(t, a.ana, e, "Pensantus", sleepSpell, slotOfLevel(1), a.at(t, "Goblin"), poolInApp)
	})
	a.undoes(t, "a concentration spell", func() {
		a.mustCast(t, a.ana, e, "Pensantus", holdPerson, slotOfLevel(2), a.at(t, "Goblin"), noCastRoll)
	})
	a.undoes(t, "Mísseis Mágicos", func() {
		a.mustCast(t, a.ana, e, "Pensantus", magicMissileSpell, slotOfLevel(1), []*playv1.SpellTarget{darts(a, t, "Goblin", 2), darts(a, t, "Capitão Goblin", 1)}, noCastRoll)
	})
	// The damage roll of an area spell settles every pending damage of the cast.
	beforeCast := a.snapshot(t)
	cast := a.mustCast(t, a.ana, e, "Pensantus", burningHands, slotOfLevel(1), both, noCastRoll)
	a.undoes(t, "the damage roll of an area spell", func() {
		a.h.roller.queue(3, 3, 3)
		a.mustDamage(t, a.ana, e, cast.GetCast().GetPendingDamages()[0].GetId(), inAppDamage)
	})
	if err := a.undo(t, a.master, e, a.log(t, a.master, e).GetUndoableEventId()); err != nil {
		t.Fatalf("UndoLastAction(the area spell) error = %v", err)
	}
	if got := a.snapshot(t); got != beforeCast {
		t.Errorf("the undo of the area spell left the combat different:\nbefore %s\nafter  %s", beforeCast, got)
	}
	// Ending a concentration, and the conditions, are undone too.
	a.mustCast(t, a.ana, e, "Pensantus", holdPerson, slotOfLevel(2), a.at(t, "Goblin"), noCastRoll)
	a.undoes(t, "ending the concentration", func() {
		if _, err := a.conditions(t, a.ana, e, "Pensantus", nil, false, true); err != nil {
			t.Fatalf("SetCombatantConditions() error = %v", err)
		}
	})
	a.undoes(t, "the conditions", func() {
		if _, err := a.conditions(t, a.master, e, "Goblin", []string{"condition:poisoned"}, true, false); err != nil {
			t.Fatalf("SetCombatantConditions() error = %v", err)
		}
	})

	// Toren's turn: Retomar o fôlego, an attack with Extra Attack, Surto de ação.
	if _, err := a.endTurn(t, a.master, e, false); err != nil { // Hold Person has no pending damage
		t.Fatalf("EndTurn() error = %v", err)
	}
	a.correct(t, a.toren, hpIs(20))
	a.undoes(t, "Retomar o fôlego", func() {
		a.h.roller.queue(6)
		if _, err := a.feature(t, a.caio, e, "Toren", secondWindKey, takeInApp); err != nil {
			t.Fatalf("TakeAction() error = %v", err)
		}
	})
	a.undoes(t, "an attack of the Attack action", func() { a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", d20(15)) })
	a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", d20(15))
	a.undoes(t, "Surto de ação", func() {
		if _, err := a.feature(t, a.caio, e, "Toren", actionSurgeKey, noTakeRoll); err != nil {
			t.Fatalf("TakeAction(surge) error = %v", err)
		}
	})
	a.undoes(t, "an opportunity attack", func() {
		if _, err := a.attackAs(t, a.master, e, "Brisa", maceKey, "Brisa", d20(15), true); err == nil {
			t.Fatal("an attack on itself worked")
		}
		if _, err := a.attackAs(t, a.master, e, "Pensantus", fireBolt, "Goblin", d20(15), true); err == nil {
			t.Fatal("a ranged opportunity attack worked")
		}
		if _, err := a.attackAs(t, a.master, e, "Brisa", maceKey, "Goblin", d20(15), true); err != nil {
			t.Fatalf("RollAttack(as_reaction) error = %v", err)
		}
	})

	// Toren falls (on his own turn the save waits for the next one): a death save
	// and a natural 20 are undone.
	a.correct(t, a.toren, hpIs(0))
	if _, err := a.endTurn(t, a.master, e, true); err != nil {
		t.Fatalf("EndTurn() error = %v", err)
	}
	a.passTo(t, e, "Toren")
	a.undoes(t, "a death save", func() {
		if _, err := a.deathSave(t, a.caio, e, "Toren", deathFace(12)); err != nil {
			t.Fatalf("RollDeathSave() error = %v", err)
		}
	})
	a.undoes(t, "a natural 20", func() {
		if _, err := a.deathSave(t, a.caio, e, "Toren", deathFace(20)); err != nil {
			t.Fatalf("RollDeathSave(20) error = %v", err)
		}
	})
	if _, err := a.deathSave(t, a.caio, e, "Toren", deathFace(1)); err != nil { // two failures stay
		t.Fatalf("RollDeathSave(1) error = %v", err)
	}
	a.passTo(t, e, "Brisa")
	// Brisa's heal reaches Toren at 0: its undo gives back the hit points and the two failures.
	healed := a.mustCast(t, a.bia, e, "Brisa", cureWounds, slotOfLevel(1), a.at(t, "Toren"), noCastRoll)
	a.undoes(t, "a heal", func() {
		a.h.roller.queue(4)
		a.mustDamage(t, a.bia, e, healed.GetCast().GetPendingDamages()[0].GetId(), inAppDamage)
	})
	if c := byLabel(t, a.get(t, a.caio), "Toren"); c.GetDeathFailures() != 2 || a.vitals(t, a.toren).GetHitPointsCurrent() != 0 {
		t.Errorf("Toren after the heal's undo = %d failures, %d PV; want 2 and 0", c.GetDeathFailures(), a.vitals(t, a.toren).GetHitPointsCurrent())
	}

	// The reaction, in its own combat: the Capitão hits Pensantus.
	b := newCasters(t)
	eb := b.castersFightNPCFirst(t)
	hit := b.mustAttack(t, b.master, eb, "Capitão Goblin", sword, "Pensantus", d20(9))
	b.undoes(t, "Escudo", func() {
		if _, err := b.useReaction(t, b.ana, eb, hit.GetPendingDamage().GetId(), slotOfLevel(1)); err != nil {
			t.Fatalf("UseReaction() error = %v", err)
		}
	})
	b.undoes(t, "declining Escudo", func() {
		if _, err := b.declineReaction(t, b.ana, eb, hit.GetPendingDamage().GetId()); err != nil {
			t.Fatalf("DeclineReaction() error = %v", err)
		}
	})
	// And a damage at 0 hit points: a failure that the undo takes away.
	b.correct(t, b.toren, hpIs(0))
	b.h.roller.queue(3)
	crit := b.mustAttack(t, b.master, eb, "Capitão Goblin", sword, "Toren", d20(20))
	b.mustDamage(t, b.master, eb, crit.GetPendingDamage().GetId(), inAppDamage)
	b.undoes(t, "a damage at 0 hit points", func() {
		if _, err := b.settle(t, b.master, eb, crit.GetPendingDamage().GetId(), true); err != nil {
			t.Fatalf("ApplyPendingDamage() error = %v", err)
		}
	})
}

// hiddenCapitaoFight starts the combat with the Capitão first and hidden, the
// goblin revealed and last, then Pensantus, Toren and Brisa.
func (a *armed) hiddenCapitaoFight(t *testing.T) *playv1.Encounter {
	t.Helper()
	return a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.capitao.GetId()}, {CharacterId: a.goblin.GetId()}},
		npcRolls: []int{20, 1},
		players:  map[string]int32{"Pensantus": 10, "Toren": 9, "Brisa": 8},
		reveal:   []string{"Goblin"},
		at:       map[string][2]int32{"Pensantus": {5, 5}, "Toren": {6, 5}, "Brisa": {5, 6}, "Capitão Goblin": {4, 4}, "Goblin": {9, 5}}, // no one between the Capitão and Pensantus: no cover (D4)
	})
}

// TestRN20_PlayersNeverReceiveSpellAndReactionSecrets: through every new call, a
// player gets no armor class, no NPC hit points, no NPC dice and nothing of a
// hidden combatant: not its ID, its label, its place among the targets or the
// attacker of an Escudo prompt.
func TestRN20_PlayersNeverReceiveSpellAndReactionSecrets(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.hiddenCapitaoFight(t)
	hidden := a.id(t, "Capitão Goblin")
	var seen []string
	note := func(who string, messages ...any) {
		for _, m := range messages {
			switch v := m.(type) {
			case interface{ ProtoReflect() protoreflect.Message }:
				seen = append(seen, who+": "+asJSONAny(t, v))
			}
		}
	}

	// The hidden Capitão hits Pensantus: the prompt has no attacker.
	hit := a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Pensantus", d20(9))
	note("Pensantus's player", a.get(t, a.ana), a.log(t, a.ana, e))
	note("Toren's player", a.get(t, a.caio), a.log(t, a.caio, e))
	res, err := a.useReaction(t, a.ana, e, hit.GetPendingDamage().GetId(), slotOfLevel(1))
	if err != nil {
		t.Fatalf("UseReaction() error = %v", err)
	}
	note("Pensantus's player", res, a.get(t, a.ana), a.log(t, a.ana, e))
	note("Toren's player", a.get(t, a.caio), a.log(t, a.caio, e))
	if len(res.GetEncounter().GetReactionPrompts()) != 0 {
		t.Errorf("prompts after answering = %v", res.GetEncounter().GetReactionPrompts())
	}
	// Her log has her Escudo, and nothing of the attack of a hidden combatant.
	var shield *playv1.CombatLogEntry
	for _, r := range a.log(t, a.ana, e).GetRounds() {
		for _, en := range r.GetEntries() {
			if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_ATTACK {
				t.Errorf("a player's log has the hidden Capitão's attack: %v", en)
			}
			if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_REACTION {
				shield = en
			}
		}
	}
	if shield == nil {
		t.Error("Pensantus's player has no line for her own Escudo")
	}

	// Pensantus's turn. A player cannot aim at the hidden Capitão, nor does the list of
	// targets have it; her own cast at the goblin gives her the word of its save, never its dice.
	a.mustEndTurn(t, a.master, e)
	_, err = a.cast(t, a.ana, e, "Pensantus", holdPerson, slotOfLevel(2), []*playv1.SpellTarget{{CombatantId: hidden}}, noCastRoll)
	wantCode(t, "a spell aimed at a hidden combatant", err, connect.CodeNotFound)
	o := a.mustOptions(t, a.ana, a.get(t, a.ana), "Pensantus")
	note("Pensantus's player", o)
	for _, st := range o.GetSpellTargets() {
		if targetOf2(st, "Capitão Goblin") != nil {
			t.Errorf("the targets of %s have the hidden Capitão", st.GetSpellKey())
		}
	}
	if st := spellTargetsOf(o, holdPerson); st == nil || targetOf2(st, "Goblin") == nil {
		t.Errorf("Hold Person's targets = %v, want the revealed goblin among them", st)
	}
	a.h.roller.queue(7)
	cast := a.mustCast(t, a.ana, e, "Pensantus", holdPerson, slotOfLevel(2), a.at(t, "Goblin"), noCastRoll)
	note("Pensantus's player", cast, a.log(t, a.ana, e))
	note("Toren's player", a.log(t, a.caio, e))

	// The master casts for Pensantus at the hidden Capitão and the goblin: the line is his alone.
	a.h.roller.queue(5, 6)
	a.mustCast(t, a.master, e, "Pensantus", burningHands, slotOfLevel(1), a.at(t, "Capitão Goblin", "Goblin"), noCastRoll)
	note("Toren's player", a.get(t, a.caio), a.log(t, a.caio, e))
	for _, r := range a.log(t, a.caio, e).GetRounds() {
		for _, en := range r.GetEntries() {
			if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_SPELL_CAST && en.GetKey() == burningHands {
				t.Errorf("a player's log has a cast that touched a hidden combatant: %v", en)
			}
		}
	}
	for _, text := range seen {
		for _, banned := range []string{`"armorClass"`, "targetArmorClass", "hitPointsCurrent", "hitPointsMax", hidden, "Capitão", a.capitao.GetId()} {
			if strings.Contains(text, banned) {
				t.Errorf("a message has %q: %s", banned, text)
			}
		}
	}
	if tg := cast.GetCast().GetTargets()[0]; tg.GetSave().GetRoll() != nil || tg.GetSave().GetOutcome() == playv1.SaveOutcome_SAVE_OUTCOME_UNSPECIFIED {
		t.Errorf("the goblin's save = %v, want the word and no dice", tg.GetSave())
	}
}

func asJSONAny(t *testing.T, m interface{ ProtoReflect() protoreflect.Message }) string {
	t.Helper()
	return asJSON(t, m.ProtoReflect().Interface())
}

// TestCombatSpellsAreIdempotent: every new write sent twice with the same key
// changes nothing the second time, writes no second event and answers as the
// first one did (a roll is never rolled again until it is good, RN-18).
func TestCombatSpellsAreIdempotent(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	events := func() int { return a.events(t) }

	// CastSpell: the slot once, the same cast.
	key := newKey()
	a.h.roller.queue(5)
	first, err := a.castKey(t, a.ana, e, "Pensantus", burningHands, slotOfLevel(1), a.at(t, "Goblin"), noCastRoll, key)
	if err != nil {
		t.Fatalf("CastSpell() error = %v", err)
	}
	n := events()
	second, err := a.castKey(t, a.ana, e, "Pensantus", burningHands, slotOfLevel(1), a.at(t, "Goblin"), noCastRoll, key)
	if err != nil || events() != n || usedSlots(a.vitals(t, a.pens), 1) != 1 || first.GetCast().GetCastId() != second.GetCast().GetCastId() ||
		first.GetCast().GetTargets()[0].GetSave().GetOutcome() != second.GetCast().GetTargets()[0].GetSave().GetOutcome() {
		t.Errorf("the retry of CastSpell = %v, %v, %d new events; want the same cast and nothing spent twice", second, err, events()-n)
	}
	// RollDamage of the cast: rolled once (the retry would roll 3d6 again).
	a.h.roller.queue(2, 2, 2)
	pid := first.GetCast().GetPendingDamages()[0].GetId()
	dkey := newKey()
	call := func() *playv1.PendingDamage {
		res, err := a.ana.combat.RollDamage(t.Context(), connect.NewRequest(&playv1.RollDamageRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), PendingDamageId: pid, IdempotencyKey: dkey, Roll: &playv1.RollDamageRequest_RollInApp{RollInApp: true},
		}))
		if err != nil {
			t.Fatalf("RollDamage() error = %v", err)
		}
		return res.Msg.GetPendingDamage()
	}
	d1, d2 := call(), call()
	if d1.GetAmount() != 6 || d2.GetAmount() != 6 {
		t.Errorf("the damage and its retry = %d and %d, want 6 (3d6 of 2s, once)", d1.GetAmount(), d2.GetAmount())
	}

	// SetCombatantConditions, TakeAction (a feature), RollDeathSave, UseReaction, DeclineReaction.
	once := func(name string, call func(key string) error) {
		t.Helper()
		key, before := newKey(), events()
		if err := call(key); err != nil {
			t.Fatalf("%s error = %v", name, err)
		}
		if events() != before+1 {
			t.Fatalf("%s wrote %d events, want 1", name, events()-before)
		}
		if err := call(key); err != nil {
			t.Fatalf("%s again error = %v", name, err)
		}
		if events() != before+1 {
			t.Fatalf("the retry of %s wrote an event", name)
		}
	}
	once("SetCombatantConditions", func(key string) error {
		_, err := a.master.combat.SetCombatantConditions(t.Context(), connect.NewRequest(&playv1.SetCombatantConditionsRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Goblin"), IdempotencyKey: key, Conditions: &playv1.ConditionList{Keys: []string{"condition:poisoned"}},
		}))
		return err
	})
	a.correct(t, a.toren, hpIs(20))
	a.mustEndTurn(t, a.ana, e) // Toren's turn... with Burning Hands' damage settled
	a.h.roller.queue(6, 1)
	once("TakeAction(Retomar o fôlego)", func(key string) error {
		_, err := a.caio.combat.TakeAction(t.Context(), connect.NewRequest(&playv1.TakeActionRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Toren"), ActionKey: secondWindKey, IdempotencyKey: key,
			Roll: &playv1.TakeActionRequest_RollInApp{RollInApp: true},
		}))
		return err
	})
	if got := a.vitals(t, a.toren).GetHitPointsCurrent(); got != 31 {
		t.Errorf("Toren = %d PV after Retomar o fôlego and its retry, want 31: healed once", got)
	}
	if used, _ := resourceUsed(a.vitals(t, a.toren), "second_wind"); used != 1 {
		t.Errorf("second_wind used = %d, want 1: spent once", used)
	}
	a.correct(t, a.toren, hpIs(0))
	if _, err := a.endTurn(t, a.master, e, true); err != nil {
		t.Fatalf("EndTurn() error = %v", err)
	}
	a.passTo(t, e, "Toren") // next round: his death save is due now
	once("RollDeathSave", func(key string) error {
		_, err := a.caio.combat.RollDeathSave(t.Context(), connect.NewRequest(&playv1.RollDeathSaveRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Toren"), IdempotencyKey: key, Roll: &playv1.RollDeathSaveRequest_D20Face{D20Face: 12},
		}))
		return err
	})
	if c := byLabel(t, a.get(t, a.caio), "Toren"); c.GetDeathSuccesses() != 1 {
		t.Errorf("Toren's successes = %d after a death save and its retry, want 1", c.GetDeathSuccesses())
	}

	// The reaction, in its own combat.
	b := newCasters(t)
	eb := b.castersFightNPCFirst(t)
	hit := b.mustAttack(t, b.master, eb, "Capitão Goblin", sword, "Pensantus", d20(14))
	once2 := func(name string, call func(key string) error) {
		t.Helper()
		key, before := newKey(), b.events(t)
		if err := call(key); err != nil {
			t.Fatalf("%s error = %v", name, err)
		}
		if err := call(key); err != nil {
			t.Fatalf("%s again error = %v", name, err)
		}
		if b.events(t) != before+1 {
			t.Fatalf("%s and its retry wrote %d events, want 1", name, b.events(t)-before)
		}
	}
	once2("UseReaction", func(key string) error {
		_, err := b.ana.combat.UseReaction(t.Context(), connect.NewRequest(&playv1.UseReactionRequest{
			CampaignId: b.campaignID, EncounterId: eb.GetId(), PendingDamageId: hit.GetPendingDamage().GetId(), Slot: slotOfLevel(1), IdempotencyKey: key,
		}))
		return err
	})
	if got := usedSlots(b.vitals(t, b.pens), 1); got != 1 {
		t.Errorf("slots used after Escudo and its retry = %d, want 1", got)
	}
	// The next hit (the reaction is used, so the damage waits for its roll at once): decline needs a prompt.
	b2 := newCasters(t)
	e2 := b2.castersFightNPCFirst(t)
	hit2 := b2.mustAttack(t, b2.master, e2, "Capitão Goblin", sword, "Pensantus", d20(14))
	key2, before := newKey(), b2.events(t)
	for range 2 {
		if _, err := b2.ana.combat.DeclineReaction(t.Context(), connect.NewRequest(&playv1.DeclineReactionRequest{
			CampaignId: b2.campaignID, EncounterId: e2.GetId(), PendingDamageId: hit2.GetPendingDamage().GetId(), IdempotencyKey: key2,
		})); err != nil {
			t.Fatalf("DeclineReaction() error = %v", err)
		}
	}
	if got := b2.events(t); got != before+1 {
		t.Errorf("DeclineReaction and its retry wrote %d events, want 1", got-before)
	}
}

// TestMR014_SpellsAuthorizationMatrix: who may call the new methods. Signed out
// is unauthenticated, an outsider or a pending member is not_found, another
// player on someone else's combatant is permission_denied, the master's methods
// are the master's, and the owner and the master get past authorization.
func TestMR014_SpellsAuthorizationMatrix(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFightNPCFirst(t)
	outsider := a.h.newUser("Intruso")
	pending := a.h.newUser("Pendente")
	a.h.joinPending(a.master, a.campaignID, pending)
	pending.createCharacter(t, a.campaignID, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Esperando")
	anonymous := a.h.anonymous()

	// The Capitão hits Pensantus: a hit that waits for her reaction.
	hit := a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Pensantus", d20(9))
	pid, campaign, enc := hit.GetPendingDamage().GetId(), a.campaignID, e.GetId()
	toren := a.id(t, "Toren")
	calls := map[string]func(u *user, ctx context.Context) error{
		"CastSpell": func(u *user, ctx context.Context) error {
			_, err := u.combat.CastSpell(ctx, connect.NewRequest(&playv1.CastSpellRequest{
				CampaignId: campaign, EncounterId: enc, CasterId: toren, SpellKey: sleepSpell, Slot: slotOfLevel(1), IdempotencyKey: newKey(),
			}))
			return err
		},
		"UseReaction": func(u *user, ctx context.Context) error {
			_, err := u.combat.UseReaction(ctx, connect.NewRequest(&playv1.UseReactionRequest{CampaignId: campaign, EncounterId: enc, PendingDamageId: pid, Slot: slotOfLevel(1), IdempotencyKey: newKey()}))
			return err
		},
		"DeclineReaction": func(u *user, ctx context.Context) error {
			_, err := u.combat.DeclineReaction(ctx, connect.NewRequest(&playv1.DeclineReactionRequest{CampaignId: campaign, EncounterId: enc, PendingDamageId: pid, IdempotencyKey: newKey()}))
			return err
		},
		"RollDeathSave": func(u *user, ctx context.Context) error {
			_, err := u.combat.RollDeathSave(ctx, connect.NewRequest(&playv1.RollDeathSaveRequest{
				CampaignId: campaign, EncounterId: enc, CombatantId: toren, IdempotencyKey: newKey(), Roll: &playv1.RollDeathSaveRequest_RollInApp{RollInApp: true},
			}))
			return err
		},
		"ConfirmDeath": func(u *user, ctx context.Context) error {
			_, err := u.combat.ConfirmDeath(ctx, connect.NewRequest(&playv1.ConfirmDeathRequest{CampaignId: campaign, EncounterId: enc, CombatantId: toren, IdempotencyKey: newKey()}))
			return err
		},
		"SetCombatantConditions": func(u *user, ctx context.Context) error {
			_, err := u.combat.SetCombatantConditions(ctx, connect.NewRequest(&playv1.SetCombatantConditionsRequest{
				CampaignId: campaign, EncounterId: enc, CombatantId: toren, IdempotencyKey: newKey(), EndConcentration: true,
			}))
			return err
		},
	}
	calls["EndConcentration"] = func(u *user, ctx context.Context) error {
		_, err := u.combat.EndConcentration(ctx, connect.NewRequest(&playv1.EndConcentrationRequest{CampaignId: campaign, EncounterId: enc, CombatantId: toren, IdempotencyKey: newKey()}))
		return err
	}
	methods := playv1.File_meurpg_play_v1_combat_proto.Services().ByName("CombatService").Methods()
	for _, name := range spellRPCs {
		if calls[name] == nil || methods.ByName(protoreflect.Name(name)) == nil {
			t.Errorf("the matrix lacks %s, or the service has no such method", name)
		}
	}
	if len(calls) != len(spellRPCs) {
		t.Errorf("the matrix has %d methods, want %d", len(calls), len(spellRPCs))
	}

	masterOnly := map[string]bool{"ConfirmDeath": true}
	// The reaction belongs to Pensantus's player, so Toren's player is "another
	// player" for it; the rest of the calls name Toren.
	ownerOf := map[string]*user{"UseReaction": a.ana, "DeclineReaction": a.ana, "CastSpell": a.caio, "RollDeathSave": a.caio, "SetCombatantConditions": a.caio, "EndConcentration": a.caio}
	for name, call := range calls {
		if err := call(anonymous, t.Context()); connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Errorf("%s signed out: %v, want unauthenticated", name, err)
		}
		for who, u := range map[string]*user{"outsider": outsider, "pending": pending} {
			if err := call(u, t.Context()); connect.CodeOf(err) != connect.CodeNotFound {
				t.Errorf("%s as %s: %v, want not_found", name, who, err)
			}
		}
		other := a.bia // Brisa's player: neither Toren's nor Pensantus's
		if err := call(other, t.Context()); connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Errorf("%s as another player: %v, want permission_denied", name, err)
		}
	}
	// The owner is never told "not yours"; only the rules may refuse. The reaction
	// calls go last: the owner's answer settles the hit for the master's try.
	for name, call := range calls {
		if masterOnly[name] {
			if err := call(ownerOf["CastSpell"], t.Context()); connect.CodeOf(err) != connect.CodePermissionDenied {
				t.Errorf("%s as a player: %v, want permission_denied", name, err)
			}
			continue
		}
		if err := call(ownerOf[name], t.Context()); connect.CodeOf(err) == connect.CodePermissionDenied || connect.CodeOf(err) == connect.CodeNotFound || connect.CodeOf(err) == connect.CodeUnauthenticated {
			t.Errorf("%s as the character's player: %v, want it past authorization", name, err)
		}
	}
	for name, call := range calls {
		if err := call(a.master, t.Context()); connect.CodeOf(err) == connect.CodePermissionDenied || connect.CodeOf(err) == connect.CodeUnauthenticated || connect.CodeOf(err) == connect.CodeNotFound {
			t.Errorf("%s as the master: %v, want it past authorization", name, err)
		}
	}
}

// TestCombatSpellEventsPerAudience: the stream's hints for the new writes reach
// each audience as for the old ones: the master hears every change, a player a
// line of the log only when no hidden combatant is in it; the vitals hint (the
// slot) reaches the master and the caster's player.
func TestCombatSpellEventsPerAudience(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	// The goblin (revealed by castersFight) is hidden again; the Capitão is in view.
	if _, err := a.master.combat.SetCombatantHidden(t.Context(), connect.NewRequest(&playv1.SetCombatantHiddenRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Goblin"), IdempotencyKey: newKey(), Hidden: true,
	})); err != nil {
		t.Fatalf("SetCombatantHidden() error = %v", err)
	}
	streams := map[string]*watcher{"master": a.master.watch(t, a.campaignID), "caster": a.ana.watch(t, a.campaignID), "other": a.caio.watch(t, a.campaignID)}
	for _, w := range streams {
		w.ready(t)
	}
	goblin := a.id(t, "Goblin")

	// A cast at the visible Capitão, then the master's cast for Pensantus at the hidden goblin.
	a.mustCast(t, a.ana, e, "Pensantus", holdPerson, slotOfLevel(2), a.at(t, "Capitão Goblin"), noCastRoll)
	a.mustCast(t, a.master, e, "Pensantus", webSpell, slotOfLevel(2), a.at(t, "Goblin"), noCastRoll)
	a.mustEndTurn(t, a.master, e) // the sentinel: Toren's turn

	count := func(who string) (logHints, vitals, encounters int) {
		w := streams[who]
		for turns := 0; turns < 1; {
			ev := w.nextChange(t)
			switch {
			case ev.GetCombatLogChanged() != nil:
				logHints++
			case ev.GetVitalsChanged() != nil:
				vitals++
			case ev.GetEncounterChanged() != nil:
				encounters++
			case ev.GetTurnChanged() != nil:
				turns++
			}
			if who != "master" && strings.Contains(protojson.Format(ev), goblin) {
				t.Fatalf("%s's stream has the hidden goblin's ID: %v", who, ev)
			}
		}
		return logHints, vitals, encounters
	}
	if l, v, enc := count("master"); l != 2 || v != 2 || enc != 2 {
		t.Errorf("the master got %d log hints, %d vitals, %d encounter hints; want 2, 2 and 2 (one for each cast)", l, v, enc)
	}
	if l, v, _ := count("caster"); l != 1 || v != 2 {
		t.Errorf("the caster's player got %d log hints, %d vitals; want 1 (the visible cast) and 2 (both slots)", l, v)
	}
	if l, v, _ := count("other"); l != 1 || v != 0 {
		t.Errorf("another player got %d log hints, %d vitals; want 1 and none", l, v)
	}
}

// TestMagicMissileDartIsAlways1d4Plus1: the damage at a higher slot is the whole
// volley (6d4 + 6 at the 4th level), and a dart stays 1d4 + 1 whatever the slot.
func TestMagicMissileDartIsAlways1d4Plus1(t *testing.T) {
	t.Parallel()
	a := newCastersAt(t, 9) // slots up to the 5th level
	e := a.castersFight(t, 1)
	// The 2nd-level slot: 4 darts.
	cast := a.mustCast(t, a.ana, e, "Pensantus", magicMissileSpell, slotOfLevel(2), []*playv1.SpellTarget{darts(a, t, "Goblin", 4)}, noCastRoll)
	if d := cast.GetCast().GetPendingDamages()[0]; d.GetDiceCount() != 4 || d.GetDiceSides() != 4 || d.GetBonus() != 4 {
		t.Errorf("4 darts at the 2nd level = %vd%v+%v, want 4d4+4", d.GetDiceCount(), d.GetDiceSides(), d.GetBonus())
	}
	// The 4th-level slot: 6 darts, shared 4 and 2 (the master casts again).
	cast = a.mustCast(t, a.master, e, "Pensantus", magicMissileSpell, slotOfLevel(4), []*playv1.SpellTarget{darts(a, t, "Goblin", 4), darts(a, t, "Capitão Goblin", 2)}, noCastRoll)
	got := cast.GetCast().GetPendingDamages()
	if len(got) != 2 || got[0].GetDiceCount() != 4 || got[0].GetBonus() != 4 || got[1].GetDiceCount() != 2 || got[1].GetBonus() != 2 || got[1].GetDiceSides() != 4 {
		t.Errorf("6 darts at the 4th level = %v, want 4d4+4 and 2d4+2", got)
	}
	if _, err := a.cast(t, a.master, e, "Pensantus", magicMissileSpell, slotOfLevel(4), []*playv1.SpellTarget{darts(a, t, "Goblin", 5)}, noCastRoll); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("5 darts of 6 = %v, want invalid_argument", err)
	}
}

// TestCantripsAreNotPartOfExtraAttack: Extra Attack belongs to the Attack action,
// that is, to weapons; a cantrip takes the whole action, in every order.
func TestCantripsAreNotPartOfExtraAttack(t *testing.T) {
	t.Parallel()
	a := newArmedWith(t, func(a *armed) {
		sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
			BaseScores: &rulesv1.AbilityScores{Strength: 16, Dexterity: 13, Constitution: 14, Intelligence: 14, Wisdom: 10, Charisma: 8}, RaceKey: "race:human",
			Classes:    []*charactersv1.ClassLevel{{ClassKey: "class:fighter", Level: 5}, {ClassKey: "class:wizard", Level: 1}},
			WeaponKeys: []string{battleaxe}, CantripKeys: []string{fireBolt},
		}}}
		res, err := a.caio.characters.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
			CampaignId: a.campaignID, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Toren", Sheet: sheet,
		}))
		if err != nil {
			t.Fatalf("CreateCharacter() error = %v", err)
		}
		a.toren = res.Msg.GetCharacter()
		a.pens = a.ana.hero(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 1, &rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8}, nil, nil)
		a.bri = a.bia.hero(t, a.campaignID, "Brisa", "class:fighter", "race:human", 2, &rulesv1.AbilityScores{Strength: 10, Dexterity: 16, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{rapier}, nil)
	})
	e := a.start(t, plan{
		npcs: []*playv1.Participant{{CharacterId: a.goblin.GetId()}}, npcRolls: []int{1},
		players: map[string]int32{"Toren": 20, "Pensantus": 10, "Brisa": 5}, reveal: []string{"Goblin"},
		at: map[string][2]int32{"Toren": {6, 5}, "Pensantus": {5, 5}, "Brisa": {5, 6}, "Goblin": {7, 5}},
	})
	// Turn 1: weapon, weapon (Extra Attack), then a cantrip is refused.
	a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", d20(3))
	o := a.mustOptions(t, a.caio, e, "Toren")
	if o.GetOptions().GetEconomy().GetAttacksLeft() != 1 || !attackOption(o, battleaxe).GetEnabled() || attackOption(o, fireBolt).GetEnabled() {
		t.Errorf("after a weapon attack: attacks left %d, axe %v, fire bolt %v; want 1, enabled, disabled",
			o.GetOptions().GetEconomy().GetAttacksLeft(), attackOption(o, battleaxe).GetEnabled(), attackOption(o, fireBolt).GetEnabled())
	}
	_, err := a.attack(t, a.caio, e, "Toren", fireBolt, "Goblin", d20(15))
	wantBlockedBy(t, "a cantrip after a weapon attack", err, blockedActionUsed)
	a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", d20(3))

	// Turn 2: a cantrip first spends the whole action: no weapon, no second cantrip.
	a.passTo(t, e, "Pensantus")
	a.passTo(t, e, "Toren")
	a.mustAttack(t, a.caio, e, "Toren", fireBolt, "Goblin", d20(3))
	if c := byLabel(t, a.get(t, a.caio), "Toren"); !c.GetActionUsed() {
		t.Error("a cantrip did not spend the action")
	}
	o = a.mustOptions(t, a.caio, e, "Toren")
	if o.GetOptions().GetEconomy().GetAttacksLeft() != 0 || attackOption(o, battleaxe).GetEnabled() || attackOption(o, battleaxe).GetReason().GetCode() != rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_ACTION_USED {
		t.Errorf("after a cantrip: attacks left %d, axe %v; want 0 and ACTION_USED", o.GetOptions().GetEconomy().GetAttacksLeft(), attackOption(o, battleaxe))
	}
	_, err = a.attack(t, a.caio, e, "Toren", battleaxe, "Goblin", d20(15))
	wantBlockedBy(t, "a weapon attack after a cantrip", err, blockedActionUsed)
	_, err = a.attack(t, a.caio, e, "Toren", fireBolt, "Goblin", d20(15))
	wantBlockedBy(t, "a second cantrip", err, blockedActionUsed)
}

// TestSpellCastIsLimitedToTenTargets: a cast and the damage roll that settles it
// fit a session event (4 KiB): ten targets work, eleven are refused.
func TestSpellCastIsLimitedToTenTargets(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 10)
	var labels []string
	for i := 1; i <= 10; i++ {
		labels = append(labels, fmt.Sprintf("Goblin %d", i))
	}
	_, err := a.cast(t, a.ana, e, "Pensantus", burningHands, slotOfLevel(1), a.at(t, append(labels, "Capitão Goblin")...), noCastRoll)
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("a cast on 11 targets = %v, want invalid_argument", err)
	}
	cast := a.mustCast(t, a.ana, e, "Pensantus", burningHands, slotOfLevel(1), a.at(t, labels...), noCastRoll)
	if len(cast.GetCast().GetPendingDamages()) != 10 {
		t.Fatalf("pending damages = %d, want 10", len(cast.GetCast().GetPendingDamages()))
	}
	res := a.mustDamage(t, a.ana, e, cast.GetCast().GetPendingDamages()[0].GetId(), inAppDamage)
	if len(res.GetCastPendingDamages()) != 9 {
		t.Errorf("the roll settled %d others, want 9", len(res.GetCastPendingDamages()))
	}
	// Both events were written, so both fit the payload limit.
	var n int
	if err := a.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM session_events WHERE kind IN ('spell_cast', 'damage_rolled')`).Scan(&n); err != nil || n != 2 {
		t.Errorf("events = %d, %v; want the cast and the roll", n, err)
	}
}

// TestHealLogDoesNotLeakTheMissingHitPoints: a heal capped at the maximum shows the
// hit points regained only to the master and the target's player; everyone else
// gets the roll (RN-20).
func TestHealLogDoesNotLeakTheMissingHitPoints(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	a.correct(t, a.toren, hpIs(40)) // of 44: 4 missing
	a.passTo(t, e, "Brisa")
	cast := a.mustCast(t, a.bia, e, "Brisa", cureWounds, slotOfLevel(1), a.at(t, "Toren"), noCastRoll)
	a.h.roller.queue(8) // 8 + 3 = 11
	a.mustDamage(t, a.bia, e, cast.GetCast().GetPendingDamages()[0].GetId(), inAppDamage)
	if got := a.vitals(t, a.toren).GetHitPointsCurrent(); got != 44 {
		t.Fatalf("Toren = %d PV, want 44", got)
	}
	amount := func(u *user) int32 {
		for _, en := range a.log(t, u, e).GetRounds()[0].GetEntries() {
			if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_SPELL_CAST {
				return en.GetSpell().GetTargets()[0].GetDamage().GetAmount()
			}
		}
		t.Fatal("no cast in the log")
		return 0
	}
	for who, want := range map[string]int32{"master": 4, "Toren's player": 4, "Pensantus's player": 11, "Brisa's player": 11} {
		u := map[string]*user{"master": a.master, "Toren's player": a.caio, "Pensantus's player": a.ana, "Brisa's player": a.bia}[who]
		if got := amount(u); got != want {
			t.Errorf("the heal for %s = %d, want %d", who, got, want)
		}
	}
}

// TestNoDeathSaveOnTheTurnTheCharacterDrops: a character that drops to 0 on its
// own turn owes its first death save at the start of its next turn.
func TestNoDeathSaveOnTheTurnTheCharacterDrops(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	a.mustEndTurn(t, a.ana, e) // Toren's turn
	a.correct(t, a.toren, hpIs(0))
	if byLabel(t, a.get(t, a.master), "Toren").GetDeathSaveDue() || byLabel(t, a.get(t, a.caio), "Toren").GetDeathSaveDue() {
		t.Fatal("a death save is due on the turn he dropped")
	}
	if _, err := a.deathSave(t, a.caio, e, "Toren", rollApp); err == nil {
		t.Error("a death save rolled on the turn he dropped")
	}
	a.mustEndTurn(t, a.caio, e) // not blocked
	a.passTo(t, e, "Toren")
	if !byLabel(t, a.get(t, a.caio), "Toren").GetDeathSaveDue() {
		t.Error("the death save is not due at the start of his next turn")
	}
}

// Escudo against a master-hidden attacker is told to the master alone: another player's
// log gets no line and their stream no hint, or they would learn a hidden NPC attacked (RN-10).
func TestShieldAgainstAHiddenAttackerIsNotInOthersLog(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.hiddenCapitaoFight(t) // the Capitão is first and hidden

	other := a.caio.watch(t, a.campaignID)
	other.ready(t)

	lines := func() int {
		n := 0
		for _, r := range a.log(t, a.caio, e).GetRounds() {
			n += len(r.GetEntries())
		}
		return n
	}
	before := lines()

	hit := a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Pensantus", d20(9))
	if _, err := a.useReaction(t, a.ana, e, hit.GetPendingDamage().GetId(), slotOfLevel(1)); err != nil {
		t.Fatalf("UseReaction() error = %v", err)
	}
	a.mustEndTurn(t, a.master, e) // the sentinel: a turn change reaches the stream

	if after := lines(); after != before {
		for _, r := range a.log(t, a.caio, e).GetRounds() {
			for _, en := range r.GetEntries() {
				t.Logf("Toren's log: %v", en)
			}
		}
		t.Errorf("Toren's log has %d lines after a hidden NPC's attack met Pensantus's Escudo, want the %d it had before", after, before)
	}
	hints := 0
	for turns := 0; turns < 1; {
		ev := other.nextChange(t)
		switch {
		case ev.GetCombatLogChanged() != nil:
			hints++
		case ev.GetTurnChanged() != nil:
			turns++
		}
	}
	if hints != 0 {
		t.Errorf("Toren's stream got %d combat_log_changed hints, want none: the only change was the hidden NPC's attack and its Escudo", hints)
	}
}

// A retried CastSpell (same key) is answered from the combat as it stands: a target the
// master hid since is not in it (RN-10).
func TestReplayedCastDoesNotShowAHiddenTarget(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	goblin := a.id(t, "Goblin")

	key := newKey()
	targets := []*playv1.SpellTarget{darts(a, t, "Goblin", 3)}
	first, err := a.castKey(t, a.ana, e, "Pensantus", magicMissileSpell, slotOfLevel(1), targets, noCastRoll, key)
	if err != nil {
		t.Fatalf("CastSpell() error = %v", err)
	}
	a.h.roller.queue(2, 2, 2)
	a.mustDamage(t, a.ana, e, first.GetCast().GetPendingDamages()[0].GetId(), inAppDamage)

	if _, err := a.master.combat.SetCombatantHidden(t.Context(), connect.NewRequest(&playv1.SetCombatantHiddenRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: goblin, IdempotencyKey: newKey(), Hidden: true,
	})); err != nil {
		t.Fatalf("SetCombatantHidden() error = %v", err)
	}

	replay, err := a.castKey(t, a.ana, e, "Pensantus", magicMissileSpell, slotOfLevel(1), targets, noCastRoll, key)
	if err != nil {
		return // refusing the replay leaks nothing
	}
	if strings.Contains(replay.String(), goblin) {
		t.Errorf("the replayed cast shows the hidden goblin %s: %v", goblin, replay.GetCast())
	}
	if n := len(replay.GetCast().GetPendingDamages()); n != 0 {
		t.Errorf("the replayed cast carries %d pending damages of the hidden goblin, want none", n)
	}
}

// Escudo's armor class holds for every hit on the target: a second hit already waiting for
// the reaction, whose total is under the new armor class, does not land.
func TestShieldAlsoStopsTheOtherHitsAwaitingTheReaction(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFightNPCFirst(t)
	before := byLabel(t, e, "Pensantus").GetHitPointsCurrent()
	first := a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Pensantus", d20(9)) // 13 vs AC 12: a hit
	second := a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Pensantus", d20(9))
	for i, r := range []*playv1.RollAttackResponse{first, second} {
		if r.GetPendingDamage().GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_AWAITING_REACTION {
			t.Fatalf("hit %d = %v, want it awaiting the reaction", i+1, r.GetPendingDamage())
		}
	}
	res, err := a.useReaction(t, a.ana, e, first.GetPendingDamage().GetId(), slotOfLevel(1))
	if err != nil {
		t.Fatalf("UseReaction() error = %v", err)
	}
	if res.GetOutcome() != playv1.ReactionOutcome_REACTION_OUTCOME_STOPPED {
		t.Fatalf("outcome = %v, want stopped", res.GetOutcome())
	}
	// Escudo is up (AC 17): the other hit, total 13, would miss, so it must not hurt her.
	if _, err := a.declineReaction(t, a.ana, e, second.GetPendingDamage().GetId()); err != nil {
		t.Logf("DeclineReaction() error = %v", err)
	}
	a.h.roller.queue(7)
	if _, err := a.damage(t, a.master, e, second.GetPendingDamage().GetId(), inAppDamage); err != nil {
		t.Logf("RollDamage() error = %v", err)
	} else if _, err := a.settle(t, a.master, e, second.GetPendingDamage().GetId(), true); err != nil {
		t.Logf("ApplyPendingDamage() error = %v", err)
	}
	if got := byLabel(t, a.get(t, a.master), "Pensantus").GetHitPointsCurrent(); got != before {
		t.Errorf("Pensantus HP = %d, want %d: a hit with total 13 against the Escudo's AC 17 must not land", got, before)
	}
}

// TestActionSurgeIsUsedOncePerTurn: a fighter of level 17 has two uses of Surto
// de ação, but only one in the same turn. The second is refused (the master may
// correct the economy), the option says why, an undo gives the use back and the
// next turn allows it again.
func TestActionSurgeIsUsedOncePerTurn(t *testing.T) {
	t.Parallel()
	a := newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 17,
			&rulesv1.AbilityScores{Strength: 16, Dexterity: 13, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{battleaxe}, nil)
		a.pens = a.ana.hero(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 1,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8}, nil, []string{fireBolt})
		a.bri = a.bia.hero(t, a.campaignID, "Brisa", "class:fighter", "race:human", 2,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 16, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{rapier}, nil)
	})
	e := a.start(t, plan{
		npcs: []*playv1.Participant{{CharacterId: a.goblin.GetId()}}, npcRolls: []int{1}, reveal: []string{"Goblin"},
		at:      map[string][2]int32{"Pensantus": {5, 5}, "Toren": {6, 5}, "Brisa": {5, 6}, "Goblin": {7, 5}},
		players: map[string]int32{"Toren": 20, "Pensantus": 15, "Brisa": 10},
	})

	o := a.mustOptions(t, a.caio, e, "Toren")
	if surge := featureOption(o, actionSurgeKey); surge == nil || !surge.GetEnabled() || surge.GetUsesLeft() != 2 {
		t.Fatalf("Surto de ação = %v, want enabled with 2 uses at fighter level 17", surge)
	}
	a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", d20(3))
	if _, err := a.feature(t, a.caio, e, "Toren", actionSurgeKey, noTakeRoll); err != nil {
		t.Fatalf("first Surto de ação error = %v", err)
	}

	o = a.mustOptions(t, a.caio, e, "Toren")
	surge := featureOption(o, actionSurgeKey)
	if surge == nil || surge.GetEnabled() || surge.GetUsesLeft() != 1 || surge.GetReason().GetCode() != rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_ALREADY_USED_THIS_TURN {
		t.Errorf("Surto de ação after the first use = %v, want disabled as already used this turn, with 1 use left", surge)
	}
	_, err := a.feature(t, a.caio, e, "Toren", actionSurgeKey, noTakeRoll)
	wantBlockedBy(t, "a second Surto de ação in the same turn", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ALREADY_USED_THIS_TURN)

	// Undoing the first use gives the turn's use back.
	a.undoLast(t, e)
	if _, err := a.feature(t, a.caio, e, "Toren", actionSurgeKey, noTakeRoll); err != nil {
		t.Fatalf("Surto de ação after undoing the first use error = %v, want it allowed", err)
	}

	// The next turn allows the last use.
	for range 4 { // Toren (his damage is discarded), Pensantus, Brisa and the goblin
		if _, err := a.endTurn(t, a.master, e, true); err != nil {
			t.Fatalf("EndTurn(discard) error = %v", err)
		}
	}
	o = a.mustOptions(t, a.caio, e, "Toren")
	if surge := featureOption(o, actionSurgeKey); surge == nil || !surge.GetEnabled() || surge.GetUsesLeft() != 1 {
		t.Errorf("Surto de ação in the next turn = %v, want enabled with 1 use left", surge)
	}
}
