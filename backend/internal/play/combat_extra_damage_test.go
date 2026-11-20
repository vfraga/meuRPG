package play

import (
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// The extra_damage effect of the table's features on the attack card (a damage added to a weapon hit once
// per turn, when the target is what the effect asks, from a resource if it names one). The names are
// invented; these tests need no database.

const (
	giantFeature = "feature:golpe-de-gigante@mesa"
	giantUses    = "golpes_de_gigante"
)

func giantTrait(applies, resource string) link.ExtraDamage {
	return link.ExtraDamage{Key: giantFeature, NamePT: "Golpe de Gigante", Amount: 3, Applies: applies, Resource: resource, TextPT: "Só contra maiores."}
}

// giantFacts is a hit of a player's character of the given size on a target of the other size.
func giantFacts(attackerSize, targetSize string, extras ...link.ExtraDamage) hitFacts {
	return hitFacts{
		attacker: playdb.Combatant{ID: "a", Kind: kindPlayer, Size: attackerSize, FeatureState: []byte(`{}`)},
		target:   playdb.Combatant{ID: "t", Size: targetSize},
		attack:   link.Attack{Key: "e", Name: "Espada", Weapon: true, Melee: true, DamageType: "damage-type:slashing"},
		sheet:    link.Sheet{Traits: link.Traits{ExtraDamages: extras}},
		usesLeft: map[string]int32{giantUses: 2},
		enc:      playdb.Encounter{Round: 2},
	}
}

func partByKey(parts []partRecord, key string) (partRecord, bool) {
	for _, p := range parts {
		if p.Key == key {
			return p, true
		}
	}
	return partRecord{}, false
}

func TestExtraDamageIsOfferedAfterAHitOnALargerTarget(t *testing.T) {
	t.Parallel()
	parts := giantFacts("medium", "large", giantTrait(rules.ExtraDamageLargerTarget, giantUses)).extraDamageParts()
	p, ok := partByKey(parts, giantFeature)
	if !ok || len(parts) != 1 {
		t.Fatalf("parts = %+v, want the extra damage alone", parts)
	}
	if p.Kind != partExtra || !p.Choosable || p.Selected || !p.Available || p.Flat != 3 || p.Count != 0 || p.Label != "Golpe de Gigante" ||
		p.DamageType != "damage-type:slashing" || p.Resource != giantUses || p.ResourceLeft != 2 || p.Rider != "Só contra maiores." || !p.Once || p.Table != "extra_damage" {
		t.Errorf("part = %+v, want a choosable flat 3 of the weapon's type, once a turn, spending %s", p, giantUses)
	}
}

func TestExtraDamageOnASameOrSmallerTargetIsOffTellingWhy(t *testing.T) {
	t.Parallel()
	for _, target := range []string{"medium", "small", "tiny"} {
		parts := giantFacts("medium", target, giantTrait(rules.ExtraDamageLargerTarget, "")).extraDamageParts()
		p, ok := partByKey(parts, giantFeature)
		if !ok || p.Available || p.Reason == "" {
			t.Errorf("target %s: part = %+v, want it listed and off with a reason", target, p)
		}
	}
	// A small attacker against a medium target: larger, so it holds.
	parts := giantFacts("small", "medium", giantTrait(rules.ExtraDamageLargerTarget, "")).extraDamageParts()
	if p, _ := partByKey(parts, giantFeature); !p.Available {
		t.Errorf("a small attacker on a medium target: part = %+v, want it available", p)
	}
	// An unknown size is medium (the sheet says none).
	parts = giantFacts("", "large", giantTrait(rules.ExtraDamageLargerTarget, "")).extraDamageParts()
	if p, _ := partByKey(parts, giantFeature); !p.Available {
		t.Errorf("an attacker with no size on a large target: part = %+v, want it available", p)
	}
}

func TestExtraDamageAgainstAnyTargetIgnoresSize(t *testing.T) {
	t.Parallel()
	p, ok := partByKey(giantFacts("large", "tiny", giantTrait(rules.ExtraDamageAnyTarget, "")).extraDamageParts(), giantFeature)
	if !ok || !p.Available || p.Resource != "" || p.ResourceLeft != 0 {
		t.Errorf("part = %+v, want it available, with no resource", p)
	}
}

func TestExtraDamageUsedThisTurnIsOffUntilTheNextTurn(t *testing.T) {
	t.Parallel()
	h := giantFacts("medium", "large", giantTrait(rules.ExtraDamageLargerTarget, ""))
	h.attacker.FeatureState = turnMarkState(giantFeature, turnKey(h.enc))
	if p, _ := partByKey(h.extraDamageParts(), giantFeature); p.Available || p.Reason == "" {
		t.Errorf("used this turn: part = %+v, want it off with a reason", p)
	}
	h.enc.Round = 3 // another turn
	if p, _ := partByKey(h.extraDamageParts(), giantFeature); !p.Available {
		t.Errorf("next turn: part = %+v, want it available again", p)
	}
}

func TestExtraDamageWithoutUsesLeftIsOff(t *testing.T) {
	t.Parallel()
	h := giantFacts("medium", "large", giantTrait(rules.ExtraDamageLargerTarget, giantUses))
	h.usesLeft = map[string]int32{giantUses: 0}
	if p, _ := partByKey(h.extraDamageParts(), giantFeature); p.Available || p.Reason != "Sem usos do recurso" {
		t.Errorf("part = %+v, want it off: no uses left", p)
	}
}

func TestExtraDamageIsForAPlayersWeaponAttackOnly(t *testing.T) {
	t.Parallel()
	npc := giantFacts("medium", "large", giantTrait(rules.ExtraDamageAnyTarget, ""))
	npc.attacker.Kind = "npc"
	if got := npc.extraDamageParts(); len(got) != 0 {
		t.Errorf("an NPC's hit offers %+v", got)
	}
	spell := giantFacts("medium", "large", giantTrait(rules.ExtraDamageAnyTarget, ""))
	spell.attack.Weapon = false
	if got := spell.extraDamageParts(); len(got) != 0 {
		t.Errorf("a spell's hit offers %+v", got)
	}
	if got := giantFacts("medium", "large").extraDamageParts(); len(got) != 0 {
		t.Errorf("a character with no extra damage offers %+v", got)
	}
}

// The extra is one more line of the damage of the hit, beside the weapon: buildParts lists it.
func TestTheHitPartsHoldTheExtraDamageBesideTheWeapon(t *testing.T) {
	t.Parallel()
	h := giantFacts("medium", "large", giantTrait(rules.ExtraDamageLargerTarget, ""))
	h.names = func(string) string { return "" }
	parts := h.buildParts()
	if len(parts) != 2 || parts[0].Key != partWeapon || parts[1].Key != giantFeature {
		t.Fatalf("parts = %+v, want the weapon then the extra damage", parts)
	}
}

// ---- dice, damage type, weapon and frequency of an extra_damage (invented: "a feat: once per turn, +1d6 fire on a ranged weapon hit") ----

func emberTrait(x func(*link.ExtraDamage)) link.ExtraDamage {
	e := link.ExtraDamage{
		Key: giantFeature, NamePT: "Brasa Viva", Applies: rules.ExtraDamageAnyTarget, DiceCount: 1, DiceSides: 6,
		DamageType: "damage-type:fire", Weapon: rules.ExtraDamageRangedWeapon, Frequency: rules.ExtraDamageOncePerTurn,
	}
	x(&e)
	return e
}

func TestExtraDamageDiceArePartsOfTheirOwnTypeThatTheCriticalHitDoubles(t *testing.T) {
	t.Parallel()
	h := giantFacts("medium", "medium", emberTrait(func(*link.ExtraDamage) {}))
	h.attack.Melee = false
	p, ok := partByKey(h.extraDamageParts(), giantFeature)
	if !ok || !p.Available || p.Count != 1 || p.Sides != 6 || p.Flat != 0 || p.DamageType != "damage-type:fire" || p.NoCrit || !p.Once {
		t.Fatalf("part = %+v, want 1d6 of fire, available, once a turn, doubled on a critical", p)
	}
	if count, fixed := partCriticalDice(p, true, combat.CriticalDoubledDice); count != 2 || fixed != 0 {
		t.Errorf("critical dice = %d and %d fixed, want 2 and 0", count, fixed)
	}
	if count, fixed := partCriticalDice(p, true, combat.CriticalMaxPlusRoll); count != 1 || fixed != 6 {
		t.Errorf("critical dice under the maximum-plus-roll rule = %d and %d fixed, want 1 and 6", count, fixed)
	}
	if count, _ := partCriticalDice(p, false, combat.CriticalDoubledDice); count != 1 {
		t.Errorf("dice on a plain hit = %d, want 1", count)
	}
}

func TestExtraDamageThatKeepsItsDiceOnACriticalHit(t *testing.T) {
	t.Parallel()
	h := giantFacts("medium", "medium", emberTrait(func(x *link.ExtraDamage) { x.NoCritDoubling = true }))
	h.attack.Melee = false
	p, _ := partByKey(h.extraDamageParts(), giantFeature)
	if !p.NoCrit {
		t.Fatalf("part = %+v, want NoCrit", p)
	}
	for _, rule := range []combat.CriticalRule{combat.CriticalDoubledDice, combat.CriticalMaxPlusRoll} {
		if count, fixed := partCriticalDice(p, true, rule); count != 1 || fixed != 0 {
			t.Errorf("rule %d: critical dice = %d and %d fixed, want 1 and 0", rule, count, fixed)
		}
	}
	shown := partsProto(playdb.PendingDamage{Critical: true, Parts: mustJSON([]partRecord{p})}, combat.CriticalDoubledDice, dtPT)
	if len(shown) != 1 || shown[0].GetDiceCount() != 1 || shown[0].GetDoubled() {
		t.Errorf("shown = %v, want one die, not doubled", shown)
	}
	// The plain one is shown doubled.
	p.NoCrit = false
	shown = partsProto(playdb.PendingDamage{Critical: true, Parts: mustJSON([]partRecord{p})}, combat.CriticalDoubledDice, dtPT)
	if len(shown) != 1 || shown[0].GetDiceCount() != 2 || !shown[0].GetDoubled() {
		t.Errorf("shown = %v, want two dice, doubled", shown)
	}
}

func TestExtraDamageWithoutATypeTakesTheWeaponsOwn(t *testing.T) {
	t.Parallel()
	h := giantFacts("medium", "medium", emberTrait(func(x *link.ExtraDamage) { x.DamageType, x.Weapon = "", rules.ExtraDamageAnyWeapon }))
	if p, _ := partByKey(h.extraDamageParts(), giantFeature); p.DamageType != "damage-type:slashing" {
		t.Errorf("damage type = %q, want the weapon's slashing", p.DamageType)
	}
}

func TestExtraDamageGoesWithTheKindOfWeaponItNames(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		weapon string
		melee  bool
		holds  bool
	}{
		{rules.ExtraDamageAnyWeapon, true, true},
		{rules.ExtraDamageAnyWeapon, false, true},
		{rules.ExtraDamageMeleeWeapon, true, true},
		{rules.ExtraDamageMeleeWeapon, false, false},
		{rules.ExtraDamageRangedWeapon, true, false},
		{rules.ExtraDamageRangedWeapon, false, true},
	} {
		h := giantFacts("medium", "medium", emberTrait(func(x *link.ExtraDamage) { x.Weapon = tc.weapon }))
		h.attack.Melee = tc.melee
		p, ok := partByKey(h.extraDamageParts(), giantFeature)
		if !ok || p.Available != tc.holds || (!tc.holds && p.Reason == "") {
			t.Errorf("weapon %s, melee %v: part = %+v, want available %v (and a reason when not)", tc.weapon, tc.melee, p, tc.holds)
		}
	}
}

