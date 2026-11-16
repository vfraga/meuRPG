package play

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/platform/logging"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// The charges of the magic items and the ammunition at the table (ChargesService). The numbers are the characters
// module's (ItemKeeper); this file decides who may ask and when, rolls the dice (RN-18), writes the history and the
// combat's line, and tells the streams. What the log line says and who reads it is combat_log.go's.

// ItemKeeper is what the characters module does for the charges of the magic items and the ammunition. Like the
// VitalsKeeper's methods, they take no caller and run inside the transaction they are given; this package decides who
// may ask and when, writes the history and tells the streams.
type ItemKeeper interface {
	// ItemAccess says who plays the character ("" for an NPC), whether it is dead and whether its player may still
	// edit the sheet. `not_found` for a character that is not in the campaign.
	ItemAccess(ctx context.Context, tx pgx.Tx, campaignID, characterID string) (playerUserID string, dead, sheetEditable bool, err error)
	// ChargedItem reads an entry of the character's sheet with its charges, and locks the character.
	ChargedItem(ctx context.Context, tx pgx.Tx, campaignID, characterID, entryID string) (*charactersv1.SheetMagicItem, error)
	// ChargedItemRead is ChargedItem for a pure read: it locks nothing.
	ChargedItemRead(ctx context.Context, tx pgx.Tx, campaignID, characterID, entryID string) (*charactersv1.SheetMagicItem, error)
	// SpendItemCharges spends charges of an entry and returns the entry after.
	SpendItemCharges(ctx context.Context, tx pgx.Tx, campaignID, characterID, entryID string, amount int32) (after *charactersv1.SheetMagicItem, err error)
	// ResolveLastCharge settles the d20 of an item whose last charge went; extra is the charges a 20 gives back.
	ResolveLastCharge(ctx context.Context, tx pgx.Tx, campaignID, characterID, entryID string, d20, extra int32) (after *charactersv1.SheetMagicItem, regained int32, err error)
	// SetItemCharges is the master's correction of an entry.
	SetItemCharges(ctx context.Context, tx pgx.Tx, campaignID, characterID, entryID string, left int32, lost charactersv1.ItemLoss) (after *charactersv1.SheetMagicItem, err error)
	// SetAmmunition sets the pieces the character holds of a kind.
	SetAmmunition(ctx context.Context, tx pgx.Tx, campaignID, characterID, ammoKey string, quantity int32) (*charactersv1.SheetAmmunition, error)
	// SpendAmmunition takes one piece of a kind for a shot in the encounter and returns the pieces left; counted is
	// false (and nothing is spent) for a kind the character has no count for.
	SpendAmmunition(ctx context.Context, tx pgx.Tx, campaignID, characterID, ammoKey, encounterID string) (left int32, counted bool, err error)
	// RefundAmmunition gives back pieces that an attack of the encounter took, when the attack is undone.
	RefundAmmunition(ctx context.Context, tx pgx.Tx, campaignID, characterID, ammoKey, encounterID string, pieces int32) (left int32, err error)
	// AmmunitionLeft is the pieces a character of the campaign holds of a kind, and whether the kind is counted.
	AmmunitionLeft(ctx context.Context, tx pgx.Tx, campaignID, characterID, ammoKey string) (left int32, counted bool, err error)
	// RecoverAmmunition takes back half of what the character fired in the encounter.
	RecoverAmmunition(ctx context.Context, tx pgx.Tx, campaignID, characterID, encounterID string) ([]*charactersv1.SheetAmmunition, error)
	// RemoveConsumable uses up one unit of a consumable: it takes the entry off the character's sheet and returns it
	// (key and name), and deletes its stored charge state. `not_found` for a character or an entry that is not there,
	// `invalid_argument` for an item that is not a consumable.
	RemoveConsumable(ctx context.Context, tx pgx.Tx, campaignID, characterID, entryID string) (*charactersv1.SheetMagicItem, error)
	// ConsumableItem reads a consumable entry of the character's sheet (key and name) and writes and locks nothing. `not_found`
	// for a character or an entry that is not there, `invalid_argument` for an item that is not a consumable.
	ConsumableItem(ctx context.Context, tx pgx.Tx, campaignID, characterID, entryID string) (*charactersv1.SheetMagicItem, error)
	// RestoreDawn gives every magic item of the campaign's characters its dawn, with the dice rolled by roll, and
	// returns the items that regained charges.
	RestoreDawn(ctx context.Context, tx pgx.Tx, campaignID string, roll link.DawnRoll) ([]link.DawnItem, error)
}

