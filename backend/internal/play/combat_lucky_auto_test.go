package play

import (
	"strings"
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// The halfling's Lucky on the saving throws the app rolls for a player's character inside someone
// else's call (SRD 5.1, Halfling; combat_lucky.go): a spell's save, a trap's and the Dexterity save
// against Hellish Rebuke. Nobody is asked: the app rolls the 1 again by itself and the new roll
// stands, even another 1; with advantage or disadvantage only the first die that shows a 1 is
// rolled again and the die that counts is picked again. An NPC is never lucky here, even with a
// halfling's sheet. The result and the log keep the reroll for whoever reads the save's dice, and
// for nobody else (RN-10). The trap's arithmetic needs no database; the other tests need
// MEURPG_TEST_DATABASE_URL.

// fullNPC creates an NPC with a full sheet of the race and class given (the master plays it).
func (u *user) fullNPC(t *testing.T, campaignID, name, race, class string, level int32, scores *rulesv1.AbilityScores, weapons []string) *charactersv1.Character {
	t.Helper()
	sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores: scores, RaceKey: race, Classes: []*charactersv1.ClassLevel{{ClassKey: class, Level: level}}, WeaponKeys: weapons,
	}}}
	res, err := u.characters.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: campaignID, Kind: charactersv1.CharacterKind_CHARACTER_KIND_BOSS, Name: name, Sheet: sheet,
	}))
	if err != nil {
		t.Fatalf("CreateCharacter(%s) error = %v", name, err)
	}
	return res.Msg.GetCharacter()
}

// halflingNPC is an NPC halfling fighter with a rapier: its sheet has Lucky, which the app never
// uses for it.
func (u *user) halflingNPC(t *testing.T, campaignID, name string) *charactersv1.Character {
	t.Helper()
	return u.fullNPC(t, campaignID, name, "race:halfling", "class:fighter", 1, abilities(10, 14, 12, 10, 8), []string{rapier})
}

// ---- a trap's save: the arithmetic (no database) ----

// luckyStatue is a trap that asks every creature caught for a Dexterity saving throw (DC 12) and
// burns the ones that fail for 2d6 fire; a pass avoids it.
func luckyStatue(targets rulesv1.TrapTargets) *rulesv1.TrapEffect {
	return &rulesv1.TrapEffect{
		Save: &rulesv1.TrapSaveEffect{
			Ability: rulesv1.Ability_ABILITY_DEXTERITY, Dc: 12, AppliesTo: rulesv1.TrapSaveApplies_TRAP_SAVE_APPLIES_CAUGHT,
			OnFail: &rulesv1.TrapOnFail{Damage: []*rulesv1.TrapDamage{{Dice: "2d6", DamageTypeKey: "damage-type:fire"}}},
			OnPass: rulesv1.TrapPassOutcome_TRAP_PASS_OUTCOME_NONE,
		},
		Targets: targets,
	}
}

// wantTrapLucky checks the d20 the app rolled again on a trap's save.
func wantTrapLucky(t *testing.T, sv trapSaveRoll, index, to int32) {
	t.Helper()
	if sv.lucky == nil || *sv.lucky != (luckyReroll{Index: index, From: 1, To: to}) {
		t.Errorf("the save's reroll = %+v, want die %d from 1 to %d", sv.lucky, index, to)
	}
}

// resolveLuckyStatue fires luckyStatue at one target with the faces given, and wants every face used.
func resolveLuckyStatue(t *testing.T, target trapTarget, faces ...int) trapOutcome {
	t.Helper()
	s := &scripted{faces: faces}
	out, err := resolveTrap(luckyStatue(rulesv1.TrapTargets_TRAP_TARGETS_MANUAL), []trapTarget{target}, combat.CriticalDoubledDice, s.d20, s.dice, nil)
	if err != nil {
		t.Fatalf("resolveTrap() error = %v", err)
	}
	if len(s.faces) != 0 {
		t.Errorf("faces left = %v, want none", s.faces)
	}
	if len(out) != 1 || len(out[0].saves) != 1 {
		t.Fatalf("resolveTrap() = %+v, want one save", out)
	}
	return out[0]
}

