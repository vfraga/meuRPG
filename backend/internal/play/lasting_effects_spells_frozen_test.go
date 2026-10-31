package play

import (
	"context"
	"fmt"
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The characterization of what a spell that lasts leaves on its targets (applyLastingSpell,
// noEffectWhy) and of the mode of the rolls of a character out of a combat (outsideModeOf).
// See lasting_core_frozen_test.go.

const (
	coreLaughter = "spell:hideous-laughter"
	coreWolf     = "monster:wolf"
	coreOgre     = "monster:ogre"
	coreMimic    = "monster:mimic"
)

// newRiderCasters is the fixture of the casters with the spells that ask a creature of a kind.
func newRiderCasters(t *testing.T) *armed {
	t.Helper()
	return newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 5,
			&rulesv1.AbilityScores{Strength: 16, Dexterity: 13, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{battleaxe}, nil)
		a.pens = a.ana.caster(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 5,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8}, nil, []string{fireBolt},
			[]string{holdPerson, coreLaughter, mageArmorKey, webSpell}, []string{holdPerson, coreLaughter, mageArmorKey, webSpell})
		a.bri = a.bia.caster(t, a.campaignID, "Brisa", "class:cleric", "race:human", 3,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 16, Constitution: 14, Intelligence: 10, Wisdom: 16, Charisma: 8}, []string{maceKey}, []string{sacredFlame}, nil,
			[]string{cureWounds, healingWord, blessKey})
	})
}

// A spell that lasts takes hold of a creature by what it is: Hold Person asks a humanoid, a
// condition the creature is immune to is no effect, Hideous Laughter asks an Intelligence over 4
// (SRD 5.1); the master reads why, the players read only that nothing happened. The effect is made
// for the targets that failed the saving throw.
func TestLastingCoreASpellTakesHoldByWhatTheTargetIs(t *testing.T) {
	t.Parallel()
	a := newRiderCasters(t)
	e := a.monsterSetup(t, false)
	a.h.roller.queue(10, 11, 12, 13)
	for _, key := range []string{bandit, coreWolf, coreOgre, coreMimic} {
		a.mustAddMonsters(t, e, func(r *playv1.AddMonstersRequest) { r.CreatureKey, r.Count = key, 1 })
	}
	e = a.begin(t, e)
	for _, label := range []string{"Bandido", "Lobo", "Ogro", "Mímico", "Goblin"} {
		showNPC(t, a, e, label) // the master casts for Pensantus, so the targets are what her player sees
	}
	var l coreLog
	save := func(face int) { a.h.roller.queue(face) }
	cast := func(spell string, slot int32, label string, face int) {
		t.Helper()
		e = a.turnOf(t, "Pensantus")
		save(face)
		res, err := a.cast(t, a.master, e, "Pensantus", spell, slotOfLevel(slot), a.at(t, label), noCastRoll)
		if err != nil {
			l.say("%s on %s -> %s", spell, label, coreRefusal(err))
			return
		}
		for _, h := range res.GetCast().GetTargets() {
			l.say("%s on %s -> lasting %v why %q save %v", spell, label, h.GetLastingEffect(), h.GetNoEffectReasonMaster(), h.GetSave().GetOutcome())
		}
	}
	cast(holdPerson, 2, "Bandido", 1)
	cast(holdPerson, 2, "Bandido", 20) // the same target again: the first stays, no second effect
	cast(holdPerson, 2, "Lobo", 1)
	cast(holdPerson, 3, "Ogro", 1)
	cast(coreLaughter, 1, "Lobo", 1)
	cast(coreLaughter, 1, "Mímico", 1)
	cast(coreLaughter, 1, "Goblin", 20)
	cast(coreLaughter, 1, "Bandido", 1)
	l.say("%s%s", a.coreEffects(t, e.GetId()), a.coreCombatants(t, e.GetId(), "Bandido", "Lobo", "Ogro", "Mímico", "Goblin", "Pensantus"))
	l.say("events:\n%s", a.coreEvents(t, eventLastingAdded, eventLastingEnded))
	wantLastingGolden(t, "lasting_core", "spells_take_hold", a.coreNames(t, l.String()))
}

