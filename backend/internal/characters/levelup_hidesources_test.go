package characters

import (
	"slices"
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// A player's level-up options carry no skill source whose owner the table retired or switched off, unless the sheet holds the
// owner: a switched-off class and an archived class a character would multiclass into, the class feature of an archived
// class the sheet does not hold, and an archived class itself even when the sheet holds it. The feature of an archived class the
// sheet holds stays, as the sheet has it.
func TestTheSkillSourcesOfAnOwnerHiddenFromAPlayerAreDropped(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, player := h.newUser("Samuel"), h.newUser("Dona")
	campaign := h.newCampaign(master, "Mirathel", player)
	classWith := func(name string) (class, feature string) {
		c := testClass(name)
		c.Levels[0].Features = []*rulesv1.TableFeature{{NamePt: "Estudo " + name, DescPt: []string{"Texto de teste."}, Effects: []*rulesv1.TableEffect{
			{Type: "choice", Choice: "skill", Count: 1, From: []string{"skill:stealth", "skill:perception"}},
		}}}
		e := master.addEntry(t, campaign, c)
		return e.GetKey(), master.entries(t, campaign)[e.GetKey()].GetTableClass().GetLevels()[0].GetFeatures()[0].GetKey()
	}
	off, offFeature := classWith("Escola apagada")
	archived, archivedFeature := classWith("Escola antiga")
	held, heldFeature := classWith("Escola da casa")
	visible, visibleFeature := classWith("Escola aberta")
	master.setOff(t, campaign, true, off)
	for _, key := range []string{archived, held} {
		if _, err := master.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: campaign, Key: key})); err != nil {
			t.Fatalf("ArchiveTableEntry(%s) error = %v", key, err)
		}
	}
	content, err := h.svc.contentFor(t.Context(), nil, campaign)
	if err != nil {
		t.Fatal(err)
	}
	sheet := pensantusSheet().GetFull()
	sheet.Classes = []*charactersv1.ClassLevel{{ClassKey: held, Level: 1}}
	target := levelUpTarget{content: content, build: buildOf(sheet), classKey: held, full: sheet}
	owners := []string{off, offFeature, archived, archivedFeature, held, heldFeature, visible, visibleFeature}
	options := func() *charactersv1.LevelUpOptions {
		out := &charactersv1.LevelUpOptions{}
		for _, k := range owners {
			out.SkillSources = append(out.SkillSources, &charactersv1.LevelUpSkillSource{OwnerKey: k, Count: 1})
		}
		return out
	}
	got := options()
	hideFromPlayer(got, target)
	var kept []string
	for _, src := range got.GetSkillSources() {
		kept = append(kept, src.GetOwnerKey())
	}
	want := []string{heldFeature, visible, visibleFeature}
	if !slices.Equal(kept, want) {
		t.Errorf("the skill sources a player reads are owned by %v, want %v", kept, want)
	}
}
