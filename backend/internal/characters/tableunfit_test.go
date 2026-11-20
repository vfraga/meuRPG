package characters

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// A table race whose trait lets the player choose one skill from a list: the sheets below pick it, and the master later
// changes the list.

type unfitTable struct {
	h              *harness
	master, player *user
	campaign       string
	race           *rulesv1.TableEntry
}

func skillTraitRace(from ...string) *rulesv1.TableRace {
	return &rulesv1.TableRace{
		NamePt: "Perito", Size: "Medium", SpeedFt: 30, AbilityBonuses: &rulesv1.AbilityScores{Constitution: 1},
		Traits: []*rulesv1.TableFeature{{NamePt: "Faro", Effects: []*rulesv1.TableEffect{{Type: "choice", Choice: "skill", Count: 1, From: from}}}},
	}
}

func newUnfitTable(t *testing.T, from ...string) *unfitTable {
	t.Helper()
	h := newHarness(t)
	master, player := h.newUser("Samuel"), h.newUser("Dona")
	campaign := h.newCampaign(master, "Mirathel", player)
	return &unfitTable{h: h, master: master, player: player, campaign: campaign, race: master.addEntry(t, campaign, skillTraitRace(from...))}
}

// sheet is Pensantus as the table's race, with these skills.
func (u *unfitTable) sheet(skills ...string) *charactersv1.CharacterSheet {
	s := pensantusSheet()
	f := s.GetFull()
	f.RaceKey, f.SubraceKey = u.race.GetKey(), ""
	f.SkillProficiencyKeys = skills
	return s
}

// narrow changes the trait's list.
func (u *unfitTable) narrow(t *testing.T, from ...string) {
	t.Helper()
	u.race = u.master.updateEntry(t, u.campaign, u.race, skillTraitRace(from...))
}

func (u *user) updateEntry(t *testing.T, campaign string, e *rulesv1.TableEntry, body proto.Message) *rulesv1.TableEntry {
	t.Helper()
	res, err := u.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(campaign, e.GetKey(), e.GetRevision(), body)))
	if err != nil {
		t.Fatalf("UpdateTableEntry() error = %v", err)
	}
	return res.Msg.GetEntry()
}

// wantUnfit fails unless err refuses the field with the sentence of a pick no list allows.
func wantUnfit(t *testing.T, err error, field, name string) {
	t.Helper()
	ce, ok := errors.AsType[*connect.Error](err)
	if !ok || ce.Code() != connect.CodeInvalidArgument {
		t.Fatalf("error = %v, want invalid_argument", err)
	}
	if !strings.Contains(ce.Message(), field) || !strings.Contains(ce.Message(), name) || !strings.Contains(ce.Message(), "não está em nenhuma das") {
		t.Errorf("message = %q, want field %s and the name %s", ce.Message(), field, name)
	}
}

func TestACharacterWithAPickNoListAllowsIsNotCreated(t *testing.T) {
	t.Parallel()
	u := newUnfitTable(t, "skill:survival")
	_, err := u.player.api.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: u.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Pensantus", Sheet: u.sheet("skill:investigation", "skill:insight", "skill:nature"),
	}))
	wantUnfit(t, err, "sheet.full.skill_proficiency_keys[2]", "Natureza")
	u.player.create(t, u.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus", u.sheet("skill:investigation", "skill:insight", "skill:survival"))
}

