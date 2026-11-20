package play

import (
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// A spell with several rays takes a typed roll for each ray at a table with physical dice (#947), on the turn and from
// the window of a readied spell: the faces of each ray are the entry of ray_rolls, in the order of the targets and of
// the rays each target takes.

// newRayCasters is newCasters with Scorching Ray for Pensantus.
func newRayCasters(t *testing.T) *armed {
	t.Helper()
	return newArmedWith(t, func(a *armed) {
		twin := tableSpell("Dardo Duplo", 1, "class:wizard", rulesv1.SpellRangeKind_SPELL_RANGE_KIND_RANGED, &rulesv1.TableSpellTarget{
			Kind: rulesv1.TableSpellTargetKind_TABLE_SPELL_TARGET_KIND_CREATURES, Count: 2,
		})
		twin.Attack = "ranged"
		twin.Damage = []*rulesv1.TableSpellDamage{{DamageTypeKey: "damage-type:force", Dice: "1d10"}}
		if _, err := a.master.table.CreateTableEntry(t.Context(), connect.NewRequest(&rulesv1.CreateTableEntryRequest{
			CampaignId: a.campaignID, Body: &rulesv1.CreateTableEntryRequest_TableSpell{TableSpell: twin},
		})); err != nil {
			t.Fatalf("CreateTableEntry(%s) error = %v", twin.GetNamePt(), err)
		}
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 5,
			&rulesv1.AbilityScores{Strength: 16, Dexterity: 13, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{battleaxe}, nil)
		a.pens = a.ana.caster(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 5,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8}, nil, []string{fireBolt},
			[]string{scorchingRay, twinDart}, []string{scorchingRay, twinDart})
		a.bri = a.bia.caster(t, a.campaignID, "Brisa", "class:cleric", "race:human", 3,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 16, Constitution: 14, Intelligence: 10, Wisdom: 16, Charisma: 8}, []string{maceKey}, []string{sacredFlame}, nil,
			[]string{cureWounds, guidingBolt})
	})
}

// twinDart is a spell of the table with an attack roll and two targets: one attack roll for each, the spell that rolls
// several attacks that is no Scorching Ray. The SRD has none at this table's level.
const twinDart = "spell:dardo-duplo@mesa"

// rays lists one ray_rolls entry for each list of faces.
func rays(faces ...[]int32) []*playv1.RayRoll {
	var out []*playv1.RayRoll
	for _, f := range faces {
		out = append(out, &playv1.RayRoll{D20Faces: f})
	}
	return out
}

func withRays(r ...*playv1.RayRoll) func(*playv1.CastSpellRequest) {
	return func(req *playv1.CastSpellRequest) { req.RayRolls = r }
}

// The Capitão stands next to Pensantus, so each ray is rolled with disadvantage: a pair, the lower counts. Against the
// goblins' armor class 15 with +7: 20 is a critical hit, 2 a miss, 10 a hit.
var (
	rayCrit = []int32{20, 20}
	rayMiss = []int32{15, 2}
	rayHit  = []int32{18, 10}
)

func logFaces(line *playv1.CombatLogEntry) [][]int32 {
	var out [][]int32
	for _, tg := range line.GetSpell().GetTargets() {
		out = append(out, tg.GetAttackRoll().GetFaces())
	}
	return out
}