// A lucky target's 1 is rolled again (15): 15 + 2 = 17 passes against 12, and nothing burns.
func TestLuckyAuto_Trap_RerollsTheOne(t *testing.T) {
	t.Parallel()
	o := resolveLuckyStatue(t, trapTarget{id: "pipo", save: 2, saveKnown: true, lucky: true}, 1, 15)
	sv := o.saves[0]
	if sv.d20 != 15 || sv.total != 17 || !sv.saved || sv.other != 0 || len(o.damages) != 0 {
		t.Errorf("the save = %+v, damages %v; want 15 + 2 = 17, passed, nothing burnt", sv, o.damages)
	}
	wantTrapLucky(t, sv, 0, 15)
}

// A target that is not lucky keeps its 1: it fails, and the next faces are the fire (3 + 4).
func TestLuckyAuto_Trap_NotLucky(t *testing.T) {
	t.Parallel()
	o := resolveLuckyStatue(t, trapTarget{id: "toren", save: 2, saveKnown: true}, 1, 3, 4)
	sv := o.saves[0]
	if sv.d20 != 1 || sv.saved || sv.lucky != nil || len(o.damages) != 1 || o.damages[0].amount != 7 {
		t.Errorf("the save = %+v, damages %+v; want the 1 kept, failed, 7 fire", sv, o.damages)
	}
}

// With advantage, 1 and 13: only the 1 is rolled again (17), and the die that counts is picked again
// from 17 and 13: the 17, the first.
func TestLuckyAuto_Trap_Advantage(t *testing.T) {
	t.Parallel()
	o := resolveLuckyStatue(t, trapTarget{id: "pipo", save: 2, saveKnown: true, lucky: true, saveMode: combat.ModeAdvantage}, 1, 13, 17)
	sv := o.saves[0]
	if sv.d20 != 17 || sv.other != 13 || sv.counted != 0 || sv.total != 19 || !sv.saved {
		t.Errorf("the save = %+v, want 17 and 13 with the 17 counted (19)", sv)
	}
	wantTrapLucky(t, sv, 0, 17)
}

// With disadvantage, 13 and 1: the second die is rolled again (6), and the lower of 13 and 6 counts:
// 6 + 2 = 8 fails, and the fire is 2 + 2.
func TestLuckyAuto_Trap_DisadvantageRerollsTheSecondDie(t *testing.T) {
	t.Parallel()
	o := resolveLuckyStatue(t, trapTarget{id: "pipo", save: 2, saveKnown: true, lucky: true, saveMode: combat.ModeDisadvantage}, 13, 1, 6, 2, 2)
	sv := o.saves[0]
	if sv.d20 != 6 || sv.other != 13 || sv.counted != 1 || sv.total != 8 || sv.saved || len(o.damages) != 1 || o.damages[0].amount != 4 {
		t.Errorf("the save = %+v, damages %+v; want 13 and 6 with the 6 counted (8), failed, 4 fire", sv, o.damages)
	}
	wantTrapLucky(t, sv, 1, 6)
}

// Both dice show a 1: the first is the one rolled again (14), and 14 counts with advantage (16 passes).
func TestLuckyAuto_Trap_TheFirstOfTwoOnes(t *testing.T) {
	t.Parallel()
	o := resolveLuckyStatue(t, trapTarget{id: "pipo", save: 2, saveKnown: true, lucky: true, saveMode: combat.ModeAdvantage}, 1, 1, 14)
	sv := o.saves[0]
	if sv.d20 != 14 || sv.other != 1 || sv.counted != 0 || sv.total != 16 || !sv.saved {
		t.Errorf("the save = %+v, want 14 and 1 with the 14 counted (16)", sv)
	}
	wantTrapLucky(t, sv, 0, 14)
}

// The new roll stands even when it is another 1: it is never rolled a third time, and the next
// faces are the fire (3 + 4).
func TestLuckyAuto_Trap_AnotherOneStands(t *testing.T) {
	t.Parallel()
	o := resolveLuckyStatue(t, trapTarget{id: "pipo", save: 2, saveKnown: true, lucky: true}, 1, 1, 3, 4)
	sv := o.saves[0]
	if sv.d20 != 1 || sv.saved || len(o.damages) != 1 || o.damages[0].amount != 7 {
		t.Errorf("the save = %+v, damages %+v; want the new 1, failed, 7 fire", sv, o.damages)
	}
	wantTrapLucky(t, sv, 0, 1)
}

