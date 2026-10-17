package play

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// answerOffer ends an offer without an attack: declined by its reactor's
// controller (or the master), or skipped or withdrawn by the master (state). It
// writes a reaction_declined event that the undo can take back.
func (s *Service) answerOffer(ctx context.Context, m authz.Membership, rawEncounter, rawOffer, rawKey, state string) (*playv1.Encounter, error) {
	key, err := parseKey(rawKey)
	if err != nil {
		return nil, err
	}
	encID, err := parseCombatID(rawEncounter, "encounter")
	if err != nil {
		return nil, err
	}
	offerID, err := parseOfferID(rawOffer)
	if err != nil {
		return nil, err
	}
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: requestHash(state, rawEncounter, rawOffer), kind: eventReactionDeclined, encounterID: encID}, func(c *combatTx) (any, error) {
		return answerInTx(ctx, c, m, offerID, state)
	})
	if err != nil {
		return nil, s.dbError(ctx, "answer an opportunity offer", err)
	}
	return s.finish(ctx, m, res, s.changed(m.CampaignID))
}

// answerInTx is answerOffer's write: find the offer, check who may answer it,
// store the state and describe it for the log.
func answerInTx(ctx context.Context, c *combatTx, m authz.Membership, offerID, state string) (any, error) {
	if err := notEnded(c.enc); err != nil {
		return nil, err
	}
	o, err := c.q.GetOpportunityOffer(ctx, playdb.GetOpportunityOfferParams{EncounterID: c.enc.ID, ID: offerID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errOfferGone()
	}
	if err != nil {
		return nil, fmt.Errorf("find the opportunity offer: %w", err)
	}
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return nil, fmt.Errorf("list the combatants: %w", err)
	}
	v := c.viewer(m, cs)                              // the fog: an NPC the player does not see is not found
	reactor, err := findCombatant(cs, o.ReactorID, v) // not_found for a reactor the player does not see
	if err != nil {
		return nil, err
	}
	if err := v.mayAct(reactor); err != nil {
		return nil, err
	}
	if err := checkAnswerable(o, state); err != nil {
		return nil, err
	}
	if err := storeAnswer(ctx, c, o, state); err != nil {
		return nil, err
	}
	c.characterID = &reactor.CharacterID
	mover, _ := findCombatant(cs, o.MoverID, combatViewer{master: true})
	return actionEvent{Round: c.enc.Round, Secret: secretOf(reactor, mover), Actor: reactor.ID, Target: o.MoverID, OfferID: o.ID, Skipped: state == offerSkipped, Withdrawn: state == offerWithdrawn}, nil
}

// checkAnswerable refuses an offer that was answered already, and a withdrawal of
// an offer a move made.
func checkAnswerable(o playdb.OpportunityOffer, state string) error {
	if o.State != offerPending {
		return errOfferGone()
	}
	if state == offerWithdrawn && o.LeftCol != nil {
		return errTheatreOnly() // only the master's own offers are taken back: a move's offer is answered or skipped
	}
	return nil
}

// storeAnswer writes the offer's new state and touches the encounter.
func storeAnswer(ctx context.Context, c *combatTx, o playdb.OpportunityOffer, state string) error {
	if _, err := c.q.SetOpportunityOfferState(ctx, playdb.SetOpportunityOfferStateParams{ID: o.ID, State: state, AnsweredAt: &c.now}); err != nil {
		return fmt.Errorf("answer the opportunity offer: %w", err)
	}
	if err := touchEncounter(ctx, c); err != nil {
		return err
	}
	return nil
}
