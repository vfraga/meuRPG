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
	waiting, err := s.readWaitingTrapDamages(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}
	trapNames, names, err := s.waitingTrapDamageNames(ctx, m.CampaignID, waiting)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.ListTrapDamagesResponse{Damages: waiting.protos(trapNames, names)}), nil
}

// waitingTrapDamages is what waits for the master: the pending damages of the open combat and
// the trap damages from outside a combat, with the combatants of the first.
type waitingTrapDamages struct {
	inCombat   []playdb.PendingDamage
	outside    []playdb.ListCampaignTrapDamagesRow
	combatants map[string]playdb.Combatant
}

// readWaitingTrapDamages reads what waits in the open combat (a combat that ended turned it into
// the rows outside), and what waits outside a combat, from any session of the campaign.
func (s *Service) readWaitingTrapDamages(ctx context.Context, campaignID string) (*waitingTrapDamages, error) {
	inCombat, err := s.openCombatTrapDamages(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	outside, err := s.queries.ListCampaignTrapDamages(ctx, campaignID)
	if err != nil {
		return nil, s.dbError(ctx, "list the trap damages", err)
	}
	w := &waitingTrapDamages{inCombat: inCombat, outside: outside, combatants: map[string]playdb.Combatant{}}
	for _, p := range inCombat {
		if _, ok := w.combatants[p.TargetID]; ok {
			continue
		}
		cs, err := s.queries.ListCombatantsWithDismissed(ctx, p.EncounterID)
		if err != nil {
			return nil, s.dbError(ctx, "list the combatants", err)
		}
		for _, c := range cs {
			w.combatants[c.ID] = c
		}
	}
	return w, nil
}

// openCombatTrapDamages are the pending trap damages of the campaign's open session; none when
// no session is open.
func (s *Service) openCombatTrapDamages(ctx context.Context, campaignID string) ([]playdb.PendingDamage, error) {
	session, err := s.queries.GetOpenGameSession(ctx, campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, s.dbError(ctx, "find the open session", err)
	}
	rows, err := s.queries.ListOpenTrapPendingDamages(ctx, session.ID)
	if err != nil {
		return nil, s.dbError(ctx, "list the trap damages in combat", err)
	}
	return rows, nil
}

// ids are the traps and the characters the waiting damages name, in the order of the damages.
func (w *waitingTrapDamages) ids() (pointIDs, characterIDs []string) {
	for _, p := range w.inCombat {
		pointIDs = append(pointIDs, deref(p.TrapPointID))
		characterIDs = append(characterIDs, w.combatants[p.TargetID].CharacterID)
	}
	for _, d := range w.outside {
		pointIDs = append(pointIDs, d.TrapPointID)
		characterIDs = append(characterIDs, d.CharacterID)
	}
	return pointIDs, characterIDs
}

// waitingTrapDamageNames reads the names of the traps and of the characters the damages name.
func (s *Service) waitingTrapDamageNames(ctx context.Context, campaignID string, w *waitingTrapDamages) (trapNames, names map[string]string, err error) {
	pointIDs, characterIDs := w.ids()
	trapNames, names = map[string]string{}, map[string]string{}
	if s.traps != nil && len(pointIDs) > 0 {
		if trapNames, err = s.traps.TrapNames(ctx, campaignID, slices.Compact(slices.Sorted(slices.Values(pointIDs)))); err != nil {
			return nil, nil, s.dbError(ctx, "read the traps' names", err)
		}
	}
	if len(characterIDs) > 0 {
		chars, err := s.roster.SessionCharacters(ctx, nil, campaignID, slices.Compact(slices.Sorted(slices.Values(characterIDs))))
		if err != nil {
			return nil, nil, s.dbError(ctx, "read the characters' names", err)
		}
		for _, c := range chars {
			names[c.ID] = c.Name
		}
	}
	return trapNames, names, nil
}

// protos are the waiting damages as the API says them: the combat's first, then the ones from outside.
func (w *waitingTrapDamages) protos(trapNames, names map[string]string) []*playv1.TrapDamage {
	var out []*playv1.TrapDamage
	for _, p := range w.inCombat {
		who := w.combatants[p.TargetID]
		out = append(out, &playv1.TrapDamage{
			Id: p.ID, EncounterId: p.EncounterID, TrapPointId: deref(p.TrapPointID), TrapName: trapNames[deref(p.TrapPointID)],
			CharacterId: who.CharacterID, CharacterName: names[who.CharacterID], CombatantId: p.TargetID,
			Status: pendingStatusToProto[p.Status], Roll: diceRoll(p.DiceCount, p.DiceSides, p.Faces, p.DiceBonus+p.CriticalMax, num(p.RollTotal), false),
			Amount: num(p.Amount), DamageTypeKey: p.DamageType, DamageTypePt: damageTypePT[p.DamageType], Half: p.Half, Critical: p.Critical,
			AppliedAmount: p.AppliedAmount, CreatedAt: timestamppb.New(p.CreatedAt),
		})
	}
	for _, d := range w.outside {
		dmg := trapDamageProto(trapDamageOf(d), trapNames[d.TrapPointID], names[d.CharacterID])
		dmg.SessionNumber, dmg.SessionStartedAt = d.SessionNumber, timestamppb.New(d.SessionStartedAt)
		out = append(out, dmg)
	}
	return out
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
	st := newTrapSettle(m, key, id.String(), apply, override)
	if err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error { return s.settleTrapDamageTx(ctx, tx, st) }); err != nil {
		return playdb.TrapDamage{}, nil, err
	}
	if !st.repeated {
		s.afterTrapSettled(ctx, st)
	}
	return st.row, st.after, nil
}

