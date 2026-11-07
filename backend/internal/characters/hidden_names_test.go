package characters

import (
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// RN-23: nothing a player receives names a hidden entry (switched off or retired), in a key or in a text.
// The master reads the names, as the positive control.

const (
	hiddenRaceName   = "Anão das Brumas"
	hiddenTraitName  = "Pele de Bruma"
	hiddenFeatName   = "Dom Oculto de Névoa"
	visibleRaceName  = "Elfo Escarlate"
	hiddenRaceStem   = "anao-das-brumas"
	hiddenTraitStem  = "pele-de-bruma"
	hiddenFeatStem   = "dom-oculto-de-nevoa"
	onlyHiddenName   = "Linhagem Antiga"
	mixedName        = "Linhagem Mista"
	lacksTraitName   = "Sem a pele"
	lacksHiddenFeat  = "Sem o dom"
	openFeatName     = "Aberto ao vale"
	grantingFeatName = "Escolha do vale"
)

// hiddenNamesTable is a campaign where the master switched a race and a feat off, and registered feats that
// name them in their prerequisites.
type hiddenNamesTable struct {
	*levelUpTable
	race, feat, trait, onlyHidden, mixed, lacksTrait, lacksFeat, open string
	background                                                        string
}

func newHiddenNamesTable(t *testing.T) *hiddenNamesTable {
	t.Helper()
	tb := &hiddenNamesTable{levelUpTable: newLevelUpTable(t, 2700)}
	m := tb.master
	setRules(t, m, tb.campaign, func(r *campaignsv1.TableRules) { r.FeatsAllowed = true })
	race := testRace(hiddenRaceName)
	race.Traits[0].NamePt = hiddenTraitName
	raceEntry := m.addEntry(t, tb.campaign, race)
	tb.race, tb.trait = raceEntry.GetKey(), raceEntry.GetTableRace().GetTraits()[0].GetKey()
	scarlet := m.addEntry(t, tb.campaign, testRace(visibleRaceName)).GetKey()
	tb.feat = m.addEntry(t, tb.campaign, &rulesv1.TableFeat{NamePt: hiddenFeatName, DescPt: []string{"Escondido."}}).GetKey()
	feat := func(name string, p *rulesv1.FeatPrerequisite) string {
		return m.addEntry(t, tb.campaign, &rulesv1.TableFeat{NamePt: name, DescPt: []string{"Texto."}, Prerequisite: p}).GetKey()
	}
	tb.onlyHidden = feat(onlyHiddenName, &rulesv1.FeatPrerequisite{Races: []string{tb.race}})
	tb.mixed = feat(mixedName, &rulesv1.FeatPrerequisite{Races: []string{tb.race, scarlet}, LacksFeatureKey: tb.feat})
	tb.lacksTrait = feat(lacksTraitName, &rulesv1.FeatPrerequisite{LacksFeatureKey: tb.trait})
	tb.lacksFeat = feat(lacksHiddenFeat, &rulesv1.FeatPrerequisite{LacksFeatureKey: tb.feat})
	tb.open = feat(openFeatName, nil)
	// A background whose feature lets the player pick a feat when the sheet is made.
	bg := testBackground("Herdeiro do vale")
	bg.Feature = &rulesv1.TableFeature{
		NamePt: grantingFeatName, DescPt: []string{"Um talento à escolha."},
		Effects: []*rulesv1.TableEffect{{Type: "choice", Choice: "feat", Count: 1}},
	}
	tb.background = m.addEntry(t, tb.campaign, bg).GetKey()
	m.setOff(t, tb.campaign, true, tb.race, tb.feat)
	return tb
}

// hiddenWords are what no player payload may carry: the keys' stems and the Portuguese names.
var hiddenWords = []string{
	hiddenRaceName, hiddenTraitName, hiddenFeatName, hiddenRaceStem, hiddenTraitStem, hiddenFeatStem,
}

// wantNames fails for each word the message does not carry (present) or carries (not present), searched in its prototext.
func wantNames(t *testing.T, what string, m proto.Message, present bool, words ...string) {
	t.Helper()
	text := prototext.MarshalOptions{Multiline: true}.Format(m)
	for _, w := range words {
		if has := strings.Contains(text, w); has != present {
			t.Errorf("%s: carries %q = %v, want %v\n%s", what, w, has, present, text)
		}
	}
}

func TestAPlayersLevelUpOptionsNameNoHiddenEntry(t *testing.T) {
	t.Parallel()
	tb := newHiddenNamesTable(t)
	player, err := tb.options(tb.owner, tb.pc)
	if err != nil {
		t.Fatal(err)
	}
	master, err := tb.options(tb.master, tb.pc)
	if err != nil {
		t.Fatal(err)
	}
	// The positive control: the master reads the hidden race, trait and feat in the feats that name them.
	wantNames(t, "the master's options", master, true, hiddenFeatName, hiddenTraitName, tb.race, tb.feat, tb.trait)
	wantNames(t, "the player's options", player, false, hiddenWords...)
	wantNames(t, "the player's options", player, false, tb.race, tb.feat, tb.trait)
	// The player is offered the feats they can read, and the one with a visible race says only that race.
	if optionOf(player, tb.open) == nil || optionOf(player, tb.lacksTrait) == nil || optionOf(player, tb.mixed) == nil {
		t.Errorf("the player's feats lack a live one: %v", player.GetFeats())
	}
	// A feat that asks only for a hidden race is not offered to them (their prerequisite would read as none), nor hinted at.
	if optionOf(player, tb.onlyHidden) != nil || optionOf(master, tb.onlyHidden) == nil {
		t.Errorf("the feat of the hidden race: player %v, master %v; want the master's only", optionOf(player, tb.onlyHidden), optionOf(master, tb.onlyHidden))
	}
	wantNames(t, "the player's options", player, false, onlyHiddenName, tb.onlyHidden)
	// The condition on a hidden feat or trait reads as hidden to the player (no key, no name) and as itself to the master.
	for _, tc := range []struct{ name, key, feature string }{{"hidden feat", tb.lacksFeat, tb.feat}, {"hidden trait", tb.lacksTrait, tb.trait}} {
		got, control := optionOf(player, tc.key), optionOf(master, tc.key)
		if got.GetPrerequisite().GetLacksFeatureKey() != "" || got.GetLacksFeatureNamePt() != "" || !got.GetLacksFeatureHidden() {
			t.Errorf("%s: the player's option = key %q name %q hidden %v, want empty and hidden", tc.name, got.GetPrerequisite().GetLacksFeatureKey(), got.GetLacksFeatureNamePt(), got.GetLacksFeatureHidden())
		}
		if control.GetPrerequisite().GetLacksFeatureKey() != tc.feature || control.GetLacksFeatureNamePt() == "" || control.GetLacksFeatureHidden() {
			t.Errorf("%s: the master's option = key %q name %q hidden %v, want the key and the name, not hidden", tc.name, control.GetPrerequisite().GetLacksFeatureKey(), control.GetLacksFeatureNamePt(), control.GetLacksFeatureHidden())
		}
	}
	if got := optionOf(player, tb.mixed).GetPrerequisite().GetRaces(); len(got) != 1 {
		t.Errorf("the mixed feat's races for the player = %v, want only the visible one", got)
	}
}

func TestAPlayersCopyOfAFeatNamesNoHiddenEntry(t *testing.T) {
	t.Parallel()
	tb := newHiddenNamesTable(t)
	list := func(u *user) *rulesv1.ListTableEntriesResponse {
		res, err := u.table.ListTableEntries(t.Context(), connect.NewRequest(&rulesv1.ListTableEntriesRequest{CampaignId: tb.campaign}))
		if err != nil {
			t.Fatal(err)
		}
		return res.Msg
	}
	master, player := list(tb.master), list(tb.owner)
	wantNames(t, "the master's entries", master, true, hiddenRaceName, hiddenFeatName, tb.race, tb.feat, tb.trait)
	wantNames(t, "the player's entries", player, false, hiddenWords...)
	wantNames(t, "the player's entries", player, false, tb.race, tb.feat, tb.trait, onlyHiddenName)
	wantNames(t, "the player's entries", player, true, mixedName, lacksTraitName, openFeatName)
	// The condition on a hidden feature reads as hidden (and keyless) to the player, as itself to the master.
	lacks := func(res *rulesv1.ListTableEntriesResponse, key string) *rulesv1.FeatPrerequisite {
		for _, e := range res.GetEntries() {
			if e.GetKey() == key {
				return e.GetTableFeat().GetPrerequisite()
			}
		}
		return nil
	}
	for _, key := range []string{tb.lacksTrait, tb.lacksFeat, tb.mixed} {
		if p := lacks(player, key); p == nil || !p.GetLacksHidden() || p.GetLacksFeatureKey() != "" {
			t.Errorf("the player's condition of %s = %v, want hidden and keyless", key, p)
		}
		if p := lacks(master, key); p == nil || p.GetLacksHidden() || p.GetLacksFeatureKey() == "" {
			t.Errorf("the master's condition of %s = %v, want the key, not hidden", key, p)
		}
	}
	// The name of a lacked feature the player reads comes from the server, never a key; the master reads it too.
	for _, key := range []string{tb.lacksTrait, tb.lacksFeat, tb.mixed} {
		if p := lacks(player, key); p.GetLacksFeatureNamePt() != "" || p.GetRacesHidden() {
			t.Errorf("the player's condition of %s names the hidden feature or marks races: %v", key, p)
		}
		if p := lacks(master, key); p.GetLacksFeatureNamePt() == "" || p.GetRacesHidden() {
			t.Errorf("the master's condition of %s = %v, want the name and no races mark", key, p)
		}
	}
	if p := lacks(player, tb.open); p.GetLacksHidden() {
		t.Errorf("a feat with no condition reads as hidden: %v", p)
	}
}

// The mark is the server's: a master who sends it stores nothing of it.
func TestTheLacksHiddenMarkIsNeverStored(t *testing.T) {
	t.Parallel()
	tb := newHiddenNamesTable(t)
	key := tb.master.addEntry(t, tb.campaign, &rulesv1.TableFeat{
		NamePt: "Marcado a mão", DescPt: []string{"Texto."},
		Prerequisite: &rulesv1.FeatPrerequisite{LacksFeatureKey: tb.trait, LacksHidden: true},
	}).GetKey()
	got := tb.master.entries(t, tb.campaign)[key].GetTableFeat().GetPrerequisite()
	if got.GetLacksHidden() || got.GetLacksFeatureKey() != tb.trait {
		t.Errorf("the stored condition = %v, want the key and no mark", got)
	}
}

func TestAPlayersFeatCardsAtCreationNameNoHiddenEntry(t *testing.T) {
	t.Parallel()
	tb := newHiddenNamesTable(t)
	sheet := pensantusSheet()
	sheet.GetFull().Background = &charactersv1.FullSheet_BackgroundKey{BackgroundKey: tb.background}
	preview := func(u *user, kind charactersv1.CharacterKind) *charactersv1.PreviewChoicesResponse {
		res, err := u.api.PreviewChoices(t.Context(), connect.NewRequest(&charactersv1.PreviewChoicesRequest{
			CampaignId: tb.campaign, Kind: kind, Sheet: sheet,
		}))
		if err != nil {
			t.Fatalf("PreviewChoices() error = %v", err)
		}
		return res.Msg
	}
	master := preview(tb.master, charactersv1.CharacterKind_CHARACTER_KIND_ENEMY)
	player := preview(tb.owner, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER)
	// The master reads the feat that names the hidden race and the hidden feat; the player, neither the names nor the feat of the race.
	wantNames(t, "the master's cards", master, true, mixedName, onlyHiddenName, hiddenFeatName, hiddenRaceName)
	wantNames(t, "the player's cards", player, false, hiddenWords...)
	wantNames(t, "the player's cards", player, false, tb.race, tb.feat, onlyHiddenName)
	wantNames(t, "the player's cards", player, true, mixedName, openFeatName, "Pede: ser "+visibleRaceName+" e uma condição que a mesa não mostra.", "Só para "+visibleRaceName+".")
}

// A hidden owner hides its features and options from a player, except where the player's own sheet has the owner
// (a retired class keeps working for who already has it) and the option itself is not retired.
func TestAnOptionOfAHiddenOwnerIsKeptWhereTheSheetHasTheOwner(t *testing.T) {
	t.Parallel()
	srd, err := rules.LoadSRD()
	if err != nil {
		t.Fatal(err)
	}
	const feature = "feature:fighter-fighting-style-defense"
	content, err := srd.With(rules.Overlay{Revision: 1, Off: []string{"class:fighter"}})
	if err != nil {
		t.Fatal(err)
	}
	fighter := levelUpTarget{content: content, build: rules.Build{Classes: []rules.ClassLevel{{Class: "class:fighter", Level: 2}}}}
	wizard := levelUpTarget{content: content, build: rules.Build{Classes: []rules.ClassLevel{{Class: "class:wizard", Level: 2}}}}
	if !content.Hidden(feature) {
		t.Fatalf("Hidden(%s) = false with the fighter off", feature)
	}
	if hiddenOption(fighter, feature) {
		t.Errorf("the fighter's own option %s is hidden from the fighter", feature)
	}
	if !hiddenOption(wizard, feature) {
		t.Errorf("the fighter's option %s is not hidden from a wizard", feature)
	}
	// In the listing, what a sheet's entry owns is read by that player and not by another.
	kept := (&tableListing{content: content, hidden: content.Hidden, mine: map[string]bool{"class:fighter": true}}).hiddenIn()
	other := (&tableListing{content: content, hidden: content.Hidden, mine: map[string]bool{"class:wizard": true}}).hiddenIn()
	if kept(feature) || !other(feature) {
		t.Errorf("hidden for the fighter = %v, for the wizard = %v; want false and true", kept(feature), other(feature))
	}
}

// An option the master switched off is named, in no payload, to a fighter who never picked it; the master reads it (the sheet that picked it is the rules test's control).
func TestASwitchedOffOptionIsNotNamedToAFighterWhoNeverPickedIt(t *testing.T) {
	t.Parallel()
	tb := newLevelUpTable(t, 2700)
	m := tb.master
	setRules(t, m, tb.campaign, func(r *campaignsv1.TableRules) { r.FeatsAllowed = true })
	const name, stem = "Guarda Dupla de Teste", "guarda-dupla-de-teste"
	option := m.addEntry(t, tb.campaign, &rulesv1.TableOption{NamePt: name, Set: "fighting_style"}).GetKey()
	feat := m.addEntry(t, tb.campaign, &rulesv1.TableFeat{NamePt: "Sem guarda", DescPt: []string{"Texto."}, Prerequisite: &rulesv1.FeatPrerequisite{LacksFeatureKey: option}}).GetKey()
	m.setOff(t, tb.campaign, true, option)
	fighter := tb.other.create(t, tb.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Toren", levelUpFighterSheet(3, 2700, "subclass:champion"))
	player, err := tb.options(tb.other, fighter)
	if err != nil {
		t.Fatal(err)
	}
	master, err := tb.options(m, fighter)
	if err != nil {
		t.Fatal(err)
	}
	wantNames(t, "the master's options", master, true, name, stem)
	wantNames(t, "the fighter's options", player, false, name, stem)
	if optionOf(player, feat) == nil {
		t.Error("the fighter is not offered the feat that asks to lack the option")
	}
}

// A feature the player may read is named in Portuguese on the feat's read, for the master and the player alike.
func TestAFeatNamesAVisibleLackedFeatureOnRead(t *testing.T) {
	t.Parallel()
	tb := newHiddenNamesTable(t)
	key := tb.master.addEntry(t, tb.campaign, &rulesv1.TableFeat{
		NamePt: "Sem agarrar", DescPt: []string{"Texto."},
		Prerequisite: &rulesv1.FeatPrerequisite{LacksFeatureKey: "feat:grappler", LacksFeatureNamePt: "Gravado"},
	}).GetKey()
	for who, u := range map[string]*user{"master": tb.master, "player": tb.owner} {
		p := u.entries(t, tb.campaign)[key].GetTableFeat().GetPrerequisite()
		if p.GetLacksFeatureKey() != "feat:grappler" || p.GetLacksFeatureNamePt() == "" || p.GetLacksFeatureNamePt() == "Gravado" || p.GetLacksHidden() {
			t.Errorf("the %s's condition = %v, want the key and the server's name", who, p)
		}
	}
	// The name is a read-only field: what the master sent is not stored.
	if got := tb.master.entries(t, tb.campaign)[key].GetTableFeat().GetPrerequisite().GetLacksFeatureNamePt(); got == "Gravado" {
		t.Errorf("the stored name leaked back: %q", got)
	}
}

// races_hidden is the player's mark: set when no asked race is left after the filter, never for the one shown.
func TestStripFeatMarksRacesHiddenOnlyWhenNoneIsLeft(t *testing.T) {
	t.Parallel()
	hidden := func(k string) bool { return k == "race:oculta@mesa" || k == "race:outra@mesa" }
	for name, tc := range map[string]struct {
		in   *rulesv1.FeatPrerequisite
		want bool
	}{
		"the single race hidden":                     {&rulesv1.FeatPrerequisite{RaceKey: "race:oculta@mesa"}, true},
		"every listed race hidden":                   {&rulesv1.FeatPrerequisite{Races: []string{"race:oculta@mesa", "race:outra@mesa"}}, true},
		"one listed race shown":                      {&rulesv1.FeatPrerequisite{Races: []string{"race:oculta@mesa", "race:elf"}}, false},
		"no race asked":                              {&rulesv1.FeatPrerequisite{Level: 3}, false},
		"the single race hidden, a shown one listed": {&rulesv1.FeatPrerequisite{RaceKey: "race:oculta@mesa", Races: []string{"race:elf"}}, false},
	} {
		feat := &rulesv1.TableFeat{NamePt: "T", Prerequisite: tc.in}
		playerView{hidden: hidden}.stripFeat(feat)
		if got := feat.GetPrerequisite().GetRacesHidden(); got != tc.want {
			t.Errorf("%s: races_hidden = %v, want %v", name, got, tc.want)
		}
	}
}

// What only a read sets is zeroed on a write and on a pack's import.
func TestReadMarksAreZeroedOnWrite(t *testing.T) {
	t.Parallel()
	feat := &rulesv1.TableFeat{Prerequisite: &rulesv1.FeatPrerequisite{LacksHidden: true, RacesHidden: true, LacksFeatureNamePt: "X", Level: 2}}
	clearReadMarks(feat)
	if p := feat.GetPrerequisite(); p.GetLacksHidden() || p.GetRacesHidden() || p.GetLacksFeatureNamePt() != "" || p.GetLevel() != 2 {
		t.Errorf("after clearReadMarks: %v", p)
	}
}
