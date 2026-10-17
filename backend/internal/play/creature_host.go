package play

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// What package characters asks of the combat that holds a character's
// creatures (MR-037; characters.CreatureHost). The creatures are the
// characters module's; a combat is this one's, so dismissing a creature takes
// its combatant out of the fight from here, in the same transaction.

// CreaturesLeaving takes the combatants of the creatures out of the campaign's
// combat that is not ended, inside tx, with a combatant_removed event, and ends
// the sight of a player looking through one of them. It returns the combat's ID,
// empty when none of them was in one and none was seen through.
func (s *Service) CreaturesLeaving(ctx context.Context, tx pgx.Tx, campaignID, actorUserID string, creatureIDs []string, at time.Time) (string, error) {
	q := s.queries.WithTx(tx)
	session, enc, found, err := openCombatOf(ctx, q, campaignID)
	if err != nil || !found {
		return "", err
	}
	cs, err := q.ListCombatants(ctx, enc.ID)
	if err != nil {
		return "", fmt.Errorf("list the combatants: %w", err)
	}
	leaving := creatureCombatantsOf(cs, creatureIDs)
	// A player looking through a familiar that goes stops seeing: the sight is the
	// familiar's, and the conditions it gave leave with it, now, not at the start
	// of the character's next turn.
	blind, err := s.lookingThrough(ctx, tx, campaignID, cs, creatureIDs)
	if err != nil {
		return "", err
	}
	if len(leaving) == 0 && len(blind) == 0 {
		return "", nil
	}
	c, err := s.openTx(ctx, combatTx{tx: tx, q: q, session: session, enc: enc, now: at, actorUserID: actorUserID, svc: s})
	if err != nil {
		return "", err
	}
	cs, leaving, err = s.endSights(ctx, c, blind, creatureIDs, cs, leaving)
	if err != nil {
		return "", err
	}
	if err := s.dismissLeavers(ctx, c, cs, leaving); err != nil {
		return "", err
	}
	if c.enc, err = q.TouchEncounter(ctx, enc.ID); err != nil {
		return "", fmt.Errorf("touch the encounter: %w", err)
	}
	if len(leaving) > 0 {
		if err := insertEvent(ctx, c, eventCombatantRemoved, &actorUserID, nil, actionEvent{Round: c.enc.Round, Reason: "creature_dismissed", Created: creatureIDsOf(leaving)}); err != nil {
			return "", err
		}
	}
	return enc.ID, nil
}

// openCombatants is the open combat of the session and its combatants; false when the session
// has no combat that is not ended.
func openCombatants(ctx context.Context, q *playdb.Queries, sessionID string) (playdb.Encounter, []playdb.Combatant, bool, error) {
	enc, err := q.GetOpenEncounter(ctx, sessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return enc, nil, false, nil
	}
	if err != nil {
		return enc, nil, false, fmt.Errorf("find the open encounter: %w", err)
	}
	cs, err := q.ListCombatants(ctx, enc.ID)
	if err != nil {
		return enc, nil, false, fmt.Errorf("list the combatants: %w", err)
	}
	return enc, cs, true, nil
}

// The questions "is this character in the combat" differ on purpose, so each one has a
// predicate of its own and the caller names the one it asks.

// undismissedOf matches a combatant of the character, of any kind, that was not dismissed: a
// defeated one still counts (a cast outside the combat is refused for it).
func undismissedOf(characterID string) func(playdb.Combatant) bool {
	return func(o playdb.Combatant) bool { return o.CharacterID == characterID && !o.Dismissed }
}

// playerRowOf matches the character's player combatant, dismissed or defeated or not.
func playerRowOf(characterID string) func(playdb.Combatant) bool {
	return func(o playdb.Combatant) bool { return o.Kind == kindPlayer && o.CharacterID == characterID }
}

// playerNotDefeatedOf matches the character's player combatant while it is still up: a defeated
// one is out of the fight, so a Revivify request is allowed for it.
func playerNotDefeatedOf(characterID string) func(playdb.Combatant) bool {
	return func(o playdb.Combatant) bool {
		return o.Kind == kindPlayer && o.CharacterID == characterID && !o.Defeated
	}
}

// defeatedPlayerRowOf matches the character's player combatant once it is defeated.
func defeatedPlayerRowOf(characterID string) func(playdb.Combatant) bool {
	return func(o playdb.Combatant) bool {
		return o.Kind == kindPlayer && o.CharacterID == characterID && o.Defeated
	}
}

// openCombatOf is the campaign's open session, locked, and its open combat; false when
// there is no session or no combat.
func openCombatOf(ctx context.Context, q *playdb.Queries, campaignID string) (playdb.GameSession, playdb.Encounter, bool, error) {
	session, err := q.GetOpenGameSessionForUpdate(ctx, campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return session, playdb.Encounter{}, false, nil // no session: no combat
	}
	if err != nil {
		return session, playdb.Encounter{}, false, fmt.Errorf("lock the open session: %w", err)
	}
	enc, err := q.GetOpenEncounter(ctx, session.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return session, enc, false, nil
	}
	if err != nil {
		return session, enc, false, fmt.Errorf("find the open encounter: %w", err)
	}
	return session, enc, true, nil
}

