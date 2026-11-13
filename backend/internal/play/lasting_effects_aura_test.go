package play

import (
	"encoding/json"
	"strings"
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// An aura (#579, slice 2): a table spell on the caster that reaches the creatures within a radius of them, on one side,
// where they stand when the roll happens. The spell is "Marca de Nanquim" of the effect tests, kept on the caster. The
// map: Pensantus (the bearer) at 5,5, Toren and Brisa next to her, the Capitão Goblin on her diagonal, the Goblin two
// squares away. These tests need the database.

// auraShape makes the mark an aura of the radius and side around the caster.
func auraShape(radius int32, affects rulesv1.TableAuraAffects, visibility rulesv1.TableEffectAudience) func(*rulesv1.TableSpell) {
	return func(s *rulesv1.TableSpell) {
		s.Target = tableTarget(rulesv1.TableSpellTargetKind_TABLE_SPELL_TARGET_KIND_SELF)
		s.Range = &rulesv1.TableSpellRange{Kind: rulesv1.SpellRangeKind_SPELL_RANGE_KIND_SELF}
		s.Effect.Aura = &rulesv1.TableEffectAura{RadiusFt: radius, Affects: affects}
		s.Effect.Visibility = visibility
	}
}

var (
	allBonus = &tableMod{
		Kind: rulesv1.TableEffectModifierKind_TABLE_EFFECT_MODIFIER_KIND_ROLL_BONUS, Value: 2,
		AppliesTo: rollKinds(rulesv1.TableEffectRollKind_TABLE_EFFECT_ROLL_KIND_ATTACK, rulesv1.TableEffectRollKind_TABLE_EFFECT_ROLL_KIND_SAVE, rulesv1.TableEffectRollKind_TABLE_EFFECT_ROLL_KIND_CHECK),
	}
	publicAura = rulesv1.TableEffectAudience_TABLE_EFFECT_AUDIENCE_PUBLIC
)

// castAura has Pensantus cast the mark on herself in the first round.
func (a *armed) castAura(t *testing.T) *playv1.Encounter {
	t.Helper()
	e := a.castersFight(t, 1)
	a.mustCast(t, a.ana, e, "Pensantus", tableMark, slotOfLevel(1), nil, noCastRoll)
	return e
}

// moveTo is the master's move of a combatant to a square.
func (a *armed) moveTo(t *testing.T, e *playv1.Encounter, label string, col, row int32) {
	t.Helper()
	if _, err := a.master.combat.MoveCombatant(t.Context(), connect.NewRequest(&playv1.MoveCombatantRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, label), IdempotencyKey: newKey(), Col: col, Row: row,
	})); err != nil {
		t.Fatalf("MoveCombatant(%s) error = %v", label, err)
	}
}

// The bonus is read where the creature stands when it rolls: inside the radius on the bearer's side, +2; the
// bearer is not reached by an aura of allies, and neither is a creature past the radius or on the other side.
func TestAnAuraGivesItsBonusWhereTheCreatureStandsWhenItRolls(t *testing.T) {
	t.Parallel()
	a := newMarkCastersWith(t, auraShape(5, rulesv1.TableAuraAffects_TABLE_AURA_AFFECTS_ALLIES, publicAura), allBonus)
	e := a.castAura(t)
	toren, pens := a.toren.GetId(), a.pens.GetId()

	bonus := func(id string) int {
		cm := a.checkModeFor(t, id, "skill:perception", "wis", true)
		n := 0
		for _, b := range cm.Bonuses {
			n += b.Value
		}
		return n
	}
	if got := bonus(toren); got != 2 {
		t.Errorf("Toren next to the bearer: check bonus = %d, want 2", got)
	}
	if got := bonus(pens); got != 0 {
		t.Errorf("the bearer of an aura of allies: check bonus = %d, want 0 (an aura of allies is not on its bearer)", got)
	}
	// His attack: the battleaxe is +6, +8 with the aura.
	a.passTo(t, e, "Toren")
	res := a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", func(r *playv1.RollAttackRequest) { r.D20Faces = []int32{12} })
	if got := res.GetRoll().GetD20().GetModifier(); got != 8 {
		t.Errorf("the attack inside the aura: modifier = %d, want 8", got)
	}
	// He steps out of it (15 ft from her): the next roll is the sheet's.
	a.moveTo(t, e, "Toren", 8, 5)
	if got := bonus(toren); got != 0 {
		t.Errorf("Toren 15 ft away: check bonus = %d, want 0", got)
	}
	res = a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", func(r *playv1.RollAttackRequest) { r.D20Faces = []int32{12} })
	if got := res.GetRoll().GetD20().GetModifier(); got != 6 {
		t.Errorf("the attack outside the aura: modifier = %d, want 6", got)
	}
	// And back: the aura reaches whoever is in the radius, wherever they came from (the exact edge counts).
	a.moveTo(t, e, "Toren", 6, 6)
	if got := bonus(toren); got != 2 {
		t.Errorf("Toren back at the diagonal: check bonus = %d, want 2", got)
	}
	a.moveTo(t, e, "Toren", 7, 5)
	if got := bonus(toren); got != 0 {
		t.Errorf("Toren one square past the radius (10 ft): check bonus = %d, want 0", got)
	}
}