func TestAnUpdateWithANewPickNoListAllowsIsRefused(t *testing.T) {
	t.Parallel()
	u := newUnfitTable(t, "skill:survival")
	pc := u.player.create(t, u.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus", u.sheet("skill:investigation", "skill:insight", "skill:survival"))
	_, err := u.player.update(t, pc, "Pensantus", u.sheet("skill:investigation", "skill:insight", "skill:nature"))
	wantUnfit(t, err, "sheet.full.skill_proficiency_keys[2]", "Natureza")
	// The same refusal answers the preview.
	_, err = u.player.preview(t, u.campaign, pc.GetId(), charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, u.sheet("skill:investigation", "skill:insight", "skill:nature"))
	wantUnfit(t, err, "sheet.full.skill_proficiency_keys[2]", "Natureza")
}

func TestAStoredSheetThatNoLongerFitsStillSaves(t *testing.T) {
	t.Parallel()
	u := newUnfitTable(t, "skill:nature", "skill:survival")
	pc := u.player.create(t, u.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus", u.sheet("skill:investigation", "skill:insight", "skill:nature"))
	u.narrow(t, "skill:survival") // the held pick is in no list now
	got := u.player.get(t, u.campaign, pc.GetId())
	saved, err := u.player.update(t, got, "Pensantus, o Sábio", got.GetSheet())
	if err != nil {
		t.Fatalf("saving the untouched sheet under another name: %v", err)
	}
	if saved.GetName() != "Pensantus, o Sábio" {
		t.Errorf("name = %q", saved.GetName())
	}
}

func TestAPickThatFitsByMovingAHeldPickIsAccepted(t *testing.T) {
	t.Parallel()
	u := newUnfitTable(t, "skill:investigation")
	// The wizard's list has investigation too: the trait takes it so that the class takes medicine.
	pc := u.player.create(t, u.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus", u.sheet("skill:investigation", "skill:insight"))
	if _, err := u.player.update(t, pc, "Pensantus", u.sheet("skill:investigation", "skill:insight", "skill:medicine")); err != nil {
		t.Fatalf("a pick that fits when a held one moves is refused: %v", err)
	}
}

const (
	thievesTools = "proficiency:thieves-tools"
	herbalismKit = "proficiency:herbalism-kit"
	smithsTools  = "proficiency:smiths-tools"
	glassTools   = "proficiency:glassblowers-tools"
)

func toolEffect(count int32, from ...string) *rulesv1.TableEffect {
	return &rulesv1.TableEffect{Type: "choice", Choice: "tool", Count: count, From: from}
}

// toolRace has a trait with two tool choices, each with its own list.
func toolRace(second *rulesv1.TableEffect) *rulesv1.TableRace {
	return &rulesv1.TableRace{
		NamePt: "Marujo", Size: "Medium", SpeedFt: 30, AbilityBonuses: &rulesv1.AbilityScores{Constitution: 1},
		Traits: []*rulesv1.TableFeature{{NamePt: "Rede do cais", DescPt: []string{"Texto de teste."}, Effects: []*rulesv1.TableEffect{toolEffect(1, thievesTools, herbalismKit), second}}},
	}
}

func (u *unfitTable) traitOf(t *testing.T, race *rulesv1.TableEntry) string {
	t.Helper()
	return u.master.entries(t, u.campaign)[race.GetKey()].GetTableRace().GetTraits()[0].GetKey()
}

func (u *unfitTable) toolSheet(t *testing.T, race *rulesv1.TableEntry, picks ...string) *charactersv1.CharacterSheet {
	t.Helper()
	s := pensantusSheet()
	f := s.GetFull()
	f.RaceKey, f.SubraceKey = race.GetKey(), ""
	owner := u.traitOf(t, race)
	f.FeatureChoiceKeys = nil
	for _, p := range picks {
		f.FeatureChoiceKeys = append(f.FeatureChoiceKeys, rules.ScopedChoice(rules.ToolPickKey(owner), p))
	}
	return s
}

func TestAnExpertiseNoListAllowsIsRefusedWithItsOwnSentence(t *testing.T) {
	t.Parallel()
	u := newUnfitTable(t, "skill:nature")
	pc := u.player.create(t, u.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus", u.sheet("skill:investigation", "skill:insight", "skill:nature"))
	u.race = u.master.updateEntry(t, u.campaign, u.race, expertiseTraitRace("skill:athletics"))
	withExpertise := u.sheet("skill:investigation", "skill:insight", "skill:nature")
	withExpertise.GetFull().ExpertiseSkillKeys = []string{"skill:investigation"}
	_, err := u.player.update(t, pc, "Pensantus", withExpertise)
	wantUnfitSentence(t, err, "sheet.full.expertise_skill_keys[0]", "A especialização em Investigação não está em nenhuma das listas")
}

func TestAToolNoChoiceOffersIsRefusedWithItsOwnSentence(t *testing.T) {
	t.Parallel()
	u := newUnfitTable(t, "skill:nature")
	bg := u.master.addEntry(t, u.campaign, toolRace(toolEffect(1, smithsTools)))
	tool := u.player.create(t, u.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Ferreira", u.toolSheet(t, bg, thievesTools, smithsTools))
	_, err := u.player.update(t, tool, "Ferreira", u.toolSheet(t, bg, thievesTools, glassTools))
	wantUnfitSentence(t, err, "sheet.full.feature_choice_keys[1]", "não está em nenhuma das escolhas de ferramenta do personagem")
}

func TestAToolOnAListWithNoRoomLeftIsRefusedWithTheSentenceOfNoRoom(t *testing.T) {
	t.Parallel()
	u := newUnfitTable(t, "skill:nature")
	bg := u.master.addEntry(t, u.campaign, toolRace(toolEffect(1, smithsTools)))
	tool := u.player.create(t, u.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Ferreira", u.toolSheet(t, bg, thievesTools, smithsTools))
	// The first list takes one of its two tools and the second takes smiths: herbalism is on a list that has no room.
	_, err := u.player.update(t, tool, "Ferreira", u.toolSheet(t, bg, thievesTools, smithsTools, herbalismKit))
	wantUnfitSentence(t, err, "sheet.full.feature_choice_keys[2]", "Não há vaga para a ferramenta")
	wantUnfitSentence(t, err, "sheet.full.feature_choice_keys[2]", "as listas que a oferecem já estão preenchidas pelas outras ferramentas marcadas. Desmarque uma delas para marcar")
}

func expertiseTraitRace(from ...string) *rulesv1.TableRace {
	r := skillTraitRace("skill:nature")
	r.Traits = append(r.Traits, &rulesv1.TableFeature{NamePt: "Estudo", Effects: []*rulesv1.TableEffect{{Type: "choice", Choice: "expertise", Count: 1, From: from}}})
	return r
}

func wantUnfitSentence(t *testing.T, err error, field, sentence string) {
	t.Helper()
	ce, ok := errors.AsType[*connect.Error](err)
	if !ok || ce.Code() != connect.CodeInvalidArgument || !strings.Contains(ce.Message(), field) || !strings.Contains(ce.Message(), sentence) {
		t.Errorf("error = %v, want invalid_argument at %s with %q", err, field, sentence)
	}
}

// A pick made when completing the choices is refused as it is on a save: the choice offers the tool, but no source takes it
// together with the picks the sheet holds (the first list lists it and its one slot is taken).
func TestCompletingChoicesRefusesAToolNoSourceTakesWithTheHeldPicks(t *testing.T) {
	t.Parallel()
	u := newUnfitTable(t, "skill:nature")
	bg := u.master.addEntry(t, u.campaign, toolRace(toolEffect(1, smithsTools)))
	pc := u.player.create(t, u.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Ferreira", u.toolSheet(t, bg, thievesTools, smithsTools))
	if h := u.h.lockSheets(u.campaign); h != 1 {
		t.Fatalf("locked %d sheets", h)
	}
	// The master adds a pick to the second choice: the sheet has one open.
	u.master.updateEntry(t, u.campaign, bg, toolRace(toolEffect(2, smithsTools, glassTools)))
	owner := u.traitOf(t, bg)
	_, err := (&choiceTable{h: u.h, campaign: u.campaign}).complete(u.player, u.player.get(t, u.campaign, pc.GetId()), pick(rules.ToolPickKey(owner), herbalismKit))
	var ce *connect.Error
	if !errors.As(err, &ce) || ce.Code() != connect.CodeInvalidArgument || !strings.Contains(ce.Message(), "Não há vaga para a ferramenta") {
		t.Errorf("error = %v, want the refusal of a tool on a list with no room", err)
	}
}

// A sheet a package brings holds its picks: it imports though no list of its table takes one of them, where the same sheet
// made now by a player is refused, and the master reads the issue about it.
func TestAnImportedSheetThatDoesNotFitImportsAndTheMasterReadsTheIssue(t *testing.T) {
	t.Parallel()
	u := newUnfitTable(t, "skill:survival")
	content, err := u.h.svc.contentFor(t.Context(), nil, u.campaign)
	if err != nil {
		t.Fatal(err)
	}
	master := authz.Membership{CampaignID: u.campaign, Role: authz.RoleMaster}
	player := authz.Membership{CampaignID: u.campaign, Role: authz.RolePlayer}
	sheet := u.sheet("skill:investigation", "skill:insight", "skill:nature")
	if _, err := checkNewSheet(content, player, kindPlayer, proto.CloneOf(sheet)); err == nil {
		t.Fatal("a sheet made now by a player with a pick no list allows is accepted")
	}
	checked, err := checkImportedSheet(content, master, kindPlayer, proto.CloneOf(sheet))
	if err != nil {
		t.Fatalf("the imported sheet is refused: %v", err)
	}
	var found bool
	for _, is := range rules.Derive(buildOf(checked.GetFull()), content).Issues {
		found = found || is.Code == rules.IssuePickFit
	}
	if !found {
		t.Error("the master's derivation of the imported sheet has no fit issue")
	}
}

func TestASkillOnAListWithNoRoomLeftIsRefusedWithTheSentenceOfNoRoom(t *testing.T) {
	t.Parallel()
	// The wizard takes two of its list, the trait takes one and lists only survival: a third wizard skill is on a list, and has no room.
	u := newUnfitTable(t, "skill:survival")
	_, err := u.player.api.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: u.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Pensantus", Sheet: u.sheet("skill:investigation", "skill:insight", "skill:medicine"),
	}))
	wantUnfitSentence(t, err, "sheet.full.skill_proficiency_keys[0]", "Não há vaga para a perícia Investigação: as listas que a oferecem já estão preenchidas pelas outras perícias marcadas. Desmarque uma delas para marcar Investigação.")
}

func TestAnExpertiseOnAListWithNoRoomLeftIsRefusedWithItsOwnSentenceOfNoRoom(t *testing.T) {
	t.Parallel()
	u := newUnfitTable(t, "skill:nature")
	race := skillTraitRace("skill:nature")
	for _, from := range [][]string{{"skill:investigation", "skill:insight"}, {"skill:nature"}} {
		race.Traits = append(race.Traits, &rulesv1.TableFeature{NamePt: "Estudo", Effects: []*rulesv1.TableEffect{{Type: "choice", Choice: "expertise", Count: 1, From: from}}})
	}
	u.race = u.master.updateEntry(t, u.campaign, u.race, race)
	s := u.sheet("skill:investigation", "skill:insight", "skill:nature")
	s.GetFull().ExpertiseSkillKeys = []string{"skill:investigation", "skill:insight"}
	_, err := u.player.api.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: u.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Pensantus", Sheet: s,
	}))
	wantUnfitSentence(t, err, "sheet.full.expertise_skill_keys[0]", "Não há vaga para a especialização em Investigação: as listas que a oferecem já estão preenchidas pelas outras especializações marcadas. Desmarque uma delas para marcar a especialização em Investigação.")
}

// The master is held to no list: a skill no list allows and an expertise no source takes are saved on an NPC, and on a
// player's character the master edits, and the master reads the fit issue on them. The same picks of a player are refused.
func TestTheMasterSavesPicksNoListAllowsAndReadsTheIssue(t *testing.T) {
	t.Parallel()
	u := newUnfitTable(t, "skill:survival")
	// nature is on no list of the sheet, and the sheet has no source of expertise.
	bad := func() *charactersv1.CharacterSheet {
		s := u.sheet("skill:investigation", "skill:insight", "skill:nature")
		s.GetFull().ExpertiseSkillKeys = []string{"skill:investigation"}
		return s
	}
	hasFitIssue := func(c *charactersv1.Character) bool {
		return slices.ContainsFunc(c.GetDerived().GetIssues(), func(is *rulesv1.Issue) bool { return is.GetCode() == rules.IssuePickFit })
	}
	enemy := charactersv1.CharacterKind_CHARACTER_KIND_ENEMY
	npc := u.master.create(t, u.campaign, enemy, "Sábio sem lista", bad())
	if got := u.master.get(t, u.campaign, npc.GetId()); !hasFitIssue(got) {
		t.Error("the master's read of the NPC has no fit issue")
	}
	// A save that changes the pick is accepted too.
	changed := u.sheet("skill:investigation", "skill:insight", "skill:history")
	changed.GetFull().ExpertiseSkillKeys = []string{"skill:insight"}
	if _, err := u.master.update(t, npc, "Sábio sem lista", changed); err != nil {
		t.Errorf("the master's save of an NPC with a pick no list allows: %v", err)
	}
	// The same sheet of a player is refused, on create and on save.
	_, err := u.player.api.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: u.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Pensantus", Sheet: bad(),
	}))
	wantUnfit(t, err, "sheet.full.skill_proficiency_keys[2]", "Natureza")
	pc := u.player.create(t, u.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus", u.sheet("skill:investigation", "skill:insight", "skill:survival"))
	_, err = u.player.update(t, pc, "Pensantus", bad())
	wantUnfit(t, err, "sheet.full.skill_proficiency_keys[2]", "Natureza")
	// The master edits the player's character with the same picks: saved, previewed, and read with the issue; the player reads none.
	if _, err := u.master.preview(t, u.campaign, pc.GetId(), charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, bad()); err != nil {
		t.Errorf("the master's preview of a player's character with a pick no list allows: %v", err)
	}
	saved, err := u.master.update(t, pc, "Pensantus", bad())
	if err != nil {
		t.Fatalf("the master's save of a player's character with a pick no list allows: %v", err)
	}
	if !hasFitIssue(u.master.get(t, u.campaign, saved.GetId())) {
		t.Error("the master's read of the player's character has no fit issue")
	}
	if hasFitIssue(u.player.get(t, u.campaign, saved.GetId())) {
		t.Error("the player's read of the character carries the fit issue")
	}
}

