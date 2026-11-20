package leaktest

import (
	"slices"
	"strings"
	"testing"

	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// A pick stored under the key of an entry the table keeps out of a player's view (switched off, or retired) is answered to
// a player as a pick stored under a key no entry has: the same status, the same code, the same message and the same field,
// in every call that takes the sheet's picks, and no answer names the entry. A pick the sheet already holds stays.
func TestAPickStoredUnderAnEntryTheTableKeepsOutIsAnsweredAsAKeyThatDoesNotExist(t *testing.T) {
	const (
		name  = "Zorvak4321"
		part  = "#tool"
		value = "=proficiency:glaives"
	)
	tool := []*rulesv1.TableEffect{{Type: "choice", Choice: "tool", Count: 1}}
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
							if !r.ok() || (what == "PreviewChoices" && !slices.Contains(choicesOf(t, r).GetNotOffered(), e.owner+part+value)) {
								t.Errorf("the master, %s: HTTP %d %s; want today's answer", what, r.status, shorten(r.body))
							}
							continue
						}
						named, none := c.call(who, e.owner), c.call(who, absent)
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

// A pick the sheet already holds under the key of an entry the table closed afterwards stays with its holder (RN-23): her
// own sheet still takes it, in the preview and in the update, and nothing is refused.
func TestAPickHeldUnderAnEntryTheTableClosedAfterwardsStaysWithItsHolder(t *testing.T) {
	for _, way := range []string{"switched off", "archived", "a feat switched off", "a feat archived"} {
		t.Run(way, func(t *testing.T) {
			p := newChoicesProbe(t)
			w, ctx := p.w, t.Context()
			tool := []*rulesv1.TableEffect{{Type: "choice", Choice: "tool", Count: 1}}
			var closed, owner string // the entry the table closes, and the key that owns the choice
			var mk func(picks ...string) *charactersv1.CharacterSheet
			var plain *charactersv1.CharacterSheet // the sheet Ana makes, before the master adds anything
			if strings.HasPrefix(way, "a feat") {
				closed = must(w.master.table.CreateTableEntry(ctx, rq(&rulesv1.CreateTableEntryRequest{
					CampaignId: w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableFeat{TableFeat: &rulesv1.TableFeat{NamePt: "Zorvak4321 feat", DescPt: []string{"Texto."}, Effects: tool}},
				}))).GetEntry().GetKey()
				owner = closed
				base := p.createClass(tableClass("Guardião Base"))
				plain = sheet(base, 1)
				mk = func(picks ...string) *charactersv1.CharacterSheet {
					s := sheet(base, 1, picks...)
					s.GetFull().FeatKeys = []string{closed}
					return s
				}
			} else {
				e := p.createEntry(t, tableClass("Zorvak4321", &rulesv1.TableFeature{NamePt: "Zorvak4321 feature", DescPt: []string{"Texto."}, Effects: tool}))
				closed = e.GetKey()
				owner = e.GetTableClass().GetLevels()[0].GetFeatures()[0].GetKey()
				plain = sheet(closed, 1)
				mk = func(picks ...string) *charactersv1.CharacterSheet { return sheet(closed, 1, picks...) }
			}
			res := choicesOf(t, p.preview(reader{person: w.ana, kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER}, mk()))
			opts := optionsOf(res, "#tool")
			if len(opts) == 0 {
				t.Fatalf("the choice of tools has no option: %v", res)
			}
			pick := owner + "#tool=" + opts[0]
			// Only the master adds a feat to a sheet: Ana's sheet gets the feat and the pick from her.
			held := must(w.ana.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
				CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Cantora", Sheet: plain,
			}))).GetCharacter()
			held = must(w.master.characters.UpdateCharacter(ctx, rq(&charactersv1.UpdateCharacterRequest{
				CampaignId: w.campaign, CharacterId: held.GetId(), Revision: held.GetRevision(), Name: held.GetName(), Sheet: mk(pick),
			}))).GetCharacter()
			if strings.HasSuffix(way, "archived") {
				must(w.master.table.ArchiveTableEntry(ctx, rq(&rulesv1.ArchiveTableEntryRequest{CampaignId: w.campaign, Key: closed})))
			} else {
				must(w.master.table.SetOptionSwitches(ctx, rq(&rulesv1.SetOptionSwitchesRequest{CampaignId: w.campaign, Switches: []*rulesv1.OptionSwitch{{Key: closed, Off: true}}})))
			}
			r := w.ana.call(charactersv1connect.CharacterServicePreviewChoicesProcedure, &charactersv1.PreviewChoicesRequest{
				CampaignId: w.campaign, CharacterId: held.GetId(), Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Sheet: mk(pick),
			})
			if !r.ok() || len(choicesOf(t, r).GetNotOffered()) != 0 {
				t.Errorf("PreviewChoices of the held pick: HTTP %d %s", r.status, shorten(r.body))
			}
			u := w.ana.call(charactersv1connect.CharacterServiceUpdateCharacterProcedure, &charactersv1.UpdateCharacterRequest{
				CampaignId: w.campaign, CharacterId: held.GetId(), Revision: held.GetRevision(), Name: "Cantora", Sheet: mk(pick),
			})
			if !u.ok() {
				t.Errorf("UpdateCharacter keeping the held pick: HTTP %d %s", u.status, shorten(u.body))
			}
		})
	}
}
