package rules

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// The state effect: a table feature that switches something on for a while (a stance for 1
// minute, a form that lasts until a long rest) and the effects of the same entry that the state
// gates. Every name here is invented.

const (
	stateSubclass = "subclass:guardiao-de-teste" + tableSuffix
	stateFeature  = "feature:postura-firme" + tableSuffix
	stateAside    = "feature:passo-firme" + tableSuffix
	stateUses     = "posturas"
	stateName     = "postura"
)

// stateKeyOfTest is the key the state has on the sheet: its feature and its name.
func stateKeyOfTest() string { return stateFeature + "#" + stateName }

// stanceEffects is "a stance for 1 minute, bonus action, 1 use per short rest, +2 AC and +10 ft
// speed".
func stanceEffects() []Effect {
	return []Effect{
		{Type: "resource", Resource: stateUses, Max: "1", Recharge: "short_rest"},
		{Type: "state", StateName: stateName, DurationRounds: "10", EndsOn: []string{StateEndsDismiss, StateEndsIncapacitated}, Resource: stateUses, Economy: "bonus_action", TextPT: "Você assume a postura."},
		{Type: "modifier", Target: "ac", Mode: "add", Value: "2", When: `active("postura")`},
		{Type: "modifier", Target: "speed.walk", Mode: "add", Value: "10", When: `active("postura")`},
	}
}

// stateOverlay is a table subclass of the SRD Fighter whose level 3 feature carries the effects,
// and, optionally, a second feature with more effects of the same entry.
func stateOverlay(effects []Effect, aside ...Effect) Overlay {
	features := []TableFeature{tf("postura-firme", "Postura Firme", effects...)}
	if len(aside) > 0 {
		features = append(features, tf("passo-firme", "Passo Firme", aside...))
	}
	return Overlay{Revision: 1, Subclasses: []TableSubclass{{
		Key: stateSubclass, NamePT: "Guardião de Teste", Class: "class:fighter",
		Levels: []TableSubclassLevel{{Level: 3, Features: features}},
	}}}
}

func stateBuild(level int, active ...string) Build {
	return Build{
		BaseScores: map[Ability]int{STR: 15, DEX: 14, CON: 13, INT: 12, WIS: 10, CHA: 8},
		Race:       "race:human", Background: "background:acolyte", Languages: testLanguages,
		Classes:      []ClassLevel{{Class: "class:fighter", Level: level, Subclass: stateSubclass}},
		ActiveStates: active,
	}
}

func stateOf(d Derived, key string) (State, bool) {
	i := slices.IndexFunc(d.States, func(s State) bool { return s.Key == key })
	if i < 0 {
		return State{}, false
	}
	return d.States[i], true
}

func TestTheStateGatesTheEffectsOfItsEntry(t *testing.T) {
	c := withOverlay(t, stateOverlay(stanceEffects()))
	off := Derive(stateBuild(3), c)
	on := Derive(stateBuild(3, stateKeyOfTest()), c)

	if on.ArmorClass != off.ArmorClass+2 {
		t.Errorf("armor class = %d with the state, %d without: want +2", on.ArmorClass, off.ArmorClass)
	}
	if on.SpeedWalkFt != off.SpeedWalkFt+10 {
		t.Errorf("speed = %d with the state, %d without: want +10", on.SpeedWalkFt, off.SpeedWalkFt)
	}
	// A key the content does not know changes nothing.
	stale := Derive(stateBuild(3, "feature:nao-existe"+tableSuffix+"#postura", stateFeature+"#outra"), c)
	if stale.ArmorClass != off.ArmorClass || len(stale.States) != len(off.States) {
		t.Errorf("an unknown active state changed the sheet: AC %d, states %+v", stale.ArmorClass, stale.States)
	}
}

func TestTheStateIsListedWithItsDurationAndHowItEnds(t *testing.T) {
	c := withOverlay(t, stateOverlay(stanceEffects()))
	s, ok := stateOf(Derive(stateBuild(3), c), stateKeyOfTest())
	if !ok {
		t.Fatal("the state is not on the sheet")
	}
	want := State{
		Key: stateKeyOfTest(), Name: stateName, NamePT: "Postura Firme", Source: stateFeature, DurationRounds: 10,
		EndsOn: []string{StateEndsDismiss, StateEndsIncapacitated}, Resource: stateUses, Economy: EconomyBonusAction, TextPT: "Você assume a postura.",
	}
	if !reflect.DeepEqual(s, want) {
		t.Errorf("state = %+v, want %+v", s, want)
	}
	if got := Derive(stateBuild(2), c).States; len(got) != 0 {
		t.Errorf("the state is on the sheet before the feature's level: %+v", got)
	}
}

