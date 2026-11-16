package play

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/logging"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
	"github.com/PuraFome/meuRPG/backend/internal/play/consumable"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The use of a consumable (ChargesService.UseConsumable): a potion, a scroll or another single-use item of the sheet is
// spent, and what it does is carried out or, for the consumables the app has no effect for, left to the table's text.

// eventConsumableUsed: a consumable was used up. Like the charge events it is not an undoable kind, so it closes the
// undo chain; its payload (consumableUsedEvent) holds ids, numbers and the catalog's names, never free text.
const eventConsumableUsed = "consumable_used"

// sourceKindItem is the source of the effect an item leaves when it is used up: the card of the effect names the item.
const sourceKindItem = "item"

// consumableUsedEvent is the payload of consumable_used (consumable.Used).
type consumableUsedEvent = consumable.Used

// consumableCall is what UseConsumable reads from its request: the call (the user's character and the key) and the receiver.
type consumableCall struct {
	chargeCall
	req    *playv1.UseConsumableRequest
	target string
	// bare says the receiver is the user and no player's character: an NPC has no hit points and no effects of its own outside a combat.
	bare bool
}

// consumableRun is what the transaction leaves for the answer: the event (which holds the outcome), the text of a use with
// no effect and the receiver's vitals.
type consumableRun struct {
	ev       consumableUsedEvent
	text     []string
	vitals   []*playv1.CharacterVitals
	target   string
	repeated bool
}

// UseConsumable implements playv1connect.ChargesServiceHandler: the character's player or the master uses up a consumable of
// the sheet, out of a combat. The receiver is the character or, for a gift, another one.
func (s *Service) UseConsumable(ctx context.Context, req *connect.Request[playv1.UseConsumableRequest]) (*connect.Response[playv1.UseConsumableResponse], error) {
	call, err := readChargeCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetCharacterId(), req.Msg.GetIdempotencyKey(), req.Msg)
	if err != nil {
		return nil, err
	}
	if call.keyText == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("idempotency_key must be a UUID"))
	}
	use := consumableCall{chargeCall: call, req: req.Msg, target: call.charText}
	if t := req.Msg.GetTargetCharacterId(); t != "" {
		id, ok := uid.Canonical(t)
		if !ok {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("character not found"))
		}
		use.target = id
	}
	var run consumableRun
	err = s.txOpen(ctx, call.m.CampaignID, func(tx pgx.Tx, h *hints, session playdb.GameSession) error {
		run = consumableRun{} // a retry starts over
		return s.useConsumableTx(ctx, tx, h, session, use, &run)
	})
	if err != nil {
		return nil, s.dbError(ctx, "use a consumable", err)
	}
	if !run.repeated {
		logging.Event(ctx, s.logger, "consumable.used", slog.String("character_id", call.charText), slog.String("item", run.ev.Item), slog.String("outcome", run.ev.Outcome))
	}
	return connect.NewResponse(consumable.Response(run.ev, run.target, run.text, run.vitals)), nil
}

// useConsumableTx is the transaction of UseConsumable (the session is locked): who may, the retry, the combat, the spent
// unit, the effect, the event and the hints.
func (s *Service) useConsumableTx(ctx context.Context, tx pgx.Tx, h *hints, session playdb.GameSession, use consumableCall, run *consumableRun) error {
	q := s.queries.WithTx(tx)
	if _, err := s.mayUseConsumable(ctx, tx, use); err != nil { // the retry of a key is answered to who may use it only
		return err
	}
	done, again, err := chargeEventByKey(ctx, q, session.ID, use.chargeCall, eventConsumableUsed)
	if err != nil {
		return err
	}
	if again {
		return s.consumableRetry(ctx, tx, h, done, use, run)
	}
	content, item, err := s.spendConsumable(ctx, tx, &use)
	if err != nil {
		return err
	}
	fight, err := s.openFight(ctx, tx, q, h, session, use, content.IsPotion(item.GetKey()))
	if err != nil {
		return err
	}
	ev := startConsumableRun(session, use, item, fight, run)
	if err := s.applyConsumable(ctx, tx, q, session, use, fight, content, run); err != nil {
		return err
	}
	return s.finishConsumable(ctx, tx, q, h, ev, use, fight, run)
}

