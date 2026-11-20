package play

import (
	"slices"
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// Sleep and Color Spray (SRD 5.1, RN-20, RN-22): the unconsciousness of Sleep lasts one minute (10 rounds) and ends at once when
// the sleeper takes damage or is shaken awake; the blindness of Color Spray ends at the end of the caster's next turn. Sleep
// skips undead, creatures immune to being charmed and anybody Fey Ancestry protects ("magic can't put you to sleep"); Color Spray
// skips the unconscious and whoever cannot see. A skipped creature costs the pool nothing; a player reads "not affected" and the
// master (or the elf's own player, for the trait) reads why. These tests need the database (MEURPG_TEST_DATABASE_URL).

const (
	// The words the master reads (the app's own) and the trait's own words (races.json, trait:fey-ancestry).
	wordsUndead      = "Sem efeito em mortos-vivos"
	wordsCharmFree   = "Sem efeito em quem é imune a ser enfeitiçado"
	wordsCannotSee   = "Sem efeito em quem não enxerga"
	wordsFeyAncestry = "Vantagem em testes de resistência contra ficar enfeitiçado; magia não faz você dormir."
)

func notAffected(r *playv1.SpellTargetResult) bool {
	return r.GetEffect().GetOutcome() == playv1.SpellEffectOutcome_SPELL_EFFECT_OUTCOME_NOT_AFFECTED
}

// effectEnded says the combatant carries no effect of the spell.
func effectOfSpell(c *playv1.Combatant, key string) bool {
	return slices.ContainsFunc(c.GetEffects(), func(f *playv1.LastingEffect) bool { return f.GetSourceKey() == key })
}

// Sleep on one goblin: 10 rounds are a minute; the creature is awake again when the caster's turn of round 11 starts, and not before.
func TestSleepEndsAfterOneMinute(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	a.stand(t, map[string][2]int32{"Pensantus": {1, 1}, "Toren": {1, 2}, "Brisa": {2, 1}})
	a.h.roller.queue(2, 4, 1, 5, 3)
	a.mustCast(t, a.ana, e, "Pensantus", sleepSpell, slotOfLevel(1), a.at(t, "Goblin"), poolInApp)
	if !hasCondition(a.get(t, a.master), t, "Goblin", unconsciousC) {
		t.Fatal("the goblin did not fall asleep")
	}
	a.mustEndTurn(t, a.ana, e)
	for range 9 { // rounds 2 to 10: the minute is not over
		a.passTo(t, e, "Pensantus")
		if !hasCondition(a.get(t, a.master), t, "Goblin", unconsciousC) {
			t.Fatalf("the goblin woke in round %d, before the minute was over", a.get(t, a.master).GetRound())
		}
		a.mustEndTurn(t, a.ana, a.get(t, a.master))
	}
	now := a.passTo(t, e, "Pensantus")
	if now.GetRound() != 11 {
		t.Fatalf("the round = %d, want 11", now.GetRound())
	}
	if hasCondition(now, t, "Goblin", unconsciousC) {
		t.Error("the goblin is still asleep when the minute ended")
	}
}

// A sleeper that takes damage wakes up; the one next to it that was not hurt sleeps on.
func TestSleepEndsWhenTheSleeperTakesDamage(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 2)
	a.stand(t, map[string][2]int32{"Pensantus": {1, 1}, "Brisa": {2, 1}, "Toren": {6, 3}, "Goblin 1": {7, 3}, "Goblin 2": {7, 4}})
	a.h.roller.queue(8, 8, 8, 8, 8)
	a.mustCast(t, a.ana, e, "Pensantus", sleepSpell, slotOfLevel(1), a.at(t, "Goblin 1", "Goblin 2"), poolInApp)
	now := a.get(t, a.master)
	if !hasCondition(now, t, "Goblin 1", unconsciousC) || !hasCondition(now, t, "Goblin 2", unconsciousC) {
		t.Fatalf("the goblins' conditions = %v, %v; want both asleep", byLabel(t, now, "Goblin 1").GetConditions(), byLabel(t, now, "Goblin 2").GetConditions())
	}
	a.passTo(t, e, "Toren")
	hit := a.mustAttack(t, a.caio, a.get(t, a.master), "Toren", battleaxe, "Goblin 1", func(r *playv1.RollAttackRequest) { r.D20Faces = []int32{15, 3} })
	if hit.GetPendingDamage() == nil {
		t.Fatalf("the attack = %v, want a hit", hit.GetRoll())
	}
	a.h.roller.queue(1, 1)
	a.mustDamage(t, a.caio, a.get(t, a.master), hit.GetPendingDamage().GetId(), inAppDamage)
	now = a.get(t, a.master)
	if hasCondition(now, t, "Goblin 1", unconsciousC) || effectOfSpell(byLabel(t, now, "Goblin 1"), sleepSpell) {
		t.Errorf("the hurt goblin is still asleep: %v %v", byLabel(t, now, "Goblin 1").GetConditions(), byLabel(t, now, "Goblin 1").GetEffects())
	}
	if !hasCondition(now, t, "Goblin 2", unconsciousC) {
		t.Error("the goblin nobody hurt woke up")
	}
}

