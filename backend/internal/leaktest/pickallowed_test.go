package leaktest

import (
	"slices"
	"strings"
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
	// Caio's sheet holds the class before the switch.
	caiosHeld := must(w.caio.characters.CreateCharacter(t.Context(), rq(&charactersv1.CreateCharacterRequest{
		CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Cantor", Sheet: sheet(class.GetKey(), 1),
	}))).GetCharacter()
	// listed reads the lists of the draft as who sees them: whether the answer came, and whether it lists the key.
	listed := func(t *testing.T, who reader, draft *charactersv1.CharacterSheet) (listed, answered bool, body string) {
		t.Helper()
		r := who.person.call(charactersv1connect.CharacterServicePreviewChoicesProcedure, &charactersv1.PreviewChoicesRequest{
			CampaignId: w.campaign, CharacterId: held.GetId(), Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Sheet: draft,
		})
		if !r.ok() {
			return false, false, string(r.body)
		}
		return slices.Contains(choicesOf(t, r).GetSkillsAllowed().GetKeys(), extra), true, string(r.body)
	}
	master, ana := reader{"the master", w.master, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, true}, reader{"Ana, the owner", w.ana, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, true}
	tableDraft, ownDraft := sheet(class.GetKey(), 1), sheet("class:wizard", 1)
	for _, who := range []reader{master, ana} { // the positive control: before the switch each reads the key
		if l, ok, _ := listed(t, who, tableDraft); !ok || !l {
			t.Fatalf("%s: before the switch the list holds %s = %v (answered %v), the positive control", who.name, extra, l, ok)
		}
	}
	must(w.master.table.SetOptionSwitches(t.Context(), rq(&rulesv1.SetOptionSwitchesRequest{
		CampaignId: w.campaign, Switches: []*rulesv1.OptionSwitch{{Key: class.GetKey(), Off: true}},
	})))
	if l, ok, _ := listed(t, master, tableDraft); !ok || !l {
		t.Errorf("the master reads what the table switched off, answered %v with %s listed %v", ok, extra, l)
	}
	// Ana's draft of the class is refused with the closed choice's answer, which holds nothing of its lists; her own draft is
	// answered with the lists of its own sources alone.
	if l, ok, body := listed(t, ana, tableDraft); l || ok || strings.Contains(body, extra) || strings.Contains(body, "Menestrel da Mesa") {
		t.Errorf("Ana's draft of a class the table switched off: answered %v, lists %s %v: %s", ok, extra, l, body)
	}
	if l, ok, _ := listed(t, ana, ownDraft); l || !ok {
		t.Errorf("Ana's draft of her own class: answered %v with %s listed %v, want the lists without it", ok, extra, l)
	}
	// A draft of a sheet that holds the class is accepted after the switch, and its lists are the ones of the master's read of the
	// same draft: the sheet keeps what it holds, and so its keys; no other key of the class reaches the player.
	read := func(who *person) *charactersv1.PreviewChoicesResponse {
		r := who.call(charactersv1connect.CharacterServicePreviewChoicesProcedure, &charactersv1.PreviewChoicesRequest{
			CampaignId: w.campaign, CharacterId: caiosHeld.GetId(), Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Sheet: sheet(class.GetKey(), 1),
		})
		if !r.ok() {
			t.Fatalf("a draft of the sheet that holds the class: HTTP %d %s", r.status, shorten(r.body))
		}
		return choicesOf(t, r)
	}
	if got, want := read(w.caio), read(w.master); !slices.Equal(got.GetSkillsAllowed().GetKeys(), want.GetSkillsAllowed().GetKeys()) || !slices.Contains(got.GetSkillsAllowed().GetKeys(), extra) {
		t.Errorf("the owner's lists of the sheet that holds the class: skills %v, the master's %v", got.GetSkillsAllowed().GetKeys(), want.GetSkillsAllowed().GetKeys())
	}
}