// spendConsumable checks the user and the receiver, and takes the unit off the sheet. It says that the use is the master's for
// an NPC (use.bare) and returns the item and the rules content.
func (s *Service) spendConsumable(ctx context.Context, tx pgx.Tx, use *consumableCall) (*rules.Content, *charactersv1.SheetMagicItem, error) {
	userPlays, err := s.canUseConsumable(ctx, tx, *use)
	if err != nil {
		return nil, nil, err
	}
	use.bare = !userPlays && use.target == use.charText
	item, err := s.vitals.RemoveConsumable(ctx, tx, use.m.CampaignID, use.charText, use.req.GetEntryId())
	if connect.CodeOf(err) == connect.CodeNotFound {
		return nil, nil, errItemGone()
	}
	if err != nil {
		return nil, nil, err
	}
	content, err := s.roster.RulesContent(ctx, tx, use.m.CampaignID)
	return content, item, err
}

// startConsumableRun fills the event of the use with the item and who received it, and says which session event the use is: the
// combat's when the user fights in it (the line of the log is the user's), else a plain one.
func startConsumableRun(session playdb.GameSession, use consumableCall, item *charactersv1.SheetMagicItem, fight *consumableFight, run *consumableRun) keyedEvent {
	run.target = use.target
	run.ev = consumableUsedEvent{Entry: item.GetId(), Item: item.GetKey(), Name: item.GetNamePt()}
	if use.target != use.charText {
		run.ev.Target = use.target
	}
	ev := keyedEvent{sessionID: session.ID, kind: eventConsumableUsed, userID: use.m.UserID, characterID: &use.charText, key: use.keyText, hash: use.hash}
	if fight != nil && fight.giver != nil {
		fight.name(&run.ev)
		ev.encounterID = &fight.c.enc.ID
	}
	return ev
}

// finishConsumable writes the event of the use and tells the receiver's vitals, the effects and the combat.
func (s *Service) finishConsumable(ctx context.Context, tx pgx.Tx, q *playdb.Queries, h *hints, ev keyedEvent, use consumableCall, fight *consumableFight, run *consumableRun) error {
	if err := s.appendKeyedEvent(ctx, q, ev, run.ev); err != nil {
		return err
	}
	var err error
	if run.vitals, err = s.receiverVitals(ctx, tx, use); err != nil {
		return err
	}
	s.tellVitalsAfter(h, use.m.CampaignID, slices.Compact([]string{use.charText, use.target})...)
	if run.ev.Outcome == consumable.OutcomeResistance {
		h.tell(use.m.CampaignID, castsChangedEvent(false)) // the effects of the character changed
	}
	h.touch()
	if fight != nil {
		return fight.finish(ctx, s, h)
	}
	return nil
}

// mayUseConsumable checks who asks: the character's player or the master. It says who plays the receiver of the use.
func (s *Service) mayUseConsumable(ctx context.Context, tx pgx.Tx, use consumableCall) (receiverOwner string, err error) {
	owner, _, _, err := s.vitals.ItemAccess(ctx, tx, use.m.CampaignID, use.charText)
	if err != nil {
		return "", err
	}
	if use.m.Role != authz.RoleMaster && (owner == "" || owner != use.m.UserID) {
		return "", connect.NewError(connect.CodePermissionDenied, errors.New("only the character's player or the master may do this"))
	}
	if use.target == use.charText {
		return owner, nil
	}
	receiverOwner, _, _, err = s.vitals.ItemAccess(ctx, tx, use.m.CampaignID, use.target)
	return receiverOwner, err
}