// The sides are the bearer's: an aura of enemies reaches the other side only, and an aura of the bearer alone,
// the bearer.
func TestAnAuraReachesTheSideItNames(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name    string
		affects rulesv1.TableAuraAffects
		want    map[string]int // the check bonus by label
	}{
		{"allies", rulesv1.TableAuraAffects_TABLE_AURA_AFFECTS_ALLIES, map[string]int{"Pensantus": 0, "Toren": 2, "Brisa": 2}},
		{"self", rulesv1.TableAuraAffects_TABLE_AURA_AFFECTS_SELF, map[string]int{"Pensantus": 2, "Toren": 0, "Brisa": 0}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a := newMarkCastersWith(t, auraShape(10, tt.affects, publicAura), allBonus)
			a.castAura(t)
			for label, want := range tt.want {
				id := map[string]string{"Pensantus": a.pens.GetId(), "Toren": a.toren.GetId(), "Brisa": a.bri.GetId()}[label]
				got := 0
				for _, b := range a.checkModeFor(t, id, "skill:perception", "wis", true).Bonuses {
					got += b.Value
				}
				if got != want {
					t.Errorf("%s: check bonus = %d, want %d", label, got, want)
				}
			}
		})
	}
}

// An aura of enemies reaches the goblin side: the Capitão (5 ft off the bearer) saves with disadvantage, the Goblin
// two squares away does not.
func TestAnAuraOfEnemiesGivesTheOtherSideDisadvantage(t *testing.T) {
	t.Parallel()
	a := newMarkCastersWith(t, auraShape(5, rulesv1.TableAuraAffects_TABLE_AURA_AFFECTS_ENEMIES, publicAura),
		&tableMod{Kind: rulesv1.TableEffectModifierKind_TABLE_EFFECT_MODIFIER_KIND_SAVE_DISADVANTAGE, Abilities: []string{"dex"}})
	e := a.castAura(t)
	a.passTo(t, e, "Brisa")
	a.h.roller.queue(15, 4)
	cast := a.mustCast(t, a.master, e, "Brisa", sacredFlame, nil, a.at(t, "Capitão Goblin"), noCastRoll)
	sv := cast.GetCast().GetTargets()[0].GetSave()
	if sv.GetMode() != playv1.RollMode_ROLL_MODE_DISADVANTAGE || !hasSourceKind(sv.GetSources(), playv1.AdvantageSourceKind_ADVANTAGE_SOURCE_KIND_EFFECT_SAVE, playv1.RollMode_ROLL_MODE_DISADVANTAGE) {
		t.Errorf("the save of the Capitão inside the aura = %v, want disadvantage from the aura", sv)
	}
	a.h.roller.queue(15)
	cast = a.mustCast(t, a.master, e, "Brisa", sacredFlame, nil, a.at(t, "Goblin"), noCastRoll)
	if sv := cast.GetCast().GetTargets()[0].GetSave(); sv.GetMode() != playv1.RollMode_ROLL_MODE_NORMAL {
		t.Errorf("the save of the Goblin outside the aura = %v, want a normal roll", sv)
	}
	// The bearer's own side is not reached, whoever stands next to them.
	if cm := a.checkModeFor(t, a.toren.GetId(), "", "dex", false); cm.Mode != combat.ModeNormal {
		t.Errorf("Toren, an ally of the bearer: mode = %v, want normal", cm.Mode)
	}
}

