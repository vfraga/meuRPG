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
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// Exhaustion (SRD 5.1, Conditions): six levels the master sets, each adding to the ones
// below it (1 disadvantage on ability checks; 2 speed halved; 3 disadvantage on attack rolls
// and saving throws; 4 hit point maximum halved; 5 speed 0; 6 death). A character keeps its
// level in its vitals, an NPC in the combat; level 4 halves the maximum and cuts the current
// hit points to it, taking the level off gives the maximum back and never heals, and level 6
// takes a character to the death confirmation (ConfirmDeath) as the third failed death save
// does. A long rest with food and drink takes one level off (SRD, Long Rest); the app does not
// count food, so the master takes it off here, and the rest offers it.

// exhaustionTarget is who the master changed the exhaustion of.
type exhaustionTarget struct {
	characterID string
	combatant   *playdb.Combatant
}

// deathFailuresToDie are the failed death saves that bring a character to the confirmation of its death (SRD 5.1).
const deathFailuresToDie = 3

// SetExhaustion implements playv1connect.LastingEffectServiceHandler.
func (s *Service) SetExhaustion(
	ctx context.Context,
	req *connect.Request[playv1.SetExhaustionRequest],
) (*connect.Response[playv1.SetExhaustionResponse], error) {
	m, key, subject, err := s.exhaustionCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetIdempotencyKey(), req.Msg.GetEncounterId(), req.Msg.GetCharacterId(), req.Msg.GetCombatantId())
	if err != nil {
		return nil, err
	}
	if req.Msg.GetLevel() < 0 || req.Msg.GetLevel() > combat.MaxExhaustion {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("level must be 0 to 6"))
	}
	level, hpMax, out, err := s.changeExhaustion(ctx, m, key, idem.Hash(req.Msg), subject, req.Msg.GetLevel(), req.Msg.GetExpectedLevel(), req.Msg.GetConfirmDeath(), eventExhaustion)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.SetExhaustionResponse{Level: level, HitPointsMax: hpMax, Encounter: out}), nil
}

// LowerExhaustion implements playv1connect.LastingEffectServiceHandler.
func (s *Service) LowerExhaustion(
	ctx context.Context,
	req *connect.Request[playv1.LowerExhaustionRequest],
) (*connect.Response[playv1.LowerExhaustionResponse], error) {
	m, key, subject, err := s.exhaustionCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetIdempotencyKey(), req.Msg.GetEncounterId(), req.Msg.GetCharacterId(), req.Msg.GetCombatantId())
	if err != nil {
		return nil, err
	}
	by := req.Msg.GetBy()
	if by < 1 || by > combat.MaxExhaustion {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("by must be 1 to 6"))
	}
	if req.Msg.GetReason() == playv1.ExhaustionLowerReason_EXHAUSTION_LOWER_REASON_UNSPECIFIED {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("reason is a long rest or the master"))
	}
	// A level lowered by a count: the target level is worked out inside, from the level now.
	level, hpMax, out, err := s.changeExhaustion(ctx, m, key, idem.Hash(req.Msg), subject, -by, req.Msg.GetExpectedLevel(), false, eventExhaustion)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.LowerExhaustionResponse{Level: level, HitPointsMax: hpMax, Encounter: out}), nil
}

// exhaustionCall checks who calls and names the subject: a character, or a combatant of a combat.
func (s *Service) exhaustionCall(ctx context.Context, campaignID, rawKey, rawEnc, rawChar, rawComb string) (authz.Membership, string, exhaustionTarget, error) {
	m, err := authz.RequireCampaignRole(ctx, campaignID, authz.RoleMaster)
	if err != nil {
		return m, "", exhaustionTarget{}, err
	}
	key, err := parseKey(rawKey)
	if err != nil {
		return m, "", exhaustionTarget{}, err
	}
	switch {
	case rawChar != "":
		id, ok := parseID(rawChar)
		if !ok {
			return m, key, exhaustionTarget{}, errCharacterNotFound()
		}
		return m, key, exhaustionTarget{characterID: id}, nil
	case rawComb != "":
		if _, err := parseCombatID(rawEnc, "encounter"); err != nil {
			return m, key, exhaustionTarget{}, err
		}
		id, err := parseCombatID(rawComb, "combatant")
		if err != nil {
			return m, key, exhaustionTarget{}, err
		}
		return m, key, exhaustionTarget{combatant: &playdb.Combatant{ID: id, EncounterID: rawEnc}}, nil
	}
	return m, key, exhaustionTarget{}, connect.NewError(connect.CodeInvalidArgument, errors.New("set character_id or combatant_id"))
}

