// Command srdimport turns the SRD 5.1 JSON of 5e-database into the snapshot
// that package rules embeds (backend/internal/rules/srd51/data).
//
// It never downloads anything. Fetch the source first, at the pinned commit,
// into a folder outside the repository, then point -src at it:
//
//	git clone --filter=blob:none --no-checkout https://github.com/5e-bits/5e-srd-api.git /tmp/5e-srd-api
//	git -C /tmp/5e-srd-api sparse-checkout set --no-cone packages/5e-database/src/2014/en
//	git -C /tmp/5e-srd-api checkout <sourceCommit>
//	cd backend && go run ./cmd/srdimport -src /tmp/5e-srd-api/packages/5e-database/src/2014/en
//
// Every input must match the sha256 in inputHashes, so the snapshot can only
// come from the reviewed commit. To move to a newer commit, change
// sourceCommit and inputHashes in the same PR, run the tool, and review the
// data diff (CONTRIBUTING.md, "Conteúdo de regras (SRD)").
//
// The output is our own normalized format (package srd51, schema.go): only
// the fields the engine uses, with stable keys such as "class:wizard".
package main

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// The source this snapshot comes from. 5e-bits/5e-database was archived on
// 23/09/2026 and moved into the 5e-srd-api monorepo (ADR-0008).
const (
	sourceRepo   = "https://github.com/5e-bits/5e-srd-api"
	sourcePath   = "packages/5e-database/src/2014/en"
	sourceCommit = "a8abc93b235c158bb8cbf042e54425b9c2fd79b8"
)

// reachWeaponFt is the reach of a melee weapon with the reach property.
const reachWeaponFt = 10

// inputHashes is the sha256 of every file the tool reads, at sourceCommit.
var inputHashes = map[string]string{
	"5e-SRD-Ability-Scores.json":    "23f08ea89e30f3d4e1cab80b3f57b61188b7894fbcab3b583177b66e9711e090",
	"5e-SRD-Skills.json":            "09f42a2d3e3d952fc2654387bf1a56dee7f02a326a68a8bc167bd562b0f1e0d9",
	"5e-SRD-Races.json":             "75e6e99c318162e229f825d79f1be0c06abe38f37d14eeea54ce32d94d051245",
	"5e-SRD-Subraces.json":          "13f6917a7f52a42ba48e7daf0464bcdf1c6ffb2efaf9a0ec8d8de523789dedb8",
	"5e-SRD-Traits.json":            "76943cc30d9d08ad3720524f81f4a1ea715439c3e59191644c4a207d4ae5fcd5",
	"5e-SRD-Classes.json":           "2c2795733d45db10758f1c62cad28ffdb1e44159d49a0669bd2dcd606ab6fcfc",
	"5e-SRD-Levels.json":            "ea6a58268f26f8536d049bd8cb45630280188cd07d2be8b46e0e4cbe5a44feeb",
	"5e-SRD-Subclasses.json":        "b3502390228ffd15c9043f6b5f25d365a9935e12056debccc2e2749e3d6c3ab1",
	"5e-SRD-Features.json":          "fd20a2a5c27996eb66c023053b1629d93dac2d1d108e5d81dfc53ddf0d6ad93c",
	"5e-SRD-Backgrounds.json":       "f98c556f1b842c2570058cd071b7c20743290358559a1f46140dbedf36f418aa",
	"5e-SRD-Proficiencies.json":     "60127a2130a5dd0a0a9b4cc20adf9061be81e9ad95218273d3b434c49d229333",
	"5e-SRD-Equipment.json":         "6dc4dc61ac71c9ae2ffffe8deae087f2dbe8f1a7b5a8675077adb5df649901c3",
	"5e-SRD-Spells.json":            "abebe7d860ec32985b804e087ecd3986145b1b772fa0c2e43cce9964b382f2ae",
	"5e-SRD-Languages.json":         "44f30bf49b9692f529e8cf61f20e71fd415f1f42b5a1b80f5c8c665ddae1e8c1",
	"5e-SRD-Damage-Types.json":      "0de4356c453c3e54b5935de528e2867f3ec7af50a03939184675caf33b6c55de",
	"5e-SRD-Magic-Schools.json":     "9901d0934c16941b871a360f1e866c7ad78679bad9126baf8a5bbd0dfee37daf",
	"5e-SRD-Weapon-Properties.json": "31604f16560b217549c2b629eef1377cde7a8d91a987cbdd4ad19db790b0d765",
	"5e-SRD-Conditions.json":        "e2c8d211a4f72722c3490217aea948e78c5235c2d51d3c08a4fd3e9d3cd4468f",
	"5e-SRD-Monsters.json":          "51edf634e1a9abefa259895b0799e22a788606df56f56e03fe24e130c76c2000",
	"5e-SRD-Magic-Items.json":       "9a6f928cbf36b268b02e09dc116995efce8a74380df0450e197117886b64993d",
}

func main() {
	src := flag.String("src", "", "folder with the 5e-database 2014/en JSON files, at the pinned commit (required)")
	out := flag.String("out", "internal/rules/srd51/data", "folder to write the snapshot into")
	flag.Parse()
	if *src == "" {
		fmt.Fprintln(os.Stderr, "srdimport: -src is required (see the comment at the top of main.go)")
		os.Exit(2)
	}
	if err := run(*src, *out); err != nil {
		fmt.Fprintln(os.Stderr, "srdimport:", err)
		os.Exit(1)
	}
}

func run(srcDir, outDir string) error {
	in, err := readInputs(srcDir)
	if err != nil {
		return err
	}
	files, err := convert(in)
	if err != nil {
		return err
	}
	return writeOutputs(outDir, files, in.hashes)
}

// inputs holds the raw bytes of every input file, already checked.
type inputs struct {
	raw    map[string][]byte
	hashes []srd51.FileHash
}

func readInputs(dir string) (*inputs, error) {
	in := &inputs{raw: map[string][]byte{}}
	names := make([]string, 0, len(inputHashes))
	for name := range inputHashes {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(b)
		got := hex.EncodeToString(sum[:])
		if got != inputHashes[name] {
			return nil, fmt.Errorf("%s: sha256 is %s, want %s: is the source at commit %s?", name, got, inputHashes[name], sourceCommit)
		}
		in.raw[name] = b
		in.hashes = append(in.hashes, srd51.FileHash{Name: name, SHA256: got})
	}
	return in, nil
}

func decode[T any](in *inputs, name string) ([]T, error) {
	var v []T
	dec := json.NewDecoder(bytes.NewReader(in.raw[name]))
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return v, nil
}

// output is one file of the snapshot, before it is written.
type output struct {
	name string
	data any
}

// convert builds every output file from the inputs.
func convert(in *inputs) ([]output, error) {
	var outs []output
	steps := []func(*inputs) (output, error){
		convertAbilities, convertSkills, convertRaces, convertSubraces, convertTraits,
		convertClasses, convertLevels, convertSubclasses, convertFeatures, convertBackgrounds,
		convertProficiencies, convertEquipment, convertSpells, convertLanguages, convertMonsters, convertMagicItems,
		named("5e-SRD-Damage-Types.json", "damage-types.json", "damage-type:"),
		named("5e-SRD-Magic-Schools.json", "magic-schools.json", "school:"),
		named("5e-SRD-Weapon-Properties.json", "weapon-properties.json", "weapon-property:"),
		named("5e-SRD-Conditions.json", "conditions.json", "condition:"),
	}
	for _, step := range steps {
		o, err := step(in)
		if err != nil {
			return nil, err
		}
		outs = append(outs, o)
	}
	return outs, nil
}

// ref is a 5e-database reference to another entry.
type ref struct {
	Index string `json:"index"`
	Name  string `json:"name"`
	URL   string `json:"url"`
}

