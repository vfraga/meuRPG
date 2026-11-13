package play

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// The effect of a table spell when it is cast (RN-22, #579): who gets it, the temporary hit points and the armor
// class it gives, and how it ends. The spell is "Marca de Nanquim" of lasting_effects_tablespell_test.go.

const (
	modAC   = rulesv1.TableEffectModifierKind_TABLE_EFFECT_MODIFIER_KIND_AC_BONUS
	modTemp = rulesv1.TableEffectModifierKind_TABLE_EFFECT_MODIFIER_KIND_TEMP_HP
)

func selfShape(s *rulesv1.TableSpell) {
	s.Range = &rulesv1.TableSpellRange{Kind: rulesv1.SpellRangeKind_SPELL_RANGE_KIND_SELF}
	s.Target = tableTarget(rulesv1.TableSpellTargetKind_TABLE_SPELL_TARGET_KIND_SELF)
}

// newSelfMark is Pensantus with a self spell that gives +2 armor class and 5 temporary hit points for a minute.
func newSelfMark(t *testing.T) *armed {
	t.Helper()
	return newMarkCastersWith(t, selfShape, &tableMod{Kind: modAC, Value: 2}, &tableMod{Kind: modTemp, Value: 5})
}

func (a *armed) selfMarked(t *testing.T) *playv1.Encounter {
	t.Helper()
	e := a.castersFight(t, 1)
	a.mustCast(t, a.ana, e, "Pensantus", tableMark, slotOfLevel(1), nil, noCastRoll)
	return a.get(t, a.master)
}

// wantMarked checks the caster's armor class bonus, temporary hit points and card.
func (a *armed) wantMarked(t *testing.T, label string, marked bool) {
	t.Helper()
	c := byLabel(t, a.get(t, a.master), label)
	bonus, temp, card := c.GetArmorClassBonus(), c.GetHitPointsTemporary(), cardOf(t, c, tableMark) != nil
	if marked && (bonus != 2 || temp != 5 || !card) {
		t.Errorf("%s: AC bonus %d, temp %d, card %v; want 2, 5 and the card", label, bonus, temp, card)
	}
	if !marked && (bonus != 0 || temp != 0 || card) {
		t.Errorf("%s: AC bonus %d, temp %d, card %v; want none of them", label, bonus, temp, card)
	}
}

func TestATableSpellOnSelfPutsItOnTheCasterInCombat(t *testing.T) {
	t.Parallel()
	a := newSelfMark(t)
	a.selfMarked(t)
	a.wantMarked(t, "Pensantus", true)
	card := cardOf(t, byLabel(t, a.get(t, a.master), "Pensantus"), tableMark)
	if !card.GetConcentration() || card.GetRoundsLeft() != 10 {
		t.Errorf("the card = concentration %v, %d rounds left; want a concentration of 10 rounds", card.GetConcentration(), card.GetRoundsLeft())
	}
}

func TestATableSpellOnSelfEndsWithTheConcentration(t *testing.T) {
	t.Parallel()
	a := newSelfMark(t)
	e := a.selfMarked(t)
	if _, err := a.ana.combat.EndConcentration(t.Context(), connect.NewRequest(&playv1.EndConcentrationRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Pensantus"), IdempotencyKey: newKey(),
	})); err != nil {
		t.Fatalf("EndConcentration() error = %v", err)
	}
	a.wantMarked(t, "Pensantus", false)
}

// The 1 minute of the spell is 10 rounds: it ends on the caster's turn of the 11th round.
func TestATableSpellOnSelfEndsByTheClockAfterTenRounds(t *testing.T) {
	t.Parallel()
	a := newSelfMark(t)
	e := a.selfMarked(t)
	for range 80 {
		cur := a.get(t, a.master)
		if cur.GetRound() >= 11 {
			break
		}
		if _, err := a.endTurn(t, a.master, e, true); err != nil {
			t.Fatalf("EndTurn() error = %v", err)
		}
	}
	a.wantMarked(t, "Pensantus", false)
	if got := a.get(t, a.master).GetRound(); got < 11 {
		t.Fatalf("the fight is at round %d, want at least 11", got)
	}
}

func TestATableSpellOnSelfIsTakenBackByTheUndo(t *testing.T) {
	t.Parallel()
	a := newSelfMark(t)
	e := a.selfMarked(t)
	if err := a.undo(t, a.master, e, a.log(t, a.master, e).GetUndoableEventId()); err != nil {
		t.Fatalf("UndoLastAction() error = %v", err)
	}
	a.wantMarked(t, "Pensantus", false)
}

// Out of a combat the self spell is a row of the caster that gives the hit points and takes them back with the clock.
func TestATableSpellOnSelfOutsideACombat(t *testing.T) {
	t.Parallel()
	a := newSelfMark(t)
	a.mustCastOut(t, a.ana, a.pens, tableMark, slotOfLevel(1), false, nil)
	if n := a.effectRows(t, a.pens.GetId(), tableMark); n != 1 {
		t.Fatalf("Pensantus has %d rows of the spell, want 1", n)
	}
	if v := a.vitals(t, a.pens); v.GetHitPointsTemporary() != 5 {
		t.Fatalf("temporary hit points = %d, want 5", v.GetHitPointsTemporary())
	}
	a.advanceGameTime(t, 61)
	if n := a.effectRows(t, a.pens.GetId(), tableMark); n != 0 {
		t.Errorf("Pensantus has %d rows after a minute, want none", n)
	}
	if v := a.vitals(t, a.pens); v.GetHitPointsTemporary() != 0 {
		t.Errorf("temporary hit points = %d after the effect ended, want 0", v.GetHitPointsTemporary())
	}
}