func TestCastScorchingRayAtAPhysicalTableTakesATypedRollForEachRay(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		darts map[string]int32
		order []string
		rolls [][]int32
		hit   []playv1.AttackOutcome
		at    []string
	}{
		{
			"three rays at one target",
			map[string]int32{"Goblin 1": 3},
			[]string{"Goblin 1"},
			[][]int32{rayCrit, rayMiss, rayHit},
			[]playv1.AttackOutcome{playv1.AttackOutcome_ATTACK_OUTCOME_CRITICAL_HIT, playv1.AttackOutcome_ATTACK_OUTCOME_MISS, playv1.AttackOutcome_ATTACK_OUTCOME_HIT},
			[]string{"Goblin 1", "Goblin 1", "Goblin 1"},
		},
		{
			"rays split over two targets",
			map[string]int32{"Goblin 1": 2, "Goblin 2": 1},
			[]string{"Goblin 1", "Goblin 2"},
			[][]int32{rayMiss, rayHit, rayCrit},
			[]playv1.AttackOutcome{playv1.AttackOutcome_ATTACK_OUTCOME_MISS, playv1.AttackOutcome_ATTACK_OUTCOME_HIT, playv1.AttackOutcome_ATTACK_OUTCOME_CRITICAL_HIT},
			[]string{"Goblin 1", "Goblin 1", "Goblin 2"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := newRayCasters(t)
			a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
			e := a.castersFight(t, 2)
			res := a.mustCast(t, a.ana, e, "Pensantus", scorchingRay, slotOfLevel(2), rayTargets(t, a, tc.darts, tc.order...), withRays(rays(tc.rolls...)...))
			got := res.GetCast().GetTargets()
			if len(got) != 3 {
				t.Fatalf("rays = %d, want 3", len(got))
			}
			for i, tg := range got {
				if tg.GetCombatantId() != a.id(t, tc.at[i]) || tg.GetOutcome() != tc.hit[i] || !slices.Equal(tg.GetAttackRoll().GetFaces(), tc.rolls[i]) {
					t.Errorf("ray %d = %v, want %s, %v, faces %v", i+1, tg, tc.at[i], tc.hit[i], tc.rolls[i])
				}
			}
			line := a.castLine(t, e, scorchingRay)
			if line == nil {
				t.Fatalf("no Scorching Ray line in the log")
			}
			for i, f := range logFaces(line) {
				if !slices.Equal(f, tc.rolls[i]) {
					t.Errorf("the log shows ray %d with faces %v, want %v", i+1, f, tc.rolls[i])
				}
			}
			if n := usedSlots(a.vitals(t, a.pens), 2); n != 1 {
				t.Errorf("2nd circle slots used = %d, want 1", n)
			}
		})
	}
}

// With no enemy next to the caster each ray is a normal roll: one face each, and the master may type them too.
func TestCastScorchingRayAtAPhysicalTableTakesOneFaceForANormalRay(t *testing.T) {
	t.Parallel()
	for name, by := range map[string]func(a *armed) *user{"the player": func(a *armed) *user { return a.ana }, "the master": func(a *armed) *user { return a.master }} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			a := newRayCasters(t)
			a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
			e := a.castersFight(t, 1)
			a.put(t, "Capitão Goblin", 1, 1)
			res := a.mustCast(t, by(a), e, "Pensantus", scorchingRay, slotOfLevel(2), rayTargets(t, a, map[string]int32{"Goblin": 3}, "Goblin"), withRays(rays([]int32{20}, []int32{2}, []int32{10})...))
			var faces [][]int32
			for _, tg := range res.GetCast().GetTargets() {
				faces = append(faces, tg.GetAttackRoll().GetFaces())
			}
			if want := [][]int32{{20}, {2}, {10}}; !slices.EqualFunc(faces, want, slices.Equal[[]int32, int32]) {
				t.Errorf("faces = %v, want %v", faces, want)
			}
		})
	}
}