func keys(prefix string, refs []ref) []string {
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, prefix+r.Index)
	}
	return out
}

// optionSet is 5e-database's "choose N from these options".
type optionSet struct {
	Choose int    `json:"choose"`
	Type   string `json:"type"`
	From   struct {
		Options []struct {
			OptionType string `json:"option_type"`
			Item       *ref   `json:"item"`
			// ability_bonus options
			AbilityScore *ref `json:"ability_score"`
			Bonus        int  `json:"bonus"`
			// Nested options: "choose N of these" and "all of these".
			Choice *struct {
				Choose int `json:"choose"`
			} `json:"choice"`
			Items []json.RawMessage `json:"items"`
		} `json:"options"`
	} `json:"from"`
}

// picks is how many things the set makes the player pick in the end. The
// rogue's first Expertise is "choose 1 of: (choose 2 skills) or (1 skill
// and thieves' tools)", which is 2 picks, not 1.
func (o *optionSet) picks() int {
	most := 1
	for _, opt := range o.From.Options {
		switch {
		case opt.Choice != nil:
			most = max(most, opt.Choice.Choose)
		case opt.OptionType == "multiple":
			most = max(most, len(opt.Items))
		}
	}
	return o.Choose * most
}

// referenced returns the indexes of the options that are plain references.
func (o *optionSet) referenced() []string {
	if o == nil {
		return nil
	}
	var out []string
	for _, opt := range o.From.Options {
		if opt.Item != nil {
			out = append(out, opt.Item.Index)
		}
	}
	return out
}

// skillChoice turns a proficiency choice of skills ("skill-arcana") into a
// Choice of skill keys. ok is false if any option is not a skill.
func skillChoice(o *optionSet) (srd51.Choice, bool) {
	if o == nil {
		return srd51.Choice{}, false
	}
	indexes := o.referenced()
	if len(indexes) == 0 || len(indexes) != len(o.From.Options) {
		return srd51.Choice{}, false
	}
	c := srd51.Choice{Choose: o.Choose}
	for _, idx := range indexes {
		skill, ok := strings.CutPrefix(idx, "skill-")
		if !ok {
			return srd51.Choice{}, false
		}
		c.From = append(c.From, "skill:"+skill)
	}
	return c, true
}

func abilityBonuses(list []struct {
	AbilityScore ref `json:"ability_score"`
	Bonus        int `json:"bonus"`
},
) map[string]int {
	out := map[string]int{}
	for _, b := range list {
		out[b.AbilityScore.Index] += b.Bonus
	}
	return out
}

func convertAbilities(in *inputs) (output, error) {
	type src struct {
		Index    string   `json:"index"`
		Name     string   `json:"name"`
		FullName string   `json:"full_name"`
		Desc     []string `json:"desc"`
	}
	rows, err := decode[src](in, "5e-SRD-Ability-Scores.json")
	if err != nil {
		return output{}, err
	}
	out := make([]srd51.AbilityScore, 0, len(rows))
	for _, r := range rows {
		out = append(out, srd51.AbilityScore{Key: r.Index, Name: r.Name, FullName: r.FullName, Desc: r.Desc})
	}
	// Keep the sheet's order (STR ... CHA), which is also the source's.
	return output{"abilities.json", out}, nil
}

func convertSkills(in *inputs) (output, error) {
	type src struct {
		Index        string   `json:"index"`
		Name         string   `json:"name"`
		Desc         []string `json:"desc"`
		AbilityScore ref      `json:"ability_score"`
	}
	rows, err := decode[src](in, "5e-SRD-Skills.json")
	if err != nil {
		return output{}, err
	}
	out := make([]srd51.Skill, 0, len(rows))
	for _, r := range rows {
		out = append(out, srd51.Skill{Key: "skill:" + r.Index, Name: r.Name, Ability: r.AbilityScore.Index, Desc: r.Desc})
	}
	sortByKey(out, func(s srd51.Skill) string { return s.Key })
	return output{"skills.json", out}, nil
}

func convertRaces(in *inputs) (output, error) {
	type src struct {
		Index          string `json:"index"`
		Name           string `json:"name"`
		Speed          int    `json:"speed"`
		Size           string `json:"size"`
		AbilityBonuses []struct {
			AbilityScore ref `json:"ability_score"`
			Bonus        int `json:"bonus"`
		} `json:"ability_bonuses"`
		AbilityBonusOptions *optionSet `json:"ability_bonus_options"`
		Languages           []ref      `json:"languages"`
		LanguageOptions     *optionSet `json:"language_options"`
		Traits              []ref      `json:"traits"`
		Subraces            []ref      `json:"subraces"`
	}
	rows, err := decode[src](in, "5e-SRD-Races.json")
	if err != nil {
		return output{}, err
	}
	out := make([]srd51.Race, 0, len(rows))
	for _, r := range rows {
		race := srd51.Race{
			Key:            "race:" + r.Index,
			Name:           r.Name,
			SpeedFt:        r.Speed,
			Size:           r.Size,
			AbilityBonuses: abilityBonuses(r.AbilityBonuses),
			Languages:      keys("language:", r.Languages),
			Traits:         keys("trait:", r.Traits),
			Subraces:       keys("subrace:", r.Subraces),
		}
		if o := r.AbilityBonusOptions; o != nil {
			c := &srd51.Choice{Choose: o.Choose}
			for _, opt := range o.From.Options {
				if opt.AbilityScore == nil || opt.Bonus != 1 {
					return output{}, fmt.Errorf("race %s: unexpected ability bonus option", r.Index)
				}
				c.From = append(c.From, opt.AbilityScore.Index)
			}
			race.AbilityBonusChoices = c
		}
		if r.LanguageOptions != nil {
			race.LanguageChoices = r.LanguageOptions.Choose
		}
		out = append(out, race)
	}
	sortByKey(out, func(r srd51.Race) string { return r.Key })
	return output{"races.json", out}, nil
}

func convertSubraces(in *inputs) (output, error) {
	type src struct {
		Index          string `json:"index"`
		Name           string `json:"name"`
		Race           ref    `json:"race"`
		Desc           string `json:"desc"`
		AbilityBonuses []struct {
			AbilityScore ref `json:"ability_score"`
			Bonus        int `json:"bonus"`
		} `json:"ability_bonuses"`
		RacialTraits []ref `json:"racial_traits"`
	}
	rows, err := decode[src](in, "5e-SRD-Subraces.json")
	if err != nil {
		return output{}, err
	}
	out := make([]srd51.Subrace, 0, len(rows))
	for _, r := range rows {
		out = append(out, srd51.Subrace{
			Key:            "subrace:" + r.Index,
			Name:           r.Name,
			Race:           "race:" + r.Race.Index,
			Desc:           r.Desc,
			AbilityBonuses: abilityBonuses(r.AbilityBonuses),
			Traits:         keys("trait:", r.RacialTraits),
		})
	}
	sortByKey(out, func(s srd51.Subrace) string { return s.Key })
	return output{"subraces.json", out}, nil
}