func (a *armed) setTemp(t *testing.T, c *charactersv1.Character, n int32) {
	t.Helper()
	a.correct(t, c, func(r *playv1.AdjustCharacterVitalsRequest) { r.HitPointsTemporary = &n })
}

func (a *armed) endMarkConcentration(t *testing.T, e *playv1.Encounter) {
	t.Helper()
	if _, err := a.ana.combat.EndConcentration(t.Context(), connect.NewRequest(&playv1.EndConcentrationRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Pensantus"), IdempotencyKey: newKey(),
	})); err != nil {
		t.Fatalf("EndConcentration() error = %v", err)
	}
}

// The temporary hit points do not stack: a pool larger than the effect's (or equal to it) is kept, and it stays
// when the effect ends, because the effect put nothing there.
func TestATableSpellTempHPKeepsAPoolThatIsNotItsOwn(t *testing.T) {
	t.Parallel()
	for _, pool := range []int32{8, 5} {
		a := newSelfMark(t)
		e := a.castersFight(t, 1)
		a.setTemp(t, a.pens, pool)
		a.mustCast(t, a.ana, e, "Pensantus", tableMark, slotOfLevel(1), nil, noCastRoll)
		if _, temp, _ := a.hp(t, "Pensantus"); temp != pool {
			t.Errorf("pool %d: temporary hit points = %d after the cast, want it kept", pool, temp)
		}
		a.endMarkConcentration(t, e)
		if cardOf(t, byLabel(t, a.get(t, a.master), "Pensantus"), tableMark) != nil {
			t.Errorf("pool %d: the effect is still on the caster", pool)
		}
		if _, temp, _ := a.hp(t, "Pensantus"); temp != pool {
			t.Errorf("pool %d: temporary hit points = %d after the effect ended, want it kept", pool, temp)
		}
	}
}

// A pool of 3 that the effect raises to 5 goes back to 3 with the undo of the cast.
func TestATableSpellTempHPUndoRestoresThePreviousPool(t *testing.T) {
	t.Parallel()
	a := newSelfMark(t)
	e := a.castersFight(t, 1)
	a.setTemp(t, a.pens, 3)
	a.mustCast(t, a.ana, e, "Pensantus", tableMark, slotOfLevel(1), nil, noCastRoll)
	if _, temp, _ := a.hp(t, "Pensantus"); temp != 5 {
		t.Fatalf("temporary hit points = %d after the cast, want 5", temp)
	}
	if err := a.undo(t, a.master, e, a.log(t, a.master, e).GetUndoableEventId()); err != nil {
		t.Fatalf("UndoLastAction() error = %v", err)
	}
	if _, temp, _ := a.hp(t, "Pensantus"); temp != 3 {
		t.Errorf("temporary hit points = %d after the undo, want 3", temp)
	}
}

// Two effects on one creature: the one that raised the pool last owns it, so the older ending leaves it be.
func newTwoMarks(t *testing.T, firstTemp, secondTemp int32) *armed {
	t.Helper()
	return newArmedWith(t, func(a *armed) {
		add := func(name string, value int32, concentration bool) string {
			sp := tableSpell(name, 1, "class:wizard", rulesv1.SpellRangeKind_SPELL_RANGE_KIND_SELF, tableTarget(rulesv1.TableSpellTargetKind_TABLE_SPELL_TARGET_KIND_SELF))
			sp.Concentration = concentration
			sp.Duration = &rulesv1.TableSpellDuration{Kind: rulesv1.SpellDurationKind_SPELL_DURATION_KIND_TIMED, Amount: 1, Unit: rulesv1.SpellDurationUnit_SPELL_DURATION_UNIT_MINUTE, UpTo: concentration}
			sp.Effect = &rulesv1.TableSpellEffect{Modifiers: []*tableMod{{Kind: modTemp, Value: value}}, Visibility: rulesv1.TableEffectAudience_TABLE_EFFECT_AUDIENCE_PUBLIC}
			if _, err := a.master.table.CreateTableEntry(t.Context(), connect.NewRequest(&rulesv1.CreateTableEntryRequest{CampaignId: a.campaignID, Body: &rulesv1.CreateTableEntryRequest_TableSpell{TableSpell: sp}})); err != nil {
				t.Fatalf("CreateTableEntry(%s) error = %v", name, err)
			}
			return "spell:" + strings.ReplaceAll(strings.ToLower(name), " ", "-") + "@mesa"
		}
		first, second := add("Marca de Nanquim", firstTemp, true), add("Marca de Cinza", secondTemp, false)
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 5,
			&rulesv1.AbilityScores{Strength: 16, Dexterity: 13, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{battleaxe}, nil)
		book := []string{first, second}
		a.pens = a.ana.caster(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 5,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8}, nil, []string{fireBolt}, book, book)
		a.bri = a.bia.caster(t, a.campaignID, "Brisa", "class:cleric", "race:human", 3,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 16, Constitution: 14, Intelligence: 10, Wisdom: 16, Charisma: 8}, []string{maceKey}, []string{sacredFlame}, nil, nil)
	})
}

