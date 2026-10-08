package characters

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
)

// The master's switches, "Opções para os jogadores" (MR-025, RN-23, ADR-0018): each
// class, subclass, race, subrace, background and spell, the SRD's and the table's,
// is on or off for the players, and the live hint tells the open screens.

func switchReq(campaign string, off bool, keys ...string) *rulesv1.SetOptionSwitchesRequest {
	r := &rulesv1.SetOptionSwitchesRequest{CampaignId: campaign}
	for _, k := range keys {
		r.Switches = append(r.Switches, &rulesv1.OptionSwitch{Key: k, Off: off})
	}
	return r
}

// setOff switches the options off (or on) as u, or fails the test.
func (u *user) setOff(t *testing.T, campaign string, off bool, keys ...string) *rulesv1.SetOptionSwitchesResponse {
	t.Helper()
	res, err := u.table.SetOptionSwitches(t.Context(), connect.NewRequest(switchReq(campaign, off, keys...)))
	if err != nil {
		t.Fatalf("SetOptionSwitches(off=%v, %v) error = %v", off, keys, err)
	}
	return res.Msg
}

// options lists the master's switches by key.
func (u *user) options(t *testing.T, campaign string) map[string]*rulesv1.OptionSwitchEntry {
	t.Helper()
	res, err := u.table.ListOptionSwitches(t.Context(), connect.NewRequest(&rulesv1.ListOptionSwitchesRequest{CampaignId: campaign}))
	if err != nil {
		t.Fatalf("ListOptionSwitches() error = %v", err)
	}
	out := map[string]*rulesv1.OptionSwitchEntry{}
	for _, e := range res.Msg.GetOptions() {
		out[e.GetKey()] = e
	}
	return out
}

func (u *user) catalog(t *testing.T, campaign string) *rulesv1.ListContentResponse {
	t.Helper()
	res, err := u.content.ListContent(t.Context(), connect.NewRequest(&rulesv1.ListContentRequest{CampaignId: campaign}))
	if err != nil {
		t.Fatalf("ListContent() error = %v", err)
	}
	return res.Msg
}

