package leaktest

import (
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1/rulesv1connect"
)

// An entry stored with a key of another kind in the list of a tool choice loads, and only the master reads the issue on it
// (loaded_with_issues): a player, another player and someone outside read none, from the same content.
func TestOnlyTheMasterReadsTheIssueOfAStoredEntryThatLoadsWithAPartLeftOut(t *testing.T) {
	p := newChoicesProbe(t)
	w, ctx := p.w, t.Context()
	entry := must(w.master.table.CreateTableEntry(ctx, rq(&rulesv1.CreateTableEntryRequest{CampaignId: w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableBackground{TableBackground: &rulesv1.TableBackground{
		NamePt: "Cais do Farol", Skills: []string{"skill:athletics", "skill:history"},
		Feature: &rulesv1.TableFeature{NamePt: "Rede do cais", DescPt: []string{"Texto de teste."}, Effects: []*rulesv1.TableEffect{{Type: "choice", Choice: "tool", Count: 1, From: []string{"proficiency:smiths-tools"}}}},
	}}}))).GetEntry()
	// What the write refuses and an earlier version stored: a key of another kind in the list.
	if _, err := w.pool.Exec(ctx, `UPDATE campaign_content SET data = jsonb_set(data, '{feature,effects,0,from}', '["proficiency:smiths-tools","skill:athletics"]'::jsonb) WHERE campaign_id = $1 AND content_key = $2`, w.campaign, entry.GetKey()); err != nil {
		t.Fatalf("store the entry: %v", err)
	}
	if _, err := w.pool.Exec(ctx, `UPDATE campaign_content_state SET revision = revision + 1 WHERE campaign_id = $1`, w.campaign); err != nil {
		t.Fatalf("bump the content revision: %v", err)
	}
	for _, who := range p.readers() {
		r := who.person.call(rulesv1connect.TableContentServiceListTableEntriesProcedure, &rulesv1.ListTableEntriesRequest{CampaignId: w.campaign})
		if who.person != w.master && !r.ok() {
			continue // someone the campaign does not let in reads nothing
		}
		if !r.ok() {
			t.Fatalf("%s: HTTP %d %s", who.name, r.status, shorten(r.body))
		}
		var res rulesv1.ListTableEntriesResponse
		if err := protojson.Unmarshal(r.body, &res); err != nil {
			t.Fatalf("%s: %v", who.name, err)
		}
		got := res.GetLoadedWithIssues()
		switch {
		case who.person == w.master:
			if len(got) != 1 || got[0].GetKey() != entry.GetKey() || got[0].GetField() != "table_background.feature.effects[0].from[1]" || len(res.GetLeftOut()) != 0 {
				t.Errorf("%s reads %v (left out %v), want one issue on the entry", who.name, got, res.GetLeftOut())
			}
		case len(got) != 0:
			t.Errorf("%s reads %v, want none", who.name, got)
		}
	}
}