func TestATableSpellOlderEffectEndingLeavesTheNewerPool(t *testing.T) {
	t.Parallel()
	a := newTwoMarks(t, 10, 6)
	e := a.castersFight(t, 1)
	a.mustCast(t, a.ana, e, "Pensantus", tableMark, slotOfLevel(1), nil, noCastRoll)
	a.setTemp(t, a.pens, 4) // damage took the 10 down to 4
	a.passTo(t, e, "Toren")
	a.passTo(t, e, "Pensantus")
	a.mustCast(t, a.ana, e, "Pensantus", "spell:marca-de-cinza@mesa", slotOfLevel(1), nil, noCastRoll)
	if _, temp, _ := a.hp(t, "Pensantus"); temp != 6 {
		t.Fatalf("temporary hit points = %d after the second effect, want 6", temp)
	}
	a.endMarkConcentration(t, e)
	if _, temp, _ := a.hp(t, "Pensantus"); temp != 6 {
		t.Errorf("temporary hit points = %d after the older effect ended, want the newer effect's 6", temp)
	}
}

// A creature of the combat (an NPC holds its own hit points): the damage of the cast and the temporary hit points
// both land, the effect's end takes the pool back, and the undo puts the hit points as they were.
func TestATableSpellTempHPOnACreature(t *testing.T) {
	t.Parallel()
	newA := func() *armed {
		return newMarkCastersWith(t, func(s *rulesv1.TableSpell) {
			s.Save = &rulesv1.SpellSave{Ability: rulesv1.Ability_ABILITY_WISDOM, OnSuccess: rulesv1.SpellSaveSuccess_SPELL_SAVE_SUCCESS_NONE}
			s.Damage = []*rulesv1.TableSpellDamage{{DamageTypeKey: "damage-type:necrotic", Dice: "1d4"}}
		}, &tableMod{Kind: modTemp, Value: 5})
	}
	cast := func(a *armed) (*playv1.Encounter, string) {
		e := a.castersFight(t, 1)
		a.h.roller.queue(1) // the goblin fails the save
		res := a.mustCast(t, a.ana, e, "Pensantus", tableMark, slotOfLevel(1), a.at(t, "Goblin"), noCastRoll)
		return e, res.GetCast().GetPendingDamages()[0].GetId()
	}
	t.Run("grant and end", func(t *testing.T) {
		t.Parallel()
		a := newA()
		e, pending := cast(a)
		if hp, temp, _ := a.hp(t, "Goblin"); hp != 7 || temp != 5 {
			t.Fatalf("goblin = %d hit points, %d temporary after the cast; want 7 and 5", hp, temp)
		}
		// The damage of the cast comes after the pool and the pool takes it first: 3 of the 5.
		a.mustDamage(t, a.master, e, pending, typedDamage(3))
		if hp, temp, _ := a.hp(t, "Goblin"); hp != 7 || temp != 2 {
			t.Fatalf("goblin = %d hit points, %d temporary after the damage; want 7 and 2", hp, temp)
		}
		a.endMarkConcentration(t, e)
		if hp, temp, _ := a.hp(t, "Goblin"); hp != 7 || temp != 0 {
			t.Errorf("goblin = %d hit points, %d temporary after the end; want 7 and 0", hp, temp)
		}
	})
	t.Run("undo", func(t *testing.T) {
		t.Parallel()
		a := newA()
		e, _ := cast(a)
		if err := a.undo(t, a.master, e, a.log(t, a.master, e).GetUndoableEventId()); err != nil {
			t.Fatalf("UndoLastAction() error = %v", err)
		}
		if hp, temp, _ := a.hp(t, "Goblin"); hp != 7 || temp != 0 {
			t.Errorf("goblin = %d hit points, %d temporary after the undo; want 7 and 0", hp, temp)
		}
	})
}

// A stored payload of the damage steps from before the Hidden flag still reads, as not hidden.
func TestStepGroupsWithoutHiddenStillDecode(t *testing.T) {
	t.Parallel()
	groups := readStepGroupsForTest(t, `[{"damage_type":"damage-type:fire","before":6,"mods":[{"Kind":"resistance","Source":"race:tiefling","DamageTypes":["damage-type:fire"]}]}]`)
	if len(groups) != 1 || len(groups[0].Mods) != 1 || groups[0].Mods[0].Hidden || groups[0].Mods[0].Source != "race:tiefling" {
		t.Errorf("groups = %+v, want one modifier that is not hidden", groups)
	}
}

func readStepGroupsForTest(t *testing.T, raw string) []stepGroup {
	t.Helper()
	var out []stepGroup
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	return out
}