// Another creature shakes the sleeper awake (SRD: "or someone uses an action to shake or slap it"): the master ends the effect.
func TestTheMasterShakesASleeperAwake(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 2)
	a.stand(t, map[string][2]int32{"Pensantus": {1, 1}, "Brisa": {2, 1}, "Toren": {6, 3}, "Goblin 1": {7, 3}, "Goblin 2": {7, 4}})
	a.h.roller.queue(8, 8, 8, 8, 8)
	a.mustCast(t, a.ana, e, "Pensantus", sleepSpell, slotOfLevel(1), a.at(t, "Goblin 1", "Goblin 2"), poolInApp)
	row := byLabel(t, a.get(t, a.master), "Goblin 1").GetEffects()
	if len(row) != 1 {
		t.Fatalf("the sleeping goblin's effects = %v, want the sleep", row)
	}
	if _, err := a.master.lasting.EndLastingEffect(t.Context(), connect.NewRequest(&playv1.EndLastingEffectRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey(), EffectId: row[0].GetId(), Scope: playv1.EffectEndScope_EFFECT_END_SCOPE_THIS,
	})); err != nil {
		t.Fatalf("EndLastingEffect() error = %v", err)
	}
	now := a.get(t, a.master)
	if hasCondition(now, t, "Goblin 1", unconsciousC) || !hasCondition(now, t, "Goblin 2", unconsciousC) {
		t.Errorf("after shaking Goblin 1: %v and %v; want only Goblin 2 asleep", byLabel(t, now, "Goblin 1").GetConditions(), byLabel(t, now, "Goblin 2").GetConditions())
	}
}

