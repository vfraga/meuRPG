package play

import (
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/advantage"
	"github.com/PuraFome/meuRPG/backend/internal/play/against"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// A creature's save with advantage from Magic Resistance (RN-20, RN-29): the master reads the mode and
// both dice; a player who does not own the creature reads the outcome only, never the mode that would
// give the trait away.
func TestMagicResistanceSaveModeIsTheMastersAlone(t *testing.T) {
	t.Parallel()
	owner := "user-owner"
	creature := playdb.Combatant{ID: "imp", UserID: &owner}
	sr := &saveRoll{Saved: true, D20: 17, D20B: 4, Counted: 0, Bonus: 2, Total: 19, DC: 15, RollMode: advantage.ModeAdvantageKey}
	master := saveView(sr, combatViewer{master: true}, playdb.Combatant{}, creature)
	if master.GetMode() != playv1.RollMode_ROLL_MODE_ADVANTAGE || len(master.GetRoll().GetFaces()) != 2 { // positive control
		t.Fatalf("the master reads %v, want advantage with two dice", master)
	}
	stranger := saveView(sr, combatViewer{userID: "user-other"}, playdb.Combatant{}, creature)
	if stranger.GetMode() != playv1.RollMode_ROLL_MODE_UNSPECIFIED || stranger.GetRoll() != nil || stranger.GetOutcome() != playv1.SaveOutcome_SAVE_OUTCOME_SAVED {
		t.Errorf("another player reads %v, want the outcome alone", stranger)
	}
}

// creatureFacts carries the trait to the engine, and a spell's save is against magic while a creature's
// own action is not: the spell save has advantage, the breath weapon none.
func TestMagicResistanceReachesTheSaveOfASpellNotOfABreath(t *testing.T) {
	t.Parallel()
	facts := creatureFacts(playdb.Combatant{ID: "imp"}, nil, link.Traits{MagicResistance: true})
	for name, tc := range map[string]struct {
		spell link.Spell
		want  combat.RollMode
	}{
		"a spell":  {link.Spell{Key: "spell:burning-hands"}, combat.ModeAdvantage},
		"a breath": {link.Spell{Key: "spell:burning-hands", Nonmagical: true}, combat.ModeNormal},
	} {
		got := combat.Resolve(combat.SaveMode(combat.SaveScene{Creature: facts, Ability: "dex", EffectVisible: true, Against: against.Spell(tc.spell)}))
		if got != tc.want {
			t.Errorf("%s: mode = %v, want %v", name, got, tc.want)
		}
	}
}
