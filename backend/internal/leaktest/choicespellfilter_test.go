package leaktest

import (
	"slices"
	"strings"
	"testing"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1/rulesv1connect"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// A choice of spells that offers the spells of some class lists (the filter): the pool is the reader's, and a pick of a
// spell the table keeps from the reader is answered as a pick of a spell that does not exist. Every name here is invented.

// spellNamePT is the Portuguese name of an SRD spell.
func spellNamePT(t *testing.T, key string) string {
	t.Helper()
	srd, err := rules.LoadSRD()
	if err != nil {
		t.Fatalf("LoadSRD: %v", err)
	}
	for _, e := range srd.Catalog().Spells {
		if e.Key == key {
			return e.NamePT
		}
	}
	t.Fatalf("no spell %s", key)
	return ""
}

// tableSpellOf makes a table spell of the circle on the lists of the classes, and gives its entry.
func (p *choicesProbe) tableSpellOf(t *testing.T, name string, level int32, classes ...string) *rulesv1.TableEntry {
	t.Helper()
	sp := &rulesv1.TableSpell{
		NamePt: name, Level: level, SchoolKey: "school:evocation",
		CastingTime: &rulesv1.TableSpellCastingTime{Unit: rulesv1.CastingTimeUnit_CASTING_TIME_UNIT_ACTION, Amount: 1},
		Range:       &rulesv1.TableSpellRange{Kind: rulesv1.SpellRangeKind_SPELL_RANGE_KIND_SELF},
		Duration:    &rulesv1.TableSpellDuration{Kind: rulesv1.SpellDurationKind_SPELL_DURATION_KIND_INSTANTANEOUS},
		Components:  &rulesv1.TableSpellComponents{Verbal: true},
		ClassKeys:   classes, DescPt: []string{"Um texto."},
		Target: &rulesv1.TableSpellTarget{Kind: rulesv1.TableSpellTargetKind_TABLE_SPELL_TARGET_KIND_SELF},
	}
	return must(p.w.master.table.CreateTableEntry(t.Context(), rq(&rulesv1.CreateTableEntryRequest{
		CampaignId: p.w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableSpell{TableSpell: sp},
	}))).GetEntry()
}

// spellFilterWorld is a campaign with a table class that offers a spell of the lists of the wizard and of another table
// class, from the 3rd and the 1st circles; the table class with the second list, and the class that asks.
type spellFilterWorld struct {
	p      *choicesProbe
	list   *rulesv1.TableEntry // the table class whose list is named
	asking *rulesv1.TableEntry
	ask    string // the key of the choice
}

// spellKind is a choice of spells (circles 1 and 3 of two lists) or of cantrips (the cantrips of the same lists), with a
// spell of the wizard's list the table switches off, and one it leaves be.
type spellKind struct {
	choice         string
	levels         []int32
	off, on        string
	circle, listed int32 // the circle of the table spells the rows make: on the wizard's list and on the other
}

var (
	spellsKind   = spellKind{"spell", []int32{1, 3}, "spell:fireball", "spell:lightning-bolt", 3, 1}
	cantripsKind = spellKind{"cantrip", nil, "spell:fire-bolt", "spell:mage-hand", 0, 0}
)

const (
	wizardSpellOff = "spell:fireball"
	wizardSpellOn  = "spell:lightning-bolt"
)

func newSpellFilterWorld(t *testing.T, kind spellKind) *spellFilterWorld {
	t.Helper()
	p := newChoicesProbe(t)
	w := p.w
	must(w.master.campaigns.SetCampaignXpMode(t.Context(), rq(&campaignsv1.SetCampaignXpModeRequest{CampaignId: w.campaign, XpMode: campaignsv1.XpMode_XP_MODE_ENEMIES})))
	list := p.createEntry(t, tableClass("Escriba do Cais"))
	asking := tableClass("Estudioso do Cais")
	asking.SubclassLevel = 5 // the level-ups of the rows stop short of it
	asking.Levels[1].Features = []*rulesv1.TableFeature{{
		NamePt: "Estudo do cais", DescPt: []string{"Texto de teste."},
		Effects: []*rulesv1.TableEffect{{Type: "choice", Choice: kind.choice, Count: 1, Filter: &rulesv1.TableChoiceFilter{
			ClassLists: []string{"class:wizard", list.GetKey()}, SpellLevels: kind.levels,
		}}},
	}}
	made := p.createEntry(t, asking)
	return &spellFilterWorld{p: p, list: list, asking: made, ask: made.GetTableClass().GetLevels()[1].GetFeatures()[0].GetKey()}
}

func (sw *spellFilterWorld) stored(spell string) string { return sw.ask + "=" + spell }

// poolOf are the keys of the spells the reader's preview of the asking class at the level offers, nothing for a reader the
// campaign does not let in.
func (sw *spellFilterWorld) poolOf(t *testing.T, who reader) (keys []string, body string) {
	t.Helper()
	r := sw.p.preview(who, sheet(sw.asking.GetKey(), 2))
	if who.member != r.ok() {
		t.Fatalf("%s: HTTP %d for the plain sheet, want a read only for a member", who.name, r.status)
	}
	if !who.member {
		return nil, string(r.body)
	}
	return optionsOf(choicesOf(t, r), sw.ask), string(r.body)
}

// The pool of a filter of spells is the same for every player, whatever the master hid from them: no player is offered a spell
// the master switched off or retired, nor one only a list they cannot read has, and the answer names none of them. The master
// reads what is switched off.
func TestAFilterOfSpellsOffersTheSamePoolToEveryPlayer(t *testing.T) {
	sw := newSpellFilterWorld(t, spellsKind)
	p, w := sw.p, sw.p.w
	onlyOnList := p.tableSpellOf(t, "Estudo da lista", spellsKind.listed, sw.list.GetKey())
	retired := p.tableSpellOf(t, "Estudo aposentado", spellsKind.circle, "class:wizard")
	pool := func(who reader) []string {
		keys, _ := sw.poolOf(t, who)
		return keys
	}
	readers := p.readers()
	// Before the master closes anything, everyone who reads is offered the same.
	var open []string
	for _, who := range readers {
		if who.member {
			open = pool(who)
			break
		}
	}
	for _, spell := range []string{wizardSpellOff, wizardSpellOn, onlyOnList.GetKey(), retired.GetKey()} {
		if !slices.Contains(open, spell) {
			t.Fatalf("before the table closes anything the pool lacks %s: %v", spell, open)
		}
	}
	must(w.master.table.ArchiveTableEntry(t.Context(), rq(&rulesv1.ArchiveTableEntryRequest{CampaignId: w.campaign, Key: retired.GetKey()})))
	must(w.master.table.SetOptionSwitches(t.Context(), rq(&rulesv1.SetOptionSwitchesRequest{CampaignId: w.campaign, Switches: []*rulesv1.OptionSwitch{
		{Key: wizardSpellOff, Off: true}, {Key: sw.list.GetKey(), Off: true},
	}})))
	hidden := []string{wizardSpellOff, onlyOnList.GetKey(), retired.GetKey()}
	names := []string{spellNamePT(t, wizardSpellOff), "Estudo da lista", "Estudo aposentado"}
	var players []string
	for _, who := range readers {
		keys, body := sw.poolOf(t, who)
		switch {
		case !who.member:
		case who.person == w.master:
			// The control: the master reads what is switched off, and what is retired closes for everyone.
			if !slices.Contains(keys, wizardSpellOff) || !slices.Contains(keys, onlyOnList.GetKey()) || slices.Contains(keys, retired.GetKey()) {
				t.Errorf("the master's pool %v, want what is switched off read and what is retired closed", keys)
			}
		default:
			for i, k := range hidden {
				if slices.Contains(keys, k) || strings.Contains(body, k) || strings.Contains(body, names[i]) {
					t.Errorf("%s: the answer names %s, which the table keeps from the players", who.name, k)
				}
			}
			if !slices.Contains(keys, wizardSpellOn) {
				t.Errorf("%s: the pool %v lacks %s, which nothing closed", who.name, keys, wizardSpellOn)
			}
			if players == nil {
				players = keys
			} else if !slices.Equal(players, keys) {
				t.Errorf("%s: the pool differs from the other players' (%d against %d spells)", who.name, len(keys), len(players))
			}
		}
	}
}

// The filter a player reads in the entries names no class list they cannot read.
func TestTheFilterOfSpellsAPlayerListsNamesNoClassListTheyCannotRead(t *testing.T) {
	sw := newSpellFilterWorld(t, spellsKind)
	w := sw.p.w
	list := &rulesv1.ListTableEntriesRequest{CampaignId: w.campaign}
	listed := func(who *person) string {
		r := who.call(rulesv1connect.TableContentServiceListTableEntriesProcedure, list)
		if !r.ok() {
			t.Fatalf("%s: HTTP %d", who.name, r.status)
		}
		return string(r.body)
	}
	if !strings.Contains(listed(w.caio), sw.list.GetKey()) {
		t.Fatalf("before the table closes the class, Caio's entries do not name %s", sw.list.GetKey())
	}
	must(w.master.table.SetOptionSwitches(t.Context(), rq(&rulesv1.SetOptionSwitchesRequest{CampaignId: w.campaign, Switches: []*rulesv1.OptionSwitch{{Key: sw.list.GetKey(), Off: true}}})))
	if !strings.Contains(listed(w.master), sw.list.GetKey()) {
		t.Errorf("the master's entries do not name %s", sw.list.GetKey())
	}
	for _, who := range []*person{w.ana, w.caio} {
		if got := listed(who); strings.Contains(got, sw.list.GetKey()) || !strings.Contains(got, sw.asking.GetKey()) {
			t.Errorf("%s: the entries name %s (or leave out the asking class)", who.name, sw.list.GetKey())
		}
	}
}

// The lists of the menu are the master's: a player is refused the menu, and the master reads the schools and the class lists.
func TestTheSchoolsAndTheClassListsOfTheMenuAreTheMasters(t *testing.T) {
	sw := newSpellFilterWorld(t, spellsKind)
	w := sw.p.w
	req := &rulesv1.GetEffectMenuRequest{CampaignId: w.campaign}
	for _, who := range []*person{w.ana, w.caio, w.pending, w.stranger} {
		if r := who.call(rulesv1connect.TableContentServiceGetEffectMenuProcedure, req); r.ok() {
			t.Errorf("%s: HTTP %d, want the menu refused", who.name, r.status)
		} else if strings.Contains(string(r.body), "spell_schools") || strings.Contains(string(r.body), "class_lists") {
			t.Errorf("%s: the refusal names a list of the menu", who.name)
		}
	}
	r := w.master.call(rulesv1connect.TableContentServiceGetEffectMenuProcedure, req)
	if !r.ok() {
		t.Fatalf("the master: HTTP %d %s", r.status, shorten(r.body))
	}
	for _, want := range []string{"spell_schools", "class_lists", "school:evocation", "class:wizard"} {
		if !strings.Contains(string(r.body), want) {
			t.Errorf("the master's menu lacks %s", want)
		}
	}
}

// How the table keeps a spell out of a reader's offer.
var spellWays = []string{"a spell is switched off", "a spell is retired", "a class list is switched off", "a class list is retired"}

// newSpellTides is the pick of a spell the table keeps out, in the rows of the options of a feature: the asking class's
// choice at level 2, the spells the table closes, and the spell a sheet holds before it does.
func newSpellTides(t *testing.T, way string, kind spellKind) *tides {
	t.Helper()
	sw := newSpellFilterWorld(t, kind)
	p, w := sw.p, sw.p.w
	td := &tides{
		p: p, asking: sw.asking.GetKey(), ask: sw.ask, live: sw.stored(kind.on), absent: sw.stored("spell:nao-existe"),
		option: func(stored string) string { return strings.TrimPrefix(stored, sw.ask+"=") },
	}
	closeSwitch := func(key string) func(*testing.T) {
		return func(t *testing.T) {
			t.Helper()
			must(w.master.table.SetOptionSwitches(t.Context(), rq(&rulesv1.SetOptionSwitchesRequest{CampaignId: w.campaign, Switches: []*rulesv1.OptionSwitch{{Key: key, Off: true}}})))
		}
	}
	retire := func(key string) func(*testing.T) {
		return func(t *testing.T) {
			t.Helper()
			must(w.master.table.ArchiveTableEntry(t.Context(), rq(&rulesv1.ArchiveTableEntryRequest{CampaignId: w.campaign, Key: key})))
		}
	}
	switch way {
	case spellWays[0]:
		td.out, td.names, td.close, td.liveStays = []string{sw.stored(kind.off)}, []string{spellNamePT(t, kind.off)}, closeSwitch(kind.off), true
	case spellWays[1]:
		spell := p.tableSpellOf(t, "Estudo aposentado", kind.circle, "class:wizard")
		td.out, td.names, td.close, td.archived, td.liveStays = []string{sw.stored(spell.GetKey())}, []string{"Estudo aposentado"}, retire(spell.GetKey()), true, true
	default:
		spell := p.tableSpellOf(t, "Estudo da lista", kind.listed, sw.list.GetKey())
		td.out, td.names, td.liveStays = []string{sw.stored(spell.GetKey())}, []string{"Estudo da lista"}, true
		td.close = closeSwitch(sw.list.GetKey())
		if way == spellWays[3] {
			td.close, td.archived = retire(sw.list.GetKey()), true
		}
	}
	return td
}

// A pick of a spell the table keeps out of the offer is answered as a pick of a spell that does not exist, by every call
// that takes a pick and for every reader the call lets in. The master reads what is switched off, and is answered so only
// for what is retired. A pick the sheet held before the table closed the spell stays.
func TestAPickOfASpellTheTableKeepsOutIsAnsweredAsAKeyThatDoesNotExist(t *testing.T) {
	for _, way := range spellWays {
		t.Run(way, func(t *testing.T) {
			td := newSpellTides(t, way, spellsKind)
			p, w := td.p, td.p.w
			ctx := t.Context()
			for _, who := range p.readers() {
				if r := p.preview(who, td.sheetAt(2, td.live)); who.member && (!r.ok() || len(choicesOf(t, r).GetNotOffered()) != 0) {
					t.Fatalf("%s: the pick of %s before the table closes anything: HTTP %d %s", who.name, td.live, r.status, shorten(r.body))
				}
			}
			holder := must(w.ana.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
				CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Estudiosa", Sheet: td.sheetAt(2, td.live),
			}))).GetCharacter()
			rising := must(w.caio.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
				CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Estudioso", Sheet: td.sheetAt(1),
			}))).GetCharacter()
			if r := w.caio.call(charactersv1connect.CharacterServicePreviewLevelUpProcedure, td.levelUp(rising, td.live)); !r.ok() {
				t.Fatalf("the level-up with %s before the table closes anything: HTTP %d %s", td.live, r.status, shorten(r.body))
			}

			td.close(t)

			td.checkWhatReadersSee(t, holder, rising)
			td.checkDrafts(t)
			td.checkSavedSheet(t, holder)
			td.checkLevelUp(t, rising)
			td.checkHeldPick(t, holder)
			td.checkCompletion(t, holder)
		})
	}
}