func convertTraits(in *inputs) (output, error) {
	type src struct {
		Index              string     `json:"index"`
		Name               string     `json:"name"`
		Races              []ref      `json:"races"`
		Subraces           []ref      `json:"subraces"`
		Desc               []string   `json:"desc"`
		Proficiencies      []ref      `json:"proficiencies"`
		ProficiencyChoices *optionSet `json:"proficiency_choices"`
		LanguageOptions    *optionSet `json:"language_options"`
		Parent             *ref       `json:"parent"`
		TraitSpecific      *struct {
			SubtraitOptions *optionSet `json:"subtrait_options"`
		} `json:"trait_specific"`
	}
	rows, err := decode[src](in, "5e-SRD-Traits.json")
	if err != nil {
		return output{}, err
	}
	out := make([]srd51.Trait, 0, len(rows))
	for _, r := range rows {
		t := srd51.Trait{
			Key:           "trait:" + r.Index,
			Name:          r.Name,
			Races:         keys("race:", r.Races),
			Subraces:      keys("subrace:", r.Subraces),
			Desc:          r.Desc,
			Proficiencies: keys("proficiency:", r.Proficiencies),
		}
		if r.ProficiencyChoices != nil {
			t.ProficiencyChoices = r.ProficiencyChoices.Choose
			for _, idx := range r.ProficiencyChoices.referenced() {
				t.ProficiencyOptions = append(t.ProficiencyOptions, "proficiency:"+idx)
			}
		}
		if r.LanguageOptions != nil {
			t.LanguageChoices = r.LanguageOptions.Choose
		}
		if r.Parent != nil {
			t.Parent = "trait:" + r.Parent.Index
		}
		if r.TraitSpecific != nil && r.TraitSpecific.SubtraitOptions != nil {
			for _, idx := range r.TraitSpecific.SubtraitOptions.referenced() {
				t.Options = append(t.Options, "trait:"+idx)
			}
		}
		out = append(out, t)
	}
	sortByKey(out, func(t srd51.Trait) string { return t.Key })
	return output{"traits.json", out}, nil
}

func convertClasses(in *inputs) (output, error) {
	type prerequisite struct {
		AbilityScore ref `json:"ability_score"`
		MinimumScore int `json:"minimum_score"`
	}
	type src struct {
		Index              string      `json:"index"`
		Name               string      `json:"name"`
		HitDie             int         `json:"hit_die"`
		ProficiencyChoices []optionSet `json:"proficiency_choices"`
		Proficiencies      []ref       `json:"proficiencies"`
		SavingThrows       []ref       `json:"saving_throws"`
		MultiClassing      struct {
			Prerequisites       []prerequisite `json:"prerequisites"`
			PrerequisiteOptions *struct {
				From struct {
					Options []prerequisite `json:"options"`
				} `json:"from"`
			} `json:"prerequisite_options"`
			Proficiencies      []ref       `json:"proficiencies"`
			ProficiencyChoices []optionSet `json:"proficiency_choices"`
		} `json:"multi_classing"`
		Subclasses   []ref `json:"subclasses"`
		Spellcasting *struct {
			Level               int `json:"level"`
			SpellcastingAbility ref `json:"spellcasting_ability"`
		} `json:"spellcasting"`
	}
	rows, err := decode[src](in, "5e-SRD-Classes.json")
	if err != nil {
		return output{}, err
	}
	subclassLevels, err := firstSubclassLevels(in)
	if err != nil {
		return output{}, err
	}
	out := make([]srd51.Class, 0, len(rows))
	for _, r := range rows {
		c := srd51.Class{
			Key:          "class:" + r.Index,
			Name:         r.Name,
			HitDie:       r.HitDie,
			SavingThrows: keys("", r.SavingThrows),
			Subclasses:   keys("subclass:", r.Subclasses),
		}
		for _, p := range r.Proficiencies {
			if !strings.HasPrefix(p.Index, "saving-throw-") {
				c.Proficiencies = append(c.Proficiencies, "proficiency:"+p.Index)
			}
		}
		found := false
		for i := range r.ProficiencyChoices {
			if choice, ok := skillChoice(&r.ProficiencyChoices[i]); ok {
				c.SkillChoices, found = choice, true
				break
			}
		}
		if !found {
			return output{}, fmt.Errorf("class %s: no skill choice", r.Index)
		}
		mc := srd51.Multiclass{Proficiencies: keys("proficiency:", r.MultiClassing.Proficiencies)}
		for _, p := range r.MultiClassing.Prerequisites {
			if mc.Minimums == nil {
				mc.Minimums = map[string]int{}
			}
			mc.Minimums[p.AbilityScore.Index] = p.MinimumScore
		}
		if po := r.MultiClassing.PrerequisiteOptions; po != nil {
			mc.AnyOf = map[string]int{}
			for _, p := range po.From.Options {
				mc.AnyOf[p.AbilityScore.Index] = p.MinimumScore
			}
		}
		for i := range r.MultiClassing.ProficiencyChoices {
			if choice, ok := skillChoice(&r.MultiClassing.ProficiencyChoices[i]); ok {
				mc.SkillChoices = &choice
			}
		}
		c.Multiclass = mc
		if r.Spellcasting != nil {
			c.Spellcasting = &srd51.ClassSpellcaster{Level: r.Spellcasting.Level, Ability: r.Spellcasting.SpellcastingAbility.Index}
		}
		for _, sub := range c.Subclasses {
			if lvl := subclassLevels[sub]; c.SubclassLevel == 0 || (lvl > 0 && lvl < c.SubclassLevel) {
				c.SubclassLevel = lvl
			}
		}
		out = append(out, c)
	}
	sortByKey(out, func(c srd51.Class) string { return c.Key })
	return output{"classes.json", out}, nil
}

// firstSubclassLevels returns, per subclass key, the first class level of
// its subclass table: the level at which the subclass is chosen.
func firstSubclassLevels(in *inputs) (map[string]int, error) {
	type src struct {
		Level    int  `json:"level"`
		Subclass *ref `json:"subclass"`
	}
	rows, err := decode[src](in, "5e-SRD-Levels.json")
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	for _, r := range rows {
		if r.Subclass == nil {
			continue
		}
		key := "subclass:" + r.Subclass.Index
		if cur, ok := out[key]; !ok || r.Level < cur {
			out[key] = r.Level
		}
	}
	return out, nil
}

func convertLevels(in *inputs) (output, error) {
	type src struct {
		Level            int             `json:"level"`
		ProfBonus        int             `json:"prof_bonus"`
		Features         []ref           `json:"features"`
		Spellcasting     map[string]int  `json:"spellcasting"`
		ClassSpecific    json.RawMessage `json:"class_specific"`
		SubclassSpecific json.RawMessage `json:"subclass_specific"`
		Class            ref             `json:"class"`
		Subclass         *ref            `json:"subclass"`
	}
	rows, err := decode[src](in, "5e-SRD-Levels.json")
	if err != nil {
		return output{}, err
	}
	out := make([]srd51.Level, 0, len(rows))
	for _, r := range rows {
		l := srd51.Level{
			Class:            "class:" + r.Class.Index,
			Level:            r.Level,
			ProfBonus:        r.ProfBonus,
			Features:         keys("feature:", r.Features),
			ClassSpecific:    compact(r.ClassSpecific),
			SubclassSpecific: compact(r.SubclassSpecific),
		}
		if r.Subclass != nil {
			l.Subclass = "subclass:" + r.Subclass.Index
		}
		if r.Spellcasting != nil {
			sc := &srd51.LevelSpellcasting{
				CantripsKnown: r.Spellcasting["cantrips_known"],
				SpellsKnown:   r.Spellcasting["spells_known"],
			}
			for i := range sc.Slots {
				sc.Slots[i] = r.Spellcasting[fmt.Sprintf("spell_slots_level_%d", i+1)]
			}
			l.Spellcasting = sc
		}
		out = append(out, l)
	}
	slices.SortFunc(out, func(a, b srd51.Level) int {
		return cmp.Or(cmp.Compare(a.Class, b.Class), cmp.Compare(a.Subclass, b.Subclass), cmp.Compare(a.Level, b.Level))
	})
	return output{"levels.json", out}, nil
}

// compact re-encodes raw JSON without spaces, and drops JSON null.
func compact(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return raw
	}
	return buf.Bytes()
}

