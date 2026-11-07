package play

import (
	"context"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// turnReads holds what one GetTurnOptions call reads of a player's character, so the sheet and the vitals are
// read once however many parts of the answer want them (Metamagic, the table's cast options, the maneuver dice).
// A part that finds nothing to do never reads them. One call, one goroutine: no lock.
type turnReads struct {
	s          *Service
	campaignID string
	who        playdb.Combatant

	sheet    link.Sheet
	hasSheet bool
	vitals   *playv1.CharacterVitals
}

func (s *Service) turnReadsOf(campaignID string, who playdb.Combatant) *turnReads {
	return &turnReads{s: s, campaignID: campaignID, who: who}
}

func (r *turnReads) sheetOf(ctx context.Context) (link.Sheet, error) {
	if r.hasSheet {
		return r.sheet, nil
	}
	sheet, err := r.s.sheetOf(ctx, nil, r.campaignID, r.who)
	if err != nil {
		return link.Sheet{}, err
	}
	r.sheet, r.hasSheet = sheet, true
	return sheet, nil
}

func (r *turnReads) vitalsOf(ctx context.Context) (*playv1.CharacterVitals, error) {
	if r.vitals != nil {
		return r.vitals, nil
	}
	v, err := r.s.vitals.GetVitals(ctx, r.campaignID, r.who.CharacterID)
	if err != nil {
		return nil, err
	}
	r.vitals = v
	return v, nil
}
