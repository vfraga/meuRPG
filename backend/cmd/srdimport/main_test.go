package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

func TestSlug(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"Shelter of the Faithful": "shelter-of-the-faithful",
		"Researcher":              "researcher",
		"  Two  Spaces! ":         "two-spaces",
	} {
		if got := slug(in); got != want {
			t.Errorf("slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSkillChoice(t *testing.T) {
	t.Parallel()
	var o optionSet
	raw := `{"choose": 2, "from": {"options": [
		{"option_type": "reference", "item": {"index": "skill-arcana"}},
		{"option_type": "reference", "item": {"index": "skill-history"}}]}}`
	if err := json.Unmarshal([]byte(raw), &o); err != nil {
		t.Fatal(err)
	}
	c, ok := skillChoice(&o)
	if !ok || c.Choose != 2 || strings.Join(c.From, ",") != "skill:arcana,skill:history" {
		t.Errorf("skillChoice = %+v, %v", c, ok)
	}

	// Instruments are not skills.
	raw = `{"choose": 3, "from": {"options": [{"option_type": "reference", "item": {"index": "lute"}}]}}`
	if err := json.Unmarshal([]byte(raw), &o); err != nil {
		t.Fatal(err)
	}
	if _, ok := skillChoice(&o); ok {
		t.Error("a choice of instruments is not a skill choice")
	}
}

// TestExpertisePicks: the rogue's first Expertise is "choose 1 of: 2
// skills, or 1 skill and thieves' tools", which is 2 picks.
func TestExpertisePicks(t *testing.T) {
	t.Parallel()
	var o optionSet
	raw := `{"choose": 1, "from": {"options": [
		{"option_type": "choice", "choice": {"choose": 2}},
		{"option_type": "multiple", "items": [{}, {}]}]}}`
	if err := json.Unmarshal([]byte(raw), &o); err != nil {
		t.Fatal(err)
	}
	if got := o.picks(); got != 2 {
		t.Errorf("picks = %d, want 2", got)
	}
}

// TestRefusesUnpinnedInput: a file that does not match inputHashes stops
// the import before anything is written.
func TestRefusesUnpinnedInput(t *testing.T) {
	t.Parallel()
	src, out := t.TempDir(), t.TempDir()
	for name := range inputHashes {
		if err := os.WriteFile(filepath.Join(src, name), []byte("[]"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	err := run(src, out)
	if err == nil || !strings.Contains(err.Error(), "sha256") {
		t.Fatalf("run = %v, want a sha256 mismatch", err)
	}
	if files, _ := os.ReadDir(out); len(files) != 0 {
		t.Errorf("wrote %d files after refusing the input", len(files))
	}
}

// TestRefusesMissingSource: -src must point at the files.
func TestRefusesMissingSource(t *testing.T) {
	t.Parallel()
	if err := run(filepath.Join(t.TempDir(), "nope"), t.TempDir()); err == nil {
		t.Error("run on a missing folder should fail")
	}
}

// TestDamageMod: the SRD writes a resistance as a list of types, sometimes
// with a rider.
func TestDamageMod(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"acid": "damage-type:acid|",
		"bludgeoning, piercing, and slashing from nonmagical weapons":          "damage-type:bludgeoning,damage-type:piercing,damage-type:slashing|from nonmagical weapons",
		"piercing and slashing from nonmagical weapons that aren't adamantine": "damage-type:piercing,damage-type:slashing|from nonmagical weapons that aren't adamantine",
		"damage from spells": "|damage from spells",
		"fire":               "damage-type:fire|",
	} {
		m := damageMod(in)
		if got := strings.Join(m.Types, ",") + "|" + m.Note; got != want {
			t.Errorf("damageMod(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestChallengeRating(t *testing.T) {
	t.Parallel()
	for in, want := range map[float64]string{0: "0", 0.125: "1/8", 0.25: "1/4", 0.5: "1/2", 1: "1", 30: "30"} {
		if got, err := challengeRating(in); err != nil || got != want {
			t.Errorf("challengeRating(%v) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []float64{0.3, -1, 31, 1.5} {
		if _, err := challengeRating(in); err == nil {
			t.Errorf("challengeRating(%v) accepted", in)
		}
	}
}

// TestConvertMonsterAction: the damage choice keeps the first option, a save
// that is only in the text is read from it, and a Multiattack keeps its
// routines.
func TestConvertMonsterAction(t *testing.T) {
	t.Parallel()
	decode := func(raw string) monsterActionSource {
		t.Helper()
		var a monsterActionSource
		if err := json.Unmarshal([]byte(raw), &a); err != nil {
			t.Fatal(err)
		}
		return a
	}

	longsword := convertMonsterAction(decode(`{"name":"Longsword","desc":"Melee Weapon Attack: +3 to hit.","attack_bonus":3,
		"damage":[{"choose":1,"type":"damage","from":{"option_set_type":"options_array","options":[
		{"option_type":"damage","damage_type":{"index":"slashing"},"damage_dice":"1d8+1","notes":"One handed"},
		{"option_type":"damage","damage_type":{"index":"slashing"},"damage_dice":"1d10+1","notes":"Two handed"}]}}]}`))
	if !longsword.HasAttack || longsword.AttackBonus != 3 || len(longsword.Damage) != 1 || longsword.Damage[0].Dice != "1d8+1" || longsword.Damage[0].DamageType != "damage-type:slashing" {
		t.Errorf("longsword = %+v", longsword)
	}

	bite := convertMonsterAction(decode(`{"name":"Bite","attack_bonus":0,"desc":"Hit: 7 piercing damage. It must succeed on a DC 11 Strength saving throw or be knocked prone."}`))
	if !bite.HasAttack || bite.AttackBonus != 0 || bite.Save == nil || bite.Save.Ability != "str" || bite.Save.DC != 11 || bite.Save.OnSuccess != "none" {
		t.Errorf("bite = %+v, want a +0 attack with a Strength DC 11 save", bite)
	}
	breath := convertMonsterAction(decode(`{"name":"Acid Breath","desc":"x","usage":{"type":"recharge on roll","dice":"1d6","min_value":5},
		"dc":{"dc_type":{"index":"dex"},"dc_value":18,"success_type":"half"}}`))
	if breath.HasAttack || breath.Save == nil || breath.Save.DC != 18 || breath.Save.OnSuccess != "half" || breath.Usage != "Recharge 5-6" {
		t.Errorf("breath = %+v", breath)
	}

	multi := convertMonsterAction(decode(`{"name":"Multiattack","multiattack_type":"action_options","desc":"x","action_options":{"choose":1,"from":{"options":[
		{"option_type":"multiple","items":[{"action_name":"Scimitar","count":"2","type":"melee"},{"action_name":"Dagger","count":"1","type":"melee"}]},
		{"option_type":"action","action_name":"Dagger","count":"2","type":"ranged"}]}}}`))
	if len(multi.Multiattack) != 2 || len(multi.Multiattack[0]) != 2 || multi.Multiattack[0][0].Count != 2 || multi.Multiattack[1][0].Kind != "ranged" {
		t.Errorf("multiattack = %+v", multi.Multiattack)
	}
	fixed := convertMonsterAction(decode(`{"name":"Multiattack","multiattack_type":"actions","desc":"x","actions":[{"action_name":"Bite","count":"1","type":"melee"},{"action_name":"Claw","count":"2","type":"melee"}]}`))
	if len(fixed.Multiattack) != 1 || len(fixed.Multiattack[0]) != 2 || fixed.Multiattack[0][1].Name != "Claw" {
		t.Errorf("fixed multiattack = %+v", fixed.Multiattack)
	}
}

// TestMonstersAreInTheInputs: the monsters file is pinned like every other.
func TestMonstersAreInTheInputs(t *testing.T) {
	t.Parallel()
	if got := inputHashes["5e-SRD-Monsters.json"]; got != "51edf634e1a9abefa259895b0799e22a788606df56f56e03fe24e130c76c2000" {
		t.Errorf("5e-SRD-Monsters.json is pinned to %q", got)
	}
	if leadingInt("60 ft.") != 60 || leadingInt("120 ft. (blind beyond this radius)") != 120 || leadingInt("") != 0 {
		t.Error("leadingInt")
	}
}

// TestConvertMonsterRows: crafted stat blocks through the whole conversion.
func TestConvertMonsterRows(t *testing.T) {
	t.Parallel()
	const base = `"index":"x","name":"X","size":"Small","type":"beast","alignment":"unaligned","hit_points":7,"hit_dice":"2d6","hit_points_roll":"2d6",
		"strength":8,"dexterity":14,"constitution":10,"intelligence":10,"wisdom":8,"charisma":8,"challenge_rating":0.25,"proficiency_bonus":2,"xp":50,
		"languages":"","damage_vulnerabilities":[],"damage_resistances":[],"damage_immunities":[],"condition_immunities":[]`
	parse := func(extra string) ([]srd51.Monster, error) {
		t.Helper()
		var rows []monstersSource
		if err := json.Unmarshal([]byte("[{"+base+","+extra+"}]"), &rows); err != nil {
			t.Fatal(err)
		}
		return convertMonsterRows(rows)
	}
	one := func(extra string) srd51.Monster {
		t.Helper()
		out, err := parse(extra)
		if err != nil || len(out) != 1 {
			t.Fatalf("convert: %v", err)
		}
		return out[0]
	}

	m := one(`"armor_class":[{"type":"dex","value":12}],"speed":{"walk":"30 ft.","fly":"60 ft.","hover":true,"burrow":"10 ft."},
		"senses":{"darkvision":"60 ft.","tremorsense":"30 ft. (blind beyond this radius)","passive_perception":9},
		"proficiencies":[{"value":4,"proficiency":{"index":"skill-stealth"}},{"value":3,"proficiency":{"index":"saving-throw-dex"}}]`)
	if m.Speed != (srd51.MonsterSpeed{Walk: 30, Fly: 60, Burrow: 10, Hover: true}) {
		t.Errorf("speed = %+v", m.Speed)
	}
	if m.Darkvision != 60 || m.Tremorsense != 30 || m.PassivePerception != 9 || m.Blindsight != 0 {
		t.Errorf("senses = %d %d %d", m.Darkvision, m.Tremorsense, m.PassivePerception)
	}
	if m.Skills["skill:stealth"] != 4 || m.Saves["dex"] != 3 || m.ChallengeRating != "1/4" {
		t.Errorf("proficiencies = %v %v, CR %s", m.Skills, m.Saves, m.ChallengeRating)
	}

	// Armor class: the armor entry over the natural one; a spell is an alternate.
	m = one(`"armor_class":[{"type":"natural","value":15},{"type":"armor","value":17,"armor":[{"index":"shield"}]}],"speed":{},"senses":{"passive_perception":9}`)
	if m.ArmorClass != 17 || m.ArmorClassType != "armor" || len(m.ArmorClassItems) != 1 || m.ArmorClassItems[0] != "equipment:shield" || len(m.ArmorClassAlts) != 0 {
		t.Errorf("azer-like AC = %d %s %v %v", m.ArmorClass, m.ArmorClassType, m.ArmorClassItems, m.ArmorClassAlts)
	}
	m = one(`"armor_class":[{"type":"dex","value":12},{"type":"spell","value":15,"spell":{"index":"mage-armor"}}],"speed":{},"senses":{"passive_perception":9}`)
	if m.ArmorClass != 12 || len(m.ArmorClassAlts) != 1 || m.ArmorClassAlts[0] != (srd51.MonsterACAlt{Value: 15, Spell: "spell:mage-armor"}) {
		t.Errorf("mage-like AC = %d %v", m.ArmorClass, m.ArmorClassAlts)
	}

	// Counts that are not numbers.
	m = one(`"armor_class":[{"type":"dex","value":12}],"speed":{},"senses":{"passive_perception":9},"actions":[
		{"name":"Bite","desc":"Melee Weapon Attack","attack_bonus":4},
		{"name":"Multiattack","desc":"x","multiattack_type":"actions","actions":[
		{"action_name":"Bite","count":"Number of Heads","type":"melee"},{"action_name":"Bites","count":"1d4","type":"melee"},{"action_name":"Bite","count":"2","type":"melee"}]}]`)
	r := m.Actions[1].Multiattack[0]
	if r[0].Count != 5 || r[0].Text != "Number of Heads" || r[1].Count != 1 || r[1].Text != "1d4" || r[1].Name != "Bite" || r[2].Count != 2 || r[2].Text != "" {
		t.Errorf("multiattack = %+v", r)
	}

	// Unknown things are refused.
	for name, extra := range map[string]string{
		"an unknown speed":         `"armor_class":[{"type":"dex","value":12}],"speed":{"teleport":"30 ft."},"senses":{"passive_perception":9}`,
		"an unknown sense":         `"armor_class":[{"type":"dex","value":12}],"speed":{},"senses":{"x-ray":"30 ft."}`,
		"an unknown proficiency":   `"armor_class":[{"type":"dex","value":12}],"speed":{},"senses":{},"proficiencies":[{"value":1,"proficiency":{"index":"tool-x"}}]`,
		"an unknown armor class":   `"armor_class":[{"type":"magic","value":12}],"speed":{},"senses":{}`,
		"no armor class":           `"armor_class":[],"speed":{},"senses":{}`,
		"a spell armor class only": `"armor_class":[{"type":"spell","value":15,"spell":{"index":"mage-armor"}}],"speed":{},"senses":{}`,
	} {
		if _, err := parse(extra); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// TestTextSaves: what a success does, read from the action's text.
func TestTextSaves(t *testing.T) {
	t.Parallel()
	for text, want := range map[string]string{
		"It must succeed on a DC 11 Strength saving throw or be knocked prone.":                                                     "none",
		"DC 13 Constitution saving throw, taking 9 poison damage, or half as much damage on a successful one.":                      "half",
		"must make a DC 13 Strength saving throw. If the saving throw is successful, the target takes half the bludgeoning damage.": "half",
		"must make a DC 13 Strength saving throw. Something odd happens.":                                                           "other",
	} {
		if got := textSuccess(text); got != want {
			t.Errorf("textSuccess(%q) = %q, want %q", text, got, want)
		}
	}
	a := convertMonsterAction(monsterActionSource{Name: "Deadly Leap", Desc: "must succeed on a DC 16 Strength or Dexterity saving throw (target's choice) or be knocked prone. On a successful save, the creature takes only half the damage."})
	if a.Save == nil || a.Save.Ability != "str" || a.Save.DC != 16 || a.Save.OnSuccess != "half" {
		t.Errorf("two-ability save = %+v", a.Save)
	}
}

// TestSpellAreas: the 5e-database's structured area_of_effect is read as the
// shape and the size in feet, and anything the engine does not know is refused.
func TestSpellAreas(t *testing.T) {
	t.Parallel()
	convert := func(extra string) ([]srd51.Spell, error) {
		row := `[{"index":"x","name":"X","desc":["d"],"range":"Self","components":["V"],"ritual":false,"duration":"Instantaneous","concentration":false,"casting_time":"1 action","level":1,"school":{"index":"evocation"},"classes":[]` + extra + `}]`
		out, err := convertSpells(&inputs{raw: map[string][]byte{"5e-SRD-Spells.json": []byte(row)}})
		if err != nil {
			return nil, err
		}
		return out.data.([]srd51.Spell), nil
	}
	got, err := convert(`,"area_of_effect":{"type":"cone","size":15}`)
	if err != nil || len(got) != 1 || got[0].AreaType != "cone" || got[0].AreaSizeFt != 15 {
		t.Fatalf("cone of 15 ft = %+v, %v", got, err)
	}
	if got, err = convert(``); err != nil || got[0].AreaType != "" || got[0].AreaSizeFt != 0 {
		t.Errorf("a spell without an area = %+v, %v", got, err)
	}
	for name, extra := range map[string]string{
		"an unknown shape":    `,"area_of_effect":{"type":"square","size":15}`,
		"a size of 0":         `,"area_of_effect":{"type":"cone","size":0}`,
		"a size off the grid": `,"area_of_effect":{"type":"sphere","size":12}`,
	} {
		if _, err := convert(extra); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// A creature's XP is the challenge rating's, whatever the source says; a challenge rating 0
// creature may give 0.
func TestMonsterXPComesFromTheChallengeRating(t *testing.T) {
	for _, c := range []struct {
		cr     string
		source int
		want   int
	}{{"1/4", 25, 50}, {"1", 100, 200}, {"1/2", 50, 100}, {"5", 1800, 1800}, {"0", 0, 0}, {"0", 10, 10}} {
		got, err := monsterXP(c.cr, c.source)
		if err != nil || got != c.want {
			t.Errorf("monsterXP(%q, %d) = %d, %v, want %d", c.cr, c.source, got, err, c.want)
		}
	}
	if _, err := monsterXP("31", 0); err == nil {
		t.Error("monsterXP(31) accepted a challenge rating the table lacks")
	}
}