// The trap's attack roll is the trap's: its natural 1 misses and is never rolled again, while the
// lucky target's save of the same trap is.
func TestLuckyAuto_Trap_TheTrapsAttackIsNeverRerolled(t *testing.T) {
	t.Parallel()
	e := luckyStatue(rulesv1.TrapTargets_TRAP_TARGETS_MANUAL)
	e.Attack = &rulesv1.TrapAttack{Bonus: 5, Count: 1, Damage: &rulesv1.TrapDamage{Dice: "1d6", DamageTypeKey: "damage-type:piercing"}}
	s := &scripted{faces: []int{1, 1, 15}} // the dart's 1, the save's 1 and its reroll
	out, err := resolveTrap(e, []trapTarget{{id: "pipo", armorClass: 12, save: 2, saveKnown: true, lucky: true}}, combat.CriticalDoubledDice, s.d20, s.dice, nil)
	if err != nil {
		t.Fatalf("resolveTrap() error = %v", err)
	}
	o := out[0]
	if len(o.attacks) != 1 || o.attacks[0].d20 != 1 || o.attacks[0].hit || len(o.saves) != 1 || o.saves[0].d20 != 15 || len(s.faces) != 0 {
		t.Errorf("the firing = %+v, faces left %v; want the dart's 1 missing and the save's 1 rolled again to 15", o, s.faces)
	}
	wantTrapLucky(t, o.saves[0], 0, 15)
}

// ---- a trap's save: in a combat and out of one ----

// trapSaveOf is the save of the creature with the label in the last trap line of the log as u reads it.
func trapSaveOf(t *testing.T, r *trapRig, u *user, label string) *playv1.TrapSaveRoll {
	t.Helper()
	var out *playv1.TrapSaveRoll
	for _, l := range logEntries(r.log(t, u, r.get(t, u))) {
		if l.GetKind() != playv1.CombatLogKind_COMBAT_LOG_KIND_TRAP_TRIGGERED {
			continue
		}
		for _, c := range l.GetTrap().GetCaught() {
			if c.GetTargetLabel() == label {
				out = c.GetSave()
			}
		}
	}
	if out == nil {
		t.Fatalf("the log has no trap save of %s", label)
	}
	return out
}

// The master fires the statue at Brisa (a halfling, Bia's, Dexterity save +4) and Toren (a human,
// Caio's) in a combat: Brisa's 1 is rolled again to 15 and she passes; Toren's 1 stands and he
// fails. The firing and the log keep the reroll with Brisa's dice, which only the master and Bia
// read (RN-10): Caio and Ana read how her save ended, nothing more.
func TestLuckyAuto_Trap_InACombat(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	statue := r.trap(t, "Estátua de Fogo", 15, 12, func(s *mapsv1.TrapSpec) {
		s.Trigger = rulesv1.TrapTrigger_TRAP_TRIGGER_MANUAL
		s.Effect = luckyStatue(rulesv1.TrapTargets_TRAP_TARGETS_MANUAL)
	})
	r.fight(t)
	r.h.roller.queue(1, 15, 1, 3, 4) // Brisa 1 and 15; Toren 1, and the fire 3 + 4
	res, err := r.fireByHand(t, statue, r.id(t, "Brisa"), r.id(t, "Toren"))
	if err != nil {
		t.Fatalf("FireTrap() error = %v", err)
	}
	caught := res.GetFiring().GetCaught()
	if len(caught) != 2 {
		t.Fatalf("the firing = %v, want Brisa and Toren", res.GetFiring())
	}
	bri, tor := caught[0].GetSave(), caught[1].GetSave()
	if f := bri.GetRoll().GetFaces(); bri.GetOutcome() != playv1.SaveOutcome_SAVE_OUTCOME_SAVED || len(f) != 1 || f[0] != 15 || bri.GetRoll().GetTotal() != 15+bri.GetRoll().GetModifier() {
		t.Errorf("Brisa's save = %v, want the 15 counted, passed", bri)
	}
	wantLuckyReroll(t, "Brisa's save in the master's answer", bri.GetRoll(), 0, 15)
	if f := tor.GetRoll().GetFaces(); tor.GetOutcome() != playv1.SaveOutcome_SAVE_OUTCOME_FAILED || len(f) != 1 || f[0] != 1 || len(tor.GetRoll().GetRerolled()) != 0 {
		t.Errorf("Toren's save = %v, want the 1 kept and failed: he is not a halfling", tor)
	}
	for name, u := range map[string]*user{"master": r.master, "Bia": r.bia} {
		wantLuckyReroll(t, name+"'s log line", trapSaveOf(t, r, u, "Brisa").GetRoll(), 0, 15)
	}
	for name, u := range map[string]*user{"Caio": r.caio, "Ana": r.ana} {
		if sv := trapSaveOf(t, r, u, "Brisa"); sv.GetRoll() != nil || sv.GetOutcome() != playv1.SaveOutcome_SAVE_OUTCOME_SAVED {
			t.Errorf("%s reads Brisa's save as %v, want the outcome alone", name, sv)
		}
	}
}