func TestTheDurationIsAFormulaAndNothingMeansUntilEnded(t *testing.T) {
	for _, tc := range []struct {
		formula string
		want    int
	}{{"prof() * 5", 10}, {"classLevel(\"fighter\")", 3}, {"0", 0}, {"", 0}} {
		effects := stanceEffects()
		effects[1].DurationRounds = tc.formula
		effects[1].EndsOn = []string{StateEndsDismiss}
		c := withOverlay(t, stateOverlay(effects))
		s, ok := stateOf(Derive(stateBuild(3), c), stateKeyOfTest())
		if !ok || s.DurationRounds != tc.want {
			t.Errorf("duration %q = %+v, want %d rounds", tc.formula, s, tc.want)
		}
	}
}

func TestTheStateIsAnActionLikeAGrantedOneAndEndsOnTheSameCard(t *testing.T) {
	c := withOverlay(t, stateOverlay(stanceEffects()))

	start, ok := actionOf(Derive(stateBuild(3), c), stateKeyOfTest())
	if !ok {
		t.Fatal("no action to start the state")
	}
	if start.Economy != EconomyBonusAction || start.Resource != stateUses || start.Source != stateFeature || start.State != stateKeyOfTest() || start.StateActive {
		t.Errorf("start action = %+v, want a bonus action that spends %s and is not yet active", start, stateUses)
	}

	end, ok := actionOf(Derive(stateBuild(3, stateKeyOfTest()), c), stateKeyOfTest())
	if !ok {
		t.Fatal("no action to end the state")
	}
	if end.Economy != EconomyFree || end.Resource != "" || !end.StateActive || end.State != stateKeyOfTest() || end.NamePT != start.NamePT {
		t.Errorf("end action = %+v, want a free action with no resource on the same card (%q)", end, start.NamePT)
	}
}

func TestTheStateIsNotOfferedWithoutItsResourceButAnActiveOneCanEnd(t *testing.T) {
	effects := stanceEffects()
	effects[0].Max = "0" // no use at this level
	c := withOverlay(t, stateOverlay(effects))

	off := Derive(stateBuild(3), c)
	if _, ok := actionOf(off, stateKeyOfTest()); ok {
		t.Error("a state with no use left to start is offered")
	}
	if _, ok := stateOf(off, stateKeyOfTest()); ok {
		t.Error("a state with no resource at this level is listed")
	}
	on := Derive(stateBuild(3, stateKeyOfTest()), c)
	if a, ok := actionOf(on, stateKeyOfTest()); !ok || !a.StateActive {
		t.Errorf("an active state cannot be ended: %+v", a)
	}
	if s, ok := stateOf(on, stateKeyOfTest()); !ok || !s.Active {
		t.Errorf("an active state is not listed as active: %+v", s)
	}
}

func TestAStateWithNoResourceCostsNothingToStart(t *testing.T) {
	effects := []Effect{{Type: "state", StateName: "forma", EndsOn: []string{StateEndsLongRest}, Economy: EconomyFree}}
	c := withOverlay(t, stateOverlay(effects))
	a, ok := actionOf(Derive(stateBuild(3), c), stateFeature+"#forma")
	if !ok || a.Resource != "" || a.Economy != EconomyFree {
		t.Errorf("action = %+v, want a free one with no resource", a)
	}
}