// The same for a choice of cantrips with a filter, in every call that takes a sheet: the pick of a cantrip the table keeps
// out is a pick of a cantrip that does not exist, and the one a sheet held stays.
func TestAPickOfACantripTheTableKeepsOutIsAnsweredAsAKeyThatDoesNotExist(t *testing.T) {
	for _, way := range spellWays {
		t.Run(way, func(t *testing.T) {
			td := newSpellTides(t, way, cantripsKind)
			p, w := td.p, td.p.w
			ctx := t.Context()
			holder := must(w.ana.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
				CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Estudiosa", Sheet: td.sheetAt(2, td.live),
			}))).GetCharacter()
			rising := must(w.caio.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
				CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Estudioso", Sheet: td.sheetAt(1),
			}))).GetCharacter()
			for _, who := range p.readers() {
				if r := p.preview(who, td.sheetAt(2, td.live)); who.member && (!r.ok() || len(choicesOf(t, r).GetNotOffered()) != 0) {
					t.Fatalf("%s: the pick of %s before the table closes anything: HTTP %d %s", who.name, td.live, r.status, shorten(r.body))
				}
			}

			td.close(t)

			td.checkWhatReadersSee(t, holder, rising)
			td.checkDrafts(t)
			td.checkSavedSheet(t, holder)
			td.checkHeldPick(t, holder)
			td.checkCompletion(t, holder)
		})
	}
}