func jsonOf(t *testing.T, m proto.Message) string {
	t.Helper()
	b, err := protojson.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// mentions says whether the JSON of a response names the key, as a string value.
func mentions(js, key string) bool { return strings.Contains(js, `"`+key+`"`) }

func (u *user) spellPage(t *testing.T, campaign, query string) *rulesv1.ListSpellsResponse {
	t.Helper()
	res, err := u.content.ListSpells(t.Context(), connect.NewRequest(&rulesv1.ListSpellsRequest{CampaignId: campaign, Query: query, PageSize: 400}))
	if err != nil {
		t.Fatalf("ListSpells() error = %v", err)
	}
	return res.Msg
}

// TestRN23_SwitchedOffOptionsAreNeverSeenByPlayers: an SRD class, an SRD spell and a
// table race are switched off; no read a player makes mentions them, the master reads
// all of them flagged, and an off class takes its subclasses with it.
func TestRN23_SwitchedOffOptionsAreNeverSeenByPlayers(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, owner, other := h.newUser("Samuel"), h.newUser("Dona"), h.newUser("Outra")
	campaign := h.newCampaign(master, "Mirathel", owner, other)
	race := master.addEntry(t, campaign, testRace("Anão do Mar"))
	subrace := master.addEntry(t, campaign, testSubrace("Do cais", race.GetKey()))
	spell := master.addEntry(t, campaign, testSpell("Raio de teste", "class:wizard"))
	// Dona's wizard uses the SRD class; it counts as one user of it.
	owner.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus", pensantusSheet())

	res := master.setOff(t, campaign, true, "class:wizard", "spell:fireball", race.GetKey())
	if res.GetChanged() != 3 || res.GetTableRevision() != 4 {
		t.Errorf("changed = %d, revision = %d, want 3 and 4 (three entries, then the switch)", res.GetChanged(), res.GetTableRevision())
	}
	hiddenKeys := []string{"class:wizard", "spell:fireball", race.GetKey(), subrace.GetKey(), "subclass:evocation"}

	// The catalog: every player gets one without them, in any reference.
	for _, who := range []*user{owner, other} {
		cat := who.catalog(t, campaign)
		js := jsonOf(t, cat)
		for _, k := range hiddenKeys {
			if mentions(js, k) {
				t.Errorf("a player's ListContent names %s", k)
			}
		}
		if !mentions(js, "class:fighter") || !mentions(js, "subclass:champion") || !mentions(js, "spell:fire-bolt") || !mentions(js, spell.GetKey()) {
			t.Error("a player's ListContent lost an option that is on")
		}
	}
	// The master reads everything, with the flag.
	mc := master.catalog(t, campaign)
	var wizard *rulesv1.CharacterClass
	for _, c := range mc.GetContent().GetClasses() {
		if c.GetKey() == "class:wizard" {
			wizard = c
		}
	}
	if wizard == nil || !wizard.GetOff() {
		t.Errorf("the master's wizard = %v, want it there and off", wizard)
	}
	if r := findRace(mc.GetContent(), race.GetKey()); r == nil || !r.GetOff() {
		t.Errorf("the master's table race = %v, want it there and off", r)
	}
	for _, s := range mc.GetContent().GetSubclasses() {
		if s.GetKey() == "subclass:evocation" && s.GetOff() {
			t.Error("the evocation subclass has its own switch on: only the class is off")
		}
	}

	// ListTableEntries: the player never gets the off race, nor the subrace under it
	// (its sheet does not use either); the master gets both, the race flagged.
	for _, who := range []*user{owner, other} {
		js := jsonOf(t, &rulesv1.ListTableEntriesResponse{Entries: mapValues(who.entries(t, campaign))})
		if mentions(js, race.GetKey()) || mentions(js, subrace.GetKey()) {
			t.Error("a player's ListTableEntries names the off race or its subrace")
		}
		if !mentions(js, spell.GetKey()) {
			t.Error("a player's ListTableEntries lost the spell that is on")
		}
	}
	me := master.entries(t, campaign)
	if !me[race.GetKey()].GetOff() || me[subrace.GetKey()].GetOff() || me[spell.GetKey()].GetOff() {
		t.Errorf("the master's flags: race off = %v, subrace off = %v, spell off = %v", me[race.GetKey()].GetOff(), me[subrace.GetKey()].GetOff(), me[spell.GetKey()].GetOff())
	}
	// A reference inside another entry to an off class is left out for the player: the
	// spell names the wizard, which is off.
	if sp := other.entries(t, campaign)[spell.GetKey()]; len(sp.GetTableSpell().GetClassKeys()) != 0 {
		t.Errorf("the player's spell still names classes: %v", sp.GetTableSpell().GetClassKeys())
	}
	if sp := me[spell.GetKey()]; !slices.Equal(sp.GetTableSpell().GetClassKeys(), []string{"class:wizard"}) {
		t.Errorf("the master's spell classes = %v", sp.GetTableSpell().GetClassKeys())
	}

	// ListSpells and GetSpellDetails.
	ps := other.spellPage(t, campaign, "")
	for _, s := range ps.GetSpells() {
		if s.GetKey() == "spell:fireball" || s.GetOff() || slices.Contains(s.GetClassKeys(), "class:wizard") {
			t.Errorf("a player's spell list has %s (off = %v, classes %v)", s.GetKey(), s.GetOff(), s.GetClassKeys())
		}
	}
	if got := other.spellPage(t, campaign, "Bola de Fogo"); slices.Contains(spellKeysOf(got), "spell:fireball") || len(got.GetSpells()) != 1 {
		t.Errorf("a player searching for the off spell found %v, want only the delayed blast", spellKeysOf(got))
	}
	ms := master.spellPage(t, campaign, "Bola de Fogo")
	if i := slices.Index(spellKeysOf(ms), "spell:fireball"); i < 0 || !ms.GetSpells()[i].GetOff() || len(ms.GetSpells()) != 2 {
		t.Errorf("the master's search = %v, want fireball flagged off", spellKeysOf(ms))
	}
	if ps.GetTotal() != master.spellPage(t, campaign, "").GetTotal()-1 {
		t.Errorf("a player's total = %d, the master's %d: want exactly the fireball less", ps.GetTotal(), master.spellPage(t, campaign, "").GetTotal())
	}
	details := func(u *user) error {
		_, err := u.content.GetSpellDetails(t.Context(), connect.NewRequest(&rulesv1.GetSpellDetailsRequest{CampaignId: campaign, SpellKey: "spell:fireball"}))
		return err
	}
	if err := details(other); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("a player reading the off spell: error = %v, want not_found", err)
	}
	if err := details(master); err != nil {
		t.Errorf("the master reading the off spell: error = %v", err)
	}

	// The master's list for the screen: the flags, the parents and the counts.
	opts := master.options(t, campaign)
	if o := opts["class:wizard"]; !o.GetOff() || !o.GetHidden() || o.GetCharactersUsing() != 1 || o.GetTable() {
		t.Errorf("the wizard's entry = %v, want off, hidden, one player using it, the SRD's", o)
	}
	if o := opts["subclass:evocation"]; o.GetOff() || !o.GetHidden() || o.GetParentKey() != "class:wizard" {
		t.Errorf("the evocation entry = %v, want its own switch on, hidden, parent the wizard", o)
	}
	if o := opts[subrace.GetKey()]; o.GetOff() || !o.GetHidden() || o.GetParentKey() != race.GetKey() || !o.GetTable() {
		t.Errorf("the subrace entry = %v", o)
	}
	if o := opts["class:fighter"]; o.GetOff() || o.GetHidden() {
		t.Errorf("the fighter's entry = %v, want on", o)
	}
	if o := opts["spell:fireball"]; o.GetLevel() != 3 || !o.GetOff() {
		t.Errorf("fireball's entry = %v", o)
	}
	// The sheet of the one who uses the wizard keeps working and reads as it did.
	got := owner.list(t, campaign)
	if len(got) != 1 {
		t.Fatalf("the owner has %d characters", len(got))
	}
	c := owner.get(t, campaign, got[0].GetId())
	if len(c.GetDerived().GetIssues()) != 0 {
		t.Errorf("the sheet of an off class has issues: %v", c.GetDerived().GetIssues())
	}
	if _, err := owner.update(t, c, "Pensantus, o Sábio", c.GetSheet()); err != nil {
		t.Errorf("renaming a sheet that uses an off class: error = %v", err)
	}

	// Turning it back on.
	master.setOff(t, campaign, false, "class:wizard", "spell:fireball", race.GetKey())
	js := jsonOf(t, other.catalog(t, campaign))
	for _, k := range hiddenKeys {
		if !mentions(js, k) {
			t.Errorf("after switching back on, a player's ListContent lacks %s", k)
		}
	}
	if o := master.options(t, campaign)["class:wizard"]; o.GetOff() || o.GetHidden() {
		t.Errorf("the wizard's entry after switching on = %v", o)
	}
}

func mapValues(m map[string]*rulesv1.TableEntry) []*rulesv1.TableEntry {
	var out []*rulesv1.TableEntry
	for _, e := range m {
		out = append(out, e)
	}
	return out
}

// TestRN23_ASheetThatUsesAnOffOptionKeepsReadingAboutIt: like an archived entry, an
// off option stays readable to the player whose own sheet has it.
func TestRN23_ASheetThatUsesAnOffOptionKeepsReadingAboutIt(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, owner, other := h.newUser("Samuel"), h.newUser("Dona"), h.newUser("Outra")
	campaign := h.newCampaign(master, "Mirathel", owner, other)
	spell := master.addEntry(t, campaign, testSpell("Raio de teste", "class:wizard"))
	owner.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus", pensantusWith(spell.GetKey()))
	master.setOff(t, campaign, true, spell.GetKey())
	read := func(u *user) error {
		_, err := u.content.GetSpellDetails(t.Context(), connect.NewRequest(&rulesv1.GetSpellDetailsRequest{CampaignId: campaign, SpellKey: spell.GetKey()}))
		return err
	}
	if err := read(owner); err != nil {
		t.Errorf("the owner of a sheet with the off spell: error = %v, want it read", err)
	}
	if err := read(other); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("another player: error = %v, want not_found", err)
	}
	if mentions(jsonOf(t, owner.spellPage(t, campaign, "")), spell.GetKey()) {
		t.Error("the page of spells lists an off spell even to the one who uses it: it is a list of choices")
	}
}