// An NPC with a halfling's sheet caught by the trap keeps its 1: the app rolls no d20 again for an NPC.
func TestLuckyAuto_Trap_NPC(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	statue := r.trap(t, "Estátua de Fogo", 15, 12, func(s *mapsv1.TrapSpec) {
		s.Trigger = rulesv1.TrapTrigger_TRAP_TRIGGER_MANUAL
		s.Effect = luckyStatue(rulesv1.TrapTargets_TRAP_TARGETS_MANUAL)
	})
	npc := r.master.halflingNPC(t, r.campaignID, "Pequeno")
	r.start(t, plan{
		npcs: []*playv1.Participant{{CharacterId: npc.GetId()}}, npcRolls: []int{2},
		players: map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1},
		reveal:  []string{"Pequeno"},
		at:      map[string][2]int32{"Toren": {6, 7}, "Pensantus": {5, 8}, "Brisa": {4, 7}, "Pequeno": {3, 8}},
	})
	r.h.roller.queue(1, 15, 3, 4)
	res, err := r.fireByHand(t, statue, r.id(t, "Pequeno"))
	if err != nil {
		t.Fatalf("FireTrap() error = %v", err)
	}
	sv := res.GetFiring().GetCaught()[0].GetSave()
	if f := sv.GetRoll().GetFaces(); sv.GetOutcome() != playv1.SaveOutcome_SAVE_OUTCOME_FAILED || len(f) != 1 || f[0] != 1 || len(sv.GetRoll().GetRerolled()) != 0 {
		t.Errorf("the NPC's save = %v, want the 1 kept and failed", sv)
	}
}

// Out of a combat, the statue fires at the tokens in its area: Brisa's 1 is rolled again to 15, and
// the firing keeps the reroll with her dice.
func TestLuckyAuto_Trap_OutsideACombat(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	statue := r.trap(t, "Estátua de Fogo", 12, 7, func(s *mapsv1.TrapSpec) {
		s.Trigger = rulesv1.TrapTrigger_TRAP_TRIGGER_MANUAL
		s.Effect = luckyStatue(rulesv1.TrapTargets_TRAP_TARGETS_AREA)
	})
	r.place(t, r.bri.GetId(), 12, 7)
	r.h.roller.queue(1, 15)
	res, err := r.fireByHand(t, statue)
	if err != nil {
		t.Fatalf("FireTrap() error = %v", err)
	}
	caught := res.GetFiring().GetCaught()
	if len(caught) != 1 || caught[0].GetTargetLabel() != "Brisa" {
		t.Fatalf("the firing = %v, want Brisa", res.GetFiring())
	}
	sv := caught[0].GetSave()
	if f := sv.GetRoll().GetFaces(); sv.GetOutcome() != playv1.SaveOutcome_SAVE_OUTCOME_SAVED || len(f) != 1 || f[0] != 15 {
		t.Errorf("Brisa's save = %v, want the 15 counted, passed", sv)
	}
	wantLuckyReroll(t, "Brisa's save out of a combat", sv.GetRoll(), 0, 15)
	if d := r.trapDamages(t); len(d) != 0 {
		t.Errorf("trap damages = %v, want none: she passed", d)
	}
}

