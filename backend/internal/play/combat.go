package play

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// The combat (MR-013): CombatService, in the same Service as
// PlayService because a combat lives inside the open game session and its
// events travel on the same stream. This file has EndTurn and EndEncounter; the
// rest of the lifecycle is in combat_start.go and combat_setup.go (who joins:
// combat_join.go), the phases of EndTurn in combat_endturn.go. The helpers are
// in combat_write.go (the transaction every change shares), combat_view.go
// (what each viewer sees) and combat_rules.go (the pure rules). The write
// calls of a turn (attack, spell, damage, action) read the request into a call,
// run the transaction as a flow and answer from an out; see
// docs/architecture/play/combat.md.
//
// A combat is an encounters row with combatants rows (migration 00043 and
// 00044). Every handler starts with one explicit check, and every change
// takes the open session's row lock, checks the idempotency key, changes the
// rows, writes a session event, and only then, after the commit, publishes.

// EndTurn implements playv1connect.CombatServiceHandler.
func (s *Service) EndTurn(
	ctx context.Context,
	req *connect.Request[playv1.EndTurnRequest],
) (*connect.Response[playv1.EndTurnResponse], error) {
	m, call, err := readEndTurnCall(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	var out endTurnOutcome
	res, err := s.write(ctx, combatWrite{m: m, key: call.key, hash: idem.Hash(req.Msg), kind: eventTurnEnded, altKind: eventTurnPartEnded, encounterID: call.encounterID}, func(c *combatTx) (any, error) {
		out = endTurnOutcome{}
		return s.endTurnWrite(ctx, c, m, call, &out)
	})
	if err != nil {
		return nil, s.dbError(ctx, "end a turn", err)
	}
	enc, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEndTurn(ctx, m.CampaignID, &out, d)
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.EndTurnResponse{Encounter: enc}), nil
}

// EndEncounter implements playv1connect.CombatServiceHandler.
func (s *Service) EndEncounter(
	ctx context.Context,
	req *connect.Request[playv1.EndEncounterRequest],
) (*connect.Response[playv1.EndEncounterResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	encID, err := parseCombatID(req.Msg.GetEncounterId(), "encounter")
	if err != nil {
		return nil, err
	}

	var ended bool
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventEncounterEnded, encounterID: encID}, func(c *combatTx) (any, error) {
		ended = false
		if c.enc.Status == statusEnded {
			return nil, nil // ended before: nothing changes, and no event
		}
		cs, err := listCombatants(ctx, c)
		if err != nil {
			return nil, err
		}
		if err := s.endEncounter(ctx, c, cs); err != nil {
			return nil, err
		}
		ended = true
		return map[string]any{"round": c.enc.Round}, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "end an encounter", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		if !ended {
			return
		}
		s.publishCreaturesOfCombat(m.CampaignID, d.cs)
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, true)
		if d.enc.MapID != nil {
			// The player characters' tokens moved with the combat's end.
			s.tellMembers(m.CampaignID, true, &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_MapChanged_{
				MapChanged: &playv1.WatchGameSessionResponse_MapChanged{MapId: *d.enc.MapID},
			}})
		}
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.EndEncounterResponse{Encounter: out}), nil
}

// endEncounter ends the combat c.enc inside the transaction: its status, and
// the player characters' tokens, and the creatures' that have one, which move to the
// squares where they ended (the map shows them where they stand). cs are its combatants.
func (s *Service) endEncounter(ctx context.Context, c *combatTx, cs []playdb.Combatant) error {
	if err := s.closeEncounter(ctx, c, cs); err != nil {
		return err
	}
	if err := s.settleEndedCombatants(ctx, c, cs); err != nil {
		return err
	}
	if c.enc.MapID == nil {
		return nil // the map was deleted: nothing to write back to
	}
	return s.moveTokensToTheirSquares(ctx, c, cs)
}

// closeEncounter writes the end of the combat itself: what the combat still owed, the
// questions about hidden creatures, and the status.
func (s *Service) closeEncounter(ctx context.Context, c *combatTx, cs []playdb.Combatant) error {
	if err := s.keepTrapDamage(ctx, c, cs); err != nil {
		return err
	}
	// A frenzied rage ends with the combat, and leaves its level of exhaustion (SRD 5.1, Berserker).
	if err := s.endFrenziedRages(ctx, c, cs); err != nil {
		return err
	}
	// What was still to answer about the hidden creatures an area hit goes with the
	// combat: nothing moves on it any more, so there is nothing to decide.
	if err := c.q.DeleteHiddenRevealsOfEncounter(ctx, c.enc.ID); err != nil {
		return fmt.Errorf("drop the hidden reveals: %w", err)
	}
	zs, err := listZones(ctx, c)
	if err != nil {
		return err
	}
	if slices.ContainsFunc(zs, func(z *zone) bool { return z.def.sight != sightNone }) {
		c.zonesMoved = true // the clouds and the darkness go with the combat: the fog is read again
	}
	ended := c.now
	enc, err := c.q.SetEncounterState(ctx, playdb.SetEncounterStateParams{
		ID: c.enc.ID, Status: statusEnded, Round: c.enc.Round, StartedAt: c.enc.StartedAt, EndedAt: &ended,
	})
	if err != nil {
		return fmt.Errorf("end the encounter: %w", err)
	}
	c.enc = enc
	return nil
}