// chargeCall is what a write of ChargesService reads from its request before the transaction.
type chargeCall struct {
	m        authz.Membership
	charText string
	keyText  string
	hash     *string
}

// readChargeCall checks that the caller is a member of the campaign and reads the character and the key of the request, in
// that order of refusals.
func readChargeCall(ctx context.Context, campaignID, characterID, key string, req proto.Message) (chargeCall, error) {
	m, err := authz.RequireCampaignMember(ctx, campaignID)
	if err != nil {
		return chargeCall{}, err
	}
	return chargeCallOf(m, characterID, key, req)
}

func chargeCallOf(m authz.Membership, characterID, key string, req proto.Message) (chargeCall, error) {
	id, err := uuid.Parse(characterID)
	if err != nil {
		return chargeCall{}, connect.NewError(connect.CodeNotFound, errors.New("character not found"))
	}
	call := chargeCall{m: m, charText: id.String(), hash: idem.Hash(req)}
	if key == "" {
		return call, nil // SetAmmunition writes no event: no key
	}
	k, err := uuid.Parse(key)
	if err != nil {
		return chargeCall{}, connect.NewError(connect.CodeInvalidArgument, errors.New("idempotency_key must be a UUID"))
	}
	call.keyText = k.String()
	return call, nil
}

// itemUseEvent is the payload of the events of the charges and of the ammunition fired. The keys round, combatant_id and
// secret are the ones the combat log reads from every event; the rest only this line reads. It is the master's and the owner's alone
// (Secret), and holds ids, numbers and the catalog's names.
type itemUseEvent struct {
	Round  int32  `json:"round"`
	Actor  string `json:"combatant_id,omitempty"`
	Secret bool   `json:"secret"`
	// Entry and Item are the entry's id on the sheet and the catalog key (the weapon's, for a piece of ammunition).
	Entry string `json:"use_entry,omitempty"`
	Item  string `json:"use_item"`
	Name  string `json:"use_name_pt,omitempty"`
	// Amount is what was spent (or regained), Left what remains and Max the item's maximum charges (0 for ammunition).
	Amount int32 `json:"use_amount"`
	Left   int32 `json:"use_left"`
	Max    int32 `json:"use_max,omitempty"`
	// Roll says the line is a d20 of the last charge; D20 is its face, RegainFace the face of the die of a 20.
	Roll       bool  `json:"use_roll,omitempty"`
	D20        int32 `json:"use_d20,omitempty"`
	RegainFace int32 `json:"use_regain_face,omitempty"`
	Physical   bool  `json:"use_physical,omitempty"`
	// Outcome is one of the itemOutcome* values; Lost the ItemLoss the item has after the call.
	Outcome string `json:"use_outcome"`
	Lost    int32  `json:"use_lost,omitempty"`
	// Ammo is the name of the kind of ammunition fired.
	Ammo string `json:"use_ammo_pt,omitempty"`
}

// What a use left behind, as the payload says it (ItemUseOutcome on the line).
const (
	itemOutcomeSpent      = "spent"
	itemOutcomeLastCharge = "last_charge"
	itemOutcomeLost       = "lost"
	itemOutcomeStays      = "stays"
	itemOutcomeRegained   = "regained"
)

// readItemUse reads the payload of one of these events.
func readItemUse(payload []byte) (itemUseEvent, error) {
	var ev itemUseEvent
	if err := json.Unmarshal(payload, &ev); err != nil {
		return itemUseEvent{}, fmt.Errorf("read an item use payload: %w", err)
	}
	return ev, nil
}

// chargeKinds are the kinds of event a call of ChargesService may have written for its key.
var chargeKinds = []string{eventItemChargeSpent, eventItemChargesRestored, eventItemDestroyed}

// chargeEventByKey is eventByKey for a call that writes one of several kinds, by what it found.
func chargeEventByKey(ctx context.Context, q *playdb.Queries, sessionID string, call chargeCall, kinds ...string) (done playdb.GetSessionEventByIdempotencyKeyRow, repeated bool, err error) {
	done, err = q.GetSessionEventByIdempotencyKey(ctx, playdb.GetSessionEventByIdempotencyKeyParams{GameSessionID: sessionID, IdempotencyKey: &call.keyText})
	switch {
	case err == nil:
		if !slices.Contains(kinds, done.Kind) || hashDiffers(done.IdempotencyHash, call.hash) {
			return done, false, errKeyReused()
		}
		return done, true, nil
	case errors.Is(err, pgx.ErrNoRows):
		return done, false, nil
	}
	return done, false, fmt.Errorf("find the event of this idempotency key: %w", err)
}