func TestNamesBelongToTheEntryNotToTheWholeTable(t *testing.T) {
	// A feat and a subclass each define "postura": the key of each is its own feature's.
	const feat = "feat:postura-de-feat" + tableSuffix
	o := stateOverlay(stanceEffects())
	o.Feats = []TableFeat{{TableEntry: TableEntry{Key: feat, NamePT: "Postura de Feat"}, Effects: []Effect{
		{Type: "state", StateName: stateName, EndsOn: []string{StateEndsDismiss}, Economy: EconomyFree},
		{Type: "modifier", Target: "initiative", Mode: "add", Value: "5", When: `active("postura")`},
	}}}
	c := withOverlay(t, o)
	b := stateBuild(3, feat+"#"+stateName)
	b.Feats = []string{feat}
	d := Derive(b, c)

	base := Derive(func() Build { x := b; x.ActiveStates = nil; return x }(), c)
	if d.ArmorClass != base.ArmorClass {
		t.Errorf("the feat's state opened the subclass's gate: AC %d, want %d", d.ArmorClass, base.ArmorClass)
	}
	if d.Initiative != base.Initiative+5 {
		t.Errorf("initiative = %d, want %d: the feat's own state is on", d.Initiative, base.Initiative+5)
	}
}

func TestAnotherFeatureOfTheEntryReadsTheStateByName(t *testing.T) {
	aside := Effect{Type: "modifier", Target: "initiative", Mode: "add", Value: "3", When: `active("postura")`}
	c := withOverlay(t, stateOverlay(stanceEffects(), aside))
	off, on := Derive(stateBuild(3), c), Derive(stateBuild(3, stateKeyOfTest()), c)
	if on.Initiative != off.Initiative+3 {
		t.Errorf("initiative = %d, want %d", on.Initiative, off.Initiative+3)
	}
}

func TestRollModeAndExtraDamageFollowTheState(t *testing.T) {
	effects := append(stanceEffects(),
		Effect{Type: "roll_mode", Roll: "advantage", Targets: []string{"save.dex"}, While: stateName},
		Effect{Type: "extra_damage", Applies: ExtraDamageAnyTarget, Value: "2", While: stateName},
	)
	c := withOverlay(t, stateOverlay(effects))

	off := Derive(stateBuild(3), c)
	if len(off.ExtraDamages) != 0 || slices.ContainsFunc(off.Hints, func(h Hint) bool { return h.Source == stateFeature && h.Mode == "advantage" }) {
		t.Errorf("the gated effects apply with the state off: %+v, %+v", off.ExtraDamages, off.Hints)
	}
	on := Derive(stateBuild(3, stateKeyOfTest()), c)
	if len(on.ExtraDamages) != 1 || on.ExtraDamages[0].Amount != 2 {
		t.Errorf("extra damages = %+v, want the +2 while the state is on", on.ExtraDamages)
	}
	if !slices.ContainsFunc(on.Hints, func(h Hint) bool {
		return h.Source == stateFeature && h.Mode == "advantage" && slices.Contains(h.Targets, "save.dex")
	}) {
		t.Errorf("no advantage hint while the state is on: %+v", on.Hints)
	}
}

func TestAResistanceWhileAStateIsListedOnlyWhileItIsOn(t *testing.T) {
	effects := append(stanceEffects(), Effect{Type: "resistance", Except: "damage-type:psychic", While: stateName})
	c := withOverlay(t, stateOverlay(effects))

	if _, ok := resistanceOf(c.Resistances(Derive(stateBuild(3), c)), stateFeature); ok {
		t.Error("the resistance is listed with the state off")
	}
	r, ok := resistanceOf(c.Resistances(Derive(stateBuild(3, stateKeyOfTest()), c)), stateFeature)
	if !ok || r.While != ResistanceAlways || slices.Contains(r.DamageTypes, "damage-type:psychic") || len(r.DamageTypes) < 5 {
		t.Errorf("resistance = %+v, ok %v, want every type but psychic, holding at once", r, ok)
	}
	// The sheet still tells it as a note, with the state's name.
	d := Derive(stateBuild(3), c)
	i := slices.IndexFunc(d.Hints, func(h Hint) bool { return h.Source == stateFeature && h.Mode == "note" })
	if i < 0 || !strings.Contains(d.Hints[i].TextPT, "Postura Firme") {
		t.Errorf("hints = %+v, want a note that names the state's feature", d.Hints)
	}
}

func TestTheRageResistanceKeepsItsOwnGate(t *testing.T) {
	o := pathOverlay(resistanceEffect())
	c := withOverlay(t, o)
	got, ok := resistanceOf(c.Resistances(Derive(pathBuild(3), c)), pathFeature)
	if !ok || got.While != ResistanceWhileRage {
		t.Errorf("resistance = %+v, ok %v, want it listed with while rage whatever the active states", got, ok)
	}
}

