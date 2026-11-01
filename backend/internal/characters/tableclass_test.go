package characters

import (
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The table's classes and subclasses end to end (MR-025, RN-23, ADR-0018): the class editor starts from the server's numbers and
// menu, a refusal points at its field, a character is created (also multiclassed)
// and goes up with a table class and a third caster's subclass, and "A classe
// mudou" tells each common change in its own sentence. Each test starts its own
// database.

// classDefaults reads the editor's defaults as u.
func (u *user) classDefaults(t *testing.T, campaign string) *rulesv1.GetClassTableDefaultsResponse {
	t.Helper()
	res, err := u.table.GetClassTableDefaults(t.Context(), connect.NewRequest(&rulesv1.GetClassTableDefaultsRequest{CampaignId: campaign}))
	if err != nil {
		t.Fatalf("GetClassTableDefaults() error = %v", err)
	}
	return res.Msg
}

// effectMenu reads the editor's menu as u.
func (u *user) effectMenu(t *testing.T, campaign string) *rulesv1.GetEffectMenuResponse {
	t.Helper()
	res, err := u.table.GetEffectMenu(t.Context(), connect.NewRequest(&rulesv1.GetEffectMenuRequest{CampaignId: campaign}))
	if err != nil {
		t.Fatalf("GetEffectMenu() error = %v", err)
	}
	return res.Msg
}

// castingTable finds the default table of a kind and preparation.
func castingTable(t *testing.T, d *rulesv1.GetClassTableDefaultsResponse, kind, preparation string) *rulesv1.CastingTableDefault {
	t.Helper()
	for _, tab := range d.GetTables() {
		if tab.GetKind() == kind && tab.GetPreparation() == preparation {
			return tab
		}
	}
	t.Fatalf("no default table for %q %q", kind, preparation)
	return nil
}

// rowsOf copies a default table's rows, the way the editor makes a class's levels.
func rowsOf(tab *rulesv1.CastingTableDefault) []*rulesv1.TableClassLevel {
	out := make([]*rulesv1.TableClassLevel, len(tab.GetRows()))
	for i, r := range tab.GetRows() {
		out[i] = proto.CloneOf(r)
	}
	return out
}

// fightingStyles are the keys of the SRD's fighting styles, from the menu's option sets.
func fightingStyles(t *testing.T, menu *rulesv1.GetEffectMenuResponse) []string {
	t.Helper()
	for _, set := range menu.GetOptionSets() {
		var keys []string
		for _, o := range set.GetOptions() {
			keys = append(keys, o.GetKey())
		}
		if slices.Contains(keys, "feature:fighter-fighting-style-defense") {
			return keys
		}
	}
	t.Fatal("the menu has no fighting style set")
	return nil
}

var valleySkills = []string{"skill:animal-handling", "skill:athletics", "skill:nature", "skill:perception", "skill:stealth", "skill:survival"}

// guardianClass is "Guardião do Vale": a half caster that prepares from the
// druid's list, whose table is the server's default one (the editor's start),
// with these features: a sense, a fighting style at level 2 chosen from
// the SRD's set, and an extra attack at level 5.
func guardianClass(t *testing.T, d *rulesv1.GetClassTableDefaultsResponse, menu *rulesv1.GetEffectMenuResponse) *rulesv1.TableClass {
	t.Helper()
	levels := rowsOf(castingTable(t, d, "half", "prepared"))
	levels[0].Features = []*rulesv1.TableFeature{{NamePt: "Olhos do Vale", DescPt: []string{"Você enxerga no escuro."}, Effects: []*rulesv1.TableEffect{{Type: "sense", Sense: "darkvision", RangeFt: 60}}}}
	levels[1].Features = []*rulesv1.TableFeature{{NamePt: "Estilo de luta", Effects: []*rulesv1.TableEffect{{Type: "choice", Choice: "feature", Count: 1, From: fightingStyles(t, menu)[:3]}}}}
	levels[4].Features = []*rulesv1.TableFeature{{NamePt: "Ataque duplo", Effects: []*rulesv1.TableEffect{{Type: "extra_attack", Count: 2}}}}
	return &rulesv1.TableClass{
		NamePt: "Guardião do Vale", HitDie: 10,
		SavingThrows: []rulesv1.Ability{rulesv1.Ability_ABILITY_STRENGTH, rulesv1.Ability_ABILITY_WISDOM},
		SkillChoose:  2, SkillFrom: valleySkills,
		Proficiencies: []string{"proficiency:light-armor", "proficiency:simple-weapons"},
		Minimums:      &rulesv1.AbilityScores{Wisdom: 13},
		SubclassLevel: 3,
		Casting:       &rulesv1.TableCasting{Kind: "half", Ability: rulesv1.Ability_ABILITY_WISDOM, Preparation: "prepared", ListFrom: "class:druid"},
		Levels:        levels,
	}
}

// valleyPath is the guardian's subclass: always-prepared spells at levels 3 and 5.
func valleyPath(class string) *rulesv1.TableSubclass {
	return &rulesv1.TableSubclass{
		NamePt: "Caminho do Vale", ClassKey: class, DescPt: []string{"Quem anda o vale inteiro."},
		Levels: []*rulesv1.TableSubclassLevel{
			{Level: 3, Features: []*rulesv1.TableFeature{{NamePt: "Passo do Vale", Effects: []*rulesv1.TableEffect{{Type: "modifier", Target: "speed.walk", Mode: "add", Value: "5"}}}}},
			{Level: 7, Features: []*rulesv1.TableFeature{{NamePt: "Olhar de longe", DescPt: []string{"Só texto."}}}},
		},
		AlwaysPrepared: []*rulesv1.TableAlwaysPrepared{{ClassLevel: 3, SpellKey: "spell:cure-wounds"}, {ClassLevel: 5, SpellKey: "spell:lesser-restoration"}},
	}
}

// inkBlade is a third caster of the Fighter: the default third-caster table, the
// Intelligence, the wizard's list.
func inkBlade(t *testing.T, d *rulesv1.GetClassTableDefaultsResponse) *rulesv1.TableSubclass {
	t.Helper()
	tab := castingTable(t, d, "third", "known")
	sub := &rulesv1.TableSubclass{
		NamePt: "Lâmina de Tinta", ClassKey: "class:fighter", DescPt: []string{"Tinta e aço."},
		Casting: &rulesv1.TableCasting{Kind: "third", Ability: rulesv1.Ability_ABILITY_INTELLIGENCE, Preparation: "known", ListFrom: "class:wizard"},
	}
	for _, r := range tab.GetRows()[tab.GetStartLevel()-1:] {
		lvl := &rulesv1.TableSubclassLevel{Level: i32(len(sub.GetLevels())) + tab.GetStartLevel(), CantripsKnown: r.GetCantripsKnown(), SpellsKnown: r.GetSpellsKnown(), Slots: slices.Clone(r.GetSlots())}
		if lvl.GetLevel() == 3 {
			lvl.Features = []*rulesv1.TableFeature{{NamePt: "Lâmina entintada", DescPt: []string{"A lâmina pinga tinta."}}}
		}
		sub.Levels = append(sub.Levels, lvl)
	}
	return sub
}

// TestTableClassEditorStartsFromTheServer: the master reads
// the defaults of the 20-level table and the closed menu of effects, a player does
// not, the numbers are the SRD's, the menu lists the table's own classes, and a
// class built from each default table is accepted untouched.
func TestTableClassEditorStartsFromTheServer(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, player, stranger := h.newUser("Samuel"), h.newUser("Davi"), h.newUser("Intruso")
	campaign := h.newCampaign(master, "Mirathel", player)
	srd := loadRules(t)

	// Only the master: a player is refused, a stranger finds no campaign.
	for name, u := range map[string]*user{"player": player, "stranger": stranger} {
		want := connect.CodePermissionDenied
		if name == "stranger" {
			want = connect.CodeNotFound
		}
		_, err := u.table.GetClassTableDefaults(t.Context(), connect.NewRequest(&rulesv1.GetClassTableDefaultsRequest{CampaignId: campaign}))
		wantCode(t, name+" GetClassTableDefaults", err, want)
		_, err = u.table.GetEffectMenu(t.Context(), connect.NewRequest(&rulesv1.GetEffectMenuRequest{CampaignId: campaign}))
		wantCode(t, name+" GetEffectMenu", err, want)
	}

	d := master.classDefaults(t, campaign)
	want := srd.TableDefaults()
	if len(d.GetProfBonus()) != 20 || d.GetSubclassLevel() != 3 || len(d.GetTables()) != len(want.Tables) || !slices.Equal(d.GetAsiLevels(), []int32{4, 8, 12, 16, 19}) {
		t.Fatalf("defaults = %v, want 20 bonuses, level 3, the ASI levels and %d tables", d, len(want.Tables))
	}
	for i, tab := range d.GetTables() {
		w := want.Tables[i]
		if tab.GetKind() != w.Kind || tab.GetPreparation() != w.Preparation || tab.GetStartLevel() != i32(w.StartLevel) || len(tab.GetRows()) != 20 || tab.GetReferenceClassKey() != w.Reference {
			t.Errorf("table %d = %v/%v start %d, want %v/%v start %d", i, tab.GetKind(), tab.GetPreparation(), tab.GetStartLevel(), w.Kind, w.Preparation, w.StartLevel)
		}
		for j, r := range tab.GetRows() {
			row := w.Rows[j]
			if r.GetProfBonus() != i32(row.ProfBonus) || r.GetCantripsKnown() != i32(row.CantripsKnown) || r.GetSpellsKnown() != i32(row.SpellsKnown) {
				t.Errorf("%s/%s row %d = %v, want %+v", tab.GetKind(), tab.GetPreparation(), j+1, r, row)
			}
			for k, n := range r.GetSlots() {
				if n != i32(row.Slots[k]) {
					t.Errorf("%s/%s row %d slots = %v, want %v", tab.GetKind(), tab.GetPreparation(), j+1, r.GetSlots(), row.Slots)
					break
				}
			}
		}
	}
	// Spot values of the SRD's tables, as the artboard states them: the Paladin's
	// half-caster table (notes) and a third caster's at levels 3 and 20.
	half := castingTable(t, d, "half", "prepared")
	for lvl, slots := range map[int][]int32{2: {2}, 5: {4, 2}, 9: {4, 3, 2}, 13: {4, 3, 3, 1}, 17: {4, 3, 3, 3, 1}} {
		got := half.GetRows()[lvl-1].GetSlots()
		if !slices.Equal(got[:len(slots)], slots) {
			t.Errorf("half caster level %d slots = %v, want %v", lvl, got, slots)
		}
	}
	if half.GetRows()[0].GetSlots() != nil || half.GetStartLevel() != 2 {
		t.Errorf("half caster level 1 = %v from level %d, want no slots before level 2", half.GetRows()[0], half.GetStartLevel())
	}

	// The menu: every type with its name, the lists they point at, the option sets,
	// the helpers and the classes classLevel takes.
	menu := master.effectMenu(t, campaign)
	if len(menu.GetTypes()) != 25 || menu.GetMaxFeaturesPerClass() != 60 || menu.GetMaxEffectsPerFeature() != 20 || menu.GetExtraAttackMin() != 2 || menu.GetExtraAttackMax() != 4 {
		t.Fatalf("menu = %d types, limits %d/%d, extra attacks %d to %d", len(menu.GetTypes()), menu.GetMaxFeaturesPerClass(), menu.GetMaxEffectsPerFeature(), menu.GetExtraAttackMin(), menu.GetExtraAttackMax())
	}
	lists := map[string]*rulesv1.EffectMenuList{}
	for _, l := range menu.GetLists() {
		lists[l.GetName()] = l
	}
	for _, ty := range menu.GetTypes() {
		if ty.GetNamePt() == "" || ty.GetHintPt() == "" || len(ty.GetFields()) == 0 {
			t.Errorf("type %v is incomplete", ty)
		}
		for _, f := range ty.GetFields() {
			if f.GetList() != "" && lists[f.GetList()] == nil {
				t.Errorf("type %s field %s points at the list %q that the menu does not have", ty.GetType(), f.GetName(), f.GetList())
			}
		}
	}
	senses := lists["senses"]
	if senses == nil || len(senses.GetValues()) != 4 || senses.GetValues()[0].GetKey() != "darkvision" || senses.GetValues()[0].GetNamePt() != "Visão no escuro" {
		t.Errorf("senses = %v", senses)
	}
	if len(menu.GetHelpers()) != 13 || len(fightingStyles(t, menu)) < 4 {
		t.Errorf("helpers = %d, fighting styles = %d", len(menu.GetHelpers()), len(fightingStyles(t, menu)))
	}
	if !slices.Contains(menu.GetClassIndexes(), "wizard") || slices.ContainsFunc(menu.GetClassIndexes(), func(s string) bool { return strings.HasSuffix(s, "@mesa") }) {
		t.Errorf("class indexes = %v, want the SRD's and no table class yet", menu.GetClassIndexes())
	}
	class := master.addEntry(t, campaign, guardianClass(t, d, menu))
	menu = master.effectMenu(t, campaign)
	if !slices.Contains(menu.GetClassIndexes(), strings.TrimPrefix(class.GetKey(), "class:")) {
		t.Errorf("class indexes = %v, want the table's guardian (%s)", menu.GetClassIndexes(), class.GetKey())
	}

	// The editor saves what it starts from: a class of each default table, and a
	// third caster's subclass of the Fighter, unchanged.
	for i, tab := range d.GetTables() {
		if tab.GetKind() == "third" {
			continue
		}
		body := &rulesv1.TableClass{
			NamePt: "Padrão " + string(rune('A'+i)), HitDie: 8, SavingThrows: []rulesv1.Ability{rulesv1.Ability_ABILITY_CONSTITUTION, rulesv1.Ability_ABILITY_WISDOM},
			SkillChoose: 2, SkillFrom: valleySkills, Levels: rowsOf(tab),
		}
		if tab.GetKind() != "" {
			list := "class:cleric"
			if tab.GetPreparation() == "known" {
				list = "class:sorcerer"
			}
			body.Casting = &rulesv1.TableCasting{Kind: tab.GetKind(), Ability: rulesv1.Ability_ABILITY_WISDOM, Preparation: tab.GetPreparation(), ListFrom: list}
		}
		master.addEntry(t, campaign, body)
	}
	master.addEntry(t, campaign, inkBlade(t, d))
}

// violationsOf creates (or updates) and returns the refusal's violations.
func (u *user) refused(t *testing.T, campaign string, body proto.Message) []*rulesv1.TableContentViolation {
	t.Helper()
	_, err := u.table.CreateTableEntry(t.Context(), connect.NewRequest(createReq(campaign, body)))
	return violationsOfErr(t, err)
}

// TestTableClassRefusalsPointAtTheirField: every rule a
// class or subclass breaks comes back as a violation at the exact field the editor
// draws, with a stable reason, and a refused write changes nothing.
func TestTableClassRefusalsPointAtTheirField(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Samuel")
	campaign := h.newCampaign(master, "Mirathel")
	d := master.classDefaults(t, campaign)
	menu := master.effectMenu(t, campaign)
	class := master.addEntry(t, campaign, guardianClass(t, d, menu))
	revision := h.contentRevision(campaign)

	guardian := func(edit func(c *rulesv1.TableClass)) proto.Message {
		c := proto.CloneOf(guardianClass(t, d, menu))
		c.NamePt = "Outro Guardião"
		edit(c)
		return c
	}
	sub := func(edit func(s *rulesv1.TableSubclass)) proto.Message {
		s := inkBlade(t, d)
		s.NamePt = "Outra Lâmina"
		edit(s)
		return s
	}
	valley := func(edit func(s *rulesv1.TableSubclass)) proto.Message {
		s := valleyPath(class.GetKey())
		s.NamePt = "Outro Caminho"
		edit(s)
		return s
	}
	feature := func(c *rulesv1.TableClass, e *rulesv1.TableEffect) {
		c.Levels[0].Features[0].Effects = []*rulesv1.TableEffect{e}
	}
	cases := []struct {
		name          string
		body          proto.Message
		field, reason string
	}{
		{"a slot count", guardian(func(c *rulesv1.TableClass) { c.Levels[4].Slots[2] = 10 }), "table_class.levels[4].slots[2]", "bad_table"},
		{"slots of eight", guardian(func(c *rulesv1.TableClass) { c.Levels[4].Slots = c.Levels[4].Slots[:8] }), "table_class.levels[4].slots", "bad_table"},
		{"slots before the casting starts", guardian(func(c *rulesv1.TableClass) { c.Levels[0].Slots = []int32{2, 0, 0, 0, 0, 0, 0, 0, 0} }), "table_class.levels[0].slots[0]", "bad_table"},
		{"cantrips known", guardian(func(c *rulesv1.TableClass) { c.Levels[6].CantripsKnown = 99 }), "table_class.levels[6].cantrips_known", "bad_table"},
		{"a caster without slots", guardian(func(c *rulesv1.TableClass) { c.Levels[8].Slots = nil }), "table_class.levels[8].slots", "bad_table"},
		{"a proficiency bonus", guardian(func(c *rulesv1.TableClass) { c.Levels[2].ProfBonus = 13 }), "table_class.levels[2].prof_bonus", "bad_value"},
		{"the casting list", guardian(func(c *rulesv1.TableClass) { c.Casting.ListFrom = "class:nada" }), "table_class.casting.list_from", "dangling_reference"},
		{"the casting kind", guardian(func(c *rulesv1.TableClass) { c.Casting.Kind = "third" }), "table_class.casting.kind", "bad_casting"},
		{"the casting ability", guardian(func(c *rulesv1.TableClass) { c.Casting.Ability = rulesv1.Ability_ABILITY_UNSPECIFIED }), "table_class.casting.ability", "bad_casting"},
		{"a saving throw", guardian(func(c *rulesv1.TableClass) { c.SavingThrows[1] = c.SavingThrows[0] }), "table_class.saving_throws[1]", "bad_value"},
		{"the skills", guardian(func(c *rulesv1.TableClass) { c.SkillChoose = 9 }), "table_class.skill_choose", "bad_value"},
		{"a skill of the list", guardian(func(c *rulesv1.TableClass) { c.SkillFrom[2] = "skill:nada" }), "table_class.skill_from[2]", "dangling_reference"},
		{"a proficiency", guardian(func(c *rulesv1.TableClass) { c.Proficiencies[1] = "proficiency:nada" }), "table_class.proficiencies[1]", "dangling_reference"},
		{"the ASI levels", guardian(func(c *rulesv1.TableClass) { c.AsiLevels = []int32{4, 8, 4} }), "table_class.asi_levels[2]", "bad_value"},
		{"a formula", guardian(func(c *rulesv1.TableClass) {
			feature(c, &rulesv1.TableEffect{Type: "modifier", Target: "initiative", Mode: "add", Value: "prof("})
		}), "table_class.levels[0].features[0].effects[0].value", "bad_formula"},
		{"a condition", guardian(func(c *rulesv1.TableClass) {
			feature(c, &rulesv1.TableEffect{Type: "modifier", Target: "initiative", Mode: "add", Value: "1", When: "armor() =="})
		}), "table_class.levels[0].features[0].effects[0].when", "bad_formula"},
		{"a resource maximum", guardian(func(c *rulesv1.TableClass) {
			feature(c, &rulesv1.TableEffect{Type: "resource", Resource: "fole", Max: "level(", Recharge: "short_rest"})
		}), "table_class.levels[0].features[0].effects[0].max", "bad_formula"},
		{"a target", guardian(func(c *rulesv1.TableClass) {
			feature(c, &rulesv1.TableEffect{Type: "modifier", Target: "sorte", Mode: "add", Value: "1"})
		}), "table_class.levels[0].features[0].effects[0].target", "bad_value"},
		{"a field the type does not read", guardian(func(c *rulesv1.TableClass) {
			feature(c, &rulesv1.TableEffect{Type: "sense", Sense: "darkvision", RangeFt: 60, Recharge: "long_rest"})
		}), "table_class.levels[0].features[0].effects[0].recharge", "bad_value"},
		{"a handler", guardian(func(c *rulesv1.TableClass) { feature(c, &rulesv1.TableEffect{Type: "handler"}) }), "table_class.levels[0].features[0].effects[0].type", "forbidden_effect"},
		{"a choice option", guardian(func(c *rulesv1.TableClass) {
			c.Levels[1].Features[0].Effects[0].From = []string{"feature:nada"}
		}), "table_class.levels[1].features[0].effects[0].from", "dangling_reference"},
		{"a granted spell", guardian(func(c *rulesv1.TableClass) {
			feature(c, &rulesv1.TableEffect{Type: "note", Spells: []string{"spell:nada"}})
		}), "table_class.levels[0].features[0].effects[0].spells", "dangling_reference"},
		{"a third caster's list", sub(func(s *rulesv1.TableSubclass) { s.Casting.ListFrom = "" }), "table_subclass.casting.list_from", "bad_casting"},
		{"a third caster's start", sub(func(s *rulesv1.TableSubclass) { s.Casting.StartLevel = 2 }), "table_subclass.casting.start_level", "bad_casting"},
		{"a third caster's rows", sub(func(s *rulesv1.TableSubclass) { s.Levels = s.Levels[:5] }), "table_subclass.levels", "bad_table"},
		{"a third caster's slots", sub(func(s *rulesv1.TableSubclass) { s.Levels[2].Slots[0] = 10 }), "table_subclass.levels[2].slots[0]", "bad_table"},
		{"a subclass of a missing class", sub(func(s *rulesv1.TableSubclass) { s.ClassKey = "class:nada@mesa" }), "table_subclass.class_key", "dangling_reference"},
		{"the subclass level", valley(func(s *rulesv1.TableSubclass) { s.Level = 9 }), "table_subclass.level", "bad_value"},
		{"an always-prepared level", valley(func(s *rulesv1.TableSubclass) { s.AlwaysPrepared[1].ClassLevel = 0 }), "table_subclass.always_prepared[1].class_level", "bad_value"},
		{"an always-prepared spell", valley(func(s *rulesv1.TableSubclass) { s.AlwaysPrepared[1].SpellKey = "spell:nada" }), "table_subclass.always_prepared[1].spell_key", "dangling_reference"},
		{"an always-prepared cantrip", valley(func(s *rulesv1.TableSubclass) { s.AlwaysPrepared[0].SpellKey = "spell:fire-bolt" }), "table_subclass.always_prepared[0].spell_key", "bad_value"},
		{"subclass levels out of order", valley(func(s *rulesv1.TableSubclass) { s.Levels[0].Level, s.Levels[1].Level = 7, 3 }), "table_subclass.levels[1].level", "bad_table"},
		{"subclass features too early", valley(func(s *rulesv1.TableSubclass) { s.Levels[0].Level = 2 }), "table_subclass.levels[0].features", "bad_table"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := master.refused(t, campaign, tc.body)
			if len(v) != 1 || v[0].GetField() != tc.field || v[0].GetReason() != tc.reason {
				t.Errorf("violations = %v, want one at %q (%s)", v, tc.field, tc.reason)
			}
		})
	}
	if got := h.contentRevision(campaign); got != revision {
		t.Errorf("revision after the refusals = %d, want %d: a refused write changes nothing", got, revision)
	}
	if n := len(master.entries(t, campaign)); n != 1 {
		t.Errorf("%d entries after the refusals, want the guardian only", n)
	}
}

