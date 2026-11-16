package play

import (
	"fmt"
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// These tests freeze what the roll of a damage by parts refuses: an extra that does not
// hold when the damage is rolled (the conditions are read again), a Divine Smite with no
// slot, more than one maneuver, no use left, and typed sums that do not fit the dice.
// They need the database (MEURPG_TEST_DATABASE_URL).

var (
	blockedExtraNotAvailable = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_EXTRA_NOT_AVAILABLE
	blockedNoSlot            = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_SLOT
)

const (
	extraNotAvailableMsg = "that extra does not hold now"
	extraWeaponDamage    = "weapon"
)

// extrasMarked marks the extras of the damage roll, with the sums typed for the dice.
func extrasMarked(marks []*playv1.SelectedExtra, typed ...*playv1.TypedPart) func(*playv1.RollDamageRequest) {
	return func(r *playv1.RollDamageRequest) {
		r.ExtrasChosen = true
		r.SelectedExtras = marks
		r.TypedParts = typed
	}
}

// setCombatantColumn writes a column of a combatant's row, as a past turn would have left it.
func (a *armed) setCombatantColumn(t *testing.T, label, column, value string) {
	t.Helper()
	if _, err := a.h.pool.Exec(t.Context(), `UPDATE combatants SET `+column+` = NULLIF($2, '') WHERE id = $1`, a.id(t, label), value); err != nil {
		t.Fatalf("set %s of %s: %v", column, label, err)
	}
}

// setFeatureMark marks the feature of the combatant as used in the turn (an entry of scope turn in feature_state).
func (a *armed) setFeatureMark(t *testing.T, label, key, turn string) {
	t.Helper()
	if _, err := a.h.pool.Exec(t.Context(), `UPDATE combatants SET feature_state = $2::jsonb WHERE id = $1`, a.id(t, label), string(turnMarkState(key, turn))); err != nil {
		t.Fatalf("mark %s of %s: %v", key, label, err)
	}
}

// turnMarkState is a feature_state with one entry of scope turn.
func turnMarkState(key, turn string) []byte {
	return featureState{key: {S: scopeTurn, At: turn, N: 1}}.bytes()
}

// extrasTurnKey is the key of the turn in progress, as the once-per-turn marks keep it.
func extrasTurnKey(e *playv1.Encounter) string {
	return fmt.Sprintf("%d:%s", e.GetRound(), e.GetCurrentCombatantId())
}

// TestRollDamageRefusesASneakAttackThatDoesNotHold: the part the hit stored as not
// available, and the one that was available but whose once-per-turn mark was set before
// the roll, are both refused with the same reason.
func TestRollDamageRefusesASneakAttackThatDoesNotHold(t *testing.T) {
	t.Parallel()
	sneak := []*playv1.SelectedExtra{{Key: "sneak-attack"}}
	typed := []*playv1.TypedPart{{PartKey: extraWeaponDamage, Sum: 3}, {PartKey: "sneak-attack", Sum: 5}}
	for _, tt := range []struct {
		name      string
		markFirst bool // the mark is set before the hit (the part is stored as not available)
	}{
		{"stored as not available", true},
		{"read again at the roll", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a, e := rollsTable(t)
			e = a.passTo(t, e, "Toren")
			a.setConditions(t, e, "Goblin", "condition:restrained")
			if tt.markFirst {
				a.setFeatureMark(t, "Toren", "sneak_attack", extrasTurnKey(e))
			}
			res := a.mustAttack(t, a.caio, a.get(t, a.caio), "Toren", rapier, "Goblin", func(r *playv1.RollAttackRequest) { r.D20Faces = []int32{5, 18} })
			if !tt.markFirst {
				a.setFeatureMark(t, "Toren", "sneak_attack", extrasTurnKey(e))
			}
			_, err := a.damage(t, a.caio, e, res.GetPendingDamage().GetId(), extrasMarked(sneak, typed...))
			wantBlockedBy(t, "RollDamage(a Sneak Attack that does not hold)", err, blockedExtraNotAvailable)
			wantRefusal(t, "RollDamage(a Sneak Attack that does not hold)", err, connect.CodeFailedPrecondition, extraNotAvailableMsg)
			// Not marking it is fine: the weapon alone rolls.
			if _, err := a.damage(t, a.caio, e, res.GetPendingDamage().GetId(), extrasMarked(nil, &playv1.TypedPart{PartKey: extraWeaponDamage, Sum: 3})); err != nil {
				t.Errorf("RollDamage(the weapon alone) error = %v", err)
			}
		})
	}
}

