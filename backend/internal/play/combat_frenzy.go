package play

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// The Berserker's Frenzy (SRD 5.1, Barbarian, Path of the Berserker): when the barbarian
// rages, it may go into a frenzy for that rage. On each of its turns after the one the
// rage began in, it may make a single melee weapon attack as a bonus action; when the rage
// ends, it gains one level of exhaustion. The choice lives on the rage's state row: a
// frenzied rage has its own combatant as `source_id` (a plain rage has none), so no column
// was needed, and the chip says "Em frenesi".

// frenzyReason is the reason of the log line of the exhaustion a frenzied rage leaves
// (lastingEvent.Reason); the screen writes "Frenesi: +1 nível de exaustão".
const frenzyReason = "frenzy"

// frenzied says the state is a rage the barbarian went into a frenzy for.
func frenzied(st playdb.CombatantState) bool {
	return st.Kind == stateRage && st.SourceID != nil && *st.SourceID == st.CombatantID
}

// frenzyReadyAt says the combatant is in a frenzied rage that began in an earlier round, so
// its turn at round offers the Frenzy attack. A rage begins on the barbarian's own turn, and
// a combatant has one turn a round: a later round is a later turn.
func frenzyReadyAt(states map[string][]playdb.CombatantState, id string, round int32) bool {
	return slices.ContainsFunc(states[id], func(st playdb.CombatantState) bool {
		return frenzied(st) && st.StartedRound < round
	})
}

// frenzyReadyNow is frenzyReadyAt inside a write, for the combat's round.
func (s *Service) frenzyReadyNow(ctx context.Context, c *combatTx, who playdb.Combatant) (bool, error) {
	states, err := s.readStates(ctx, c.tx, c.enc.ID)
	if err != nil {
		return false, err
	}
	return frenzyReadyAt(states, who.ID, c.enc.Round), nil
}

// frenzyReadyIn is frenzyReadyAt for the turn options of a combatant read outside a write: it
// reads the states of the combat and, only when the combatant is frenzied, its round.
func (s *Service) frenzyReadyIn(ctx context.Context, tx pgx.Tx, who playdb.Combatant) (bool, error) {
	q := s.queriesIn(tx)
	rows, err := q.ListCombatantStates(ctx, who.EncounterID)
	if err != nil {
		return false, fmt.Errorf("list the states: %w", err)
	}
	if !slices.ContainsFunc(rows, func(st playdb.CombatantState) bool { return st.CombatantID == who.ID && frenzied(st) }) {
		return false, nil
	}
	enc, err := q.GetEncounterByID(ctx, who.EncounterID)
	if err != nil {
		return false, fmt.Errorf("find the encounter: %w", err)
	}
	return frenzyReadyAt(statesOf(rows), who.ID, enc.Round), nil
}

// afterFrenzyEnded gives the one level of exhaustion a frenzied rage leaves when it ends, by
// whatever path (the turn's answer, the bonus action, the minute, unconsciousness, the master
// ending the combat). ended are the rage states that were just removed; a plain rage in them
// leaves nothing.
func (s *Service) afterFrenzyEnded(ctx context.Context, c *combatTx, who playdb.Combatant, ended []playdb.CombatantState) error {
	if !slices.ContainsFunc(ended, frenzied) {
		return nil
	}
	return s.addExhaustionLevel(ctx, c, who)
}

// addExhaustionLevel puts one level of exhaustion on a combatant inside the combat's
// transaction, as the master's SetExhaustion does (SRD 5.1, Conditions): a character's level
// is its vitals', an NPC's its combatant's, from level 4 the hit point maximum is halved,
// level 6 takes a character to the death confirmation. The line is the owner's and the
// master's alone (RN-10). It does nothing at level 6.
func (s *Service) addExhaustionLevel(ctx context.Context, c *combatTx, who playdb.Combatant) error {
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the combatants: %w", err)
	}
	i := slices.IndexFunc(cs, func(o playdb.Combatant) bool { return o.ID == who.ID })
	if i < 0 {
		return nil
	}
	fresh := cs[i] // the hit points of this very change, not the ones read before it
	before := fresh.ExhaustionLevel
	if fresh.Kind == kindPlayer {
		v, err := s.vitals.GetVitalsTx(ctx, c.tx, c.session.CampaignID, fresh.CharacterID)
		if err != nil {
			return err
		}
		before = v.GetExhaustionLevel()
	}
	if before >= combat.MaxExhaustion {
		return nil
	}
	target := before + 1
	if err := s.raiseExhaustion(ctx, c, fresh, target); err != nil {
		return err
	}
	if err := c.q.SetCombatantExhaustionLevel(ctx, playdb.SetCombatantExhaustionLevelParams{ID: fresh.ID, ExhaustionLevel: target}); err != nil {
		return fmt.Errorf("copy the exhaustion: %w", err)
	}
	if err := s.refreshCombatants(ctx, c, fresh.ID); err != nil {
		return err
	}
	return insertEvent(ctx, c, eventExhaustion, &c.actorUserID, nil, actionEvent{
		Round: c.enc.Round, Secret: fresh.Hidden, Actor: fresh.ID,
		Lasting: &lastingEvent{Key: conditionExhaustion, Change: "exhaustion", Level: target, Before: before, OwnerOnly: true, Targets: []string{fresh.ID}, Reason: frenzyReason},
	})
}

// raiseExhaustion writes the new level where the combatant keeps it: a character's in its
// vitals, an NPC's in its combatant.
func (s *Service) raiseExhaustion(ctx context.Context, c *combatTx, fresh playdb.Combatant, target int32) error {
	if fresh.Kind != kindPlayer {
		_, err := s.setNPCExhaustion(ctx, c, fresh, target)
		return err
	}
	_, after, err := s.vitals.SetExhaustion(ctx, c.tx, c.session.CampaignID, fresh.CharacterID, target)
	if err != nil {
		return err
	}
	c.told = append(c.told, after)
	if target != combat.MaxExhaustion {
		return nil
	}
	// Level 6 is death: the character goes to the confirmation as the third failed death
	// save does; only the master's ConfirmDeath says it died (RN-03).
	if err := c.q.SetCombatantDeathSaves(ctx, playdb.SetCombatantDeathSavesParams{ID: fresh.ID, DeathSuccesses: 0, DeathFailures: combat.DeathSaveGoal, DeathSaveRolled: fresh.DeathSaveRolled, Defeated: fresh.Defeated}); err != nil {
		return fmt.Errorf("take the character to the death confirmation: %w", err)
	}
	return nil
}

// endFrenziedRages ends the frenzied rages of a combat that is ending, with the line of why
// ("left") and the exhaustion each leaves. A plain rage is left as it is.
func (s *Service) endFrenziedRages(ctx context.Context, c *combatTx, cs []playdb.Combatant) error {
	states, err := s.readStates(ctx, c.tx, c.enc.ID)
	if err != nil {
		return err
	}
	for _, cb := range cs {
		if slices.ContainsFunc(states[cb.ID], frenzied) {
			if err := s.endRage(ctx, c, cb, "left"); err != nil {
				return err
			}
		}
	}
	return nil
}
