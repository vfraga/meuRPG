package leaktest

import (
	"slices"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The changed entries of a sheet carry the code of the issue each sentence comes from (ChangedContent.codes). A feat
// the player holds is edited to ask for a class the table switched off: the player reads the sentence without the class
// and the entry's codes are the fixed code of the issue, one per sentence, with no word of the class; the master's read
// of the same sheet is the positive control (it names the class, with the same codes).
func TestTheCodesOfAChangedFeatNameNothingTheTableHides(t *testing.T) {
	w := newWorld(t)
	ctx := t.Context()
	m := w.master
	className := w.secrets.marker("changed-codes-hidden-class")
	class := must(m.table.CreateTableEntry(ctx, rq(&rulesv1.CreateTableEntryRequest{
		CampaignId: w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableClass{TableClass: summonerClass(className, "Prática", "Texto.", nil)},
	}))).GetEntry()
	w.secrets.id("changed-codes-hidden-class", class.GetKey())
	rules := must(m.campaigns.GetTableRules(ctx, rq(&campaignsv1.GetTableRulesRequest{CampaignId: w.campaign}))).GetRules()
	rules.FeatsAllowed = true
	must(m.campaigns.SetTableRules(ctx, rq(&campaignsv1.SetTableRulesRequest{CampaignId: w.campaign, Rules: rules})))
	must(m.table.SetOptionSwitches(ctx, rq(&rulesv1.SetOptionSwitchesRequest{CampaignId: w.campaign, Switches: []*rulesv1.OptionSwitch{{Key: class.GetKey(), Off: true}}})))
	feat := must(m.table.CreateTableEntry(ctx, rq(&rulesv1.CreateTableEntryRequest{
		CampaignId: w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableFeat{TableFeat: &rulesv1.TableFeat{NamePt: w.secrets.public("changed-codes-feat"), DescPt: []string{"Texto."}}},
	}))).GetEntry()

	cur := must(m.characters.GetCharacter(ctx, rq(&charactersv1.GetCharacterRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId()}))).GetCharacter()
	sheet := proto.CloneOf(cur.GetSheet())
	sheet.GetFull().FeatKeys = []string{feat.GetKey()}
	must(m.characters.UpdateCharacter(ctx, rq(&charactersv1.UpdateCharacterRequest{
		CampaignId: w.campaign, CharacterId: w.pens.GetId(), Revision: cur.GetRevision(), Name: cur.GetName(), Sheet: sheet,
	})))
	body := proto.CloneOf(feat.GetTableFeat())
	body.Prerequisite = &rulesv1.FeatPrerequisite{ClassLevel: &rulesv1.FeatClassLevel{ClassKey: class.GetKey(), Level: 3}}
	must(m.table.UpdateTableEntry(ctx, rq(&rulesv1.UpdateTableEntryRequest{
		CampaignId: w.campaign, Key: feat.GetKey(), ExpectedRevision: feat.GetRevision(),
		Body: &rulesv1.UpdateTableEntryRequest_TableFeat{TableFeat: body},
	})))

	read := func(p *person) (*rulesv1.ChangedContent, string) {
		t.Helper()
		res := p.call(charactersv1connect.CharacterServiceGetCharacterProcedure, &charactersv1.GetCharacterRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId()})
		got := must(p.characters.GetCharacter(ctx, rq(&charactersv1.GetCharacterRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId()}))).GetCharacter()
		for _, ch := range got.GetDerived().GetChangedContent() {
			if ch.GetKey() == feat.GetKey() {
				return ch, string(res.body)
			}
		}
		t.Fatalf("%s reads no changed entry for the feat: %s", p.name, shorten(res.body))
		return nil, ""
	}
	owner, ownerBody := read(w.ana)
	control, masterBody := read(m)
	if !slices.Equal(owner.GetCodes(), control.GetCodes()) || len(owner.GetCodes()) != len(owner.GetMessages()) || len(owner.GetCodes()) == 0 {
		t.Errorf("Ana reads codes %v for %d sentences, the master %v", owner.GetCodes(), len(owner.GetMessages()), control.GetCodes())
	}
	for _, c := range owner.GetCodes() {
		if strings.Contains(c, className) || strings.Contains(c, class.GetKey()) {
			t.Errorf("a code of Ana's read names the hidden class: %q", c)
		}
	}
	for _, hidden := range []string{className, class.GetKey()} {
		if strings.Contains(ownerBody, hidden) {
			t.Errorf("Ana's sheet names %q, an entry the table hides: %s", hidden, shorten([]byte(ownerBody)))
		}
	}
	if !strings.Contains(masterBody, className) {
		t.Errorf("the master's sheet does not name %q (the positive control): %s", className, shorten([]byte(masterBody)))
	}
}
