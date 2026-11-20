package leaktest

import (
	"slices"
	"strings"
	"testing"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1/rulesv1connect"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The rows of a choice that mixes kinds (a skill or a language, a skill or a tool): every key it offers is the SRD's, so the
// pool is the same for every reader, and a pick no alternative offers is answered as a key the content does not have.

func alternativesClass(p *choicesProbe) (class, owner string) {
	made := p.createEntry(p.w.t, tableClass("Contrabandista do Farol", &rulesv1.TableFeature{
		NamePt: "Rede do farol", DescPt: []string{"Texto de teste."},
		Effects: []*rulesv1.TableEffect{{Type: "choice", Choice: "alternatives", Count: 1, Alternatives: []*rulesv1.ChoiceAlternative{
			{Kind: "skill", From: []string{"skill:stealth", "skill:persuasion"}},
			{Kind: "tool", Filter: &rulesv1.TableChoiceFilter{ProficiencyKinds: []string{"weapon"}}},
		}}},
	}))
	return made.GetKey(), made.GetTableClass().GetLevels()[0].GetFeatures()[0].GetKey()
}

func alternativesKeyOf(owner string) string {
	return rules.AlternativesKey(owner, &rules.Effect{Count: 1, Alternatives: []rules.ChoiceAlternative{
		{Kind: "skill", From: []string{"skill:stealth", "skill:persuasion"}},
		{Kind: "tool", Filter: &rules.ChoiceFilter{ProficiencyKinds: []string{"weapon"}}},
	}})
}

func TestAChoiceOfAlternativesOffersTheSamePoolToEveryone(t *testing.T) {
	p := newChoicesProbe(t)
	class, owner := alternativesClass(p)
	key := alternativesKeyOf(owner)
	srd, err := rules.LoadSRD()
	if err != nil {
		t.Fatalf("LoadSRD: %v", err)
	}
	want := []string{"skill:persuasion", "skill:stealth"}
	for _, e := range srd.Catalog().Proficiencies {
		if e.Kind == "weapon" {
			want = append(want, e.Key)
		}
	}
	slices.Sort(want)
	for _, who := range p.readers() {
		t.Run(who.name, func(t *testing.T) {
			plain := p.preview(who, sheet(class, 1))
			if who.member != plain.ok() {
				t.Fatalf("HTTP %d for the plain sheet, want a read only for a member", plain.status)
			}
			if !who.member {
				return
			}
			if got := optionsOf(choicesOf(t, plain), key[len(owner):]); !slices.Equal(got, want) {
				t.Errorf("the pool has %d options, want the %d keys of the alternatives and nothing else", len(got), len(want))
			}
			if strings.Contains(string(plain.body), "proficiency:thieves-tools") || strings.Contains(string(plain.body), "skill:arcana") {
				t.Error("the answer names a key no alternative offers")
			}
			for _, inside := range []string{"skill:stealth", "proficiency:glaives"} {
				taken := p.preview(who, sheet(class, 1, key+"="+inside))
				if !taken.ok() || len(choicesOf(t, taken).GetNotOffered()) != 0 {
					t.Errorf("the pick of %s: HTTP %d %s; want it taken", inside, taken.status, shorten(taken.body))
				}
			}
		})
	}
}

// A pick no alternative offers is answered as a pick of a key the content does not have, by every call that takes a pick.
func TestAPickNoAlternativeOffersIsAnsweredAsAKeyThatDoesNotExist(t *testing.T) {
	p := newChoicesProbe(t)
	class, owner := alternativesClass(p)
	key := alternativesKeyOf(owner)
	const absent = "skill:nenhuma-pericia"
	for _, who := range p.readers() {
		for _, outside := range []string{"skill:arcana", "language:elvish", "proficiency:thieves-tools"} {
			named, none := p.preview(who, sheet(class, 1, key+"="+outside)), p.preview(who, sheet(class, 1, key+"="+absent))
			if !sameAnswer(named, outside, none, absent) {
				t.Errorf("%s, PreviewChoices of %s: HTTP %d %s\nof a key that does not exist: HTTP %d %s", who.name, outside, named.status, shorten(named.body), none.status, shorten(none.body))
			}
			call := func(k string) reply {
				return who.person.call(charactersv1connect.CharacterServicePreviewCharacterProcedure, &charactersv1.PreviewCharacterRequest{
					CampaignId: p.w.campaign, Kind: who.kind, Sheet: sheet(class, 1, key+"="+k),
				})
			}
			if named, none := call(outside), call(absent); !sameAnswer(named, outside, none, absent) {
				t.Errorf("%s, PreviewCharacter of %s: HTTP %d %s\nof a key that does not exist: HTTP %d %s", who.name, outside, named.status, shorten(named.body), none.status, shorten(none.body))
			}
		}
	}
}

// What a player reads of the class carries the alternatives as written, and the master reads the same.
func TestTheEntriesAPlayerListsCarryTheAlternativesAsWritten(t *testing.T) {
	p := newChoicesProbe(t)
	class, _ := alternativesClass(p)
	list := &rulesv1.ListTableEntriesRequest{CampaignId: p.w.campaign}
	for _, who := range []*person{p.w.master, p.w.ana} {
		r := who.call(rulesv1connect.TableContentServiceListTableEntriesProcedure, list)
		if !r.ok() || !strings.Contains(string(r.body), class) {
			t.Fatalf("%s: HTTP %d, want the entries with %s", who.name, r.status, class)
		}
		if !strings.Contains(string(r.body), `"alternatives"`) {
			t.Errorf("%s reads no alternatives", who.name)
		}
	}
}
