package play

import (
	"strings"
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// Both sides of a contest add the roll bonuses of their lasting effects that apply to ability checks, through the
// reader the other checks use. A bonus to saves never enters a contest.

func rollBonusOf(n int32, kind rulesv1.TableEffectRollKind) *tableMod {
	return &tableMod{
		Kind: rulesv1.TableEffectModifierKind_TABLE_EFFECT_MODIFIER_KIND_ROLL_BONUS, Value: n, AppliesTo: rollKinds(kind),
	}
}

var (
	checkBonus3 = rollBonusOf(3, rulesv1.TableEffectRollKind_TABLE_EFFECT_ROLL_KIND_CHECK)
	saveBonus3  = rollBonusOf(3, rulesv1.TableEffectRollKind_TABLE_EFFECT_ROLL_KIND_SAVE)
)

// contestRolls is the roll of the marked side of a contest, as its player and as the master read it.
type contestRolls struct{ player, master *playv1.CheckRoll }

// markedContest marks Toren (who starts a grapple on the Hobgoblin) or Brisa (whom the Hobgoblin grapples) with Pensantus's
// spell, hides the effect when asked, and plays the contest.
func markedContest(t *testing.T, side string, hide bool, mod *tableMod) contestRolls {
	t.Helper()
	a := newMarkCasters(t, mod)
	c := a.arena(t, arenaPlan{toren: 10, brisa: 11, pens: 20})
	target := map[string]string{"initiator": "Toren", "defender": "Brisa"}[side]
	a.mustCast(t, a.ana, c.e, "Pensantus", tableMark, slotOfLevel(1), a.at(t, target), noCastRoll)
	if hide {
		hideEffect(t, a, c.refresh(t), target)
	}
	var out contestRolls
	if side == "initiator" {
		c.advance(t, "Toren")
		c.h.roller.queue(10)
		started := c.mustStart(t, a.caio, func(r *playv1.StartContestRequest) {
			r.InitiatorId, r.TargetId, r.Purpose = c.id(t, "Toren"), c.id(t, c.hob), purposeGrappleP
		})
		out.player = started.GetContest().GetInitiatorRoll()
		out.master = c.state(t, a.master).GetContests()[0].GetInitiatorRoll()
		return out
	}
	c.advance(t, c.hob)
	c.h.roller.queue(17)
	started := c.mustStart(t, a.master, func(r *playv1.StartContestRequest) {
		r.InitiatorId, r.TargetId, r.Purpose = c.id(t, c.hob), c.id(t, "Brisa"), purposeGrappleP
	})
	answered := c.mustRespond(t, a.bia, started.GetContest().GetId(), answerSkillAthletics, 8)
	out.player = answered.GetContest().GetDefenderRoll()
	out.master = c.state(t, a.master).GetContests()[0].GetDefenderRoll()
	return out
}

func noteTexts(r *playv1.CheckRoll) string {
	var out []string
	for _, n := range r.GetNotes() {
		out = append(out, n.GetLabelPt())
	}
	return strings.Join(out, " | ")
}

func TestAContestAddsTheCheckBonusOfAnEffect(t *testing.T) {
	t.Parallel()
	for _, side := range []string{"initiator", "defender"} {
		t.Run(side, func(t *testing.T) {
			t.Parallel()
			base := markedContest(t, side, false, saveBonus3)
			with := markedContest(t, side, false, checkBonus3)
			if got, want := with.player.GetModifier(), base.player.GetModifier()+3; got != want {
				t.Errorf("modifier = %d, want %d: the base and the effect's +3", got, want)
			}
			if with.player.GetTotal() != with.player.GetFaces()[0]+with.player.GetModifier() {
				t.Errorf("roll = %v, want the total to be the d20 and the modifier", with.player)
			}
			if with.player.GetMode() != playv1.RollModeKind_ROLL_MODE_KIND_NORMAL || len(with.player.GetFaces()) != 1 {
				t.Errorf("roll = %v, want one d20 in normal mode: the bonus note is no advantage or disadvantage", with.player)
			}
			for _, n := range with.player.GetNotes() {
				if n.GetKind() == "effect_bonus" && n.GetAdvantage() {
					t.Errorf("note %v says advantage, want none", n)
				}
			}
			if txt := noteTexts(with.player); !strings.Contains(txt, "Marca de Nanquim +3") {
				t.Errorf("notes = %q, want the effect and its bonus named", txt)
			}
		})
	}
}

// A bonus to saving throws is no bonus to a contest: the modifier and the notes are as with no effect at all.
func TestAContestIgnoresTheSaveBonusOfAnEffect(t *testing.T) {
	t.Parallel()
	for _, side := range []string{"initiator", "defender"} {
		t.Run(side, func(t *testing.T) {
			t.Parallel()
			saves := markedContest(t, side, false, saveBonus3)
			attacks := markedContest(t, side, false, rollBonusOf(3, rulesv1.TableEffectRollKind_TABLE_EFFECT_ROLL_KIND_ATTACK))
			if saves.player.GetModifier() != attacks.player.GetModifier() {
				t.Errorf("modifier with a save bonus = %d, with an attack bonus = %d, want the same base", saves.player.GetModifier(), attacks.player.GetModifier())
			}
			if txt := noteTexts(saves.player); strings.Contains(txt, "Marca") || strings.Contains(txt, "+3") {
				t.Errorf("notes = %q, want no effect note: a save bonus does not count", txt)
			}
			if saves.player.GetMode() != playv1.RollModeKind_ROLL_MODE_KIND_NORMAL {
				t.Errorf("mode = %v, want normal: a bonus says nothing of advantage", saves.player.GetMode())
			}
		})
	}
}

// RN-10: what the master hides adds its bonus, and the player reads "Outra fonte" while the master reads the effect.
func TestAContestNamesAHiddenEffectToTheMasterAlone(t *testing.T) {
	t.Parallel()
	for _, side := range []string{"initiator", "defender"} {
		t.Run(side, func(t *testing.T) {
			t.Parallel()
			r := markedContest(t, side, true, checkBonus3)
			if txt := noteTexts(r.player); !strings.Contains(txt, "Outra fonte +3") || strings.Contains(txt, "Marca") {
				t.Errorf("the player reads notes %q, want %q and no name", txt, "Outra fonte +3")
			}
			if txt := noteTexts(r.master); !strings.Contains(txt, "Marca de Nanquim +3") {
				t.Errorf("the master reads notes %q, want the effect's name", txt)
			}
			if r.player.GetModifier() != r.master.GetModifier() {
				t.Errorf("the player reads modifier %d and the master %d, want the same number", r.player.GetModifier(), r.master.GetModifier())
			}
		})
	}
}

func skillBonusOf(n int32, skill string) *tableMod {
	return &tableMod{Kind: rulesv1.TableEffectModifierKind_TABLE_EFFECT_MODIFIER_KIND_CHECK_BONUS, Value: n, Skill: skill}
}

// markedEscape marks Brisa, whom the Hobgoblin grapples with an escape DC of 25, and plays her escape with the skill
// (a d20 of 8); it gives her roll as she and as the master read it.
func markedEscape(t *testing.T, skill playv1.ContestSkill, hide bool, mod *tableMod) contestRolls {
	t.Helper()
	a := newMarkCasters(t, mod)
	c := a.arena(t, arenaPlan{toren: 10, brisa: 11, pens: 20, at: map[string][2]int32{"Brisa": {5, 3}}})
	a.mustCast(t, a.ana, c.e, "Pensantus", tableMark, slotOfLevel(1), a.at(t, "Brisa"), noCastRoll)
	if hide {
		hideEffect(t, a, c.refresh(t), "Brisa")
	}
	c.grappledByAttack(t, "Brisa", 25)
	c.advance(t, "Brisa")
	c.h.roller.queue(8)
	esc := c.mustStart(t, a.bia, func(r *playv1.StartContestRequest) {
		r.InitiatorId, r.Purpose, r.Skill = c.id(t, "Brisa"), purposeEscapeP, skill
	}).GetContest()
	var out contestRolls
	out.player = esc.GetInitiatorRoll()
	for _, v := range c.state(t, a.master).GetContests() {
		if v.GetId() == esc.GetId() {
			out.master = v.GetInitiatorRoll()
		}
	}
	if out.master == nil {
		out.master = out.player
	}
	return out
}

// An escape is a check of its own initiator: it adds the check bonus too (the grappled side is the one that rolls).
func TestAnEscapeAddsTheCheckBonusOfAnEffect(t *testing.T) {
	t.Parallel()
	acro, athl := playv1.ContestSkill_CONTEST_SKILL_ACROBATICS, playv1.ContestSkill_CONTEST_SKILL_ATHLETICS
	base := markedEscape(t, acro, false, saveBonus3)
	with := markedEscape(t, acro, false, checkBonus3)
	if got, want := with.player.GetModifier(), base.player.GetModifier()+3; got != want {
		t.Errorf("escape modifier = %d, want %d: the base and the effect's +3", got, want)
	}
	if txt := noteTexts(with.player); !strings.Contains(txt, "Marca de Nanquim +3") {
		t.Errorf("notes = %q, want the effect and its bonus named", txt)
	}
	hidden := markedEscape(t, acro, true, checkBonus3)
	if txt := noteTexts(hidden.player); !strings.Contains(txt, "Outra fonte +3") || strings.Contains(txt, "Marca") {
		t.Errorf("the player reads notes %q, want %q and no name", txt, "Outra fonte +3")
	}

	// A bonus of one skill is for that skill alone: Acrobatics' +3 raises no Athletics escape.
	plain := markedEscape(t, athl, false, saveBonus3)
	other := markedEscape(t, athl, false, skillBonusOf(3, "skill:acrobatics"))
	if other.player.GetModifier() != plain.player.GetModifier() {
		t.Errorf("Athletics escape modifier = %d with an Acrobatics bonus, want %d", other.player.GetModifier(), plain.player.GetModifier())
	}
	if txt := noteTexts(other.player); strings.Contains(txt, "+3") {
		t.Errorf("notes = %q, want no note: the bonus is for another skill", txt)
	}
	same := markedEscape(t, acro, false, skillBonusOf(3, "skill:acrobatics"))
	if got, want := same.player.GetModifier(), base.player.GetModifier()+3; got != want {
		t.Errorf("Acrobatics escape modifier = %d with an Acrobatics bonus, want %d", got, want)
	}
}

// A Hide is a Stealth check: the bonus of its effects counts, with its note.
func TestAHideAddsTheCheckBonusOfAnEffect(t *testing.T) {
	t.Parallel()
	hide := func(mod *tableMod) *playv1.CheckRoll {
		a := newMarkCasters(t, mod)
		c := a.arena(t, arenaPlan{toren: 10, brisa: 11, pens: 20})
		a.mustCast(t, a.ana, c.e, "Pensantus", tableMark, slotOfLevel(1), a.at(t, "Toren"), noCastRoll)
		c.advance(t, "Toren")
		return c.mustHide(t, a.caio, "Toren", actionHide, 10).GetRoll()
	}
	base, with := hide(saveBonus3), hide(skillBonusOf(3, "skill:stealth"))
	if got, want := with.GetModifier(), base.GetModifier()+3; got != want {
		t.Errorf("Stealth modifier = %d, want %d: the base and the effect's +3", got, want)
	}
	if got := with.GetTotal(); got != 10+with.GetModifier() {
		t.Errorf("total = %d, want the d20 and the modifier", got)
	}
	if txt := noteTexts(with); !strings.Contains(txt, "Marca de Nanquim +3") {
		t.Errorf("notes = %q, want the effect and its bonus named", txt)
	}
	if txt := noteTexts(base); strings.Contains(txt, "+3") {
		t.Errorf("notes = %q, want none: a save bonus does not count", txt)
	}
}

// The answer a player is offered says what it would roll before the roll: the same bonus, with the same names.
func TestTheAnswerOptionsOfAContestCountTheCheckBonus(t *testing.T) {
	t.Parallel()
	options := func(mod *tableMod, hide bool, reader func(*armed) *user) []*playv1.ContestSkillOption {
		a := newMarkCasters(t, mod)
		c := a.arena(t, arenaPlan{toren: 10, brisa: 11, pens: 20})
		a.mustCast(t, a.ana, c.e, "Pensantus", tableMark, slotOfLevel(1), a.at(t, "Brisa"), noCastRoll)
		if hide {
			hideEffect(t, a, c.refresh(t), "Brisa")
		}
		c.advance(t, c.hob)
		c.h.roller.queue(17)
		started := c.mustStart(t, a.master, func(r *playv1.StartContestRequest) {
			r.InitiatorId, r.TargetId, r.Purpose = c.id(t, c.hob), c.id(t, "Brisa"), purposeGrappleP
		})
		for _, v := range c.state(t, reader(a)).GetContests() {
			if v.GetId() == started.GetContest().GetId() {
				return v.GetAnswerOptions()
			}
		}
		t.Fatal("the reader sees no contest")
		return nil
	}
	notes := func(o *playv1.ContestSkillOption) string {
		var texts []string
		for _, n := range o.GetNotes() {
			texts = append(texts, n.GetLabelPt())
		}
		return strings.Join(texts, " | ")
	}
	master := func(a *armed) *user { return a.master }
	player := func(a *armed) *user { return a.bia }
	base := options(saveBonus3, false, master)
	with := options(checkBonus3, false, master)
	if len(with) == 0 || len(with) != len(base) {
		t.Fatalf("Brisa has %d answer options with the bonus and %d without, want the same non-zero number", len(with), len(base))
	}
	for i, o := range with {
		if got := notes(o); !strings.Contains(got, "Marca de Nanquim +3") {
			t.Errorf("option %v has notes %q, want the effect's name and bonus", o.GetSkill(), got)
		}
		if got, want := o.GetModifier(), base[i].GetModifier()+3; got != want {
			t.Errorf("option %v modifier = %d, want %d: the base and the effect's +3", o.GetSkill(), got, want)
		}
	}
	for _, o := range options(checkBonus3, true, player) {
		if got := notes(o); !strings.Contains(got, "Outra fonte +3") || strings.Contains(got, "Marca") {
			t.Errorf("the player's option %v has notes %q, want %q and no name", o.GetSkill(), got, "Outra fonte +3")
		}
	}
	for _, o := range options(checkBonus3, true, master) {
		if got := notes(o); !strings.Contains(got, "Marca de Nanquim +3") {
			t.Errorf("the master's option %v has notes %q, want the effect's name", o.GetSkill(), got)
		}
	}
}
