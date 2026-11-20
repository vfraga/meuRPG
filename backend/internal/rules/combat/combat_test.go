package combat

import (
	"errors"
	"slices"
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

func TestResolveAttack(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                  string
		bonus, ac, face       int
		total                 int
		hit, critical, fumble bool
	}{
		{"hits on the exact AC", 6, 19, 13, 19, true, false, false},
		{"misses one short", 6, 20, 13, 19, false, false, false},
		{"natural 20 hits a high AC", 2, 30, 20, 22, true, true, false},
		{"natural 1 misses a low AC", 10, 5, 1, 11, false, false, true},
		{"19 is not a critical", 6, 10, 19, 25, true, false, false},
		{"negative bonus", -1, 10, 11, 10, true, false, false},
	}
	for _, tt := range tests {
		got := ResolveAttack(tt.bonus, tt.ac, tt.face)
		want := AttackResult{Total: tt.total, Hit: tt.hit, Critical: tt.critical, Fumble: tt.fumble}
		if got != want {
			t.Errorf("%s: ResolveAttack(%d, %d, %d) = %+v, want %+v", tt.name, tt.bonus, tt.ac, tt.face, got, want)
		}
	}
}

func TestDamageTotal(t *testing.T) {
	t.Parallel()
	axe := rules.DiceFormula{Count: 1, Sides: 8, Bonus: 3}
	tests := []struct {
		name     string
		f        rules.DiceFormula
		faces    []int
		critical bool
		want     int
		err      bool
	}{
		{"Toren's 9 from the canonical fight", axe, []int{6}, false, 9, false},
		{"critical doubles the dice, not the bonus", axe, []int{6, 8}, true, 17, false},
		{"Goblin 3's critical: 2d6 (5, 4) + 2", rules.DiceFormula{Count: 1, Sides: 6, Bonus: 2}, []int{5, 4}, true, 11, false},
		{"Fire Bolt has no bonus", rules.DiceFormula{Count: 1, Sides: 10}, []int{7}, false, 7, false},
		{"a negative bonus cannot go below 0", rules.DiceFormula{Count: 1, Sides: 4, Bonus: -3}, []int{1}, false, 0, false},
		{"too few dice", axe, []int{6}, true, 0, true},
		{"too many dice", axe, []int{6, 2}, false, 0, true},
		{"a face above the die", axe, []int{9}, false, 0, true},
		{"a face of 0", axe, []int{0}, false, 0, true},
	}
	for _, tt := range tests {
		got, err := DamageTotal(tt.f, tt.faces, tt.critical)
		if (err != nil) != tt.err || got != tt.want || (err != nil && !errors.Is(err, ErrBadRoll)) {
			t.Errorf("%s: DamageTotal = %d, %v; want %d, error %v", tt.name, got, err, tt.want, tt.err)
		}
	}

	if n := DiceToRoll(rules.DiceFormula{Count: 3, Sides: 4}, true); n != 6 {
		t.Errorf("DiceToRoll critical = %d, want 6", n)
	}
	if lo, hi := DiceRange(rules.DiceFormula{Count: 2, Sides: 6, Bonus: 5}, false); lo != 2 || hi != 12 {
		t.Errorf("DiceRange(2d6+5) = %d..%d, want 2..12 (the bonus is not part of it)", lo, hi)
	}
	if lo, hi := DiceRange(axe, true); lo != 2 || hi != 16 {
		t.Errorf("DiceRange(1d8+3, critical) = %d..%d, want 2..16", lo, hi)
	}
}

// TestCriticalDice is the dice of a hit under each critical rule: the SRD's doubled
// dice, and the dice once with the maximum kept (the bonus is never part of it).
func TestCriticalDice(t *testing.T) {
	t.Parallel()
	axe := rules.DiceFormula{Count: 1, Sides: 8, Bonus: 3}
	tests := []struct {
		name         string
		f            rules.DiceFormula
		critical     bool
		rule         CriticalRule
		count, fixed int
	}{
		{"not a critical (doubled)", axe, false, CriticalDoubledDice, 1, 0},
		{"not a critical (max)", axe, false, CriticalMaxPlusRoll, 1, 0},
		{"doubled dice", axe, true, CriticalDoubledDice, 2, 0},
		{"max plus a roll: 8 kept, one die", axe, true, CriticalMaxPlusRoll, 1, 8},
		{"several dice: 2d6 keeps 12", rules.DiceFormula{Count: 2, Sides: 6, Bonus: 4}, true, CriticalMaxPlusRoll, 2, 12},
		{"eight dice, then none doubled", rules.DiceFormula{Count: 8, Sides: 6}, true, CriticalMaxPlusRoll, 8, 48},
		{"a flat damage has nothing to double or keep", rules.DiceFormula{Bonus: 5}, true, CriticalMaxPlusRoll, 0, 0},
		{"a flat damage doubled", rules.DiceFormula{Bonus: 5}, true, CriticalDoubledDice, 0, 0},
	}
	for _, tt := range tests {
		count, fixed := CriticalDice(tt.f, tt.critical, tt.rule)
		if count != tt.count || fixed != tt.fixed {
			t.Errorf("%s: CriticalDice = %d dice and %d fixed, want %d and %d", tt.name, count, fixed, tt.count, tt.fixed)
		}
	}
}

