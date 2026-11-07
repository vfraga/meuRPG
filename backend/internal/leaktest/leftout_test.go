package leaktest

import (
	"strings"
	"testing"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1/rulesv1connect"
)

// TestLeftOutIsTheMastersAlone: the entries the rules refuse (the fixture's stored spells with a level of 12) are left out of the
// campaign, and the master alone reads which, why, and how many. The master's list names the live one and not the one he
// retired ("Remover"); a player's list, which is also what the campaign page's notice counts from, carries no left_out, no count,
// no path and not the name or the key of the retired entry.
func TestLeftOutIsTheMastersAlone(t *testing.T) {
	w := newWorld(t)
	list := &rulesv1.ListTableEntriesRequest{CampaignId: w.campaign}

	// The positive control: the master reads the live refused spell, with its field and reason, and not the retired one.
	master := must(w.master.table.ListTableEntries(t.Context(), rq(list)))
	got := master.GetLeftOut()
	if len(got) != 1 || got[0].GetKey() != w.leftOutKey {
		t.Fatalf("the master's left_out = %v, want only the live refused spell %s", got, w.leftOutKey)
	}
	if got[0].GetField() == "" || got[0].GetReason() == "" {
		t.Errorf("the master's left_out has no field or reason: %v", got[0])
	}
	masterBody := string(w.master.call(rulesv1connect.TableContentServiceListTableEntriesProcedure, list).body)
	if !strings.Contains(masterBody, "leftOut") {
		t.Errorf("the master's answer has no leftOut (the positive control): %s", shorten([]byte(masterBody)))
	}
	for _, v := range got {
		if v.GetKey() == w.retiredLeftOutKey {
			t.Errorf("the retired spell is still in the master's left_out")
		}
	}

	// A player reads none of it, whoever asks and whichever the field: the answer has no left_out at all, so the count
	// the campaign page makes of it is nothing, and the retired entry is not named.
	for _, p := range []*person{w.ana, w.caio} {
		resp := must(p.table.ListTableEntries(t.Context(), rq(list)))
		if n := len(resp.GetLeftOut()); n != 0 {
			t.Errorf("%s reads %d entries in left_out", p.name, n)
		}
		body := string(p.call(rulesv1connect.TableContentServiceListTableEntriesProcedure, list).body)
		for what, needle := range map[string]string{
			"a left_out field": "leftOut", "the path of the refused field": got[0].GetField(), "the key of the retired entry": w.retiredLeftOutKey,
			"the name of the retired entry": w.retiredLeftOutName, "the engine's reason": got[0].GetReason() + `"`,
		} {
			if needle != "" && strings.Contains(body, needle) {
				t.Errorf("%s reads %s in the entries: %s", p.name, what, shorten([]byte(body)))
			}
		}
	}
}