// Two auras of the one spell reach a creature once (the same source gives once), and two sources give both.
func TestTwoAurasOfOneSourceGiveOnce(t *testing.T) {
	t.Parallel()
	at := func(id, side string, col, row int32) playdb.Combatant {
		return playdb.Combatant{ID: id, Side: side, GridCol: &col, GridRow: &row}
	}
	mark := func(id, owner, source string) playdb.CombatantState {
		mods, _ := json.Marshal([]rules.EffectModifier{
			{Kind: rules.ModifierAura, RadiusFt: 10, Affects: rules.AuraAffectsAllies},
			{Kind: rules.ModifierRollBonus, Value: 2, AppliesTo: []string{rules.RollAppliesCheck}},
		})
		return playdb.CombatantState{ID: id, CombatantID: owner, Kind: stateEffect, SourceKey: &source, Modifiers: mods, PlayerVisible: true}
	}
	cs := []playdb.Combatant{at("a", "players", 1, 1), at("b", "players", 3, 1), at("c", "players", 2, 1), at("far", "players", 20, 20), at("foe", "npcs", 2, 2)}
	same := withAuras(map[string][]playdb.CombatantState{"a": {mark("r1", "a", "spell:x")}, "b": {mark("r2", "b", "spell:x")}}, cs, nil)
	if got := len(checkBonusesOf(same, "c", "skill:perception", true)); got != 1 {
		t.Errorf("a creature between two auras of one spell has %d bonuses, want 1", got)
	}
	both := withAuras(map[string][]playdb.CombatantState{"a": {mark("r1", "a", "spell:x")}, "b": {mark("r2", "b", "spell:y")}}, cs, nil)
	if got := len(checkBonusesOf(both, "c", "skill:perception", true)); got != 2 {
		t.Errorf("a creature between two auras of two spells has %d bonuses, want 2", got)
	}
	for _, id := range []string{"far", "foe"} {
		if got := len(checkBonusesOf(both, id, "skill:perception", true)); got != 0 {
			t.Errorf("%s is out of both auras and has %d bonuses, want 0", id, got)
		}
	}
	// A bearer is reached by the aura of another, not by its own: its own row and the one view of the other's.
	if got := len(same["a"]); got != 2 || len(checkBonusesOf(same, "a", "skill:perception", true)) != 1 {
		t.Errorf("the bearer a has %d rows and %d bonuses, want its aura row and the other's one bonus", got, len(checkBonusesOf(same, "a", "skill:perception", true)))
	}
	// Nothing about who is inside was written: the map that was given has what it had.
	if got := len(checkBonusesOf(map[string][]playdb.CombatantState{"a": {mark("r1", "a", "spell:x")}}, "a", "skill:perception", true)); got != 0 {
		t.Errorf("the bearer's own aura row gives it %d bonuses before withAuras, want 0", got)
	}
	// Without a grid (RN-25) nobody has a square, and nobody but the master can say who is inside.
	unplaced := []playdb.Combatant{{ID: "a", Side: "players"}, {ID: "c", Side: "players"}}
	if got := len(checkBonusesOf(withAuras(map[string][]playdb.CombatantState{"a": {mark("r1", "a", "spell:x")}}, unplaced, nil), "c", "skill:perception", true)); got != 0 {
		t.Errorf("a creature with no square has %d bonuses from an aura, want 0", got)
	}
	plain := map[string][]playdb.CombatantState{"a": {{ID: "x", Kind: stateEffect}}}
	if got := withAuras(plain, cs, nil); len(got) != 1 || len(got["a"]) != 1 {
		t.Errorf("a combat with no aura read as %v", got)
	}
}

// effectsAs lists the lasting effects of the combat as the user reads them: the master's table, a player's the
// effects on the combatants of the view.
func (a *armed) effectsAs(t *testing.T, u *user, e *playv1.Encounter) []*playv1.LastingEffect {
	t.Helper()
	if u != a.master {
		var out []*playv1.LastingEffect
		for _, c := range a.get(t, u).GetCombatants() {
			out = append(out, c.GetEffects()...)
		}
		return out
	}
	res, err := u.lasting.ListLastingEffects(t.Context(), connect.NewRequest(&playv1.ListLastingEffectsRequest{CampaignId: a.campaignID, EncounterId: e.GetId()}))
	if err != nil {
		t.Fatalf("ListLastingEffects() error = %v", err)
	}
	return res.Msg.GetEffects()
}

// The damage is worked out where the target stands when it lands: an aura of resistance on the enemies of the bearer
// halves the goblin 10 ft from her, and a goblin that is further takes it all.
func TestAnAuraOfResistanceHalvesTheDamageInsideTheRadiusOnly(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name     string
		col, row int32 // the goblin's square; Toren stands next to it
		want     int32  // the hit points the goblin (7) has left after 9 slashing
	}{
		{"inside, 10 ft from the bearer", 7, 5, 3},
		{"outside, 15 ft from the bearer", 8, 5, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a := newMarkCastersWith(t, auraShape(10, rulesv1.TableAuraAffects_TABLE_AURA_AFFECTS_ENEMIES, publicAura), &tableMod{
				Kind: rulesv1.TableEffectModifierKind_TABLE_EFFECT_MODIFIER_KIND_RESISTANCE, ResistanceKind: rulesv1.TableResistanceKind_TABLE_RESISTANCE_KIND_RESIST,
				DamageTypes: []string{"damage-type:slashing"},
			})
			e := a.castAura(t)
			a.moveTo(t, e, "Goblin", tt.col, tt.row)
			a.moveTo(t, e, "Toren", tt.col-1, tt.row)
			a.passTo(t, e, "Toren")
			res := a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", func(r *playv1.RollAttackRequest) { r.D20Faces = []int32{15} })
			a.mustDamage(t, a.caio, e, res.GetPendingDamage().GetId(), typedDamage(6))
			if got, _, _ := a.hp(t, "Goblin"); got != tt.want {
				t.Errorf("the goblin has %d hit points after the hit, want %d", got, tt.want)
			}
		})
	}
}