func TestApplyDamage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		hp, temp, dmg int
		want          DamageResult
	}{
		{"plain damage", 31, 0, 5, DamageResult{HP: 26}},
		{"temporary first, fully absorbed", 20, 8, 5, DamageResult{HP: 20, TempHP: 3, Absorbed: 5}},
		{"temporary first, then hit points", 20, 3, 5, DamageResult{HP: 18, Absorbed: 3}},
		{"floor at 0 and falls", 8, 0, 8, DamageResult{HP: 0, FellToZero: true}},
		{"overkill reports the excess", 8, 0, 20, DamageResult{HP: 0, FellToZero: true, Excess: 12}},
		{"already at 0 does not fall again", 0, 0, 4, DamageResult{HP: 0, Excess: 4}},
		{"zero damage", 10, 2, 0, DamageResult{HP: 10, TempHP: 2}},
		{"negative damage is nothing", 10, 2, -4, DamageResult{HP: 10, TempHP: 2}},
		{"temporary absorbs the blow that would drop", 5, 5, 5, DamageResult{HP: 5, Absorbed: 5}},
	}
	for _, tt := range tests {
		if got := ApplyDamage(tt.hp, tt.temp, tt.dmg); got != tt.want {
			t.Errorf("%s: ApplyDamage(%d, %d, %d) = %+v, want %+v", tt.name, tt.hp, tt.temp, tt.dmg, got, tt.want)
		}
	}
}

func TestApplyHeal(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		hp, max, amount int
		want            HealResult
	}{
		{"heals", 17, 23, 4, HealResult{HP: 21, Healed: 4}},
		{"caps at the maximum", 17, 23, 10, HealResult{HP: 23, Healed: 6}},
		{"from 0", 0, 24, 7, HealResult{HP: 7, Healed: 7}},
		{"already full", 23, 23, 5, HealResult{HP: 23}},
		{"negative is nothing", 10, 23, -3, HealResult{HP: 10}},
		{"above the maximum is left alone", 30, 23, 5, HealResult{HP: 30}},
	}
	for _, tt := range tests {
		if got := ApplyHeal(tt.hp, tt.max, tt.amount); got != tt.want {
			t.Errorf("%s: ApplyHeal(%d, %d, %d) = %+v, want %+v", tt.name, tt.hp, tt.max, tt.amount, got, tt.want)
		}
	}
}

// The Bardic Inspiration die added to a death save moves the total across 10, never a natural 1 or a natural 20.
func TestDeathSaveWithADie(t *testing.T) {
	tests := []struct {
		name         string
		face, bonus  int
		wantSuccess  int
		wantFailures int
		revived      bool
	}{
		{"a 7 with a 4 passes", 7, 4, 1, 0, false},
		{"a 7 with a 2 fails", 7, 2, 0, 1, false},
		{"a 9 with a 1 passes", 9, 1, 1, 0, false},
		{"a natural 1 stays two failures", 1, 8, 0, 2, false},
		{"a natural 20 revives", 20, 0, 0, 0, true},
	}
	for _, tt := range tests {
		got := DeathSaveWith(tt.face, tt.bonus, 0, 0)
		if got.Successes != tt.wantSuccess || got.Failures != tt.wantFailures || (got.Outcome == DeathSaveRevived) != tt.revived {
			t.Errorf("%s: DeathSaveWith(%d, %d) = %+v", tt.name, tt.face, tt.bonus, got)
		}
	}
}