func TestCastScorchingRayAtAPhysicalTableRefusesWrongRayRollsAndSpendsNothing(t *testing.T) {
	t.Parallel()
	one := []int32{12}
	cases := []struct {
		name string
		roll func(*playv1.CastSpellRequest)
	}{
		{"too few entries", withRays(rays(rayHit, rayHit)...)},
		{"too many entries", withRays(rays(rayHit, rayHit, rayHit, rayHit)...)},
		{"one face where the ray rolls a pair", withRays(rays(rayHit, one, rayHit)...)},
		{"three faces in an entry", withRays(rays(rayHit, []int32{1, 2, 3}, rayHit)...)},
		{"an entry with no face", withRays(rays(rayHit, nil, rayHit)...)},
		{"a face of 0", withRays(rays(rayHit, []int32{0, 5}, rayHit)...)},
		{"a face of 21", withRays(rays(rayHit, []int32{5, 21}, rayHit)...)},
		{"only d20_face", func(r *playv1.CastSpellRequest) { r.Roll = &playv1.CastSpellRequest_D20Face{D20Face: 12} }},
		{"only d20_faces", func(r *playv1.CastSpellRequest) { r.D20Faces = []int32{12, 3} }},
		{"ray_rolls with d20_face", func(r *playv1.CastSpellRequest) {
			r.Roll = &playv1.CastSpellRequest_D20Face{D20Face: 12}
			r.RayRolls = rays(rayHit, rayHit, rayHit)
		}},
		{"ray_rolls with d20_faces", func(r *playv1.CastSpellRequest) {
			r.D20Faces = []int32{12, 3}
			r.RayRolls = rays(rayHit, rayHit, rayHit)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := newRayCasters(t)
			a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
			e := a.castersFight(t, 1)
			before, _, _ := a.hp(t, "Goblin")
			_, err := a.cast(t, a.ana, e, "Pensantus", scorchingRay, slotOfLevel(2), rayTargets(t, a, map[string]int32{"Goblin": 3}, "Goblin"), tc.roll)
			wantCode(t, tc.name, err, connect.CodeInvalidArgument)
			if after, _, _ := a.hp(t, "Goblin"); after != before {
				t.Errorf("the target's hit points = %d, want %d: a refused cast changes nothing", after, before)
			}
			if n := usedSlots(a.vitals(t, a.pens), 2); n != 0 {
				t.Errorf("2nd circle slots used = %d, want none spent", n)
			}
			if a.castLine(t, e, scorchingRay) != nil {
				t.Errorf("a refused cast left a line in the log")
			}
		})
	}
}

func TestRayRollsAreRefusedForACastThatHasNoSeveralRays(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		table campaignsv1.DiceMode
		spell string
		slot  *playv1.SpellSlot
		darts map[string]int32
		roll  func(*playv1.CastSpellRequest)
		code  connect.Code
	}{
		{"a single ray", campaignsv1.DiceMode_DICE_MODE_PHYSICAL, guidingBolt, slotOfLevel(1), map[string]int32{"Goblin": 0}, withRays(rays(rayHit, rayHit)...), connect.CodeInvalidArgument},
		{
			"one entry for the three rays of a ray spell", campaignsv1.DiceMode_DICE_MODE_PHYSICAL, scorchingRay, slotOfLevel(2),
			map[string]int32{"Goblin": 3},
			withRays(rays(rayHit)...), connect.CodeInvalidArgument,
		},
		{"a save spell", campaignsv1.DiceMode_DICE_MODE_PHYSICAL, sacredFlame, nil, map[string]int32{"Goblin": 0}, withRays(rays(rayHit)...), connect.CodeInvalidArgument},
		{
			"roll_in_app", campaignsv1.DiceMode_DICE_MODE_APP, scorchingRay, slotOfLevel(2),
			map[string]int32{"Goblin": 3},
			func(r *playv1.CastSpellRequest) {
				r.Roll = &playv1.CastSpellRequest_RollInApp{RollInApp: true}
				r.RayRolls = rays(rayHit, rayHit, rayHit)
			}, connect.CodeInvalidArgument,
		},
		{
			"an in-app table", campaignsv1.DiceMode_DICE_MODE_APP, scorchingRay, slotOfLevel(2),
			map[string]int32{"Goblin": 3},
			withRays(rays(rayHit, rayHit, rayHit)...), connect.CodeFailedPrecondition,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := newRayCasters(t)
			a.forceDice(t, tc.table)
			e := a.castersFight(t, 1)
			caster, by := "Pensantus", a.ana
			if tc.spell == guidingBolt || tc.spell == sacredFlame {
				caster, by, e = "Brisa", a.bia, a.turnOf(t, "Brisa")
			}
			before, _, _ := a.hp(t, "Goblin")
			_, err := a.cast(t, by, e, caster, tc.spell, tc.slot, rayTargets(t, a, tc.darts, "Goblin"), tc.roll)
			wantCode(t, tc.name, err, tc.code)
			if after, _, _ := a.hp(t, "Goblin"); after != before {
				t.Errorf("the target's hit points = %d, want %d: a refused cast changes nothing", after, before)
			}
			if a.castLine(t, e, tc.spell) != nil {
				t.Errorf("a refused cast left a line in the log")
			}
		})
	}
}