func TestExtraDamageOnEveryHitIsNotRememberedByTheTurn(t *testing.T) {
	t.Parallel()
	h := giantFacts("medium", "medium", emberTrait(func(x *link.ExtraDamage) {
		x.Frequency = rules.ExtraDamageEachHit
		x.Weapon = rules.ExtraDamageAnyWeapon
	}))
	h.attacker.FeatureState = turnMarkState(giantFeature, turnKey(h.enc))
	p, _ := partByKey(h.extraDamageParts(), giantFeature)
	if p.Once || !p.Available {
		t.Errorf("part = %+v, want it available again this turn and not marked once", p)
	}
}

// The dice of the extra and of the weapon are separate lines of their own types: resistance to one type
// takes only that line (the steps work by damage type).
func TestExtraDamageDiceAddAComponentOfTheirOwnType(t *testing.T) {
	t.Parallel()
	s := &Service{}
	extra := partRecord{Key: giantFeature, Kind: partExtra, Choosable: true, Selected: true, Count: 1, Sides: 6, DamageType: "damage-type:fire", Table: tableExtraDamage}
	weapon := partRecord{Key: partWeapon, Kind: partWeapon, Count: 1, Sides: 8, Flat: 2, DamageType: "damage-type:slashing", Available: true}
	for _, tc := range []struct {
		critical          bool
		slashing, fire    int
		fireDice, fireSum int32
	}{
		{false, 5 + 2, 4, 1, 4},
		{true, 5 + 4 + 2, 6 + 3, 2, 9},
	} {
		s.roller = &scriptedRoller{faces: map[bool][]int{false: {5, 4}, true: {5, 4, 6, 3}}[tc.critical]}
		got, err := s.rollParts(rollPartsInput{
			p: playdb.PendingDamage{Critical: tc.critical}, parts: []partRecord{weapon, extra}, rule: combat.CriticalDoubledDice,
			choice: extraChoice{chosen: true, marks: map[string]*playv1.SelectedExtra{giantFeature: {Key: giantFeature}}}, in: rollInput{inApp: true},
			selectOK: func(partRecord, *playv1.SelectedExtra) error { return nil },
		})
		if err != nil {
			t.Fatalf("critical %v: rollParts() error = %v", tc.critical, err)
		}
		if got.byType["damage-type:slashing"] != tc.slashing || got.byType["damage-type:fire"] != tc.fire || got.total != tc.slashing+tc.fire {
			t.Errorf("critical %v: byType = %v total %d, want slashing %d and fire %d", tc.critical, got.byType, got.total, tc.slashing, tc.fire)
		}
		if r := got.rolls[1]; r.Count != tc.fireDice || r.Sum != tc.fireSum || r.DamageType != "damage-type:fire" || !r.Counted {
			t.Errorf("critical %v: the extra rolled %+v, want %d dice summing %d of fire", tc.critical, r, tc.fireDice, tc.fireSum)
		}
	}
}

