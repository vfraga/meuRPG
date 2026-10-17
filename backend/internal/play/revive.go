package play

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// Bringing the dead back (RN-03, SRD 5.1 "Dropping to 0 Hit Points" and Revivify). The master's
// "Reviver" is CharacterService.ReviveCharacter, which asks this package, through
// characters.ReviewHost, to put the character's combatant back in the combat it was in. The spell
// Revivify (revivify.go) does the same through reviveCombatant. A creature that lives again
// keeps its place in the initiative order and acts on its next turn: the SRD has no "no turn
// this round" rule.

// eventCharacterRevived is the log line "O mestre reviveu <name>" (session_event_kinds).
const eventCharacterRevived = "character_revived"

// The events characters.ReviewHost.PublishCharacterEvent names.
const (
	characterChangesRequested = "changes_requested"
	characterResubmitted      = "resubmitted"
	characterRevived          = "revived"
)

// reviveCombatant puts a dead combatant back in the fight where it was in the order, with no
// death save counted, out of the running turn (so it acts on its next one). A player's
// character gets its hit points from the characters module (the caller did it); an NPC gets
// 1 hit point here. The combat is touched.
func (s *Service) reviveCombatant(ctx context.Context, c *combatTx, who playdb.Combatant) error {
	if err := c.q.ReviveCombatant(ctx, who.ID); err != nil {
		return fmt.Errorf("revive the combatant: %w", err)
	}
	if who.Kind != kindPlayer {
		if err := c.q.SetCombatantHitPoints(ctx, playdb.SetCombatantHitPointsParams{
			ID: who.ID, HpCurrent: new(int32(1)), HpTemp: who.HpTemp, Defeated: false,
		}); err != nil {
			return fmt.Errorf("give the creature its hit point: %w", err)
		}
	}
	if err := touchEncounter(ctx, c); err != nil {
		return err
	}
	return nil
}

// ReviveInCombat implements characters.ReviewHost: the character's combatant in the campaign's
// combat that is not ended, if it is dead there (the master confirmed the death in this combat),
// lives again, inside tx, and the combat's log says "O mestre reviveu <name>". It returns the
// combat's ID, or "" when the character is in none, or is in it but was not defeated (the
// master marked it dead from its page): then nothing in the combat changes, and the caller
// writes the line to the session.
func (s *Service) ReviveInCombat(ctx context.Context, tx pgx.Tx, campaignID, characterID, actorUserID string, at time.Time) (string, error) {
	q := s.queries.WithTx(tx)
	// Dying ended what was on the character; living again brings back none of it (RN-22).
	if err := q.DeleteCharacterEffectsOfCharacter(ctx, characterID); err != nil {
		return "", fmt.Errorf("clear the old effects of the revived character: %w", err)
	}
	if _, _, err := s.vitals.SetArmorBase(ctx, tx, campaignID, characterID, 0); err != nil && connect.CodeOf(err) != connect.CodeNotFound {
		return "", err
	}
	session, enc, found, err := openCombatOf(ctx, q, campaignID)
	if err != nil || !found {
		return "", err
	}
	cs, err := q.ListCombatants(ctx, enc.ID)
	if err != nil {
		return "", fmt.Errorf("list the combatants: %w", err)
	}
	who := defeatedPlayerOf(cs, characterID)
	if who == nil {
		return "", nil
	}
	c, err := s.openTx(ctx, combatTx{tx: tx, q: q, session: session, enc: enc, now: at, actorUserID: actorUserID, svc: s, master: true})
	if err != nil {
		return "", err
	}
	if err := s.reviveCombatant(ctx, c, *who); err != nil {
		return "", err
	}
	c.characterID = &characterID
	if err := insertEvent(ctx, c, eventCharacterRevived, &actorUserID, nil, actionEvent{Round: c.enc.Round, Actor: who.ID, Secret: who.Hidden}); err != nil {
		return "", err
	}
	return enc.ID, nil
}

// defeatedPlayerOf is the combatant of the character in cs when it is defeated, the last
// one when several match; nil otherwise.
func defeatedPlayerOf(cs []playdb.Combatant, characterID string) *playdb.Combatant {
	var who *playdb.Combatant
	for i := range cs {
		if cs[i].Kind == kindPlayer && cs[i].CharacterID == characterID && cs[i].Defeated {
			who = &cs[i]
		}
	}
	return who
}

// PublishCharacterEvent implements characters.ReviewHost: the hint that a pending character's
// review changed (to the master and the owner), or that a dead character lives again (the same,
// and the vitals, which everyone in the session reads). It carries no reason (RN-10). Call it
// after the commit.
func (s *Service) PublishCharacterEvent(ctx context.Context, campaignID, ownerUserID, characterID, event string) {
	var msg *playv1.WatchGameSessionResponse
	switch event {
	case characterChangesRequested:
		msg = &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_CharacterChangesRequested_{
			CharacterChangesRequested: &playv1.WatchGameSessionResponse_CharacterChangesRequested{CharacterId: characterID},
		}}
	case characterResubmitted:
		msg = &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_CharacterResubmitted_{
			CharacterResubmitted: &playv1.WatchGameSessionResponse_CharacterResubmitted{CharacterId: characterID},
		}}
	case characterRevived:
		msg = &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_CharacterRevived_{
			CharacterRevived: &playv1.WatchGameSessionResponse_CharacterRevived{CharacterId: characterID},
		}}
		if v, err := s.vitals.GetVitals(ctx, campaignID, characterID); err == nil {
			s.publishVitals(campaignID, v)
		}
	default:
		return
	}
	s.hub.Publish(campaignID, live.Event{Audience: live.Audience{Master: true, UserID: ownerUserID}, Message: msg})
}