// hitPointsAverage is the sheet's hit points as the editor sends them.
func hitPointsAverage() *charactersv1.HitPoints {
	return &charactersv1.HitPoints{Method: charactersv1.HitPointsMethod_HIT_POINTS_METHOD_AVERAGE}
}

// ownedSubclass sets the subclass of a class level to key.
func ownedSubclass(c *charactersv1.ClassLevel, key string) *charactersv1.ClassLevel {
	c.Subclass = &charactersv1.ClassLevel_SubclassKey{SubclassKey: key}
	return c
}

// TestTableClassMulticlassAtCreation (MR-025): Rafa creates Corvina,
// Mage 3 / Cleric 1, each class offering the table's subclass; the SRD's
// prerequisites are checked on the sheet, the hit points follow the table's rule,
// and a table class together with an SRD class, or a third caster with one, gets
// the multiclass spell slots.
func TestTableClassMulticlassAtCreation(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, rafa, p2, p3, p4, p5 := h.newUser("Samuel"), h.newUser("Rafa"), h.newUser("Lia"), h.newUser("Davi"), h.newUser("Bia"), h.newUser("Caio")
	campaign := h.newCampaign(master, "Mirathel", rafa, p2, p3, p4, p5)
	d, menu := master.classDefaults(t, campaign), master.effectMenu(t, campaign)

	guardian := master.addEntry(t, campaign, guardianClass(t, d, menu))
	ink := master.addEntry(t, campaign, inkBlade(t, d))
	tradition := master.addEntry(t, campaign, &rulesv1.TableSubclass{
		NamePt: "Tradição da Tinta", ClassKey: "class:wizard", DescPt: []string{"Magia escrita."},
		Levels: []*rulesv1.TableSubclassLevel{{Level: 2, Features: []*rulesv1.TableFeature{testNote("tinta", "Tinta viva", "A tinta obedece.")}}},
	})
	path := master.addEntry(t, campaign, &rulesv1.TableSubclass{
		NamePt: "Domínio do Caminho", ClassKey: "class:cleric", DescPt: []string{"Quem anda longe."},
		Levels:         []*rulesv1.TableSubclassLevel{{Level: 1, Features: []*rulesv1.TableFeature{testNote("passos", "Passos Longos", "Você anda mais.")}}},
		AlwaysPrepared: []*rulesv1.TableAlwaysPrepared{{ClassLevel: 1, SpellKey: "spell:longstrider"}, {ClassLevel: 1, SpellKey: "spell:detect-magic"}},
	})

	corvina := func(scores *rulesv1.AbilityScores) *charactersv1.FullSheet {
		return &charactersv1.FullSheet{
			BaseScores: scores, RaceKey: "race:human",
			Background:           &charactersv1.FullSheet_BackgroundKey{BackgroundKey: "background:acolyte"},
			Languages:            testLanguages,
			Classes:              []*charactersv1.ClassLevel{ownedSubclass(&charactersv1.ClassLevel{ClassKey: "class:wizard", Level: 3}, tradition.GetKey()), ownedSubclass(&charactersv1.ClassLevel{ClassKey: "class:cleric", Level: 1}, path.GetKey())},
			SkillProficiencyKeys: []string{"skill:arcana", "skill:investigation"},
			CantripKeys:          []string{"spell:fire-bolt", "spell:mage-hand", "spell:sacred-flame"},
			KnownSpellKeys:       []string{"spell:magic-missile", "spell:shield", "spell:sleep"},
			PreparedSpellKeys:    []string{"spell:magic-missile", "spell:shield", "spell:cure-wounds"},
			HitPoints:            hitPointsAverage(), ExperiencePoints: 2700,
		}
	}
	strong := &rulesv1.AbilityScores{Strength: 10, Dexterity: 13, Constitution: 14, Intelligence: 16, Wisdom: 14, Charisma: 8}
	create := func(u *user, name string, full *charactersv1.FullSheet) (*charactersv1.Character, error) {
		res, err := u.api.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
			CampaignId: campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: name,
			Sheet: &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: full}},
		}))
		if err != nil {
			return nil, err
		}
		return res.Msg.GetCharacter(), nil
	}
	issueCodes := func(c *charactersv1.Character) []string {
		var out []string
		for _, is := range c.GetDerived().GetIssues() {
			out = append(out, is.GetCode())
		}
		return out
	}

	// Mago 3 / Clérigo 1, both subclasses of the table, the prerequisites met: no issue.
	c, err := create(rafa, "Corvina", corvina(strong))
	if err != nil {
		t.Fatalf("CreateCharacter(Corvina) error = %v", err)
	}
	dr := c.GetDerived()
	if len(dr.GetIssues()) != 0 || dr.GetTotalLevel() != 4 || len(dr.GetClasses()) != 2 || len(dr.GetSpellcasting()) != 2 {
		t.Fatalf("Corvina: issues %v, level %d, %d classes, %d casters; want a clean 4th-level, two-class, two-caster sheet", issueCodes(c), dr.GetTotalLevel(), len(dr.GetClasses()), len(dr.GetSpellcasting()))
	}
	if got := dr.GetSpellSlots(); len(got) < 2 || got[0].GetCount() != 4 || got[1].GetCount() != 3 {
		t.Errorf("slots = %v, want the multiclass table's 4 and 3 for four caster levels", got)
	}
	for _, name := range []string{"Longos", "Detectar"} {
		if !slices.ContainsFunc(dr.GetSpells(), func(s *rulesv1.CharacterSpell) bool {
			return s.GetPrepared() && strings.Contains(s.GetSpell().GetNamePt(), name)
		}) {
			t.Errorf("the domain's always-prepared spell %q is not prepared on the sheet: %v", name, dr.GetSpells())
		}
	}
	if !slices.ContainsFunc(dr.GetFeatures(), func(f *rulesv1.Feature) bool { return f.GetNamePt() == "Tinta viva" }) || !slices.ContainsFunc(dr.GetFeatures(), func(f *rulesv1.Feature) bool { return f.GetNamePt() == "Passos Longos" }) {
		t.Errorf("features = %v, want both subclasses' features", dr.GetFeatures())
	}
	// Each class's HP: d6 at level 1... the first class gives its full die: the Mago
	// is first (6 + 2), then 4 + 2 at levels 2 and 3, and the Clérigo 5 + 2.
	if want := int32(6+2) + 2*(4+2) + (5 + 2); dr.GetHitPointsMax() != want {
		t.Errorf("hit points = %d, want %d (the first class's full die, the others' averages)", dr.GetHitPointsMax(), want)
	}

	// A prerequisite that is not met refuses a player (SRD 5.1 "Multiclassing"): the
	// Cleric asks for Wisdom 13 and Corvina has 10. The master's NPC goes past it,
	// with the sheet's multiclass_prerequisite issue.
	weak := proto.CloneOf(strong)
	weak.Wisdom = 10
	_, err = create(p2, "Corvina fraca", corvina(weak))
	if r := refusal(t, "CreateCharacter(a missing prerequisite)", err); r.GetReason() != charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_MULTICLASS_PREREQUISITE ||
		r.GetClassKey() != "class:cleric" || r.GetAbility() != rulesv1.Ability_ABILITY_WISDOM || r.GetMinimum() != 13 || r.GetHave() != 11 || r.GetField() != "sheet.full.classes[1].class_key" {
		t.Errorf("refusal = %v, want the Cleric's Wisdom 13 (Corvina has 11 with the human's +1) at classes[1]", r)
	}
	if c, err := master.api.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_BOSS, Name: "Corvina fraca",
		Sheet: &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: corvina(weak)}},
	})); err != nil || !slices.Contains(issueCodes(c.Msg.GetCharacter()), "multiclass_prerequisite") {
		t.Errorf("the master's NPC with a missing prerequisite: error %v; want the multiclass_prerequisite issue and a created sheet", err)
	}

	// A subclass chosen before its class level (the Mago picks at level 2) is the issue too.
	early := corvina(strong)
	early.Classes[0].Level = 1
	early.CantripKeys, early.KnownSpellKeys, early.PreparedSpellKeys = []string{"spell:fire-bolt", "spell:sacred-flame"}, []string{"spell:magic-missile"}, []string{"spell:magic-missile", "spell:cure-wounds"}
	if c, err := create(p3, "Corvina cedo", early); err != nil || !slices.Contains(issueCodes(c), "subclass_level") {
		t.Errorf("a subclass too early: issues %v, error %v; want the subclass_level issue", issueCodes(c), err)
	}

	// A table class with an SRD class: Guardião 2 / Mago 2 (half and full casters:
	// one plus two caster levels, the 3rd-level row of the table: 4 and 2 slots).
	mix := &charactersv1.FullSheet{
		BaseScores: &rulesv1.AbilityScores{Strength: 10, Dexterity: 13, Constitution: 14, Intelligence: 14, Wisdom: 15, Charisma: 8}, RaceKey: "race:human",
		Background:           &charactersv1.FullSheet_BackgroundKey{BackgroundKey: "background:acolyte"},
		Languages:            testLanguages,
		Classes:              []*charactersv1.ClassLevel{{ClassKey: guardian.GetKey(), Level: 2}, {ClassKey: "class:wizard", Level: 2}},
		SkillProficiencyKeys: []string{"skill:nature", "skill:perception"},
		FeatureChoiceKeys:    []string{"feature:fighter-fighting-style-defense"},
		CantripKeys:          []string{"spell:fire-bolt", "spell:mage-hand"},
		KnownSpellKeys:       []string{"spell:magic-missile", "spell:shield", "spell:sleep"},
		PreparedSpellKeys:    []string{"spell:magic-missile", "spell:cure-wounds"},
		HitPoints:            hitPointsAverage(), ExperiencePoints: 900,
	}
	c, err = create(p4, "Mista", mix)
	if err != nil {
		t.Fatalf("CreateCharacter(Guardião 2 / Mago 2) error = %v", err)
	}
	if got := c.GetDerived().GetSpellSlots(); len(c.GetDerived().GetIssues()) != 0 || len(got) < 2 || got[0].GetCount() != 4 || got[1].GetCount() != 2 {
		t.Errorf("Guardião 2 / Mago 2: issues %v, slots %v; want a clean sheet with 4 and 2 slots", c.GetDerived().GetIssues(), got)
	}

	// A third caster's Fighter with a Wizard: one third-caster level and two full ones.
	knight := &charactersv1.FullSheet{
		BaseScores: &rulesv1.AbilityScores{Strength: 14, Dexterity: 13, Constitution: 14, Intelligence: 14, Wisdom: 10, Charisma: 8}, RaceKey: "race:human",
		Background:           &charactersv1.FullSheet_BackgroundKey{BackgroundKey: "background:acolyte"},
		Languages:            testLanguages,
		Classes:              []*charactersv1.ClassLevel{ownedSubclass(&charactersv1.ClassLevel{ClassKey: "class:fighter", Level: 3}, ink.GetKey()), {ClassKey: "class:wizard", Level: 2}},
		SkillProficiencyKeys: []string{"skill:athletics", "skill:perception"},
		FeatureChoiceKeys:    []string{"feature:fighter-fighting-style-defense"},
		CantripKeys:          []string{"spell:fire-bolt", "spell:mage-hand"},
		KnownSpellKeys:       []string{"spell:magic-missile", "spell:shield", "spell:sleep"},
		PreparedSpellKeys:    []string{"spell:magic-missile"},
		HitPoints:            hitPointsAverage(), ExperiencePoints: 6500,
	}
	c, err = create(p5, "Cavaleiro mago", knight)
	if err != nil {
		t.Fatalf("CreateCharacter(Guerreiro 3 / Mago 2) error = %v", err)
	}
	if got := c.GetDerived().GetSpellSlots(); len(c.GetDerived().GetIssues()) != 0 || len(got) < 2 || got[0].GetCount() != 4 || got[1].GetCount() != 2 || len(c.GetDerived().GetSpellcasting()) != 2 {
		t.Errorf("Guerreiro 3 (Lâmina de Tinta) / Mago 2: issues %v, slots %v; want a clean sheet with 4 and 2 slots and two casters", c.GetDerived().GetIssues(), got)
	}
}