// exhaustionChange is what a change of exhaustion asks for.
type exhaustionChange struct {
	subject exhaustionTarget
	// level is the level to set; a negative number takes that many levels off the one it is now.
	level int32
	// expected is the level the master's screen shows.
	expected     int32
	confirmDeath bool
}

// exhaustionDone is what the transaction leaves for the answer and the streams.
type exhaustionDone struct {
	level, hpMax int32
	vitals       *playv1.CharacterVitals
	// hintLog says the players are told the log changed: false when the line is the owner's and the
	// master's, or secret.
	hintLog bool
}

// changeExhaustion sets the level (a negative number takes that many levels off the one it is
// now) of a character or a combatant in one transaction: the vitals or the combatant's row, the
// state the effects leave, the death confirmation of level 6 and the line of the log.
func (s *Service) changeExhaustion(ctx context.Context, m authz.Membership, key string, hash *string, subject exhaustionTarget, level, expected int32, confirmDeath bool, kind string) (int32, int32, *playv1.Encounter, error) {
	ch := exhaustionChange{subject: subject, level: level, expected: expected, confirmDeath: confirmDeath}
	var done exhaustionDone
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: hash, kind: kind, encounterID: exhaustionEncounterID(subject)}, func(c *combatTx) (any, error) {
		done = exhaustionDone{}
		return s.writeExhaustion(ctx, c, m, ch, &done)
	})
	if err != nil {
		return 0, 0, nil, s.dbError(ctx, "set the exhaustion", err)
	}
	var enc *playv1.Encounter
	if res.encounterID != "" {
		enc, err = s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
			s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
			s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, done.hintLog)
		})
		if err != nil {
			return 0, 0, nil, err
		}
	}
	s.publishVitals(m.CampaignID, done.vitals)
	return done.level, done.hpMax, enc, nil
}

// exhaustionEncounterID is the combat the call names, when it names a combatant.
func exhaustionEncounterID(subject exhaustionTarget) string {
	if subject.combatant == nil {
		return ""
	}
	if id, err := parseCombatID(subject.combatant.EncounterID, "encounter"); err == nil {
		return id
	}
	return subject.combatant.EncounterID
}

// writeExhaustion is the transaction of changeExhaustion: find who it is, check the level the
// screen showed, write the level, copy it to the combatant and write the line.
func (s *Service) writeExhaustion(ctx context.Context, c *combatTx, m authz.Membership, ch exhaustionChange, done *exhaustionDone) (any, error) {
	cs, err := s.exhaustionCombat(ctx, c)
	if err != nil {
		return nil, err
	}
	who, err := exhaustionWho(cs, ch.subject, m)
	if err != nil {
		return nil, err
	}
	before, err := s.exhaustionBefore(ctx, c, ch.subject, who)
	if err != nil {
		return nil, err
	}
	target, err := ch.targetFrom(before)
	if err != nil {
		return nil, err
	}
	ev := actionEvent{Round: c.enc.Round, Secret: who != nil && who.Hidden}
	if who != nil {
		ev.Actor = who.ID
		c.characterID = &who.CharacterID
	}
	if err := s.applyExhaustion(ctx, c, ch.subject, who, target, before, done); err != nil {
		return nil, err
	}
	done.level = target
	if who != nil {
		if err := s.copyExhaustion(ctx, c, *who, target); err != nil {
			return nil, err
		}
	}
	if c.enc.ID != "" {
		if err := touchEncounter(ctx, c); err != nil {
			return nil, err
		}
	}
	// The exhaustion is the owner's and the master's: no line for the other players (RN-10).
	ev.Lasting = &lastingEvent{Key: conditionExhaustion, Change: "exhaustion", Level: target, Before: before, OwnerOnly: true}
	if who != nil {
		ev.Lasting.Targets = []string{who.ID}
	}
	done.hintLog = !ev.Secret && !ev.Lasting.OwnerOnly
	return ev, nil
}