// ---- end to end (these need the database, MEURPG_TEST_DATABASE_URL) ----

type giantTable struct {
	*armed
}

// newGiantTable makes a table subclass of the Fighter whose level 3 feature adds 3 damage to a weapon hit
// once per turn, against `applies`, spending a use of a pool of two, and Pensantus, Fighter 5, who has it.
func newGiantTable(t *testing.T, applies string) *giantTable {
	t.Helper()
	return newGiantTableWith(t, &rulesv1.TableEffect{Type: "extra_damage", Applies: applies, Value: "3", Resource: giantUses, TextPt: "Só contra maiores."})
}

// newGiantTableWith is newGiantTable with the extra_damage effect the test gives.
func newGiantTableWith(t *testing.T, extra *rulesv1.TableEffect, more ...*rulesv1.TableEffect) *giantTable {
	t.Helper()
	g := &giantTable{}
	g.armed = newArmedWith(t, func(a *armed) {
		feature := &rulesv1.TableFeature{
			NamePt: "Golpe de Gigante", DescPt: []string{"Dano extra."},
			Effects: []*rulesv1.TableEffect{
				{Type: "resource", Resource: giantUses, Max: "2", Recharge: "short_rest"},
				extra,
			},
		}
		feature.Effects = append(feature.Effects, more...)
		sub := &rulesv1.TableSubclass{
			NamePt: "Caçador de Gigantes", ClassKey: "class:fighter", DescPt: []string{"Golpes."},
			Levels: []*rulesv1.TableSubclassLevel{{Level: 3, Features: []*rulesv1.TableFeature{feature}}},
		}
		res, err := a.master.table.CreateTableEntry(t.Context(), connect.NewRequest(&rulesv1.CreateTableEntryRequest{
			CampaignId: a.campaignID, Body: &rulesv1.CreateTableEntryRequest_TableSubclass{TableSubclass: sub},
		}))
		if err != nil {
			t.Fatalf("CreateTableEntry() error = %v", err)
		}
		sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
			BaseScores: &rulesv1.AbilityScores{Strength: 14, Dexterity: 13, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, RaceKey: "race:human",
			Background: &charactersv1.FullSheet_BackgroundKey{BackgroundKey: "background:acolyte"},
			Languages:  []string{"Anão", "Élfico", "Gnômico"},
			Classes: []*charactersv1.ClassLevel{{
				ClassKey: "class:fighter", Level: 5, Subclass: &charactersv1.ClassLevel_SubclassKey{SubclassKey: res.Msg.GetEntry().GetKey()},
			}},
			SkillProficiencyKeys: []string{"skill:athletics", "skill:perception"},
			FeatureChoiceKeys:    []string{"feature:fighter-fighting-style-defense"},
			WeaponKeys:           []string{longsword},
			HitPoints:            &charactersv1.HitPoints{Method: charactersv1.HitPointsMethod_HIT_POINTS_METHOD_AVERAGE}, ExperiencePoints: 6500,
		}}}
		made, err := a.ana.characters.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
			CampaignId: a.campaignID, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Pensantus", Sheet: sheet,
		}))
		if err != nil {
			t.Fatalf("CreateCharacter(Pensantus) error = %v", err)
		}
		a.pens = made.Msg.GetCharacter()
		if issues := a.pens.GetDerived().GetIssues(); len(issues) != 0 {
			t.Fatalf("the sheet has issues: %v", issues)
		}
		scores := &rulesv1.AbilityScores{Strength: 14, Dexterity: 13, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 2, scores, []string{battleaxe}, nil)
		a.bri = a.bia.hero(t, a.campaignID, "Brisa", "class:fighter", "race:human", 2, scores, []string{rapier}, nil)
	})
	return g
}