// TestRollDamageRefusesTypedSumsThatDoNotFit: with physical dice a sum is typed for each
// group of dice, once, inside the dice's range.
func TestRollDamageRefusesTypedSumsThatDoNotFit(t *testing.T) {
	t.Parallel()
	a, e, pendingID := partEditTable(t)
	sneak := []*playv1.SelectedExtra{{Key: "sneak-attack"}}
	roll := func(typed ...*playv1.TypedPart) error {
		_, err := a.damage(t, a.caio, e, pendingID, extrasMarked(sneak, typed...))
		return err
	}
	part := func(key string, sum int32) *playv1.TypedPart { return &playv1.TypedPart{PartKey: key, Sum: sum} }
	wantRefusal(t, "RollDamage(no sum for Sneak Attack)", roll(part(extraWeaponDamage, 3)),
		connect.CodeInvalidArgument, "typed_parts needs a sum for sneak-attack")
	wantRefusal(t, "RollDamage(no sum for the weapon)", roll(part("sneak-attack", 5)),
		connect.CodeInvalidArgument, "typed_parts needs a sum for weapon")
	wantRefusal(t, "RollDamage(the weapon above its dice)", roll(part(extraWeaponDamage, 99), part("sneak-attack", 5)),
		connect.CodeInvalidArgument, "the sum of weapon must be 1 to 8")
	wantRefusal(t, "RollDamage(Sneak Attack below its dice)", roll(part(extraWeaponDamage, 3), part("sneak-attack", 1)),
		connect.CodeInvalidArgument, "the sum of sneak-attack must be 2 to 12")
	wantRefusal(t, "RollDamage(a part twice)", roll(part(extraWeaponDamage, 3), part(extraWeaponDamage, 4)),
		connect.CodeInvalidArgument, "typed_parts must name each part once")
	wantRefusal(t, "RollDamage(a part with no key)", roll(part("", 3)),
		connect.CodeInvalidArgument, "typed_parts must name a part_key or a damage_type")
	if err := roll(part(extraWeaponDamage, 3), part("sneak-attack", 5)); err != nil {
		t.Errorf("RollDamage(the sums that fit) error = %v", err)
	}
}

// TestRollDamageRefusesAColossusSlayerAlreadyUsed: the Hunter's once-per-turn mark is
// read again when the damage is rolled.
func TestRollDamageRefusesAColossusSlayerAlreadyUsed(t *testing.T) {
	t.Parallel()
	a := newArmedWith(t, func(a *armed) {
		a.toren = a.caio.heroWith(t, a.campaignID, "Toren", classLevel("class:ranger", 3, "subclass:hunter"), abilities(16, 14, 14, 10, 10), []string{rapier}, nil,
			[]string{"feature:hunters-prey-colossus-slayer"})
		a.pens = a.ana.hero(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 1,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8}, nil, []string{fireBolt})
		a.bri = a.bia.hero(t, a.campaignID, "Brisa", "class:fighter", "race:human", 2,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 16, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{rapier}, nil)
	})
	e := a.threeAndAGoblin(t)
	if _, err := a.adjustHP(t, e, "Goblin", damageHP(1)); err != nil {
		t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
	}
	res := a.mustAttack(t, a.caio, e, "Toren", rapier, "Goblin", d20(15))
	a.setFeatureMark(t, "Toren", "colossus_slayer", extrasTurnKey(e))
	_, err := a.damage(t, a.caio, e, res.GetPendingDamage().GetId(),
		extrasMarked([]*playv1.SelectedExtra{{Key: "colossus-slayer"}}, &playv1.TypedPart{PartKey: extraWeaponDamage, Sum: 1}, &playv1.TypedPart{PartKey: "colossus-slayer", Sum: 2}))
	wantBlockedBy(t, "RollDamage(a Colossus Slayer already used)", err, blockedExtraNotAvailable)
	wantRefusal(t, "RollDamage(a Colossus Slayer already used)", err, connect.CodeFailedPrecondition, extraNotAvailableMsg)
}