// stateRefusals is the table of what the overlay refuses, each at its field.
var stateRefusals = []struct {
	name   string
	edit   func(es []Effect)
	field  string
	reason string
}{
	{"no name", func(es []Effect) { es[1].StateName = "" }, ".state_name", ReasonValue},
	{"a name with capitals and a space", func(es []Effect) { es[1].StateName = "Minha Postura" }, ".state_name", ReasonValue},
	{"a name over 40 characters", func(es []Effect) { es[1].StateName = strings.Repeat("a", 41) }, ".state_name", ReasonValue},
	{"a duration that does not compile", func(es []Effect) { es[1].DurationRounds = "level(" }, ".duration_rounds", ReasonFormula},
	{"a duration that reads a state", func(es []Effect) { es[1].DurationRounds = `active("postura") ? 1 : 2` }, ".duration_rounds", ReasonFormula},
	{"an unknown end", func(es []Effect) { es[1].EndsOn = []string{"never"} }, ".ends_on", ReasonValue},
	{"a repeated end", func(es []Effect) { es[1].EndsOn = []string{StateEndsDismiss, StateEndsDismiss} }, ".ends_on", ReasonValue},
	{"no duration and no end", func(es []Effect) { es[1].DurationRounds, es[1].EndsOn = "", nil }, ".ends_on", ReasonValue},
	{"a resource nothing defines", func(es []Effect) { es[1].Resource = "fantasma" }, ".resource", ReasonReference},
	{"no economy", func(es []Effect) { es[1].Economy = "" }, ".economy", ReasonValue},
	{"a reaction", func(es []Effect) { es[1].Economy = EconomyReaction }, ".economy", ReasonValue},
	{"a movement", func(es []Effect) { es[1].Economy = EconomyMovement }, ".economy", ReasonValue},
	{"tags", func(es []Effect) { es[1].Tags = []string{"about:teste"} }, ".tags", ReasonValue},
	{"a field of another type", func(es []Effect) { es[1].Target = "ac" }, ".target", ReasonValue},
	{"a gate on a modifier", func(es []Effect) { es[2].While = stateName }, ".while", ReasonValue},
	{"a state of nobody on a resistance", func(es []Effect) {
		es[3] = Effect{Type: "resistance", Except: "damage-type:psychic", While: "fantasma"}
	}, ".while", ReasonReference},
	{"a state of nobody on a roll mode", func(es []Effect) {
		es[3] = Effect{Type: "roll_mode", Roll: "advantage", Targets: []string{"attack"}, While: "fantasma"}
	}, ".while", ReasonReference},
	{"rage on a roll mode", func(es []Effect) {
		es[3] = Effect{Type: "roll_mode", Roll: "advantage", Targets: []string{"attack"}, While: ResistanceWhileRage}
	}, ".while", ReasonReference},
	{"a state of nobody on an extra damage", func(es []Effect) {
		es[3] = Effect{Type: "extra_damage", Applies: ExtraDamageAnyTarget, Value: "1", While: "fantasma"}
	}, ".while", ReasonReference},
	{"active() of a state of nobody", func(es []Effect) { es[2].When = `active("fantasma")` }, ".when", ReasonReference},
	{"active() in a value", func(es []Effect) { es[2].Value = `active("postura") ? 2 : 1` }, ".value", ReasonFormula},
	{"active() with something that is not a name", func(es []Effect) { es[2].When = `active("Postura")` }, ".when", ReasonFormula},
}

func TestTheStateOverlayRefusalsNameTheirField(t *testing.T) {
	t.Parallel()
	for _, tc := range stateRefusals {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			es := slices.Clone(stanceEffects())
			es = append(es, Effect{Type: "note", TextPT: "Reserva."})
			tc.edit(es)
			o := stateOverlay(es)
			o.Strict = []string{stateSubclass}
			_, err := loadForTest(t).With(o)
			var oe *OverlayError
			if !errors.As(err, &oe) {
				t.Fatalf("want an OverlayError, got %v", err)
			}
			if !slices.ContainsFunc(oe.Violations(), func(v *OverlayError) bool {
				return strings.Contains(v.Field, "effects[") && strings.HasSuffix(v.Field, tc.field) && v.Reason == tc.reason
			}) {
				t.Errorf("violations = %+v, want one at %s with reason %s", oe.Violations(), tc.field, tc.reason)
			}
		})
	}
}