// spellTable is a level 17 wizard (Pensantus, Intelligence 20), a cleric and a fighter, a goblin and, next to them, a Zumbi
// (undead), an Armadura (a construct immune to being charmed and to being blinded) and a Capitão Goblin of 27 hit points.
func spellTable(t *testing.T, races map[string]string, elfNPC ...bool) (*armed, *playv1.Encounter) {
	t.Helper()
	race := func(who, own string) string {
		if r, ok := races[who]; ok {
			return r
		}
		return own
	}
	a := newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", race("Toren", "race:human"), 5,
			&rulesv1.AbilityScores{Strength: 16, Dexterity: 13, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{battleaxe}, nil)
		spells := []string{sleepSpell, colorSpray}
		a.pens = a.ana.caster(t, a.campaignID, "Pensantus", "class:wizard", race("Pensantus", "race:gnome"), 17,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 20, Wisdom: 10, Charisma: 8}, nil, nil, spells, spells)
		a.bri = a.bia.caster(t, a.campaignID, "Brisa", "class:cleric", race("Brisa", "race:human"), 11,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 16, Constitution: 14, Intelligence: 10, Wisdom: 16, Charisma: 8}, []string{maceKey}, []string{spareDying}, nil,
			[]string{cureWounds})
	})
	p := plan{
		npcs: []*playv1.Participant{{CharacterId: a.goblin.GetId()}, {CharacterId: a.capitao.GetId()}}, npcRolls: []int{3, 2},
		players: map[string]int32{"Pensantus": 18, "Brisa": 16, "Toren": 14}, reveal: []string{"Goblin", "Capitão Goblin"}, setup: true,
		at: map[string][2]int32{"Pensantus": {3, 3}, "Brisa": {1, 1}, "Toren": {1, 2}, "Goblin": {5, 3}, "Capitão Goblin": {5, 4}},
	}
	if len(elfNPC) > 0 && elfNPC[0] { // an NPC the master wrote with the elf's race
		sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
			BaseScores: &rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 10, Wisdom: 10, Charisma: 8},
			RaceKey:    "race:elf", Classes: []*charactersv1.ClassLevel{{ClassKey: "class:fighter", Level: 1}}, WeaponKeys: []string{battleaxe},
		}}}
		res, err := a.master.characters.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
			CampaignId: a.campaignID, Kind: charactersv1.CharacterKind_CHARACTER_KIND_ENEMY, Name: "Elfo", Sheet: sheet,
		}))
		if err != nil {
			t.Fatalf("CreateCharacter() error = %v", err)
		}
		p.npcs, p.npcRolls, p.reveal = append(p.npcs, &playv1.Participant{CharacterId: res.Msg.GetCharacter().GetId()}), append(p.npcRolls, 1), append(p.reveal, "Elfo")
		p.at["Elfo"] = [2]int32{4, 3}
	}
	e := a.start(t, p)
	for name, key := range map[string]string{"Zumbi": "monster:zombie", "Armadura": "monster:animated-armor"} {
		a.h.roller.queue(10)
		e = a.mustAddMonsters(t, e, func(r *playv1.AddMonstersRequest) {
			r.CreatureKey, r.Count, r.Name, r.Hidden = key, 1, name, new(false)
		}).GetEncounter()
	}
	a.stand(t, map[string][2]int32{"Zumbi": {5, 2}, "Armadura": {6, 3}})
	e = a.begin(t, e)
	e = a.passTo(t, e, "Pensantus")
	return a, e
}

// The master reads, for each target, why it was skipped; a player reads the same "not affected" for every reason.
func TestSleepSkipsUndeadAndTheCharmImmuneAndTheirHitPointsAreNotSpent(t *testing.T) {
	t.Parallel()
	for _, viewer := range []string{"master", "player"} {
		t.Run(viewer, func(t *testing.T) {
			t.Parallel()
			a, e := spellTable(t, nil)
			u := a.master
			if viewer == "player" {
				u = a.ana
			}
			// 5d8 = 40: the Goblin (7) fits and 33 are left; the Zumbi (22) and the Armadura (33) would each fit too if the pool did not skip them.
			a.h.roller.queue(8, 8, 8, 8, 8)
			res := a.mustCast(t, u, e, "Pensantus", sleepSpell, slotOfLevel(1), a.at(t, "Goblin", "Zumbi", "Armadura"), poolInApp)
			now := a.get(t, a.master)
			if !hasCondition(now, t, "Goblin", unconsciousC) {
				t.Error("the Goblin was not put to sleep")
			}
			for label, words := range map[string]string{"Zumbi": wordsUndead, "Armadura": wordsCharmFree} {
				got := a.targetOf(t, e, res, label)
				if hasCondition(now, t, label, unconsciousC) || !notAffected(got) {
					t.Errorf("%s: asleep %v, effect %v; want skipped", label, hasCondition(now, t, label, unconsciousC), got.GetEffect())
				}
				want := ""
				if viewer == "master" {
					want = words
				}
				if got.GetNoEffectReasonMaster() != want {
					t.Errorf("%s: the %s reads %q, want %q", label, viewer, got.GetNoEffectReasonMaster(), want)
				}
				if viewer == "master" && got.GetEffect().GetPoolLeft() != 33 {
					t.Errorf("%s: pool left = %d, want 33: a skipped creature spends nothing", label, got.GetEffect().GetPoolLeft())
				}
			}
		})
	}
}