// An in-app table keeps rolling the rays in the app.
func TestCastScorchingRayAtAnInAppTableIsUnchanged(t *testing.T) {
	t.Parallel()
	a := newRayCasters(t)
	a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_APP)
	e := a.castersFight(t, 1)
	a.h.roller.queue(20, 20, 2, 2, 10, 10)
	res := a.mustCast(t, a.ana, e, "Pensantus", scorchingRay, slotOfLevel(2), rayTargets(t, a, map[string]int32{"Goblin": 3}, "Goblin"), poolInApp)
	if n := len(res.GetCast().GetTargets()); n != 3 {
		t.Errorf("rays = %d, want 3", n)
	}
}

// The release of a held Scorching Ray takes the rays' dice from the window.
func (a *armed) readyScorchingRay(t *testing.T) *playv1.Encounter {
	t.Helper()
	a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
	return a.readyForRelease(t, scorchingRay, 2, "Goblin")
}

func withRayRolls(faces ...[]int32) func(*playv1.AnswerReactionRequest) {
	return func(r *playv1.AnswerReactionRequest) {
		useSpellAnswer(r)
		r.Targets = []*playv1.SpellTarget{{Darts: 3}}
		r.RayRolls = rays(faces...)
	}
}

func TestAReadiedScorchingRayIsReleasedWithTypedRays(t *testing.T) {
	t.Parallel()
	for name, by := range map[string]func(a *armed) *user{"the player": func(a *armed) *user { return a.ana }, "the master": func(a *armed) *user { return a.master }} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			a := newRayCasters(t)
			e := a.readyScorchingRay(t)
			w := a.mustFire(t, e, "Pensantus")
			goblin := a.id(t, "Goblin")
			set := func(r *playv1.AnswerReactionRequest) {
				withRayRolls(rayCrit, rayMiss, rayHit)(r)
				r.Targets[0].CombatantId = goblin
			}
			res := a.mustAnswer(t, by(a), e, w.GetId(), set)
			if !res.GetResult().GetUsed() {
				t.Fatalf("the use's result = %v", res.GetResult())
			}
			line := a.castLine(t, e, scorchingRay)
			if line == nil {
				t.Fatalf("no Scorching Ray line in the log")
			}
			if got, want := logFaces(line), [][]int32{rayCrit, rayMiss, rayHit}; !slices.EqualFunc(got, want, slices.Equal[[]int32, int32]) {
				t.Errorf("the log shows %v, want %v", got, want)
			}
			if a.readiedStatus(t, "Pensantus") != readiedFired {
				t.Errorf("row %q, want fired", a.readiedStatus(t, "Pensantus"))
			}
		})
	}
}