// failed_save reaches only the targets that fail the save; hit only the target the attack hits.
func TestATableSpellAppliesOnlyToFailedSaves(t *testing.T) {
	t.Parallel()
	a := newMarkCastersWith(t, func(s *rulesv1.TableSpell) {
		s.Target = &rulesv1.TableSpellTarget{Kind: rulesv1.TableSpellTargetKind_TABLE_SPELL_TARGET_KIND_CREATURES, Count: 2}
		s.Save = &rulesv1.SpellSave{Ability: rulesv1.Ability_ABILITY_WISDOM, OnSuccess: rulesv1.SpellSaveSuccess_SPELL_SAVE_SUCCESS_NONE}
		s.Effect.Applies = rulesv1.TableEffectApplies_TABLE_EFFECT_APPLIES_FAILED_SAVE
	}, &tableMod{Kind: modAC, Value: 2})
	e := a.castersFight(t, 1)
	a.h.roller.queue(1, 20)
	a.mustCast(t, a.ana, e, "Pensantus", tableMark, slotOfLevel(1), a.at(t, "Goblin", "Capitão Goblin"), noCastRoll)
	e = a.get(t, a.master)
	failed, saved := 0, 0
	for _, l := range []string{"Goblin", "Capitão Goblin"} {
		if cardOf(t, byLabel(t, e, l), tableMark) != nil {
			failed++
		} else {
			saved++
		}
	}
	if failed != 1 || saved != 1 {
		t.Errorf("%d targets got the effect and %d did not; want one of each", failed, saved)
	}
}

func TestATableSpellAppliesOnlyToAHit(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		face int32
		want bool
	}{{"hit", 20, true}, {"miss", 1, false}} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			a := newMarkCastersWith(t, func(s *rulesv1.TableSpell) {
				s.Attack = "ranged"
				s.Damage = []*rulesv1.TableSpellDamage{{DamageTypeKey: "damage-type:necrotic", Dice: "1d4"}}
				s.Effect.Applies = rulesv1.TableEffectApplies_TABLE_EFFECT_APPLIES_HIT
			}, &tableMod{Kind: modAC, Value: 2})
			e := a.castersFight(t, 1)
			a.mustCast(t, a.ana, e, "Pensantus", tableMark, slotOfLevel(1), a.at(t, "Goblin"), func(r *playv1.CastSpellRequest) { r.D20Faces = []int32{c.face, c.face} })
			if got := cardOf(t, byLabel(t, a.get(t, a.master), "Goblin"), tableMark) != nil; got != c.want {
				t.Errorf("the goblin has the effect = %v, want %v", got, c.want)
			}
		})
	}
}

// RN-10: a player who does not own the creature reads the label of an effect, never its numbers or the spell's name;
// the master and the creature's owner read all of it.
func TestATableSpellEffectDoesNotLeakItsNumbers(t *testing.T) {
	t.Parallel()
	a := newMarkCastersWith(t, func(s *rulesv1.TableSpell) {
		s.Effect.Visibility = rulesv1.TableEffectAudience_TABLE_EFFECT_AUDIENCE_OWNER
		s.Effect.PlayerLabelPt = "Marcado"
	}, &tableMod{Kind: modTemp, Value: 77}, &tableMod{
		Kind: rulesv1.TableEffectModifierKind_TABLE_EFFECT_MODIFIER_KIND_ROLL_BONUS, Value: 13,
		AppliesTo: rollKinds(rulesv1.TableEffectRollKind_TABLE_EFFECT_ROLL_KIND_ATTACK),
	})
	e := a.castersFight(t, 1)
	a.mustCast(t, a.ana, e, "Pensantus", tableMark, slotOfLevel(1), a.at(t, "Toren"), noCastRoll)

	// The creature's card, as each reader gets it, with the combatant's own numbers.
	cardFor := func(u *user) (*playv1.LastingEffect, *playv1.Combatant) {
		c := byLabel(t, a.get(t, u), "Toren")
		return cardOf(t, c, tableMark), c
	}
	for name, u := range map[string]*user{"master": a.master, "owner": a.caio} {
		card, c := cardFor(u)
		if card == nil || len(card.GetModifiers()) != 2 || card.GetModifiers()[0].GetValue()+card.GetModifiers()[1].GetValue() != 90 || !strings.Contains(strings.Join(card.GetTagsPt(), " "), "77") {
			t.Errorf("%s reads %v (%v), want the numbers", name, card, c)
		}
	}
	if v := a.vitals(t, a.toren); v.GetHitPointsTemporary() != 77 {
		t.Errorf("Toren has %d temporary hit points, want 77", v.GetHitPointsTemporary())
	}
	card, c := cardFor(a.bia)
	if c.GetHitPointsTemporary() == 77 || strings.Contains(c.String(), "Marca de Nanquim") {
		t.Errorf("another player reads %v", c)
	}
	if card != nil {
		for _, m := range card.GetModifiers() {
			t.Errorf("another player reads the modifier %v", m)
		}
		read := strings.Join(card.GetTagsPt(), " ") + card.GetSourceNamePt() + strings.Join(card.GetChangesPt(), " ")
		if strings.Contains(read, "77") || strings.Contains(read, "13") || strings.Contains(read, "Marca de Nanquim") {
			t.Errorf("another player reads %v", card)
		}
	}
	// The other player's log and view carry the canaries nowhere.
	// last_seq is a counter of the events the reader may see, never a hit point or a duration
	seen := initiativeRe.ReplaceAllString(lastSeqRe.ReplaceAllString(a.log(t, a.bia, e).String(), "")+combatantsText(a.get(t, a.bia)), "")
	for _, canary := range []string{"77", "13"} {
		if m := regexp.MustCompile(`.{0,60}(^|[^0-9A-Za-z-])` + canary + `([^0-9A-Za-z-]|$).{0,20}`).FindString(seen); m != "" {
			t.Errorf("the other player's log or view carries %s: %q", canary, m)
		}
	}
}

func combatantsText(e *playv1.Encounter) string {
	var b strings.Builder
	for _, c := range e.GetCombatants() {
		b.WriteString(c.String())
	}
	return b.String()
}