func (g *giantTable) hit(t *testing.T, e *playv1.Encounter) *playv1.PendingDamage {
	t.Helper()
	res := g.mustAttack(t, g.ana, e, "Pensantus", longsword, "Capitão Goblin", d20(19))
	if res.GetPendingDamage() == nil {
		t.Fatalf("the attack = %v, want a hit", res.GetRoll())
	}
	return res.GetPendingDamage()
}

func (g *giantTable) usesLeft(t *testing.T) int32 {
	t.Helper()
	left, _ := poolOf(g.vitals(t, g.pens), giantUses)
	return left
}

// Marking the extra adds its fixed damage once, spends a use, and the same turn offers it off.
func TestExtraDamageIsAddedOncePerTurnAndSpendsAUse(t *testing.T) {
	t.Parallel()
	g := newGiantTable(t, rules.ExtraDamageAnyTarget)
	e := g.passTo(t, g.castersFight(t, 1), "Pensantus")
	first := g.hit(t, e)
	p := partOf(first.GetParts(), giantFeatureKey(t, g))
	if p == nil || !p.GetChoosable() || !p.GetAvailable() || p.GetFlat() != 3 || p.GetDiceCount() != 0 || p.GetManeuver() || p.GetResourceLeft() != 2 {
		t.Fatalf("the offered extra = %v, want a choosable flat 3 with 2 uses, not a maneuver", p)
	}
	g.h.roller.queue(5) // the weapon die alone: the extra rolls nothing
	dmg := g.mustDamage(t, g.ana, e, first.GetId(), marking(p.GetKey())).GetPendingDamage()
	var extra *playv1.DamagePartRoll
	for _, r := range dmg.GetPartRolls() {
		if r.GetPartKey() == p.GetKey() {
			extra = r
		}
	}
	if extra == nil || !extra.GetCounted() || extra.GetFlat() != 3 {
		t.Fatalf("the part rolls = %v, want the extra counted as a flat 3", dmg.GetPartRolls())
	}
	if got := g.usesLeft(t); got != 1 {
		t.Errorf("uses left = %d, want 1", got)
	}
	second := g.hit(t, e) // the second attack of Extra Attack, the same turn
	if q := partOf(second.GetParts(), p.GetKey()); q == nil || q.GetAvailable() || q.GetReasonPt() == "" {
		t.Fatalf("the extra the same turn = %v, want it listed and off with a reason", q)
	}
	if _, err := g.damage(t, g.ana, e, second.GetId(), marking(p.GetKey())); err == nil {
		t.Error("the extra twice in one turn is accepted")
	}
	if got := g.usesLeft(t); got != 1 {
		t.Errorf("uses left after the refusal = %d, want 1", got)
	}
}

