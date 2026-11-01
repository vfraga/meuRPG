package play

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// heldDie is the Bardic Inspiration die a character holds now, wherever it is kept: a row of
// character_inspiration out of a combat, the combatant's own die in a running combat (the row is
// gone then, so the two are never both set).
type heldDie struct {
	Sides int32
	// Source is the character of the bard that gave it, nil when it is gone.
	Source *string
	// Combatant is the character's combatant in a running combat, nil out of one; Encounter and
	// Round are then the combat and its round. They are set even when the combatant holds no die
	// (dieOf says not found): a halfling's Lucky question waits in the round without one.
	Combatant *playdb.Combatant
	Encounter string
	Round     int32
}

// rollerState is what the gate knows of the roller: the die it holds now, if any (Holds), wherever
// the die is kept.
type rollerState struct {
	Die   heldDie
	Holds bool
	// Indom says the roller has Indomitable with a use left; the gate reads it for the saving throws only.
	Indom bool
	// Stroke says the roller has Stroke of Luck with a use left; the gate reads it for the attacks and checks that ask it.
	Stroke bool
}

// stateOfCombatant is the state of a roller whose combatant the path already has: the die it holds in
// the round.
func stateOfCombatant(c playdb.Combatant, round int32) rollerState {
	die := heldDie{Combatant: &c, Encounter: c.EncounterID, Round: round}
	if !holdsDie(c, round) {
		return rollerState{Die: die}
	}
	die.Sides = *c.InspirationSides
	return rollerState{Die: die, Holds: true}
}

// rollerStateOf reads the roller's state: the combatant the path gave, or, for a character that does not
// know where it is, the die it holds wherever it is kept (dieOf). It reads through the transaction's
// queries, so inside a change it sees the change.
func (s *Service) rollerStateOf(ctx context.Context, env gateEnv, r heldRoller) (rollerState, error) {
	if r.Combatant != nil {
		return stateOfCombatant(*r.Combatant, r.Round), nil
	}
	die, found, err := s.dieOf(ctx, env.q, r.CampaignID, r.CharacterID)
	return rollerState{Die: die, Holds: found}, err
}

// inCombat is a character's place in the running combat of its campaign.
type inCombat struct {
	enc playdb.Encounter
	cs  []playdb.Combatant
	who playdb.Combatant
}

// combatantOf finds the combatant of a character in the active combat of the campaign's open game
// session; false when it is in none (no session, no combat, a combat not running, or not one of its
// combatants).
func (s *Service) combatantOf(ctx context.Context, q *playdb.Queries, campaignID, characterID string) (inCombat, bool, error) {
	session, err := q.GetOpenGameSession(ctx, campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return inCombat{}, false, nil
	}
	if err != nil {
		return inCombat{}, false, fmt.Errorf("find the open session: %w", err)
	}
	enc, err := q.GetOpenEncounter(ctx, session.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return inCombat{}, false, nil
	}
	if err != nil {
		return inCombat{}, false, fmt.Errorf("find the open encounter: %w", err)
	}
	if enc.Status != statusActive {
		return inCombat{}, false, nil
	}
	cs, err := q.ListCombatants(ctx, enc.ID)
	if err != nil {
		return inCombat{}, false, fmt.Errorf("list the combatants: %w", err)
	}
	i := slices.IndexFunc(cs, func(o playdb.Combatant) bool { return backedByCharacter(o) && o.CharacterID == characterID })
	if i < 0 {
		return inCombat{}, false, nil
	}
	return inCombat{enc: enc, cs: cs, who: cs[i]}, true, nil
}

// dieIn is the die the combatant holds in its combat (and whether it holds one: the combatant and the
// round come back anyway, for the round of a Lucky question).
func dieIn(in inCombat) (heldDie, bool) {
	who := in.who
	die := heldDie{Combatant: &who, Encounter: in.enc.ID, Round: in.enc.Round}
	if !holdsDie(who, in.enc.Round) {
		return die, false
	}
	die.Sides = *who.InspirationSides
	if who.InspirationFrom != nil {
		if j := slices.IndexFunc(in.cs, func(o playdb.Combatant) bool { return o.ID == *who.InspirationFrom && backedByCharacter(o) }); j >= 0 {
			die.Source = &in.cs[j].CharacterID
		}
	}
	return die, true
}