// TestSelectionCheckerHoldsTheExtrasThatDoNotNeedTheDatabase: the once-per-turn marks, the
// concentration of Hunter's Mark and the maneuvers of a creature that is no player's character
// are read from the attacker alone.
func TestSelectionCheckerHoldsTheExtrasThatDoNotNeedTheDatabase(t *testing.T) {
	t.Parallel()
	const turn = "2:combatant"
	s := &Service{}
	c := &combatTx{enc: playdb.Encounter{Round: 2, CurrentCombatantID: new("combatant")}}
	hunters := "spell:hunters-mark"
	other := "spell:bless"
	mark := turn
	earlier := "1:combatant"
	tests := []struct {
		name     string
		attacker playdb.Combatant
		part     partRecord
		holds    bool
	}{
		{"a part that is not available", playdb.Combatant{}, partRecord{Key: "sneak-attack"}, false},
		{"Sneak Attack not used this turn", playdb.Combatant{}, partRecord{Key: "sneak-attack", Available: true}, true},
		{"Sneak Attack used this turn", playdb.Combatant{FeatureState: turnMarkState("sneak_attack", mark)}, partRecord{Key: "sneak-attack", Available: true}, false},
		{"Sneak Attack used in another turn", playdb.Combatant{FeatureState: turnMarkState("sneak_attack", earlier)}, partRecord{Key: "sneak-attack", Available: true}, true},
		{"Colossus Slayer not used this turn", playdb.Combatant{}, partRecord{Key: "colossus-slayer", Available: true}, true},
		{"Colossus Slayer used this turn", playdb.Combatant{FeatureState: turnMarkState("colossus_slayer", mark)}, partRecord{Key: "colossus-slayer", Available: true}, false},
		{"Hunter's Mark while concentrating on it", playdb.Combatant{ConcentrationSpell: &hunters}, partRecord{Key: "hunters-mark", Available: true}, true},
		{"Hunter's Mark with no concentration", playdb.Combatant{}, partRecord{Key: "hunters-mark", Available: true}, false},
		{"Hunter's Mark while concentrating on another spell", playdb.Combatant{ConcentrationSpell: &other}, partRecord{Key: "hunters-mark", Available: true}, false},
		{"a maneuver of a creature that is no player's", playdb.Combatant{Kind: kindNPC}, partRecord{Key: "maneuver", Resource: "dice", Available: true}, false},
		{"an extra the rules have no condition for", playdb.Combatant{}, partRecord{Key: "something-else", Available: true}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := s.selectionChecker(t.Context(), c, "campaign", tt.attacker)(tt.part, nil)
			if tt.holds {
				if err != nil {
					t.Errorf("the check = %v, want the extra to hold", err)
				}
				return
			}
			wantBlockedBy(t, "the check", err, blockedExtraNotAvailable)
			wantRefusal(t, "the check", err, connect.CodeFailedPrecondition, extraNotAvailableMsg)
		})
	}
}

// TestRollDamageRefusesADivineSmiteWithoutASlot: the smite names the slot it spends, a
// level from 1 to 9 that the paladin has free.
func TestRollDamageRefusesADivineSmiteWithoutASlot(t *testing.T) {
	t.Parallel()
	a, _ := smiteTable(t)
	res := a.mustAttack(t, a.caio, a.get(t, a.caio), "Toren", battleaxe, "Esqueleto", d20(15))
	e := a.get(t, a.caio)
	roll := func(marks ...*playv1.SelectedExtra) error {
		_, err := a.damage(t, a.caio, e, res.GetPendingDamage().GetId(), extrasMarked(marks,
			&playv1.TypedPart{PartKey: extraWeaponDamage, Sum: 1}, &playv1.TypedPart{PartKey: "divine-smite", Sum: 2}, &playv1.TypedPart{PartKey: "divine-smite-extra", Sum: 1}))
		return err
	}
	const names = "a Divine Smite names the slot it spends"
	wantRefusal(t, "RollDamage(a smite with no slot)", roll(&playv1.SelectedExtra{Key: "divine-smite"}), connect.CodeInvalidArgument, names)
	wantRefusal(t, "RollDamage(a smite with slot 10)", roll(&playv1.SelectedExtra{Key: "divine-smite", SlotLevel: 10}), connect.CodeInvalidArgument, names)
	err := roll(&playv1.SelectedExtra{Key: "divine-smite", SlotLevel: 3})
	b := wantBlockedBy(t, "RollDamage(a smite with a slot the paladin has not)", err, blockedNoSlot)
	wantRefusal(t, "RollDamage(a smite with a slot the paladin has not)", err, connect.CodeFailedPrecondition, "there is no free spell slot of that level")
	if b.GetMinLevel() != 3 {
		t.Errorf("the refusal names level %d, want the 3 that was asked", b.GetMinLevel())
	}
	err = roll(&playv1.SelectedExtra{Key: "divine-smite", SlotLevel: 1, Pact: true})
	wantBlockedBy(t, "RollDamage(a smite with a pact slot the paladin has not)", err, blockedNoSlot)
	if err := roll(&playv1.SelectedExtra{Key: "divine-smite", SlotLevel: 1}); err != nil {
		t.Errorf("RollDamage(a smite with a free slot) error = %v", err)
	}
}