// The master's undo gives the use and the turn's mark back.
func TestExtraDamageUndoGivesTheUseAndTheMarkBack(t *testing.T) {
	t.Parallel()
	g := newGiantTable(t, rules.ExtraDamageAnyTarget)
	e := g.passTo(t, g.castersFight(t, 1), "Pensantus")
	first := g.hit(t, e)
	key := giantFeatureKey(t, g)
	g.h.roller.queue(5)
	g.mustDamage(t, g.ana, e, first.GetId(), marking(key))
	if got := g.usesLeft(t); got != 1 {
		t.Fatalf("uses left = %d, want 1", got)
	}
	g.undoLast(t, e) // the damage roll
	if got := g.usesLeft(t); got != 2 {
		t.Errorf("uses left after the undo = %d, want 2", got)
	}
	again := partOf(first.GetParts(), key)
	if again == nil || !again.GetAvailable() {
		t.Fatalf("the extra after the undo = %v, want it available again", again)
	}
	g.h.roller.queue(5)
	if _, err := g.damage(t, g.ana, e, first.GetId(), marking(key)); err != nil {
		t.Errorf("rolling the damage again with the extra: %v", err)
	}
}

// Against a creature that is not larger the extra is listed off, and refused if marked.
func TestExtraDamageOnALargerTargetNeedsALargerTarget(t *testing.T) {
	t.Parallel()
	g := newGiantTable(t, rules.ExtraDamageLargerTarget)
	e := g.passTo(t, g.castersFight(t, 1), "Pensantus")
	pending := g.hit(t, e) // the Capitão Goblin is not larger than a human
	key := giantFeatureKey(t, g)
	if p := partOf(pending.GetParts(), key); p == nil || p.GetAvailable() || p.GetReasonPt() == "" {
		t.Fatalf("the extra on a creature that is not larger = %v, want it listed and off with a reason", p)
	}
	if _, err := g.damage(t, g.ana, e, pending.GetId(), marking(key)); err == nil {
		t.Error("the extra on a creature that is not larger is accepted")
	}
	if got := g.usesLeft(t); got != 2 {
		t.Errorf("uses left = %d, want 2", got)
	}
}