var lastSeqRe = regexp.MustCompile(`last_seq:[0-9]+`)

var initiativeRe = regexp.MustCompile(`initiative[a-z_]*:-?[0-9]+`)

func hideEffect(t *testing.T, a *armed, e *playv1.Encounter, label string) {
	t.Helper()
	id := a.rpcEffectID(t, tableMark, label)
	if _, err := a.rpcVisibility(t, a.master, e, id, func(r *playv1.SetLastingEffectVisibilityRequest) { r.PlayerVisible = false }); err != nil {
		t.Fatalf("SetLastingEffectVisibility() error = %v", err)
	}
}

// RN-10: what an effect the master hides adds to an attack is "Outra fonte" to the player and the spell to the master.
func TestATableSpellHiddenBonusIsOutraFonteToThePlayer(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		who   string
		hides bool // whether the roller is a player
	}{{"player", true}, {"master", false}} {
		t.Run(c.who, func(t *testing.T) {
			t.Parallel()
			a := newMarkCasters(t, &tableMod{
				Kind: rulesv1.TableEffectModifierKind_TABLE_EFFECT_MODIFIER_KIND_ROLL_BONUS, Value: 2,
				AppliesTo: rollKinds(rulesv1.TableEffectRollKind_TABLE_EFFECT_ROLL_KIND_ATTACK),
			})
			e := a.markOn(t, "Toren")
			hideEffect(t, a, e, "Toren")
			a.passTo(t, e, "Toren")
			u := a.master
			if c.hides {
				u = a.caio
			}
			res := a.mustAttack(t, u, e, "Toren", battleaxe, "Goblin", func(r *playv1.RollAttackRequest) { r.D20Faces = []int32{12} })
			texts := sourceTextsOf(res.GetRoll().GetSources())
			if got := strings.Contains(texts, "Outra fonte"); got != c.hides {
				t.Errorf("sources = %q, Outra fonte = %v, want %v", texts, got, c.hides)
			}
			if got := strings.Contains(texts, "Marca de Nanquim"); got == c.hides {
				t.Errorf("sources = %q, the spell's name listed = %v, want %v", texts, got, !c.hides)
			}
		})
	}
}

// A negative roll_bonus is a penalty on the roll.
func TestATableSpellNegativeRollBonusLowersTheRoll(t *testing.T) {
	t.Parallel()
	a := newMarkCasters(t, &tableMod{
		Kind: rulesv1.TableEffectModifierKind_TABLE_EFFECT_MODIFIER_KIND_ROLL_BONUS, Value: -3,
		AppliesTo: rollKinds(rulesv1.TableEffectRollKind_TABLE_EFFECT_ROLL_KIND_ATTACK, rulesv1.TableEffectRollKind_TABLE_EFFECT_ROLL_KIND_SAVE),
	})
	e := a.markOn(t, "Toren")
	a.passTo(t, e, "Toren")
	res := a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", func(r *playv1.RollAttackRequest) { r.D20Faces = []int32{12} })
	if got := res.GetRoll().GetD20().GetModifier(); got != 3 {
		t.Errorf("the attack's modifier = %d, want 3 (+6 and -3)", got)
	}
	if cm := a.checkModeFor(t, a.toren.GetId(), "", "dex", false); bonusTotal(cm.Bonuses) != -3 {
		t.Errorf("a save's bonus = %d, want -3", bonusTotal(cm.Bonuses))
	}
}

// nonmagical_bps halves the slashing damage of a weapon and leaves a spell's damage alone.
func TestATableSpellNonmagicalResistanceHalvesAWeaponOnly(t *testing.T) {
	t.Parallel()
	a := newMarkCasters(t, &tableMod{
		Kind: rulesv1.TableEffectModifierKind_TABLE_EFFECT_MODIFIER_KIND_RESISTANCE, DamageTypes: []string{"nonmagical_bps"},
	})
	e := a.markOn(t, "Goblin")
	a.passTo(t, e, "Toren")
	res := a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", func(r *playv1.RollAttackRequest) { r.D20Faces = []int32{15} })
	a.mustDamage(t, a.caio, e, res.GetPendingDamage().GetId(), typedDamage(6)) // 9 with the Strength bonus, 4 after the resistance
	if got, _, _ := a.hp(t, "Goblin"); got != 3 {
		t.Errorf("the goblin has %d hit points after a weapon, want 3", got)
	}
}

func TestATableSpellNonmagicalResistanceLeavesASpellAlone(t *testing.T) {
	t.Parallel()
	a := newMarkCasters(t, &tableMod{
		Kind: rulesv1.TableEffectModifierKind_TABLE_EFFECT_MODIFIER_KIND_RESISTANCE, DamageTypes: []string{"nonmagical_bps"},
	})
	e := a.markOn(t, "Goblin")
	a.passTo(t, e, "Toren")
	a.passTo(t, e, "Pensantus") // the next round: the action of the cast is spent
	res := a.mustAttack(t, a.ana, e, "Pensantus", fireBolt, "Goblin", func(r *playv1.RollAttackRequest) { r.D20Faces = []int32{15, 15} })
	a.mustDamage(t, a.ana, e, res.GetPendingDamage().GetId(), typedDamage(6))
	if got, _, _ := a.hp(t, "Goblin"); got != 1 {
		t.Errorf("the goblin has %d hit points after a Fire Bolt of 6, want 1: the clause is for weapons", got)
	}
}

