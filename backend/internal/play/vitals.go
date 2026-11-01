package play

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// The kinds of session_events rows (session_event_kinds).
const eventCharacterVitalsAdjusted = "character_vitals_adjusted"

// adjustCall is what AdjustCharacterVitals reads from its request before the transaction.
type adjustCall struct {
	m        authz.Membership
	charText string
	keyText  string
	hash     *string
	msg      *playv1.AdjustCharacterVitalsRequest
}

// adjustRun is what the transaction of AdjustCharacterVitals leaves for after the commit.
type adjustRun struct {
	after        *playv1.CharacterVitals
	repeated     bool
	touched      *playdb.Encounter
	check        concentrationCheck // what the damage asks of a spell the character concentrates on
	castsEnded   bool
	visionMap    string // the map whose fog the change touches
	shapeChanged bool
}

// readAdjustCall reads and checks the request. The refusals come in the order of the checks below.
func readAdjustCall(ctx context.Context, msg *playv1.AdjustCharacterVitalsRequest) (adjustCall, error) {
	m, err := authz.RequireCampaignRole(ctx, msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return adjustCall{}, err
	}
	characterID, err := uuid.Parse(msg.GetCharacterId())
	if err != nil {
		return adjustCall{}, connect.NewError(connect.CodeNotFound, errors.New("character not found"))
	}
	key, err := uuid.Parse(msg.GetIdempotencyKey())
	if err != nil {
		return adjustCall{}, connect.NewError(connect.CodeInvalidArgument, errors.New("idempotency_key must be a UUID"))
	}
	return adjustCall{m: m, charText: characterID.String(), keyText: key.String(), hash: idem.Hash(msg), msg: msg}, nil
}

// AdjustCharacterVitals implements playv1connect.PlayServiceHandler: the
// master's correction of a character's vitals (RN-02).
//
// One transaction locks the open session's row, checks the idempotency
// key, changes the vitals (through the VitalsKeeper) and appends the
// session event. The lock makes two corrections in the same session take
// turns, so the events get their numbers in order. Only after the commit
// does the change go out on the live streams.
func (s *Service) AdjustCharacterVitals(
	ctx context.Context,
	req *connect.Request[playv1.AdjustCharacterVitalsRequest],
) (*connect.Response[playv1.AdjustCharacterVitalsResponse], error) {
	call, err := readAdjustCall(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	var run adjustRun
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		run = adjustRun{} // a retry starts over
		return s.adjustWrite(ctx, tx, call, &run)
	})
	if err != nil {
		return nil, s.dbError(ctx, "adjust a character's vitals", err)
	}
	if run.repeated {
		// Nothing changed and nothing goes out: answer with the vitals as
		// they are now.
		current, err := s.vitals.GetVitals(ctx, call.m.CampaignID, call.charText)
		if err != nil {
			return nil, s.dbError(ctx, "get vitals", err)
		}
		return connect.NewResponse(&playv1.AdjustCharacterVitalsResponse{Vitals: current}), nil
	}
	s.publishAdjusted(ctx, call.m.CampaignID, run)
	return connect.NewResponse(&playv1.AdjustCharacterVitalsResponse{Vitals: run.after, ConcentrationDc: run.check.dc, ConcentrationCastId: run.check.castID}), nil
}