// giantFeatureKey is the key the table gave the feature: the extra's part is named by it.
func giantFeatureKey(t *testing.T, g *giantTable) string {
	t.Helper()
	for _, f := range g.pens.GetDerived().GetFeatures() {
		if f.GetName() == "Golpe de Gigante" {
			return f.GetKey()
		}
	}
	t.Fatalf("Pensantus has no Golpe de Gigante among %v", g.pens.GetDerived().GetFeatures())
	return ""
}

// A feature: once per turn, +1d6 fire on a melee weapon hit. The die is rolled beside the weapon's and lands as a
// line of its own type.
func TestExtraDamageDiceAreRolledAsALineOfTheirOwnType(t *testing.T) {
	t.Parallel()
	g := newGiantTableWith(t, &rulesv1.TableEffect{Type: "extra_damage", Applies: rules.ExtraDamageAnyTarget, Dice: "1", DiceSides: 6, DamageType: "damage-type:fire", Weapon: rules.ExtraDamageMeleeWeapon})
	e := g.passTo(t, g.castersFight(t, 1), "Pensantus")
	key := giantFeatureKey(t, g)
	plain := g.hit(t, e)
	p := partOf(plain.GetParts(), key)
	if p == nil || !p.GetAvailable() || p.GetDiceCount() != 1 || p.GetDiceSides() != 6 || p.GetFlat() != 0 || p.GetDamageTypeKey() != "damage-type:fire" || p.GetDoubled() {
		t.Fatalf("the offered extra = %v, want a choosable 1d6 of fire", p)
	}
	g.h.roller.queue(5, 4) // the longsword die, then the extra die
	dmg := g.mustDamage(t, g.ana, e, plain.GetId(), marking(key)).GetPendingDamage()
	fire := partRollOf(dmg.GetPartRolls(), key)
	if fire == nil || !fire.GetCounted() || fire.GetDiceCount() != 1 || fire.GetSum() != 4 || fire.GetDamageTypeKey() != "damage-type:fire" {
		t.Fatalf("the extra rolled %v, want one die of fire summing 4", dmg.GetPartRolls())
	}
	if weapon := partRollOf(dmg.GetPartRolls(), partWeapon); weapon == nil || weapon.GetDamageTypeKey() != "damage-type:slashing" {
		t.Errorf("the weapon rolled %v, want its own type apart from the fire", dmg.GetPartRolls())
	}
}

