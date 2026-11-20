package play

import (
	"context"
	"errors"
	"slices"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/play/consumable"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// scrollRun is what the reading leaves for the answer: the event and the spell's name as the reader may read it.
type scrollRun struct {
	ev   consumableUsedEvent
	name string
}

// ReadScroll implements playv1connect.ChargesServiceHandler: the reading of a Spell Scroll (consumable.ScrollRefusal, rules.ScrollRead).
func (s *Service) ReadScroll(ctx context.Context, req *connect.Request[playv1.ReadScrollRequest]) (*connect.Response[playv1.ReadScrollResponse], error) {
	r := req.Msg
	call, err := readChargeCall(ctx, r.GetCampaignId(), r.GetCharacterId(), r.GetIdempotencyKey(), r)
	if err == nil && (call.keyText == "" || !consumable.RollOK(r)) {
		err = connect.NewError(connect.CodeInvalidArgument, errors.New("idempotency_key must be a UUID and the roll the app's or a face from 1 to 20"))
	}
	if err != nil {
		return nil, err
	}
	var run scrollRun
	err = s.txOpen(ctx, call.m.CampaignID, func(tx pgx.Tx, h *hints, session playdb.GameSession) error {
		run = scrollRun{}
		return s.readScrollTx(ctx, tx, h, session, call, r, &run)
	})
	if err != nil {
		return nil, s.dbError(ctx, "read a scroll", err)
	}
	return connect.NewResponse(&playv1.ReadScrollResponse{Reading: consumable.ScrollReading(run.ev, run.name)}), nil
}

func (s *Service) readScrollTx(ctx context.Context, tx pgx.Tx, h *hints, session playdb.GameSession, call chargeCall, r *playv1.ReadScrollRequest, run *scrollRun) error {
	q, use := s.queries.WithTx(tx), consumableCall{chargeCall: call, target: call.charText}
	if _, err := s.mayUseConsumable(ctx, tx, use); err != nil {
		return err
	}
	done, again, err := chargeEventByKey(ctx, q, session.ID, call, consumable.KindScrollRead, eventConsumableUsed)
	if err != nil || again {
		return s.scrollRetry(ctx, tx, h, done, call, run, err)
	}
	if _, err := s.canUseConsumable(ctx, tx, use); err != nil {
		return err
	}
	e, err := s.scrollOf(ctx, tx, call, r.GetEntryId())
	if err != nil {
		return err
	}
	fight, err := s.scrollFight(ctx, tx, q, h, session, call, e)
	if err != nil {
		return err
	}
	d20, typed := 0, false
	if e.CheckNeeded {
		typed = r.GetD20Face() != 0 // only a check has a roll to store
		if d20, err = s.scrollD20(ctx, tx, call.m, r); err != nil {
			return err
		}
	}
	run.ev, run.name = consumableUsedEvent{Entry: e.EntryID, Item: e.ItemKey, Name: e.ItemNamePT}, e.SpellNamePT
	run.ev.Reading(call.charText, e, consumable.CheckOf(e, d20), typed)
	ev := keyedEvent{sessionID: session.ID, kind: consumable.KindScrollRead, userID: call.m.UserID, characterID: &call.charText, key: call.keyText, hash: call.hash}
	if fight != nil {
		fight.name(&run.ev)
		ev.encounterID = &fight.c.enc.ID
	}
	if run.ev.Outcome == consumable.OutcomeScrollFailed { // the scroll comes apart: its unit and the casting time's economy are spent
		ev.kind = eventConsumableUsed
		if err := s.spendFailedScroll(ctx, tx, call, e, fight); err != nil {
			return err
		}
		s.tellVitalsAfter(h, call.m.CampaignID, call.charText)
	}
	if err := s.appendKeyedEvent(ctx, q, ev, run.ev); err != nil {
		return err
	}
	h.touch()
	if fight != nil {
		return fight.finish(ctx, s, h)
	}
	return nil
}

// scrollOf reads the entry's scroll and refuses what cannot be read, whatever the turn.
func (s *Service) scrollOf(ctx context.Context, tx pgx.Tx, call chargeCall, entryID string) (link.ScrollEntry, error) {
	e, err := s.vitals.ScrollOf(ctx, tx, call.m.CampaignID, call.charText, entryID)
	if connect.CodeOf(err) == connect.CodeNotFound {
		return e, errItemGone()
	}
	if err != nil {
		return e, err
	}
	if ref := consumable.ScrollRefusal(e); ref != nil {
		return e, errEncounter(ref.Reason, ref.Text)
	}
	return e, nil
}

// scrollFight opens the combat the reader fights in, nil when it fights in none. In a running combat the read needs the reader's turn
// and the economy of the spell's casting time (the master has the last word on the economy); a combat in setup has no turn.
func (s *Service) scrollFight(ctx context.Context, tx pgx.Tx, q *playdb.Queries, h *hints, session playdb.GameSession, call chargeCall, e link.ScrollEntry) (*consumableFight, error) {
	enc, cs, found, err := openCombatants(ctx, q, session.ID)
	i := slices.IndexFunc(cs, func(c playdb.Combatant) bool { return c.CharacterID == call.charText && !c.Dismissed })
	if err != nil || !found || i < 0 {
		return nil, err
	}
	c, err := s.openTx(ctx, combatTx{tx: tx, q: q, session: session, enc: enc, now: s.now(), hints: h, actorUserID: call.m.UserID, svc: s})
	if err != nil {
		return nil, err
	}
	if enc.Status == statusActive {
		if err := s.mustActNow(ctx, c, cs[i]); err != nil {
			return nil, err
		}
		if ref := consumable.EconomyRefusal(consumable.TurnOf(cs[i]), e, call.m.Role == authz.RoleMaster); ref != nil {
			return nil, errEncounter(ref.Reason, ref.Text)
		}
	}
	return &consumableFight{c: c, cs: cs, giver: &cs[i]}, nil
}

// scrollD20 is the d20 of the check: the app's, or the face typed from a real die when the table's rule lets the reader roll so (RN-18).
func (s *Service) scrollD20(ctx context.Context, tx pgx.Tx, m authz.Membership, r *playv1.ReadScrollRequest) (int, error) {
	if r.GetRoll() == nil {
		return 0, connect.NewError(connect.CodeInvalidArgument, errors.New("the check needs the app's roll or a d20 face"))
	}
	if err := s.mustRollThisWay(ctx, tx, m, rollInput{inApp: r.GetRollInApp(), typed: int(r.GetD20Face())}); err != nil {
		return 0, err
	}
	if r.GetRollInApp() {
		return s.roller.Roll(20)
	}
	return int(r.GetD20Face()), nil
}

// spendFailedScroll takes the unit off the sheet and, in a running combat, spends the economy of the casting time.
func (s *Service) spendFailedScroll(ctx context.Context, tx pgx.Tx, call chargeCall, e link.ScrollEntry, fight *consumableFight) error {
	if _, err := s.vitals.RemoveConsumable(ctx, tx, call.m.CampaignID, call.charText, e.EntryID); err != nil {
		return err
	}
	if fight == nil || fight.c.enc.Status != statusActive || !consumable.Spend(fight.giver, e.Economy) {
		return nil
	}
	g := fight.giver
	return fight.c.q.SetCombatantEconomy(ctx, playdb.SetCombatantEconomyParams{ID: g.ID, ActionUsed: g.ActionUsed, BonusActionUsed: g.BonusActionUsed, ReactionUsed: g.ReactionUsed, Dashed: g.Dashed})
}

// scrollRetry answers the retry of a key found stored with what the first call answered (the first call's error, when the key is
// another call's).
func (s *Service) scrollRetry(ctx context.Context, tx pgx.Tx, h *hints, done playdb.GetSessionEventByIdempotencyKeyRow, call chargeCall, run *scrollRun, err error) error {
	if err == nil {
		err = replayCharacterEvent(done, call.charText, "scroll", &run.ev)
	}
	if err == nil && run.ev.Reader != call.charText {
		err = errKeyReused()
	}
	content, cerr := s.roster.RulesContent(ctx, tx, call.m.CampaignID)
	if err != nil || cerr != nil {
		return errors.Join(err, cerr)
	}
	if run.name = content.NamePT(run.ev.Spell); call.m.Role != authz.RoleMaster && content.Hidden(run.ev.Spell) {
		run.name = ""
	}
	h.replay()
	return nil
}
