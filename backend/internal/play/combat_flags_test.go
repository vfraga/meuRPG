package play

import (
	"slices"
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The flag effect in a combat (#572): never_surprised and ignore_hostile_nearby. These tests need the database. Every
// name is invented. The control of each is the same fight with the other flag, so the effect is the only difference.

func flagEffect(flag string) []*rulesv1.TableEffect {
	return []*rulesv1.TableEffect{{Type: "flag", Flag: flag}}
}

// flagRig is the party of the surprise tests, with a fighter, Toren, whose table subclass has the effects.
type flagRig struct {
	a     *armed
	entry *rulesv1.TableEntry
	// wizardEntry is the same subclass for a wizard: Pensantus has it.
	wizardEntry *rulesv1.TableEntry
}

func newFlagRig(t *testing.T, effects []*rulesv1.TableEffect) *flagRig {
	t.Helper()
	r := &flagRig{}
	r.a = newArmedWith(t, func(a *armed) {
		create := func(class string) *rulesv1.TableEntry {
			res, err := a.master.table.CreateTableEntry(t.Context(), connect.NewRequest(&rulesv1.CreateTableEntryRequest{
				CampaignId: a.campaignID, Body: &rulesv1.CreateTableEntryRequest_TableSubclass{TableSubclass: flagSubclass(class, effects, "")},
			}))
			if err != nil {
				t.Fatalf("CreateTableEntry() error = %v", err)
			}
			return res.Msg.GetEntry()
		}
		r.entry, r.wizardEntry = create("class:fighter"), create("class:wizard")
		a.toren = a.caio.heroWith(t, a.campaignID, "Toren", classLevel("class:fighter", 5, r.entry.GetKey()),
			abilities(16, 13, 14, 10, 10), []string{longsword}, nil, []string{"feature:fighter-fighting-style-defense"})
		a.pens = a.ana.heroWith(t, a.campaignID, "Pensantus", classLevel("class:wizard", 3, r.wizardEntry.GetKey()),
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8}, nil, []string{fireBolt}, nil)
		a.bri = a.bia.hero(t, a.campaignID, "Brisa", "class:fighter", "race:human", 2, scores16(), []string{rapier}, nil)
	})
	return r
}

// flagSubclass is a subclass of the class whose one feature has the effects; featureKey is the feature's key when the
// subclass is an update of one that exists.
func flagSubclass(class string, effects []*rulesv1.TableEffect, featureKey string) *rulesv1.TableSubclass {
	return &rulesv1.TableSubclass{
		NamePt: "Sentinela de Teste de " + class[len("class:"):], ClassKey: class, DescPt: []string{"Um posto de vigia."},
		Levels: []*rulesv1.TableSubclassLevel{{Level: 3, Features: []*rulesv1.TableFeature{{Key: featureKey, NamePt: "Sinal de Posto", DescPt: []string{"Fique de olho."}, Effects: effects}}}},
	}
}

// replaceFlag gives the subclass the effects instead of the ones it has, as the master editing the table does.
func (r *flagRig) replaceFlag(t *testing.T, effects []*rulesv1.TableEffect) {
	t.Helper()
	featureKey := r.entry.GetTableSubclass().GetLevels()[0].GetFeatures()[0].GetKey()
	if _, err := r.a.master.table.UpdateTableEntry(t.Context(), connect.NewRequest(&rulesv1.UpdateTableEntryRequest{
		CampaignId: r.a.campaignID, Key: r.entry.GetKey(), ExpectedRevision: r.entry.GetRevision(),
		Body: &rulesv1.UpdateTableEntryRequest_TableSubclass{TableSubclass: flagSubclass("class:fighter", effects, featureKey)},
	})); err != nil {
		t.Fatalf("UpdateTableEntry() error = %v", err)
	}
}

func TestAMasterMarksACharacterSurprisedWhenItsFlagIsAnotherOne(t *testing.T) {
	t.Parallel()
	r := newFlagRig(t, flagEffect(rules.FlagIgnoreHostileNearby))
	c := r.a.arena(t, arenaPlan{setup: true})
	if view := c.mustSetSurprised(t, "Toren", true); !slices.Equal(view.GetCombatantIds(), []string{c.id(t, "Toren")}) {
		t.Errorf("the master reads %v surprised, want Toren: only never_surprised protects", view.GetCombatantIds())
	}
}

func TestAMasterCannotMarkACharacterSurprisedThatAFeatureSaysIsNeverSurprised(t *testing.T) {
	t.Parallel()
	r := newFlagRig(t, flagEffect(rules.FlagNeverSurprised))
	a := r.a
	c := a.arena(t, arenaPlan{setup: true})
	if _, err := c.setSurprised(t, a.master, "Toren", true); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("marking Toren surprised: error = %v, want failed_precondition", err)
	}
	if got := c.surprisedAs(t, a.master); len(got) != 0 {
		t.Errorf("the master reads %v surprised after the refusal, want nobody", got)
	}
	// The flag is Toren's: Brisa is marked as always, and taking a mark off is never refused.
	if view := c.mustSetSurprised(t, "Brisa", true); !slices.Equal(view.GetCombatantIds(), []string{c.id(t, "Brisa")}) {
		t.Errorf("the master reads %v surprised, want Brisa alone", view.GetCombatantIds())
	}
	c.mustSetSurprised(t, "Toren", false)

	c.e = a.begin(t, c.e)
	if opts := a.mustOptions(t, a.caio, c.e, "Toren"); !opts.GetYourTurn() || opts.GetContestState().GetSurprised() {
		t.Errorf("Toren's options = %v, want his turn and no surprise", opts)
	}
	a.mustAttack(t, a.caio, c.e, "Toren", longsword, c.hob, d20(15))
}