// characterDie is the die a character holds out of a combat: its row of character_inspiration.
func (s *Service) characterDie(ctx context.Context, q *playdb.Queries, campaignID, characterID string) (heldDie, bool, error) {
	row, err := q.GetCharacterInspiration(ctx, playdb.GetCharacterInspirationParams{CharacterID: characterID, CampaignID: campaignID})
	if errors.Is(err, pgx.ErrNoRows) {
		return heldDie{}, false, nil
	}
	if err != nil {
		return heldDie{}, false, fmt.Errorf("find the die of the character: %w", err)
	}
	return heldDie{Sides: row.Sides, Source: row.SourceCharacterID}, true, nil
}

// dieOf finds the die a character holds. While a combat is active and the character is one of
// its combatants the die is the combatant's (and it holds none if the combatant holds none: the
// combatant and the round still come back, for the round of a Lucky question); otherwise it is
// the character's row. It reads through q, so inside a change it sees the change.
func (s *Service) dieOf(ctx context.Context, q *playdb.Queries, campaignID, characterID string) (heldDie, bool, error) {
	in, found, err := s.combatantOf(ctx, q, campaignID, characterID)
	if err != nil {
		return heldDie{}, false, err
	}
	if found {
		die, holds := dieIn(in)
		return die, holds, nil
	}
	return s.characterDie(ctx, q, campaignID, characterID)
}

// spendDie uses up the die a roll settled on: the combatant's own, or the character's row.
func spendDie(ctx context.Context, q *playdb.Queries, characterID string, die heldDie) error {
	if die.Combatant != nil {
		if err := q.SetCombatantInspirationDie(ctx, playdb.SetCombatantInspirationDieParams{ID: die.Combatant.ID}); err != nil {
			return fmt.Errorf("use the die of the combatant: %w", err)
		}
		return nil
	}
	n, err := q.DeleteCharacterInspiration(ctx, characterID)
	if err != nil {
		return fmt.Errorf("use the die: %w", err)
	}
	if n == 0 {
		return errDieGone()
	}
	return nil
}

func errDieGone() error {
	return connect.NewError(connect.CodeFailedPrecondition, errors.New("the character holds no die any more"))
}

// stale says a hold made in a combat no longer has anything to answer: its round is over (or the
// combat ended), or, in inspiration_holds, the die is not the combatant's any more (used or expired) and
// no Lucky question waits, which needs no die. A weapon attack is stale only by its round, as it always
// was in roll_holds, wherever it is kept.
func (h heldRoll) stale(st rollerState) bool {
	if h.Round == nil {
		return false
	}
	if st.Die.Combatant == nil || st.Die.Round != *h.Round {
		return true
	}
	return h.checksTheAnswer() && !st.Holds && !luckyAsks(h) && !indomitableAsks(h) && !strokeAsks(h)
}

// fromSameRequest says the roll was made by the request with this key (a retry of it), which a reaction
// window's replay did not: its question is under a key of its own.
func (h heldRoll) fromSameRequest(key string) bool { return !h.Window && h.Key == key }

// heldRefusal is the refusal of a roll while a roll of the same roller waits: for its Lucky answer
// (which comes first), else for the Bardic Inspiration one.
func heldRefusal(h heldRoll) error {
	if luckyAsks(h) {
		return resourceBlocked(playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_LUCKY_PENDING, "answer the Lucky question of the roll first")
	}
	if indomitableAsks(h) {
		return resourceBlocked(playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_INDOMITABLE_PENDING, "answer the Indomitable question of the roll first")
	}
	if strokeAsks(h) {
		return resourceBlocked(playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_STROKE_OF_LUCK_PENDING, "answer the Stroke of Luck question of the roll first")
	}
	return resourceBlocked(playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_INSPIRATION_PENDING, "answer the Bardic Inspiration question of the roll first")
}
