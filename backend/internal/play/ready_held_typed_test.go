package play

import (
	"testing"

	"connectrpc.com/connect"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// A readied spell attack released with the table's dice is held by the d20 question like any spell attack (#841, #938):
// the typed faces are kept with the hold, the question names the face typed, and the answer lands the cast.
func TestAReadiedSpellAttackReleasedWithTypedFacesAsksItsDieQuestion(t *testing.T) {
	t.Parallel()
	r := d20RerollParty(t, true, d20RerollEffects("2", 2, "1"), "", "Brisa")
	r.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
	r.closeFight(t)
	e := r.turnOf(t, "Brisa")
	r.mustReady(t, r.bia, r.readySpell(t, e, "Brisa", guidingBolt, 1, "Goblin"))
	e = r.mustEndTurn(t, r.master, e)
	w := r.mustFire(t, e, "Brisa")
	res := r.mustAnswer(t, r.bia, e, w.GetId(), useFaces(nil, 2, 15))
	offer := res.GetHeldRollOffer()
	if res.GetResult().GetUsed() || offer == nil {
		t.Fatalf("answer = %v, want the question and no use yet", res)
	}
	wantRerollOffer(t, "the held readied spell attack", offer, playv1.HeldRollKind_HELD_ROLL_KIND_SPELL_ATTACK, 0, rerollUses, 1)
	if got := offer.GetD20().GetFaces(); len(got) != 2 || got[0] != 2 || got[1] != 15 {
		t.Fatalf("the question's d20 = %v, want the typed pair [2 15]", offer.GetD20())
	}
	if lines := r.castLines(t, e, guidingBolt); len(lines) != 0 {
		t.Errorf("cast lines before the answer = %d, want none", len(lines))
	}
	out := r.rolledAgain(t, r.bia, offer, 18)
	targets := out.GetSpellCast().GetCast().GetTargets()
	if len(targets) != 1 || targets[0].GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_HIT {
		t.Fatalf("the cast = %v, want a hit on the new 18", out.GetSpellCast().GetCast())
	}
	if r.readiedStatus(t, "Brisa") != readiedFired {
		t.Errorf("row %q, want fired", r.readiedStatus(t, "Brisa"))
	}
}

// A grant of Scorching Ray at cast level 3 (#881) takes four rays, so four ray_rolls entries at a physical-dice table: the
// count of the rays and of the entries follows the grant's cast level, with no slot.
func TestCastScorchingRayThroughAGrantTakesTheRayRollsOfItsCastLevel(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		rolls [][]int32
		ok    bool
	}{
		{"four entries", [][]int32{rayCrit, rayMiss, rayHit, rayHit}, true},
		{"three entries", [][]int32{rayCrit, rayMiss, rayHit}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a, feat := newLevelParty(t, scorchingRay, 3)
			a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
			e := a.castersFight(t, 1)
			res, err := a.cast(t, a.ana, e, "Pensantus", scorchingRay, nil, rayTargets(t, a, map[string]int32{"Goblin": 4}, "Goblin"), func(r *playv1.CastSpellRequest) {
				grantedCast(feat)(r)
				withRays(rays(tc.rolls...)...)(r)
			})
			if !tc.ok {
				wantCode(t, tc.name, err, connect.CodeInvalidArgument)
				if used, _ := resourceUsed(a.vitals(t, a.pens), graceUses); used != 0 {
					t.Errorf("a refused cast spent %d uses", used)
				}
				return
			}
			if err != nil {
				t.Fatalf("CastSpell error = %v", err)
			}
			if n := len(res.GetCast().GetTargets()); n != 4 {
				t.Errorf("rays = %d, want 4", n)
			}
			noSlotSpent(t, a)
		})
	}
}