// exhaustionCombat loads the combat the change runs in (a character's is the session's, if it
// runs) and its combatants; nil when there is none.
func (s *Service) exhaustionCombat(ctx context.Context, c *combatTx) ([]playdb.Combatant, error) {
	if c.enc.ID == "" { // a character, and the combat of the session if it runs
		enc, err := c.q.GetLatestEncounter(ctx, c.session.ID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("find the encounter: %w", err)
		}
		if err == nil && enc.Status != statusEnded {
			c.enc = enc
		}
	}
	if c.enc.ID == "" {
		return nil, nil
	}
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return nil, fmt.Errorf("list the combatants: %w", err)
	}
	return cs, nil
}

// exhaustionWho is the combatant the change is about: the one named, or the character's in the
// combat; nil for a character that is not in it.
func exhaustionWho(cs []playdb.Combatant, subject exhaustionTarget, m authz.Membership) (*playdb.Combatant, error) {
	if subject.combatant != nil {
		cb, err := findCombatant(cs, subject.combatant.ID, viewerOf(m))
		if err != nil {
			return nil, err
		}
		return &cb, nil
	}
	if i := slices.IndexFunc(cs, func(o playdb.Combatant) bool { return o.Kind == kindPlayer && o.CharacterID == subject.characterID }); i >= 0 {
		return &cs[i], nil
	}
	return nil, nil
}

// exhaustionCharacterID is the character whose vitals hold the level: the combatant's, or the one named.
func exhaustionCharacterID(subject exhaustionTarget, who *playdb.Combatant) string {
	if who != nil {
		return who.CharacterID
	}
	return subject.characterID
}

// exhaustionBefore is the level now: a character's is its vitals', an NPC's its combatant's.
func (s *Service) exhaustionBefore(ctx context.Context, c *combatTx, subject exhaustionTarget, who *playdb.Combatant) (int32, error) {
	if who != nil && who.Kind != kindPlayer {
		return who.ExhaustionLevel, nil
	}
	v, err := s.vitals.GetVitalsTx(ctx, c.tx, c.session.CampaignID, exhaustionCharacterID(subject, who))
	if err != nil {
		return 0, err
	}
	return v.GetExhaustionLevel(), nil
}

// targetFrom is the level to set, once the level now is the one the screen showed; level 6 is
// death and asks for the master's confirmation.
func (ch exhaustionChange) targetFrom(before int32) (int32, error) {
	if ch.expected != before {
		return 0, connect.NewError(connect.CodeAborted, errors.New("the level of exhaustion is not the one the screen shows"))
	}
	target := ch.level
	if ch.level < 0 {
		target = max(before+ch.level, 0)
	}
	if target == combat.MaxExhaustion && !ch.confirmDeath && target != before {
		return 0, connect.NewError(connect.CodeFailedPrecondition, errors.New("level 6 is death: confirm_death must be true"))
	}
	return target, nil
}

// applyExhaustion writes the level: an NPC's on its combatant, a character's in its vitals.
func (s *Service) applyExhaustion(ctx context.Context, c *combatTx, subject exhaustionTarget, who *playdb.Combatant, target, before int32, done *exhaustionDone) error {
	if who != nil && who.Kind != kindPlayer {
		hp, err := s.setNPCExhaustion(ctx, c, *who, target)
		if err != nil {
			return err
		}
		done.hpMax = hp
		return nil
	}
	return s.setCharacterExhaustion(ctx, c, subject, who, target, before, done)
}

