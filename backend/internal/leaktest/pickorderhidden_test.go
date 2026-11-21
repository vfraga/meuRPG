package leaktest

import (
	"testing"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The order of the refusals on a new pick: a draft with a skill no list allows beside a tool pick of an owner the master hid
// (switched off or archived), that the stored sheet does not hold, is answered as the same draft with an owner that does not
// exist, whatever call takes it, the preview of the choices included, which refuses no pick that does not fit.
func TestAHiddenToolOwnerIsAnsweredAsAMissingOneBesideAnUnfitSkill(t *testing.T) {
	p := newChoicesProbe(t)
	w, ctx := p.w, t.Context()
	const tool, unfit, absent = "proficiency:smiths-tools", "skill:stealth", "feature:nenhum--dono@mesa"
	held := must(w.ana.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
		CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Perita", Sheet: sheet("class:wizard", 1),
	}))).GetCharacter()
	feature := func(name string) *rulesv1.TableEntry {
		return must(w.master.table.CreateTableEntry(ctx, rq(&rulesv1.CreateTableEntryRequest{
			CampaignId: w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableClassFeature{TableClassFeature: &rulesv1.TableClassFeature{
				NamePt: name, DescPt: []string{"Texto de teste."}, ClassKey: "class:wizard", Level: 1,
				Effects: []*rulesv1.TableEffect{{Type: "choice", Choice: "tool", Count: 1, From: []string{tool, "proficiency:thieves-tools"}}},
			}},
		}))).GetEntry()
	}
	off, archived := feature("Oficio fechado"), feature("Oficio arquivado")
	must(w.master.table.SetOptionSwitches(ctx, rq(&rulesv1.SetOptionSwitchesRequest{CampaignId: w.campaign, Switches: []*rulesv1.OptionSwitch{{Key: off.GetKey(), Off: true}}})))
	must(w.master.table.ArchiveTableEntry(ctx, rq(&rulesv1.ArchiveTableEntryRequest{CampaignId: w.campaign, Key: archived.GetKey()})))
	draft := func(owner string) *charactersv1.CharacterSheet {
		s := sheet("class:wizard", 1)
		s.GetFull().SkillProficiencyKeys = []string{unfit}
		s.GetFull().FeatureChoiceKeys = []string{rules.ScopedChoice(rules.ToolPickKey(owner), tool)}
		return s
	}
	calls := map[string]func(who *person, owner string) reply{
		"create": func(who *person, owner string) reply {
			return who.call(charactersv1connect.CharacterServiceCreateCharacterProcedure, &charactersv1.CreateCharacterRequest{
				CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Nova", Sheet: draft(owner),
			})
		},
		"preview": func(who *person, owner string) reply {
			return who.call(charactersv1connect.CharacterServicePreviewCharacterProcedure, &charactersv1.PreviewCharacterRequest{
				CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Sheet: draft(owner),
			})
		},
		"update": func(who *person, owner string) reply {
			return who.call(charactersv1connect.CharacterServiceUpdateCharacterProcedure, &charactersv1.UpdateCharacterRequest{
				CampaignId: w.campaign, CharacterId: held.GetId(), Revision: held.GetRevision(), Name: held.GetName(), Sheet: draft(owner),
			})
		},
		"preview of the choices": func(who *person, owner string) reply {
			return who.call(charactersv1connect.CharacterServicePreviewChoicesProcedure, &charactersv1.PreviewChoicesRequest{
				CampaignId: w.campaign, CharacterId: held.GetId(), Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Sheet: draft(owner),
			})
		},
	}
	for call, send := range calls {
		who := w.ana
		if call == "create" {
			who = w.caio
		}
		none := send(who, absent)
		if none.ok() {
			t.Errorf("%s: an owner that does not exist is accepted: the row measures nothing", call)
		}
		for how, e := range map[string]*rulesv1.TableEntry{"switched off": off, "archived": archived} {
			named := send(who, e.GetKey())
			if !sameAnswer(named, e.GetKey(), none, absent) {
				t.Errorf("%s, %s owner: HTTP %d %s\nan owner that does not exist: HTTP %d %s", call, how, named.status, shorten(named.body), none.status, shorten(none.body))
			}
		}
	}
}
