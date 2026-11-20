package leaktest

import (
	"testing"

	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// A pick of a choice of alternatives stored under the key of an entry the table keeps out of a player's view (switched off,
// or retired) is answered to a player as a pick stored under a key no entry has, in every call that takes the sheet's
// picks. The master reads what is switched off or retired, and keeps the answer she gets today. A player's own PreviewCharacter
// is refused with 403 whatever the table switches (only the master makes a character for a player, and the master's preview
// with ForPlayer judges the pick in the master's view), so that call cannot show whether the switch decides: the row pins the
// same answer for the player and for a key that does not exist, and the other three calls are the ones the switch decides.
func TestAnAlternativePickStoredUnderAnEntryTheTableKeepsOutIsAnsweredAsAKeyThatDoesNotExist(t *testing.T) {
	const (
		name  = "Zorvak4321"
		value = "=skill:stealth"
	)
	// The choice of alternatives of the owner; the key of a pick carries the owner and the signature of its alternatives.
	tool := []*rulesv1.TableEffect{{Type: "choice", Choice: "alternatives", Count: 1, Alternatives: []*rulesv1.ChoiceAlternative{
		{Kind: "skill", From: []string{"skill:stealth", "skill:persuasion"}},
		{Kind: "tool", Filter: &rulesv1.TableChoiceFilter{ProficiencyKinds: []string{"weapon"}}},
	}}}
	part := alternativesKeyOf("")
	feature := func() *rulesv1.TableFeature {
		return &rulesv1.TableFeature{NamePt: name + " feature", DescPt: []string{"Texto."}, Effects: tool}
	}
	type entry struct{ owner, closed string } // the key that owns the choice, and the entry the table closes
	classEntry := func(p *choicesProbe) entry {
		e := p.createEntry(t, tableClass(name, feature()))
		return entry{e.GetTableClass().GetLevels()[0].GetFeatures()[0].GetKey(), e.GetKey()}
	}
	ways := []struct {
		way   string
		make  func(p *choicesProbe) entry
		close func(p *choicesProbe, key string)
	}{
		{"a feature of a class that is switched off", classEntry, func(p *choicesProbe, key string) {
			must(p.w.master.table.SetOptionSwitches(t.Context(), rq(&rulesv1.SetOptionSwitchesRequest{CampaignId: p.w.campaign, Switches: []*rulesv1.OptionSwitch{{Key: key, Off: true}}})))
		}},
		{"a feat that is switched off", func(p *choicesProbe) entry {
			k := must(p.w.master.table.CreateTableEntry(t.Context(), rq(&rulesv1.CreateTableEntryRequest{
				CampaignId: p.w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableFeat{TableFeat: &rulesv1.TableFeat{NamePt: name + " feat", DescPt: []string{"Texto."}, Effects: tool}},
			}))).GetEntry().GetKey()
			return entry{k, k}
		}, func(p *choicesProbe, key string) {
			must(p.w.master.table.SetOptionSwitches(t.Context(), rq(&rulesv1.SetOptionSwitchesRequest{CampaignId: p.w.campaign, Switches: []*rulesv1.OptionSwitch{{Key: key, Off: true}}})))
		}},
		{"a feature of a class that is archived", classEntry, func(p *choicesProbe, key string) {
			must(p.w.master.table.ArchiveTableEntry(t.Context(), rq(&rulesv1.ArchiveTableEntryRequest{CampaignId: p.w.campaign, Key: key})))
		}},
	}
	const absent = "feature:nenhuma--opcao@mesa"
	for _, w := range ways {
		t.Run(w.way, func(t *testing.T) {
			p := newChoicesProbe(t)
			e := w.make(p)
			base := p.createClass(tableClass("Guardião Base"))
			wd, ctx := p.w, t.Context()
			ana := must(wd.ana.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
				CampaignId: wd.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Cantora", Sheet: sheet(base, 1),
			}))).GetCharacter()
			mine := must(wd.master.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
				CampaignId: wd.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_ENEMY, Name: "Sombra", Sheet: sheet(base, 1),
			}))).GetCharacter()
			w.close(p, e.closed)

			calls := map[string]struct {
				mutates bool
				call    func(who reader, owner string) reply
			}{
				"PreviewChoices": {false, func(who reader, owner string) reply { return p.preview(who, sheet(base, 1, owner+part+value)) }},
				"PreviewCharacter": {false, func(who reader, owner string) reply {
					return who.person.call(charactersv1connect.CharacterServicePreviewCharacterProcedure, &charactersv1.PreviewCharacterRequest{
						CampaignId: wd.campaign, Kind: who.kind, Sheet: sheet(base, 1, owner+part+value), ForPlayer: who.person != wd.master,
					})
				}},
				"CreateCharacter": {true, func(who reader, owner string) reply {
					return who.person.call(charactersv1connect.CharacterServiceCreateCharacterProcedure, &charactersv1.CreateCharacterRequest{
						CampaignId: wd.campaign, Kind: who.kind, Name: "Outro", Sheet: sheet(base, 1, owner+part+value),
					})
				}},
				"UpdateCharacter": {true, func(who reader, owner string) reply {
					target := ana
					if who.person == wd.master {
						target = mine // the master updates a character of her own, so Ana's revision stays
					}
					return who.person.call(charactersv1connect.CharacterServiceUpdateCharacterProcedure, &charactersv1.UpdateCharacterRequest{
						CampaignId: wd.campaign, CharacterId: target.GetId(), Revision: target.GetRevision(), Name: target.GetName(), Sheet: sheet(base, 1, owner+part+value),
					})
				}},
			}
			for what, c := range calls {
				t.Run(what, func(t *testing.T) {
					for _, who := range p.readers() {
						if who.person == wd.master {
							// The master reads what is switched off or retired: today's answer.
							r := c.call(who, e.owner)
							if !r.ok() {
								t.Errorf("the master, %s: HTTP %d %s; want today's answer", what, r.status, shorten(r.body))
							}
							continue
						}
						named, none := c.call(who, e.owner), c.call(who, absent)
						if named.ok() {
							t.Errorf("%s, %s: the pick under the entry is taken: HTTP %d", what, who.name, named.status)
						}
						if !sameAnswer(named, e.owner, none, absent) {
							t.Errorf("%s, %s: the pick under the entry: HTTP %d %s\nthe pick under a key that does not exist: HTTP %d %s", what, who.name, named.status, shorten(named.body), none.status, shorten(none.body))
						}
						if contains(named.body, name) {
							t.Errorf("%s, %s: the answer names %s", what, who.name, name)
						}
					}
				})
			}
		})
	}
}