// ---- a spell's save ----

// luckySaveFight starts a combat of the casters' party (newCastersOf, every hero of race; "" keeps
// each one's own) with the goblin far away, and, with withNPC, Pequeno (halflingNPC) next to Toren,
// on first's turn. Brisa's Sacred Flame (DC 13, a Dexterity save, no cover) reaches everyone.
func luckySaveFight(t *testing.T, race, first string, withNPC bool) (*armed, *playv1.Encounter) {
	t.Helper()
	a := newCastersOf(t, 3, race)
	faces := map[string]int32{"Toren": 6, "Pensantus": 4, "Brisa": 5}
	faces[first] = 20
	p := plan{
		npcs: []*playv1.Participant{{CharacterId: a.goblin.GetId()}}, npcRolls: []int{1},
		players: faces, reveal: []string{"Goblin"},
		at: map[string][2]int32{"Brisa": {3, 3}, "Toren": {5, 3}, "Pensantus": {3, 5}, "Goblin": {9, 9}},
	}
	if withNPC {
		npc := a.master.halflingNPC(t, a.campaignID, "Pequeno")
		p.npcs = append(p.npcs, &playv1.Participant{CharacterId: npc.GetId()})
		p.npcRolls = append(p.npcRolls, 1)
		p.reveal = append(p.reveal, "Pequeno")
		p.at["Pequeno"] = [2]int32{6, 4}
	}
	e := a.start(t, p)
	passed := a.passTo(t, e, first)
	return a, passed
}

// flameAt has Brisa cast Sacred Flame at the target, as Bia.
func (a *armed) flameAt(t *testing.T, e *playv1.Encounter, target string) *playv1.CastSpellResponse {
	t.Helper()
	return a.mustCast(t, a.bia, e, "Brisa", sacredFlame, nil, a.at(t, target), noCastRoll)
}

// castSaveOf is the save of the target with the label in the last cast of the log as u reads it.
func (a *armed) castSaveOf(t *testing.T, u *user, e *playv1.Encounter, label string) *playv1.SaveResult {
	t.Helper()
	var out *playv1.SaveResult
	for _, l := range logEntries(a.log(t, u, e)) {
		if l.GetKind() != playv1.CombatLogKind_COMBAT_LOG_KIND_SPELL_CAST {
			continue
		}
		for _, tg := range l.GetSpell().GetTargets() {
			if tg.GetTargetLabel() == label {
				out = tg.GetSave()
			}
		}
	}
	if out == nil {
		t.Fatalf("the log has no save of %s", label)
	}
	return out
}

// Brisa's Sacred Flame on Toren, a halfling (Caio's): his 1 is rolled again by the app to 15 and he
// passes (the 1 would have failed). The roll keeps the reroll for the master and Caio, who read
// Toren's dice; Bia, who cast, and Ana read how the save ended, with no dice and no reroll (RN-10).
func TestLuckyAuto_SpellSave_RerollsTheOne(t *testing.T) {
	t.Parallel()
	a, e := luckySaveFight(t, "race:halfling", "Brisa", false)
	a.h.roller.queue(1, 15)
	res := a.flameAt(t, e, "Toren")
	if sv := res.GetCast().GetTargets()[0].GetSave(); sv.GetOutcome() != playv1.SaveOutcome_SAVE_OUTCOME_SAVED || sv.GetRoll() != nil {
		t.Errorf("the caster's player reads Toren's save as %v, want the outcome alone", sv)
	}
	for name, u := range map[string]*user{"master": a.master, "Caio": a.caio} {
		sv := a.castSaveOf(t, u, e, "Toren")
		roll := sv.GetRoll()
		if f := roll.GetFaces(); sv.GetOutcome() != playv1.SaveOutcome_SAVE_OUTCOME_SAVED || len(f) != 1 || f[0] != 15 || roll.GetTotal() != 15+roll.GetModifier() {
			t.Errorf("%s reads Toren's save as %v, want the 15 counted, passed", name, sv)
		}
		wantLuckyReroll(t, name+"'s log line", roll, 0, 15)
	}
	for name, u := range map[string]*user{"Bia": a.bia, "Ana": a.ana} {
		if sv := a.castSaveOf(t, u, e, "Toren"); sv.GetRoll() != nil {
			t.Errorf("%s reads Toren's dice %v, want none", name, sv.GetRoll())
		}
	}
}