// Color Spray: the blindness is until the end of the caster's next turn, not for good.
func TestColorSprayBlindnessEndsAtTheEndOfTheCastersNextTurn(t *testing.T) {
	t.Parallel()
	a, e := spellTable(t, nil)
	a.put(t, "Pensantus", 3, 3)
	a.stand(t, map[string][2]int32{"Goblin": {4, 3}, "Zumbi": {3, 8}, "Armadura": {9, 9}, "Capitão Goblin": {9, 8}})
	a.h.roller.queue(10, 10, 10, 10, 10, 10)
	a.mustCast(t, a.ana, e, "Pensantus", colorSpray, slotOfLevel(1), a.at(t, "Goblin"), poolInApp)
	if !hasCondition(a.get(t, a.master), t, "Goblin", blindedC) {
		t.Fatal("the goblin was not blinded")
	}
	a.mustEndTurn(t, a.ana, e)
	now := a.passTo(t, e, "Pensantus") // the caster's next turn: the goblin still cannot see
	if !hasCondition(now, t, "Goblin", blindedC) {
		t.Fatal("the goblin could see again before the end of the caster's next turn")
	}
	now = a.mustEndTurn(t, a.ana, now)
	if hasCondition(a.get(t, a.master), t, "Goblin", blindedC) {
		t.Errorf("the goblin is still blind after the end of the caster's next turn: %v", byLabel(t, now, "Goblin").GetConditions())
	}
}

// Color Spray ignores the unconscious and whoever cannot see: a blinded creature, and one that has no sight (the Armadura is immune to
// being blinded). They spend nothing of the pool.
func TestColorSpraySkipsTheUnconsciousAndThoseWhoCannotSee(t *testing.T) {
	t.Parallel()
	a, e := spellTable(t, nil)
	a.stand(t, map[string][2]int32{"Goblin": {4, 3}, "Capitão Goblin": {5, 3}, "Zumbi": {6, 3}, "Armadura": {5, 4}})
	a.setConditions(t, e, "Goblin", unconsciousC)
	a.setConditions(t, e, "Capitão Goblin", blindedC)
	// 6d10 = 60: the Zumbi (22) is blinded and 38 are left; the Capitão (27) would fit too and the Armadura (33) would not, if the pool did not skip them.
	a.h.roller.queue(10, 10, 10, 10, 10, 10)
	res := a.mustCast(t, a.master, e, "Pensantus", colorSpray, slotOfLevel(1), a.at(t, "Goblin", "Capitão Goblin", "Zumbi", "Armadura"), poolInApp)
	now := a.get(t, a.master)
	if !hasCondition(now, t, "Zumbi", blindedC) {
		t.Error("the Zumbi was not blinded")
	}
	for label, words := range map[string]string{"Goblin": "", "Capitão Goblin": wordsCannotSee, "Armadura": wordsCannotSee} {
		left := int32(38)
		if label == "Goblin" {
			left = 60 // the lowest hit points: it comes first, before the Zumbi
		}
		got := a.targetOf(t, e, res, label)
		if !notAffected(got) || got.GetNoEffectReasonMaster() != words || got.GetEffect().GetPoolLeft() != left {
			t.Errorf("%s = %v, reason %q, pool left %d; want skipped, %q and %d left", label, got.GetEffect().GetOutcome(), got.GetNoEffectReasonMaster(), got.GetEffect().GetPoolLeft(), words, left)
		}
	}
	if effectOfSpell(byLabel(t, now, "Capitão Goblin"), colorSpray) || effectOfSpell(byLabel(t, now, "Armadura"), colorSpray) {
		t.Error("a creature that cannot see got Color Spray's effect")
	}
}

// elfTable is spellTable with Toren an elf and Pensantus a half-elf: both carry Fey Ancestry.
func elfTable(t *testing.T) (*armed, *playv1.Encounter) {
	t.Helper()
	return spellTable(t, map[string]string{"Toren": "race:elf", "Pensantus": "race:half-elf"})
}