func TestExtraDamageDiceAreDoubledByACriticalHitUnlessTheFeatureSaysNot(t *testing.T) {
	t.Parallel()
	for _, keep := range []bool{false, true} {
		g := newGiantTableWith(t, &rulesv1.TableEffect{
			Type: "extra_damage", Applies: rules.ExtraDamageAnyTarget, Dice: "1", DiceSides: 6, DamageType: "damage-type:fire", NoCritDoubling: keep,
		})
		e := g.passTo(t, g.castersFight(t, 1), "Pensantus")
		key := giantFeatureKey(t, g)
		res := g.mustAttack(t, g.ana, e, "Pensantus", longsword, "Capitão Goblin", d20(20))
		pending := res.GetPendingDamage()
		if pending == nil || !pending.GetCritical() {
			t.Fatalf("keep %v: the attack = %v, want a critical hit", keep, res.GetRoll())
		}
		want := int32(2)
		if keep {
			want = 1
		}
		if p := partOf(pending.GetParts(), key); p == nil || p.GetDiceCount() != want || p.GetDoubled() == keep {
			t.Fatalf("keep %v: the offered extra = %v, want %d dice", keep, p, want)
		}
		g.h.roller.queue(3, 3, 6, 2, 2) // the weapon's dice, then the extra's
		dmg := g.mustDamage(t, g.ana, e, pending.GetId(), marking(key)).GetPendingDamage()
		if fire := partRollOf(dmg.GetPartRolls(), key); fire == nil || fire.GetDiceCount() != want {
			t.Errorf("keep %v: the extra rolled %v, want %d dice", keep, dmg.GetPartRolls(), want)
		}
	}
}

