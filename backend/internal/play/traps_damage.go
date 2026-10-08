package play

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// The damage a fired trap did to a player's character, waiting for the master
// (MR-035, Etapa 9, D5; RN-02). In a combat it is a pending_damages row, which the
// master applies with CombatService.ApplyPendingDamage like any other; outside a
// combat it is a trap_damages row, applied here to the character's vitals. The
// master may change the amount before applying: resistances are not modeled.

// ListTrapDamages implements playv1connect.PlayServiceHandler.
func (s *Service) ListTrapDamages(
	ctx context.Context,
	req *connect.Request[playv1.ListTrapDamagesRequest],
) (*connect.Response[playv1.ListTrapDamagesResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	// What waits in the open combat (a combat that ended turned it into the rows below),
	// and what waits outside a combat, from any session of the campaign.
	var inCombat []playdb.PendingDamage
	if session, err := s.queries.GetOpenGameSession(ctx, m.CampaignID); err == nil {
		if inCombat, err = s.queries.ListOpenTrapPendingDamages(ctx, session.ID); err != nil {
			return nil, s.dbError(ctx, "list the trap damages in combat", err)
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, s.dbError(ctx, "find the open session", err)
	}
	outside, err := s.queries.ListCampaignTrapDamages(ctx, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "list the trap damages", err)
	}
	var pointIDs, characterIDs []string
	combatants := map[string]playdb.Combatant{}
	for _, p := range inCombat {
		pointIDs = append(pointIDs, deref(p.TrapPointID))
		if _, ok := combatants[p.TargetID]; !ok {
			cs, err := s.queries.ListCombatantsWithDismissed(ctx, p.EncounterID)
			if err != nil {
				return nil, s.dbError(ctx, "list the combatants", err)
			}
			for _, c := range cs {
				combatants[c.ID] = c
			}
		}
		characterIDs = append(characterIDs, combatants[p.TargetID].CharacterID)
	}
	for _, d := range outside {
		pointIDs = append(pointIDs, d.TrapPointID)
		characterIDs = append(characterIDs, d.CharacterID)
	}
	trapNames, names := map[string]string{}, map[string]string{}
	if s.traps != nil && len(pointIDs) > 0 {
		if trapNames, err = s.traps.TrapNames(ctx, m.CampaignID, slices.Compact(slices.Sorted(slices.Values(pointIDs)))); err != nil {
			return nil, s.dbError(ctx, "read the traps' names", err)
		}
	}
	if len(characterIDs) > 0 {
		chars, err := s.roster.SessionCharacters(ctx, nil, m.CampaignID, slices.Compact(slices.Sorted(slices.Values(characterIDs))))
		if err != nil {
			return nil, s.dbError(ctx, "read the characters' names", err)
		}
		for _, c := range chars {
			names[c.ID] = c.Name
		}
	}
	res := &playv1.ListTrapDamagesResponse{}
	for _, p := range inCombat {
		who := combatants[p.TargetID]
		res.Damages = append(res.Damages, &playv1.TrapDamage{
			Id: p.ID, EncounterId: p.EncounterID, TrapPointId: deref(p.TrapPointID), TrapName: trapNames[deref(p.TrapPointID)],
			CharacterId: who.CharacterID, CharacterName: names[who.CharacterID], CombatantId: p.TargetID,
			Status: pendingStatusToProto[p.Status], Roll: diceRoll(p.DiceCount, p.DiceSides, p.Faces, p.DiceBonus+p.CriticalMax, num(p.RollTotal), false),
			Amount: num(p.Amount), DamageTypeKey: p.DamageType, DamageTypePt: damageTypePT[p.DamageType], Half: p.Half, Critical: p.Critical,
			AppliedAmount: p.AppliedAmount, CreatedAt: timestamppb.New(p.CreatedAt),
		})
	}
	for _, d := range outside {
		out := trapDamageProto(trapDamageOf(d), trapNames[d.TrapPointID], names[d.CharacterID])
		out.SessionNumber, out.SessionStartedAt = d.SessionNumber, timestamppb.New(d.SessionStartedAt)
		res.Damages = append(res.Damages, out)
	}
	return connect.NewResponse(res), nil
}

// trapDamageOf is the row of a listing as the trap_damages row it is.
func trapDamageOf(d playdb.ListCampaignTrapDamagesRow) playdb.TrapDamage {
	return playdb.TrapDamage{
		ID: d.ID, GameSessionID: d.GameSessionID, TrapPointID: d.TrapPointID, FireID: d.FireID, CharacterID: d.CharacterID, Status: d.Status,
		Critical: d.Critical, DiceCount: d.DiceCount, DiceSides: d.DiceSides, DiceBonus: d.DiceBonus, CriticalMax: d.CriticalMax, DamageType: d.DamageType, Faces: d.Faces,
		RollTotal: d.RollTotal, Half: d.Half, Amount: d.Amount, AppliedAmount: d.AppliedAmount, CreatedAt: d.CreatedAt, ResolvedAt: d.ResolvedAt,
	}
}

// trapDamageProto is a trap_damages row as the API says it.
func trapDamageProto(d playdb.TrapDamage, trapName, characterName string) *playv1.TrapDamage {
	status := map[string]playv1.PendingDamageStatus{
		"rolled": playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_ROLLED, "applied": playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED,
		"discarded": playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_DISCARDED,
	}[d.Status]
	return &playv1.TrapDamage{
		Id: d.ID, TrapPointId: d.TrapPointID, TrapName: trapName, CharacterId: d.CharacterID, CharacterName: characterName, Status: status,
		Roll: diceRoll(d.DiceCount, d.DiceSides, d.Faces, d.DiceBonus+d.CriticalMax, d.RollTotal, false), Amount: d.Amount,
		DamageTypeKey: d.DamageType, DamageTypePt: damageTypePT[d.DamageType], Half: d.Half, Critical: d.Critical,
		AppliedAmount: d.AppliedAmount, CreatedAt: timestamppb.New(d.CreatedAt),
	}
}

// settleTrapDamage applies or discards a trap damage outside a combat. apply says
// which; override is the master's amount, nil to apply the rolled one.
func (s *Service) settleTrapDamage(ctx context.Context, m authz.Membership, key, rawID string, apply bool, override *int32) (playdb.TrapDamage, *playv1.CharacterVitals, error) {
	id, err := uuid.Parse(rawID)
	if err != nil {
		return playdb.TrapDamage{}, nil, connect.NewError(connect.CodeNotFound, errors.New("trap damage not found"))
	}
	kind := eventDamageDiscarded
	if apply {
		kind = eventDamageApplied
	}
	// The key is kept on the damage it settled, scoped to the campaign, with a hash of what it
	// asked: which damage, which action and which amount. The damage outlives its session, so
	// the key cannot rest on a session event alone.
	amountText := "-"
	if override != nil {
		amountText = strconv.Itoa(int(*override))
	}
	scopedKey, hash := idem.Scope(m.CampaignID, key), requestHash(id.String(), kind, amountText)
	var row playdb.TrapDamage
	var after *playv1.CharacterVitals
	var repeated, shapeChanged bool
	var touched *playdb.Encounter
	var visionMap string
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		after, repeated, shapeChanged, touched, visionMap = nil, false, false, nil, ""
		// The damage outlives its session, so it may be settled with none open (then there
		// is no event to write; the damage keeps the key).
		session, err := q.GetOpenGameSessionForUpdate(ctx, m.CampaignID)
		hasSession := err == nil
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("lock the open session: %w", err)
		}
		found, err := q.GetCampaignTrapDamageForUpdate(ctx, playdb.GetCampaignTrapDamageForUpdateParams{CampaignID: m.CampaignID, ID: id.String()})
		if errors.Is(err, pgx.ErrNoRows) {
			return connect.NewError(connect.CodeNotFound, errors.New("trap damage not found"))
		}
		if err != nil {
			return fmt.Errorf("find the trap damage: %w", err)
		}
		row = trapDamageOf(playdb.ListCampaignTrapDamagesRow(found))
		prior, err := q.GetTrapDamageBySettleKey(ctx, scopedKey)
		switch {
		case err == nil:
			if prior.ID != row.ID {
				return idem.ErrReused()
			}
			if err := idem.SameRequest(prior.SettleHash, hash); err != nil {
				return err
			}
			row, repeated = prior, true // a retry: the damage is as the first call left it
			return nil
		case !errors.Is(err, pgx.ErrNoRows):
			return fmt.Errorf("find the damage of this idempotency key: %w", err)
		}
		if hasSession {
			done, err := q.GetSessionEventByIdempotencyKey(ctx, playdb.GetSessionEventByIdempotencyKeyParams{GameSessionID: session.ID, IdempotencyKey: &key})
			switch {
			case err == nil:
				var prior actionEvent
				if done.Kind != kind || json.Unmarshal(done.Payload, &prior) != nil || prior.Pending != row.ID {
					return connect.NewError(connect.CodeInvalidArgument, errors.New("idempotency_key was already used for another change"))
				}
				repeated = true // a retry: the damage is as the first call left it
				return nil
			case !errors.Is(err, pgx.ErrNoRows):
				return fmt.Errorf("find the event of this idempotency key: %w", err)
			}
		}
		if row.Status != "rolled" {
			return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_DAMAGE_RESOLVED, "the damage was applied or discarded already")
		}
		c, err := s.openTx(ctx, combatTx{tx: tx, q: q, session: session, now: s.now(), characterID: &row.CharacterID, actorUserID: m.UserID})
		if err != nil {
			return err
		}
		ev := actionEvent{Pending: row.ID, Target: row.CharacterID, Key: "trap", Amount: row.Amount, DamageType: row.DamageType}
		final, applied := "discarded", (*int32)(nil)
		if apply {
			final = "applied"
			amount := row.Amount
			if override != nil && *override != row.Amount {
				amount, applied = *override, override
			}
			ev.Amount, ev.Overridden, ev.Rolled = amount, applied != nil, row.Amount
			// RN-02: the damage goes through the character's vitals, temporary hit
			// points first, never below 0; a druid in a beast form takes it on the
			// beast, and what is left goes to the druid when the beast falls (MR-037).
			// What changes is the open session's, even when the trap fired in an earlier one.
			now, err := s.vitals.GetVitalsTx(ctx, tx, m.CampaignID, row.CharacterID)
			if err != nil {
				return err
			}
			sessionID := row.GameSessionID
			if hasSession {
				sessionID = session.ID
			}
			var req *playv1.AdjustCharacterVitalsRequest
			var left, carried int32
			if now.GetWildShape() != nil {
				req, left, carried = beastDamageRequest(now, amount)
			} else {
				dmg := combat.ApplyDamage(int(now.GetHitPointsCurrent()), int(now.GetHitPointsTemporary()), int(amount))
				hp, temp := clamp32(dmg.HP, 0, maxHitPointChange), clamp32(dmg.TempHP, 0, maxHitPointChange)
				req = &playv1.AdjustCharacterVitalsRequest{HitPointsCurrent: &hp, HitPointsTemporary: &temp}
			}
			before, vit, err := s.changeVitals(ctx, q, tx, sessionID, m.CampaignID, row.CharacterID, req)
			if err != nil {
				return err
			}
			if now.GetWildShape() != nil && left == 0 { // the beast fell: the keeper ended the form, its numbers and its event are ours
				if vit, err = s.formEnds(ctx, q, tx, sessionID, m.CampaignID, row.CharacterID, now.GetWildShape().GetBeastKey(), endedByDamage, carried); err != nil {
					return err
				}
			}
			after = vit
			shapeChanged = (before.GetWildShape() == nil) != (vit.GetWildShape() == nil)
			ev.Carried = carried
			ev.Before, ev.After = new(hpStateOf(before)), new(hpStateOf(vit))
			if hasSession {
				// A combat in progress with this character shows its state from the vitals.
				if touched, err = s.touchCombatOf(ctx, q, session.ID, row.CharacterID, nil); err != nil {
					return err
				}
				visionMap = deref(session.CurrentMapID)
				if touched != nil {
					visionMap = deref(touched.MapID)
				}
			}
		}
		resolved := c.now
		if row, err = q.SetTrapDamageStatus(ctx, playdb.SetTrapDamageStatusParams{ID: row.ID, Status: final, ResolvedAt: &resolved, AppliedAmount: applied, SettleKey: scopedKey, SettleHash: hash}); err != nil {
			return fmt.Errorf("settle the trap damage: %w", err)
		}
		if !hasSession {
			return nil
		}
		_, err = insertSceneEvent(ctx, c, kind, &m.UserID, &key, ev)
		return err
	})
	if err != nil {
		return playdb.TrapDamage{}, nil, err
	}
	if !repeated {
		s.publishVitals(m.CampaignID, after)
		s.Publish(m.CampaignID, false, mapChangedHint("")) // the trap card has one damage less
		if touched != nil {
			s.publishEncounterChanged(ctx, m.CampaignID, *touched)
		}
		if shapeChanged { // the beast's senses went away: the fog hears of it (MR-036)
			s.maps.VisionChanged(ctx, m.CampaignID, visionMap)
		}
	}
	return row, after, nil
}