func TestAMarkMadeBeforeTheNeverSurprisedFlagCameCountsForNothing(t *testing.T) {
	t.Parallel()
	r := newFlagRig(t, flagEffect(rules.FlagIgnoreHostileNearby))
	a := r.a
	c := a.arena(t, arenaPlan{setup: true})
	c.mustSetSurprised(t, "Toren", true)
	if got := c.surprisedAs(t, a.master); !slices.Equal(got, []string{c.id(t, "Toren")}) {
		t.Fatalf("the master reads %v surprised, want Toren: the mark came first", got)
	}

	r.replaceFlag(t, flagEffect(rules.FlagNeverSurprised))
	for who, u := range map[string]*user{"the master": a.master, "Toren's player": a.caio} {
		if got := c.surprisedAs(t, u); len(got) != 0 {
			t.Errorf("%s reads %v surprised once the flag came, want nobody", who, got)
		}
	}
	got, err := c.suggestions(t, a.master)
	if err != nil {
		t.Fatalf("GetSurpriseSuggestion() error = %v", err)
	}
	if s := c.suggestionFor(t, got, "Toren"); s.GetSurprised() {
		t.Errorf("the suggestion for Toren = %v, want it not surprised: the mark counts for nothing", s)
	}

	// Toren's first turn is a turn: no lockout, no gate, an attack is rolled.
	c.e = a.begin(t, c.e)
	opts := a.mustOptions(t, a.caio, c.e, "Toren")
	if !opts.GetYourTurn() || opts.GetContestState().GetSurprised() || !attackOption(opts, longsword).GetEnabled() {
		t.Fatalf("Toren's options = %v, want his turn, no surprise and the sword enabled", opts)
	}
	a.mustAttack(t, a.caio, c.e, "Toren", longsword, c.hob, d20(15))
}

func TestTheSurpriseSuggestionNeverSuggestsACharacterThatIsNeverSurprised(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		flag          string
		wantTorenHeld bool
	}{
		"another flag suggests Toren":      {rules.FlagIgnoreHostileNearby, true},
		"never_surprised does not suggest": {rules.FlagNeverSurprised, false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newFlagRig(t, flagEffect(tc.flag))
			a := r.a
			// Toren acts last, so his first turn has not come when the creatures have hidden.
			c := a.arena(t, arenaPlan{toren: 1, brisa: 15, pens: 12})
			// Both creatures of the other side hide well, so every player's passive Perception loses to them.
			for _, label := range []string{c.hob, "Goblin"} {
				c.advance(t, label)
				tried := c.mustHide(t, a.master, label, "", 20)
				c.mustResolveHide(t, tried.GetId(), nil)
			}
			got, err := c.suggestions(t, a.master)
			if err != nil {
				t.Fatalf("GetSurpriseSuggestion() error = %v", err)
			}
			toren := c.suggestionFor(t, got, "Toren")
			if toren.GetSuggested() != tc.wantTorenHeld || toren.GetReason() != playv1.SurpriseSuggestionReason_SURPRISE_SUGGESTION_REASON_HIDERS_BEAT {
				t.Errorf("Toren suggested = %v, want %v with the reason the numbers gave (hiders beat): %v", toren.GetSuggested(), tc.wantTorenHeld, toren)
			}
			if brisa := c.suggestionFor(t, got, "Brisa"); !brisa.GetSuggested() {
				t.Errorf("Brisa suggested = false, want true: she has no flag: %v", brisa)
			}
		})
	}
}

