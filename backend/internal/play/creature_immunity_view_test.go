package play

import (
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// The refusal of a condition by a stat block's or a sheet's immunity (giveCondition) is read by the
// master only (RN-20, RN-29): a player's view of the same hit is the view of a hit that was not refused.
func TestMR042_OnlyTheMasterReadsWhyAConditionWasRefused(t *testing.T) {
	t.Parallel()
	target := playdb.Combatant{ID: "pudding"}
	refused, plain := castHit{Target: "pudding", NoEffectWhy: whyImmune}, castHit{Target: "pudding"}
	master := targetResultProto(actionEvent{}, refused, combatViewer{master: true}, playdb.Combatant{}, target)
	if master.GetNoEffectReasonMaster() != whyImmune {
		t.Errorf("the master reads %q, want %q", master.GetNoEffectReasonMaster(), whyImmune)
	}
	if got := targetResultProto(actionEvent{}, plain, combatViewer{master: true}, playdb.Combatant{}, target); got.GetNoEffectReasonMaster() != "" { // control
		t.Errorf("the master reads %q for a hit nothing refused", got.GetNoEffectReasonMaster())
	}
	player := targetResultProto(actionEvent{}, refused, combatViewer{}, playdb.Combatant{}, target)
	if want := targetResultProto(actionEvent{}, plain, combatViewer{}, playdb.Combatant{}, target); player.String() != want.String() {
		t.Errorf("a player reads %v, want %v (the same as a hit nothing refused)", player, want)
	}
}

func TestSheetImmuneToReadsAStatBlockImmunity(t *testing.T) {
	t.Parallel()
	traits := link.Traits{ConditionImmunities: []link.ConditionImmunity{{Source: "stat-block", Conditions: []string{"condition:prone"}}}}
	if !sheetImmuneTo(traits, "condition:prone") || sheetImmuneTo(traits, "condition:poisoned") {
		t.Error("sheetImmuneTo must hold for the listed condition only")
	}
}