// The duration of the effect of a spell follows the spell: rounds of the minute counted from the
// caster, the rider's own clock, a concentration with no time; the saving throw's DC is kept; an
// effect that sets a base armor class is made for each target that wears no armor.
func TestLastingCoreASpellsEffectKeepsItsClockAndDC(t *testing.T) {
	t.Parallel()
	a := newRiderCasters(t)
	e := a.closeFight(t)
	var l coreLog
	cast := func(caster, spell string, slot int32, targets ...string) {
		t.Helper()
		e = a.turnOf(t, caster)
		res, err := a.cast(t, a.master, e, caster, spell, slotOfLevel(slot), a.at(t, targets...), noCastRoll)
		if err != nil {
			l.say("%s on %v -> %s", spell, targets, coreRefusal(err))
			return
		}
		for _, h := range res.GetCast().GetTargets() {
			l.say("%s on %v -> lasting %v why %q", spell, targets, h.GetLastingEffect(), h.GetNoEffectReasonMaster())
		}
	}
	a.h.roller.queue(1)
	cast("Pensantus", holdPerson, 2, "Goblin")
	cast("Pensantus", mageArmorKey, 1, "Toren")
	cast("Pensantus", mageArmorKey, 1, "Brisa")
	a.h.roller.queue(1, 1)
	cast("Brisa", blessKey, 1, "Toren", "Pensantus", "Brisa")
	l.say("%s%s", a.coreEffects(t, e.GetId()), a.coreCombatants(t, e.GetId(), "Goblin", "Pensantus", "Toren", "Brisa"))
	l.say("events:\n%s", a.coreEvents(t, eventLastingAdded, eventLastingEnded))
	wantLastingGolden(t, "lasting_core", "spells_clock", a.coreNames(t, l.String()))
}

// The mode, the dice, the bonuses and the hidden sources of a roll of a character that is in no
// combat: what its effects (the rows of character_effects) and its exhaustion give.
func TestLastingCoreOutsideModeOfACharacter(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	var l coreLog
	toren := a.toren.GetId()
	rolls := []struct {
		name    string
		skill   string
		ability string
		check   bool
	}{{"an acrobatics check", "skill:acrobatics", "dex", true}, {"a stealth check", "skill:stealth", "dex", true}, {"a Dexterity save", "", "dex", false}, {"a Wisdom save", "", "wis", false}}
	show := func(what string) {
		t.Helper()
		for _, r := range rolls {
			var cm checkMode
			a.coreInTx(t, "", func(ctx context.Context, c *combatTx) error {
				var err error
				cm, err = a.h.svc.outsideModeOf(ctx, c.tx, c.session.CampaignID, toren, r.skill, r.ability, r.check)
				return err
			})
			l.say("%s, %s: mode %v", what, r.name, cm.Mode)
			for _, src := range cm.Sources {
				hidden := cm.hide != nil && cm.hide(src)
				l.say("  source %s condition=%q effect=%v hidden=%v", src.Kind, src.Condition, src.Effect, hidden)
			}
			for _, d := range cm.Dice {
				l.say("  die %s d%d sign %d hidden=%v once=%v character=%v", d.Key, d.Faces, d.Sign, d.Hidden, d.Once, d.CharacterID == toren)
			}
			for _, b := range cm.Bonuses {
				l.say("  bonus %s %+d hidden=%v", b.Key, b.Value, b.Hidden)
			}
			l.say("  helps %v rolled %v", cm.Helps, cm.Rolled)
		}
	}
	show("nothing")
	if _, err := a.coreSetExhaustion(t, a.master, &playv1.Encounter{}, func(r *playv1.SetExhaustionRequest) {
		r.Subject = &playv1.SetExhaustionRequest_CharacterId{CharacterId: toren}
		r.Level = 3
	}); err != nil {
		t.Fatalf("SetExhaustion() error = %v", err)
	}
	show("exhaustion 3")
	row := func(key, mods, conds string, visible bool) {
		t.Helper()
		a.execSQL(t, fmt.Sprintf(`INSERT INTO character_effects (campaign_id, character_id, group_id, source_key, source_kind, modifiers, condition_keys, duration_kind, seconds_left, player_visible)
			VALUES ($1, $2, gen_random_uuid(), '%s', 'spell', '%s', %s, 'rounds', 600, %v)`, key, mods, conds, visible), a.campaignID, toren)
	}
	row("spell:enhance-ability", `[{"kind":"check_advantage","abilities":["dex"]}]`, "ARRAY[]::TEXT[]", true)
	row("spell:haste", `[{"kind":"save_advantage","abilities":["dex"]},{"kind":"ac_bonus","value":2}]`, "ARRAY[]::TEXT[]", true)
	row("spell:pass-without-trace", `[{"kind":"check_bonus","skill":"skill:stealth","value":10}]`, "ARRAY[]::TEXT[]", true)
	row("spell:bless", `[{"kind":"roll_die","die":4,"sign":1,"applies_to":["attack","save"]}]`, "ARRAY[]::TEXT[]", true)
	row("spell:guidance", `[{"kind":"roll_die","die":4,"sign":1,"applies_to":["check"],"once":true}]`, "ARRAY[]::TEXT[]", true)
	row("spell:hold-person", `[]`, "ARRAY['condition:poisoned']", true)
	show("visible effects")
	a.execSQL(t, `UPDATE character_effects SET player_visible = false WHERE character_id = $1 AND source_key IN ('spell:bless', 'spell:hold-person', 'spell:pass-without-trace', 'spell:haste')`, toren)
	show("some effects hidden")
	wantLastingGolden(t, "lasting_core", "outside_mode", a.coreNames(t, l.String()))
}