// RN-10: the damage step of a resistance the master hides reads "Outra fonte" to the character's player, in the
// pending damage and in the log, and the spell's name to the master.
func TestATableSpellHiddenResistanceStepIsOutraFonteToThePlayer(t *testing.T) {
	t.Parallel()
	a := newMarkCasters(t, &tableMod{
		Kind: rulesv1.TableEffectModifierKind_TABLE_EFFECT_MODIFIER_KIND_RESISTANCE, DamageTypes: []string{"all"},
	})
	e := a.markOn(t, "Toren")
	hideEffect(t, a, e, "Toren")
	a.passTo(t, e, "Goblin")
	hit := a.mustAttack(t, a.master, e, "Goblin", "basic:0", "Toren", d20(19))
	dmg := a.mustDamage(t, a.master, e, hit.GetPendingDamage().GetId(), typedDamage(6)).GetPendingDamage()
	stepsText := func(steps []*playv1.DamageStep) string {
		var b strings.Builder
		for _, s := range steps {
			b.WriteString(s.GetLabelPt() + " " + strings.Join(s.GetSourceKeys(), ",") + "|")
		}
		return b.String()
	}
	if got := stepsText(dmg.GetSteps()); !strings.Contains(got, "Marca de Nanquim") && !strings.Contains(got, tableMark) {
		t.Fatalf("the master reads the steps %q, want the spell", got)
	}
	var log string
	for _, r := range a.log(t, a.caio, e).GetRounds() {
		for _, en := range r.GetEntries() {
			log += stepsText(en.GetDamage().GetSteps())
		}
	}
	if !strings.Contains(log, "Outra fonte") || strings.Contains(log, "Marca de Nanquim") || strings.Contains(log, tableMark) {
		t.Errorf("the player reads the steps %q, want Outra fonte and no spell", log)
	}
}

// The undo of the effect that took the pool over gives the older effect its claim back: ending it clears the 5.
func TestATableSpellUndoOfATakeoverRestoresTheOlderClaim(t *testing.T) {
	t.Parallel()
	a := newTwoMarks(t, 5, 10)
	e := a.castersFight(t, 1)
	a.mustCast(t, a.ana, e, "Pensantus", tableMark, slotOfLevel(1), nil, noCastRoll)
	a.passTo(t, e, "Toren")
	a.passTo(t, e, "Pensantus")
	a.mustCast(t, a.ana, e, "Pensantus", "spell:marca-de-cinza@mesa", slotOfLevel(1), nil, noCastRoll)
	if _, temp, _ := a.hp(t, "Pensantus"); temp != 10 {
		t.Fatalf("temporary hit points = %d after the second effect, want 10", temp)
	}
	if err := a.undo(t, a.master, e, a.log(t, a.master, e).GetUndoableEventId()); err != nil {
		t.Fatalf("UndoLastAction() error = %v", err)
	}
	if _, temp, _ := a.hp(t, "Pensantus"); temp != 5 {
		t.Fatalf("temporary hit points = %d after the undo, want 5", temp)
	}
	a.endMarkConcentration(t, e)
	if _, temp, _ := a.hp(t, "Pensantus"); temp != 0 {
		t.Errorf("temporary hit points = %d after the older effect ended, want 0", temp)
	}
}

// A recast of the same spell replaces the effect and keeps its claim: the pool the first cast gave goes when it ends.
func TestATableSpellRecastKeepsTheClaimOnTheTempHP(t *testing.T) {
	t.Parallel()
	t.Run("combat", func(t *testing.T) {
		t.Parallel()
		a := newSelfMark(t)
		e := a.castersFight(t, 1)
		a.mustCast(t, a.ana, e, "Pensantus", tableMark, slotOfLevel(1), nil, noCastRoll)
		a.passTo(t, e, "Toren")
		a.passTo(t, e, "Pensantus")
		a.mustCast(t, a.ana, e, "Pensantus", tableMark, slotOfLevel(1), nil, noCastRoll)
		if _, temp, _ := a.hp(t, "Pensantus"); temp != 5 {
			t.Fatalf("temporary hit points = %d after the recast, want 5", temp)
		}
		a.endMarkConcentration(t, e)
		if _, temp, _ := a.hp(t, "Pensantus"); temp != 0 {
			t.Errorf("temporary hit points = %d after the end, want 0", temp)
		}
	})
	t.Run("outside a combat", func(t *testing.T) {
		t.Parallel()
		a := newSelfMark(t)
		a.mustCastOut(t, a.ana, a.pens, tableMark, slotOfLevel(1), false, nil)
		a.mustCastOut(t, a.ana, a.pens, tableMark, slotOfLevel(1), false, nil)
		if v := a.vitals(t, a.pens); v.GetHitPointsTemporary() != 5 {
			t.Fatalf("temporary hit points = %d after the recast, want 5", v.GetHitPointsTemporary())
		}
		a.advanceGameTime(t, 61)
		if v := a.vitals(t, a.pens); v.GetHitPointsTemporary() != 0 {
			t.Errorf("temporary hit points = %d after the end, want 0", v.GetHitPointsTemporary())
		}
	})
}