func TestTwoStatesOfOneEntryCannotShareAName(t *testing.T) {
	second := Effect{Type: "state", StateName: stateName, EndsOn: []string{StateEndsDismiss}, Economy: EconomyFree}
	o := stateOverlay(stanceEffects(), second)
	_, err := loadForTest(t).With(o)
	var oe *OverlayError
	if !errors.As(err, &oe) || !slices.ContainsFunc(oe.Violations(), func(v *OverlayError) bool { return strings.HasSuffix(v.Field, ".state_name") }) {
		t.Fatalf("want a refusal at state_name, got %v", err)
	}
}

func TestAnEntryHasAtMostEightStates(t *testing.T) {
	var many []Effect
	for i := range MaxEntryStates + 1 {
		many = append(many, Effect{Type: "state", StateName: "estado_" + string(rune('a'+i)), EndsOn: []string{StateEndsDismiss}, Economy: EconomyFree})
	}
	o := stateOverlay(many[:MaxEntryStates])
	if _, err := loadForTest(t).With(o); err != nil {
		t.Fatalf("%d states are refused: %v", MaxEntryStates, err)
	}
	o = stateOverlay(many[:5], many[5:]...)
	_, err := loadForTest(t).With(o)
	var oe *OverlayError
	if !errors.As(err, &oe) || !slices.ContainsFunc(oe.Violations(), func(v *OverlayError) bool { return v.Reason == ReasonLimit }) {
		t.Fatalf("want a limit refusal for %d states, got %v", len(many), err)
	}
}

func TestAStateOfAnotherEntryIsNotVisibleToAGate(t *testing.T) {
	o := stateOverlay(stanceEffects())
	o.Feats = []TableFeat{{TableEntry: TableEntry{Key: "feat:vizinho" + tableSuffix, NamePT: "Vizinho"}, Effects: []Effect{
		{Type: "modifier", Target: "initiative", Mode: "add", Value: "1", When: `active("postura")`},
	}}}
	_, err := loadForTest(t).With(o)
	var oe *OverlayError
	if !errors.As(err, &oe) || !slices.ContainsFunc(oe.Violations(), func(v *OverlayError) bool { return strings.HasSuffix(v.Field, ".when") && v.Reason == ReasonReference }) {
		t.Fatalf("want a reference refusal at when, got %v", err)
	}
}

func TestStatesAreOnTheEffectMenu(t *testing.T) {
	menu := loadForTest(t).EffectMenu()
	i := slices.IndexFunc(menu.Types, func(m MenuType) bool { return m.Type == "state" })
	if i < 0 {
		t.Fatal("no state type on the menu")
	}
	fields := map[string]MenuField{}
	for _, f := range menu.Types[i].Fields {
		fields[f.Name] = f
	}
	for _, name := range []string{"state_name", "economy"} {
		if !fields[name].Required {
			t.Errorf("%s is not required", name)
		}
	}
	if f := fields["ends_on"]; f.Kind != "choices" || f.List != ListStateEnds {
		t.Errorf("ends_on = %+v, want several of %s", f, ListStateEnds)
	}
	if f := fields["duration_rounds"]; f.Kind != "formula" || f.Required {
		t.Errorf("duration_rounds = %+v, want an optional formula", f)
	}
	if f := fields["economy"]; f.List != ListStateEconomies {
		t.Errorf("economy = %+v, want the list %s", f, ListStateEconomies)
	}
	if f := fields["resource"]; f.Kind != "resource_ref" {
		t.Errorf("resource = %+v, want a resource reference", f)
	}
	for _, l := range []string{ListStateEnds, ListStateEconomies} {
		if !slices.ContainsFunc(menu.Lists, func(m MenuList) bool { return m.Name == l && len(m.Values) > 0 }) {
			t.Errorf("list %s is missing", l)
		}
	}
	for _, h := range menu.Helpers {
		if h.Call == `active("<estado>")` {
			return
		}
	}
	t.Error(`the formula helpers lack active("<estado>")`)
}