// A human's 1 stands: no die is rolled again, and the save fails.
func TestLuckyAuto_SpellSave_NotAHalfling(t *testing.T) {
	t.Parallel()
	a, e := luckySaveFight(t, "", "Brisa", false)
	a.h.roller.queue(1, 15)
	a.flameAt(t, e, "Toren")
	sv := a.castSaveOf(t, a.master, e, "Toren")
	if f := sv.GetRoll().GetFaces(); sv.GetOutcome() != playv1.SaveOutcome_SAVE_OUTCOME_FAILED || len(f) != 1 || f[0] != 1 || len(sv.GetRoll().GetRerolled()) != 0 {
		t.Errorf("Toren's save = %v, want the 1 kept and failed", sv)
	}
}

// Toren dodged on his turn (advantage on Dexterity saves, SRD 5.1): the app rolls 1 and 13, rolls
// only the 1 again (17), and the die that counts is picked again from 17 and 13: the 17.
func TestLuckyAuto_SpellSave_Advantage(t *testing.T) {
	t.Parallel()
	a, e := luckySaveFight(t, "race:halfling", "Toren", false)
	if _, err := a.action(t, a.caio, e, "Toren", "standard:dodge"); err != nil {
		t.Fatalf("Toren's Dodge: %v", err)
	}
	e = a.passTo(t, e, "Brisa")
	a.h.roller.queue(1, 13, 17)
	a.flameAt(t, e, "Toren")
	sv := a.castSaveOf(t, a.master, e, "Toren")
	roll := sv.GetRoll()
	if f := roll.GetFaces(); sv.GetMode() != playv1.RollMode_ROLL_MODE_ADVANTAGE || len(f) != 2 || f[0] != 17 || f[1] != 13 || roll.GetCountedIndex() != 0 || roll.GetTotal() != 17+roll.GetModifier() {
		t.Errorf("Toren's save = %v, want 17 and 13 with advantage, the 17 counted", sv)
	}
	wantLuckyReroll(t, "Toren's save", roll, 0, 17)
}

// The new roll stands even when it is another 1: it is never rolled again, and the save fails.
func TestLuckyAuto_SpellSave_AnotherOneStands(t *testing.T) {
	t.Parallel()
	a, e := luckySaveFight(t, "race:halfling", "Brisa", false)
	a.h.roller.queue(1, 1, 20)
	a.flameAt(t, e, "Toren")
	sv := a.castSaveOf(t, a.master, e, "Toren")
	if f := sv.GetRoll().GetFaces(); sv.GetOutcome() != playv1.SaveOutcome_SAVE_OUTCOME_FAILED || len(f) != 1 || f[0] != 1 {
		t.Errorf("Toren's save = %v, want the new 1, failed", sv)
	}
	wantLuckyReroll(t, "Toren's save", sv.GetRoll(), 0, 1)
}

// An NPC with a halfling's sheet keeps its 1: the app rolls no d20 again for an NPC.
func TestLuckyAuto_SpellSave_NPC(t *testing.T) {
	t.Parallel()
	a, e := luckySaveFight(t, "", "Brisa", true)
	a.h.roller.queue(1, 15)
	a.flameAt(t, e, "Pequeno")
	sv := a.castSaveOf(t, a.master, e, "Pequeno")
	if f := sv.GetRoll().GetFaces(); sv.GetOutcome() != playv1.SaveOutcome_SAVE_OUTCOME_FAILED || len(f) != 1 || f[0] != 1 || len(sv.GetRoll().GetRerolled()) != 0 {
		t.Errorf("the NPC's save = %v, want the 1 kept and failed", sv)
	}
}

// ---- the Dexterity save against Hellish Rebuke ----

