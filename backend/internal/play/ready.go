package play

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// Ready (SRD 5.1, "Ready"; ReadyService, ready.proto). On its turn a combatant spends its action to hold another until a
// trigger the player names in their own words: the row (readied_actions) waits until the master fires it, the owner gives
// it up (CancelReadied) or the readier's next turn starts (it is lost; ready_turn.go). The trigger and the held action are
// the master's and the readier's player's; every other player reads that the combatant readied an action (RN-10).
// ReadyAction runs on a takeActionFlow (combat_take_action.go) for the checks and the spend of the action Ready shares with
// TakeAction, and adds its own (ready_flow.go).

// The events of a readied action.
const (
	eventActionReadied = "action_readied"
	eventReadiedFired  = "readied_fired"
	eventReadiedLost   = "readied_lost"
)

// actionReady is the standard action Ready: ReadyAction takes it, TakeAction refuses it.
const actionReady = "standard:ready"

// The kinds of action a readied_actions row holds.
const (
	readiedAttack = "attack"
	readiedSpell  = "spell"
	readiedMove   = "move"
	readiedOther  = "other"
)

// The statuses of a readied_actions row: it waits, or how it ended.
const (
	readiedWaiting  = "waiting"
	readiedFired    = "fired"
	readiedIgnored  = "ignored"
	readiedLost     = "lost"
	readiedBroken   = "broken"
	readiedCanceled = "canceled"
)

var readiedKinds = map[playv1.ReadiedKind]string{
	playv1.ReadiedKind_READIED_KIND_ATTACK: readiedAttack,
	playv1.ReadiedKind_READIED_KIND_SPELL:  readiedSpell,
	playv1.ReadiedKind_READIED_KIND_MOVE:   readiedMove,
	playv1.ReadiedKind_READIED_KIND_OTHER:  readiedOther,
}

var readiedKindToProto = map[string]playv1.ReadiedKind{
	readiedAttack: playv1.ReadiedKind_READIED_KIND_ATTACK,
	readiedSpell:  playv1.ReadiedKind_READIED_KIND_SPELL,
	readiedMove:   playv1.ReadiedKind_READIED_KIND_MOVE,
	readiedOther:  playv1.ReadiedKind_READIED_KIND_OTHER,
}

var readiedStatusToProto = map[string]playv1.ReadiedStatus{
	readiedWaiting:  playv1.ReadiedStatus_READIED_STATUS_WAITING,
	readiedFired:    playv1.ReadiedStatus_READIED_STATUS_FIRED,
	readiedIgnored:  playv1.ReadiedStatus_READIED_STATUS_IGNORED,
	readiedLost:     playv1.ReadiedStatus_READIED_STATUS_LOST,
	readiedBroken:   playv1.ReadiedStatus_READIED_STATUS_BROKEN,
	readiedCanceled: playv1.ReadiedStatus_READIED_STATUS_CANCELED,
}

// readiedEvent is the readied action in the payload of action_readied, readied_fired and readied_lost: the row and what it
// holds, which the log reads without the row (the undo deletes it, the end of the combat drops it).
type readiedEvent struct {
	ID        string `json:"id"`
	Trigger   string `json:"trigger,omitempty"`
	Kind      string `json:"kind,omitempty"`
	ActionKey string `json:"action_key,omitempty"`
	// Name is the held action's name in Portuguese when it was readied.
	Name   string `json:"name,omitempty"`
	MoveFt int32  `json:"move_ft,omitempty"`
	Target string `json:"target_id,omitempty"`
	// Canceled says a readied_lost was the owner's CancelReadied, not the start of the next turn.
	Canceled bool `json:"canceled,omitempty"`
	// Defeated says a readied_lost came from the readier's defeat: it is written before the event of the change that
	// defeated it, and the undo of that change goes past it (chainReadiedDefeat).
	Defeated bool `json:"defeated,omitempty"`
	// Dismissed says a readied_lost came from the readier leaving the order (a creature dismissed): as Defeated.
	Dismissed bool `json:"dismissed,omitempty"`
	// Window is the READIED window the master opened (FireReadied). Opened says the readied_fired event is the fire
	// itself, which writes no line; Ended is how the window's answer ended the row ("fired" or "ignored").
	Window string `json:"window,omitempty"`
	Opened bool   `json:"opened,omitempty"`
	Ended  string `json:"ended,omitempty"`
	// Line is the readied_fired line a use wrote before the event of the action it ran: the undo of the action takes it
	// back (linesBefore).
	Line string `json:"line,omitempty"`
}

