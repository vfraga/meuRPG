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

// The same list with the source one table class feature the master archived: the feature is on a class the table keeps (the
// SRD wizard), so no class filter can hide it, and only the filter of the sources keeps its key out of a player's list.
func TestAnArchivedClassFeatureAddsNoSkillKeyToAPlayersListForADraft(t *testing.T) {
	p := newChoicesProbe(t)
	w, ctx := p.w, t.Context()
	const extra = "skill:performance"
	held := must(w.ana.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
		CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Menestrel", Sheet: sheet("class:wizard", 1),
	}))).GetCharacter()
	listed := func(who *person) bool {
		r := who.call(charactersv1connect.CharacterServicePreviewChoicesProcedure, &charactersv1.PreviewChoicesRequest{
			CampaignId: w.campaign, CharacterId: held.GetId(), Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Sheet: sheet("class:wizard", 1),
		})
		if !r.ok() {
			t.Fatalf("PreviewChoices: HTTP %d %s", r.status, shorten(r.body))
		}
		return slices.Contains(choicesOf(t, r).GetSkillsAllowed().GetKeys(), extra)
	}
	feature := must(w.master.table.CreateTableEntry(ctx, rq(&rulesv1.CreateTableEntryRequest{
		CampaignId: w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableClassFeature{TableClassFeature: &rulesv1.TableClassFeature{
			NamePt: "Canto", DescPt: []string{"Texto de teste."}, ClassKey: "class:wizard", Level: 1,
			Effects: []*rulesv1.TableEffect{{Type: "choice", Choice: "skill", Count: 1, From: []string{extra}}},
		}},
	}))).GetEntry()
	for name, who := range map[string]*person{"the master": w.master, "Ana": w.ana} {
		if !listed(who) {
			t.Fatalf("%s: before the archive the list lacks %s, the positive control", name, extra)
		}
	}
	must(w.master.table.ArchiveTableEntry(ctx, rq(&rulesv1.ArchiveTableEntryRequest{CampaignId: w.campaign, Key: feature.GetKey()})))
	if !listed(w.master) {
		t.Errorf("the master reads what is archived, and the list lacks %s", extra)
	}
	if listed(w.ana) {
		t.Errorf("Ana's list holds %s of an archived class feature", extra)
	}
}