func convertSubclasses(in *inputs) (output, error) {
	type src struct {
		Index          string   `json:"index"`
		Name           string   `json:"name"`
		Class          ref      `json:"class"`
		SubclassFlavor string   `json:"subclass_flavor"`
		Desc           []string `json:"desc"`
		Spells         []struct {
			Prerequisites []struct {
				Index string `json:"index"`
				Type  string `json:"type"`
			} `json:"prerequisites"`
			Spell ref `json:"spell"`
		} `json:"spells"`
	}
	rows, err := decode[src](in, "5e-SRD-Subclasses.json")
	if err != nil {
		return output{}, err
	}
	out := make([]srd51.Subclass, 0, len(rows))
	for _, r := range rows {
		s := srd51.Subclass{
			Key:    "subclass:" + r.Index,
			Name:   r.Name,
			Class:  "class:" + r.Class.Index,
			Flavor: r.SubclassFlavor,
			Desc:   r.Desc,
		}
		for _, sp := range r.Spells {
			ss := srd51.SubclassSpell{Spell: "spell:" + sp.Spell.Index}
			for _, p := range sp.Prerequisites {
				switch p.Type {
				case "level":
					// "cleric-3" is the 3rd cleric level.
					var lvl int
					_, err := fmt.Sscanf(strings.TrimPrefix(p.Index, r.Class.Index+"-"), "%d", &lvl)
					if err != nil {
						return output{}, fmt.Errorf("subclass %s: prerequisite %q: %w", r.Index, p.Index, err)
					}
					ss.ClassLevel = lvl
				case "feature":
					ss.WithFeatures = append(ss.WithFeatures, "feature:"+p.Index)
				default:
					return output{}, fmt.Errorf("subclass %s: unknown prerequisite type %q", r.Index, p.Type)
				}
			}
			s.Spells = append(s.Spells, ss)
		}
		out = append(out, s)
	}
	sortByKey(out, func(s srd51.Subclass) string { return s.Key })
	return output{"subclasses.json", out}, nil
}

func convertFeatures(in *inputs) (output, error) {
	type src struct {
		Index           string                     `json:"index"`
		Name            string                     `json:"name"`
		Class           ref                        `json:"class"`
		Subclass        *ref                       `json:"subclass"`
		Level           int                        `json:"level"`
		Desc            []string                   `json:"desc"`
		Parent          *ref                       `json:"parent"`
		FeatureSpecific map[string]json.RawMessage `json:"feature_specific"`
	}
	rows, err := decode[src](in, "5e-SRD-Features.json")
	if err != nil {
		return output{}, err
	}
	out := make([]srd51.Feature, 0, len(rows))
	for _, r := range rows {
		f := srd51.Feature{
			Key:   "feature:" + r.Index,
			Name:  r.Name,
			Class: "class:" + r.Class.Index,
			Level: r.Level,
			Desc:  r.Desc,
		}
		if r.Subclass != nil {
			f.Subclass = "subclass:" + r.Subclass.Index
		}
		if r.Parent != nil {
			f.Parent = "feature:" + r.Parent.Index
		}
		if raw, ok := r.FeatureSpecific["subfeature_options"]; ok {
			var o optionSet
			if err := json.Unmarshal(raw, &o); err != nil {
				return output{}, fmt.Errorf("feature %s: %w", r.Index, err)
			}
			f.OptionsChoose = o.Choose
			for _, idx := range o.referenced() {
				f.Options = append(f.Options, "feature:"+idx)
			}
		}
		if raw, ok := r.FeatureSpecific["invocations"]; ok {
			var refs []ref
			if err := json.Unmarshal(raw, &refs); err != nil {
				return output{}, fmt.Errorf("feature %s: %w", r.Index, err)
			}
			f.Options = keys("feature:", refs)
		}
		if raw, ok := r.FeatureSpecific["expertise_options"]; ok {
			var o optionSet
			if err := json.Unmarshal(raw, &o); err != nil {
				return output{}, fmt.Errorf("feature %s: %w", r.Index, err)
			}
			f.ExpertiseChoices = o.picks()
		}
		out = append(out, f)
	}
	sortByKey(out, func(f srd51.Feature) string { return f.Key })
	return output{"features.json", out}, nil
}

func convertBackgrounds(in *inputs) (output, error) {
	type src struct {
		Index                 string     `json:"index"`
		Name                  string     `json:"name"`
		StartingProficiencies []ref      `json:"starting_proficiencies"`
		LanguageOptions       *optionSet `json:"language_options"`
		Feature               struct {
			Name string   `json:"name"`
			Desc []string `json:"desc"`
		} `json:"feature"`
	}
	rows, err := decode[src](in, "5e-SRD-Backgrounds.json")
	if err != nil {
		return output{}, err
	}
	out := make([]srd51.Background, 0, len(rows))
	for _, r := range rows {
		b := srd51.Background{
			Key:  "background:" + r.Index,
			Name: r.Name,
			Feature: srd51.BackgroundFeature{
				Key:  "background-feature:" + slug(r.Feature.Name),
				Name: r.Feature.Name,
				Desc: r.Feature.Desc,
			},
		}
		for _, p := range r.StartingProficiencies {
			if skill, ok := strings.CutPrefix(p.Index, "skill-"); ok {
				b.Skills = append(b.Skills, "skill:"+skill)
			} else {
				b.Proficiencies = append(b.Proficiencies, "proficiency:"+p.Index)
			}
		}
		if r.LanguageOptions != nil {
			b.LanguageChoices = r.LanguageOptions.Choose
		}
		out = append(out, b)
	}
	sortByKey(out, func(b srd51.Background) string { return b.Key })
	return output{"backgrounds.json", out}, nil
}

// slug turns "Shelter of the Faithful" into "shelter-of-the-faithful".
func slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

func convertProficiencies(in *inputs) (output, error) {
	type src struct {
		Index     string `json:"index"`
		Type      string `json:"type"`
		Name      string `json:"name"`
		Reference *ref   `json:"reference"`
	}
	rows, err := decode[src](in, "5e-SRD-Proficiencies.json")
	if err != nil {
		return output{}, err
	}
	kinds := map[string]string{
		"Armor":               "armor",
		"Weapons":             "weapon",
		"Artisan's Tools":     "tool",
		"Gaming Sets":         "tool",
		"Musical Instruments": "tool",
		"Skills":              "skill",
		"Saving Throws":       "saving-throw",
		"Vehicles":            "other",
		"Other":               "other",
	}
	prefixes := []struct{ url, key string }{
		{"/api/2014/equipment-categories/", "equipment-category:"},
		{"/api/2014/equipment/", "equipment:"},
		{"/api/2014/skills/", "skill:"},
		{"/api/2014/ability-scores/", ""},
	}
	out := make([]srd51.Proficiency, 0, len(rows))
	for _, r := range rows {
		kind, ok := kinds[r.Type]
		if !ok {
			return output{}, fmt.Errorf("proficiency %s: unknown type %q", r.Index, r.Type)
		}
		p := srd51.Proficiency{Key: "proficiency:" + r.Index, Name: r.Name, Kind: kind, Refs: []string{}}
		if r.Reference != nil {
			for _, pre := range prefixes {
				if rest, ok := strings.CutPrefix(r.Reference.URL, pre.url); ok {
					p.Refs = append(p.Refs, pre.key+rest)
					break
				}
			}
		}
		out = append(out, p)
	}
	sortByKey(out, func(p srd51.Proficiency) string { return p.Key })
	return output{"proficiencies.json", out}, nil
}