// RN-10: a player reads an aura's bonus on their own roll as "Outra fonte" when the effect is the bearer's alone (the
// default), and the name when the master made it public; never the radius, whose card only the master and the
// bearer's player read. The master and the bearer's player are the positive controls.
func TestAnAuraNamesItselfAndItsRadiusToNoOneWhoMayNotRead(t *testing.T) {
	t.Parallel()
	const radius, bonus = 25, 13
	owner := rulesv1.TableEffectAudience_TABLE_EFFECT_AUDIENCE_OWNER
	a := newMarkCastersWith(t, auraShape(radius, rulesv1.TableAuraAffects_TABLE_AURA_AFFECTS_ALLIES, owner), &tableMod{
		Kind: rulesv1.TableEffectModifierKind_TABLE_EFFECT_MODIFIER_KIND_ROLL_BONUS, Value: bonus,
		AppliesTo: rollKinds(rulesv1.TableEffectRollKind_TABLE_EFFECT_ROLL_KIND_ATTACK),
	})
	e := a.castAura(t)
	a.passTo(t, e, "Toren")
	res := a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", func(r *playv1.RollAttackRequest) { r.D20Faces = []int32{12} })
	srcs := sourceTextsOf(res.GetRoll().GetSources())
	if !strings.Contains(srcs, "Outra fonte +13") || strings.Contains(srcs, "Nanquim") {
		t.Errorf("Toren's player reads the sources %q, want \"Outra fonte +13\" and no name", srcs)
	}
	if got := res.GetRoll().GetD20().GetModifier(); got != 6+bonus {
		t.Errorf("the roll's modifier = %d, want the bonus counted (%d)", got, 6+bonus)
	}
	for _, tt := range []struct {
		name      string
		u         *user
		wantCard  bool
		wantRange bool
	}{
		{"the master", a.master, true, true},
		{"the bearer's player", a.ana, true, true},
		{"another player", a.caio, false, false},
	} {
		var card *playv1.LastingEffect
		for _, f := range a.effectsAs(t, tt.u, e) {
			if f.GetSourceKey() == tableMark {
				card = f
			}
		}
		if tt.wantCard != (card != nil) {
			t.Errorf("%s: reads the aura's card = %v, want %v", tt.name, card != nil, tt.wantCard)
			continue
		}
		if card == nil {
			continue
		}
		if got := card.GetRadiusFt() == radius && card.GetAffects() == "allies"; got != tt.wantRange {
			t.Errorf("%s: reads the radius and the side = %v, want %v (radius %d, affects %q)", tt.name, got, tt.wantRange, card.GetRadiusFt(), card.GetAffects())
		}
	}
	// Whatever Caio reads of the effects names no radius: not in the cards, not in the tags.
	for _, f := range a.effectsAs(t, a.caio, e) {
		if f.GetRadiusFt() != 0 || f.GetAffects() != "" || strings.Contains(strings.Join(f.GetTagsPt(), "|"), "Aura") {
			t.Errorf("Caio reads an aura's reach on %q: radius %d, affects %q, tags %q", f.GetSourceKey(), f.GetRadiusFt(), f.GetAffects(), f.GetTagsPt())
		}
	}
}

// An aura ends as its row does: the master ends the effect and the next roll is the sheet's.
func TestAnAuraEndsWithItsRow(t *testing.T) {
	t.Parallel()
	a := newMarkCastersWith(t, auraShape(10, rulesv1.TableAuraAffects_TABLE_AURA_AFFECTS_ALLIES, publicAura), allBonus)
	e := a.castAura(t)
	check := func() int {
		n := 0
		for _, b := range a.checkModeFor(t, a.toren.GetId(), "skill:perception", "wis", true).Bonuses {
			n += b.Value
		}
		return n
	}
	if got := check(); got != 2 {
		t.Fatalf("before it ends: check bonus = %d, want 2", got)
	}
	var id string
	for _, f := range a.effectsAs(t, a.master, e) {
		if f.GetSourceKey() == tableMark {
			id = f.GetId()
		}
	}
	if id == "" {
		t.Fatal("the master reads no aura row")
	}
	if _, err := a.master.lasting.EndLastingEffect(t.Context(), connect.NewRequest(&playv1.EndLastingEffectRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), EffectId: id, Scope: playv1.EffectEndScope_EFFECT_END_SCOPE_THIS, IdempotencyKey: newKey(),
	})); err != nil {
		t.Fatalf("EndLastingEffect() error = %v", err)
	}
	if got := check(); got != 0 {
		t.Errorf("after it ends: check bonus = %d, want 0", got)
	}
}