// TestTableClassMulticlassHitPointsFollowTheRule: a multiclass sheet made at
// creation counts every level after the first for the table's hit points rule
// (RN-24): with "roll" the rolls are as many as the levels after the first, and
// the average is refused; with "average", the rolls are.
func TestTableClassMulticlassHitPointsFollowTheRule(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, p1, p2, p3 := h.newUser("Samuel"), h.newUser("Rafa"), h.newUser("Lia"), h.newUser("Davi")
	campaign := h.newCampaign(master, "Mirathel", p1, p2, p3)
	d, menu := master.classDefaults(t, campaign), master.effectMenu(t, campaign)
	guardian := master.addEntry(t, campaign, guardianClass(t, d, menu))
	setRules(t, master, campaign, hitPointsRule(campaignsv1.HitPointsRule_HIT_POINTS_RULE_ROLL))

	sheet := func(hp *charactersv1.HitPoints) *charactersv1.CharacterSheet {
		return &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
			BaseScores: &rulesv1.AbilityScores{Strength: 10, Dexterity: 13, Constitution: 14, Intelligence: 14, Wisdom: 15, Charisma: 8}, RaceKey: "race:human",
			Background:           &charactersv1.FullSheet_BackgroundKey{BackgroundKey: "background:acolyte"},
			Languages:            testLanguages,
			Classes:              []*charactersv1.ClassLevel{{ClassKey: guardian.GetKey(), Level: 1}, {ClassKey: "class:wizard", Level: 2}},
			SkillProficiencyKeys: []string{"skill:nature", "skill:perception"},
			CantripKeys:          []string{"spell:fire-bolt", "spell:mage-hand"},
			HitPoints:            hp, ExperiencePoints: 900,
		}}}
	}
	create := func(u *user, hp *charactersv1.HitPoints) error {
		_, err := u.api.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
			CampaignId: campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Mista", Sheet: sheet(hp),
		}))
		return err
	}
	rolled := func(rolls ...int32) *charactersv1.HitPoints {
		return &charactersv1.HitPoints{Method: charactersv1.HitPointsMethod_HIT_POINTS_METHOD_ROLLED, Rolls: rolls}
	}
	for name, hp := range map[string]*charactersv1.HitPoints{"the average": hitPointsAverage(), "too few rolls": rolled(3)} {
		err := create(p1, hp)
		r := refusal(t, "CreateCharacter with "+name, err)
		if r.GetReason() != charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_HIT_POINTS_RULE || r.GetField() != "full.hit_points.method" {
			t.Errorf("%s: refusal = %v, want HIT_POINTS_RULE at full.hit_points.method", name, r)
		}
	}
	if err := create(p1, rolled(3, 4)); err != nil {
		t.Errorf("two rolls for two levels after the first: error = %v", err)
	}

	setRules(t, master, campaign, hitPointsRule(campaignsv1.HitPointsRule_HIT_POINTS_RULE_AVERAGE))
	if err := create(p2, rolled(3, 4)); err == nil {
		t.Error("rolls were accepted when the table's rule is the average")
	}
	if err := create(p2, hitPointsAverage()); err != nil {
		t.Errorf("the average under the average rule: error = %v", err)
	}
}

// testLanguages are the three languages a human with the acolyte background is owed (one of the race, two of the
// background), listed on the sheet so that no language pick is open.
var testLanguages = []string{"Anão", "Élfico", "Gnômico"}
