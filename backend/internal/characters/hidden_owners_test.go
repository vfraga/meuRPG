package characters

import (
	"slices"
	"testing"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// What a hidden owner hides, and what a player's own sheet keeps (RN-23).

const (
	sharedStyle     = "option:fighting_style:estilo-comum@mesa"
	rangerFeature   = "classfeature:olhar-de-teste@mesa"
	rangerStyleOnly = "option:hunters_prey:presa-de-teste@mesa"
)

func ownersContent(t *testing.T, off ...string) *rules.Content {
	t.Helper()
	srd, err := rules.LoadSRD()
	if err != nil {
		t.Fatal(err)
	}
	c, err := srd.With(rules.Overlay{
		Revision: 1, Off: off,
		ClassFeatures: []rules.TableClassFeature{{TableEntry: rules.TableEntry{Key: rangerFeature, NamePT: "Olhar de teste"}, Class: "class:ranger", Level: 1}},
		Options: []rules.TableOption{
			{TableEntry: rules.TableEntry{Key: sharedStyle, NamePT: "Estilo comum"}, Set: "fighting_style"},
			{TableEntry: rules.TableEntry{Key: rangerStyleOnly, NamePT: "Presa de teste"}, Set: "hunters_prey"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// A table option several classes offer is hidden only when all of them are.
func TestAnOptionSeveralClassesOfferIsHiddenOnlyWithAllOfThem(t *testing.T) {
	t.Parallel()
	one := ownersContent(t, "class:fighter")
	if one.Hidden(sharedStyle) {
		t.Fatalf("Hidden(%s) = true with only the fighter off: the paladin and the ranger still offer it", sharedStyle)
	}
	paladin := levelUpTarget{content: one, build: rules.Build{Classes: []rules.ClassLevel{{Class: "class:paladin", Level: 2}}}}
	if hiddenOption(paladin, sharedStyle) {
		t.Errorf("the option is hidden from a paladin's swap pool")
	}
	// The switches' list and the player's entries agree.
	l := &tableListing{content: one, hidden: one.Hidden, mine: map[string]bool{}}
	e := entryRow{row: charactersdb.CampaignContent{ContentKey: sharedStyle}, kind: rulesv1.TableContentKind_TABLE_CONTENT_KIND_OPTION, body: &rulesv1.TableOption{}}
	if !l.playerReads(e) {
		t.Error("a player without a sheet does not read the option the paladin and the ranger offer")
	}
	all := ownersContent(t, "class:fighter", "class:paladin", "class:ranger")
	if !all.Hidden(sharedStyle) {
		t.Errorf("Hidden(%s) = false with every class that offers it off", sharedStyle)
	}
	if l := (&tableListing{content: all, hidden: all.Hidden, mine: map[string]bool{}}); l.playerReads(e) {
		t.Error("a player reads the option with every class that offers it off")
	}
}

// A class that is off still gives its table feature and option entries to a player whose sheet has it.
func TestAPlayerWhoseClassIsOffReadsTheClassFeatureAndOptionEntries(t *testing.T) {
	t.Parallel()
	c := ownersContent(t, "class:ranger")
	feature := entryRow{row: charactersdb.CampaignContent{ContentKey: rangerFeature}, kind: rulesv1.TableContentKind_TABLE_CONTENT_KIND_CLASS_FEATURE, body: &rulesv1.TableClassFeature{}}
	option := entryRow{row: charactersdb.CampaignContent{ContentKey: rangerStyleOnly}, kind: rulesv1.TableContentKind_TABLE_CONTENT_KIND_OPTION, body: &rulesv1.TableOption{}}
	for _, e := range []entryRow{feature, option} {
		key := e.row.ContentKey
		if !c.Hidden(key) {
			t.Fatalf("Hidden(%s) = false with the ranger off", key)
		}
		none := &tableListing{content: c, hidden: c.Hidden, mine: map[string]bool{}}
		ranger := &tableListing{content: c, hidden: c.Hidden, mine: map[string]bool{"class:ranger": true, "subclass:hunter": true}}
		if none.playerReads(e) || !ranger.playerReads(e) {
			t.Errorf("%s: read without the class = %v, with the class = %v; want false and true", key, none.playerReads(e), ranger.playerReads(e))
		}
	}
}

// The subrace of a race the master switched off still names the traits of the race it replaces, for the player whose sheet has both; and a feat
// that asks for that race still names it to them.
func TestAPlayerWhoseRaceIsOffKeepsTheRaceNamedInTheirCopies(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, dona, davi := h.newUser("Samuel"), h.newUser("Dona"), h.newUser("Davi")
	campaign := h.newCampaign(master, "Mirathel", dona, davi)
	race := testRace("Anão das Brumas")
	race.Traits = append(race.Traits, &rulesv1.TableFeature{NamePt: "Marca do vale", Effects: []*rulesv1.TableEffect{{Type: "note", TextPt: "Marca."}}})
	raceEntry := master.addEntry(t, campaign, race)
	trait := raceEntry.GetTableRace().GetTraits()[1].GetKey()
	sub := testSubrace("Da Colina", raceEntry.GetKey())
	sub.ReplacesTraitKeys = []string{trait}
	subKey := master.addEntry(t, campaign, sub).GetKey()
	feat := testFeat("Punho de ferro")
	feat.Prerequisite.RaceKey = raceEntry.GetKey()
	featKey := master.addEntry(t, campaign, feat).GetKey()
	bg := master.addEntry(t, campaign, testBackground("Guarda de farol")).GetKey()
	spell := master.addEntry(t, campaign, testSpell("Raio de teste", "class:wizard")).GetKey()
	sheet := sheetWithTable(raceEntry.GetKey(), bg, spell)
	sheet.GetFull().SubraceKey = subKey
	dona.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Icaro", sheet)
	master.setOff(t, campaign, true, raceEntry.GetKey())

	read := func(u *user) map[string]*rulesv1.TableEntry { return u.entries(t, campaign) }
	got := read(dona)
	if s := got[subKey]; s == nil || !slices.Equal(s.GetTableSubrace().GetReplacesTraitKeys(), []string{trait}) {
		t.Errorf("Dona's copy of the subrace = %v, want the trait of her race kept", s)
	}
	if f := got[featKey]; f == nil || f.GetTableFeat().GetPrerequisite().GetRaceKey() != raceEntry.GetKey() {
		t.Errorf("Dona's copy of the feat = %v, want her race kept", f)
	}
	// Davi, with no sheet, reads neither the subrace nor the feat of the hidden race.
	other := read(davi)
	if other[subKey] != nil || other[featKey] != nil {
		t.Errorf("Davi reads the subrace %v or the feat %v of a hidden race", other[subKey], other[featKey])
	}
}

// A feat's "lacks" condition on a hidden feature is blanked for a player unless the sheet has the feature.
func TestAFeatLacksConditionOnAFeatureTheSheetHasIsNotBlanked(t *testing.T) {
	t.Parallel()
	c := ownersContent(t, "class:ranger")
	feat := func() *rulesv1.FeatOption {
		return &rulesv1.FeatOption{
			Prerequisite:       &rulesv1.FeatPrerequisite{LacksFeatureKey: rangerFeature},
			LacksFeatureNamePt: "Olhar de teste",
			Unmet:              []*rulesv1.FeatUnmet{{Kind: rulesv1.FeatUnmetKind_FEAT_UNMET_KIND_LACKS_FEATURE, Key: rangerFeature, NamePt: "Olhar de teste"}},
		}
	}
	ranger := levelUpTarget{content: c, build: rules.Build{Classes: []rules.ClassLevel{{Class: "class:ranger", Level: 2}}}}
	wizard := levelUpTarget{content: c, build: rules.Build{Classes: []rules.ClassLevel{{Class: "class:wizard", Level: 2}}}}
	own, other := feat(), feat()
	hideFeatRace(own, ranger)
	hideFeatRace(other, wizard)
	if own.GetPrerequisite().GetLacksFeatureKey() != rangerFeature || own.GetLacksFeatureNamePt() == "" || own.GetUnmet()[0].GetKey() != rangerFeature {
		t.Errorf("the ranger's feat condition was blanked: %v", own)
	}
	if other.GetPrerequisite().GetLacksFeatureKey() != "" || other.GetLacksFeatureNamePt() != "" || other.GetUnmet()[0].GetKey() != "" || other.GetUnmet()[0].GetNamePt() != "" {
		t.Errorf("the wizard's feat condition names the hidden feature: %v", other)
	}
	if own.GetLacksFeatureHidden() || own.GetUnmet()[0].GetHidden() {
		t.Errorf("the ranger's feat condition is marked hidden: %v", own)
	}
	if !other.GetLacksFeatureHidden() || !other.GetUnmet()[0].GetHidden() {
		t.Errorf("the wizard's feat condition is not marked hidden: %v", other)
	}
}

// The marker says a condition names something the player may not read: set where the key and the name are
// blanked, never where the sheet keeps them or some race is still shown (the master's view skips the blanking).
func TestAFeatConditionOnAHiddenEntryIsMarkedHidden(t *testing.T) {
	t.Parallel()
	c := ownersContent(t, "race:elf", "race:dwarf")
	const lacks = rulesv1.FeatUnmetKind_FEAT_UNMET_KIND_LACKS_FEATURE
	const race = rulesv1.FeatUnmetKind_FEAT_UNMET_KIND_RACE
	for _, tc := range []struct {
		name       string
		unmet      *rulesv1.FeatUnmet
		build      rules.Build
		wantKey    string
		wantKeys   []string
		wantHidden bool
	}{
		{"one hidden race", &rulesv1.FeatUnmet{Kind: race, Key: "race:elf"}, rules.Build{}, "", nil, true},
		{"the sheet's own hidden race", &rulesv1.FeatUnmet{Kind: race, Key: "race:elf"}, rules.Build{Race: "race:elf"}, "race:elf", nil, false},
		{"every race of several hidden", &rulesv1.FeatUnmet{Kind: race, Keys: []string{"race:elf", "race:dwarf"}}, rules.Build{}, "", nil, true},
		{"one of several races shown", &rulesv1.FeatUnmet{Kind: race, Keys: []string{"race:elf", "race:human"}}, rules.Build{}, "", []string{"race:human"}, false},
		{"a race that is shown", &rulesv1.FeatUnmet{Kind: race, Key: "race:human"}, rules.Build{}, "race:human", nil, false},
		{"a hidden feature", &rulesv1.FeatUnmet{Kind: lacks, Key: rangerFeature, NamePt: "Olhar de teste"}, rules.Build{}, "", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			content := c
			if tc.unmet.GetKind() == lacks {
				content = ownersContent(t, "class:ranger")
			}
			f := &rulesv1.FeatOption{Unmet: []*rulesv1.FeatUnmet{tc.unmet}}
			hideFeatRace(f, levelUpTarget{content: content, build: tc.build})
			u := f.GetUnmet()[0]
			if u.GetKey() != tc.wantKey || !slices.Equal(u.GetKeys(), tc.wantKeys) || u.GetHidden() != tc.wantHidden {
				t.Errorf("unmet = key %q keys %q hidden %v, want key %q keys %q hidden %v", u.GetKey(), u.GetKeys(), u.GetHidden(), tc.wantKey, tc.wantKeys, tc.wantHidden)
			}
			if tc.wantHidden && u.GetNamePt() != "" {
				t.Errorf("the hidden condition keeps its name %q", u.GetNamePt())
			}
		})
	}
}

// The switches' list says a shared option is not hidden while a class that offers it is on.
func TestOptionSwitchesDoNotHideAnOptionAnotherClassOffers(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, dona := h.newUser("Samuel"), h.newUser("Dona")
	campaign := h.newCampaign(master, "Mirathel", dona)
	key := master.addEntry(t, campaign, &rulesv1.TableOption{NamePt: "Estilo comum", Set: "fighting_style"}).GetKey()
	master.setOff(t, campaign, true, "class:fighter")
	if o := master.options(t, campaign)[key]; o == nil || o.GetHidden() {
		t.Errorf("the option in the switches' list = %v, want it listed and not hidden", o)
	}
	if dona.entries(t, campaign)[key] == nil {
		t.Error("a player does not read the option")
	}
	master.setOff(t, campaign, true, "class:paladin", "class:ranger")
	if o := master.options(t, campaign)[key]; o == nil || !o.GetHidden() {
		t.Errorf("with every offering class off the option = %v, want it hidden", o)
	}
	if dona.entries(t, campaign)[key] != nil {
		t.Error("a player reads the option with every class that offers it off")
	}
}