// A ranged-only extra is listed off, with its reason, on a melee hit and refused if it is marked.
func TestExtraDamageForRangedWeaponsIsOffOnAMeleeHit(t *testing.T) {
	t.Parallel()
	g := newGiantTableWith(t, &rulesv1.TableEffect{Type: "extra_damage", Applies: rules.ExtraDamageAnyTarget, Dice: "1", DiceSides: 6, Weapon: rules.ExtraDamageRangedWeapon})
	e := g.passTo(t, g.castersFight(t, 1), "Pensantus")
	key := giantFeatureKey(t, g)
	pending := g.hit(t, e)
	if p := partOf(pending.GetParts(), key); p == nil || p.GetAvailable() || p.GetReasonPt() == "" {
		t.Fatalf("the extra on a melee hit = %v, want it listed and off with a reason", p)
	}
	if _, err := g.damage(t, g.ana, e, pending.GetId(), marking(key)); err == nil {
		t.Error("the ranged-only extra on a melee hit is accepted")
	}
}

// With "each hit" the extra is added to every hit of the turn.
func TestExtraDamageOnEveryHitIsAddedToTheSecondAttackToo(t *testing.T) {
	t.Parallel()
	g := newGiantTableWith(t, &rulesv1.TableEffect{Type: "extra_damage", Applies: rules.ExtraDamageAnyTarget, Dice: "1", DiceSides: 4, Frequency: rules.ExtraDamageEachHit})
	e := g.passTo(t, g.castersFight(t, 1), "Pensantus")
	key := giantFeatureKey(t, g)
	for range 2 {
		pending := g.hit(t, e)
		if p := partOf(pending.GetParts(), key); p == nil || !p.GetAvailable() {
			t.Fatalf("the extra = %v, want it available on each hit of the turn", p)
		}
		g.mustDamage(t, g.ana, e, pending.GetId(), marking(key))
	}
}

func partRollOf(rolls []*playv1.DamagePartRoll, key string) *playv1.DamagePartRoll {
	for _, r := range rolls {
		if r.GetPartKey() == key {
			return r
		}
	}
	return nil
}

func TestExtraDamageWithALineIsOfferedOnThatLinesHitOnly(t *testing.T) {
	t.Parallel()
	const launcher = "pseudo:" + giantFeature
	for _, tc := range []struct {
		name, line string
		attack     link.Attack
		holds      bool
	}{
		{"the named line", launcher, link.Attack{Key: launcher, Weapon: true}, true},
		{"another line", launcher, link.Attack{Key: "equipment:rapier", Weapon: true, Melee: true}, false},
		{"the pact weapon", rules.LinePact, link.Attack{Key: "equipment:rapier", Weapon: true, Melee: true, Pact: true}, true},
		{"a weapon that is not the pact weapon", rules.LinePact, link.Attack{Key: "equipment:dagger", Weapon: true, Melee: true}, false},
		{"a weapon by key", "equipment:rapier", link.Attack{Key: "equipment:rapier", Weapon: true, Melee: true}, true},
		{"no line goes with every line", "", link.Attack{Key: "equipment:dagger", Weapon: true, Melee: true}, true},
	} {
		h := giantFacts("medium", "medium", emberTrait(func(x *link.ExtraDamage) { x.Weapon, x.Line = rules.ExtraDamageAnyWeapon, tc.line }))
		h.attack = tc.attack
		p, ok := partByKey(h.extraDamageParts(), giantFeature)
		if !ok || p.Available != tc.holds || (!tc.holds && p.Reason == "") {
			t.Errorf("%s: part = %+v, want available %v (and a reason when not)", tc.name, p, tc.holds)
		}
	}
}