// The master completes the choices of a sheet with a tool no source takes with the held picks, which a player is refused
// (TestCompletingChoicesRefusesAToolNoSourceTakesWithTheHeldPicks).
func TestTheMasterCompletesAChoiceWithAToolNoSourceTakes(t *testing.T) {
	t.Parallel()
	u := newUnfitTable(t, "skill:nature")
	bg := u.master.addEntry(t, u.campaign, toolRace(toolEffect(1, smithsTools)))
	pc := u.player.create(t, u.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Ferreira", u.toolSheet(t, bg, thievesTools, smithsTools))
	if h := u.h.lockSheets(u.campaign); h != 1 {
		t.Fatalf("locked %d sheets", h)
	}
	u.master.updateEntry(t, u.campaign, bg, toolRace(toolEffect(2, smithsTools, glassTools)))
	owner := u.traitOf(t, bg)
	if _, err := (&choiceTable{h: u.h, campaign: u.campaign}).complete(u.master, u.master.get(t, u.campaign, pc.GetId()), pick(rules.ToolPickKey(owner), herbalismKit)); err != nil {
		t.Errorf("the master's completion of a choice with a tool no source takes: %v", err)
	}
}

// refuseUnfitPick is the one place that holds a player to the lists: called directly, it refuses a player's pick no list
// allows and never the master's. The paths that call it are covered by the tests above, and the level up by the leaktest row.
func TestOnlyAPlayerIsRefusedAPickNoListAllows(t *testing.T) {
	t.Parallel()
	u := newUnfitTable(t, "skill:survival")
	content, err := u.h.svc.contentFor(t.Context(), nil, u.campaign)
	if err != nil {
		t.Fatal(err)
	}
	bad := u.sheet("skill:investigation", "skill:insight", "skill:nature").GetFull()
	for who, m := range map[string]authz.Membership{
		"the master": {CampaignID: u.campaign, Role: authz.RoleMaster},
		"a player":   {CampaignID: u.campaign, Role: authz.RolePlayer},
	} {
		err := refuseUnfitPick(content, m, noSheet, bad)
		if (who == "a player") != (err != nil) {
			t.Errorf("%s: refuseUnfitPick = %v", who, err)
		}
	}
}