// RN-10: a bonus of an effect only the creature's player reads (the table spell's default) is not named in the
// save the caster's player is answered (it reads "Outra fonte" or nothing), and the master reads the spell.
func TestATableSpellOwnerOnlyBonusIsOutraFonteInAnotherPlayersSave(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		who      string
		master   bool
		wantName bool
	}{{"caster's player", false, false}, {"master", true, true}} {
		t.Run(c.who, func(t *testing.T) {
			t.Parallel()
			a := newMarkCastersWith(t, func(s *rulesv1.TableSpell) {
				s.Effect.Visibility = rulesv1.TableEffectAudience_TABLE_EFFECT_AUDIENCE_OWNER
			},
				&tableMod{
					Kind: rulesv1.TableEffectModifierKind_TABLE_EFFECT_MODIFIER_KIND_ROLL_BONUS, Value: 13,
					AppliesTo: rollKinds(rulesv1.TableEffectRollKind_TABLE_EFFECT_ROLL_KIND_SAVE),
				})
			e := a.markOn(t, "Toren")
			a.passTo(t, e, "Brisa")
			a.h.roller.queue(11)
			u := a.bia
			if c.master {
				u = a.master
			}
			cast := a.mustCast(t, u, e, "Brisa", sacredFlame, nil, a.at(t, "Toren"), noCastRoll)
			texts := sourceTextsOf(cast.GetCast().GetTargets()[0].GetSave().GetSources())
			if got := strings.Contains(texts, "Marca de Nanquim"); got != c.wantName {
				t.Errorf("sources = %q, names the spell = %v, want %v", texts, got, c.wantName)
			}
			if c.wantName && !strings.Contains(texts, "+13") {
				t.Errorf("sources = %q, want the +13", texts)
			}
		})
	}
}

// RN-10: out of a combat, another player reads the label of a public effect on a character, never its numbers.
func TestATableSpellCharacterEffectDoesNotLeakItsNumbersOutsideACombat(t *testing.T) {
	t.Parallel()
	a := newMarkCastersWith(t, func(s *rulesv1.TableSpell) { s.Effect.PlayerLabelPt = "Marcado" },
		&tableMod{Kind: modTemp, Value: 77},
		&tableMod{
			Kind: rulesv1.TableEffectModifierKind_TABLE_EFFECT_MODIFIER_KIND_ROLL_BONUS, Value: 13,
			AppliesTo: rollKinds(rulesv1.TableEffectRollKind_TABLE_EFFECT_ROLL_KIND_ATTACK),
		})
	a.mustCastOut(t, a.ana, a.pens, tableMark, slotOfLevel(1), false, []*charactersv1.Character{a.toren})
	read := func(u *user) *playv1.CharacterEffect {
		res, err := u.lasting.ListCharacterEffects(t.Context(), connect.NewRequest(&playv1.ListCharacterEffectsRequest{CampaignId: a.campaignID}))
		if err != nil {
			t.Fatalf("ListCharacterEffects() error = %v", err)
		}
		for _, f := range res.Msg.GetEffects() {
			if f.GetCharacterId() == a.toren.GetId() {
				return f
			}
		}
		return nil
	}
	for name, u := range map[string]*user{"master": a.master, "owner": a.caio} {
		f := read(u)
		if f == nil || !strings.Contains(strings.Join(f.GetTagsPt(), " "), "77") || !strings.Contains(strings.Join(f.GetTagsPt(), " "), "13") {
			t.Errorf("%s reads %v, want the numbers", name, f)
		}
	}
	f := read(a.bia)
	if f == nil {
		t.Fatal("another player does not read the public effect at all")
	}
	if got := strings.Join(f.GetTagsPt(), " "); got != "Marcado" {
		t.Errorf("another player reads the tags %q, want only the label", got)
	}
}

// RN-10: a table effect whose caster the viewer cannot see names no caster to that viewer. A player's combatant is
// never hidden, so no fixture can make the table spell's caster unseen; the builders of the card's origin and clock
// are read directly with a table-spell row.
func TestATableSpellEffectOfAnUnseenCasterNamesNoCaster(t *testing.T) {
	t.Parallel()
	caster := playdb.Combatant{ID: "c1", Label: "Pensantus"}
	kind, rounds, source := "spell", rules.EffectDurationRounds, tableMark
	row := playdb.CombatantState{SourceKind: &kind, SourceKey: &source, SourceID: &caster.ID, DurationKind: &rounds}
	end := int32(11)
	row.EndsRound, row.EndsCombatantID = &end, &caster.ID
	names := func(_ string) string { return "Marca de Nanquim" }
	player := &effectViewer{v: combatViewer{}, d: &encounterData{}, names: names, byID: map[string]playdb.Combatant{caster.ID: caster}}
	master := &effectViewer{v: combatViewer{master: true}, d: &encounterData{}, names: names, byID: map[string]playdb.Combatant{caster.ID: caster}}
	if got := player.originOf(row, caster, false); strings.Contains(got, "Pensantus") {
		t.Errorf("origin for a player = %q, names the caster", got)
	}
	if got := player.clockText(row, caster, false); strings.Contains(got, "Pensantus") {
		t.Errorf("clock for a player = %q, names the caster", got)
	}
	if got := master.originOf(row, caster, true); !strings.Contains(got, "Pensantus") {
		t.Errorf("origin for the master = %q, want the caster", got)
	}
}