// adjustWrite is the transaction of AdjustCharacterVitals: lock, retry, the change, the
// event, the beast form that ended, the combat that shows it, the concentration.
func (s *Service) adjustWrite(ctx context.Context, tx pgx.Tx, call adjustCall, run *adjustRun) error {
	q := s.queries.WithTx(tx)
	session, err := lockOpenSessionOrNone(ctx, q, call.m.CampaignID) // RN-02: during the session
	if err != nil {
		return err
	}
	done, again, err := eventByKey(ctx, q, session.ID, call.keyText, eventCharacterVitalsAdjusted, call.hash)
	if err != nil {
		return err
	}
	if again {
		if done.CharacterID == nil || *done.CharacterID != call.charText {
			return connect.NewError(connect.CodeInvalidArgument, errors.New("idempotency_key was already used for another change"))
		}
		run.repeated = true // a retry of a correction already made
		return nil
	}
	// Healing from 0 also resets the death saves of the character's combatant
	// in the session's combat (RN-03).
	before, adjusted, err := s.changeVitals(ctx, q, tx, session.ID, call.m.CampaignID, call.charText, call.msg)
	if err != nil {
		return err
	}
	payload, err := vitalsPayload(before, adjusted)
	if err != nil {
		return err
	}
	if err := s.appendKeyedEvent(ctx, q, keyedEvent{
		sessionID: session.ID, kind: eventCharacterVitalsAdjusted, userID: call.m.UserID, characterID: &call.charText, key: call.keyText, hash: call.hash,
	}, json.RawMessage(payload)); err != nil {
		return err
	}
	run.shapeChanged = (before.GetWildShape() == nil) != (adjusted.GetWildShape() == nil)
	run.visionMap = conv.OrZero(session.CurrentMapID)
	ownBody, adjusted, err := s.leaveBeastForm(ctx, tx, q, session.ID, call, before, adjusted)
	if err != nil {
		return err
	}
	// A combat in progress with this character shows its state ("Caído", the
	// healing) from the vitals: its revision goes up in the same transaction, so
	// every screen reads it again.
	if run.touched, err = s.touchCombatOf(ctx, q, session.ID, call.charText, ownBody); err != nil {
		return err
	}
	if run.touched != nil {
		run.visionMap = conv.OrZero(run.touched.MapID)
		if err := s.followCorrectionInCombat(ctx, tx, q, session, call, *run.touched, before, adjusted); err != nil {
			return err
		}
	}
	// Damage outside a combat asks the Constitution save of a spell the character
	// concentrates on (casting.go).
	if run.check, run.castsEnded, err = s.concentrationAfter(ctx, tx, q, session, call.m, before, adjusted); err != nil {
		return err
	}
	run.after = adjusted
	return nil
}

// leaveBeastForm is what the master taking a druid out of its beast form (the beast's hit
// points to 0) leaves: the character's own numbers are back on its combatant, and the history
// says so (MR-037). It returns that body (nil when the form did not end) and the vitals as
// they are then. The event has no combat, so it is nothing for an undo to take back.
func (s *Service) leaveBeastForm(ctx context.Context, tx pgx.Tx, q *playdb.Queries, sessionID string, call adjustCall, before, adjusted *playv1.CharacterVitals) (*link.Character, *playv1.CharacterVitals, error) {
	if before.GetWildShape() == nil || adjusted.GetWildShape() != nil || adjusted.GetHitPointsCurrent() <= 0 {
		return nil, adjusted, nil
	}
	adjusted, body, err := s.vitals.SetWildShape(ctx, tx, call.m.CampaignID, call.charText, "", 0)
	if err != nil {
		return nil, adjusted, err
	}
	seq, err := q.NextSessionEventSeq(ctx, sessionID)
	if err != nil {
		return nil, adjusted, fmt.Errorf("next event number: %w", err)
	}
	encID, round, actor, hidden := s.shapeLine(ctx, q, sessionID, call.charText)
	ended, err := json.Marshal(actionEvent{OwnerCharacter: call.charText, Beast: before.GetWildShape().GetBeastKey(), Reason: endedByMaster, EncounterID: encID, Round: round, Actor: actor, Secret: hidden})
	if err != nil {
		return nil, adjusted, fmt.Errorf("encode the event payload: %w", err)
	}
	var encounterID *string
	if encID != "" {
		encounterID = &encID
	}
	if _, err := insertSessionEvent(ctx, q, playdb.InsertSessionEventParams{
		GameSessionID: sessionID, Seq: seq, Kind: eventWildShapeEnded, ActorUserID: &call.m.UserID, CharacterID: &call.charText, Payload: ended, CreatedAt: s.now(), EncounterID: encounterID,
	}); err != nil {
		return nil, adjusted, fmt.Errorf("insert session event: %w", err)
	}
	return &body, adjusted, nil
}

