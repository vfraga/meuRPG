package rules

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"
)

// overlayBuildCases corrupt the full overlay as a whole, or one entry key or name
// of each kind, to freeze what build and claimKeys refuse, with which field and
// reason, and which of several problems comes first.
func overlayBuildCases() map[string]func(o *Overlay) {
	feat := TableFeat{TableEntry: TableEntry{Key: "feat:talento" + tableSuffix, NamePT: "Talento"}}
	return map[string]func(o *Overlay){
		"ok: the full overlay":  func(_ *Overlay) {},
		"ok: with a feat":       func(o *Overlay) { o.Feats = append(o.Feats, feat) },
		"ok: revision 0":        func(o *Overlay) { o.Revision = 0 },
		"revision negative":     func(o *Overlay) { o.Revision = -1 },
		"revision before limit": func(o *Overlay) { o.Revision = -1; o.Feats = make([]TableFeat, MaxOverlayEntries) },
		"301 entries": func(o *Overlay) {
			o.Feats = make([]TableFeat, MaxOverlayEntries-len(o.Classes)-len(o.Subclasses)-len(o.Spells)-len(o.Races)-len(o.Subraces)-len(o.Backgrounds)+1)
		},
		"ok: 300 entries": func(o *Overlay) {
			for i := range MaxOverlayEntries - len(o.Classes) - len(o.Subclasses) - len(o.Spells) - len(o.Races) - len(o.Subraces) - len(o.Backgrounds) {
				f := feat
				f.Key = "feat:talento-" + strconv.Itoa(i) + tableSuffix
				o.Feats = append(o.Feats, f)
			}
		},
		"limit before budget": func(o *Overlay) {
			o.Feats = make([]TableFeat, MaxOverlayEntries)
			o.Feats[0].Effects = make([]Effect, MaxFeatureEffects+1)
		},
		"budget before keys": func(o *Overlay) {
			o.Classes[0].Key = "nope"
			o.Feats = []TableFeat{{TableEntry: TableEntry{Key: "nope"}, Effects: make([]Effect, MaxFeatureEffects+1)}}
		},

		// The keys and names, by kind, in the order claimKeys walks them.
		"class key prefix":            func(o *Overlay) { o.Classes[1].Key = "subclass:x" + tableSuffix },
		"class key suffix":            func(o *Overlay) { o.Classes[1].Key = "class:x" },
		"class name empty":            func(o *Overlay) { o.Classes[1].NamePT = "" },
		"subclass key reserved":       func(o *Overlay) { o.Subclasses[2].Key = "subclass:wild-shape-x" + tableSuffix },
		"subclass name spaces":        func(o *Overlay) { o.Subclasses[2].NamePT = " Caminho" },
		"race key too long":           func(o *Overlay) { o.Races[0].Key = "race:" + strings.Repeat("a", MaxSlugLength+1) + tableSuffix },
		"race name too long":          func(o *Overlay) { o.Races[0].NamePT = strings.Repeat("a", maxNameRunes+1) },
		"subrace key bad character":   func(o *Overlay) { o.Subraces[0].Key = "subrace:Colina" + tableSuffix },
		"subrace name two lines":      func(o *Overlay) { o.Subraces[0].NamePT = "Da\nColina" },
		"background key spellcasting": func(o *Overlay) { o.Backgrounds[0].Key = "background:spellcasting" + tableSuffix },
		"background name hidden":      func(o *Overlay) { o.Backgrounds[0].NamePT = "Guarda\u200b" },
		"spell key empty slug":        func(o *Overlay) { o.Spells[3].Key = "spell:" + tableSuffix },
		"spell key twice":             func(o *Overlay) { o.Spells[3].Key = o.Spells[1].Key },
		"spell name empty":            func(o *Overlay) { o.Spells[3].NamePT = "" },
		"feat key of a spell":         func(o *Overlay) { f := feat; f.Key = "spell:talento" + tableSuffix; o.Feats = append(o.Feats, f) },
		"feat name empty":             func(o *Overlay) { f := feat; f.NamePT = ""; o.Feats = append(o.Feats, f) },
		"key before name": func(o *Overlay) {
			o.Classes[1].Key, o.Classes[1].NamePT = "class:x", ""
		},
		"keys in key order, not the caller's": func(o *Overlay) {
			o.Spells[0].Key, o.Spells[4].Key = "spell:zz"+tableSuffix, "spell:aa"
		},
		"classes claimed before feats": func(o *Overlay) {
			f := feat
			f.Key = "feat:x"
			o.Feats = append(o.Feats, f)
			o.Classes[4].NamePT = ""
		},
		"subclasses claimed before races": func(o *Overlay) {
			o.Races[0].NamePT = ""
			o.Subclasses[0].NamePT = ""
		},
		"races claimed before subraces": func(o *Overlay) {
			o.Subraces[0].NamePT = ""
			o.Races[0].NamePT = ""
		},
		"subraces claimed before backgrounds": func(o *Overlay) {
			o.Backgrounds[0].NamePT = ""
			o.Subraces[0].NamePT = ""
		},
		"backgrounds claimed before spells": func(o *Overlay) {
			o.Spells[0].NamePT = ""
			o.Backgrounds[0].NamePT = ""
		},
		"spells claimed before feats": func(o *Overlay) {
			o.Feats = append(o.Feats, TableFeat{TableEntry: TableEntry{Key: "feat:x" + tableSuffix}})
			o.Spells[2].NamePT = ""
		},
		"a feature key taken by an entry": func(o *Overlay) {
			o.Backgrounds[0].Feature.Key = "background-feature:x" + tableSuffix
			o.Backgrounds = append(o.Backgrounds, o.Backgrounds[0])
			o.Backgrounds[1].Key = "background:outro" + tableSuffix
		},

		// The entries, in the order build adds them, and what a failing one brings.
		"spells added before races": func(o *Overlay) {
			o.Races[0].Size = "Huge"
			o.Spells[2].Level = 12
		},
		"races added before subraces": func(o *Overlay) {
			o.Subraces[0].Race = "race:nope"
			o.Races[0].Size = "Huge"
		},
		"subraces added before classes": func(o *Overlay) {
			o.Classes[0].HitDie = 7
			o.Subraces[0].Race = "race:nope"
		},
		"classes added before subclasses": func(o *Overlay) {
			o.Subclasses[0].Class = "class:nope"
			o.Classes[0].HitDie = 7
		},
		"subclasses added before backgrounds": func(o *Overlay) {
			o.Backgrounds[0].Skills = nil
			o.Subclasses[0].Class = "class:nope"
		},
		"backgrounds added before feats": func(o *Overlay) {
			f := feat
			f.Prerequisite.Level = -1
			o.Feats = append(o.Feats, f)
			o.Backgrounds[0].Skills = nil
		},
		"feat refused": func(o *Overlay) {
			f := feat
			f.Prerequisite.Level = -1
			o.Feats = append(o.Feats, f)
		},
		"a failing race brings its traits' effects": func(o *Overlay) {
			o.Races[0].Size = "Huge"
			o.Races[0].Traits[0].Effects = append(o.Races[0].Traits[0].Effects, Effect{Type: "teleport"}, Effect{Type: "modifier", Target: "nope", Mode: "add", Value: "1"})
		},
		"a failing feat brings its effects": func(o *Overlay) {
			f := feat
			f.Prerequisite.Level = -1
			f.Effects = []Effect{{Type: "handler"}}
			o.Feats = append(o.Feats, f)
		},
		"a failing spell has no effects to bring": func(o *Overlay) {
			o.Spells[0].Level = -1
			o.Races[0].Traits[0].Effects = []Effect{{Type: "teleport"}}
		},
		"a failing feature has no entry to compile": func(o *Overlay) {
			o.Races[0].Traits[0].Key = "trait:x"
			o.Races[0].Traits = append(o.Races[0].Traits, TableFeature{Key: "trait:outro" + tableSuffix, NamePT: "Outro", Effects: []Effect{{Type: "teleport"}}})
		},
		"effects compiled once every entry exists": func(o *Overlay) {
			o.Backgrounds[0].Feature.Effects = []Effect{{Type: "teleport"}}
			o.Races[0].Traits[0].Effects = []Effect{{Type: "modifier", Target: "nope", Mode: "add", Value: "1"}}
		},
		"strict refuses a stray field": func(o *Overlay) {
			o.Strict = []string{o.Backgrounds[0].Key}
			o.Backgrounds[0].Feature.Effects[0].Count = 3
		},
		"ok: lax ignores a stray field": func(o *Overlay) {
			o.Strict = []string{o.Races[0].Key}
			o.Backgrounds[0].Feature.Effects[0].Count = 3
		},
	}
}