// TestRN23_ANewChoiceOfAnOffOptionIsRefused: creating, editing and leveling up
// refuse a new choice of an off option, with a typed reason; a sheet that has it
// keeps working and an unrelated edit succeeds; the master's own characters may use it.
func TestRN23_ANewChoiceOfAnOffOptionIsRefused(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.svc.SetLevelUps(xpLevelUps{})
	master, owner, other := h.newUser("Samuel"), h.newUser("Dona"), h.newUser("Outra")
	campaign := h.newCampaign(master, "Mirathel", owner, other)
	bg := master.addEntry(t, campaign, testBackground("Guarda de farol"))
	sub := master.addEntry(t, campaign, testSubclass("Caminho do Vento", "class:fighter"))
	withBackground := func(key string) *charactersv1.CharacterSheet {
		s := pensantusSheet()
		s.GetFull().Background = &charactersv1.FullSheet_BackgroundKey{BackgroundKey: key}
		s.GetFull().SkillProficiencyKeys = []string{"skill:investigation", "skill:medicine"}
		return s
	}
	// Outra's sheet uses the background before the switch.
	pc := other.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Terceiro", withBackground(bg.GetKey()))
	// Toren, a Fighter 2 with 900 XP, for the level-up.
	toren := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores:        &rulesv1.AbilityScores{Strength: 15, Dexterity: 12, Constitution: 14, Intelligence: 8, Wisdom: 10, Charisma: 10},
		RaceKey:           "race:human",
		Background:        &charactersv1.FullSheet_BackgroundKey{BackgroundKey: "background:acolyte"},
		Classes:           []*charactersv1.ClassLevel{{ClassKey: "class:fighter", Level: 2}},
		ArmorKey:          "equipment:chain-mail",
		WeaponKeys:        []string{"equipment:longsword"},
		FeatureChoiceKeys: []string{"feature:fighter-fighting-style-defense"},
		ExperiencePoints:  900,
	}}}
	player := h.newUser("Quarta")
	h.join(master, campaign, player)
	pcT := player.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Toren", toren)

	master.setOff(t, campaign, true, bg.GetKey(), sub.GetKey())

	// Create.
	_, err := owner.api.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Novo", Sheet: withBackground(bg.GetKey()),
	}))
	if b := charBlocked(t, err); b.GetReason() != charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_SWITCHED_OFF_CONTENT || b.GetContentKey() != bg.GetKey() {
		t.Errorf("CreateCharacter with an off background: %v, want SWITCHED_OFF_CONTENT with the key", b)
	}
	// Update: a new choice is refused, the sheet that has it keeps working, and an
	// unrelated edit succeeds.
	mine := owner.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus", pensantusSheet())
	_, err = owner.api.UpdateCharacter(t.Context(), connect.NewRequest(&charactersv1.UpdateCharacterRequest{
		CampaignId: campaign, CharacterId: mine.GetId(), Revision: mine.GetRevision(), Name: mine.GetName(), Sheet: withBackground(bg.GetKey()),
	}))
	if b := charBlocked(t, err); b.GetReason() != charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_SWITCHED_OFF_CONTENT || b.GetCharacterId() != mine.GetId() {
		t.Errorf("UpdateCharacter picking an off background: %v", b)
	}
	got := other.get(t, campaign, pc.GetId())
	if _, err := other.api.UpdateCharacter(t.Context(), connect.NewRequest(&charactersv1.UpdateCharacterRequest{
		CampaignId: campaign, CharacterId: pc.GetId(), Revision: got.GetRevision(), Name: "Terceiro, o Sábio", Sheet: got.GetSheet(),
	})); err != nil {
		t.Errorf("a sheet that already has the off background: error = %v, want it to keep working", err)
	}
	if got.GetDerived().GetBackgroundNamePt() != "Guarda de farol" {
		t.Errorf("the off background's name on the sheet = %q", got.GetDerived().GetBackgroundNamePt())
	}
	if _, err := owner.update(t, mine, "Pensantus, o Renomeado", mine.GetSheet()); err != nil {
		t.Errorf("an unrelated edit: error = %v", err)
	}
	// The master may use it for his own characters (an NPC).
	if _, err := master.api.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_ENEMY, Name: "Capitão", Sheet: withBackground(bg.GetKey()),
	})); err != nil {
		t.Errorf("the master's NPC with an off background: error = %v, want it made", err)
	}

	h.lockSheets(campaign) // the first game session started: a level-up is how a locked sheet grows
	pcT = player.get(t, campaign, pcT.GetId())

	// Level-up: the options leave the off subclass out for the player and mark it for
	// the master; choosing it is refused.
	subclasses := func(u *user) map[string]bool {
		res, err := u.api.GetLevelUpOptions(t.Context(), connect.NewRequest(&charactersv1.GetLevelUpOptionsRequest{CampaignId: campaign, CharacterId: pcT.GetId()}))
		if err != nil {
			t.Fatalf("GetLevelUpOptions() error = %v", err)
		}
		out := map[string]bool{}
		for _, s := range res.Msg.GetOptions().GetSubclasses() {
			out[s.GetKey()] = s.GetOff()
		}
		return out
	}
	if k := subclasses(player); hasKey(k, sub.GetKey()) || !hasKey(k, "subclass:champion") {
		t.Errorf("the player's subclasses = %v, want the champion and not the off one", k)
	}
	if k := subclasses(master); !k[sub.GetKey()] || k["subclass:champion"] {
		t.Errorf("the master's subclasses = %v, want the off one marked", k)
	}
	levelUp := func(u *user, subclass string) error {
		_, err := u.api.LevelUpCharacter(t.Context(), connect.NewRequest(&charactersv1.LevelUpCharacterRequest{
			CampaignId: campaign, CharacterId: pcT.GetId(), Revision: pcT.GetRevision(),
			Choices: &charactersv1.LevelUpChoices{
				ClassKey: "class:fighter", SubclassKey: subclass,
				HitPoints: &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE},
			},
		}))
		return err
	}
	refused := func(err error) *charactersv1.LevelUpRefusal {
		ce, ok := errors.AsType[*connect.Error](err)
		if !ok || ce.Code() != connect.CodeFailedPrecondition {
			t.Fatalf("level-up with an off subclass: error = %v, want failed_precondition", err)
		}
		for _, d := range ce.Details() {
			if v, derr := d.Value(); derr == nil {
				if r, ok := v.(*charactersv1.LevelUpRefusal); ok {
					return r
				}
			}
		}
		return nil
	}
	if r := refused(levelUp(player, sub.GetKey())); r.GetReason() != charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_SWITCHED_OFF_CHOICE || r.GetField() != "full.classes[0].subclass_key" {
		t.Errorf("refusal = %v, want SWITCHED_OFF_CHOICE at the subclass", r)
	}
	// The master's own level-up is not refused for it (the preview, which would write nothing).
	pv, err := master.api.PreviewLevelUp(t.Context(), connect.NewRequest(&charactersv1.PreviewLevelUpRequest{
		CampaignId: campaign, CharacterId: pcT.GetId(),
		Choices: &charactersv1.LevelUpChoices{
			ClassKey: "class:fighter", SubclassKey: sub.GetKey(),
			HitPoints: &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE},
		},
	}))
	if err != nil || pv.Msg.GetRefusal() != nil || pv.Msg.GetAfter() == nil {
		t.Errorf("the master's preview with an off subclass: error = %v, refusal = %v", err, pv.Msg.GetRefusal())
	}
	// The player's preview of the refused choice carries no sheet: a guessed key reads nothing.
	pv, err = player.api.PreviewLevelUp(t.Context(), connect.NewRequest(&charactersv1.PreviewLevelUpRequest{
		CampaignId: campaign, CharacterId: pcT.GetId(),
		Choices: &charactersv1.LevelUpChoices{
			ClassKey: "class:fighter", SubclassKey: sub.GetKey(),
			HitPoints: &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE},
		},
	}))
	if err != nil || pv.Msg.GetRefusal().GetReason() != charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_SWITCHED_OFF_CHOICE || pv.Msg.GetAfter() != nil {
		t.Errorf("the player's preview of an off subclass: error = %v, refusal = %v, after = %v; want the refusal and no sheet", err, pv.Msg.GetRefusal(), pv.Msg.GetAfter())
	}
	// A sheet that already has an off class keeps working (question 80): the subclass
	// that is on is offered and accepted, whatever the class's switch says.
	master.setOff(t, campaign, false, sub.GetKey())
	master.setOff(t, campaign, true, "class:fighter")
	if k := subclasses(player); !hasKey(k, sub.GetKey()) || !hasKey(k, "subclass:champion") || k[sub.GetKey()] || k["subclass:champion"] {
		t.Errorf("the player's subclasses under their own off class = %v, want both, on", k)
	}
	if err := levelUp(player, sub.GetKey()); err != nil {
		t.Errorf("level-up of a sheet with an off class, picking its subclass: error = %v", err)
	}
	master.setOff(t, campaign, false, "class:fighter")
	// And a player may create a sheet with the background again.
	master.setOff(t, campaign, false, bg.GetKey())
	sixth := h.newUser("Sexta")
	h.join(master, campaign, sixth)
	if _, err := sixth.api.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Novo", Sheet: withBackground(bg.GetKey()),
	})); err != nil {
		t.Errorf("CreateCharacter with the background back on: error = %v", err)
	}
}