func convertEquipment(in *inputs) (output, error) {
	type damage struct {
		DamageDice string `json:"damage_dice"`
		DamageType ref    `json:"damage_type"`
	}
	type src struct {
		Index             string `json:"index"`
		Name              string `json:"name"`
		EquipmentCategory ref    `json:"equipment_category"`
		WeaponCategory    string `json:"weapon_category"`
		WeaponRange       string `json:"weapon_range"`
		Damage            *damage
		TwoHandedDamage   *damage `json:"two_handed_damage"`
		Range             *struct {
			Normal int `json:"normal"`
			Long   int `json:"long"`
		} `json:"range"`
		ThrowRange *struct {
			Normal int `json:"normal"`
			Long   int `json:"long"`
		} `json:"throw_range"`
		Properties    []ref  `json:"properties"`
		ArmorCategory string `json:"armor_category"`
		ArmorClass    *struct {
			Base     int  `json:"base"`
			DexBonus bool `json:"dex_bonus"`
			MaxBonus int  `json:"max_bonus"`
		} `json:"armor_class"`
		StrMinimum          int  `json:"str_minimum"`
		StealthDisadvantage bool `json:"stealth_disadvantage"`
		GearCategory        ref  `json:"gear_category"`
		Quantity            int  `json:"quantity"`
	}
	rows, err := decode[src](in, "5e-SRD-Equipment.json")
	if err != nil {
		return output{}, err
	}
	var out []srd51.Equipment
	for _, r := range rows {
		e := srd51.Equipment{Key: "equipment:" + r.Index, Name: r.Name}
		switch r.EquipmentCategory.Index {
		case "armor":
			if r.ArmorClass == nil {
				return output{}, fmt.Errorf("armor %s: no armor_class", r.Index)
			}
			e.Kind = "armor"
			e.Armor = &srd51.Armor{
				Category:            strings.ToLower(r.ArmorCategory),
				BaseAC:              r.ArmorClass.Base,
				DexBonus:            r.ArmorClass.DexBonus,
				MaxDexBonus:         r.ArmorClass.MaxBonus,
				StrMinimum:          r.StrMinimum,
				StealthDisadvantage: r.StealthDisadvantage,
			}
		case "weapon":
			e.Kind = "weapon"
			w := &srd51.Weapon{
				Category:   strings.ToLower(r.WeaponCategory),
				Range:      strings.ToLower(r.WeaponRange),
				Properties: keys("weapon-property:", r.Properties),
			}
			if r.Damage != nil {
				w.Damage = r.Damage.DamageDice
				w.DamageType = "damage-type:" + r.Damage.DamageType.Index
			}
			if r.TwoHandedDamage != nil {
				w.TwoHandedDamage = r.TwoHandedDamage.DamageDice
			}
			if r.Range != nil {
				w.NormalRangeFt, w.LongRangeFt = r.Range.Normal, r.Range.Long
			}
			if w.Range == "melee" && slices.Contains(w.Properties, "weapon-property:reach") {
				// The source gives a reach weapon the 5 ft of any melee
				// weapon; the property is what makes it 10.
				w.NormalRangeFt = reachWeaponFt
			}
			if r.ThrowRange != nil {
				w.ThrowNormalFt, w.ThrowLongFt = r.ThrowRange.Normal, r.ThrowRange.Long
			}
			e.Weapon = w
		case "tools":
			e.Kind = "tool"
		case "adventuring-gear":
			e.Kind = "gear"
			e.Gear = &srd51.Gear{Ammunition: r.GearCategory.Index == "ammunition", PackQuantity: r.Quantity}
		default:
			continue // mounts and vehicles
		}
		out = append(out, e)
	}
	sortByKey(out, func(e srd51.Equipment) string { return e.Key })
	return output{"equipment.json", out}, nil
}

func convertSpells(in *inputs) (output, error) {
	type src struct {
		Index         string   `json:"index"`
		Name          string   `json:"name"`
		Desc          []string `json:"desc"`
		HigherLevel   []string `json:"higher_level"`
		Range         string   `json:"range"`
		Components    []string `json:"components"`
		Material      string   `json:"material"`
		Ritual        bool     `json:"ritual"`
		Duration      string   `json:"duration"`
		Concentration bool     `json:"concentration"`
		CastingTime   string   `json:"casting_time"`
		Level         int      `json:"level"`
		AttackType    string   `json:"attack_type"`
		Damage        []struct {
			DamageType             *ref              `json:"damage_type"`
			DamageAtCharacterLevel map[string]string `json:"damage_at_character_level"`
			DamageAtSlotLevel      map[string]string `json:"damage_at_slot_level"`
		} `json:"damage"`
		DC *struct {
			DCType    ref    `json:"dc_type"`
			DCSuccess string `json:"dc_success"`
		} `json:"dc"`
		HealAtSlotLevel map[string]string `json:"heal_at_slot_level"`
		AreaOfEffect    *struct {
			Type string `json:"type"`
			Size int    `json:"size"`
		} `json:"area_of_effect"`
		School     ref   `json:"school"`
		Classes    []ref `json:"classes"`
		Subclasses []ref `json:"subclasses"`
	}
	rows, err := decode[src](in, "5e-SRD-Spells.json")
	if err != nil {
		return output{}, err
	}
	out := make([]srd51.Spell, 0, len(rows))
	for _, r := range rows {
		s := srd51.Spell{
			Key:             "spell:" + r.Index,
			Name:            r.Name,
			Level:           r.Level,
			School:          "school:" + r.School.Index,
			Classes:         keys("class:", r.Classes),
			Subclasses:      keys("subclass:", r.Subclasses),
			Ritual:          r.Ritual,
			Concentration:   r.Concentration,
			CastingTime:     r.CastingTime,
			Range:           r.Range,
			Duration:        r.Duration,
			Components:      r.Components,
			Material:        r.Material,
			AttackType:      r.AttackType,
			HealAtSlotLevel: r.HealAtSlotLevel,
			Desc:            r.Desc,
			HigherLevel:     r.HigherLevel,
		}
		for _, d := range r.Damage {
			sd := srd51.SpellDamage{AtCharacterLevel: d.DamageAtCharacterLevel, AtSlotLevel: d.DamageAtSlotLevel}
			if d.DamageType != nil {
				sd.DamageType = "damage-type:" + d.DamageType.Index
			}
			s.Damage = append(s.Damage, sd)
		}
		if r.DC != nil {
			s.SaveAbility = r.DC.DCType.Index
			s.SaveSuccess = r.DC.DCSuccess
		}
		if a := r.AreaOfEffect; a != nil {
			// The shape and the size are the engine's data (the "Alvo" of a spell):
			// refuse what it does not know instead of guessing.
			if !slices.Contains([]string{"cone", "cube", "cylinder", "line", "sphere"}, a.Type) || a.Size < 5 || a.Size%5 != 0 {
				return output{}, fmt.Errorf("spell %q: area_of_effect %q of %d ft is not a cone, cube, cylinder, line or sphere of a multiple of 5 ft", r.Index, a.Type, a.Size)
			}
			s.AreaType, s.AreaSizeFt = a.Type, a.Size
		}
		out = append(out, s)
	}
	sortByKey(out, func(s srd51.Spell) string { return s.Key })
	return output{"spells.json", out}, nil
}

func convertLanguages(in *inputs) (output, error) {
	type src struct {
		Index  string `json:"index"`
		Name   string `json:"name"`
		Type   string `json:"type"`
		Script string `json:"script"`
	}
	rows, err := decode[src](in, "5e-SRD-Languages.json")
	if err != nil {
		return output{}, err
	}
	out := make([]srd51.Language, 0, len(rows))
	for _, r := range rows {
		out = append(out, srd51.Language{Key: "language:" + r.Index, Name: r.Name, Type: r.Type, Script: r.Script})
	}
	sortByKey(out, func(l srd51.Language) string { return l.Key })
	return output{"languages.json", out}, nil
}