// TestRollDamageRefusesTwoManeuversAndOneWithNoUseLeft: one maneuver for each attack, and
// the uses are read again when the damage is rolled.
func TestRollDamageRefusesTwoManeuversAndOneWithNoUseLeft(t *testing.T) {
	t.Parallel()
	m := newManeuverTable(t, "4")
	e := m.passTo(t, m.castersFight(t, 1), "Pensantus")
	pending := m.hit(t, e)
	_, err := m.damage(t, m.ana, e, pending.GetId(), marking(m.heron, m.bull))
	wantBlockedBy(t, "RollDamage(two maneuvers)", err, blockedExtraNotAvailable)
	wantRefusal(t, "RollDamage(two maneuvers)", err, connect.CodeFailedPrecondition, "only one maneuver for each attack")

	one := newManeuverTable(t, "1")
	e = one.passTo(t, one.castersFight(t, 1), "Pensantus")
	pending = one.hit(t, e)
	// The only die is spent between the hit and the roll (the master's correction).
	one.correct(t, one.pens, func(r *playv1.AdjustCharacterVitalsRequest) {
		r.ResourcesUsed = []*playv1.ResourceUsed{{Key: maneuverDiceKey, Used: 1}}
	})
	if got := diceLeft(t, one); got != 0 {
		t.Fatalf("dice left = %d, want 0", got)
	}
	_, err = one.damage(t, one.ana, e, pending.GetId(), marking(one.heron))
	wantBlockedBy(t, "RollDamage(a maneuver with no die left)", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_USES)
	wantRefusal(t, "RollDamage(a maneuver with no die left)", err, connect.CodeFailedPrecondition, "no uses left")
}

// partsFight is a theatre combat with a creature of the SRD beside Toren (the party's
// fighter, Brisa a second one with a rapier), Toren first.
func partsFight(t *testing.T, toren func(a *armed), monster string) (*armed, *playv1.Encounter, string) {
	t.Helper()
	a := newArmedWith(t, func(a *armed) {
		toren(a)
		a.pens = a.ana.hero(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 1, abilities(10, 14, 12, 16, 8), nil, []string{fireBolt})
		a.bri = a.bia.hero(t, a.campaignID, "Brisa", "class:fighter", "race:human", 2, abilities(10, 16, 14, 10, 8), []string{rapier}, nil)
	})
	e := a.monsterSetup(t, true)
	a.h.roller.queue(1)
	res := a.mustAddMonsters(t, e, func(r *playv1.AddMonstersRequest) {
		r.CreatureKey, r.Count, r.Hidden = monster, 1, new(false)
	})
	label := ""
	for _, c := range res.GetEncounter().GetCombatants() {
		if c.GetId() == res.GetCombatantIds()[0] {
			label = c.GetLabel()
		}
	}
	if label == "" {
		t.Fatalf("%s is not in the combat", monster)
	}
	e = a.begin(t, a.get(t, a.master))
	return a, e, label
}

