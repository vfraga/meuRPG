package play

import (
	"context"
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

type countingVitals struct {
	VitalsKeeper
	reads int
}

func (c *countingVitals) GetVitals(_ context.Context, _, id string) (*playv1.CharacterVitals, error) {
	c.reads++
	return &playv1.CharacterVitals{CharacterId: id}, nil
}

// The parts of a turn's options that want the vitals (the table's cast options, the maneuver dice) share one read
// of them, however many ask: a call reads them once.
func TestTheTurnReadsOfACallReadTheVitalsOnce(t *testing.T) {
	t.Parallel()
	keeper := &countingVitals{}
	r := (&Service{vitals: keeper}).turnReadsOf("camp", playdb.Combatant{CharacterID: "ch1"})
	first, err := r.vitalsOf(t.Context())
	if err != nil {
		t.Fatalf("vitalsOf() error = %v", err)
	}
	second, err := r.vitalsOf(t.Context())
	if err != nil || second != first || keeper.reads != 1 {
		t.Errorf("after two asks: reads = %d, same value = %v, err = %v; want one read and the same vitals", keeper.reads, second == first, err)
	}
}