// characterBlocked is the failed_precondition of a character that cannot do this now, with the detail the app reads.
func characterBlocked(reason charactersv1.CharacterBlockedReason, characterID, msg string) error {
	err := connect.NewError(connect.CodeFailedPrecondition, errors.New(msg))
	if detail, derr := connect.NewErrorDetail(&charactersv1.CharacterBlocked{Reason: reason, CharacterId: characterID}); derr == nil {
		err.AddDetail(detail)
	}
	return err
}

// chargesBlocked is the failed_precondition of a rule of ChargesService, with its detail.
func chargesBlocked(reason playv1.ChargesBlockedReason, msg string, needed int) error {
	err := connect.NewError(connect.CodeFailedPrecondition, errors.New(msg))
	if detail, derr := connect.NewErrorDetail(&playv1.ChargesBlocked{Reason: reason, Needed: clamp32(needed, 0, 1<<20)}); derr == nil {
		err.AddDetail(detail)
	}
	return err
}

// requireItemCharacter lets through the master and the character's player and nobody else, and refuses a dead character
// (SpendCharges, RollLastCharge and RecoverAmmunition are for the living). `not_found` for a character that is not in the campaign.
// It returns the user who plays the character.
func (s *Service) requireItemCharacter(ctx context.Context, tx pgx.Tx, m authz.Membership, characterID string) (string, error) {
	owner, dead, _, err := s.vitals.ItemAccess(ctx, tx, m.CampaignID, characterID)
	if err != nil {
		return "", err
	}
	if m.Role != authz.RoleMaster && (owner == "" || owner != m.UserID) {
		return "", connect.NewError(connect.CodePermissionDenied, errors.New("only the character's player or the master may do this"))
	}
	if dead {
		return "", characterBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_CHARACTER_DEAD, characterID, "the character is dead")
	}
	return owner, nil
}

// itemName is the Portuguese name of a catalog item, the key's last part when the content has none.
func (s *Service) itemName(ctx context.Context, tx pgx.Tx, campaignID, key string) (string, error) {
	names, err := s.roster.ContentNames(ctx, tx, campaignID)
	if err != nil {
		return "", err
	}
	if name := names(key); name != "" {
		return name, nil
	}
	return key[strings.LastIndex(key, ":")+1:], nil
}

// itemWrite is what a write of the charges tells the transaction about the event it leaves.
type itemWrite struct {
	call chargeCall
	kind string
	ev   itemUseEvent
	// line says the event is a line of the combat's log when the character fights in a running combat.
	line bool
}

// writeItemEvent writes the session event of the call. When the character is a combatant of the running combat and the
// call is a line of its log, the event belongs to that combat, in the round it is in, and the log's line is the master's and the
// owner's (RN-10); otherwise it is a plain event of the session. It tells the streams.
func (s *Service) writeItemEvent(ctx context.Context, tx pgx.Tx, q *playdb.Queries, h *hints, session playdb.GameSession, w itemWrite) error {
	campaignID := session.CampaignID
	if w.line {
		enc, cs, found, err := openCombatants(ctx, q, session.ID)
		if err != nil {
			return err
		}
		if who := wearerIn(cs, w.call.charText); found && enc.Status == statusActive && who != nil {
			return s.writeItemLine(ctx, tx, q, h, session, enc, *who, w)
		}
	}
	err := s.appendKeyedEvent(ctx, q, keyedEvent{
		sessionID: session.ID, kind: w.kind, userID: w.call.m.UserID, characterID: &w.call.charText, key: w.call.keyText, hash: w.call.hash,
	}, w.ev)
	if err != nil {
		return err
	}
	s.tellVitalsAfter(h, campaignID, w.call.charText)
	h.touch()
	return nil
}

// writeItemLine is writeItemEvent in a running combat that has the character in it.
func (s *Service) writeItemLine(ctx context.Context, tx pgx.Tx, q *playdb.Queries, h *hints, session playdb.GameSession, enc playdb.Encounter, who playdb.Combatant, w itemWrite) error {
	c, err := s.openTx(ctx, combatTx{tx: tx, q: q, session: session, enc: enc, now: s.now(), actorUserID: w.call.m.UserID, svc: s, hash: w.call.hash, hints: h})
	if err != nil {
		return err
	}
	ev := w.ev
	ev.Round, ev.Actor, ev.Secret = c.enc.Round, who.ID, true
	body, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("encode the event payload: %w", err)
	}
	c.characterID = &w.call.charText
	if _, err := insertRaw(ctx, c, w.kind, &w.call.m.UserID, &w.call.keyText, body); err != nil {
		return err
	}
	if err := touch(ctx, c); err != nil { // as every change of a combat: the stream carries the new revision
		return err
	}
	s.tellVitalsAfter(h, session.CampaignID, w.call.charText)
	s.tellEncounterRowAfter(h, session.CampaignID, c.enc)
	for _, a := range []live.Audience{{Master: true}, {UserID: conv.OrZero(who.UserID)}} {
		if a.Master || a.UserID != "" {
			h.tell(session.CampaignID, live.Event{Audience: a, Message: &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_CombatLogChanged_{
				CombatLogChanged: &playv1.WatchGameSessionResponse_CombatLogChanged{EncounterId: c.enc.ID},
			}}})
		}
	}
	return nil
}