func TestDeathSave(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		face, succ, fail int
		want             DeathSaveResult
	}{
		{"10 is a success (round 4: 14)", 14, 0, 1, DeathSaveResult{Successes: 1, Failures: 1, Outcome: DeathSaveContinues}},
		{"exactly 10 succeeds", 10, 0, 0, DeathSaveResult{Successes: 1, Outcome: DeathSaveContinues}},
		{"9 fails", 9, 1, 0, DeathSaveResult{Successes: 1, Failures: 1, Outcome: DeathSaveContinues}},
		{"natural 1 is two failures", 1, 0, 0, DeathSaveResult{Failures: 2, Outcome: DeathSaveContinues}},
		{"natural 1 on one failure kills", 1, 1, 1, DeathSaveResult{Successes: 1, Failures: 3, Outcome: DeathSaveDying}},
		{"third success is stable and clears the failures", 12, 2, 1, DeathSaveResult{Successes: 3, Outcome: DeathSaveStable}},
		{"third failure is dying, for the master to confirm", 5, 1, 2, DeathSaveResult{Successes: 1, Failures: 3, Outcome: DeathSaveDying}},
		{"natural 20 brings back 1 HP and resets", 20, 1, 2, DeathSaveResult{Outcome: DeathSaveRevived, HP: 1}},
	}
	for _, tt := range tests {
		if got := DeathSave(tt.face, tt.succ, tt.fail); got != tt.want {
			t.Errorf("%s: DeathSave(%d, %d, %d) = %+v, want %+v", tt.name, tt.face, tt.succ, tt.fail, got, tt.want)
		}
	}

	if DamageWhileDown(false) != 1 || DamageWhileDown(true) != 2 {
		t.Error("damage at 0 HP is one failure, two on a critical hit")
	}
	// Brisa, round 3: one arrow at 0 HP is her first failure; a critical
	// hit later brings her to the third.
	r := AddFailures(0, 0, DamageWhileDown(false))
	if r.Failures != 1 || r.Outcome != DeathSaveContinues {
		t.Errorf("first arrow = %+v", r)
	}
	if r = AddFailures(r.Successes, r.Failures, DamageWhileDown(true)); r.Failures != 3 || r.Outcome != DeathSaveDying {
		t.Errorf("then a critical = %+v", r)
	}
	if r = AddFailures(0, 2, 2); r.Failures != 3 {
		t.Errorf("failures are capped at 3, got %+v", r)
	}
}

func TestConcentration(t *testing.T) {
	t.Parallel()
	for damage, want := range map[int]int{0: 10, 1: 10, 19: 10, 20: 10, 21: 10, 22: 11, 30: 15, 41: 20} {
		if got := ConcentrationDC(damage); got != want {
			t.Errorf("ConcentrationDC(%d) = %d, want %d", damage, got, want)
		}
	}
}

// --- TurnOptions, with the canonical fight's characters ---

func pensantusBuild() rules.Build {
	return rules.Build{
		BaseScores: map[rules.Ability]int{rules.STR: 12, rules.DEX: 16, rules.CON: 15, rules.INT: 16, rules.WIS: 13, rules.CHA: 12},
		Race:       "race:gnome", Subrace: "subrace:rock-gnome",
		Classes:                []rules.ClassLevel{{Class: "class:wizard", Subclass: "subclass:evocation", Level: 3}},
		CustomBackgroundName:   "Sábio",
		CustomBackgroundSkills: []string{"skill:arcana", "skill:history"},
		Weapons:                []string{"equipment:quarterstaff"},
		Cantrips:               []string{"spell:fire-bolt", "spell:ray-of-frost", "spell:minor-illusion"},
		SpellsKnown: []string{
			"spell:magic-missile", "spell:shield", "spell:sleep", "spell:web", "spell:misty-step", "spell:find-familiar",
		},
		SpellsPrepared: []string{"spell:magic-missile", "spell:shield", "spell:sleep", "spell:web", "spell:misty-step"},
	}
}

func derive(t *testing.T, b rules.Build) rules.Derived {
	t.Helper()
	c, err := rules.LoadSRD()
	if err != nil {
		t.Fatal(err)
	}
	return rules.Derive(b, c)
}

func spellOf(t *testing.T, o TurnOptions, key string) SpellOption {
	t.Helper()
	for _, s := range o.Spells {
		if s.Spell.Key == key {
			return s
		}
	}
	t.Fatalf("no option for %s in %+v", key, o.Spells)
	return SpellOption{}
}

func levels(s SpellOption) []int {
	var out []int
	for _, c := range s.Slots {
		out = append(out, c.Level)
	}
	return out
}