// trapSettle is one settling of a trap damage: what the master asked, and what the transaction
// left for the streams once it is committed.
type trapSettle struct {
	m        authz.Membership
	key      string
	id       string
	apply    bool
	override *int32
	kind     string
	// scopedKey and hash stand for the request: the key of the campaign and a hash of what it
	// asked, which damage, which action and which amount.
	scopedKey *string
	hash      *string

	row          playdb.TrapDamage
	after        *playv1.CharacterVitals
	repeated     bool
	shapeChanged bool
	touched      *playdb.Encounter
	visionMap    string
}

// newTrapSettle describes the request. The key is kept on the damage it settled, scoped to the
// campaign: the damage outlives its session, so the key cannot rest on a session event alone.
func newTrapSettle(m authz.Membership, key, id string, apply bool, override *int32) *trapSettle {
	kind := eventDamageDiscarded
	if apply {
		kind = eventDamageApplied
	}
	amountText := "-"
	if override != nil {
		amountText = strconv.Itoa(int(*override))
	}
	return &trapSettle{
		m: m, key: key, id: id, apply: apply, override: override, kind: kind,
		scopedKey: idem.Scope(m.CampaignID, key), hash: requestHash(id, kind, amountText),
	}
}

// reset forgets what an attempt of the transaction left, for the next one.
func (st *trapSettle) reset() {
	st.after, st.repeated, st.shapeChanged, st.touched, st.visionMap = nil, false, false, nil, ""
}

// settleTrapDamageTx is the settling inside its transaction: the damage is found, a retry answers
// with the damage as the first call left it, and a new settling changes the vitals, closes the
// damage and writes the event.
func (s *Service) settleTrapDamageTx(ctx context.Context, tx pgx.Tx, st *trapSettle) error {
	q := s.queries.WithTx(tx)
	st.reset()
	// The damage outlives its session, so it may be settled with none open (then there
	// is no event to write; the damage keeps the key).
	session, hasSession, err := lockTrapSettleSession(ctx, q, st.m.CampaignID)
	if err != nil {
		return err
	}
	found, err := q.GetCampaignTrapDamageForUpdate(ctx, playdb.GetCampaignTrapDamageForUpdateParams{CampaignID: st.m.CampaignID, ID: st.id})
	if errors.Is(err, pgx.ErrNoRows) {
		return connect.NewError(connect.CodeNotFound, errors.New("trap damage not found"))
	}
	if err != nil {
		return fmt.Errorf("find the trap damage: %w", err)
	}
	st.row = trapDamageOf(playdb.ListCampaignTrapDamagesRow(found))
	if done, err := st.replay(ctx, q, session, hasSession); err != nil || done {
		return err
	}
	if st.row.Status != "rolled" {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_DAMAGE_RESOLVED, "the damage was applied or discarded already")
	}
	c, err := s.openTx(ctx, combatTx{tx: tx, q: q, session: session, now: s.now(), characterID: &st.row.CharacterID, actorUserID: st.m.UserID})
	if err != nil {
		return err
	}
	ev := actionEvent{Pending: st.row.ID, Target: st.row.CharacterID, Key: "trap", Amount: st.row.Amount, DamageType: st.row.DamageType}
	final, applied := "discarded", (*int32)(nil)
	if st.apply {
		final = "applied"
		if applied, err = s.landTrapDamageOnVitals(ctx, tx, q, st, session, hasSession, &ev); err != nil {
			return err
		}
	}
	resolved := c.now
	if st.row, err = q.SetTrapDamageStatus(ctx, playdb.SetTrapDamageStatusParams{ID: st.row.ID, Status: final, ResolvedAt: &resolved, AppliedAmount: applied, SettleKey: st.scopedKey, SettleHash: st.hash}); err != nil {
		return fmt.Errorf("settle the trap damage: %w", err)
	}
	if !hasSession {
		return nil
	}
	_, err = insertSceneEvent(ctx, c, st.kind, &st.m.UserID, &st.key, ev)
	return err
}