// chargeRun is what the transaction of a write of the charges leaves for after the commit.
type chargeRun struct {
	item     *charactersv1.SheetMagicItem
	ev       itemUseEvent
	repeated bool
}

// SpendCharges implements playv1connect.ChargesServiceHandler: the character's player or the master spends the
// charges of an item.
func (s *Service) SpendCharges(ctx context.Context, req *connect.Request[playv1.SpendChargesRequest]) (*connect.Response[playv1.SpendChargesResponse], error) {
	call, err := readChargeCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetCharacterId(), req.Msg.GetIdempotencyKey(), req.Msg)
	if err != nil {
		return nil, err
	}
	var run chargeRun
	err = s.txOpen(ctx, call.m.CampaignID, func(tx pgx.Tx, h *hints, session playdb.GameSession) error {
		run = chargeRun{} // a retry starts over
		return s.spendChargesTx(ctx, tx, h, session, call, req.Msg, &run)
	})
	if err != nil {
		return nil, s.dbError(ctx, "spend charges", err)
	}
	if !run.repeated {
		logging.Event(ctx, s.logger, "item_charges.spent", slog.String("character_id", call.charText), slog.Int("amount", int(req.Msg.GetAmount())), slog.String("outcome", run.ev.Outcome))
	}
	return connect.NewResponse(&playv1.SpendChargesResponse{Item: run.item}), nil
}

// spendChargesTx is the transaction of SpendCharges (the session is locked): retry, who may, spend, write, tell.
func (s *Service) spendChargesTx(ctx context.Context, tx pgx.Tx, h *hints, session playdb.GameSession, call chargeCall, req *playv1.SpendChargesRequest, run *chargeRun) error {
	q := s.queries.WithTx(tx)
	done, again, err := chargeEventByKey(ctx, q, session.ID, call, chargeKinds...)
	if err != nil {
		return err
	}
	if again {
		return s.chargeRetry(ctx, tx, h, call, done, req.GetEntryId(), run)
	}
	if _, err := s.requireItemCharacter(ctx, tx, call.m, call.charText); err != nil {
		return err
	}
	after, err := s.vitals.SpendItemCharges(ctx, tx, call.m.CampaignID, call.charText, req.GetEntryId(), req.GetAmount())
	if err != nil {
		return err
	}
	name, err := s.itemName(ctx, tx, call.m.CampaignID, after.GetKey())
	if err != nil {
		return err
	}
	ev := itemUseEvent{
		Entry: after.GetId(), Item: after.GetKey(), Name: name, Amount: req.GetAmount(), Left: after.GetChargesLeft(), Max: after.GetChargesMax(),
		Outcome: itemOutcomeSpent, Lost: int32(after.GetLost()),
	}
	kind := eventItemChargeSpent
	switch {
	case after.GetLost() != charactersv1.ItemLoss_ITEM_LOSS_UNSPECIFIED:
		ev.Outcome, kind = itemOutcomeLost, eventItemDestroyed
	case after.GetLastRollPending():
		ev.Outcome = itemOutcomeLastCharge
	}
	run.item, run.ev = after, ev
	return s.writeItemEvent(ctx, tx, q, h, session, itemWrite{call: call, kind: kind, ev: ev, line: true})
}

// chargeRetry answers the retry of a key found stored: the item as it stands now, and the hints the first call may have died before telling.
func (s *Service) chargeRetry(ctx context.Context, tx pgx.Tx, h *hints, call chargeCall, done playdb.GetSessionEventByIdempotencyKeyRow, entryID string, run *chargeRun) error {
	if done.CharacterID == nil || *done.CharacterID != call.charText {
		return errKeyReused()
	}
	ev, err := readItemUse(done.Payload)
	if err != nil {
		return err
	}
	item, err := s.vitals.ChargedItem(ctx, tx, call.m.CampaignID, call.charText, entryID)
	if err != nil {
		return err
	}
	run.item, run.ev, run.repeated = item, ev, true
	h.replay()
	s.tellVitalsAfter(h, call.m.CampaignID, call.charText)
	return nil
}