// Round 2, Pensantus's turn: 1st level 1 free of 4, 2nd level 0 free of 2.
func TestOptionsPensantus(t *testing.T) {
	t.Parallel()
	d := derive(t, pensantusBuild())
	u := Usage{SlotsUsed: [9]int{3, 2}}
	o := Options(d, TurnState{MovementUsedFt: 10}, u)

	if o.Economy.Movement != (Movement{SpeedFt: 25, UsedFt: 10, LeftFt: 15}) || !o.Economy.Action.Available || o.Economy.Reaction.Used {
		t.Errorf("economy = %+v", o.Economy)
	}

	// Web and Misty Step need a 2nd level slot: NO_SLOT with the minimum.
	for _, key := range []string{"spell:web", "spell:misty-step"} {
		s := spellOf(t, o, key)
		if s.Enabled || s.Reason == nil || *s.Reason != (Reason{Code: ReasonNoSlot, MinLevel: 2}) || len(s.Slots) != 0 {
			t.Errorf("%s = %+v, want NO_SLOT{2}", key, s)
		}
	}
	// Misty Step is a bonus action.
	if s := spellOf(t, o, "spell:misty-step"); s.Economy != rules.EconomyBonusAction {
		t.Errorf("misty step economy = %q", s.Economy)
	}
	// Magic Missile can only use the last 1st level slot.
	mm := spellOf(t, o, "spell:magic-missile")
	if !mm.Enabled || mm.Reason != nil || !slices.Equal(levels(mm), []int{1}) || mm.Slots[0].Free != 1 || mm.Slots[0].Pact {
		t.Errorf("magic missile = %+v, want enabled with 1st level (1 free)", mm)
	}
	// Shield is only cast when he is hit.
	if sh := spellOf(t, o, "spell:shield"); sh.Enabled || sh.Reason.Code != ReasonReactionOnlyWhenHit || sh.Economy != rules.EconomyReaction {
		t.Errorf("shield = %+v", sh)
	}
	// Cantrips need no slot; the damaging ones are attacks, not spells.
	if mi := spellOf(t, o, "spell:minor-illusion"); !mi.Enabled || len(mi.Slots) != 0 {
		t.Errorf("minor illusion = %+v", mi)
	}
	for _, s := range o.Spells {
		if s.Spell.Key == "spell:fire-bolt" {
			t.Error("Fire Bolt is in both attacks and spells")
		}
	}
	// Fire Bolt +6 1d10.
	var fb *AttackOption
	for i := range o.Attacks {
		if o.Attacks[i].Attack.Key == "spell:fire-bolt" {
			fb = &o.Attacks[i]
		}
	}
	if fb == nil || !fb.Enabled || fb.Attack.AttackBonus != 6 || fb.Attack.DamageDice != (rules.DiceFormula{Count: 1, Sides: 10}) {
		t.Errorf("fire bolt = %+v", fb)
	}
	if len(o.StandardActions) != 10 || !o.StandardActions[0].Enabled {
		t.Errorf("standard actions = %+v", o.StandardActions)
	}

	// After the action is spent: attacks, spells and standard actions say
	// ACTION_USED; the bonus action spell still works only if it has a slot.
	used := Options(d, TurnState{ActionUsed: true}, Usage{SlotsUsed: [9]int{3, 0}})
	if a := used.Attacks[0]; a.Enabled || a.Reason.Code != ReasonActionUsed {
		t.Errorf("attack after the action = %+v", a)
	}
	if s := spellOf(t, used, "spell:web"); s.Enabled || s.Reason.Code != ReasonActionUsed {
		t.Errorf("web after the action = %+v", s)
	}
	if s := spellOf(t, used, "spell:misty-step"); !s.Enabled || !slices.Equal(levels(s), []int{2}) {
		t.Errorf("misty step with a free 2nd level slot = %+v", s)
	}
	if s := used.StandardActions[2]; s.Enabled || s.Reason.Code != ReasonActionUsed {
		t.Errorf("Dash after the action = %+v", s)
	}
	bonus := Options(d, TurnState{BonusActionUsed: true}, Usage{})
	if s := spellOf(t, bonus, "spell:misty-step"); s.Enabled || s.Reason.Code != ReasonBonusActionUsed {
		t.Errorf("misty step after the bonus action = %+v", s)
	}

	// A spell cast with a bonus action leaves no other spell but a cantrip of 1 action.
	afterBonus := Options(d, TurnState{BonusActionUsed: true, BonusSpellCast: true}, Usage{})
	if s := spellOf(t, afterBonus, "spell:web"); s.Enabled || s.Reason.Code != ReasonBonusActionSpellLimit {
		t.Errorf("web after a bonus action spell = %+v, want the bonus action spell limit", s)
	}
	afterAction := Options(d, TurnState{ActionUsed: false, SpellCast: true}, Usage{})
	if s := spellOf(t, afterAction, "spell:misty-step"); s.Enabled || s.Reason.Code != ReasonBonusActionSpellLimit {
		t.Errorf("misty step after another spell = %+v, want the bonus action spell limit", s)
	}
	if s := spellOf(t, afterAction, "spell:web"); !s.Enabled {
		t.Errorf("web after another action spell = %+v, want it enabled (only a bonus action spell is limited)", s)
	}

	// Fresh rest: every slot is a choice, from the spell's level up.
	fresh := Options(d, TurnState{}, Usage{})
	if got := levels(spellOf(t, fresh, "spell:web")); !slices.Equal(got, []int{2}) {
		t.Errorf("web slots = %v, want [2] (a level 3 wizard has no 3rd level slots)", got)
	}
	if got := levels(spellOf(t, fresh, "spell:magic-missile")); !slices.Equal(got, []int{1, 2}) {
		t.Errorf("magic missile slots = %v, want [1 2]", got)
	}
}

func TestOptionsSpellOfTooLongCastingTime(t *testing.T) {
	t.Parallel()
	b := pensantusBuild()
	b.SpellsKnown = append(b.SpellsKnown, "spell:identify")
	b.SpellsPrepared = append(b.SpellsPrepared, "spell:identify")
	o := Options(derive(t, b), TurnState{}, Usage{})
	if s := spellOf(t, o, "spell:identify"); s.Enabled || s.Reason.Code != ReasonTooLong || s.Economy != "" {
		t.Errorf("identify (1 minute) = %+v", s)
	}
}