// TestRollDamageRerollsTheLowFacesOfAGreatWeapon: Great Weapon Fighting rolls a die that
// came up 1 or 2 again, once, and the part shows the reroll.
func TestRollDamageRerollsTheLowFacesOfAGreatWeapon(t *testing.T) {
	t.Parallel()
	a, e, shrub := partsFight(t, func(a *armed) {
		a.toren = a.caio.heroWith(t, a.campaignID, "Toren", classLevel("class:fighter", 2, ""), abilities(16, 12, 14, 10, 8), []string{"equipment:greatsword"}, nil,
			[]string{"feature:fighter-fighting-style-great-weapon-fighting"})
	}, "monster:awakened-shrub")
	hit := a.mustAttack(t, a.caio, e, "Toren", "equipment:greatsword", shrub, d20(15))
	a.h.roller.queue(1, 5, 4) // the two dice, then the reroll of the 1
	dmg := a.mustDamage(t, a.caio, e, hit.GetPendingDamage().GetId(), inAppDamage).GetPendingDamage()
	var weapon *playv1.DamagePartRoll
	for _, r := range dmg.GetPartRolls() {
		if r.GetPartKey() == extraWeaponDamage {
			weapon = r
		}
	}
	if weapon == nil {
		t.Fatalf("the part rolls = %v, want the weapon's", dmg.GetPartRolls())
	}
	if f := weapon.GetFaces(); len(f) != 2 || f[0] != 4 || f[1] != 5 || weapon.GetSum() != 9 {
		t.Errorf("the weapon rolled %v (sum %d), want 4 and 5 (9) after the reroll of the 1", weapon.GetFaces(), weapon.GetSum())
	}
	if rr := weapon.GetRerolled(); len(rr) != 1 || rr[0].GetIndex() != 0 || rr[0].GetFrom() != 1 || rr[0].GetTo() != 4 || rr[0].GetSourceKey() != "" {
		t.Errorf("the rerolls = %v, want the die 0 from 1 to 4", rr)
	}
}

// Great Weapon Fighting rerolls a versatile weapon held in both hands (SRD 5.1, "Fighting Style"), and
// not the same weapon held in one hand beside a shield.
func TestRollDamageRerollsAVersatileWeaponOnlyInBothHands(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		shield bool
	}{{"in both hands", false}, {"beside a shield", true}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a, e, shrub := partsFight(t, func(a *armed) {
				a.toren = a.caio.versatileHero(t, a.campaignID, tc.shield)
			}, "monster:awakened-shrub")
			hit := a.mustAttack(t, a.caio, e, "Toren", "equipment:longsword", shrub, d20(15))
			a.h.roller.queue(1, 7) // the die, then the reroll of the 1 when it is rolled again
			dmg := a.mustDamage(t, a.caio, e, hit.GetPendingDamage().GetId(), inAppDamage).GetPendingDamage()
			var rerolled int
			for _, r := range dmg.GetPartRolls() {
				if r.GetPartKey() == extraWeaponDamage {
					rerolled = len(r.GetRerolled())
				}
			}
			if want := map[bool]int{false: 1, true: 0}[tc.shield]; rerolled != want {
				t.Errorf("the weapon's rerolls = %d, want %d (parts %v)", rerolled, want, dmg.GetPartRolls())
			}
		})
	}
}

// TestDuelingKeepsALoneVersatileWeaponInOneHand: with the Dueling style a lone longsword rolls the one-handed
// die with the style's +2, and Great Weapon Fighting beside Dueling never joins it on the same roll: the
// longsword is in one hand (no reroll), the greatsword is in two (the reroll, no +2).
func TestDuelingKeepsALoneVersatileWeaponInOneHand(t *testing.T) {
	t.Parallel()
	const dueling, greatWeapon = "feature:fighter-fighting-style-dueling", "feature:fighter-fighting-style-great-weapon-fighting"
	for _, tc := range []struct {
		name                string
		class               *charactersv1.ClassLevel
		weapon              string
		styles              []string
		sides, bonus, rolls int32
		rerolls             int
	}{
		{"Dueling, a lone longsword", classLevel("class:fighter", 2, ""), "equipment:longsword", []string{dueling}, 8, 5, 1, 0},
		{"Dueling and Great Weapon Fighting, a lone longsword", classLevel("class:fighter", 10, "subclass:champion"), "equipment:longsword", []string{dueling, greatWeapon}, 8, 5, 1, 0},
		{"Dueling and Great Weapon Fighting, a greatsword", classLevel("class:fighter", 10, "subclass:champion"), "equipment:greatsword", []string{dueling, greatWeapon}, 6, 3, 2, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a, e, shrub := partsFight(t, func(a *armed) {
				a.toren = a.caio.heroWith(t, a.campaignID, "Toren", tc.class, abilities(16, 12, 14, 10, 8), []string{tc.weapon}, nil, tc.styles)
			}, "monster:awakened-shrub")
			hit := a.mustAttack(t, a.caio, e, "Toren", tc.weapon, shrub, d20(15)).GetPendingDamage()
			if hit.GetDiceCount() != tc.rolls || hit.GetDiceSides() != tc.sides || hit.GetBonus() != tc.bonus {
				t.Errorf("the damage to roll = %dd%d%+d, want %dd%d%+d", hit.GetDiceCount(), hit.GetDiceSides(), hit.GetBonus(), tc.rolls, tc.sides, tc.bonus)
			}
			a.h.roller.queue(1, 5, 4) // a 1 first: Great Weapon Fighting would roll it again
			dmg := a.mustDamage(t, a.caio, e, hit.GetId(), inAppDamage).GetPendingDamage()
			var rerolled int
			for _, r := range dmg.GetPartRolls() {
				if r.GetPartKey() == extraWeaponDamage {
					rerolled = len(r.GetRerolled())
				}
			}
			if rerolled != tc.rerolls {
				t.Errorf("the weapon's rerolls = %d, want %d (parts %v)", rerolled, tc.rerolls, dmg.GetPartRolls())
			}
		})
	}
}