// rollLastChargeInput is how the d20 and the die of a 20 come: rolled here or typed from real dice.
type rollLastChargeInput struct {
	in         rollInput
	regainFace int
}

// readRollLastCharge reads how the roll comes: the app's, or the typed face of a real d20.
func readRollLastCharge(req *playv1.RollLastChargeRequest) (rollLastChargeInput, error) {
	var out rollLastChargeInput
	switch roll := req.GetRoll().(type) {
	case *playv1.RollLastChargeRequest_RollInApp:
		if !roll.RollInApp {
			return out, connect.NewError(connect.CodeInvalidArgument, errors.New("roll_in_app must be true"))
		}
		out.in.inApp = true
	case *playv1.RollLastChargeRequest_D20Face:
		out.in.typed = int(roll.D20Face)
		if out.in.typed < 1 || out.in.typed > 20 {
			return out, connect.NewError(connect.CodeInvalidArgument, errors.New("d20_face must be a face of the d20"))
		}
	default:
		return out, connect.NewError(connect.CodeInvalidArgument, errors.New("set roll_in_app or d20_face"))
	}
	out.regainFace = int(req.GetTypedRegainFace())
	return out, nil
}

// RollLastCharge implements playv1connect.ChargesServiceHandler: the d20 of an item whose last charge went.
func (s *Service) RollLastCharge(ctx context.Context, req *connect.Request[playv1.RollLastChargeRequest]) (*connect.Response[playv1.RollLastChargeResponse], error) {
	call, err := readChargeCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetCharacterId(), req.Msg.GetIdempotencyKey(), req.Msg)
	if err != nil {
		return nil, err
	}
	input, err := readRollLastCharge(req.Msg)
	if err != nil {
		return nil, err
	}
	var run chargeRun
	err = s.txOpen(ctx, call.m.CampaignID, func(tx pgx.Tx, h *hints, session playdb.GameSession) error {
		run = chargeRun{} // a retry starts over
		return s.rollLastChargeTx(ctx, tx, h, session, call, req.Msg.GetEntryId(), input, &run)
	})
	if err != nil {
		return nil, s.dbError(ctx, "roll the last charge", err)
	}
	if !run.repeated {
		logging.Event(ctx, s.logger, "item_charges.last_rolled", slog.String("character_id", call.charText), slog.Int("d20", int(run.ev.D20)), slog.String("outcome", run.ev.Outcome))
	}
	return connect.NewResponse(&playv1.RollLastChargeResponse{
		Item: run.item, D20: run.ev.D20, Regained: regainedOf(run.ev), Lost: charactersv1.ItemLoss(run.ev.Lost), RegainFace: run.ev.RegainFace,
	}), nil
}

// regainedOf is the charges a roll gave back (the amount of a REGAINED line).
func regainedOf(ev itemUseEvent) int32 {
	if ev.Outcome == itemOutcomeRegained {
		return ev.Amount
	}
	return 0
}

// rollLastChargeTx is the transaction of RollLastCharge (the session is locked): retry, who may, the way of rolling, the
// dice, the keeper's settling, the event.
func (s *Service) rollLastChargeTx(ctx context.Context, tx pgx.Tx, h *hints, session playdb.GameSession, call chargeCall, entryID string, input rollLastChargeInput, run *chargeRun) error {
	q := s.queries.WithTx(tx)
	done, again, err := chargeEventByKey(ctx, q, session.ID, call, chargeKinds...)
	if err != nil {
		return err
	}
	if again {
		return s.chargeRetry(ctx, tx, h, call, done, entryID, run)
	}
	if _, err := s.requireItemCharacter(ctx, tx, call.m, call.charText); err != nil {
		return err
	}
	if err := s.mustRollThisWay(ctx, tx, call.m, input.in); err != nil {
		return err
	}
	item, err := s.vitals.ChargedItem(ctx, tx, call.m.CampaignID, call.charText, entryID)
	if err != nil {
		return err
	}
	if !item.GetLastRollPending() {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("the item has no last charge to roll"))
	}
	d20, regainFace, extra, err := s.rollTheLastCharge(ctx, tx, call.m.CampaignID, item.GetKey(), input)
	if err != nil {
		return err
	}
	after, regained, err := s.vitals.ResolveLastCharge(ctx, tx, call.m.CampaignID, call.charText, entryID, d20, extra)
	if err != nil {
		return err
	}
	name, err := s.itemName(ctx, tx, call.m.CampaignID, after.GetKey())
	if err != nil {
		return err
	}
	ev := itemUseEvent{
		Entry: after.GetId(), Item: after.GetKey(), Name: name, Left: after.GetChargesLeft(), Max: after.GetChargesMax(), Roll: true, D20: d20,
		RegainFace: regainFace, Physical: !input.in.inApp, Outcome: itemOutcomeStays, Lost: int32(after.GetLost()),
	}
	kind := eventItemChargeSpent
	switch {
	case after.GetLost() != charactersv1.ItemLoss_ITEM_LOSS_UNSPECIFIED:
		ev.Outcome, kind = itemOutcomeLost, eventItemDestroyed
	case regained > 0:
		ev.Outcome, ev.Amount, kind = itemOutcomeRegained, regained, eventItemChargesRestored
	}
	run.item, run.ev = after, ev
	return s.writeItemEvent(ctx, tx, q, h, session, itemWrite{call: call, kind: kind, ev: ev, line: true})
}