// hostileRig is a fight where Toren, with a ranged line of his own (the ray of the weapon tests) and the flag, stands next
// to the goblin.
func hostileRig(t *testing.T, flag string) *stanceRig {
	t.Helper()
	return newStanceRigOf(t, append(weaponEffects(), flagEffect(flag)...), abilities(10, 13, 14, 10, 16))
}

func hostileSource(tg *playv1.TargetInReach) bool {
	return hasSource(tg, playv1.AdvantageSourceKind_ADVANTAGE_SOURCE_KIND_HOSTILE_NEARBY)
}

func TestARangedWeaponAttackNextToAHostileCreatureHasDisadvantageWithoutTheFlag(t *testing.T) {
	t.Parallel()
	r := hostileRig(t, rules.FlagNeverSurprised)
	line := attackLine(t, r.derived(t), rayName)
	tg := targetOf(r.a.mustOptions(t, r.a.caio, r.a.get(t, r.a.master), "Toren"), line.GetKey(), "Goblin")
	if tg == nil || tg.GetRollMode() != playv1.RollMode_ROLL_MODE_DISADVANTAGE || !hostileSource(tg) {
		t.Fatalf("the ray's target = %v, want disadvantage with the hostile-nearby source", tg)
	}
}

func TestAnIgnoreHostileNearbyFlagLiftsThatDisadvantageFromARangedWeaponAttackAndItsSources(t *testing.T) {
	t.Parallel()
	r := hostileRig(t, rules.FlagIgnoreHostileNearby)
	line := attackLine(t, r.derived(t), rayName)
	e := r.a.get(t, r.a.master)
	tg := targetOf(r.a.mustOptions(t, r.a.caio, e, "Toren"), line.GetKey(), "Goblin")
	if tg == nil || tg.GetRollMode() != playv1.RollMode_ROLL_MODE_NORMAL || hostileSource(tg) {
		t.Fatalf("the ray's target = %v, want a normal roll and no hostile-nearby source", tg)
	}
	// One d20 face rolls it: the mode is normal.
	res := r.a.mustAttack(t, r.a.caio, e, "Toren", line.GetKey(), "Goblin", d20(5))
	if roll := res.GetRoll(); roll.GetD20().GetTotal() != 5+line.GetAttackBonus() {
		t.Errorf("the attack roll = %v, want one d20 of 5 plus the sheet's bonus", roll)
	}
}

func TestAnIgnoreHostileNearbyFlagKeepsTheDisadvantageOfASpellAttack(t *testing.T) {
	t.Parallel()
	r := newFlagRig(t, flagEffect(rules.FlagIgnoreHostileNearby))
	a := r.a
	// Pensantus acts first and stands next to the Hobgoblin.
	c := a.arena(t, arenaPlan{toren: 15, brisa: 10, pens: 20, at: map[string][2]int32{"Pensantus": {3, 4}}})
	tg := targetOf(a.mustOptions(t, a.ana, c.e, "Pensantus"), fireBolt, c.hob)
	if tg == nil || tg.GetRollMode() != playv1.RollMode_ROLL_MODE_DISADVANTAGE || !hostileSource(tg) {
		t.Fatalf("the cantrip's target = %v, want disadvantage with the hostile-nearby source: the flag is for weapons", tg)
	}
}

func TestACharacterMarkedSurprisedWhoseSheetCannotBeReadIsNotImmuneAndNothingFails(t *testing.T) {
	t.Parallel()
	r := newFlagRig(t, flagEffect(rules.FlagNeverSurprised))
	a := r.a
	c := a.arena(t, arenaPlan{setup: true})
	// Brisa has no flag; the master marks her, and then her character goes back to the reserve, where the combat reads no sheet of it.
	c.mustSetSurprised(t, "Brisa", true)
	if _, err := a.h.pool.Exec(t.Context(), `UPDATE characters SET reserved = true, player_user_id = NULL WHERE id = $1`, a.bri.GetId()); err != nil {
		t.Fatalf("reserve Brisa's character: %v", err)
	}
	if got := c.surprisedAs(t, a.master); !slices.Equal(got, []string{c.id(t, "Brisa")}) {
		t.Errorf("the master reads %v surprised, want Brisa: no sheet, no flag", got)
	}
}