// endSights ends the sight of the players who look through a creature that leaves and
// reads the combatants again, as their conditions changed; cs and leaving are returned as
// they were when none looks through one.
func (s *Service) endSights(ctx context.Context, c *combatTx, blind []playdb.Combatant, creatureIDs []string, cs, leaving []playdb.Combatant) (all, going []playdb.Combatant, err error) {
	for _, who := range blind {
		if err := s.endSight(ctx, c, who, sightFamiliarGone, false); err != nil {
			return nil, nil, err
		}
	}
	if len(blind) == 0 {
		return cs, leaving, nil
	}
	if cs, err = c.q.ListCombatants(ctx, c.enc.ID); err != nil { // their conditions changed
		return nil, nil, fmt.Errorf("list the combatants: %w", err)
	}
	return cs, creatureCombatantsOf(cs, creatureIDs), nil
}

// creatureCombatantsOf are the combatants of cs that are one of the creatures.
func creatureCombatantsOf(cs []playdb.Combatant, creatureIDs []string) []playdb.Combatant {
	return slices.DeleteFunc(slices.Clone(cs), func(o playdb.Combatant) bool {
		return o.CreatureID == nil || !slices.Contains(creatureIDs, *o.CreatureID)
	})
}

// lookingThrough are the players of cs whose character looks through one of the creatures.
func (s *Service) lookingThrough(ctx context.Context, tx pgx.Tx, campaignID string, cs []playdb.Combatant, creatureIDs []string) ([]playdb.Combatant, error) {
	var blind []playdb.Combatant
	for _, who := range cs {
		if who.Kind != kindPlayer {
			continue
		}
		vitals, err := s.vitals.GetVitalsTx(ctx, tx, campaignID, who.CharacterID)
		if connect.CodeOf(err) == connect.CodeNotFound {
			continue // the character died or left meanwhile
		}
		if err != nil {
			return nil, err
		}
		if sight := vitals.GetFamiliarSight(); sight != nil && slices.Contains(creatureIDs, sight.GetCreatureId()) {
			blind = append(blind, who)
		}
	}
	return blind, nil
}

// dismissLeavers takes the combatants of the creatures that leave out of the combat; cs
// are the combatants as they are. Nothing is done when none leaves.
func (s *Service) dismissLeavers(ctx context.Context, c *combatTx, cs, leaving []playdb.Combatant) error {
	if len(leaving) == 0 {
		return nil
	}
	// A creature that leaves keeps the hit points it has (it may be dismissed
	// right after, which the characters module settles).
	if err := s.writeBackCreatures(ctx, c, leaving); err != nil {
		return err
	}
	if err := s.discardDamageOf(ctx, c, cs, leaving); err != nil {
		return err
	}
	_, _, err := s.dropCombatants(ctx, c, cs, leaving, false)
	return err
}

// CreatureInCombat says whether the creature is a combatant of the campaign's
// combat that is not ended, inside tx.
func (s *Service) CreatureInCombat(ctx context.Context, tx pgx.Tx, campaignID, creatureID string) (bool, error) {
	rows, err := s.queries.WithTx(tx).ListOpenCombatantsOfCreatures(ctx, playdb.ListOpenCombatantsOfCreaturesParams{CampaignID: campaignID, CreatureIds: []string{creatureID}})
	if err != nil {
		return false, fmt.Errorf("find the creature in a combat: %w", err)
	}
	return len(rows) > 0, nil
}

// PublishCreaturesChanged tells the master and the owner's player that their
// creature lists changed. Call it after the commit.
func (s *Service) PublishCreaturesChanged(campaignID, ownerUserID string) {
	s.publishCreaturesChanged(campaignID, ownerUserID)
}

// PublishEncounterChanged tells every stream that the combat changed. Call it
// after the commit.
func (s *Service) PublishEncounterChanged(ctx context.Context, campaignID, encounterID string) {
	enc, err := s.queries.GetEncounterByID(ctx, encounterID)
	if err != nil {
		return // the combat is gone: nothing to tell
	}
	s.publishEncounterChanged(ctx, campaignID, enc)
	s.publishLogChanged(ctx, campaignID, enc.ID, false) // the damage waiting on it was dropped: the master's line; whether a player's was hidden is not told here
	// A creature that left may have passed the turn, and the new turn ends a familiar's
	// sight (MR-036): the vitals and the fog hear of it too.
	if all, err := s.vitals.ListVitals(ctx, campaignID); err == nil {
		for _, v := range all {
			s.publishVitals(campaignID, v)
		}
	}
	s.maps.VisionChanged(ctx, campaignID, deref(enc.MapID))
}

// LockSession takes the open session's row (characters.CreatureHost): the lock
// every combat write takes first.
func (s *Service) LockSession(ctx context.Context, tx pgx.Tx, campaignID string) error {
	if _, err := s.queries.WithTx(tx).GetOpenGameSessionForUpdate(ctx, campaignID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("lock the open session: %w", err)
	}
	return nil
}

// CreatureRenamed gives the creature's combatant its new name (characters.CreatureHost).
func (s *Service) CreatureRenamed(ctx context.Context, tx pgx.Tx, campaignID, creatureID, name string) (string, error) {
	ids, err := s.queries.WithTx(tx).SetCreatureCombatantLabel(ctx, playdb.SetCreatureCombatantLabelParams{CreatureID: creatureID, Label: name, CampaignID: campaignID})
	if err != nil {
		return "", fmt.Errorf("rename the creature's combatant: %w", err)
	}
	if len(ids) == 0 {
		return "", nil
	}
	return ids[0], nil
}
