package characters

import (
	"errors"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
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