// TestRN23_TheRevisionAndTheCacheFollowTheSwitches: every change moves the revision
// and the content a player reads, and a call that changes nothing moves nothing.
func TestRN23_TheRevisionAndTheCacheFollowTheSwitches(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, player := h.newUser("Samuel"), h.newUser("Dona")
	campaign := h.newCampaign(master, "Mirathel", player)
	if got := h.contentRevision(campaign); got != 0 {
		t.Fatalf("revision before = %d", got)
	}
	seen := func() (int32, string, bool) {
		c := player.catalog(t, campaign)
		return c.GetTableRevision(), c.GetContent().GetContentVersion(), mentions(jsonOf(t, c), "race:dwarf")
	}
	if rev, v, ok := seen(); rev != 0 || !ok || strings.Contains(v, "+mesa") {
		t.Fatalf("before: revision %d, version %q, dwarf %v", rev, v, ok)
	}
	res := master.setOff(t, campaign, true, "race:dwarf")
	if res.GetTableRevision() != 1 || h.contentRevision(campaign) != 1 {
		t.Errorf("revision after the switch = %d / %d, want 1", res.GetTableRevision(), h.contentRevision(campaign))
	}
	if rev, v, ok := seen(); rev != 1 || ok || !strings.HasSuffix(v, "+mesa.1") {
		t.Errorf("after off: revision %d, version %q, dwarf %v", rev, v, ok)
	}
	// Asking for what already is changes nothing: no bump.
	again := master.setOff(t, campaign, true, "race:dwarf")
	if again.GetChanged() != 0 || again.GetTableRevision() != 1 || h.contentRevision(campaign) != 1 || len(again.GetOptions()) != 0 {
		t.Errorf("a repeated switch: %v, revision %d", again, h.contentRevision(campaign))
	}
	// Two options at once are one write, one revision.
	res = master.setOff(t, campaign, true, "race:elf", "race:dwarf", "spell:fireball")
	if res.GetChanged() != 2 || res.GetTableRevision() != 2 {
		t.Errorf("a mixed call: changed %d, revision %d, want 2 and 2", res.GetChanged(), res.GetTableRevision())
	}
	res = master.setOff(t, campaign, false, "race:dwarf")
	if rev, _, ok := seen(); rev != 3 || !ok || res.GetTableRevision() != 3 {
		t.Errorf("after on: revision %d, dwarf %v", rev, ok)
	}
	// The content is cached by revision: the master's own entry writes still work next
	// to a set of switches, and the set survives them.
	race := master.addEntry(t, campaign, testRace("Anão do Mar"))
	if !mentions(jsonOf(t, player.catalog(t, campaign)), race.GetKey()) || mentions(jsonOf(t, player.catalog(t, campaign)), "race:elf") {
		t.Error("a table write dropped the switches or the entry")
	}
	if !master.options(t, campaign)["race:elf"].GetOff() {
		t.Error("the elf is not off in the master's list")
	}
	// The response names what changed for the players: a class takes its subclasses along.
	res = master.setOff(t, campaign, true, "class:cleric")
	keys := map[string]bool{}
	for _, o := range res.GetOptions() {
		keys[o.GetKey()] = o.GetHidden()
	}
	if !keys["class:cleric"] || !keys["subclass:life"] {
		t.Errorf("the changed options = %v, want the cleric and its life domain, hidden", keys)
	}
	if len(res.GetOptions()) != 2 {
		t.Errorf("%d options changed, want the cleric and its one SRD subclass", len(res.GetOptions()))
	}
}