func TestAReadiedScorchingRayRefusesRayRollsItDoesNotUse(t *testing.T) {
	t.Parallel()
	a := newRayCasters(t)
	e := a.readyScorchingRay(t)
	w := a.mustFire(t, e, "Pensantus")
	pass := func(r *playv1.AnswerReactionRequest) {
		r.Answer, r.RayRolls = playv1.ReactionChoice_REACTION_CHOICE_PASS, rays(rayHit)
	}
	_, err := a.answerReaction(t, a.ana, e, w.GetId(), pass)
	wantCode(t, "ray_rolls on a pass", err, connect.CodeInvalidArgument)
	wrong := func(r *playv1.AnswerReactionRequest) {
		withRayRolls(rayHit, rayHit)(r)
		r.Targets[0].CombatantId = a.id(t, "Goblin")
	}
	_, err = a.answerReaction(t, a.ana, e, w.GetId(), wrong)
	wantCode(t, "two rays for three", err, connect.CodeInvalidArgument)
	if !a.readiedOpen(t, "Pensantus") || a.readiedStatus(t, "Pensantus") != readiedWaiting {
		t.Errorf("a refused answer left the window open %v and row %q", a.readiedOpen(t, "Pensantus"), a.readiedStatus(t, "Pensantus"))
	}
}

func TestAReadiedScorchingRayReleaseReplaysWithTheSameKey(t *testing.T) {
	t.Parallel()
	a := newRayCasters(t)
	e := a.readyScorchingRay(t)
	w := a.mustFire(t, e, "Pensantus")
	key := newKey()
	again := func(r *playv1.AnswerReactionRequest) {
		withRayRolls(rayCrit, rayMiss, rayHit)(r)
		r.Targets[0].CombatantId, r.IdempotencyKey = a.id(t, "Goblin"), key
	}
	first := a.mustAnswer(t, a.ana, e, w.GetId(), again)
	second := a.mustAnswer(t, a.ana, e, w.GetId(), again)
	if !first.GetResult().GetUsed() || !second.GetResult().GetUsed() {
		t.Errorf("results = %v, %v", first.GetResult(), second.GetResult())
	}
	if n := len(a.castLines(t, e, scorchingRay)); n != 1 {
		t.Errorf("Scorching Ray lines = %d, want 1", n)
	}
}

// An in-app table releases a held Scorching Ray as before.
func TestAReadiedScorchingRayAtAnInAppTableIsUnchanged(t *testing.T) {
	t.Parallel()
	a := newRayCasters(t)
	a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_APP)
	e := a.readyForRelease(t, scorchingRay, 2, "Goblin")
	w := a.mustFire(t, e, "Pensantus")
	a.h.roller.queue(20, 20, 2, 2, 10, 10)
	res := a.mustAnswer(t, a.ana, e, w.GetId(), func(r *playv1.AnswerReactionRequest) {
		useSpellAnswer(r)
		r.Targets = []*playv1.SpellTarget{{CombatantId: a.id(t, "Goblin"), Darts: 3}}
	})
	if !res.GetResult().GetUsed() {
		t.Fatalf("the use's result = %v", res.GetResult())
	}
	if line := a.castLine(t, e, scorchingRay); line == nil || len(line.GetSpell().GetTargets()) != 3 {
		t.Errorf("the release's line = %v, want three rays", line)
	}
}

// A spell that rolls one attack for each of its targets, and is no Scorching Ray, takes ray_rolls too: one entry per attack.
func TestCastASpellWithSeveralAttackRollsTakesOneRayRollEach(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		roll func(*playv1.CastSpellRequest)
		ok   bool
	}{
		{"an entry for each target", withRays(rays(rayCrit, rayMiss)...), true},
		{"one entry for two targets", withRays(rays(rayHit)...), false},
		{"one d20_face for two targets", func(r *playv1.CastSpellRequest) { r.Roll = &playv1.CastSpellRequest_D20Face{D20Face: 12} }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := newRayCasters(t)
			a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
			e := a.castersFight(t, 2)
			targets := a.at(t, "Goblin 1", "Goblin 2")
			if !tc.ok {
				before, _, _ := a.hp(t, "Goblin 1")
				_, err := a.cast(t, a.ana, e, "Pensantus", twinDart, slotOfLevel(1), targets, tc.roll)
				wantCode(t, tc.name, err, connect.CodeInvalidArgument)
				if after, _, _ := a.hp(t, "Goblin 1"); after != before || usedSlots(a.vitals(t, a.pens), 1) != 0 {
					t.Errorf("a refused cast changed the hit points (%d, was %d) or spent a slot", after, before)
				}
				return
			}
			res := a.mustCast(t, a.ana, e, "Pensantus", twinDart, slotOfLevel(1), targets, tc.roll)
			got := res.GetCast().GetTargets()
			if len(got) != 2 || got[0].GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_CRITICAL_HIT || got[1].GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_MISS ||
				!slices.Equal(got[0].GetAttackRoll().GetFaces(), rayCrit) || !slices.Equal(got[1].GetAttackRoll().GetFaces(), rayMiss) {
				t.Errorf("targets = %v, want a critical hit with %v and a miss with %v", got, rayCrit, rayMiss)
			}
		})
	}
}