// settleEndedCombatants gives back what the combat held: the reactions, the creatures' hit
// points, the sights, the effects and the concentration.
func (s *Service) settleEndedCombatants(ctx context.Context, c *combatTx, cs []playdb.Combatant) error {
	// What a reaction window held is discarded with the combat, without effect: a held
	// cast or roll never happened, so nothing of it was spent.
	if err := s.discardReactions(ctx, c); err != nil {
		return err
	}
	// The creatures keep the hit points they had when the fight ended (MR-037).
	if err := s.writeBackCreatures(ctx, c, cs); err != nil {
		return err
	}
	// A familiar's sight begun in the fight ends with it (MR-036).
	if err := s.endCombatSights(ctx, c, cs); err != nil {
		return err
	}
	// The temporary states that last a number of rounds end with the fight that counted them.
	if err := s.endCombatStates(ctx, c, cs); err != nil {
		return err
	}
	// The effects on the characters go back to game time with the rounds they have left.
	if err := s.effectsToCharacters(ctx, c, cs, nil); err != nil {
		return err
	}
	// The concentration the combatants carried goes back to the casts (casting_combat.go).
	return s.endCarriedCasts(ctx, c, cs)
}

// moveTokensToTheirSquares moves the tokens of the player characters, and of the creatures
// that have one, to where they stand.
func (s *Service) moveTokensToTheirSquares(ctx context.Context, c *combatTx, cs []playdb.Combatant) error {
	grid := link.Grid{Columns: c.enc.GridColumns, Rows: c.enc.GridRows}
	var positions []link.TokenPosition
	for _, cb := range cs {
		if cb.Kind == kindPlayer && placed(cb) {
			x, y := centerOf(grid, *cb.GridCol, *cb.GridRow)
			positions = append(positions, link.TokenPosition{CharacterID: cb.CharacterID, XBP: x, YBP: y})
		}
		// A creature's token moves too, if the master gave it one (MR-037): the fight
		// never makes a token for it.
		if isCreature(cb) && !cb.Dismissed && placed(cb) {
			x, y := centerOf(grid, *cb.GridCol, *cb.GridRow)
			positions = append(positions, link.TokenPosition{CreatureID: conv.OrZero(cb.CreatureID), XBP: x, YBP: y})
		}
	}
	if err := s.maps.SetTokenPositions(ctx, c.tx, *c.enc.MapID, positions, c.now); err != nil {
		return fmt.Errorf("move the tokens: %w", err)
	}
	return nil
}

// endOpenEncounter ends the session's combat, if it has one, when the master
// ends the session (EndGameSession), inside its transaction, with a session
// event.
func (s *Service) endOpenEncounter(ctx context.Context, tx pgx.Tx, q *playdb.Queries, session playdb.GameSession, actorUserID string) (cs []playdb.Combatant, told []*playv1.CharacterVitals, visionMap string, err error) {
	enc, err := q.GetOpenEncounter(ctx, session.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, "", nil
	}
	if err != nil {
		return nil, nil, "", fmt.Errorf("find the open encounter: %w", err)
	}
	cs, err = q.ListCombatants(ctx, enc.ID)
	if err != nil {
		return nil, nil, "", fmt.Errorf("list the combatants: %w", err)
	}
	c, err := s.openTx(ctx, combatTx{tx: tx, q: q, session: session, enc: enc, now: s.now(), svc: s, actorUserID: actorUserID})
	if err != nil {
		return nil, nil, "", err
	}
	if err := s.endEncounter(ctx, c, cs); err != nil {
		return nil, nil, "", err
	}
	if c.zonesMoved {
		visionMap = conv.OrZero(c.enc.MapID)
	}
	return cs, c.told, visionMap, insertEvent(ctx, c, eventEncounterEnded, &actorUserID, nil, map[string]any{"round": c.enc.Round, "reason": "session_ended"})
}