// lockTrapSettleSession locks the campaign's open session, and says whether there is one.
func lockTrapSettleSession(ctx context.Context, q *playdb.Queries, campaignID string) (playdb.GameSession, bool, error) {
	session, err := q.GetOpenGameSessionForUpdate(ctx, campaignID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return playdb.GameSession{}, false, fmt.Errorf("lock the open session: %w", err)
	}
	return session, err == nil, nil
}

// replay says whether the key settled this damage already (a retry): the damage's own row
// remembers it, and so does an event of the open session. A retry leaves the damage as the first
// call left it.
func (st *trapSettle) replay(ctx context.Context, q *playdb.Queries, session playdb.GameSession, hasSession bool) (bool, error) {
	if done, err := st.replayByDamage(ctx, q); err != nil || done {
		return done, err
	}
	if !hasSession {
		return false, nil
	}
	return st.replayByEvent(ctx, q, session)
}

// replayByDamage finds the damage the key settled: it must be this one, asked the same way.
func (st *trapSettle) replayByDamage(ctx context.Context, q *playdb.Queries) (bool, error) {
	prior, err := q.GetTrapDamageBySettleKey(ctx, st.scopedKey)
	switch {
	case err == nil:
		if prior.ID != st.row.ID {
			return false, idem.ErrReused()
		}
		if err := idem.SameRequest(prior.SettleHash, st.hash); err != nil {
			return false, err
		}
		st.row, st.repeated = prior, true // a retry: the damage is as the first call left it
		return true, nil
	case !errors.Is(err, pgx.ErrNoRows):
		return false, fmt.Errorf("find the damage of this idempotency key: %w", err)
	}
	return false, nil
}

// replayByEvent finds the event of the session the key wrote: it must be this damage's.
func (st *trapSettle) replayByEvent(ctx context.Context, q *playdb.Queries, session playdb.GameSession) (bool, error) {
	done, err := q.GetSessionEventByIdempotencyKey(ctx, playdb.GetSessionEventByIdempotencyKeyParams{GameSessionID: session.ID, IdempotencyKey: &st.key})
	switch {
	case err == nil:
		var prior actionEvent
		if done.Kind != st.kind || json.Unmarshal(done.Payload, &prior) != nil || prior.Pending != st.row.ID {
			return false, errKeyReused()
		}
		st.repeated = true // a retry: the damage is as the first call left it
		return true, nil
	case !errors.Is(err, pgx.ErrNoRows):
		return false, fmt.Errorf("find the event of this idempotency key: %w", err)
	}
	return false, nil
}