// TestRN10_ContentChangedTellsTheSession: a write to the table's content, an archive
// and a switch send content_changed after they commit, and a refused or empty call
// sends nothing. The hint is the stream's own message: nothing in it.
func TestRN10_ContentChangedTellsTheSession(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	spy := &liveSpy{}
	h.svc.SetLive(spy)
	master, player := h.newUser("Samuel"), h.newUser("Dona")
	campaign := h.newCampaign(master, "Mirathel", player)
	hints := func() int { spy.mu.Lock(); defer spy.mu.Unlock(); return len(spy.content) }
	if hints() != 0 {
		t.Fatal("a hint before any write")
	}
	e := master.addEntry(t, campaign, testBackground("Guarda de farol"))
	if hints() != 1 {
		t.Errorf("hints after CreateTableEntry = %d, want 1", hints())
	}
	if _, err := master.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(campaign, e.GetKey(), e.GetRevision(), testBackground("Guarda do farol")))); err != nil {
		t.Fatal(err)
	}
	if hints() != 2 {
		t.Errorf("hints after UpdateTableEntry = %d, want 2", hints())
	}
	if _, err := master.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: campaign, Key: e.GetKey()})); err != nil {
		t.Fatal(err)
	}
	if _, err := master.table.UnarchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.UnarchiveTableEntryRequest{CampaignId: campaign, Key: e.GetKey()})); err != nil {
		t.Fatal(err)
	}
	if hints() != 4 {
		t.Errorf("hints after archive and unarchive = %d, want 4", hints())
	}
	master.setOff(t, campaign, true, "class:wizard")
	if hints() != 5 {
		t.Errorf("hints after a switch = %d, want 5", hints())
	}
	master.setOff(t, campaign, true, "class:wizard") // already off
	if hints() != 5 {
		t.Errorf("a switch that changed nothing sent a hint (%d)", hints())
	}
	// Refused writes send nothing.
	bad := testBackground("Outro")
	bad.Skills = []string{"skill:nonexistent"}
	_, _ = master.table.CreateTableEntry(t.Context(), connect.NewRequest(createReq(campaign, bad)))
	_, _ = master.table.SetOptionSwitches(t.Context(), connect.NewRequest(switchReq(campaign, true, "class:ghost")))
	_, _ = player.table.SetOptionSwitches(t.Context(), connect.NewRequest(switchReq(campaign, true, "class:fighter")))
	if hints() != 5 {
		t.Errorf("a refused write sent a hint (%d)", hints())
	}
	for _, c := range spy.content {
		if c != campaign {
			t.Errorf("a hint for campaign %q, want %q", c, campaign)
		}
	}
}

// TestMR025_TheSwitchesAreTheMastersAlone: only the master asks for or writes the
// switches; a pending member and a stranger get not_found, a player permission_denied;
// a bad request is invalid_argument and changes nothing.
func TestMR025_TheSwitchesAreTheMastersAlone(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, player, pending, stranger := h.newUser("Samuel"), h.newUser("Dona"), h.newUser("Pendente"), h.newUser("Estranho")
	campaign := h.newCampaign(master, "Mirathel", player)
	h.joinPending(master, campaign, pending)
	for name, u := range map[string]*user{"player": player, "pending": pending, "stranger": stranger, "anonymous": h.anonymous()} {
		_, errSet := u.table.SetOptionSwitches(t.Context(), connect.NewRequest(switchReq(campaign, true, "class:wizard")))
		_, errList := u.table.ListOptionSwitches(t.Context(), connect.NewRequest(&rulesv1.ListOptionSwitchesRequest{CampaignId: campaign}))
		want := connect.CodeNotFound
		switch name {
		case "player":
			want = connect.CodePermissionDenied
		case "anonymous":
			want = connect.CodeUnauthenticated
		}
		if connect.CodeOf(errSet) != want || connect.CodeOf(errList) != want {
			t.Errorf("%s: set = %v, list = %v, want %v", name, errSet, errList, want)
		}
	}
	if h.contentRevision(campaign) != 0 {
		t.Error("a refused call moved the revision")
	}
	// Bad requests.
	many := make([]string, MaxOptionSwitches+1)
	for i := range many {
		many[i] = fmt.Sprintf("spell:s%d", i)
	}
	for name, req := range map[string]*rulesv1.SetOptionSwitchesRequest{
		"empty":      switchReq(campaign, true),
		"too many":   switchReq(campaign, true, many...),
		"unknown":    switchReq(campaign, true, "class:wizard", "class:ghost"),
		"not a kind": switchReq(campaign, true, "feature:fighter-fighting-style-defense"),
		"twice":      switchReq(campaign, true, "class:wizard", "class:wizard"),
		"not mine":   switchReq(campaign, true, "class:x@mesa"),
	} {
		if _, err := master.table.SetOptionSwitches(t.Context(), connect.NewRequest(req)); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("%s: error = %v, want invalid_argument", name, err)
		}
	}
	if h.contentRevision(campaign) != 0 || master.options(t, campaign)["class:wizard"].GetOff() {
		t.Error("a refused call changed something")
	}
	// The table's own key of another campaign is no key here.
	other := h.newCampaign(master, "Outra mesa")
	e := master.addEntry(t, other, testRace("Anão do Mar"))
	if _, err := master.table.SetOptionSwitches(t.Context(), connect.NewRequest(switchReq(campaign, true, e.GetKey()))); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("another campaign's entry: error = %v, want invalid_argument", err)
	}
}