// monstersSource is a stat block of 5e-SRD-Monsters.json, the fields we keep.
type monstersSource struct {
	Index        string `json:"index"`
	Name         string `json:"name"`
	Size         string `json:"size"`
	Type         string `json:"type"`
	Subtype      string `json:"subtype"`
	Alignment    string `json:"alignment"`
	ArmorClasses []struct {
		Type      string `json:"type"`
		Value     int    `json:"value"`
		Desc      string `json:"desc"`
		Armor     []ref  `json:"armor"`
		Spell     *ref   `json:"spell"`
		Condition *ref   `json:"condition"`
	} `json:"armor_class"`
	HitPoints     int                        `json:"hit_points"`
	HitDice       string                     `json:"hit_dice"`
	HitPointsRoll string                     `json:"hit_points_roll"`
	Speed         map[string]json.RawMessage `json:"speed"`
	Str           int                        `json:"strength"`
	Dex           int                        `json:"dexterity"`
	Con           int                        `json:"constitution"`
	Int           int                        `json:"intelligence"`
	Wis           int                        `json:"wisdom"`
	Cha           int                        `json:"charisma"`
	Proficiencies []struct {
		Value       int `json:"value"`
		Proficiency ref `json:"proficiency"`
	} `json:"proficiencies"`
	Vulnerabilities     []string               `json:"damage_vulnerabilities"`
	Resistances         []string               `json:"damage_resistances"`
	Immunities          []string               `json:"damage_immunities"`
	ConditionImmunities []ref                  `json:"condition_immunities"`
	Senses              map[string]any         `json:"senses"`
	Languages           string                 `json:"languages"`
	ChallengeRating     float64                `json:"challenge_rating"`
	ProficiencyBonus    int                    `json:"proficiency_bonus"`
	XP                  int                    `json:"xp"`
	SpecialAbilities    []monsterAbilitySource `json:"special_abilities"`
	Actions             []monsterActionSource  `json:"actions"`
	Reactions           []monsterAbilitySource `json:"reactions"`
	LegendaryActions    []monsterAbilitySource `json:"legendary_actions"`
}

type monsterUsage struct {
	Type      string   `json:"type"`
	Times     int      `json:"times"`
	Dice      string   `json:"dice"`
	MinValue  int      `json:"min_value"`
	RestTypes []string `json:"rest_types"`
}

type monsterAbilitySource struct {
	Name  string        `json:"name"`
	Desc  string        `json:"desc"`
	Usage *monsterUsage `json:"usage"`
}

type monsterDamageSource struct {
	DamageType *ref   `json:"damage_type"`
	DamageDice string `json:"damage_dice"`
	// From is a choice of damage: the first option is kept.
	From *struct {
		Options []struct {
			DamageType *ref   `json:"damage_type"`
			DamageDice string `json:"damage_dice"`
		} `json:"options"`
	} `json:"from"`
}

type monsterDCSource struct {
	DCType    ref    `json:"dc_type"`
	DCValue   int    `json:"dc_value"`
	SuccessTy string `json:"success_type"`
}

// monsterSubSource is a named effect inside an action: a breath of the
// metallic dragons' "Breath Weapons", a roar of the androsphinx.
type monsterSubSource struct {
	Name   string                `json:"name"`
	DC     *monsterDCSource      `json:"dc"`
	Damage []monsterDamageSource `json:"damage"`
}

type monsterMultiItem struct {
	ActionName string `json:"action_name"`
	Count      string `json:"count"`
	Type       string `json:"type"`
}

type monsterActionSource struct {
	Name        string                `json:"name"`
	Desc        string                `json:"desc"`
	Usage       *monsterUsage         `json:"usage"`
	AttackBonus *int                  `json:"attack_bonus"`
	Damage      []monsterDamageSource `json:"damage"`
	DC          *monsterDCSource      `json:"dc"`
	// Options and Attacks carry structured sub-effects (breath weapons, roars).
	Options *struct {
		From struct {
			Options []monsterSubSource `json:"options"`
		} `json:"from"`
	} `json:"options"`
	Attacks         []monsterSubSource `json:"attacks"`
	MultiattackType string             `json:"multiattack_type"`
	Actions         []monsterMultiItem `json:"actions"`
	ActionOptions   *struct {
		From struct {
			Options []struct {
				OptionType string             `json:"option_type"`
				ActionName string             `json:"action_name"`
				Count      string             `json:"count"`
				Type       string             `json:"type"`
				Items      []monsterMultiItem `json:"items"`
			} `json:"options"`
		} `json:"from"`
	} `json:"action_options"`
}

// leadingInt reads the number a string such as "60 ft." or "30 ft. (hover)"
// starts with.
func leadingInt(s string) int {
	n, _, _ := strings.Cut(strings.TrimSpace(s), " ")
	v, _ := strconv.Atoi(n)
	return v
}

// damageTypeWords are the SRD's damage types, as the monster lists name them.
var damageTypeWords = regexp.MustCompile(`^(acid|bludgeoning|cold|fire|force|lightning|necrotic|piercing|poison|psychic|radiant|slashing|thunder)(, and |, | and |\b)`)

// damageMod splits an SRD vulnerability, resistance or immunity entry into
// the damage types it names and the rest of its text.
func damageMod(s string) srd51.MonsterDamageMod {
	var out srd51.MonsterDamageMod
	rest := strings.TrimSpace(s)
	for {
		m := damageTypeWords.FindStringSubmatch(rest)
		if m == nil {
			break
		}
		out.Types = append(out.Types, "damage-type:"+m[1])
		rest = rest[len(m[0]):]
	}
	out.Note = strings.TrimSpace(rest)
	return out
}

func damageMods(list []string) []srd51.MonsterDamageMod {
	var out []srd51.MonsterDamageMod
	for _, s := range list {
		out = append(out, damageMod(s))
	}
	return out
}

// challengeRating writes 0.125, 0.25 and 0.5 as the SRD's "1/8", "1/4" and
// "1/2".
func challengeRating(v float64) (string, error) {
	switch v {
	case 0.125:
		return "1/8", nil
	case 0.25:
		return "1/4", nil
	case 0.5:
		return "1/2", nil
	}
	if v < 0 || v > 30 || v != float64(int(v)) {
		return "", fmt.Errorf("challenge rating %v", v)
	}
	return strconv.Itoa(int(v)), nil
}

// xpByChallengeRating is the experience of each challenge rating, the SRD's table. The
// importer takes a creature's XP from here, not from the source, which has the value
// of four creatures at half of it.
//
//nolint:mnd // the SRD's table itself; rules.TestCreatureXPMatchesChallengeRatingTable checks every creature against effects/advancement.json
var xpByChallengeRating = map[string]int{
	"0": 10, "1/8": 25, "1/4": 50, "1/2": 100, "1": 200, "2": 450, "3": 700, "4": 1100, "5": 1800, "6": 2300, "7": 2900,
	"8": 3900, "9": 5000, "10": 5900, "11": 7200, "12": 8400, "13": 10000, "14": 11500, "15": 13000, "16": 15000, "17": 18000,
	"18": 20000, "19": 22000, "20": 25000, "21": 33000, "22": 41000, "23": 50000, "24": 62000, "25": 75000, "26": 90000,
	"27": 105000, "28": 120000, "29": 135000, "30": 155000,
}

// monsterXP is the XP of a creature of the challenge rating. A challenge rating 0 creature
// gives 0 by convention (the frog, the sea horse) or 10: the source's value is kept if it is one of these.
func monsterXP(cr string, source int) (int, error) {
	xp, ok := xpByChallengeRating[cr]
	if !ok {
		return 0, fmt.Errorf("challenge rating %q has no XP", cr)
	}
	if cr == "0" && source == 0 {
		return 0, nil
	}
	return xp, nil
}