func TestOptionsMovement(t *testing.T) {
	t.Parallel()
	d := derive(t, pensantusBuild()) // speed 25 ft
	tests := []struct {
		turn TurnState
		want Movement
	}{
		{TurnState{}, Movement{SpeedFt: 25, LeftFt: 25}},
		{TurnState{MovementUsedFt: 10}, Movement{SpeedFt: 25, UsedFt: 10, LeftFt: 15}},
		{TurnState{Dashed: true, MovementUsedFt: 30}, Movement{SpeedFt: 50, UsedFt: 30, LeftFt: 20}},
		{TurnState{MovementUsedFt: 40}, Movement{SpeedFt: 25, UsedFt: 40}},
	}
	for _, tt := range tests {
		if got := Options(d, tt.turn, Usage{}).Economy.Movement; got != tt.want {
			t.Errorf("movement %+v = %+v, want %+v", tt.turn, got, tt.want)
		}
	}
}

// Toren: Battleaxe +5 1d8+3; Second Wind, a bonus action, 1 use per short
// rest.
func TestOptionsToren(t *testing.T) {
	t.Parallel()
	b := rules.Build{
		BaseScores: map[rules.Ability]int{rules.STR: 15, rules.DEX: 14, rules.CON: 13, rules.INT: 12, rules.WIS: 10, rules.CHA: 8},
		Race:       "race:human", Background: "background:acolyte",
		Classes: []rules.ClassLevel{{Class: "class:fighter", Level: 3}},
		Weapons: []string{"equipment:battleaxe"},
	}
	d := derive(t, b)
	o := Options(d, TurnState{}, Usage{})

	// The battleaxe, then the unarmed strike every character has.
	if len(o.Attacks) != 2 || o.Attacks[1].Attack.Key != "attack:unarmed-strike" || o.Attacks[0].Attack.AttackBonus != 5 || o.Attacks[0].Attack.DamageDice != (rules.DiceFormula{Count: 1, Sides: 8, Bonus: 3}) {
		t.Errorf("attacks = %+v", o.Attacks)
	}
	if len(o.Spells) != 0 {
		t.Errorf("a fighter has no spells: %+v", o.Spells)
	}
	var sw *ActionOption
	for i := range o.FeatureActions {
		if o.FeatureActions[i].Action.Key == "feature:second-wind" {
			sw = &o.FeatureActions[i]
		}
	}
	if sw == nil || !sw.Enabled || sw.Action.Economy != rules.EconomyBonusAction || sw.UsesLeft != 1 {
		t.Fatalf("second wind = %+v", sw)
	}

	// Spent: NO_USES, saying when it comes back.
	spent, err := SpendResource(d, Usage{}, "second_wind")
	if err != nil {
		t.Fatal(err)
	}
	o = Options(d, TurnState{}, spent)
	for _, a := range o.FeatureActions {
		if a.Action.Key == "feature:second-wind" && (a.Enabled || *a.Reason != (Reason{Code: ReasonNoUses, Recharge: rules.RechargeShortRest}) || a.UsesLeft != 0) {
			t.Errorf("second wind after use = %+v", a)
		}
	}
	// The bonus action spent wins over the uses.
	o = Options(d, TurnState{BonusActionUsed: true}, Usage{})
	for _, a := range o.FeatureActions {
		if a.Action.Key == "feature:second-wind" && (a.Enabled || a.Reason.Code != ReasonBonusActionUsed) {
			t.Errorf("second wind with the bonus action used = %+v", a)
		}
	}
}

func TestOptionsWarlockPactMagic(t *testing.T) {
	t.Parallel()
	b := rules.Build{
		BaseScores: map[rules.Ability]int{rules.STR: 10, rules.DEX: 14, rules.CON: 14, rules.INT: 10, rules.WIS: 10, rules.CHA: 16},
		Race:       "race:human", Background: "background:acolyte",
		Classes:     []rules.ClassLevel{{Class: "class:warlock", Level: 3}},
		Cantrips:    []string{"spell:eldritch-blast"},
		SpellsKnown: []string{"spell:charm-person", "spell:hold-person"},
	}
	d := derive(t, b)
	if d.PactMagic == nil || d.PactMagic.SlotLevel != 2 || d.PactMagic.Slots != 2 {
		t.Fatalf("pact magic = %+v", d.PactMagic)
	}
	o := Options(d, TurnState{}, Usage{})
	charm := spellOf(t, o, "spell:charm-person")
	if !charm.Enabled || len(charm.Slots) != 1 || charm.Slots[0] != (SlotChoice{Level: 2, Pact: true, Free: 2, Total: 2}) {
		t.Errorf("charm = %+v, want one pact slot of level 2", charm)
	}
	spent, err := SpendPactSlot(d, Usage{})
	if err != nil {
		t.Fatal(err)
	}
	spent, _ = SpendPactSlot(d, spent)
	if _, err := SpendPactSlot(d, spent); !errors.Is(err, ErrNoSlot) {
		t.Errorf("third pact slot error = %v, want ErrNoSlot", err)
	}
	if charm = spellOf(t, Options(d, TurnState{}, spent), "spell:charm-person"); charm.Enabled || charm.Reason.Code != ReasonNoSlot || charm.Reason.MinLevel != 1 {
		t.Errorf("charm without pact slots = %+v", charm)
	}
}