// readiedEventOf is the event's copy of a row, with the name the held action had when it was readied.
func readiedEventOf(row playdb.ReadiedAction) *readiedEvent {
	var key, target string
	if row.ActionKey != nil {
		key = *row.ActionKey
	}
	if row.TargetID != nil {
		target = *row.TargetID
	}
	return &readiedEvent{ID: row.ID, Trigger: row.TriggerText, Kind: row.Kind, ActionKey: key, Name: row.ActionNamePt, MoveFt: row.MoveFt, Target: target}
}

// ReadyAction implements playv1connect.ReadyServiceHandler.
func (s *Service) ReadyAction(
	ctx context.Context,
	req *connect.Request[playv1.ReadyActionRequest],
) (*connect.Response[playv1.ReadyActionResponse], error) {
	m, key, encID, err := memberCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetIdempotencyKey(), req.Msg.GetEncounterId())
	if err != nil {
		return nil, err
	}
	call, err := readReadyCall(req.Msg, key, encID)
	if err != nil {
		return nil, err
	}
	var made actionEvent
	var vitals []*playv1.CharacterVitals
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventActionReadied, encounterID: encID}, func(c *combatTx) (any, error) {
		f := newReadyFlow(s, c, m, call)
		ev, err := f.run(ctx)
		made, vitals = ev, f.vitals
		return ev, err
	})
	if err != nil {
		return nil, s.dbError(ctx, "ready an action", err)
	}
	enc, err := s.readiedAnswer(ctx, m, res, made, vitals...)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.ReadyActionResponse{Encounter: enc}), nil
}

// CancelReadied implements playv1connect.ReadyServiceHandler.
func (s *Service) CancelReadied(
	ctx context.Context,
	req *connect.Request[playv1.CancelReadiedRequest],
) (*connect.Response[playv1.CancelReadiedResponse], error) {
	m, key, encID, err := memberCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetIdempotencyKey(), req.Msg.GetEncounterId())
	if err != nil {
		return nil, err
	}
	id, err := parseCombatID(req.Msg.GetReadiedId(), "readied action")
	if err != nil {
		return nil, err
	}
	var made actionEvent
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventReadiedLost, encounterID: encID}, func(c *combatTx) (any, error) {
		ev, err := s.cancelReadiedTx(ctx, c, m, id)
		made = ev
		return ev, err
	})
	if err != nil {
		return nil, s.dbError(ctx, "give up a readied action", err)
	}
	enc, err := s.readiedAnswer(ctx, m, res, made)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.CancelReadiedResponse{Encounter: enc}), nil
}

// readiedAnswer tells the table after the commit (the combat changed, and its log: a hidden readier's line is the
// master's alone, and the vitals of what the action spent: the slot a readied spell took) and answers the encounter as the caller
// sees it.
func (s *Service) readiedAnswer(ctx context.Context, m authz.Membership, res combatResult, made actionEvent, vitals ...*playv1.CharacterVitals) (*playv1.Encounter, error) {
	ev, err := resultEvent(res, made)
	if err != nil {
		return nil, s.dbError(ctx, "read the readied action", err)
	}
	return s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !ev.Secret)
		for _, v := range vitals {
			s.publishVitals(ctx, m.CampaignID, v)
		}
	})
}

// errReadiedNotFound is the answer for a readied action that does not wait, or that the caller may not touch: the same
// for both (RN-10).
func errReadiedNotFound() error {
	return connect.NewError(connect.CodeNotFound, errors.New("readied action not found"))
}