// publishAdjusted tells the streams and the fog what the committed correction changed.
func (s *Service) publishAdjusted(ctx context.Context, campaignID string, run adjustRun) {
	s.hub.Publish(campaignID, live.Event{
		Audience: vitalsAudience(run.after),
		Message: &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_VitalsChanged_{
			VitalsChanged: &playv1.WatchGameSessionResponse_VitalsChanged{Vitals: run.after},
		}},
	})
	pctx, stop := afterCommit(ctx) // the change is committed: a caller that hangs up must not leave the streams and the fog unheard
	defer stop()
	if run.touched != nil {
		s.publishEncounterChanged(pctx, campaignID, *run.touched)
	}
	if run.shapeChanged { // the beast's senses went away: the fog hears of it (MR-036)
		s.maps.VisionChanged(pctx, campaignID, run.visionMap)
	}
	if run.check.castID != "" && run.castsEnded {
		s.publishCastsChanged(campaignID, run.check.secret)
	}
}

// touchCombatOf tells the session's open combat that a character's vitals changed:
// when the character is a player's combatant in it, the combat's revision goes up
// (the "Caído" state and the healing show from the vitals, so every screen has to
// read it again), after giving the combatant its own body back when ownBody is the
// character's own numbers (its beast form ended). It returns the encounter as
// touched, nil when the character is not in the combat; the caller publishes
// encounter_changed after the commit. It runs in the change's transaction.
func (s *Service) touchCombatOf(ctx context.Context, q *playdb.Queries, sessionID, characterID string, ownBody *link.Character) (*playdb.Encounter, error) {
	enc, err := q.GetOpenEncounter(ctx, sessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find the open encounter: %w", err)
	}
	cs, err := q.ListCombatants(ctx, enc.ID)
	if err != nil {
		return nil, fmt.Errorf("list the combatants: %w", err)
	}
	i := slices.IndexFunc(cs, func(c playdb.Combatant) bool { return c.Kind == kindPlayer && c.CharacterID == characterID })
	if i < 0 {
		return nil, nil
	}
	if ownBody != nil {
		if err := applyBody(ctx, &combatTx{q: q}, cs[i], *ownBody); err != nil {
			return nil, err
		}
	}
	touched, err := q.TouchEncounter(ctx, enc.ID)
	if err != nil {
		return nil, fmt.Errorf("touch the encounter: %w", err)
	}
	return &touched, nil
}

// endConcentrationOfDownCharacter ends the concentration of a character the master set to 0 hit
// points while the character is a combatant of the open combat: it is unconscious and stops
// concentrating with no save (SRD 5.1, "Duration"), the same end a damage to 0 has. The end is
// the damage's own (stopConcentrating), so the spell's effects, its creatures and the log line
// follow; no pending damage is behind it.
func (s *Service) endConcentrationOfDownCharacter(ctx context.Context, tx pgx.Tx, q *playdb.Queries, session playdb.GameSession, call adjustCall, enc playdb.Encounter, adjusted *playv1.CharacterVitals) error {
	if adjusted.GetHitPointsCurrent() > 0 {
		return nil
	}
	cs, err := q.ListCombatants(ctx, enc.ID)
	if err != nil {
		return fmt.Errorf("list the combatants: %w", err)
	}
	i := slices.IndexFunc(cs, func(c playdb.Combatant) bool {
		return c.Kind == kindPlayer && c.CharacterID == call.charText && c.ConcentrationSpell != nil
	})
	if i < 0 {
		return nil
	}
	c, err := s.openTx(ctx, combatTx{tx: tx, q: q, session: session, enc: enc, now: s.now(), actorUserID: call.m.UserID, svc: s, master: true})
	if err != nil {
		return err
	}
	return s.endConcentrationAtZero(ctx, c, cs[i], "")
}

// followCorrectionInCombat is what a correction leaves in the combat the character is in: the
// concentration of one set to 0 hit points, and the states that ended.
func (s *Service) followCorrectionInCombat(ctx context.Context, tx pgx.Tx, q *playdb.Queries, session playdb.GameSession, call adjustCall, enc playdb.Encounter, before, adjusted *playv1.CharacterVitals) error {
	if err := s.endConcentrationOfDownCharacter(ctx, tx, q, session, call, enc, adjusted); err != nil {
		return err
	}
	return s.followCorrectionStates(ctx, tx, q, session, call, enc, before, adjusted)
}