// The card of a table spell's check_advantage names the ability once, and carries none of the SRD's notes.
func TestATableSpellCheckAdvantageIsTaggedOnce(t *testing.T) {
	t.Parallel()
	a := newMarkCasters(t, &tableMod{Kind: rulesv1.TableEffectModifierKind_TABLE_EFFECT_MODIFIER_KIND_CHECK_ADVANTAGE, Abilities: []string{"str"}})
	e := a.markOn(t, "Toren")
	card := cardOf(t, byLabel(t, a.get(t, a.master), "Toren"), tableMark)
	if card == nil {
		t.Fatalf("no card on Toren in %v", e)
	}
	n := 0
	for _, tag := range card.GetTagsPt() {
		if strings.HasPrefix(tag, "Vantagem em testes") {
			n++
		}
	}
	if n != 1 || len(card.GetChangesPt()) != 0 && strings.Contains(strings.Join(card.GetChangesPt(), " "), "Touro") {
		t.Errorf("tags %q, changes %q; want one tag and no SRD note", card.GetTagsPt(), card.GetChangesPt())
	}
}

// RN-10: a public effect that poisons a character out of a combat shows another player nothing of the poison
// (no condition name, no label); the master and the character's player read it.
func TestATableSpellOwnerOnlyConditionDoesNotLeakOutsideACombat(t *testing.T) {
	t.Parallel()
	a := newMarkCastersWith(t, func(s *rulesv1.TableSpell) { s.Effect.Conditions = []string{"condition:poisoned"} }, &tableMod{Kind: modAC, Value: 2})
	a.mustCastOut(t, a.ana, a.pens, tableMark, slotOfLevel(1), false, []*charactersv1.Character{a.toren})
	read := func(u *user) *playv1.CharacterEffect {
		res, err := u.lasting.ListCharacterEffects(t.Context(), connect.NewRequest(&playv1.ListCharacterEffectsRequest{CampaignId: a.campaignID}))
		if err != nil {
			t.Fatalf("ListCharacterEffects() error = %v", err)
		}
		for _, f := range res.Msg.GetEffects() {
			if f.GetCharacterId() == a.toren.GetId() {
				return f
			}
		}
		return nil
	}
	for name, u := range map[string]*user{"master": a.master, "owner": a.caio} {
		if f := read(u); f == nil || !strings.Contains(strings.Join(f.GetConditionNamesPt(), " "), "Envenenado") {
			t.Errorf("%s reads %v, want the poison", name, f)
		}
	}
	if f := read(a.bia); f != nil && strings.Contains(f.String(), "Envenenado") {
		t.Errorf("another player reads the poison: %v", f)
	}
}

// The effects on a creature that only its player reads count as unread for any other viewer, and the master
// and the player read them.
func TestHasUnreadEffectByAudience(t *testing.T) {
	t.Parallel()
	user := "u1"
	toren := playdb.Combatant{ID: "t", UserID: &user}
	states := map[string][]playdb.CombatantState{"t": {{ID: "e", Kind: "effect", PlayerVisible: true, Audience: audienceOwner}}}
	for _, c := range []struct {
		name string
		v    combatViewer
		want bool
	}{
		{"another player", combatViewer{userID: "u2"}, true},
		{"the owner", combatViewer{userID: "u1"}, false},
	} {
		if got := hasUnreadEffect(states, toren, c.v); got != c.want {
			t.Errorf("%s: unread = %v, want %v", c.name, got, c.want)
		}
	}
	states["t"][0].Audience = "all"
	if hasUnreadEffect(states, toren, combatViewer{userID: "u2"}) {
		t.Error("a public effect is unread by another player")
	}
}

// The temp_hp row keeps the spell's own number for the card and the claim apart.
func TestTempHPClaimIsApartFromTheValue(t *testing.T) {
	t.Parallel()
	mods := []rules.EffectModifier{{Kind: rules.ModifierTempHP, Value: 5}}
	if got := combat.TempHPClaim(mods); got != 5 {
		t.Errorf("a row with no claim claims its value: %d", got)
	}
	for pool, want := range map[int]int{0: 0, 3: 3} {
		var back []rules.EffectModifier
		if err := json.Unmarshal(claimedTempHP(mods, pool), &back); err != nil {
			t.Fatal(err)
		}
		if back[0].Value != 5 || combat.TempHPClaim(back) != want {
			t.Errorf("pool %d: %+v claims %d, want value 5 and claim %d", pool, back[0], combat.TempHPClaim(back), want)
		}
	}
}

// A card keeps the spell's number when the effect raised nothing.
func TestATableSpellCardKeepsTheSpellsTempHPWhenNothingWasRaised(t *testing.T) {
	t.Parallel()
	a := newSelfMark(t)
	e := a.castersFight(t, 1)
	a.setTemp(t, a.pens, 8)
	a.mustCast(t, a.ana, e, "Pensantus", tableMark, slotOfLevel(1), nil, noCastRoll)
	card := cardOf(t, byLabel(t, a.get(t, a.ana), "Pensantus"), tableMark)
	if card == nil || !strings.Contains(strings.Join(card.GetTagsPt(), " "), "5") {
		t.Errorf("card = %v, want the spell's 5 temporary hit points in the tags", card)
	}
	for _, m := range card.GetModifiers() {
		if m.GetKind() == playv1.EffectModifierKind_EFFECT_MODIFIER_KIND_TEMP_HP && m.GetValue() != 5 {
			t.Errorf("the card's temp_hp = %d, want 5", m.GetValue())
		}
	}
}