// rollTheLastCharge rolls the d20 (or takes the typed face) and, for a 20 on an item that gives charges back, the die that counts
// them, the same way. It returns the d20, the face of the regain die (the sum of the dice when the item rolls more than one) and the
// charges the 20 gives back (the die plus the item's bonus), 0 for any other face. Without the typed die's face a typed 20 is refused.
func (s *Service) rollTheLastCharge(ctx context.Context, tx pgx.Tx, campaignID, itemKey string, input rollLastChargeInput) (d20, regainFace, regain int32, err error) {
	face := input.in.typed
	if input.in.inApp {
		rolled, err := dice.Roll(s.roller, dice.Expr{Count: 1, Sides: d20Sides})
		if err != nil {
			return 0, 0, 0, fmt.Errorf("roll the d20 of the last charge: %w", err)
		}
		face = rolled.Faces[0]
	}
	d20 = clamp32(face, 1, 20)
	if d20 != d20Sides {
		return d20, 0, 0, nil
	}
	content, err := s.roster.RulesContent(ctx, tx, campaignID)
	if err != nil {
		return 0, 0, 0, err
	}
	ic, _ := content.ItemCharges(itemKey)
	f := ic.EmptyHighRegain
	if f.Count == 0 {
		return d20, 0, 0, nil // this item gives nothing back on a 20
	}
	total := input.regainFace
	if input.in.inApp {
		rolled, err := dice.Roll(s.roller, dice.Expr{Count: f.Count, Sides: f.Sides})
		if err != nil {
			return 0, 0, 0, fmt.Errorf("roll the charges the last charge gives back: %w", err)
		}
		total = rolled.Total
	}
	switch {
	case total == 0:
		return 0, 0, 0, chargesBlocked(playv1.ChargesBlockedReason_CHARGES_BLOCKED_REASON_REGAIN_FACE_NEEDED, "a 20 gives charges back: type the face of its die too", f.Sides)
	case total < f.Count || total > f.Count*f.Sides:
		return 0, 0, 0, connect.NewError(connect.CodeInvalidArgument, errors.New("typed_regain_face must be a face of the die"))
	}
	return d20, clamp32(total, 0, 1<<20), clamp32(total+f.Bonus, 0, 1<<20), nil
}