func monsterUsageText(u *monsterUsage) string {
	switch {
	case u == nil:
		return ""
	case u.Type == "per day":
		return fmt.Sprintf("%d/day", u.Times)
	case u.Type == "recharge on roll" && u.MinValue >= 6:
		return "Recharge 6"
	case u.Type == "recharge on roll":
		return fmt.Sprintf("Recharge %d-6", u.MinValue)
	case u.Type == "recharge after rest":
		return "Recharges after a short or long rest"
	}
	return u.Type
}

func convertMonsters(in *inputs) (output, error) {
	rows, err := decode[monstersSource](in, "5e-SRD-Monsters.json")
	if err != nil {
		return output{}, err
	}
	out, err := convertMonsterRows(rows)
	if err != nil {
		return output{}, err
	}
	if len(out) != 334 {
		return output{}, fmt.Errorf("monsters: %d creatures, the SRD 5.1 has 334", len(out))
	}
	return output{"monsters.json", out}, nil
}

// convertMonsterRows converts stat blocks, sorted by key.
func convertMonsterRows(rows []monstersSource) ([]srd51.Monster, error) {
	out := make([]srd51.Monster, 0, len(rows))
	for _, r := range rows {
		m, err := convertMonster(r)
		if err != nil {
			return nil, fmt.Errorf("monster %s: %w", r.Index, err)
		}
		out = append(out, m)
	}
	sortByKey(out, func(m srd51.Monster) string { return m.Key })
	return out, nil
}

// armorClass picks the armor class worn: the "armor" entry over the "natural"
// or "dex" one when both are listed (the azer's 17 with a shield). A "spell" or
// "condition" entry is another way to have one, kept apart.
func armorClass(r monstersSource, m *srd51.Monster) error {
	best := -1
	for i, a := range r.ArmorClasses {
		switch a.Type {
		case "armor":
			best = i
		case "natural", "dex":
			if best < 0 {
				best = i
			}
		case "spell", "condition":
			alt := srd51.MonsterACAlt{Value: a.Value}
			switch {
			case a.Spell != nil:
				alt.Spell = "spell:" + a.Spell.Index
			case a.Condition != nil:
				alt.Condition = "condition:" + a.Condition.Index
			default:
				return fmt.Errorf("armor class %q names no spell or condition", a.Type)
			}
			m.ArmorClassAlts = append(m.ArmorClassAlts, alt)
		default:
			return fmt.Errorf("unknown armor class type %q", a.Type)
		}
	}
	if best < 0 {
		return errors.New("no armor class")
	}
	a := r.ArmorClasses[best]
	m.ArmorClass, m.ArmorClassType, m.ArmorClassDesc = a.Value, a.Type, a.Desc
	for _, item := range a.Armor {
		m.ArmorClassItems = append(m.ArmorClassItems, "equipment:"+item.Index)
	}
	return nil
}

func convertMonster(r monstersSource) (srd51.Monster, error) {
	cr, err := challengeRating(r.ChallengeRating)
	if err != nil {
		return srd51.Monster{}, err
	}
	xp, err := monsterXP(cr, r.XP)
	if err != nil {
		return srd51.Monster{}, err
	}
	m := srd51.Monster{
		Key: "monster:" + r.Index, Name: r.Name, Size: r.Size, Type: r.Type, Subtype: r.Subtype, Alignment: r.Alignment,
		HitPoints: r.HitPoints, HitDice: r.HitDice, HitPointsRoll: r.HitPointsRoll,
		Str: r.Str, Dex: r.Dex, Con: r.Con, Int: r.Int, Wis: r.Wis, Cha: r.Cha,
		Vulnerabilities: damageMods(r.Vulnerabilities), Resistances: damageMods(r.Resistances), Immunities: damageMods(r.Immunities),
		ConditionImmunities: keys("condition:", r.ConditionImmunities),
		Languages:           r.Languages, ChallengeRating: cr, XP: xp, ProficiencyBonus: r.ProficiencyBonus,
	}
	if err := armorClass(r, &m); err != nil {
		return srd51.Monster{}, err
	}
	for kind, raw := range r.Speed {
		if kind == "hover" {
			m.Speed.Hover = string(raw) == "true"
			continue
		}
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return srd51.Monster{}, fmt.Errorf("speed %s: %w", kind, err)
		}
		ft := leadingInt(text)
		switch kind {
		case "walk":
			m.Speed.Walk = ft
		case "fly":
			m.Speed.Fly = ft
		case "swim":
			m.Speed.Swim = ft
		case "climb":
			m.Speed.Climb = ft
		case "burrow":
			m.Speed.Burrow = ft
		default:
			return srd51.Monster{}, fmt.Errorf("unknown speed %q", kind)
		}
	}
	for k, v := range r.Senses {
		text, _ := v.(string)
		switch k {
		case "darkvision":
			m.Darkvision = leadingInt(text)
		case "blindsight":
			m.Blindsight = leadingInt(text)
		case "tremorsense":
			m.Tremorsense = leadingInt(text)
		case "truesight":
			m.Truesight = leadingInt(text)
		case "passive_perception":
			n, _ := v.(float64)
			m.PassivePerception = int(n)
		default:
			return srd51.Monster{}, fmt.Errorf("unknown sense %q", k)
		}
	}
	for _, p := range r.Proficiencies {
		switch idx := p.Proficiency.Index; {
		case strings.HasPrefix(idx, "saving-throw-"):
			if m.Saves == nil {
				m.Saves = map[string]int{}
			}
			m.Saves[strings.TrimPrefix(idx, "saving-throw-")] = p.Value
		case strings.HasPrefix(idx, "skill-"):
			if m.Skills == nil {
				m.Skills = map[string]int{}
			}
			m.Skills["skill:"+strings.TrimPrefix(idx, "skill-")] = p.Value
		default:
			return srd51.Monster{}, fmt.Errorf("unknown proficiency %q", idx)
		}
	}
	for _, a := range r.SpecialAbilities {
		m.SpecialAbilities = append(m.SpecialAbilities, srd51.MonsterAbility{Name: a.Name, Desc: a.Desc, Usage: monsterUsageText(a.Usage)})
	}
	for _, a := range r.Reactions {
		m.Reactions = append(m.Reactions, srd51.MonsterAbility{Name: a.Name, Desc: a.Desc})
	}
	for _, a := range r.LegendaryActions {
		m.LegendaryActions = append(m.LegendaryActions, srd51.MonsterAbility{Name: a.Name, Desc: a.Desc})
	}
	for _, a := range r.Actions {
		m.Actions = append(m.Actions, expandMonsterAction(a)...)
	}
	fixMultiattackNames(&m)
	return m, nil
}

// textSaveRe finds a saving throw in an action's text, such as "DC 11 Strength
// saving throw" or "DC 16 Strength or Dexterity saving throw" (the first
// ability is kept).
var textSaveRe = regexp.MustCompile(`DC (\d+) (Strength|Dexterity|Constitution|Intelligence|Wisdom|Charisma)(?: or (?:Strength|Dexterity|Constitution|Intelligence|Wisdom|Charisma))? saving throw`)

var (
	halfRe    = regexp.MustCompile(`(?i)half as much damage|takes only half|half (?:of )?the [a-z, ]*damage`)
	succeedRe = regexp.MustCompile(`must succeed on a DC|succeed on a DC`)
)

// textSuccess says what a success does, from the action's text: "half" when it
// halves the damage, "none" when the text only says the target must succeed,
// and "other" (read the description) when it says neither.
func textSuccess(text string) string {
	switch {
	case halfRe.MatchString(text):
		return "half"
	case succeedRe.MatchString(text):
		return "none"
	}
	return "other"
}