func TestSpendSlot(t *testing.T) {
	t.Parallel()
	d := derive(t, pensantusBuild()) // 4 first level slots, 2 second level
	u := Usage{SlotsUsed: [9]int{3, 2}}
	if SlotsFree(d, u, 1) != 1 || SlotsFree(d, u, 2) != 0 || SlotsFree(d, u, 3) != 0 || SlotsFree(d, u, 0) != 0 || SlotsFree(d, u, 10) != 0 {
		t.Errorf("free = %d %d %d", SlotsFree(d, u, 1), SlotsFree(d, u, 2), SlotsFree(d, u, 3))
	}
	after, err := SpendSlot(d, u, 1)
	if err != nil || after.SlotsUsed[0] != 4 || u.SlotsUsed[0] != 3 {
		t.Errorf("SpendSlot(1) = %+v, %v (the input must stay %+v)", after, err, u)
	}
	for _, level := range []int{2, 3, 0, 10} {
		if got, err := SpendSlot(d, u, level); !errors.Is(err, ErrNoSlot) || got.SlotsUsed != u.SlotsUsed {
			t.Errorf("SpendSlot(%d) = %+v, %v; want ErrNoSlot and no change", level, got, err)
		}
	}
	if _, err := SpendPactSlot(d, u); !errors.Is(err, ErrNoSlot) {
		t.Errorf("a wizard has no pact slots: %v", err)
	}
}

func TestSpendResource(t *testing.T) {
	t.Parallel()
	b := rules.Build{
		BaseScores: map[rules.Ability]int{rules.STR: 15, rules.DEX: 14, rules.CON: 13, rules.INT: 12, rules.WIS: 10, rules.CHA: 8},
		Race:       "race:human", Background: "background:acolyte",
		Classes: []rules.ClassLevel{{Class: "class:barbarian", Level: 3}},
	}
	d := derive(t, b) // rage: 3 uses
	u := Usage{}
	for i := 1; i <= 3; i++ {
		var err error
		if u, err = SpendResource(d, u, "rage"); err != nil {
			t.Fatalf("use %d: %v", i, err)
		}
	}
	if left, ok := ResourceLeft(d, u, "rage"); !ok || left != 0 || u.ResourcesUsed["rage"] != 3 {
		t.Errorf("after 3 uses: left %d, ok %v, used %v", left, ok, u.ResourcesUsed)
	}
	if _, err := SpendResource(d, u, "rage"); !errors.Is(err, ErrNoUses) {
		t.Errorf("fourth rage error = %v", err)
	}
	if _, err := SpendResource(d, u, "ki"); !errors.Is(err, ErrNoUses) {
		t.Errorf("a resource the character lacks: %v", err)
	}
	if _, ok := ResourceLeft(d, u, "ki"); ok {
		t.Error("a barbarian has no ki")
	}
}

// TestSpellArithmetic: the darts of Magic Missile, a save against the DC, half
// damage rounded down, and the Shield bonus.
func TestSpellArithmetic(t *testing.T) {
	t.Parallel()
	for slot, want := range map[int]int{1: 3, 2: 4, 3: 5, 9: 11} {
		if got := MissileDarts(slot); got != want {
			t.Errorf("MissileDarts(%d) = %d, want %d", slot, got, want)
		}
	}
	if !SaveSucceeded(14, 14) || SaveSucceeded(13, 14) {
		t.Error("a save succeeds when the total reaches the DC, and not below it")
	}
	if got := HalfDamage(7); got != 3 {
		t.Errorf("HalfDamage(7) = %d, want 3 (rounded down)", got)
	}
	if ShieldACBonus != 5 {
		t.Errorf("ShieldACBonus = %d, want 5", ShieldACBonus)
	}
}