// canUseConsumable checks that the user and the receiver are alive, that the receiver is a player's character and that the
// user is not at 0 hit points (an unconscious creature does not act, a creature at 0 may still receive). It says whether the
// user is a player's character: an NPC the master uses for has no hit points to be at 0.
func (s *Service) canUseConsumable(ctx context.Context, tx pgx.Tx, use consumableCall) (userPlays bool, err error) {
	owner, err := s.requireItemCharacter(ctx, tx, use.m, use.charText)
	if err != nil {
		return false, err
	}
	if use.target != use.charText {
		receiver, dead, _, err := s.vitals.ItemAccess(ctx, tx, use.m.CampaignID, use.target)
		if err != nil {
			return false, err
		}
		if receiver == "" { // an NPC or a reserved character is no player's to be given a potion
			return false, errCharacterNotFound()
		}
		if dead {
			return false, characterBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_CHARACTER_DEAD, use.target, "the receiver is dead")
		}
	}
	if owner == "" {
		return false, nil
	}
	return true, s.mustNotBeDown(ctx, tx, use.m.CampaignID, playdb.Combatant{Kind: kindPlayer, CharacterID: use.charText})
}

// applyConsumable carries out what the item does to the receiver, by the content's potion table, and fills the event. The
// fight is the combat the use happens in, nil outside one; its receiver is nil when the receiver is not a combatant of it.
func (s *Service) applyConsumable(ctx context.Context, tx pgx.Tx, q *playdb.Queries, session playdb.GameSession, use consumableCall, fight *consumableFight, content *rules.Content, run *consumableRun) error {
	potion, ok := content.PotionEffect(run.ev.Item)
	ok = ok && !use.bare // a character with no vitals has nothing to heal or to resist for: the master tells what the potion did
	switch {
	case ok && potion.Kind == rules.PotionHeal:
		return s.healWithPotion(ctx, tx, q, session, use, fight, potion, &run.ev)
	case ok && potion.Kind == rules.PotionResistance:
		run.ev.Outcome, run.ev.DamageType = consumable.OutcomeResistance, potion.DamageType
		if fight.receives() {
			return fight.resist(ctx, s, potion, run.ev.Item)
		}
		return s.putPotionResistance(ctx, tx, q, session, use, potion, run.ev.Item)
	}
	run.ev.Outcome = consumable.OutcomeTextOnly
	item, _ := content.MagicItem(run.ev.Item)
	run.text = consumable.Text(item)
	return nil
}