// SetCharges implements playv1connect.ChargesServiceHandler: the master's correction of an item's charges.
func (s *Service) SetCharges(ctx context.Context, req *connect.Request[playv1.SetChargesRequest]) (*connect.Response[playv1.SetChargesResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	call, err := chargeCallOf(m, req.Msg.GetCharacterId(), req.Msg.GetIdempotencyKey(), req.Msg)
	if err != nil {
		return nil, err
	}
	var run chargeRun
	err = s.txOpen(ctx, call.m.CampaignID, func(tx pgx.Tx, h *hints, session playdb.GameSession) error {
		run = chargeRun{} // a retry starts over
		return s.setChargesTx(ctx, tx, h, session, call, req.Msg, &run)
	})
	if err != nil {
		return nil, s.dbError(ctx, "set charges", err)
	}
	if !run.repeated {
		logging.Event(ctx, s.logger, "item_charges.set", slog.String("character_id", call.charText), slog.Int("left", int(run.ev.Left)), slog.String("outcome", run.ev.Outcome))
	}
	return connect.NewResponse(&playv1.SetChargesResponse{Item: run.item}), nil
}

// setChargesTx is the transaction of SetCharges: retry, the keeper's write, the event (never a line of a combat's log).
func (s *Service) setChargesTx(ctx context.Context, tx pgx.Tx, h *hints, session playdb.GameSession, call chargeCall, req *playv1.SetChargesRequest, run *chargeRun) error {
	q := s.queries.WithTx(tx)
	done, again, err := chargeEventByKey(ctx, q, session.ID, call, chargeKinds...)
	if err != nil {
		return err
	}
	if again {
		return s.chargeRetry(ctx, tx, h, call, done, req.GetEntryId(), run)
	}
	after, err := s.vitals.SetItemCharges(ctx, tx, call.m.CampaignID, call.charText, req.GetEntryId(), req.GetChargesLeft(), req.GetLost())
	if err != nil {
		return err
	}
	name, err := s.itemName(ctx, tx, call.m.CampaignID, after.GetKey())
	if err != nil {
		return err
	}
	ev := itemUseEvent{
		Entry: after.GetId(), Item: after.GetKey(), Name: name, Left: after.GetChargesLeft(), Max: after.GetChargesMax(),
		Outcome: itemOutcomeSpent, Lost: int32(after.GetLost()),
	}
	kind := eventItemChargesRestored
	if after.GetLost() != charactersv1.ItemLoss_ITEM_LOSS_UNSPECIFIED {
		ev.Outcome, kind = itemOutcomeLost, eventItemDestroyed
	}
	run.item, run.ev = after, ev
	return s.writeItemEvent(ctx, tx, q, h, session, itemWrite{call: call, kind: kind, ev: ev})
}

// SetAmmunition implements playv1connect.ChargesServiceHandler: the pieces a character holds of a kind. The owner sets them
// while the sheet is open to editing, the master at any time. No session is needed and no event is written.
func (s *Service) SetAmmunition(ctx context.Context, req *connect.Request[playv1.SetAmmunitionRequest]) (*connect.Response[playv1.SetAmmunitionResponse], error) {
	call, err := readChargeCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetCharacterId(), "", req.Msg)
	if err != nil {
		return nil, err
	}
	var ammo *charactersv1.SheetAmmunition
	err = s.txOrNone(ctx, call.m.CampaignID, func(tx pgx.Tx, h *hints, _ string) error {
		ammo = nil // a retry starts over
		owner, _, editable, err := s.vitals.ItemAccess(ctx, tx, call.m.CampaignID, call.charText)
		if err != nil {
			return err
		}
		if call.m.Role != authz.RoleMaster && (owner == "" || owner != call.m.UserID || !editable) {
			return connect.NewError(connect.CodePermissionDenied, errors.New("only the master may set the ammunition once the sheet is locked, and only the character's player before"))
		}
		if ammo, err = s.vitals.SetAmmunition(ctx, tx, call.m.CampaignID, call.charText, req.Msg.GetAmmoKey(), req.Msg.GetQuantity()); err != nil {
			return err
		}
		s.tellVitalsAfter(h, call.m.CampaignID, call.charText)
		h.touch()
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "set ammunition", err)
	}
	logging.Event(ctx, s.logger, "ammunition.set", slog.String("character_id", call.charText), slog.String("ammo_key", ammo.GetKey()), slog.Int("quantity", int(ammo.GetQuantity())))
	return connect.NewResponse(&playv1.SetAmmunitionResponse{Ammunition: ammo}), nil
}

// ammunitionRecoveredEvent is the payload of ammunition_recovered: the kinds the search gave back, with the pieces held after.
type ammunitionRecoveredEvent struct {
	Encounter string               `json:"encounter_id"`
	Kinds     []ammunitionKindSeen `json:"kinds"`
}

// ammunitionKindSeen is one kind of ammunition of a recovery.
type ammunitionKindSeen struct {
	Key      string `json:"key"`
	Name     string `json:"name_pt"`
	Quantity int32  `json:"quantity"`
	Spent    int32  `json:"spent"`
}

// RecoverAmmunition implements playv1connect.ChargesServiceHandler: half of what a combat fired, back, once the combat is over.
func (s *Service) RecoverAmmunition(ctx context.Context, req *connect.Request[playv1.RecoverAmmunitionRequest]) (*connect.Response[playv1.RecoverAmmunitionResponse], error) {
	call, err := readChargeCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetCharacterId(), req.Msg.GetIdempotencyKey(), req.Msg)
	if err != nil {
		return nil, err
	}
	var out []*charactersv1.SheetAmmunition
	var repeated bool
	err = s.txOpen(ctx, call.m.CampaignID, func(tx pgx.Tx, h *hints, session playdb.GameSession) error {
		out, repeated = nil, false // a retry starts over
		var err error
		out, repeated, err = s.recoverAmmunitionTx(ctx, tx, h, session, call)
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "recover ammunition", err)
	}
	if !repeated {
		logging.Event(ctx, s.logger, "ammunition.recovered", slog.String("character_id", call.charText), slog.Int("kinds", len(out)))
	}
	return connect.NewResponse(&playv1.RecoverAmmunitionResponse{Ammunition: out}), nil
}