// TestOverlayBuildCharacterization freezes what build refuses before and while
// it adds the entries, and the one refusal of a content that has a table layer.
func TestOverlayBuildCharacterization(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	got := map[string]string{}
	for name, edit := range overlayBuildCases() {
		o := fullOverlay(t, srd)
		edit(&o)
		c, err := srd.With(o)
		got[name] = violationText(err)
		if (err == nil) != (c != nil) || strings.HasPrefix(name, "ok:") != (err == nil) {
			t.Errorf("%s: content %v, error %v", name, c != nil, err)
		}
	}
	layered := withOverlayOn(t, srd, Overlay{Revision: 1})
	_, err := layered.With(Overlay{Revision: -1})
	got["a second table layer"] = violationText(err)
	goldenText(t, "overlay-build-refusals", got)
}

// richOverlay is the full overlay with a feat, the posture subclass and its
// option list, entries archived and stamped, and switches of every sort, so
// the content golden sees every write build makes.
func richOverlay(t *testing.T, srd *Content) Overlay {
	t.Helper()
	o := fullOverlay(t, srd)
	o.Subclasses = append(o.Subclasses, postureOverlay().Subclasses...)
	stamp := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	o.Feats = []TableFeat{{
		TableEntry:   TableEntry{Key: "feat:talento" + tableSuffix, NamePT: "Talento", Revision: 5, ChangedAt: stamp},
		DescPT:       []string{"Texto do talento."},
		Prerequisite: FeatPrerequisite{Minimums: map[Ability]int{STR: 13}, Race: o.Subraces[0].Key},
		Effects:      []Effect{{Type: "ability_increase", From: []string{"str", "con"}, Count: 1, Value: "1"}},
	}}
	o.Races[0].Archived = true
	o.Spells[0].Revision, o.Spells[0].ChangedAt = 4, stamp
	o.Off = []string{"class:fighter", o.Spells[1].Key, "feature:nope", "skill:arcana", postureSubclass}
	return o
}