// An upcast Scorching Ray at the 3rd level makes four rays: four entries pass, three (or five) are refused.
func TestCastAnUpcastScorchingRayTakesFourRayRolls(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		rolls [][]int32
		ok    bool
	}{
		{"four entries", [][]int32{rayCrit, rayMiss, rayHit, rayHit}, true},
		{"three entries", [][]int32{rayCrit, rayMiss, rayHit}, false},
		{"five entries", [][]int32{rayCrit, rayMiss, rayHit, rayHit, rayHit}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := newRayCasters(t)
			a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
			e := a.castersFight(t, 1)
			before, _, _ := a.hp(t, "Goblin")
			res, err := a.cast(t, a.ana, e, "Pensantus", scorchingRay, slotOfLevel(3), rayTargets(t, a, map[string]int32{"Goblin": 4}, "Goblin"), withRays(rays(tc.rolls...)...))
			if !tc.ok {
				wantCode(t, tc.name, err, connect.CodeInvalidArgument)
				if after, _, _ := a.hp(t, "Goblin"); after != before || usedSlots(a.vitals(t, a.pens), 3) != 0 {
					t.Errorf("a refused cast changed the hit points (%d, was %d) or spent a slot", after, before)
				}
				return
			}
			if err != nil {
				t.Fatalf("CastSpell error = %v", err)
			}
			if n := len(res.GetCast().GetTargets()); n != 4 {
				t.Errorf("rays = %d, want 4", n)
			}
			if n := usedSlots(a.vitals(t, a.pens), 3); n != 1 {
				t.Errorf("3rd circle slots used = %d, want 1", n)
			}
		})
	}
}

// The mode of a ray is its target's: a prone goblin more than 5 feet away is hit with disadvantage (a pair for each of
// its rays), a plain one with a single d20. A wrong count of faces names the ray it was typed for.
func TestCastScorchingRayTakesTheFacesEachTargetsModeAsks(t *testing.T) {
	t.Parallel()
	setup := func(t *testing.T) (*armed, *playv1.Encounter, []*playv1.SpellTarget) {
		t.Helper()
		a := newRayCasters(t)
		a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
		e := a.castersFight(t, 2)
		a.put(t, "Capitão Goblin", 1, 1)
		a.setConditions(t, e, "Goblin 1", "condition:prone")
		return a, e, rayTargets(t, a, map[string]int32{"Goblin 1": 2, "Goblin 2": 1}, "Goblin 1", "Goblin 2")
	}
	t.Run("typed for each target's mode", func(t *testing.T) {
		t.Parallel()
		a, e, targets := setup(t)
		res := a.mustCast(t, a.ana, e, "Pensantus", scorchingRay, slotOfLevel(2), targets, withRays(rays(rayHit, rayMiss, []int32{12})...))
		var got [][]int32
		for _, tg := range res.GetCast().GetTargets() {
			got = append(got, tg.GetAttackRoll().GetFaces())
		}
		if want := [][]int32{rayHit, rayMiss, {12}}; !slices.EqualFunc(got, want, slices.Equal[[]int32, int32]) {
			t.Errorf("faces = %v, want %v", got, want)
		}
	})
	for name, tc := range map[string]struct {
		rolls []*playv1.RayRoll
		ray   string
	}{
		"one face for the second ray of the prone goblin": {rays(rayHit, []int32{12}, []int32{12}), "ray_rolls[1]"},
		"a pair for the plain goblin":                     {rays(rayHit, rayMiss, rayHit), "ray_rolls[2]"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			a, e, targets := setup(t)
			_, err := a.cast(t, a.ana, e, "Pensantus", scorchingRay, slotOfLevel(2), targets, withRays(tc.rolls...))
			wantCode(t, name, err, connect.CodeInvalidArgument)
			if !strings.Contains(err.Error(), tc.ray+":") {
				t.Errorf("error = %v, want it to name %s", err, tc.ray)
			}
			if usedSlots(a.vitals(t, a.pens), 2) != 0 || a.castLine(t, e, scorchingRay) != nil {
				t.Errorf("a refused cast spent a slot or left a line in the log")
			}
		})
	}
}

