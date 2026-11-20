package characters

import (
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

const enemyKind = charactersv1.CharacterKind_CHARACTER_KIND_ENEMY

// hasFitIssue says the master's derivation of the character's sheet carries the fit issue.
func hasFitIssue(t *testing.T, u *unfitTable, c *charactersv1.Character) bool {
	t.Helper()
	content, err := u.h.svc.contentFor(t.Context(), nil, u.campaign)
	if err != nil {
		t.Fatal(err)
	}
	for _, is := range rules.Derive(buildOf(c.GetSheet().GetFull()), content).Issues {
		if is.Code == rules.IssuePickFit {
			return true
		}
	}
	return false
}

// The master creates and saves an NPC with a skill no list allows and an expertise no source takes, and reads the issue; the
// same sheets sent by a player are refused.
func TestTheMasterSavesANpcWithPicksNoListAllows(t *testing.T) {
	t.Parallel()
	u := newUnfitTable(t, "skill:survival")
	npc := u.master.create(t, u.campaign, enemyKind, "Capanga", u.sheet("skill:investigation", "skill:insight", "skill:nature"))
	if !hasFitIssue(t, u, npc) {
		t.Error("the master reads no fit issue of the skill")
	}
	_, err := u.player.api.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: u.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Pensantus", Sheet: u.sheet("skill:investigation", "skill:insight", "skill:nature"),
	}))
	wantUnfit(t, err, "sheet.full.skill_proficiency_keys[2]", "Natureza")

	// The save of an NPC and the preview take the same picks.
	fits := u.master.create(t, u.campaign, enemyKind, "Guarda", u.sheet("skill:investigation", "skill:insight", "skill:survival"))
	if _, err := u.master.update(t, fits, "Guarda", u.sheet("skill:investigation", "skill:insight", "skill:nature")); err != nil {
		t.Fatalf("the master's save of a pick no list allows: %v", err)
	}
	if _, err := u.master.preview(t, u.campaign, fits.GetId(), enemyKind, u.sheet("skill:investigation", "skill:insight", "skill:medicine")); err != nil {
		t.Fatalf("the master's preview of a pick no list allows: %v", err)
	}

	// An expertise with no source for it.
	u.race = u.master.updateEntry(t, u.campaign, u.race, expertiseTraitRace("skill:athletics"))
	withExpertise := u.sheet("skill:investigation", "skill:insight", "skill:nature")
	withExpertise.GetFull().ExpertiseSkillKeys = []string{"skill:investigation"}
	exp := u.master.create(t, u.campaign, enemyKind, "Perito", withExpertise)
	if len(exp.GetSheet().GetFull().GetExpertiseSkillKeys()) != 1 || !hasFitIssue(t, u, exp) {
		t.Error("the expertise is not kept with the issue")
	}
	pc := u.player.create(t, u.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus", u.sheet("skill:investigation", "skill:insight", "skill:nature"))
	_, err = u.player.update(t, pc, "Pensantus", withExpertise)
	wantUnfitSentence(t, err, "sheet.full.expertise_skill_keys[0]", "A especialização em Investigação não está em nenhuma das listas")
}

// The master edits a player's character with a pick no list allows, and the player's own reads never carry the issue.
func TestTheMasterEditsAPlayersCharacterWithAPickNoListAllows(t *testing.T) {
	t.Parallel()
	u := newUnfitTable(t, "skill:survival")
	pc := u.player.create(t, u.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus", u.sheet("skill:investigation", "skill:insight", "skill:survival"))
	if _, err := u.master.update(t, pc, "Pensantus", u.sheet("skill:investigation", "skill:insight", "skill:nature")); err != nil {
		t.Fatalf("the master's edit: %v", err)
	}
	got := u.player.get(t, u.campaign, pc.GetId())
	for _, is := range got.GetDerived().GetIssues() {
		if is.GetCode() == rules.IssuePickFit {
			t.Errorf("the player reads the fit issue: %v", is)
		}
	}
}

// Completing the choices of a character, the master is not refused a tool no source takes with the held picks; an owner that
// is no tool stays an unknown key.
func TestTheMasterCompletesAToolNoSourceTakes(t *testing.T) {
	t.Parallel()
	u := newUnfitTable(t, "skill:nature")
	bg := u.master.addEntry(t, u.campaign, toolRace(toolEffect(1, smithsTools)))
	pc := u.player.create(t, u.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Ferreira", u.toolSheet(t, bg, thievesTools, smithsTools))
	if h := u.h.lockSheets(u.campaign); h != 1 {
		t.Fatalf("locked %d sheets", h)
	}
	u.master.updateEntry(t, u.campaign, bg, toolRace(toolEffect(2, smithsTools, glassTools)))
	owner := u.traitOf(t, bg)
	tb := &choiceTable{h: u.h, campaign: u.campaign}
	if _, err := tb.complete(u.master, u.master.get(t, u.campaign, pc.GetId()), pick(rules.ToolPickKey(owner), herbalismKit)); err != nil {
		t.Fatalf("the master's completion: %v", err)
	}
}
