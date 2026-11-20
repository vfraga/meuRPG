package characters

import (
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
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
	if _, err := u.master.preview(t, u.campaign, fits.GetId(), enemyKind, u.sheet("skill:investigation", "skill:insight", "skill:stealth")); err != nil {
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

// The master's unknown-key answers stay main's whatever other picks the sheet has: an unlisted skill beside a tool pick that is
// no tool is refused as an unknown key as the tool pick alone is. The level up
// reaches the check without the sheet validation before it, so the check is called as the level up calls it.
func TestTheMastersUnknownKeyAnswersHoldWhateverOtherPicksTheSheetHas(t *testing.T) {
	t.Parallel()
	u := newUnfitTable(t, "skill:nature")
	bg := u.master.addEntry(t, u.campaign, toolRace(toolEffect(1, smithsTools)))
	owner := u.traitOf(t, bg)
	content, err := u.h.svc.contentFor(t.Context(), nil, u.campaign)
	if err != nil {
		t.Fatal(err)
	}
	view := content.ForMasterPicks()
	stored := u.toolSheet(t, bg, thievesTools).GetFull()
	for name, pick := range map[string]string{
		"no tool": rules.ScopedChoice(rules.ToolPickKey(owner), "skill:nature"),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			held := func() *charactersv1.FullSheet { return stored }
			withPick := func(skills ...string) *charactersv1.FullSheet {
				full := proto.Clone(stored).(*charactersv1.FullSheet)
				full.FeatureChoiceKeys = append(full.FeatureChoiceKeys, pick)
				if skills != nil {
					full.SkillProficiencyKeys = skills
				}
				return full
			}
			alone := refuseUnfitPickIn(view, true, held, withPick())
			with := refuseUnfitPickIn(view, true, held, withPick("skill:investigation", "skill:insight", "skill:stealth"))
			if alone == nil || with == nil {
				t.Fatalf("errors alone = %v, with an unlisted skill = %v, want both refused", alone, with)
			}
			if alone.Error() != with.Error() {
				t.Errorf("with an unlisted skill = %v, want the answer of the tool pick alone: %v", with, alone)
			}
		})
	}
}

// The master's preview of a player's level up with a skill no list places: accepted, the issue on the sheet after the level for
// the master and never on the player's read; the same pick in the player's own preview is refused.
func TestTheMasterLevelsUpAPlayersCharacterWithAPickNoListPlaces(t *testing.T) {
	t.Parallel()
	u := newUnfitTable(t, "skill:nature")
	u.h.svc.SetLevelUps(xpLevelUps{})
	sub := u.master.addEntry(t, u.campaign, &rulesv1.TableSubclass{
		NamePt: "Mestre do Saber", ClassKey: "class:fighter", DescPt: []string{"Um caminho de teste."},
		Levels: []*rulesv1.TableSubclassLevel{{Level: 3, Features: []*rulesv1.TableFeature{{
			NamePt: "Estudo", DescPt: []string{"Uma perícia à escolha."},
			Effects: []*rulesv1.TableEffect{{Type: "choice", Choice: "skill", Count: 1, From: []string{"skill:arcana", "skill:history"}}},
		}}}},
	})
	fighter := levelUpFighterSheet(2, 900, "")
	fighter.GetFull().SkillProficiencyKeys = []string{"skill:athletics", "skill:perception"}
	pc := u.player.create(t, u.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Ícaro", fighter)
	u.h.lockSheets(u.campaign)
	tb := &levelUpTable{h: u.h, master: u.master, owner: u.player, campaign: u.campaign}
	choices := &charactersv1.LevelUpChoices{
		ClassKey: "class:fighter", SubclassKey: sub.GetKey(),
		HitPoints: &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE},
	}
	choices.SkillProficiencyKeys = []string{"skill:medicine"}
	issued := func(res *charactersv1.PreviewLevelUpResponse) bool {
		for _, is := range res.GetAfter().GetIssues() {
			if is.GetCode() == rules.IssuePickFit {
				return true
			}
		}
		return false
	}
	asMaster, err := tb.preview(u.master, pc, choices)
	if err != nil || asMaster.GetRefusal() != nil {
		t.Fatalf("the master's level up: %v, refusal %v", err, asMaster.GetRefusal())
	}
	if !issued(asMaster) {
		t.Errorf("the master reads no fit issue after the level: %v", asMaster.GetAfter().GetIssues())
	}
	asPlayer, err := tb.preview(u.player, pc, choices)
	if err == nil && asPlayer.GetRefusal() == nil {
		t.Error("the player's level up with a pick no list places is accepted")
	}
	if err == nil && issued(asPlayer) {
		t.Error("the player reads the fit issue after the level")
	}
	for _, is := range u.player.get(t, u.campaign, pc.GetId()).GetDerived().GetIssues() {
		if is.GetCode() == rules.IssuePickFit {
			t.Errorf("the player reads the fit issue: %v", is)
		}
	}
}