// rebukeFight is newLuckyTable with Diabrete, an NPC tiefling fighter 3 (the Infernal Legacy), next
// to Pipo (a halfling, Caio's) and Toren (a human, Ana's), on first's turn.
func rebukeFight(t *testing.T, first string) (*armed, *playv1.Encounter) {
	t.Helper()
	a := newLuckyTable(t)
	npc := a.master.fullNPC(t, a.campaignID, "Diabrete", "race:tiefling", "class:fighter", 3, abilities(10, 12, 14, 10, 14), []string{rapier})
	faces := map[string]int32{"Pipo": 5, "Toren": 4, "Orla": 3}
	faces[first] = 20
	e := a.start(t, plan{
		npcs: []*playv1.Participant{{CharacterId: npc.GetId()}}, npcRolls: []int{1},
		players: faces, reveal: []string{"Diabrete"},
		at: map[string][2]int32{"Pipo": {4, 3}, "Toren": {3, 4}, "Orla": {5, 3}, "Diabrete": {4, 4}},
	})
	passed := a.passTo(t, e, first)
	return a, passed
}

// rebukeOn has the master hit Diabrete with who's battleaxe and answer Diabrete's Hellish Rebuke with
// the Infernal Legacy; faces are what the app rolls then (the save's d20 and the fire's 3d10).
func (a *armed) rebukeOn(t *testing.T, e *playv1.Encounter, who string, faces ...int) *playv1.HellishRebukeResult {
	t.Helper()
	hit := a.mustAttack(t, a.master, e, who, battleaxe, "Diabrete", d20(15))
	a.mustDamage(t, a.master, e, hit.GetPendingDamage().GetId(), typedDamage(3))
	w := a.windowOf(t, a.master, playv1.ReactionKind_REACTION_KIND_HELLISH_REBUKE)
	if w == nil {
		t.Fatal("Diabrete has no Hellish Rebuke window after being hurt")
	}
	a.h.roller.queue(faces...)
	res := a.mustAnswer(t, a.master, e, w.GetId(), func(r *playv1.AnswerReactionRequest) {
		r.Answer, r.UseRacial = playv1.ReactionChoice_REACTION_CHOICE_USE, true
	})
	out := res.GetResult().GetHellishRebuke()
	if out.GetSave() == nil {
		t.Fatalf("the result = %v, want the save rolled at once: the aggressor is a player's character", res.GetResult())
	}
	return out
}

// logSays says whether a reaction line of the log, as u reads it, has the words.
func (a *armed) logSays(t *testing.T, u *user, e *playv1.Encounter, words string) bool {
	t.Helper()
	for _, l := range logEntries(a.log(t, u, e)) {
		if strings.Contains(l.GetReactionTextPt(), words) {
			return true
		}
	}
	return false
}

// Pipo hits Diabrete, which answers with Hellish Rebuke: Pipo's 1 on the Dexterity save is rolled
// again by the app to 12. The master's answer and his log line have the reroll with the numbers;
// the players read how the save ended, with neither (RN-10).
func TestLuckyAuto_HellishRebuke_RerollsTheOne(t *testing.T) {
	t.Parallel()
	a, e := rebukeFight(t, "Pipo")
	r := a.rebukeOn(t, e, "Pipo", 1, 12, 5, 5, 5)
	save := r.GetSave()
	if f := save.GetFaces(); len(f) != 1 || f[0] != 12 || save.GetTotal() != 12+save.GetModifier() || r.GetSaved() != (save.GetTotal() >= r.GetSaveDc()) {
		t.Errorf("the save = %v (DC %d, saved %v), want the 12 counted", save, r.GetSaveDc(), r.GetSaved())
	}
	wantLuckyReroll(t, "the save in the master's answer", save, 0, 12)
	if !a.logSays(t, a.master, e, "Sortudo: 1 → 12.") {
		t.Error("the master's log has no \"Sortudo: 1 → 12.\"")
	}
	for name, u := range map[string]*user{"Caio": a.caio, "Ana": a.ana} {
		if a.logSays(t, u, e, "Sortudo") {
			t.Errorf("%s reads the reroll of a save whose dice are the master's", name)
		}
		if !a.logSays(t, u, e, "Repreensão Infernal") {
			t.Errorf("%s reads no Hellish Rebuke line", name)
		}
	}
}