// tableOnly keeps the entries of m whose key is the table's.
func tableOnly[V any](m map[string]V) map[string]V {
	out := map[string]V{}
	for k, v := range m {
		if isTableKey(k) {
			out[k] = v
		}
	}
	return out
}

// TestOverlayContentGolden freezes every write With makes for a rich overlay:
// the entries of each kind in the clone, the indexes it fills and the version.
func TestOverlayContentGolden(t *testing.T) {
	t.Parallel()
	srd := loadForTest(t)
	c, err := srd.With(richOverlay(t, srd))
	if err != nil {
		t.Fatal(err)
	}
	n := c.c
	parts := map[string]any{
		"version":        strings.TrimPrefix(n.version, srd.Version()) + " table=" + strconv.FormatBool(n.table) + " revision=" + strconv.Itoa(n.tableRevision),
		"spells":         tableOnly(n.spells),
		"spellTargets":   n.spellTargets,
		"races":          tableOnly(n.races),
		"raceChoice":     n.raceChoice,
		"subraces":       tableOnly(n.subraces),
		"traits":         tableOnly(n.traits),
		"features":       tableOnly(n.features),
		"offeredBy":      n.offeredBy,
		"backgrounds":    tableOnly(n.backgrounds),
		"bgEquipment":    n.bgEquipment,
		"feats":          tableOnly(n.feats),
		"effects":        tableOnly(n.effects),
		"names":          tableOnly(n.namesPT),
		"namesEN":        tableOnly(n.namesEN),
		"archived":       n.archived,
		"off":            n.off,
		"entryRevision":  n.entryRevision,
		"entryChangedAt": n.entryChangedAt,
		"listFrom":       n.listFrom,
		"dwarfSubraces":  n.races["race:dwarf"].Subraces,
		"raceSubraces":   n.races["race:anao-das-brumas"+tableSuffix].Subraces,
	}
	got := map[string]string{}
	for name, v := range parts {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		got[name] = string(b)
	}
	if n.programs != nil {
		t.Error("the compile cache outlived build")
	}
	goldenText(t, "overlay-content", got)
}