// versatileHero creates Toren, a fighter with Great Weapon Fighting and a longsword, maybe with a shield.
func (u *user) versatileHero(t *testing.T, campaignID string, shield bool) *charactersv1.Character {
	t.Helper()
	sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores: abilities(16, 12, 14, 10, 8), RaceKey: "race:human", Classes: []*charactersv1.ClassLevel{classLevel("class:fighter", 2, "")},
		WeaponKeys: []string{"equipment:longsword"}, Shield: shield,
		FeatureChoiceKeys: []string{"feature:fighter-fighting-style-great-weapon-fighting"},
	}}}
	res, err := u.characters.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: campaignID, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Toren", Sheet: sheet,
	}))
	if err != nil {
		t.Fatalf("CreateCharacter(Toren) error = %v", err)
	}
	return res.Msg.GetCharacter()
}

// TestMonsterResistanceHalvesTheDamageAndTheMasterSeesTheStep: a creature resistant to a
// type takes half of it, the master reads the step with the creature's key as its source,
// and the player reads the damage as rolled.
func TestMonsterResistanceHalvesTheDamageAndTheMasterSeesTheStep(t *testing.T) {
	t.Parallel()
	a, e, shrub := partsFight(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 2, abilities(16, 12, 14, 10, 8), []string{rapier}, nil)
	}, "monster:awakened-shrub")
	hit := a.mustAttack(t, a.caio, e, "Toren", rapier, shrub, d20(15))
	a.mustDamage(t, a.caio, e, hit.GetPendingDamage().GetId(), typedDamage(6))
	attackLine := func(u *user) *playv1.CombatLogDamage {
		for _, r := range a.log(t, u, e).GetRounds() {
			for _, en := range r.GetEntries() {
				if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_ATTACK {
					return en.GetDamage()
				}
			}
		}
		return nil
	}
	master := attackLine(a.master)
	if master == nil {
		t.Fatal("the master reads no attack line")
	}
	if steps := master.GetSteps(); len(steps) != 1 || steps[0].GetKind() != playv1.DamageStepKind_DAMAGE_STEP_KIND_RESISTANCE ||
		len(steps[0].GetSourceKeys()) != 1 || steps[0].GetSourceKeys()[0] != "monster:awakened-shrub" || steps[0].GetBefore() != 9 || steps[0].GetAfter() != 4 {
		t.Errorf("the master's steps = %v, want one resistance from the creature, 9 to 4", master.GetSteps())
	}
	if player := attackLine(a.caio); player == nil || len(player.GetSteps()) != 0 || player.GetAmount() != 9 {
		t.Errorf("the player reads %v, want the damage as rolled (9) and no step", player)
	}
}

// TestRageResistanceHoldsOnlyWhileRaging: a barbarian who is not raging takes the whole
// damage, and the resistance of the rage is no step of it.
func TestRageResistanceHoldsOnlyWhileRaging(t *testing.T) {
	t.Parallel()
	a, e := rollsTable(t)
	e = a.passTo(t, e, "Goblin")
	a.h.roller.queue(4)
	hit := a.mustAttack(t, a.master, e, "Goblin", sword, "Brisa", d20(18))
	dmg := a.mustDamage(t, a.master, e, hit.GetPendingDamage().GetId(), inAppDamage).GetPendingDamage()
	if len(dmg.GetSteps()) != 0 || dmg.GetAmount() != 6 || dmg.GetAmountAfterSteps() != 0 {
		t.Errorf("the damage = %v, want 6 with no step: Brisa is not raging", dmg)
	}
}