// TestRN23_ConcurrentSwitchesAreOrdered: switches of one campaign run one after the
// other, each its own revision, and none is lost.
func TestRN23_ConcurrentSwitchesAreOrdered(t *testing.T) {
	t.Parallel()
	const n = 8
	dbtest.PoolSize(t, n)
	h := newHarness(t)
	master := h.newUser("Samuel")
	campaign := h.newCampaign(master, "Mirathel")
	keys := []string{"spell:fireball", "spell:fire-bolt", "spell:light", "spell:shield", "class:wizard", "race:dwarf", "background:acolyte", "subclass:evocation"}
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := range n {
		wg.Go(func() {
			_, errs[i] = master.table.SetOptionSwitches(context.Background(), connect.NewRequest(switchReq(campaign, true, keys[i])))
		})
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("switch %d error = %v", i, err)
		}
	}
	if got := h.contentRevision(campaign); got != n {
		t.Errorf("revision = %d, want %d, one per switch", got, n)
	}
	opts := master.options(t, campaign)
	for _, k := range keys {
		if !opts[k].GetOff() {
			t.Errorf("%s is not off: a switch was lost", k)
		}
	}
	// Racing the same key both ways ends in a state one of them asked for, never both.
	var wg2 sync.WaitGroup
	for i := range n {
		wg2.Go(func() {
			_, _ = master.table.SetOptionSwitches(context.Background(), connect.NewRequest(switchReq(campaign, i%2 == 0, "race:elf")))
		})
	}
	wg2.Wait()
	rev := h.contentRevision(campaign)
	if rev < n+1 || rev > 2*n {
		t.Errorf("revision after the race = %d, want between %d and %d", rev, n+1, 2*n)
	}
}

// TestRN23_TheOwnSheetKeepsAnOffSRDOptionReadable: the exemption for the caller's own
// sheet covers the SRD's keys too: the owner of a sheet with an SRD spell (or class)
// still reads it after the master switches it off; another player does not.
func TestRN23_TheOwnSheetKeepsAnOffSRDOptionReadable(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, owner, other := h.newUser("Samuel"), h.newUser("Dona"), h.newUser("Outra")
	campaign := h.newCampaign(master, "Mirathel", owner, other)
	sheet := pensantusSheet()
	spell := sheet.GetFull().GetKnownSpellKeys()[0]
	owner.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus", sheet)
	master.setOff(t, campaign, true, spell, "class:wizard")
	read := func(u *user) error {
		_, err := u.content.GetSpellDetails(t.Context(), connect.NewRequest(&rulesv1.GetSpellDetailsRequest{CampaignId: campaign, SpellKey: spell}))
		return err
	}
	if err := read(owner); err != nil {
		t.Errorf("the owner of a sheet with the off SRD spell %s: error = %v, want it read", spell, err)
	}
	if err := read(other); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("another player: error = %v, want not_found", err)
	}
	// A hidden class asked for in the spells page answers as an unknown one to a
	// player whose sheet lacks it, so a guessed key is never confirmed.
	filter := func(u *user, class string) *rulesv1.ListSpellsResponse {
		res, err := u.content.ListSpells(t.Context(), connect.NewRequest(&rulesv1.ListSpellsRequest{CampaignId: campaign, ClassKey: class}))
		if err != nil {
			t.Fatalf("ListSpells(class %q) error = %v", class, err)
		}
		return res.Msg
	}
	unknown := filter(other, "class:no-such-class")
	hiddenAsked := filter(other, "class:wizard")
	if hiddenAsked.GetTotal() != 0 || len(hiddenAsked.GetSpells()) != 0 || hiddenAsked.GetNextPageToken() != unknown.GetNextPageToken() || hiddenAsked.GetContentVersion() != unknown.GetContentVersion() {
		t.Errorf("a hidden class from a player = %v, want exactly what an unknown class answers (%v)", hiddenAsked, unknown)
	}
	if got := filter(owner, "class:wizard"); got.GetTotal() == 0 {
		t.Error("the owner of a wizard cannot list the wizard's spells")
	}
	if got := filter(master, "class:wizard"); got.GetTotal() == 0 {
		t.Error("the master cannot list an off class's spells")
	}
}

// TestRN23_AWriteNeverFillsTheLiveCache: a switch builds what it answers without the
// live source, so a transaction that rolls back leaves no content nobody committed
// for its revision.
func TestRN23_AWriteNeverFillsTheLiveCache(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	src := h.svc.content.(*TableSource)
	master, player := h.newUser("Samuel"), h.newUser("Dona")
	campaign := h.newCampaign(master, "Mirathel", player)
	master.setOff(t, campaign, true, "race:dwarf") // revision 1
	cached := func(rev int32) bool { return src.cached(liveKey{campaign, rev}) != nil }
	// A call that fails after it began: the context is canceled, nothing is written.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := master.table.SetOptionSwitches(ctx, connect.NewRequest(switchReq(campaign, true, "race:elf"))); err == nil {
		t.Fatal("a canceled switch succeeded")
	}
	if h.contentRevision(campaign) != 1 || cached(2) {
		t.Errorf("after the aborted switch: revision %d, revision 2 cached = %v", h.contentRevision(campaign), cached(2))
	}
	// A committed switch fills nothing either: the next read does.
	master.setOff(t, campaign, true, "race:elf") // revision 2
	if cached(2) {
		t.Error("a write filled the cache for its own revision")
	}
	js := jsonOf(t, player.catalog(t, campaign))
	if mentions(js, "race:elf") || mentions(js, "race:dwarf") || !mentions(js, "race:human") {
		t.Error("the content served after the switches is wrong")
	}
	if !cached(2) {
		t.Error("a read did not fill the cache")
	}
}