// trapDamageNames reads the names a trap damage's answer shows.
func (s *Service) trapDamageNames(ctx context.Context, campaignID string, d playdb.TrapDamage) (trapName, characterName string) {
	if s.traps != nil {
		if names, err := s.traps.TrapNames(ctx, campaignID, []string{d.TrapPointID}); err == nil {
			trapName = names[d.TrapPointID]
		}
	}
	if chars, err := s.roster.SessionCharacters(ctx, nil, campaignID, []string{d.CharacterID}); err == nil && len(chars) == 1 {
		characterName = chars[0].Name
	}
	return trapName, characterName
}

// ApplyTrapDamage implements playv1connect.PlayServiceHandler.
func (s *Service) ApplyTrapDamage(
	ctx context.Context,
	req *connect.Request[playv1.ApplyTrapDamageRequest],
) (*connect.Response[playv1.ApplyTrapDamageResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	override := req.Msg.Amount
	if override != nil && (*override < 0 || *override > maxHitPointChange) {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("amount must be 0 to %d", maxHitPointChange))
	}
	row, _, err := s.settleTrapDamage(ctx, m, key, req.Msg.GetTrapDamageId(), true, override)
	if err != nil {
		return nil, s.dbError(ctx, "apply a trap damage", err)
	}
	trapName, name := s.trapDamageNames(ctx, m.CampaignID, row)
	return connect.NewResponse(&playv1.ApplyTrapDamageResponse{Damage: trapDamageProto(row, trapName, name)}), nil
}