// TestAttacksLeft: Extra Attack leaves the second attack open after the first
// one spent the action, and an action spent on something else leaves none.
func TestAttacksLeft(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		perAction int
		turn      TurnState
		left      int
		reason    string
	}{
		{"fresh turn, one attack", 1, TurnState{}, 1, ""},
		{"fresh turn, extra attack", 2, TurnState{}, 2, ""},
		{"after the first of two", 2, TurnState{ActionUsed: true, AttacksMade: 1}, 1, ""},
		{"after the second of two", 2, TurnState{ActionUsed: true, AttacksMade: 2}, 0, ReasonAttacksUsed},
		{"single attack spent", 1, TurnState{ActionUsed: true, AttacksMade: 1}, 0, ReasonActionUsed},
		{"action spent on a spell", 2, TurnState{ActionUsed: true}, 0, ReasonActionUsed},
	}
	for _, tt := range tests {
		if got := AttacksLeft(tt.perAction, tt.turn); got != tt.left {
			t.Errorf("%s: AttacksLeft = %d, want %d", tt.name, got, tt.left)
		}
		o := attackOption(tt.perAction, tt.turn)
		if o.Enabled != (tt.reason == "") || (tt.reason != "" && o.Reason.Code != tt.reason) {
			t.Errorf("%s: option = %+v, want reason %q", tt.name, o, tt.reason)
		}
	}
}

// MR-014: the spells come sorted: castable now first, then the
// rest, each group by circle (cantrips first) and Portuguese name.
func TestMR014_SpellsSortByAvailabilityThenCircle(t *testing.T) {
	t.Parallel()
	d := derive(t, pensantusBuild())
	keys := func(o TurnOptions) []string {
		var out []string
		for _, s := range o.Spells {
			out = append(out, s.Spell.Key)
		}
		return out
	}

	// Round 2, 1 slot of the 1st circle left: the cantrip and
	// Magic Missile and Sleep are castable; Shield, Web and Misty Step are not.
	one := Options(d, TurnState{}, Usage{SlotsUsed: [9]int{3, 2}})
	want := []string{"spell:minor-illusion", "spell:magic-missile", "spell:sleep", "spell:shield", "spell:misty-step", "spell:web"}
	if got := keys(one); !slices.Equal(got, want) {
		t.Errorf("1 slot: %v, want %v", got, want)
	}

	// Round 4, 0 slots of the 1st circle: only the cantrip is
	// castable. Then the rest, by circle and name: the 1st (Escudo Arcano,
	// Mísseis Mágicos, Sono), the 2nd (Passo Nebuloso, Teia).
	none := Options(d, TurnState{}, Usage{SlotsUsed: [9]int{4, 2}})
	want = []string{"spell:minor-illusion", "spell:shield", "spell:magic-missile", "spell:sleep", "spell:misty-step", "spell:web"}
	if got := keys(none); !slices.Equal(got, want) {
		t.Errorf("0 slots: %v, want %v", got, want)
	}
	for i, s := range none.Spells {
		if wantEnabled := i == 0; s.Enabled != wantEnabled {
			t.Errorf("0 slots: %s enabled = %v, want %v", s.Spell.Key, s.Enabled, wantEnabled)
		}
	}
	// Shield is never castable on the character's own turn: it sorts with the
	// rest even with every slot free, and says why.
	fresh := Options(d, TurnState{}, Usage{})
	sh := spellOf(t, fresh, "spell:shield")
	if sh.Enabled || sh.Reason.Code != ReasonReactionOnlyWhenHit {
		t.Errorf("shield = %+v", sh)
	}
	if got := keys(fresh); got[len(got)-1] != "spell:shield" {
		t.Errorf("with every slot free Shield should sort last, got %v", got)
	}
}

func TestSortSpellsOrderAndTies(t *testing.T) {
	t.Parallel()
	mk := func(key, pt string, level int, enabled bool) SpellOption {
		return SpellOption{Enabled: enabled, Spell: rules.SpellEntry{Key: key, NamePT: pt, Level: level}}
	}
	got := []SpellOption{
		mk("c", "Zumbido", 0, true),
		mk("a", "Ânimo", 0, true), // accents do not sort after the Zs
		mk("t1", "Igual", 1, true),
		mk("t2", "Igual", 1, true), // a tie keeps the sheet's order
		mk("d", "Dádiva", 1, false),
		mk("b", "Bênção", 1, false),
		mk("e", "Chama", 2, true),
	}
	sortSpells(got)
	var keys []string
	for _, s := range got {
		keys = append(keys, s.Spell.Key)
	}
	if want := []string{"a", "c", "t1", "t2", "e", "b", "d"}; !slices.Equal(keys, want) {
		t.Errorf("order = %v, want %v", keys, want)
	}
}

// A character that becomes stable has both counts back at zero (SRD 5.1): the
// next hit while down is its first failure, not its third.
func TestStableCharacterStartsTheCountsOverAfterAHit(t *testing.T) {
	r := DeathSave(15, 2, 2)
	if r.Outcome != DeathSaveStable || r.Failures != 0 {
		t.Fatalf("third success = %+v, want stable with no failures", r)
	}
	hit := AddFailures(0, r.Failures, DamageWhileDown(false))
	if hit.Failures != 1 || hit.Outcome != DeathSaveContinues {
		t.Errorf("first hit on a stable character = %+v, want one failure and still alive", hit)
	}
}