// A human's 1 stands: no die is rolled again.
func TestLuckyAuto_HellishRebuke_NotAHalfling(t *testing.T) {
	t.Parallel()
	a, e := rebukeFight(t, "Toren")
	save := a.rebukeOn(t, e, "Toren", 1, 12, 5, 5, 5).GetSave()
	if f := save.GetFaces(); len(f) != 1 || f[0] != 1 || len(save.GetRerolled()) != 0 {
		t.Errorf("Toren's save = %v, want the 1 kept", save)
	}
	if a.logSays(t, a.master, e, "Sortudo") {
		t.Error("the master's log says Lucky for a human")
	}
}

// The new roll stands even when it is another 1: it is never rolled again.
func TestLuckyAuto_HellishRebuke_AnotherOneStands(t *testing.T) {
	t.Parallel()
	a, e := rebukeFight(t, "Pipo")
	r := a.rebukeOn(t, e, "Pipo", 1, 1, 9, 5, 5)
	if f := r.GetSave().GetFaces(); len(f) != 1 || f[0] != 1 || r.GetSaved() {
		t.Errorf("Pipo's save = %v (saved %v), want the new 1, failed", r.GetSave(), r.GetSaved())
	}
	wantLuckyReroll(t, "Pipo's save", r.GetSave(), 0, 1)
}

// An NPC aggressor's save is the master's (the second step): an NPC with a halfling's sheet that
// hurt a tiefling keeps its 1, even rolled in the app.
func TestLuckyAuto_HellishRebuke_NPC(t *testing.T) {
	t.Parallel()
	a := newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 5, abilities(16, 13, 14, 10, 8), []string{battleaxe}, nil)
		a.pens = tieflingFighter(t, a)
		a.bri = a.bia.hero(t, a.campaignID, "Brisa", "class:fighter", "race:human", 2, abilities(10, 16, 14, 10, 8), []string{rapier}, nil)
	})
	npc := a.master.halflingNPC(t, a.campaignID, "Pequeno")
	e := a.start(t, plan{
		npcs: []*playv1.Participant{{CharacterId: npc.GetId()}}, npcRolls: []int{20},
		players: map[string]int32{"Toren": 3, "Reator": 2, "Brisa": 1},
		reveal:  []string{"Pequeno"},
		at:      map[string][2]int32{"Toren": {3, 3}, "Pequeno": {4, 3}, "Reator": {4, 4}, "Brisa": {8, 8}},
	})
	e = a.passTo(t, e, "Pequeno")
	hit := a.mustAttack(t, a.master, e, "Pequeno", rapier, "Reator", d20(15))
	a.mustDamage(t, a.master, e, hit.GetPendingDamage().GetId(), typedDamage(3))
	if _, err := a.settle(t, a.master, e, hit.GetPendingDamage().GetId(), true); err != nil {
		t.Fatalf("ApplyPendingDamage() error = %v", err)
	}
	w := a.windowOf(t, a.ana, playv1.ReactionKind_REACTION_KIND_HELLISH_REBUKE)
	if w == nil {
		t.Fatal("the tiefling has no Hellish Rebuke window after being hurt")
	}
	a.mustAnswer(t, a.ana, e, w.GetId(), func(r *playv1.AnswerReactionRequest) {
		r.Answer, r.UseRacial = playv1.ReactionChoice_REACTION_CHOICE_USE, true
	})
	second := a.windowOf(t, a.master, playv1.ReactionKind_REACTION_KIND_HELLISH_REBUKE)
	if second == nil || !second.GetSecondStep() {
		t.Fatalf("the master's window = %v, want the second step", second)
	}
	a.h.roller.queue(1, 9, 5, 5)
	done := a.mustAnswer(t, a.master, e, second.GetId(), func(r *playv1.AnswerReactionRequest) {
		r.Answer, r.Roll = playv1.ReactionChoice_REACTION_CHOICE_USE, &playv1.AnswerReactionRequest_RollInApp{RollInApp: true}
	})
	save := done.GetResult().GetHellishRebuke().GetSave()
	if f := save.GetFaces(); len(f) != 1 || f[0] != 1 || len(save.GetRerolled()) != 0 {
		t.Errorf("the NPC's save = %v, want the 1 kept", save)
	}
}