// setCharacterExhaustion sets a character's level in its vitals; level 6 takes it to the death
// confirmation (in a combat, as the third failed death save does) or, with no combat to hold
// one, the master's confirm_death is the confirmation (RN-03) and it dies as the master's
// "marcar como morto" leaves it.
func (s *Service) setCharacterExhaustion(ctx context.Context, c *combatTx, subject exhaustionTarget, who *playdb.Combatant, target, before int32, done *exhaustionDone) error {
	charID := exhaustionCharacterID(subject, who)
	_, after, err := s.vitals.SetExhaustion(ctx, c.tx, c.session.CampaignID, charID, target)
	if err != nil {
		return err
	}
	done.vitals, done.hpMax = after, after.GetHitPointsMax()
	if charID != "" && who == nil {
		c.characterID = &charID
	}
	dies := target == combat.MaxExhaustion
	if who == nil && dies && target != before {
		return s.roster.MarkDead(ctx, c.tx, c.session.CampaignID, charID, c.now)
	}
	if who != nil && dies {
		if err := c.q.SetCombatantDeathSaves(ctx, playdb.SetCombatantDeathSavesParams{ID: who.ID, DeathSuccesses: 0, DeathFailures: deathFailuresToDie, DeathSaveRolled: who.DeathSaveRolled, Defeated: who.Defeated}); err != nil {
			return fmt.Errorf("take the character to the death confirmation: %w", err)
		}
	}
	return nil
}

// copyExhaustion copies the level to the combatant and works out what it leaves on it.
func (s *Service) copyExhaustion(ctx context.Context, c *combatTx, who playdb.Combatant, target int32) error {
	if err := c.q.SetCombatantExhaustionLevel(ctx, playdb.SetCombatantExhaustionLevelParams{ID: who.ID, ExhaustionLevel: target}); err != nil {
		return fmt.Errorf("copy the exhaustion: %w", err)
	}
	return s.refreshCombatants(ctx, c, who.ID)
}

// setNPCExhaustion puts a level of exhaustion on an NPC: from level 4 its maximum is halved and its
// hit points cut to it, and the maximum it had is kept to give it back; level 6 defeats it. It returns
// the maximum it has now.
func (s *Service) setNPCExhaustion(ctx context.Context, c *combatTx, who playdb.Combatant, level int32) (int32, error) {
	base := num(who.HpMax)
	if who.HpMaxBase != nil {
		base = *who.HpMaxBase
	}
	newMax := clamp32(combat.ExhaustedMaxHP(int(base), int(level)), 0, 1<<30)
	current := num(who.HpCurrent)
	var baseKept *int32
	if combat.ExhaustionAt(int(level)).MaxHPHalved {
		baseKept = &base
		current = min(current, newMax)
	}
	pct := int32(100)
	switch ex := combat.ExhaustionAt(int(level)); {
	case ex.SpeedZero:
		pct = 0
	case ex.SpeedHalved:
		pct = 50
	}
	defeated := who.Defeated || level >= combat.MaxExhaustion
	if level >= combat.MaxExhaustion {
		current = 0
	}
	var hpMax, hpCur *int32
	if who.HpMax != nil {
		hpMax, hpCur = &newMax, &current
	}
	if err := c.q.SetCombatantExhaustion(ctx, playdb.SetCombatantExhaustionParams{ID: who.ID, ExhaustionLevel: level, HpMax: hpMax, HpCurrent: hpCur, HpMaxBase: baseKept, EffectSpeedPct: pct, Defeated: defeated}); err != nil {
		return 0, fmt.Errorf("set the exhaustion: %w", err)
	}
	if defeated && !who.Defeated && c.enc.Status == statusActive {
		cs, err := c.q.ListCombatants(ctx, c.enc.ID)
		if err != nil {
			return 0, fmt.Errorf("list the combatants: %w", err)
		}
		if _, err := leaveTurn(ctx, c, cs, who); err != nil {
			return 0, err
		}
	}
	return newMax, nil
}