// recoverAmmunitionTx is the transaction of RecoverAmmunition: retry, who may, the combat the pieces were fired in, the keeper's
// recovery, the event.
func (s *Service) recoverAmmunitionTx(ctx context.Context, tx pgx.Tx, h *hints, session playdb.GameSession, call chargeCall) ([]*charactersv1.SheetAmmunition, bool, error) {
	q := s.queries.WithTx(tx)
	done, again, err := eventByKey(ctx, q, session.ID, call.keyText, eventAmmunitionRecovered, call.hash)
	if err != nil {
		return nil, false, err
	}
	if again {
		var ev ammunitionRecoveredEvent
		if err := replayCharacterEvent(done, call.charText, "ammunition recovery", &ev); err != nil {
			return nil, false, err
		}
		h.replay()
		s.tellVitalsAfter(h, call.m.CampaignID, call.charText)
		out := make([]*charactersv1.SheetAmmunition, 0, len(ev.Kinds))
		for _, k := range ev.Kinds {
			out = append(out, &charactersv1.SheetAmmunition{Key: k.Key, NamePt: k.Name, Quantity: k.Quantity, SpentLastCombat: k.Spent, Recovered: true})
		}
		return out, true, nil
	}
	if _, err := s.requireItemCharacter(ctx, tx, call.m, call.charText); err != nil {
		return nil, false, err
	}
	encounters, err := q.ListEncountersOfCharacter(ctx, call.charText)
	if err != nil {
		return nil, false, fmt.Errorf("list the combats of the character: %w", err)
	}
	if len(encounters) == 0 {
		return nil, false, connect.NewError(connect.CodeInvalidArgument, errors.New("the character fought no combat: there is no ammunition to recover"))
	}
	if encounters[0].Status != statusEnded {
		return nil, false, chargesBlocked(playv1.ChargesBlockedReason_CHARGES_BLOCKED_REASON_COMBAT_RUNNING, "the combat is not over: search for the ammunition after it ends", 0)
	}
	out, encounterID, err := s.recoverFromLatest(ctx, tx, call, encounters)
	if err != nil {
		return nil, false, err
	}
	ev := ammunitionRecoveredEvent{Encounter: encounterID}
	for _, a := range out {
		ev.Kinds = append(ev.Kinds, ammunitionKindSeen{Key: a.GetKey(), Name: a.GetNamePt(), Quantity: a.GetQuantity(), Spent: a.GetSpentLastCombat()})
	}
	err = s.appendKeyedEvent(ctx, q, keyedEvent{
		sessionID: session.ID, kind: eventAmmunitionRecovered, userID: call.m.UserID, characterID: &call.charText, key: call.keyText, hash: call.hash,
	}, ev)
	if err != nil {
		return nil, false, err
	}
	s.tellVitalsAfter(h, call.m.CampaignID, call.charText)
	h.touch()
	return out, false, nil
}

// recoverFromLatest takes back the pieces of the character's latest ended combat that fired some (the ones that fired none
// are skipped, newest first). The keeper's refusals become the call's: the pieces were taken back already, or none was fired.
func (s *Service) recoverFromLatest(ctx context.Context, tx pgx.Tx, call chargeCall, encounters []playdb.Encounter) ([]*charactersv1.SheetAmmunition, string, error) {
	for _, enc := range encounters {
		if enc.Status != statusEnded {
			continue
		}
		out, err := s.vitals.RecoverAmmunition(ctx, tx, call.m.CampaignID, call.charText, enc.ID)
		switch {
		case err == nil:
			return out, enc.ID, nil
		case errors.Is(err, link.ErrNothingToRecover):
			continue
		case errors.Is(err, link.ErrAmmunitionRecovered):
			return nil, "", chargesBlocked(playv1.ChargesBlockedReason_CHARGES_BLOCKED_REASON_ALREADY_RECOVERED, "the ammunition of the last combat was already taken back", 0)
		}
		return nil, "", err
	}
	return nil, "", connect.NewError(connect.CodeInvalidArgument, errors.New("the character fired no ammunition in its last combats"))
}