// healWithPotion rolls the potion's dice (the app's, or the faces typed from real dice by the table's rule, RN-18) and heals the
// receiver up to the maximum: in a combat with the combat's heal (a creature at 0 hit points gets up and its death saves
// reset), else through the vitals. The session's lock holds every other write of the vitals back while it reads and writes.
func (s *Service) healWithPotion(ctx context.Context, tx pgx.Tx, q *playdb.Queries, session playdb.GameSession, use consumableCall, fight *consumableFight, potion rules.PotionEffect, ev *consumableUsedEvent) error {
	if err := s.mustRollThisWay(ctx, tx, use.m, rollInput{inApp: use.req.GetRollInApp()}); err != nil {
		return err
	}
	faces, total, err := consumable.Roll(s.roller, potion.Heal, use.req.GetRollInApp(), use.req.GetTypedFaces())
	if errors.Is(err, consumable.ErrFaces) {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	if err != nil {
		return err
	}
	var healed int32
	if fight.receives() {
		healed, err = fight.heal(ctx, s, total)
	} else {
		healed, err = s.healCharacter(ctx, tx, q, session, use, total)
	}
	if err != nil {
		return err
	}
	ev.Outcome, ev.Dice, ev.Faces, ev.Total, ev.Healed, ev.Physical = consumable.OutcomeHealed, potion.Heal.Text(), faces, total, healed, !use.req.GetRollInApp()
	return nil
}

// healCharacter gives the receiver the hit points of a heal outside a combat, as the combat's heal does (a beast form is healed
// before the druid), and says how many it regained.
func (s *Service) healCharacter(ctx context.Context, tx pgx.Tx, q *playdb.Queries, session playdb.GameSession, use consumableCall, total int32) (int32, error) {
	before, err := s.vitals.GetVitalsTx(ctx, tx, use.m.CampaignID, use.target)
	if err != nil {
		return 0, err
	}
	req, healed := healRequest(before, total)
	if healed > 0 {
		if _, _, err := s.changeVitals(ctx, q, tx, session.ID, use.m.CampaignID, use.target, req); err != nil {
			return 0, err
		}
	}
	return healed, nil
}

// putPotionResistance gives the receiver the potion's resistance as an effect of the character outside a combat: one hour of
// game time, the owner's and the master's to read. A second potion of the same kind starts the hour again.
func (s *Service) putPotionResistance(ctx context.Context, tx pgx.Tx, q *playdb.Queries, session playdb.GameSession, use consumableCall, potion rules.PotionEffect, key string) error {
	if err := s.requireOutsideCombat(ctx, tx, q, use.m.CampaignID, session.ID, []string{use.target}); err != nil {
		return err
	}
	group := uuid.New().String()
	if _, err := q.DeleteCharacterEffectsOfSpell(ctx, playdb.DeleteCharacterEffectsOfSpellParams{CharacterID: use.target, SourceKey: key, GroupID: group}); err != nil {
		return fmt.Errorf("replace the same potion's resistance: %w", err)
	}
	body, err := consumable.ResistanceJSON(potion)
	if err != nil {
		return err
	}
	seconds := consumable.Seconds(potion)
	if _, err := q.InsertCharacterEffect(ctx, playdb.InsertCharacterEffectParams{
		CampaignID: use.m.CampaignID, CharacterID: use.target, GroupID: group, SourceKey: key, SourceKind: sourceKindItem,
		ConditionKeys: []string{}, Modifiers: body, DurationKind: rules.EffectDurationRounds, SecondsLeft: &seconds,
		PlayerVisible: true, Audience: audienceOwner, CreatedAt: s.now(),
	}); err != nil {
		return fmt.Errorf("put the resistance on the character: %w", err)
	}
	return nil
}

// receiverVitals are the receiver's vitals as they stand, for the master and for the receiver's own player: a giver that
// does not play the receiver is told what the use did (the hit points regained) and no more. None for a character that is
// no player's.
func (s *Service) receiverVitals(ctx context.Context, tx pgx.Tx, use consumableCall) ([]*playv1.CharacterVitals, error) {
	if use.m.Role != authz.RoleMaster && use.target != use.charText {
		owner, _, _, err := s.vitals.ItemAccess(ctx, tx, use.m.CampaignID, use.target)
		if err != nil || owner != use.m.UserID {
			return nil, err
		}
	}
	v, err := s.vitals.GetVitalsTx(ctx, tx, use.m.CampaignID, use.target)
	if connect.CodeOf(err) == connect.CodeNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return []*playv1.CharacterVitals{v}, nil
}

// consumableRetry answers the retry of a key found stored: what the first call answered, with the receiver's vitals as they
// stand, and the hints the first call may have died before telling.
func (s *Service) consumableRetry(ctx context.Context, tx pgx.Tx, h *hints, done playdb.GetSessionEventByIdempotencyKeyRow, use consumableCall, run *consumableRun) error {
	if err := replayCharacterEvent(done, use.charText, "consumable", &run.ev); err != nil {
		return err
	}
	if use.target != cmp.Or(run.ev.Target, use.charText) {
		return errKeyReused()
	}
	content, err := s.roster.RulesContent(ctx, tx, use.m.CampaignID)
	if err != nil {
		return err
	}
	if item, ok := content.MagicItem(run.ev.Item); ok && run.ev.Outcome == consumable.OutcomeTextOnly {
		run.text = consumable.Text(item)
	}
	if run.vitals, err = s.receiverVitals(ctx, tx, use); err != nil {
		return err
	}
	run.target, run.repeated = use.target, true
	h.replay()
	s.tellVitalsAfter(h, use.m.CampaignID, slices.Compact([]string{use.charText, use.target})...)
	return nil
}
