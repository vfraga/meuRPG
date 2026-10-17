package play

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// What the changes of a character's form share: Wild Shape (wildshape.go) and the familiar's
// sight (familiarsight.go) are two changes of one character inside the table's play, made by
// the character's player or the master. Each finds the character and where it stands, may
// spend the action of a turn, seals its event, and answers the same way.

// formScene is where the character stands when a form change begins: its combatant and the
// combat's, when it is in a combat that is not ended; turns says the combat has begun, so the
// change costs the action of a turn.
type formScene struct {
	who      *playdb.Combatant
	cs       []playdb.Combatant
	inCombat bool
	turns    bool
	mapID    string
}

// formSceneOf finds where the character stands (playerCharacterOf) and the map the change reaches:
// the combat's, or the session's current one outside a combat.
func (s *Service) formSceneOf(ctx context.Context, c *combatTx, m authz.Membership, characterID string) (formScene, error) {
	_, who, cs, err := s.playerCharacterOf(ctx, c, m, characterID)
	if err != nil {
		return formScene{}, err
	}
	f := formScene{who: who, cs: cs, inCombat: who != nil, mapID: deref(c.session.CurrentMapID)}
	f.turns = f.inCombat && c.enc.Status == statusActive // in SETUP nobody has a turn to spend
	if f.inCombat {
		f.mapID = deref(c.enc.MapID)
	}
	return f, nil
}

// seal fills in the event's owner and, in a combat, its combat, round and actor, touches the
// combat and names the character the change was about.
func (f formScene) seal(ctx context.Context, c *combatTx, characterID string, made *actionEvent) error {
	made.OwnerCharacter = characterID
	if f.inCombat {
		made.EncounterID = c.enc.ID
		if made.Actor == "" { // in SETUP no action is spent: still the combatant's
			made.Round, made.Secret, made.Actor = c.enc.Round, f.who.Hidden, f.who.ID
		}
		if err := touchEncounter(ctx, c); err != nil {
			return err
		}
	}
	c.characterID = &characterID
	return nil
}

// answerFormChange is what a change of a form or a sight answers with, after its
// transaction: a retry of another character's change is refused, a retry answers
// with the vitals as they are now, and a change made in a combat tells the combat.
func (s *Service) answerFormChange(ctx context.Context, m authz.Membership, characterID string, res combatResult, ev actionEvent, vitals *playv1.CharacterVitals) (*playv1.CharacterVitals, *playv1.Encounter, error) {
	if res.repeated && ev.OwnerCharacter != characterID {
		// The key is another character's change: never answer with that character's vitals.
		return nil, nil, errKeyReused()
	}
	if res.repeated {
		var err error
		if vitals, err = s.vitals.GetVitals(ctx, m.CampaignID, characterID); err != nil {
			return nil, nil, s.dbError(ctx, "get vitals", err)
		}
	}
	if ev.EncounterID == "" {
		return vitals, nil, nil
	}
	res.encounterID = ev.EncounterID
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !ev.Secret)
	})
	if err != nil {
		return nil, nil, err
	}
	return vitals, out, nil
}

// putBackSpentAction gives back what a change spent on its turn: the economy and
// the running start the event kept. It does nothing for a change that spent
// nothing (the master's, or one made in SETUP).
func putBackSpentAction(ctx context.Context, c *combatTx, who playdb.Combatant, ev actionEvent) error {
	if !ev.Spent {
		return nil
	}
	if err := c.q.SetCombatantEconomy(ctx, playdb.SetCombatantEconomyParams{
		ID: who.ID, ActionUsed: ev.ActionBefore, BonusActionUsed: ev.BonusBefore, ReactionUsed: ev.ReactionBefore, Dashed: ev.DashedBefore,
	}); err != nil {
		return fmt.Errorf("put back the economy: %w", err)
	}
	if ev.RunBefore != 0 {
		if err := c.q.SetCombatantRun(ctx, playdb.SetCombatantRunParams{ID: who.ID, LastMoveDft: ev.RunBefore}); err != nil {
			return fmt.Errorf("put back the running start: %w", err)
		}
	}
	return nil
}

// playerCharacterOf finds the character a caller may change inside a write: a
// living player's character of the campaign that the master, or its own player,
// acts for. It returns it, and the combat it is in, if it is a combatant of one that
// is not ended (c.enc is then set).
func (s *Service) playerCharacterOf(ctx context.Context, c *combatTx, m authz.Membership, characterID string) (link.Character, *playdb.Combatant, []playdb.Combatant, error) {
	chars, err := s.roster.CombatCharacters(ctx, c.tx, m.CampaignID, []string{characterID})
	if err != nil {
		return link.Character{}, nil, nil, err
	}
	if len(chars) != 1 || !chars[0].Player {
		return link.Character{}, nil, nil, errCharacterNotFound()
	}
	if m.Role != authz.RoleMaster && chars[0].PlayerUserID != m.UserID {
		return link.Character{}, nil, nil, connect.NewError(connect.CodePermissionDenied, errors.New("only the character's player or the master may do this"))
	}
	enc, cs, found, err := openCombatants(ctx, c.q, c.session.ID)
	if err != nil {
		return link.Character{}, nil, nil, err
	}
	if !found {
		return chars[0], nil, nil, nil
	}
	isHer := playerRowOf(characterID)
	for i := range cs {
		if isHer(cs[i]) {
			c.enc = enc
			return chars[0], &cs[i], cs, nil
		}
	}
	return chars[0], nil, nil, nil
}

// spendAction spends the combatant's action (or its bonus action) for a change
// made on its turn in a running combat, and fills in what an undo puts back. The
// master may act again with the action used. It checks that it is the combatant's
// turn and that it may act, and adds what an undo puts back to ev.
func (s *Service) spendAction(ctx context.Context, c *combatTx, v combatViewer, who playdb.Combatant, bonus bool, ev *actionEvent) error {
	if err := s.mustActNow(ctx, c, who); err != nil {
		return err
	}
	used, reason, what := who.ActionUsed, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ACTION_USED, "the action of this turn is used"
	if bonus {
		used, reason, what = who.BonusActionUsed, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_BONUS_ACTION_USED, "the bonus action of this turn is used"
	}
	if used && !v.master {
		return errEncounter(reason, what)
	}
	run, err := breakRun(ctx, c, who)
	if err != nil {
		return err
	}
	ev.Round, ev.Secret, ev.Actor, ev.RunBefore = c.enc.Round, who.Hidden, who.ID, run
	ev.ActionBefore, ev.BonusBefore, ev.ReactionBefore, ev.DashedBefore = who.ActionUsed, who.BonusActionUsed, who.ReactionUsed, who.Dashed
	ev.Spent = true
	after := who
	if bonus {
		after.BonusActionUsed = true
	} else {
		after.ActionUsed = true
	}
	if err := c.q.SetCombatantEconomy(ctx, playdb.SetCombatantEconomyParams{
		ID: who.ID, ActionUsed: after.ActionUsed, BonusActionUsed: after.BonusActionUsed, ReactionUsed: after.ReactionUsed, Dashed: after.Dashed,
	}); err != nil {
		return fmt.Errorf("spend the action: %w", err)
	}
	return nil
}