// TestRN23_ASubraceUnderTheSheetsOwnOffRaceKeepsWorking: the twin of the level-up
// case: a sheet that has an off race may change to another subrace of it, and a new
// sheet cannot pick the race or its subrace.
func TestRN23_ASubraceUnderTheSheetsOwnOffRaceKeepsWorking(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, owner, other := h.newUser("Samuel"), h.newUser("Dona"), h.newUser("Outra")
	campaign := h.newCampaign(master, "Mirathel", owner, other)
	race := master.addEntry(t, campaign, testRace("Anão do Mar"))
	sub1 := master.addEntry(t, campaign, testSubrace("Do cais", race.GetKey()))
	sub2 := master.addEntry(t, campaign, testSubrace("Do farol", race.GetKey()))
	with := func(subrace string) *charactersv1.CharacterSheet {
		s := pensantusSheet()
		s.GetFull().RaceKey, s.GetFull().SubraceKey = race.GetKey(), subrace
		return s
	}
	pc := owner.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus", with(sub1.GetKey()))
	master.setOff(t, campaign, true, race.GetKey())
	if _, err := owner.update(t, pc, "Pensantus", with(sub2.GetKey())); err != nil {
		t.Errorf("a sheet with an off race changing to another subrace of it: error = %v", err)
	}
	_, err := other.api.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Novo", Sheet: with(sub1.GetKey()),
	}))
	if b := charBlocked(t, err); b.GetReason() != charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_SWITCHED_OFF_CONTENT || b.GetContentKey() != race.GetKey() {
		t.Errorf("a new sheet with the off race: %v, want SWITCHED_OFF_CONTENT at the race", b)
	}
	// A subrace on its own under an off race is refused when the sheet lacks the race (the
	// race is refused first, so the key named is the race's, never a leak of the child).
	// ListTableEntries: the child is hidden from a player whose sheet lacks it and shown
	// to the one whose sheet uses it.
	if e := other.entries(t, campaign); e[sub1.GetKey()] != nil || e[sub2.GetKey()] != nil || e[race.GetKey()] != nil {
		t.Error("a player without the race reads it or its subraces")
	}
	got := owner.entries(t, campaign)
	if got[sub2.GetKey()] == nil || got[sub1.GetKey()] != nil || got[race.GetKey()] != nil {
		t.Errorf("the owner reads %v, want only the subrace their sheet uses", keysOf(got))
	}
}

func keysOf(m map[string]*rulesv1.TableEntry) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// TestRN23_TheListedChoicesOfAnEntryNeverNameAnOffSpell: a "choose a cantrip from
// these" effect lists SRD spells, which are switchable: a player never reads an off one.
func TestRN23_TheListedChoicesOfAnEntryNeverNameAnOffSpell(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, player := h.newUser("Samuel"), h.newUser("Dona")
	campaign := h.newCampaign(master, "Mirathel", player)
	race := testRace("Anão do Mar")
	race.Traits = append(race.Traits, &rulesv1.TableFeature{NamePt: "Dom do mar", Effects: []*rulesv1.TableEffect{{
		Type: "choice", Choice: "cantrip", Count: 1, From: []string{"spell:fire-bolt", "spell:light"},
	}}})
	entry := master.addEntry(t, campaign, race)
	master.setOff(t, campaign, true, "spell:fire-bolt")
	choices := func(u *user) []string {
		var out []string
		for _, tr := range u.entries(t, campaign)[entry.GetKey()].GetTableRace().GetTraits() {
			for _, ef := range tr.GetEffects() {
				out = append(out, ef.GetFrom()...)
			}
		}
		return out
	}
	if got := choices(player); !slices.Equal(got, []string{"spell:light"}) {
		t.Errorf("the player reads the choices %v, want only the spell that is on", got)
	}
	if got := choices(master); !slices.Equal(got, []string{"spell:fire-bolt", "spell:light"}) {
		t.Errorf("the master reads the choices %v, want both", got)
	}
}

// TestRN23_APreviewOfARetiredChoiceCarriesNoSheet: a player who guesses an archived
// key reads its features nowhere, not in the preview's `after` either.
func TestRN23_APreviewOfARetiredChoiceCarriesNoSheet(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.svc.SetLevelUps(xpLevelUps{})
	master, player := h.newUser("Samuel"), h.newUser("Dona")
	campaign := h.newCampaign(master, "Mirathel", player)
	sub := master.addEntry(t, campaign, testSubclass("Caminho do Vento", "class:fighter"))
	toren := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores:        &rulesv1.AbilityScores{Strength: 15, Dexterity: 12, Constitution: 14, Intelligence: 8, Wisdom: 10, Charisma: 10},
		RaceKey:           "race:human",
		Background:        &charactersv1.FullSheet_BackgroundKey{BackgroundKey: "background:acolyte"},
		Classes:           []*charactersv1.ClassLevel{{ClassKey: "class:fighter", Level: 2}},
		ArmorKey:          "equipment:chain-mail",
		WeaponKeys:        []string{"equipment:longsword"},
		FeatureChoiceKeys: []string{"feature:fighter-fighting-style-defense"},
		ExperiencePoints:  900,
	}}}
	pc := player.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Toren", toren)
	h.lockSheets(campaign)
	if _, err := master.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: campaign, Key: sub.GetKey()})); err != nil {
		t.Fatal(err)
	}
	pv, err := player.api.PreviewLevelUp(t.Context(), connect.NewRequest(&charactersv1.PreviewLevelUpRequest{
		CampaignId: campaign, CharacterId: pc.GetId(),
		Choices: &charactersv1.LevelUpChoices{
			ClassKey: "class:fighter", SubclassKey: sub.GetKey(),
			HitPoints: &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE},
		},
	}))
	if err != nil {
		t.Fatalf("PreviewLevelUp() error = %v", err)
	}
	if pv.Msg.GetRefusal().GetReason() != charactersv1.LevelUpRefusalReason_LEVEL_UP_REFUSAL_REASON_ARCHIVED_CHOICE || pv.Msg.GetAfter() != nil {
		t.Errorf("preview of an archived subclass: error = %v, refusal = %v, after = %v", err, pv.Msg.GetRefusal(), pv.Msg.GetAfter())
	}
}

// TestRN23_SwitchingAnArchivedEntryWorks: an archived table entry can be switched off
// and on like any other; it is hidden either way, and archiving keeps its switch.
func TestRN23_SwitchingAnArchivedEntryWorks(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, player := h.newUser("Samuel"), h.newUser("Dona")
	campaign := h.newCampaign(master, "Mirathel", player)
	e := master.addEntry(t, campaign, testBackground("Guarda de farol"))
	if _, err := master.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: campaign, Key: e.GetKey()})); err != nil {
		t.Fatal(err)
	}
	res := master.setOff(t, campaign, true, e.GetKey())
	if res.GetChanged() != 1 || len(res.GetOptions()) != 1 || !res.GetOptions()[0].GetHidden() || !res.GetOptions()[0].GetOff() {
		t.Errorf("switching an archived entry: changed %d, options %v; want its own switch changed and still hidden", res.GetChanged(), res.GetOptions())
	}
	o := master.options(t, campaign)[e.GetKey()]
	if !o.GetOff() || !o.GetArchived() || !o.GetHidden() {
		t.Errorf("the entry = %v, want off, archived and hidden", o)
	}
	if _, err := master.table.UnarchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.UnarchiveTableEntryRequest{CampaignId: campaign, Key: e.GetKey()})); err != nil {
		t.Fatal(err)
	}
	if o := master.options(t, campaign)[e.GetKey()]; !o.GetOff() || o.GetArchived() || !o.GetHidden() {
		t.Errorf("after unarchiving: %v, want still off and hidden", o)
	}
	if !master.entries(t, campaign)[e.GetKey()].GetOff() {
		t.Error("ListTableEntries does not say the entry is off")
	}
	res = master.setOff(t, campaign, false, e.GetKey())
	if len(res.GetOptions()) != 1 || res.GetOptions()[0].GetHidden() {
		t.Errorf("switching it back on = %v", res.GetOptions())
	}
}