// A 1 and a 20 on separate rays: the 1 misses and the 20 is a critical hit, each opening (or not) its own damage.
func TestCastScorchingRayRecordsANatural1AndANatural20OnSeparateRays(t *testing.T) {
	t.Parallel()
	a := newRayCasters(t)
	a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
	e := a.castersFight(t, 1)
	res := a.mustCast(t, a.ana, e, "Pensantus", scorchingRay, slotOfLevel(2), rayTargets(t, a, map[string]int32{"Goblin": 3}, "Goblin"), withRays(rays([]int32{1, 1}, []int32{20, 20}, rayHit)...))
	got := res.GetCast().GetTargets()
	want := []playv1.AttackOutcome{playv1.AttackOutcome_ATTACK_OUTCOME_MISS, playv1.AttackOutcome_ATTACK_OUTCOME_CRITICAL_HIT, playv1.AttackOutcome_ATTACK_OUTCOME_HIT}
	if len(got) != 3 {
		t.Fatalf("rays = %d, want 3", len(got))
	}
	for i, tg := range got {
		if tg.GetOutcome() != want[i] {
			t.Errorf("ray %d = %v, want %v", i+1, tg.GetOutcome(), want[i])
		}
	}
	var dice []int32
	for _, p := range res.GetCast().GetPendingDamages() {
		dice = append(dice, p.GetDiceCount())
	}
	if !slices.Equal(dice, []int32{4, 2}) {
		t.Errorf("pending damage dice = %v, want 4 (the critical) then 2 (the hit), none for the 1", dice)
	}
}

// A refused request replayed with its key is refused again and stores nothing.
func TestCastScorchingRayRefusedRayRollsReplayRefusedAndStoreNothing(t *testing.T) {
	t.Parallel()
	a := newRayCasters(t)
	a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
	e := a.castersFight(t, 1)
	key := newKey()
	for i := range 2 {
		before, _, _ := a.hp(t, "Goblin")
		_, err := a.castKey(t, a.ana, e, "Pensantus", scorchingRay, slotOfLevel(2), rayTargets(t, a, map[string]int32{"Goblin": 3}, "Goblin"), withRays(rays(rayHit, rayHit)...), key)
		wantCode(t, "a refused cast, try "+string(rune('1'+i)), err, connect.CodeInvalidArgument)
		if after, _, _ := a.hp(t, "Goblin"); after != before || usedSlots(a.vitals(t, a.pens), 2) != 0 || a.castLine(t, e, scorchingRay) != nil {
			t.Errorf("try %d: a refused cast changed the hit points (%d, was %d), spent a slot or left a line", i+1, after, before)
		}
	}
	// The key is free: the same key with the right rays casts.
	_, err := a.castKey(t, a.ana, e, "Pensantus", scorchingRay, slotOfLevel(2), rayTargets(t, a, map[string]int32{"Goblin": 3}, "Goblin"), withRays(rays(rayHit, rayHit, rayHit)...), key)
	if err != nil || a.castLine(t, e, scorchingRay) == nil {
		t.Errorf("the key of a refused cast could not be used again")
	}
}