func TestAdjustForType(t *testing.T) {
	skeleton := TypeModifiers{Vulnerable: []string{"damage-type:bludgeoning"}, Immune: []string{"damage-type:poison"}}
	fireproof := TypeModifiers{Resistant: []string{"damage-type:fire"}, Vulnerable: []string{"damage-type:fire"}}
	for _, tt := range []struct {
		name   string
		amount int
		typ    string
		m      TypeModifiers
		want   int
	}{
		{"vulnerability doubles", 6, "damage-type:bludgeoning", skeleton, 12},
		{"immunity is none", 9, "damage-type:poison", skeleton, 0},
		{"another type is untouched", 7, "damage-type:slashing", skeleton, 7},
		{"resistance halves down", 7, "damage-type:fire", TypeModifiers{Resistant: []string{"damage-type:fire"}}, 3},
		{"one is half of nothing", 1, "damage-type:fire", TypeModifiers{Resistant: []string{"damage-type:fire"}}, 0},
		{"resistant and vulnerable: halve, then double", 7, "damage-type:fire", fireproof, 6},
		{"a typeless damage is untouched", 5, "", skeleton, 5},
		{"nothing stays nothing", 0, "damage-type:bludgeoning", skeleton, 0},
		{"no modifiers", 8, "damage-type:fire", TypeModifiers{}, 8},
	} {
		if got := AdjustForType(tt.amount, tt.typ, tt.m); got != tt.want {
			t.Errorf("%s: AdjustForType(%d, %q) = %d, want %d", tt.name, tt.amount, tt.typ, got, tt.want)
		}
	}
}

// TestSpellLimited: the SRD's Casting Time rule for a spell cast with a bonus
// action: no other spell in the turn, in either order, but a cantrip of 1 action.
func TestSpellLimited(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		turn    TurnState
		economy string
		level   int
		want    bool
	}{
		{"nothing cast yet", TurnState{}, rules.EconomyAction, 1, false},
		{"an action spell after a bonus action spell", TurnState{BonusSpellCast: true}, rules.EconomyAction, 1, true},
		{"a bonus action spell after a bonus action spell", TurnState{BonusSpellCast: true}, rules.EconomyBonusAction, 1, true},
		{"a bonus action cantrip after a bonus action spell", TurnState{BonusSpellCast: true}, rules.EconomyBonusAction, 0, true},
		{"a cantrip of 1 action after a bonus action spell", TurnState{BonusSpellCast: true}, rules.EconomyAction, 0, false},
		{"a bonus action spell after an action spell", TurnState{SpellCast: true}, rules.EconomyBonusAction, 1, true},
		{"an action spell after an action spell", TurnState{SpellCast: true}, rules.EconomyAction, 1, false},
		{"a bonus action spell after a cantrip of 1 action", TurnState{}, rules.EconomyBonusAction, 1, false},
	} {
		if got := SpellLimited(tc.turn, tc.economy, tc.level); got != tc.want {
			t.Errorf("%s: SpellLimited = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// The reasons a scroll's spell could not be cast this turn are those of a spell option: the economy of its casting time, then the
// limit of a bonus action spell; a longer casting spends none of the turn.
func TestScrollReadReason(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		turn    TurnState
		economy string
		level   int
		want    string
	}{
		{name: "a free turn", economy: rules.EconomyAction, level: 3},
		{name: "the action is used", turn: TurnState{ActionUsed: true}, economy: rules.EconomyAction, level: 3, want: ReasonActionUsed},
		{name: "the bonus action is used", turn: TurnState{BonusActionUsed: true}, economy: rules.EconomyBonusAction, level: 2, want: ReasonBonusActionUsed},
		{name: "the reaction is used", turn: TurnState{ReactionUsed: true}, economy: rules.EconomyReaction, level: 1, want: ReasonReactionUsed},
		{name: "another economy used does not matter", turn: TurnState{ActionUsed: true}, economy: rules.EconomyBonusAction, level: 2},
		{name: "a bonus action spell after another spell", turn: TurnState{SpellCast: true}, economy: rules.EconomyBonusAction, level: 2, want: ReasonBonusActionSpellLimit},
		{name: "a spell after a bonus action spell", turn: TurnState{BonusSpellCast: true}, economy: rules.EconomyAction, level: 3, want: ReasonBonusActionSpellLimit},
		{name: "a cantrip of 1 action after a bonus action spell", turn: TurnState{BonusSpellCast: true}, economy: rules.EconomyAction, level: 0},
		{name: "a longer casting spends none", turn: TurnState{ActionUsed: true, BonusActionUsed: true, ReactionUsed: true}, economy: "", level: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var got string
			if r := ScrollReadReason(tt.turn, tt.economy, tt.level); r != nil {
				got = r.Code
			}
			if got != tt.want {
				t.Errorf("reason %q, want %q", got, tt.want)
			}
		})
	}
}
