package characters

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// Finding U7-5 (review/unit-07-characters.md): featureKeys runs before any bound
// on the number of features (MaxTableFeatures and MaxTableEntryBytes are checked
// later), and keyAssigner.assign rebuilds the slug set for every unkeyed feature
// and probes -2, -3, ... linearly, so a body within the 4 MiB request limit costs
// far more than a refusal should, inside the transaction that holds the campaign's
// content revision row.
func TestReview7_ManyUnkeyedFeaturesAreRefusedFast(t *testing.T) {
	h := newHarness(t)
	master := h.newUser("Samuel")
	campaign := h.newCampaign(master, "Mirathel")

	for _, n := range []int{5000} {
		race := testRace("Raça enorme")
		race.Traits = make([]*rulesv1.TableFeature, n)
		for i := range race.Traits {
			race.Traits[i] = &rulesv1.TableFeature{NamePt: "x"}
		}
		ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
		start := time.Now()
		_, err := master.table.CreateTableEntry(ctx, connect.NewRequest(createReq(campaign, race)))
		elapsed := time.Since(start)
		cancel()
		code := connect.CodeOf(err)
		t.Logf("N=%d: %v, code=%v", n, elapsed, code)
		if code != connect.CodeInvalidArgument && code != connect.CodeResourceExhausted {
			t.Errorf("N=%d: code = %v (err %v), want invalid_argument or resource_exhausted", n, code, err)
		}
		if elapsed > 2*time.Second {
			t.Errorf("N=%d: the refusal took %v, want under 2s", n, elapsed)
		}
	}
}