// A readied release whose ray typed one face where the ray rolls a pair is refused: nothing spent, the window open.
func TestAReadiedScorchingRayRefusesAModeMismatchAndSpendsNothing(t *testing.T) {
	t.Parallel()
	a := newRayCasters(t)
	e := a.readyScorchingRay(t)
	w := a.mustFire(t, e, "Pensantus")
	slots := usedSlots(a.vitals(t, a.pens), 2) // the readying spent it
	before, _, _ := a.hp(t, "Goblin")
	_, err := a.answerReaction(t, a.ana, e, w.GetId(), func(r *playv1.AnswerReactionRequest) {
		withRayRolls([]int32{12}, []int32{12}, []int32{12})(r)
		r.Targets[0].CombatantId = a.id(t, "Goblin")
	})
	wantCode(t, "a face for a pair", err, connect.CodeInvalidArgument)
	if after, _, _ := a.hp(t, "Goblin"); after != before || usedSlots(a.vitals(t, a.pens), 2) != slots || a.castLine(t, e, scorchingRay) != nil {
		t.Errorf("a refused release changed the hit points (%d, was %d), the slots or the log", after, before)
	}
	if !a.readiedOpen(t, "Pensantus") || a.readiedStatus(t, "Pensantus") != readiedWaiting {
		t.Errorf("the window is open %v and the row %q, want open and waiting", a.readiedOpen(t, "Pensantus"), a.readiedStatus(t, "Pensantus"))
	}
}

// The answer refuses ray_rolls beside its own way of rolling, in its own terms, and on a readied attack.
func TestAReadiedReleaseRefusesRayRollsBesideOtherDiceAndOnAnAttack(t *testing.T) {
	t.Parallel()
	a := newRayCasters(t)
	e := a.readyScorchingRay(t)
	w := a.mustFire(t, e, "Pensantus")
	for name, set := range map[string]func(*playv1.AnswerReactionRequest){
		"typed":     func(r *playv1.AnswerReactionRequest) { r.Roll = &playv1.AnswerReactionRequest_Typed{Typed: 12} },
		"d20_faces": func(r *playv1.AnswerReactionRequest) { r.D20Faces = []int32{12, 3} },
	} {
		_, err := a.answerReaction(t, a.ana, e, w.GetId(), func(r *playv1.AnswerReactionRequest) {
			withRayRolls(rayHit, rayHit, rayHit)(r)
			r.Targets[0].CombatantId = a.id(t, "Goblin")
			set(r)
		})
		wantCode(t, name+" with ray_rolls", err, connect.CodeInvalidArgument)
		if strings.Contains(err.Error(), "d20_face,") {
			t.Errorf("%s with ray_rolls: error = %v, want the answer's own fields, not the cast's", name, err)
		}
	}
	if !a.readiedOpen(t, "Pensantus") || a.readiedStatus(t, "Pensantus") != readiedWaiting {
		t.Errorf("a refused answer left the window open %v and row %q", a.readiedOpen(t, "Pensantus"), a.readiedStatus(t, "Pensantus"))
	}
}

func TestAReadiedAttackRefusesRayRolls(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
	e := a.readyThenPass(t, nil)
	w := a.mustFire(t, e, "Toren")
	_, err := a.answerReaction(t, a.caio, e, w.GetId(), func(r *playv1.AnswerReactionRequest) {
		useSpellAnswer(r)
		r.RayRolls = rays(rayHit)
	})
	wantCode(t, "ray_rolls on a readied attack", err, connect.CodeInvalidArgument)
	if a.readiedStatus(t, "Toren") != readiedWaiting {
		t.Errorf("row %q, want waiting", a.readiedStatus(t, "Toren"))
	}
}