func TestTheGateIsOnTheMenuOfEveryTypeThatReadsIt(t *testing.T) {
	menu := loadForTest(t).EffectMenu()
	for _, typ := range gatedEffectTypes {
		i := slices.IndexFunc(menu.Types, func(m MenuType) bool { return m.Type == typ })
		if i < 0 {
			t.Fatalf("type %s is not on the menu", typ)
		}
		j := slices.IndexFunc(menu.Types[i].Fields, func(f MenuField) bool { return f.Name == "while" })
		if j < 0 {
			t.Errorf("type %s reads the gate but its menu has no while", typ)
			continue
		}
		if f := menu.Types[i].Fields[j]; f.Kind != "state_ref" || f.Required {
			t.Errorf("type %s: while = %+v, want an optional state reference", typ, f)
		}
		if !slices.Contains(ownFields[typ], "while") {
			t.Errorf("type %s: while is not in its own fields", typ)
		}
	}
}

func TestAStateCannotBeNamedRage(t *testing.T) {
	_, err := loadForTest(t).With(Overlay{Revision: 1, Classes: []TableClass{classWithEffects([]Effect{
		{Type: "state", StateName: "rage", EndsOn: []string{StateEndsDismiss}, Economy: EconomyFree},
	})}})
	if err == nil {
		t.Fatal("a state named rage is accepted: a resistance's while rage would mean the barbarian's rage")
	}
}

// entryStateEffects are the effects of an entry with a state of its own, a resistance gated by `while` and a modifier gated by active().
func entryStateEffects() []Effect {
	return []Effect{
		{Type: "state", StateName: "postura", EndsOn: []string{StateEndsDismiss}, Economy: EconomyFree},
		{Type: "resistance", Except: "damage-type:psychic", While: "postura"},
		{Type: "modifier", Target: "speed.walk", Mode: "add", Value: "10", When: `active("postura")`},
	}
}

func TestAClassFeatureAndAnOptionDefineTheirOwnStates(t *testing.T) {
	feature := TableClassFeature{
		TableEntry: TableEntry{Key: "classfeature:postura-de-teste@mesa", NamePT: "Postura de Teste"},
		Class:      "class:ranger", Level: 1, Effects: entryStateEffects(),
	}
	option := TableOption{
		TableEntry: TableEntry{Key: "option:fighting_style:postura-de-teste@mesa", NamePT: "Postura de Teste"},
		Set:        "fighting_style", Effects: entryStateEffects(),
	}
	for name, o := range map[string]Overlay{
		"class feature": {Revision: 1, ClassFeatures: []TableClassFeature{feature}},
		"option":        {Revision: 1, Options: []TableOption{option}},
	} {
		if _, err := loadForTest(t).With(o); err != nil {
			t.Errorf("%s: a state, a resistance with while and a modifier with active() of the same entry are refused: %v", name, err)
		}
	}

	twice := append(entryStateEffects(), Effect{Type: "state", StateName: "postura", EndsOn: []string{StateEndsDismiss}, Economy: EconomyFree})
	feature.Effects, option.Effects = twice, twice
	for name, o := range map[string]Overlay{
		"class feature": {Revision: 1, ClassFeatures: []TableClassFeature{feature}},
		"option":        {Revision: 1, Options: []TableOption{option}},
	} {
		if _, err := loadForTest(t).With(o); err == nil {
			t.Errorf("%s: two states with one name are accepted", name)
		}
	}
}

func TestADurationThatFailsKeepsTheStateListedForUntilItEndsAndSaysSo(t *testing.T) {
	effects := stanceEffects()
	effects[1].DurationRounds = `10 / (classLevel("fighter") - 3)` // divides by zero at level 3
	effects[1].EndsOn = []string{StateEndsDismiss}
	c := withOverlay(t, stateOverlay(effects))
	d := Derive(stateBuild(3), c)
	s, ok := stateOf(d, stateKeyOfTest())
	if !ok || s.DurationRounds != 0 {
		t.Errorf("state = %+v (listed %v), want it listed with no duration: it lasts until it ends", s, ok)
	}
	i := slices.IndexFunc(d.Issues, func(i Issue) bool { return i.Code == IssueFormula })
	if i < 0 {
		t.Fatalf("issues = %+v, want a formula issue", d.Issues)
	}
	if got := d.Issues[i].Message; !strings.Contains(got, "dura até acabar") || strings.Contains(got, "ignorado") {
		t.Errorf("message = %q, want it to say the state lasts until it ends and not that the effect was skipped", got)
	}
}