// landTrapDamageOnVitals puts the damage on the character's vitals and tells the event what
// changed. It returns the master's amount when he changed it, nil when the rolled one was applied.
// RN-02: the damage goes through the character's vitals, temporary hit points first, never below
// 0; a druid in a beast form takes it on the beast, and what is left goes to the druid when the
// beast falls (MR-037). What changes is the open session's, even when the trap fired in an
// earlier one.
func (s *Service) landTrapDamageOnVitals(ctx context.Context, tx pgx.Tx, q *playdb.Queries, st *trapSettle, session playdb.GameSession, hasSession bool, ev *actionEvent) (*int32, error) {
	amount, applied := st.row.Amount, (*int32)(nil)
	if st.override != nil && *st.override != st.row.Amount {
		amount, applied = *st.override, st.override
	}
	ev.Amount, ev.Overridden, ev.Rolled = amount, applied != nil, st.row.Amount
	campaignID, characterID := st.m.CampaignID, st.row.CharacterID
	now, err := s.vitals.GetVitalsTx(ctx, tx, campaignID, characterID)
	if err != nil {
		return nil, err
	}
	sessionID := st.row.GameSessionID
	if hasSession {
		sessionID = session.ID
	}
	req, left, carried := trapDamageVitalsRequest(now, amount, ev)
	before, vit, err := s.changeVitals(ctx, q, tx, sessionID, campaignID, characterID, req)
	if err != nil {
		return nil, err
	}
	if now.GetWildShape() != nil && left == 0 { // the beast fell: the keeper ended the form, its numbers and its event are ours
		if vit, err = s.formEnds(ctx, q, tx, sessionID, campaignID, characterID, now.GetWildShape().GetBeastKey(), endedByDamage, carried); err != nil {
			return nil, err
		}
	}
	st.after = vit
	st.shapeChanged = (before.GetWildShape() == nil) != (vit.GetWildShape() == nil)
	ev.Carried = carried
	ev.Before, ev.After = new(hpStateOf(before)), new(hpStateOf(vit))
	if hasSession {
		if err := s.followSettledCombat(ctx, q, st, session); err != nil {
			return nil, err
		}
	}
	return applied, nil
}

// trapDamageVitalsRequest is the change of the vitals a damage makes: on the beast for a druid in a
// beast form (with what is left and what is carried over to the druid), else on the temporary hit
// points first. The half-orc who would fall stays on its feet with 1 hit point (SRD 5.1), and the
// event says so.
func trapDamageVitalsRequest(now *playv1.CharacterVitals, amount int32, ev *actionEvent) (req *playv1.AdjustCharacterVitalsRequest, left, carried int32) {
	if now.GetWildShape() != nil {
		return beastDamageRequest(now, amount)
	}
	dmg := combat.ApplyDamage(int(now.GetHitPointsCurrent()), int(now.GetHitPointsTemporary()), int(amount))
	req = &playv1.AdjustCharacterVitalsRequest{}
	if relentlessEndurance(now, dmg) {
		dmg.HP, ev.Relentless = 1, true
		req.ResourcesUsed = []*playv1.ResourceUsed{{Key: resRelentlessEndurance, Used: usesSpentOf(now, resRelentlessEndurance) + 1}}
	}
	hp, temp := clamp32(dmg.HP, 0, maxHitPointChange), clamp32(dmg.TempHP, 0, maxHitPointChange)
	req.HitPointsCurrent, req.HitPointsTemporary = &hp, &temp
	return req, 0, 0
}

// followSettledCombat shows a combat in progress with the character its state from the vitals, and
// keeps the map the fog is told about.
func (s *Service) followSettledCombat(ctx context.Context, q *playdb.Queries, st *trapSettle, session playdb.GameSession) error {
	touched, err := s.touchCombatOf(ctx, q, session.ID, st.row.CharacterID, nil)
	if err != nil {
		return err
	}
	st.touched = touched
	st.visionMap = deref(session.CurrentMapID)
	if touched != nil {
		st.visionMap = deref(touched.MapID)
	}
	return nil
}

// afterTrapSettled tells the streams, after the commit, what the settling changed.
func (s *Service) afterTrapSettled(ctx context.Context, st *trapSettle) {
	campaignID := st.m.CampaignID
	pctx, stop := afterCommit(ctx) // the change is committed: a caller that hangs up must not leave the streams and the fog unheard
	defer stop()
	s.publishVitals(campaignID, st.after)
	s.Publish(campaignID, false, mapChangedHint("")) // the trap card has one damage less
	if st.touched != nil {
		s.publishEncounterChanged(pctx, campaignID, *st.touched)
	}
	if st.shapeChanged { // the beast's senses went away: the fog hears of it (MR-036)
		s.maps.VisionChanged(pctx, campaignID, st.visionMap)
	}
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