// DiscardTrapDamage implements playv1connect.PlayServiceHandler.
func (s *Service) DiscardTrapDamage(
	ctx context.Context,
	req *connect.Request[playv1.DiscardTrapDamageRequest],
) (*connect.Response[playv1.DiscardTrapDamageResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	row, _, err := s.settleTrapDamage(ctx, m, key, req.Msg.GetTrapDamageId(), false, nil)
	if err != nil {
		return nil, s.dbError(ctx, "discard a trap damage", err)
	}
	trapName, name := s.trapDamageNames(ctx, m.CampaignID, row)
	return connect.NewResponse(&playv1.DiscardTrapDamageResponse{Damage: trapDamageProto(row, trapName, name)}), nil
}

// keepTrapDamage turns the trap damage a combat still holds for the master into
// trap_damages rows when the combat ends, inside the change's transaction: the damage
// to a player's character always waits for the master (question 73, RN-02), and a
// combat that ended has no way to apply it. The rows are the combat's, now outside it.
func (s *Service) keepTrapDamage(ctx context.Context, c *combatTx, cs []playdb.Combatant) error {
	open, err := c.q.ListOpenTrapPendingDamagesOfEncounter(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the trap damage that waits: %w", err)
	}
	for _, p := range open {
		i := slices.IndexFunc(cs, func(o playdb.Combatant) bool { return o.ID == p.TargetID })
		if i < 0 {
			continue
		}
		if _, err := c.q.InsertTrapDamage(ctx, playdb.InsertTrapDamageParams{
			GameSessionID: c.session.ID, TrapPointID: deref(p.TrapPointID), FireID: p.ID, CharacterID: cs[i].CharacterID, Critical: p.Critical,
			DiceCount: p.DiceCount, DiceSides: p.DiceSides, DiceBonus: p.DiceBonus, CriticalMax: p.CriticalMax, DamageType: p.DamageType, Faces: p.Faces,
			RollTotal: num(p.RollTotal), Half: p.Half, Amount: num(p.Amount), CreatedAt: p.CreatedAt,
		}); err != nil {
			return fmt.Errorf("keep the trap damage: %w", err)
		}
		if _, err := c.q.SetPendingDamageStatus(ctx, playdb.SetPendingDamageStatusParams{ID: p.ID, Status: pendingDiscarded, ResolvedAt: &c.now}); err != nil {
			return fmt.Errorf("close the pending damage: %w", err)
		}
	}
	return nil
}
