package leaktest

import (
	"slices"
	"testing"

	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The skills the editor offers come from the sources of the sheet, as the reader's own content sees them: a table class that
// the table switches off adds no key to the list a player reads for a draft of the sheet they own, and the master, who reads
// what is switched off, still sees its keys. (A sheet that already holds the class keeps it, and so its keys.)
func TestASwitchedOffTableClassAddsNoSkillKeyToAPlayersListForADraft(t *testing.T) {
	p := newChoicesProbe(t)
	w := p.w
	const extra = "skill:performance"
	class := p.createEntry(t, tableClass("Menestrel da Mesa", &rulesv1.TableFeature{
		NamePt: "Canto", DescPt: []string{"Texto de teste."},
		Effects: []*rulesv1.TableEffect{{Type: "choice", Choice: "skill", Count: 1, From: []string{extra}}},
	}))
	// Ana's stored sheet is an ordinary one; the draft she previews takes the table class.
	held := must(w.ana.characters.CreateCharacter(t.Context(), rq(&charactersv1.CreateCharacterRequest{
		CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Menestrel", Sheet: sheet("class:wizard", 1),
	}))).GetCharacter()
	// listed reads the lists of the draft as who sees them: whether the answer came, and whether it lists the key.
	listed := func(t *testing.T, who reader) (listed, answered bool) {
		t.Helper()
		r := who.person.call(charactersv1connect.CharacterServicePreviewChoicesProcedure, &charactersv1.PreviewChoicesRequest{
			CampaignId: w.campaign, CharacterId: held.GetId(), Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Sheet: sheet(class.GetKey(), 1),
		})
		if !r.ok() {
			return false, false
		}
		return slices.Contains(choicesOf(t, r).GetSkillsAllowed().GetKeys(), extra), true
	}
	master, ana := reader{"the master", w.master, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, true}, reader{"Ana, the owner", w.ana, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, true}
	for _, who := range []reader{master, ana} { // the positive control: before the switch each reads the key
		if l, ok := listed(t, who); !ok || !l {
			t.Fatalf("%s: before the switch the list holds %s = %v (answered %v), the positive control", who.name, extra, l, ok)
		}
	}
	must(w.master.table.SetOptionSwitches(t.Context(), rq(&rulesv1.SetOptionSwitchesRequest{
		CampaignId: w.campaign, Switches: []*rulesv1.OptionSwitch{{Key: class.GetKey(), Off: true}},
	})))
	if l, ok := listed(t, master); !ok || !l {
		t.Errorf("the master reads what the table switched off, answered %v with %s listed %v", ok, extra, l)
	}
	if l, _ := listed(t, ana); l {
		t.Errorf("Ana's list holds %s of a class the table switched off", extra)
	}
}