func convertDamage(list []monsterDamageSource) []srd51.MonsterDamage {
	var out []srd51.MonsterDamage
	for _, d := range list {
		typ, dice := d.DamageType, d.DamageDice
		if d.From != nil && len(d.From.Options) > 0 { // a choice: keep the first option
			typ, dice = d.From.Options[0].DamageType, d.From.Options[0].DamageDice
		}
		if typ == nil || dice == "" {
			continue
		}
		out = append(out, srd51.MonsterDamage{Dice: dice, DamageType: "damage-type:" + typ.Index})
	}
	return out
}

func convertDC(dc *monsterDCSource) *srd51.MonsterSave {
	if dc == nil {
		return nil
	}
	return &srd51.MonsterSave{Ability: dc.DCType.Index, DC: dc.DCValue, OnSuccess: dc.SuccessTy}
}

func convertMonsterAction(a monsterActionSource) srd51.MonsterAction {
	out := srd51.MonsterAction{Name: a.Name, Desc: strings.TrimSpace(a.Desc), Usage: monsterUsageText(a.Usage)}
	if a.AttackBonus != nil {
		out.HasAttack, out.AttackBonus = true, *a.AttackBonus
	}
	out.Damage = convertDamage(a.Damage)
	out.Save = convertDC(a.DC)
	if out.Save == nil {
		// Many attacks (a wolf's bite) have a rider save that 5e-database
		// leaves in the text only.
		if m := textSaveRe.FindStringSubmatch(a.Desc); m != nil {
			dc, _ := strconv.Atoi(m[1])
			out.Save = &srd51.MonsterSave{Ability: strings.ToLower(m[2][:3]), DC: dc, OnSuccess: textSuccess(a.Desc)}
		}
	}
	// Counts: a number, or words for the master (the hydra has "Number of
	// Heads", five by the SRD; the violet fungus "1d4"); the engine always
	// gets a number of at least 1.
	count := func(x monsterMultiItem) srd51.MonsterAttackCount {
		c, err := strconv.Atoi(x.Count)
		if err == nil {
			return srd51.MonsterAttackCount{Name: x.ActionName, Count: c, Kind: x.Type}
		}
		c = 1
		if x.Count == "Number of Heads" {
			c = 5
		}
		return srd51.MonsterAttackCount{Name: x.ActionName, Count: c, Kind: x.Type, Text: x.Count}
	}
	switch a.MultiattackType {
	case "actions":
		var routine []srd51.MonsterAttackCount
		for _, x := range a.Actions {
			routine = append(routine, count(x))
		}
		out.Multiattack = [][]srd51.MonsterAttackCount{routine}
	case "action_options":
		for _, o := range a.ActionOptions.From.Options {
			var routine []srd51.MonsterAttackCount
			if o.OptionType == "multiple" {
				for _, x := range o.Items {
					routine = append(routine, count(x))
				}
			} else {
				routine = append(routine, count(monsterMultiItem{ActionName: o.ActionName, Count: o.Count, Type: o.Type}))
			}
			out.Multiattack = append(out.Multiattack, routine)
		}
	}
	return out
}

// expandMonsterAction is the action, followed by the named effects the SRD
// gives structured inside it (each breath of a metallic dragon, each roar of
// the androsphinx), so their DCs are not lost. Each one's text is the paragraph
// of the action's text that starts with its name.
func expandMonsterAction(a monsterActionSource) []srd51.MonsterAction {
	out := []srd51.MonsterAction{convertMonsterAction(a)}
	subs := a.Attacks
	if a.Options != nil {
		subs = append(subs, a.Options.From.Options...)
	}
	if len(subs) > 0 {
		out[0].Save = nil // the parent's text names several DCs: the sub-effects carry them
	}
	for _, sub := range subs {
		text := ""
		for line := range strings.SplitSeq(a.Desc, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), sub.Name+".") {
				text = strings.TrimSpace(line)
			}
		}
		if text == "" {
			text = a.Desc
		}
		out = append(out, srd51.MonsterAction{Name: sub.Name, Desc: text, Usage: monsterUsageText(a.Usage), Damage: convertDamage(sub.Damage), Save: convertDC(sub.DC)})
	}
	return out
}

// fixMultiattackNames points each Multiattack routine at an action of the same
// stat block when the SRD's name differs slightly: "Claws" for "Claw", "Claw"
// for "Claw (Oni Form Only)", "Bite (Bat or Vampire Form Only)" for "Bite".
// What matches nothing keeps its name (a spell, the glabrezu's casting).
func fixMultiattackNames(m *srd51.Monster) {
	names := map[string]bool{}
	for _, a := range m.Actions {
		names[a.Name] = true
	}
	resolve := func(n string) string {
		if names[n] {
			return n
		}
		if base, _, ok := strings.Cut(n, " ("); ok && names[base] {
			return base
		}
		if names[strings.TrimSuffix(n, "s")] {
			return strings.TrimSuffix(n, "s")
		}
		for _, a := range m.Actions {
			if strings.HasPrefix(a.Name, n+" (") {
				return a.Name
			}
		}
		return n
	}
	for i := range m.Actions {
		for _, routine := range m.Actions[i].Multiattack {
			for j := range routine {
				routine[j].Name = resolve(routine[j].Name)
			}
		}
	}
}

// named converts a small list (damage types, schools...) into Named entries.
func named(inName, outName, prefix string) func(*inputs) (output, error) {
	return func(in *inputs) (output, error) {
		type src struct {
			Index string          `json:"index"`
			Name  string          `json:"name"`
			Desc  json.RawMessage `json:"desc"`
		}
		rows, err := decode[src](in, inName)
		if err != nil {
			return output{}, err
		}
		out := make([]srd51.Named, 0, len(rows))
		for _, r := range rows {
			n := srd51.Named{Key: prefix + r.Index, Name: r.Name, Desc: []string{}}
			// desc is a list of paragraphs in most files and one string in
			// a few.
			if len(r.Desc) > 0 && json.Unmarshal(r.Desc, &n.Desc) != nil {
				var one string
				if err := json.Unmarshal(r.Desc, &one); err != nil {
					return output{}, fmt.Errorf("%s %s: desc: %w", inName, r.Index, err)
				}
				n.Desc = []string{one}
			}
			out = append(out, n)
		}
		sortByKey(out, func(n srd51.Named) string { return n.Key })
		return output{outName, out}, nil
	}
}

func sortByKey[T any](s []T, key func(T) string) {
	slices.SortStableFunc(s, func(a, b T) int { return cmp.Compare(key(a), key(b)) })
}

// writeOutputs writes every file, then manifest.json with their hashes. It
// removes stale .json files so a renamed output never lingers.
func writeOutputs(dir string, files []output, inputHashes []srd51.FileHash) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	old, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return err
	}
	for _, f := range old {
		if err := os.Remove(f); err != nil {
			return err
		}
	}
	manifest := srd51.Manifest{
		SourceRepo:      sourceRepo,
		SourceCommit:    sourceCommit,
		SourcePath:      sourcePath,
		SnapshotVersion: "srd51@" + sourceCommit[:12],
		Inputs:          inputHashes,
	}
	for _, f := range files {
		b, err := encode(f.data)
		if err != nil {
			return fmt.Errorf("%s: %w", f.name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, f.name), b, 0o600); err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		manifest.Outputs = append(manifest.Outputs, srd51.FileHash{Name: f.name, SHA256: hex.EncodeToString(sum[:])})
	}
	slices.SortFunc(manifest.Outputs, func(a, b srd51.FileHash) int { return cmp.Compare(a.Name, b.Name) })
	b, err := encode(manifest)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), b, 0o600); err != nil {
		return err
	}
	if len(files) == 0 {
		return errors.New("no output")
	}
	return nil
}

// encode writes indented JSON with a final newline, without escaping <, >
// and & (the SRD text has them, and escaped text is hard to review).
func encode(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", " ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