// TestRN23_ASwitchIsOrderedAgainstASheetWrite: a switch that commits between a
// handler's first read of the content and its transaction is seen by the
// transaction, so the new choice is refused (the twin of
// TestTableContentWritesAreOrderedAgainstContentWrites).
func TestRN23_ASwitchIsOrderedAgainstASheetWrite(t *testing.T) {
	t.Parallel()
	var src *afterReadSource
	h := newHarnessWith(t, func(c *Config) {
		src = &afterReadSource{ContentSource: c.Content}
		c.Content = src
	})
	master, owner := h.newUser("Samuel"), h.newUser("Dona")
	campaign := h.newCampaign(master, "Mirathel", owner)
	bg := master.addEntry(t, campaign, testBackground("Guarda de farol"))
	with := func(key string) *charactersv1.CharacterSheet {
		s := pensantusSheet()
		s.GetFull().Background = &charactersv1.FullSheet_BackgroundKey{BackgroundKey: key}
		s.GetFull().SkillProficiencyKeys = []string{"skill:investigation", "skill:medicine"}
		return s
	}
	src.hook = func() {
		if _, err := master.table.SetOptionSwitches(context.Background(), connect.NewRequest(switchReq(campaign, true, bg.GetKey()))); err != nil {
			t.Errorf("the switch in between: %v", err)
		}
	}
	_, err := owner.api.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Pensantus", Sheet: with(bg.GetKey()),
	}))
	if b := charBlocked(t, err); b.GetReason() != charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_SWITCHED_OFF_CONTENT || b.GetContentKey() != bg.GetKey() {
		t.Errorf("CreateCharacter with a switch in between: %v, want SWITCHED_OFF_CONTENT", b)
	}
	// And an update that takes a background switched off in between.
	pc := owner.createPensantus(t, campaign)
	bg2 := master.addEntry(t, campaign, testBackground("Outro fundo"))
	src.hook = func() {
		if _, err := master.table.SetOptionSwitches(context.Background(), connect.NewRequest(switchReq(campaign, true, bg2.GetKey()))); err != nil {
			t.Errorf("the switch in between: %v", err)
		}
	}
	_, err = owner.api.UpdateCharacter(t.Context(), connect.NewRequest(&charactersv1.UpdateCharacterRequest{
		CampaignId: campaign, CharacterId: pc.GetId(), Revision: pc.GetRevision(), Name: pc.GetName(), Sheet: with(bg2.GetKey()),
	}))
	if b := charBlocked(t, err); b.GetReason() != charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_SWITCHED_OFF_CONTENT || b.GetContentKey() != bg2.GetKey() {
		t.Errorf("UpdateCharacter with a switch in between: %v, want SWITCHED_OFF_CONTENT", b)
	}
}

// A player's preview of a level-up that picks a switched-off choice carries no sheet, whatever
// else the choices get wrong (here an in-app hit point roll never made): what is off never
// appears to players (RN-23).
func TestPreviewOfASwitchedOffChoiceCarriesNoSheetWithAnotherRefusal(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.svc.SetLevelUps(xpLevelUps{})
	master, player := h.newUser("Samuel"), h.newUser("Dona")
	campaign := h.newCampaign(master, "Mirathel", player)
	sub := master.addEntry(t, campaign, testSubclass("Caminho do Vento", "class:fighter"))
	toren := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores:        &rulesv1.AbilityScores{Strength: 15, Dexterity: 12, Constitution: 14, Intelligence: 8, Wisdom: 10, Charisma: 10},
		RaceKey:           "race:human",
		Background:        &charactersv1.FullSheet_BackgroundKey{BackgroundKey: "background:acolyte"},
		Classes:           []*charactersv1.ClassLevel{{ClassKey: "class:fighter", Level: 2}},
		ArmorKey:          "equipment:chain-mail",
		WeaponKeys:        []string{"equipment:longsword"},
		FeatureChoiceKeys: []string{"feature:fighter-fighting-style-defense"},
		ExperiencePoints:  900,
	}}}
	pc := player.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Toren", toren)
	h.lockSheets(campaign)
	master.setOff(t, campaign, true, sub.GetKey())

	preview := func(method charactersv1.LevelUpHitPointsMethod) *charactersv1.PreviewLevelUpResponse {
		pv, err := player.api.PreviewLevelUp(t.Context(), connect.NewRequest(&charactersv1.PreviewLevelUpRequest{
			CampaignId: campaign, CharacterId: pc.GetId(),
			Choices: &charactersv1.LevelUpChoices{
				ClassKey: "class:fighter", SubclassKey: sub.GetKey(),
				HitPoints: &charactersv1.LevelUpHitPoints{Method: method},
			},
		}))
		if err != nil {
			t.Fatalf("PreviewLevelUp() error = %v", err)
		}
		return pv.Msg
	}

	// Control: no earlier refusal, correctly suppressed.
	if r := preview(charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE); r.GetAfter() != nil {
		t.Errorf("control (average): after = %v, want none", r.GetAfter())
	}
	// Earlier refusal: in-app roll never made.
	r := preview(charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_ROLLED_IN_APP)
	t.Logf("refusal = %v", r.GetRefusal())
	if r.GetAfter() != nil {
		js := jsonOf(t, r.GetAfter())
		t.Errorf("player's preview of an off subclass with refusal %v carries `after` (mentions subclass key: %v, name: %v): %.600s",
			r.GetRefusal().GetReason(), strings.Contains(js, sub.GetKey()), strings.Contains(js, "Caminho do Vento"), js)
	}
}