// followCorrectionStates is what a correction that ended states leaves in the combat of the
// character: the combatant is refreshed and each state has its line, with the master's
// reason for the ones he named and the incapacitated one for the rest (the hit points at 0).
func (s *Service) followCorrectionStates(ctx context.Context, tx pgx.Tx, q *playdb.Queries, session playdb.GameSession, call adjustCall, enc playdb.Encounter, before, adjusted *playv1.CharacterVitals) error {
	ended := endedBetween(before, adjusted)
	if len(ended) == 0 {
		return nil
	}
	c, err := s.openTx(ctx, combatTx{tx: tx, q: q, session: session, enc: enc, now: s.now(), actorUserID: call.m.UserID, svc: s, master: true})
	if err != nil {
		return err
	}
	return s.followEndedStates(ctx, c, call.charText, ended, func(key string) string {
		if slices.Contains(call.msg.GetEndStates(), key) {
			return stateEndMaster
		}
		return stateEndIncapacitated
	})
}

// vitalsNumbers are the numbers a session event keeps about a character's
// vitals: no names, no free text (docs/privacy.md).
type vitalsNumbers struct {
	HitPointsCurrent   int32   `json:"hit_points_current"`
	HitPointsTemporary int32   `json:"hit_points_temporary"`
	SpellSlotsUsed     []int32 `json:"spell_slots_used,omitempty"` // index 0 is spell level 1
	PactSlotsUsed      int32   `json:"pact_slots_used,omitempty"`
	HitDiceUsed        int32   `json:"hit_dice_used"`
	// ResourcesUsed are the uses spent of the class and race resources, by
	// resource key.
	ResourcesUsed map[string]int32 `json:"resources_used,omitempty"`
	// The beast of a Wild Shape form and its hit points (MR-037), when there is one.
	WildShapeBeast string `json:"wild_shape_beast,omitempty"`
	WildShapeHP    int32  `json:"wild_shape_hp,omitempty"`
}

func numbersOf(v *playv1.CharacterVitals) vitalsNumbers {
	n := vitalsNumbers{
		HitPointsCurrent:   v.GetHitPointsCurrent(),
		HitPointsTemporary: v.GetHitPointsTemporary(),
		PactSlotsUsed:      v.GetPactSlots().GetUsed(),
		HitDiceUsed:        v.GetHitDiceUsed(),
		WildShapeBeast:     v.GetWildShape().GetBeastKey(),
		WildShapeHP:        v.GetWildShape().GetHitPointsCurrent(),
	}
	for _, r := range v.GetResources() {
		if r.GetUsed() > 0 {
			if n.ResourcesUsed == nil {
				n.ResourcesUsed = map[string]int32{}
			}
			n.ResourcesUsed[r.GetKey()] = r.GetUsed()
		}
	}
	for _, slot := range v.GetSpellSlots() {
		level := int(slot.GetLevel())
		if level < 1 || level > 9 {
			continue // never: the characters module sends levels 1 to 9
		}
		for len(n.SpellSlotsUsed) < level {
			n.SpellSlotsUsed = append(n.SpellSlotsUsed, 0)
		}
		n.SpellSlotsUsed[level-1] = slot.GetUsed()
	}
	return n
}

// vitalsPayload is a character_vitals_adjusted event's payload: the numbers
// before and after, so the history can tell what the master changed and,
// later, undo it.
func vitalsPayload(before, after *playv1.CharacterVitals) ([]byte, error) {
	b, err := json.Marshal(struct {
		Before vitalsNumbers `json:"before"`
		After  vitalsNumbers `json:"after"`
	}{numbersOf(before), numbersOf(after)})
	if err != nil {
		return nil, fmt.Errorf("encode the event payload: %w", err)
	}
	return b, nil
}

// PublishVitalsChanged sends a character's vitals as they are now, the way the
// master's correction does: to the master and to the player who plays the
// character, nobody else (RN-10). The characters module calls it after a
// level-up, which changes the maximum and the current hit points. A character
// that is not a living player character of the campaign, or vitals that cannot
// be read, send nothing. Call it after the commit.
func (s *Service) PublishVitalsChanged(ctx context.Context, campaignID, characterID string) {
	pctx, stop := afterCommit(ctx)
	defer stop()
	v, err := s.vitals.GetVitals(pctx, campaignID, characterID)
	if err != nil {
		return
	}
	s.hub.Publish(campaignID, live.Event{
		Audience: vitalsAudience(v),
		Message: &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_VitalsChanged_{
			VitalsChanged: &playv1.WatchGameSessionResponse_VitalsChanged{Vitals: v},
		}},
	})
}
