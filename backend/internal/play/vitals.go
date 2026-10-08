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
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// The kinds of session_events rows (session_event_kinds).
const eventCharacterVitalsAdjusted = "character_vitals_adjusted"

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
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	characterID, err := uuid.Parse(req.Msg.GetCharacterId())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("character not found"))
	}
	key, err := uuid.Parse(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("idempotency_key must be a UUID"))
	}
	keyText, charText := key.String(), characterID.String()
	hash := idem.Hash(req.Msg)

	var after *playv1.CharacterVitals
	var repeated bool
	var touched *playdb.Encounter
	visionMap, shapeChanged := "", false // the map whose fog the change touches
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		after, repeated, touched, visionMap, shapeChanged = nil, false, nil, "", false // a retry starts over
		q := s.queries.WithTx(tx)
		session, err := q.GetOpenGameSessionForUpdate(ctx, m.CampaignID)
		if errors.Is(err, pgx.ErrNoRows) {
			return errNoOpenSession() // RN-02: during the session
		}
		if err != nil {
			return fmt.Errorf("lock the open session: %w", err)
		}

		done, err := q.GetSessionEventByIdempotencyKey(ctx, playdb.GetSessionEventByIdempotencyKeyParams{
			GameSessionID: session.ID, IdempotencyKey: &keyText,
		})
		switch {
		case err == nil:
			if done.Kind != eventCharacterVitalsAdjusted || done.CharacterID == nil || *done.CharacterID != charText || hashDiffers(done.IdempotencyHash, hash) {
				return connect.NewError(connect.CodeInvalidArgument, errors.New("idempotency_key was already used for another change"))
			}
			repeated = true // a retry of a correction already made
			return nil
		case !errors.Is(err, pgx.ErrNoRows):
			return fmt.Errorf("find the event of this idempotency key: %w", err)
		}

		// Healing from 0 also resets the death saves of the character's combatant
		// in the session's combat (RN-03).
		before, adjusted, err := s.changeVitals(ctx, q, tx, session.ID, m.CampaignID, charText, req.Msg)
		if err != nil {
			return err
		}
		payload, err := vitalsPayload(before, adjusted)
		if err != nil {
			return err
		}
		seq, err := q.NextSessionEventSeq(ctx, session.ID)
		if err != nil {
			return fmt.Errorf("next event number: %w", err)
		}
		if _, err := q.InsertSessionEvent(ctx, playdb.InsertSessionEventParams{
			GameSessionID:   session.ID,
			Seq:             seq,
			Kind:            eventCharacterVitalsAdjusted,
			ActorUserID:     &m.UserID,
			CharacterID:     &charText,
			Payload:         payload,
			IdempotencyKey:  &keyText,
			IdempotencyHash: hash,
			CreatedAt:       s.now(),
		}); err != nil {
			return fmt.Errorf("insert session event: %w", err)
		}
		// The master took a druid out of its beast form (the beast's hit points to 0):
		// the character's own numbers are back on its combatant, and the history says
		// so (MR-037). The event has no combat, so it is nothing for an undo to take back.
		var ownBody *link.Character
		shapeChanged = (before.GetWildShape() == nil) != (adjusted.GetWildShape() == nil)
		visionMap = deref(session.CurrentMapID)
		if before.GetWildShape() != nil && adjusted.GetWildShape() == nil && adjusted.GetHitPointsCurrent() > 0 {
			var body link.Character
			if adjusted, body, err = s.vitals.SetWildShape(ctx, tx, m.CampaignID, charText, "", 0); err != nil {
				return err
			}
			ownBody = &body
			seq, err := q.NextSessionEventSeq(ctx, session.ID)
			if err != nil {
				return fmt.Errorf("next event number: %w", err)
			}
			encID, round, actor, hidden := s.shapeLine(ctx, q, session.ID, charText)
			ended, err := json.Marshal(actionEvent{OwnerCharacter: charText, Beast: before.GetWildShape().GetBeastKey(), Reason: endedByMaster, EncounterID: encID, Round: round, Actor: actor, Secret: hidden})
			if err != nil {
				return fmt.Errorf("encode the event payload: %w", err)
			}
			var encounterID *string
			if encID != "" {
				encounterID = &encID
			}
			if _, err := q.InsertSessionEvent(ctx, playdb.InsertSessionEventParams{
				GameSessionID: session.ID, Seq: seq, Kind: eventWildShapeEnded, ActorUserID: &m.UserID, CharacterID: &charText, Payload: ended, CreatedAt: s.now(), EncounterID: encounterID,
			}); err != nil {
				return fmt.Errorf("insert session event: %w", err)
			}
		}
		// A combat in progress with this character shows its state ("Caído", the
		// healing) from the vitals: its revision goes up in the same transaction, so
		// every screen reads it again.
		if touched, err = s.touchCombatOf(ctx, q, session.ID, charText, ownBody); err != nil {
			return err
		}
		if touched != nil {
			visionMap = deref(touched.MapID)
		}
		after = adjusted
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "adjust a character's vitals", err)
	}
	if repeated {
		// Nothing changed and nothing goes out: answer with the vitals as
		// they are now.
		current, err := s.vitals.GetVitals(ctx, m.CampaignID, charText)
		if err != nil {
			return nil, s.dbError(ctx, "get vitals", err)
		}
		return connect.NewResponse(&playv1.AdjustCharacterVitalsResponse{Vitals: current}), nil
	}
	s.hub.Publish(m.CampaignID, live.Event{
		Audience: vitalsAudience(after),
		Message: &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_VitalsChanged_{
			VitalsChanged: &playv1.WatchGameSessionResponse_VitalsChanged{Vitals: after},
		}},
	})
	if touched != nil {
		s.publishEncounterChanged(ctx, m.CampaignID, *touched)
	}
	if shapeChanged { // the beast's senses went away: the fog hears of it (MR-036)
		s.maps.VisionChanged(ctx, m.CampaignID, visionMap)
	}
	return connect.NewResponse(&playv1.AdjustCharacterVitalsResponse{Vitals: after}), nil
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

// vitalsNumbers are the numbers a session event keeps about a character's
// vitals: no names, no free text (docs/privacy.md).
type vitalsNumbers struct {
	HitPointsCurrent   int32   `json:"hit_points_current"`
	HitPointsTemporary int32   `json:"hit_points_temporary"`
	SpellSlotsUsed     []int32 `json:"spell_slots_used,omitempty"` // index 0 is spell level 1
	PactSlotsUsed      int32   `json:"pact_slots_used,omitempty"`
	HitDiceUsed        int32   `json:"hit_dice_used"`
	// ResourcesUsed are the uses spent of the class and race resources, by
	// resource key (Etapa 6).
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