// Fey Ancestry: magic cannot put an elf to sleep. The master reads the trait's own words, so does the elf's own player (here the
// caster, in the area of her own spell), and another player reads only that nothing happened; the pool is not spent on the elf.
func TestSleepDoesNotPutAnElfToSleep(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		viewer, target string
		by             func(*armed) *user
		want           string
	}{
		{"the master", "Toren", func(a *armed) *user { return a.master }, wordsFeyAncestry},
		{"the elf's own player", "Pensantus", func(a *armed) *user { return a.ana }, wordsFeyAncestry},
		{"another player", "Toren", func(a *armed) *user { return a.ana }, ""},
	} {
		t.Run(tc.viewer, func(t *testing.T) {
			t.Parallel()
			a, e := elfTable(t)
			a.stand(t, map[string][2]int32{"Toren": {4, 3}, "Goblin": {5, 3}, "Capitão Goblin": {9, 9}, "Zumbi": {9, 8}, "Armadura": {8, 9}})
			a.h.roller.queue(8, 8, 8, 8, 8)
			res := a.mustCast(t, tc.by(a), e, "Pensantus", sleepSpell, slotOfLevel(1), a.at(t, "Pensantus", "Toren", "Goblin"), poolInApp)
			now := a.get(t, a.master)
			for _, who := range []string{"Pensantus", "Toren"} {
				if hasCondition(now, t, who, unconsciousC) {
					t.Errorf("%s was put to sleep", who)
				}
			}
			if !hasCondition(now, t, "Goblin", unconsciousC) {
				t.Error("the Goblin was not put to sleep")
			}
			got := a.targetOf(t, e, res, tc.target)
			if !notAffected(got) || got.GetNoEffectReasonMaster() != tc.want {
				t.Errorf("%s reads for %s: %v, %q; want not affected and %q", tc.viewer, tc.target, got.GetEffect().GetOutcome(), got.GetNoEffectReasonMaster(), tc.want)
			}
			if tc.viewer == "the master" && got.GetEffect().GetPoolLeft() != 33 {
				t.Errorf("pool left after the elf = %d, want 33: Fey Ancestry spends nothing", got.GetEffect().GetPoolLeft())
			}
		})
	}
}

// Fey Ancestry is against magic that puts to sleep, not against unconsciousness: another source still works on an elf.
func TestAnotherSourceOfUnconsciousnessStillAffectsAnElf(t *testing.T) {
	t.Parallel()
	a, e := elfTable(t)
	a.setConditions(t, e, "Toren", unconsciousC)
	if !hasCondition(a.get(t, a.master), t, "Toren", unconsciousC) {
		t.Error("the master could not make the elf unconscious")
	}
}

// An NPC elf too (a character the master wrote with the elf's race): the sheet carries the trait, and only the master reads it.
func TestSleepSkipsAnNPCElfAndOnlyTheMasterReadsTheTrait(t *testing.T) {
	t.Parallel()
	for viewer, want := range map[string]string{"master": wordsFeyAncestry, "player": ""} {
		t.Run(viewer, func(t *testing.T) {
			t.Parallel()
			a, e := spellTable(t, nil, true)
			u := a.master
			if viewer == "player" {
				u = a.ana
			}
			a.stand(t, map[string][2]int32{"Capitão Goblin": {9, 9}, "Zumbi": {9, 8}, "Armadura": {8, 9}})
			a.h.roller.queue(8, 8, 8, 8, 8)
			res := a.mustCast(t, u, e, "Pensantus", sleepSpell, slotOfLevel(1), a.at(t, "Elfo", "Goblin"), poolInApp)
			if got := a.targetOf(t, e, res, "Elfo"); !notAffected(got) || got.GetNoEffectReasonMaster() != want {
				t.Errorf("the %s reads %v, %q for the NPC elf; want not affected and %q", viewer, got.GetEffect().GetOutcome(), got.GetNoEffectReasonMaster(), want)
			}
			if hasCondition(a.get(t, a.master), t, "Elfo", unconsciousC) {
				t.Error("the NPC elf fell asleep")
			}
		})
	}
}
