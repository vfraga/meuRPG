package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// These tests freeze what the importer writes, so its converters can be split
// without changing one byte of the snapshot. Two layers:
//
//   - TestSrdImportReproducesTheSnapshot runs the whole tool over the pinned
//     5e-database files and compares every output with the committed
//     internal/rules/srd51/data, byte for byte. The source is not in the
//     repository: fetch it as the comment at the top of main.go says and run
//     with SRDIMPORT_SRC=<the 2014/en folder>. Without it the test is skipped.
//   - The other tests feed crafted rows that reach every branch of the big
//     converters (CI runs them) and compare with goldens in
//     testdata/characterization; refusals are checked by their exact message.
//
// Run with -update-srdimport-golden to write the goldens again, and read the diff.
var updateSrdImportGolden = flag.Bool("update-srdimport-golden", false, "write testdata/characterization again")

// srdImportSnapshotDir is the committed snapshot, relative to this package.
const srdImportSnapshotDir = "../../internal/rules/srd51/data"

func TestSrdImportReproducesTheSnapshot(t *testing.T) {
	t.Parallel()
	src := os.Getenv("SRDIMPORT_SRC")
	if src == "" {
		t.Skip("SRDIMPORT_SRC is not set: the pinned 5e-database files are needed (see main.go)")
	}
	out := t.TempDir()
	if err := run(src, out); err != nil {
		t.Fatal(err)
	}
	got, err := filepath.Glob(filepath.Join(out, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.Glob(filepath.Join(srdImportSnapshotDir, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	names := func(paths []string) []string {
		var n []string
		for _, p := range paths {
			n = append(n, filepath.Base(p))
		}
		return n
	}
	if !slices.Equal(names(got), names(want)) {
		t.Fatalf("files = %v, the snapshot has %v", names(got), names(want))
	}
	for _, name := range names(want) {
		g, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		w, err := os.ReadFile(filepath.Join(srdImportSnapshotDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(g, w) {
			t.Errorf("%s differs from the committed snapshot", name)
		}
	}
}

// srdImportGolden compares v, encoded as the tool writes it, with a golden file.
func srdImportGolden(t *testing.T, name string, v any) {
	t.Helper()
	got, err := encode(v)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("testdata", "characterization", name)
	if *updateSrdImportGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the golden (-update-srdimport-golden writes it): %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from the golden:\n%s", name, got)
	}
}

// srdImportInputs builds the inputs of a converter from files given inline.
func srdImportInputs(files map[string]string) *inputs {
	in := &inputs{raw: map[string][]byte{}}
	for name, body := range files {
		in.raw[name] = []byte(body)
	}
	return in
}

// wantSrdImportRefusal checks that a converter refuses with exactly msg.
func wantSrdImportRefusal(t *testing.T, name string, err error, msg string) {
	t.Helper()
	if err == nil {
		t.Errorf("%s: accepted, want %q", name, msg)
		return
	}
	if err.Error() != msg {
		t.Errorf("%s: error %q, want %q", name, err.Error(), msg)
	}
}

const srdImportSkillChoice = `{"choose": 2, "type": "proficiencies", "from": {"options": [
	{"option_type": "reference", "item": {"index": "skill-arcana"}},
	{"option_type": "reference", "item": {"index": "skill-history"}}]}}`

const srdImportOtherSkillChoice = `{"choose": 1, "type": "proficiencies", "from": {"options": [
	{"option_type": "reference", "item": {"index": "skill-stealth"}}]}}`

const srdImportToolChoice = `{"choose": 1, "type": "proficiencies", "from": {"options": [
	{"option_type": "reference", "item": {"index": "lute"}}]}}`

// srdImportLevels gives sub-a1 the levels 3 and 5, sub-a2 level 2 and sub-b level 6.
const srdImportLevels = `[
	{"level": 1, "class": {"index": "a"}},
	{"level": 5, "class": {"index": "a"}, "subclass": {"index": "sub-a1"}},
	{"level": 3, "class": {"index": "a"}, "subclass": {"index": "sub-a1"}},
	{"level": 2, "class": {"index": "a"}, "subclass": {"index": "sub-a2"}},
	{"level": 6, "class": {"index": "b"}, "subclass": {"index": "sub-b"}}]`

// TestSrdImportClassesCharacterization: the saving throws are not proficiencies,
// the first skill choice wins for the class and the last one for multiclassing,
// both kinds of prerequisite, spellcasting, and the subclass level is the lowest
// known one (a subclass without levels only counts while nothing is known yet).
func TestSrdImportClassesCharacterization(t *testing.T) {
	t.Parallel()
	classes := `[
	{"index": "c", "name": "C", "hit_die": 8, "proficiency_choices": [` + srdImportSkillChoice + `], "proficiencies": [], "saving_throws": [], "multi_classing": {}, "subclasses": []},
	{"index": "a", "name": "A", "hit_die": 10,
		"proficiency_choices": [` + srdImportToolChoice + `, ` + srdImportSkillChoice + `, ` + srdImportOtherSkillChoice + `],
		"proficiencies": [{"index": "saving-throw-str"}, {"index": "light-armor"}, {"index": "shields"}],
		"saving_throws": [{"index": "str"}, {"index": "con"}],
		"multi_classing": {
			"prerequisites": [{"ability_score": {"index": "str"}, "minimum_score": 13}, {"ability_score": {"index": "cha"}, "minimum_score": 13}],
			"prerequisite_options": {"from": {"options": [{"ability_score": {"index": "str"}, "minimum_score": 13}, {"ability_score": {"index": "dex"}, "minimum_score": 13}]}},
			"proficiencies": [{"index": "light-armor"}],
			"proficiency_choices": [` + srdImportSkillChoice + `, ` + srdImportToolChoice + `, ` + srdImportOtherSkillChoice + `]},
		"subclasses": [{"index": "sub-a1"}, {"index": "sub-a2"}, {"index": "sub-none"}],
		"spellcasting": {"level": 1, "spellcasting_ability": {"index": "cha"}}},
	{"index": "b", "name": "B", "hit_die": 6, "proficiency_choices": [` + srdImportSkillChoice + `], "proficiencies": [], "saving_throws": [],
		"multi_classing": {"prerequisite_options": {"from": {"options": []}}},
		"subclasses": [{"index": "sub-none"}, {"index": "sub-b"}]}]`
	out, err := convertClasses(srdImportInputs(map[string]string{"5e-SRD-Classes.json": classes, "5e-SRD-Levels.json": srdImportLevels}))
	if err != nil {
		t.Fatal(err)
	}
	if out.name != "classes.json" {
		t.Errorf("name = %q", out.name)
	}
	srdImportGolden(t, "classes.json", out.data)

	noSkill := `[{"index": "x", "name": "X", "proficiency_choices": [` + srdImportToolChoice + `], "multi_classing": {}}]`
	_, err = convertClasses(srdImportInputs(map[string]string{"5e-SRD-Classes.json": noSkill, "5e-SRD-Levels.json": srdImportLevels}))
	wantSrdImportRefusal(t, "no skill choice", err, "class x: no skill choice")
	_, err = convertClasses(srdImportInputs(map[string]string{"5e-SRD-Classes.json": `{`, "5e-SRD-Levels.json": srdImportLevels}))
	wantSrdImportRefusal(t, "bad classes", err, "5e-SRD-Classes.json: unexpected EOF")
	_, err = convertClasses(srdImportInputs(map[string]string{"5e-SRD-Classes.json": noSkill, "5e-SRD-Levels.json": `{`}))
	wantSrdImportRefusal(t, "bad levels", err, "5e-SRD-Levels.json: unexpected EOF")
}

// TestSrdImportSubclassesCharacterization: a spell's prerequisites are class
// levels ("cleric-3") and features; anything else is refused.
func TestSrdImportSubclassesCharacterization(t *testing.T) {
	t.Parallel()
	rows := `[
	{"index": "life", "name": "Life", "class": {"index": "cleric"}, "subclass_flavor": "Divine Domain", "desc": ["Text."], "spells": [
		{"prerequisites": [{"index": "cleric-3", "type": "level"}], "spell": {"index": "bless"}},
		{"prerequisites": [{"index": "cleric-5", "type": "level"}, {"index": "channel", "type": "feature"}, {"index": "other", "type": "feature"}], "spell": {"index": "revivify"}},
		{"prerequisites": [], "spell": {"index": "light"}}]},
	{"index": "champion", "name": "Champion", "class": {"index": "fighter"}, "subclass_flavor": "Martial Archetype", "desc": ["Text."]}]`
	out, err := convertSubclasses(srdImportInputs(map[string]string{"5e-SRD-Subclasses.json": rows}))
	if err != nil {
		t.Fatal(err)
	}
	if out.name != "subclasses.json" {
		t.Errorf("name = %q", out.name)
	}
	srdImportGolden(t, "subclasses.json", out.data)

	for name, c := range map[string]struct{ rows, msg string }{
		"a level that is not a number": {
			`[{"index": "s", "class": {"index": "cleric"}, "spells": [{"prerequisites": [{"index": "cleric-x", "type": "level"}], "spell": {"index": "bless"}}]}]`,
			`subclass s: prerequisite "cleric-x": expected integer`,
		},
		"an unknown prerequisite": {
			`[{"index": "s", "class": {"index": "cleric"}, "spells": [{"prerequisites": [{"index": "cleric-3", "type": "level"}, {"index": "x", "type": "race"}], "spell": {"index": "bless"}}]}]`,
			`subclass s: unknown prerequisite type "race"`,
		},
		"bad json": {`{`, "5e-SRD-Subclasses.json: unexpected EOF"},
	} {
		_, err := convertSubclasses(srdImportInputs(map[string]string{"5e-SRD-Subclasses.json": c.rows}))
		wantSrdImportRefusal(t, name, err, c.msg)
	}
}

// TestSrdImportFeaturesCharacterization: subclass, parent, the three kinds of
// feature_specific, and the invocations replacing subfeature options when a
// feature has both.
func TestSrdImportFeaturesCharacterization(t *testing.T) {
	t.Parallel()
	rows := `[
	{"index": "plain", "name": "Plain", "class": {"index": "fighter"}, "level": 1, "desc": ["Text."]},
	{"index": "sub", "name": "Sub", "class": {"index": "cleric"}, "subclass": {"index": "life"}, "level": 2, "desc": ["Text."], "parent": {"index": "plain"}},
	{"index": "style", "name": "Style", "class": {"index": "fighter"}, "level": 1, "desc": [], "feature_specific": {"subfeature_options": {"choose": 1, "from": {"options": [
		{"option_type": "reference", "item": {"index": "style-archery"}}, {"option_type": "choice", "choice": {"choose": 2}}, {"option_type": "reference", "item": {"index": "style-defense"}}]}}}},
	{"index": "invocations", "name": "Invocations", "class": {"index": "warlock"}, "level": 2, "desc": [], "feature_specific": {"invocations": [{"index": "agonizing"}, {"index": "devils-sight"}]}},
	{"index": "both", "name": "Both", "class": {"index": "warlock"}, "level": 2, "desc": [], "feature_specific": {
		"subfeature_options": {"choose": 3, "from": {"options": [{"option_type": "reference", "item": {"index": "dropped"}}]}},
		"invocations": [{"index": "kept"}]}},
	{"index": "expertise", "name": "Expertise", "class": {"index": "rogue"}, "level": 1, "desc": [], "feature_specific": {"expertise_options": {"choose": 1, "from": {"options": [
		{"option_type": "choice", "choice": {"choose": 2}}, {"option_type": "multiple", "items": [{}, {}, {}]}]}}}},
	{"index": "other", "name": "Other", "class": {"index": "rogue"}, "level": 3, "desc": [], "feature_specific": {"something_else": 1}}]`
	out, err := convertFeatures(srdImportInputs(map[string]string{"5e-SRD-Features.json": rows}))
	if err != nil {
		t.Fatal(err)
	}
	if out.name != "features.json" {
		t.Errorf("name = %q", out.name)
	}
	srdImportGolden(t, "features.json", out.data)

	for name, c := range map[string]struct{ rows, msg string }{
		"bad subfeature options": {
			`[{"index": "f", "feature_specific": {"subfeature_options": "x"}}]`,
			"feature f: json: cannot unmarshal string into Go value of type main.optionSet",
		},
		"bad invocations": {
			`[{"index": "f", "feature_specific": {"invocations": {}}}]`,
			"feature f: json: cannot unmarshal object into Go value of type []main.ref",
		},
		"bad expertise options": {
			`[{"index": "f", "feature_specific": {"expertise_options": 3}}]`,
			"feature f: json: cannot unmarshal number into Go value of type main.optionSet",
		},
		"bad json": {`{`, "5e-SRD-Features.json: unexpected EOF"},
	} {
		_, err := convertFeatures(srdImportInputs(map[string]string{"5e-SRD-Features.json": c.rows}))
		wantSrdImportRefusal(t, name, err, c.msg)
	}
}

// TestSrdImportTraitsCharacterization: every optional part of a trait, and a
// trait_specific without subtrait options.
func TestSrdImportTraitsCharacterization(t *testing.T) {
	t.Parallel()
	rows := `[
	{"index": "plain", "name": "Plain", "races": [{"index": "elf"}], "subraces": [], "desc": ["Text."], "proficiencies": []},
	{"index": "full", "name": "Full", "races": [{"index": "dwarf"}], "subraces": [{"index": "hill-dwarf"}], "desc": ["Text."],
		"proficiencies": [{"index": "battleaxes"}],
		"proficiency_choices": {"choose": 1, "from": {"options": [{"option_type": "reference", "item": {"index": "smiths-tools"}}, {"option_type": "choice", "choice": {"choose": 1}}]}},
		"language_options": {"choose": 2},
		"parent": {"index": "plain"},
		"trait_specific": {"subtrait_options": {"choose": 1, "from": {"options": [{"option_type": "reference", "item": {"index": "draconic-black"}}]}}}},
	{"index": "specific", "name": "Specific", "races": [], "subraces": [], "desc": [], "proficiencies": [], "trait_specific": {"damage_type": {"index": "fire"}}}]`
	out, err := convertTraits(srdImportInputs(map[string]string{"5e-SRD-Traits.json": rows}))
	if err != nil {
		t.Fatal(err)
	}
	if out.name != "traits.json" {
		t.Errorf("name = %q", out.name)
	}
	srdImportGolden(t, "traits.json", out.data)
	_, err = convertTraits(srdImportInputs(map[string]string{"5e-SRD-Traits.json": `{`}))
	wantSrdImportRefusal(t, "bad json", err, "5e-SRD-Traits.json: unexpected EOF")
}

// TestSrdImportEquipmentCharacterization: armor, weapons with and without each
// part (the reach property makes a melee weapon's range 10, not a ranged one's),
// tools, and the rest left out.
func TestSrdImportEquipmentCharacterization(t *testing.T) {
	t.Parallel()
	rows := `[
	{"index": "chain-mail", "name": "Chain Mail", "equipment_category": {"index": "armor"}, "armor_category": "Heavy",
		"armor_class": {"base": 16, "dex_bonus": false}, "str_minimum": 13, "stealth_disadvantage": true},
	{"index": "studded", "name": "Studded", "equipment_category": {"index": "armor"}, "armor_category": "Light", "armor_class": {"base": 12, "dex_bonus": true, "max_bonus": 0}},
	{"index": "longsword", "name": "Longsword", "equipment_category": {"index": "weapon"}, "weapon_category": "Martial", "weapon_range": "Melee",
		"damage": {"damage_dice": "1d8", "damage_type": {"index": "slashing"}}, "two_handed_damage": {"damage_dice": "1d10", "damage_type": {"index": "slashing"}},
		"range": {"normal": 5}, "properties": [{"index": "versatile"}]},
	{"index": "glaive", "name": "Glaive", "equipment_category": {"index": "weapon"}, "weapon_category": "Martial", "weapon_range": "Melee",
		"damage": {"damage_dice": "1d10", "damage_type": {"index": "slashing"}}, "range": {"normal": 5}, "properties": [{"index": "heavy"}, {"index": "reach"}]},
	{"index": "whip-ranged", "name": "Odd", "equipment_category": {"index": "weapon"}, "weapon_category": "Simple", "weapon_range": "Ranged",
		"range": {"normal": 30, "long": 120}, "properties": [{"index": "reach"}]},
	{"index": "net", "name": "Net", "equipment_category": {"index": "weapon"}, "weapon_category": "Martial", "weapon_range": "Ranged", "properties": []},
	{"index": "dagger", "name": "Dagger", "equipment_category": {"index": "weapon"}, "weapon_category": "Simple", "weapon_range": "Melee",
		"damage": {"damage_dice": "1d4", "damage_type": {"index": "piercing"}}, "range": {"normal": 5}, "throw_range": {"normal": 20, "long": 60}, "properties": [{"index": "thrown"}]},
	{"index": "lute", "name": "Lute", "equipment_category": {"index": "tools"}},
	{"index": "rope", "name": "Rope", "equipment_category": {"index": "adventuring-gear"}}]`
	out, err := convertEquipment(srdImportInputs(map[string]string{"5e-SRD-Equipment.json": rows}))
	if err != nil {
		t.Fatal(err)
	}
	if out.name != "equipment.json" {
		t.Errorf("name = %q", out.name)
	}
	srdImportGolden(t, "equipment.json", out.data)

	_, err = convertEquipment(srdImportInputs(map[string]string{"5e-SRD-Equipment.json": `[{"index": "x", "equipment_category": {"index": "armor"}}]`}))
	wantSrdImportRefusal(t, "armor without a class", err, "armor x: no armor_class")
	_, err = convertEquipment(srdImportInputs(map[string]string{"5e-SRD-Equipment.json": `{`}))
	wantSrdImportRefusal(t, "bad json", err, "5e-SRD-Equipment.json: unexpected EOF")
	out, err = convertEquipment(srdImportInputs(map[string]string{"5e-SRD-Equipment.json": `[{"index": "rope", "equipment_category": {"index": "adventuring-gear"}}]`}))
	if err != nil || out.data.([]srd51.Equipment) != nil {
		t.Errorf("nothing kept = %#v, %v (a nil list, written as null)", out.data, err)
	}
}

// srdImportMonsterBase is a valid stat block without armor class, speed and senses.
const srdImportMonsterBase = `"name":"X","size":"Medium","type":"humanoid","subtype":"any race","alignment":"any","hit_points":11,"hit_dice":"2d8","hit_points_roll":"2d8+2",
	"strength":10,"dexterity":12,"constitution":12,"intelligence":10,"wisdom":10,"charisma":10,"proficiency_bonus":2,"languages":"Common"`

func srdImportMonsterRows(t *testing.T, raw string) []monstersSource {
	t.Helper()
	var rows []monstersSource
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

// TestSrdImportMonstersCharacterization: every speed and sense, saves and
// skills, the three lists of abilities with and without usage, actions with
// sub-effects and every way a Multiattack name is matched.
func TestSrdImportMonstersCharacterization(t *testing.T) {
	t.Parallel()
	rows := srdImportMonsterRows(t, `[
	{"index":"full",`+srdImportMonsterBase+`,"challenge_rating":0,"xp":0,
		"armor_class":[{"type":"natural","value":13,"desc":"natural armor"},{"type":"condition","value":16,"condition":{"index":"x"}}],
		"speed":{"walk":"30 ft.","fly":"60 ft.","swim":"40 ft.","climb":"20 ft.","burrow":"10 ft.","hover":true},
		"senses":{"darkvision":"60 ft.","blindsight":"10 ft.","tremorsense":"30 ft.","truesight":"120 ft.","passive_perception":14},
		"proficiencies":[{"value":3,"proficiency":{"index":"saving-throw-dex"}},{"value":5,"proficiency":{"index":"skill-perception"}},
			{"value":4,"proficiency":{"index":"saving-throw-wis"}},{"value":6,"proficiency":{"index":"skill-stealth"}}],
		"damage_vulnerabilities":["fire"],"damage_resistances":["cold, lightning"],"damage_immunities":["poison"],"condition_immunities":[{"index":"poisoned"}],
		"special_abilities":[{"name":"Innate","desc":"Text.","usage":{"type":"per day","times":3}},{"name":"Keen","desc":"Text."}],
		"reactions":[{"name":"Parry","desc":"Text.","usage":{"type":"per day","times":1}}],
		"legendary_actions":[{"name":"Detect","desc":"Text."}],
		"actions":[
			{"name":"Multiattack","desc":"x","multiattack_type":"actions","actions":[
				{"action_name":"Bite","count":"1","type":"melee"},{"action_name":"Claws","count":"2","type":"melee"},
				{"action_name":"Tail (Dragon Form Only)","count":"1","type":"melee"},{"action_name":"Gore","count":"1","type":"melee"},
				{"action_name":"Spell","count":"1","type":"melee"}]},
			{"name":"Bite","desc":"Melee.","attack_bonus":4,"damage":[{"damage_type":{"index":"piercing"},"damage_dice":"1d6"}]},
			{"name":"Claw","desc":"Melee.","attack_bonus":4},
			{"name":"Tail","desc":"Melee.","attack_bonus":4},
			{"name":"Gore (Beast Form Only)","desc":"Melee.","attack_bonus":4},
			{"name":"Breath Weapons","desc":"It uses one.\nFire Breath. Fire.\nSleep Breath. Sleep.","usage":{"type":"recharge on roll","min_value":5},
				"options":{"from":{"options":[{"name":"Fire Breath","dc":{"dc_type":{"index":"dex"},"dc_value":13,"success_type":"half"},"damage":[{"damage_type":{"index":"fire"},"damage_dice":"4d6"}]},
					{"name":"Missing Breath","dc":{"dc_type":{"index":"con"},"dc_value":13,"success_type":"none"}}]}},
				"attacks":[{"name":"Sleep Breath","dc":{"dc_type":{"index":"con"},"dc_value":13,"success_type":"none"}}]}]},
	{"index":"bare",`+srdImportMonsterBase+`,"challenge_rating":0.5,"xp":100,"armor_class":[{"type":"armor","value":15,"armor":[{"index":"leather-armor"},{"index":"shield"}]},{"type":"spell","value":18,"spell":{"index":"shield"}}],
		"speed":{},"senses":{}}]`)
	out, err := convertMonsterRows(rows)
	if err != nil {
		t.Fatal(err)
	}
	srdImportGolden(t, "monsters.json", out)

	const ok = `"armor_class":[{"type":"dex","value":12}],"challenge_rating":1`
	for name, c := range map[string]struct{ extra, msg string }{
		"a challenge rating out of the table": {`"armor_class":[{"type":"dex","value":12}],"challenge_rating":31`, "monster m: challenge rating 31"},
		"no armor class":                      {`"armor_class":[],"challenge_rating":1`, "monster m: no armor class"},
		"a speed that is not text":            {ok + `,"speed":{"walk":30}`, "monster m: speed walk: json: cannot unmarshal number into Go value of type string"},
		"an unknown speed":                    {ok + `,"speed":{"teleport":"30 ft."}`, `monster m: unknown speed "teleport"`},
		"an unknown sense":                    {ok + `,"senses":{"x-ray":"30 ft."}`, `monster m: unknown sense "x-ray"`},
		"an unknown proficiency":              {ok + `,"proficiencies":[{"value":1,"proficiency":{"index":"tool-x"}}]`, `monster m: unknown proficiency "tool-x"`},
	} {
		_, err := convertMonsterRows(srdImportMonsterRows(t, `[{"index":"m",`+srdImportMonsterBase+`,`+c.extra+`}]`))
		wantSrdImportRefusal(t, name, err, c.msg)
	}
}

// TestSrdImportMagicItemRefusals: each refusal of the magic items, by its
// exact message, in the order the checks run.
func TestSrdImportMagicItemRefusals(t *testing.T) {
	t.Parallel()
	noDesc := magicRow("a", "Rare", false)
	noDesc.Desc = nil
	badCat := magicRow("a", "Rare", false)
	badCat.EquipmentCategory.Index = "gadget"
	for name, c := range map[string]struct {
		rows []magicItemsSource
		msg  string
	}{
		"an unknown category":        {[]magicItemsSource{badCat}, `magic item a: unknown category "gadget"`},
		"an unknown rarity":          {[]magicItemsSource{magicRow("a", "Mythic", false)}, `magic item a: unknown rarity "Mythic"`},
		"an empty description":       {[]magicItemsSource{noDesc}, "magic item a: no description"},
		"a repeated index":           {[]magicItemsSource{magicRow("a", "Rare", false), magicRow("a", "Rare", false)}, "magic item a: listed twice"},
		"varies alone":               {[]magicItemsSource{magicRow("a", "Varies", false)}, "magic item a: rarity varies but it has no variants"},
		"a variant with variants":    {[]magicItemsSource{magicRow("v", "Rare", true, "w"), magicRow("w", "Rare", true)}, "magic item v: a variant with variants"},
		"a family item without kids": {[]magicItemsSource{magicRow("crystal-ball", "Very Rare", false)}, "magic item crystal-ball: listed as a family that is also an item, but it is not a family with a rarity"},
		"a family item that varies": {
			[]magicItemsSource{magicRow("crystal-ball", "Varies", false, "v"), magicRow("v", "Rare", true)},
			"magic item crystal-ball: listed as a family that is also an item, but it is not a family with a rarity",
		},
		"a missing variant":     {[]magicItemsSource{magicRow("b", "Varies", false, "b-1")}, "magic item item:b: variant item:b-1 does not exist"},
		"not a variant":         {[]magicItemsSource{magicRow("f", "Rare", false, "v"), magicRow("v", "Rare", false)}, "magic item item:f: item:v is listed as a variant but is not one"},
		"two families":          {[]magicItemsSource{magicRow("a", "Rare", false, "v"), magicRow("b", "Rare", false, "v"), magicRow("v", "Rare", true)}, "magic item item:v: in two families, item:a and item:b"},
		"a variant that varies": {[]magicItemsSource{magicRow("f", "Rare", false, "v"), magicRow("v", "Varies", true)}, "magic item v: rarity varies but it has no variants"},
		"a variant of nobody":   {[]magicItemsSource{magicRow("b-1", "Rare", true)}, "magic item item:b-1: a variant of no family"},
	} {
		_, err := convertMagicItemRows(c.rows)
		wantSrdImportRefusal(t, name, err, c.msg)
	}
	_, err := convertMagicItems(srdImportInputs(map[string]string{"5e-SRD-Magic-Items.json": `[]`}))
	wantSrdImportRefusal(t, "a short list", err, "magic items: 0 entries, want 362 at the pinned commit")
	_, err = convertMagicItems(srdImportInputs(map[string]string{"5e-SRD-Magic-Items.json": `{`}))
	wantSrdImportRefusal(t, "bad json", err, "5e-SRD-Magic-Items.json: unexpected EOF")
}

// TestSrdImportMagicItemsCharacterization: a family with its variants, the
// Crystal Ball (a family that is an item too), attunement with and without a
// restriction, and a name with spaces around it.
func TestSrdImportMagicItemsCharacterization(t *testing.T) {
	t.Parallel()
	plain := magicRow("plain", "Common", false)
	plain.Name = "  Plain  "
	plain.Desc = []string{"Wondrous item, common", "Text."}
	plain.EquipmentCategory.Index = "wondrous-items"
	attuned := magicRow("attuned", "Rare", false)
	attuned.Desc = []string{"Ring, rare (requires attunement)"}
	out, err := convertMagicItemRows([]magicItemsSource{
		magicRow("fam-2", "Rare", true), magicRow("fam", "Varies", false, "fam-1", "fam-2"), magicRow("fam-1", "Uncommon", true),
		plain, attuned,
		magicRow("crystal-ball", "Very Rare", false, "crystal-ball-x"), magicRow("crystal-ball-x", "Legendary", true),
	})
	if err != nil {
		t.Fatal(err)
	}
	srdImportGolden(t, "magic-items.json", out)
}
