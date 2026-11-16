package play

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
	"github.com/PuraFome/meuRPG/backend/internal/play/advantage"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// The refund of the Inspiration (RN-43): a roll spent it and it added nothing over every source of advantage (the sources the roller may
// not read already gave it). The roll resolves at once, and the master is asked whether to give the Inspiration back. The answer "give
// back" goes through the one refund path (hold_forget.go: refundSpentFavor, which keeps the one-row rule); "keep" leaves it spent. The
// player reads only the refund, and never why. An unanswered prompt keeps it spent and closes as kept when the session ends.

// The statuses of favor_refund_prompts.
const (
	refundOpen     = "open"
	refundReturned = "returned"
	refundKept     = "kept"
)

// refundLabels name a roll by its kind, as the prompt reads it.
var refundLabels = map[heldKind]string{
	heldWeaponAttack: "Ataque", heldSpellAttack: "Ataque mágico", heldEffectSave: "Salvaguarda de efeito", heldConcentrationSave: "Salvaguarda de concentração",
	heldSceneCheck: "Teste de perícia", heldGroupCheck: "Teste em grupo", heldTrapSearch: "Busca por armadilhas", heldHintTry: "Tentativa de dica",
	heldHide: "Esconder-se", heldContest: "Disputa",
}

// hiddenSourceJSON is a source the roller could not read, as the prompt keeps it.
type hiddenSourceJSON struct {
	Kind      string `json:"kind"`
	Effect    int    `json:"effect"`
	Condition string `json:"condition,omitempty"`
	Against   string `json:"against,omitempty"`
}

// openRefundPrompt opens the master's question, in the roll's transaction: the row, the event (favor_refund_prompted, the master's) and
// the master's hint. It returns the prompt's id, which the roll's event and its hold remember. It moves no number by itself: the roll's write
// moves it.
func (s *Service) openRefundPrompt(ctx context.Context, env gateEnv, a favorAsk, r *refundAsk) (string, error) {
	open, err := env.q.GetOpenGameSession(ctx, a.CampaignID)
	if err != nil {
		return "", fmt.Errorf("find the open session: %w", err)
	}
	hidden := make([]hiddenSourceJSON, 0, len(r.Hidden))
	for _, src := range r.Hidden {
		hidden = append(hidden, hiddenSourceJSON{Kind: src.Kind, Effect: int(src.Effect), Condition: src.Condition, Against: src.Against})
	}
	body, err := json.Marshal(hidden)
	if err != nil {
		return "", fmt.Errorf("encode the hidden sources: %w", err)
	}
	prompt, err := env.q.InsertFavorRefundPrompt(ctx, playdb.InsertFavorRefundPromptParams{
		CampaignID: a.CampaignID, GameSessionID: open.ID, CharacterID: a.CharacterID, RollLabelPt: refundLabels[a.Kind], HiddenSources: body,
	})
	if err != nil {
		return "", fmt.Errorf("open the refund prompt: %w", err)
	}
	if err := s.writeRefundEvent(ctx, env.q, open.ID, nil, eventFavorRefundPrompted, prompt, nil); err != nil {
		return "", err
	}
	s.tellRefundMaster(ctx, env.tx, env.h, a.CampaignID, a.CharacterID)
	return prompt.ID, nil
}

// tellRefundMaster moves the session's number and tells the master alone, with the roller's vitals hint, to read GetFavor again: a prompt
// opened or closed. The roller's player is never told: they read nothing of a question the master answers.
func (s *Service) tellRefundMaster(ctx context.Context, tx pgx.Tx, h *hints, campaignID, characterID string) {
	if h == nil {
		return
	}
	h.touch()
	if v, err := s.vitals.GetVitalsTx(ctx, tx, campaignID, characterID); err == nil {
		h.tell(campaignID, live.Event{Audience: live.Audience{Master: true}, Message: vitalsHint(v).Message})
	}
}

// writeRefundEvent writes an event of a prompt: ids only. A refund written in a combat carries the combat (a line of its log).
func (s *Service) writeRefundEvent(ctx context.Context, q *playdb.Queries, sessionID string, actor *string, kind string, p playdb.FavorRefundPrompt, st *favorInCombat) error {
	body := any(favorRefundEvent{PromptID: p.ID, CharacterID: p.CharacterID})
	if st != nil {
		body = favorRefundCombatEvent{favorRefundEvent{PromptID: p.ID, CharacterID: p.CharacterID}, *st}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode the event payload: %w", err)
	}
	seq, err := q.NextSessionEventSeq(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("next event number: %w", err)
	}
	char := p.CharacterID
	params := playdb.InsertSessionEventParams{GameSessionID: sessionID, Seq: seq, Kind: kind, ActorUserID: actor, CharacterID: &char, Payload: raw, CreatedAt: s.now()}
	if st != nil {
		params.EncounterID = &st.EncounterID
	}
	if _, err := insertSessionEvent(ctx, q, params); err != nil {
		return fmt.Errorf("insert session event: %w", err)
	}
	return nil
}

// readRefundPrompts are the master's open questions, with the sources the roller could not read as the master reads them.
func (s *Service) readRefundPrompts(ctx context.Context, m authz.Membership) ([]*playv1.FavorRefundPrompt, error) {
	rows, err := s.queries.ListOpenFavorRefundPrompts(ctx, m.CampaignID)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	return s.refundPromptViews(ctx, m.CampaignID, rows)
}

// refundPromptViews names the prompts for the master.
func (s *Service) refundPromptViews(ctx context.Context, campaignID string, rows []playdb.FavorRefundPrompt) ([]*playv1.FavorRefundPrompt, error) {
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.CharacterID
	}
	chars, err := s.roster.SessionCharacters(ctx, nil, campaignID, ids)
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(chars))
	for _, c := range chars {
		names[c.ID] = c.Name
	}
	text := advantage.TextContext{Names: s.namesFor(ctx, campaignID)}
	out := make([]*playv1.FavorRefundPrompt, 0, len(rows))
	for _, r := range rows {
		var hidden []hiddenSourceJSON
		if err := json.Unmarshal(r.HiddenSources, &hidden); err != nil {
			return nil, fmt.Errorf("read the hidden sources: %w", err)
		}
		view := &playv1.FavorRefundPrompt{Id: r.ID, CharacterId: r.CharacterID, CharacterName: names[r.CharacterID], RollLabelPt: r.RollLabelPt, Status: refundStatusProto(r.Status)}
		for _, h := range hidden {
			view.HiddenSources = append(view.HiddenSources, advantage.SourceProto(combat.Source{Kind: h.Kind, Effect: combat.RollMode(h.Effect), Condition: h.Condition, Against: h.Against}, text, nil))
		}
		out = append(out, view)
	}
	return out, nil
}

func refundStatusProto(status string) playv1.FavorRefundStatus {
	switch status {
	case refundOpen:
		return playv1.FavorRefundStatus_FAVOR_REFUND_STATUS_OPEN
	case refundReturned:
		return playv1.FavorRefundStatus_FAVOR_REFUND_STATUS_RETURNED
	case refundKept:
		return playv1.FavorRefundStatus_FAVOR_REFUND_STATUS_KEPT
	}
	return playv1.FavorRefundStatus_FAVOR_REFUND_STATUS_UNSPECIFIED
}

// refundCall is what AnswerFavorRefund reads before the transaction.
type refundCall struct {
	m         authz.Membership
	promptID  string
	giveBack  bool
	key       string
	hash      *string
	sessionID string
}

// AnswerFavorRefund implements playv1connect.FavorServiceHandler: the master gives the Inspiration back or keeps it spent.
func (s *Service) AnswerFavorRefund(ctx context.Context, req *connect.Request[playv1.AnswerFavorRefundRequest]) (*connect.Response[playv1.AnswerFavorRefundResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	id, ok := uid.Canonical(req.Msg.GetPromptId())
	if !ok {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("prompt_id must be a UUID"))
	}
	row, err := s.queries.GetFavorRefundPrompt(ctx, playdb.GetFavorRefundPromptParams{ID: id, CampaignID: m.CampaignID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("the question was not found"))
	}
	if err != nil {
		return nil, s.dbError(ctx, "find the refund question", err)
	}
	call := refundCall{m: m, promptID: id, giveBack: req.Msg.GetGiveBack(), key: key, hash: idem.Hash(req.Msg), sessionID: row.GameSessionID}
	var out playdb.FavorRefundPrompt
	err = s.tx(ctx, call.sessionID, func(tx pgx.Tx, h *hints) (err error) {
		out, err = s.answerRefundIn(ctx, tx, h, call)
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "answer the refund question", err)
	}
	views, err := s.refundPromptViews(ctx, m.CampaignID, []playdb.FavorRefundPrompt{out})
	if err != nil {
		return nil, s.dbError(ctx, "name the refund question", err)
	}
	return connect.NewResponse(&playv1.AnswerFavorRefundResponse{Prompt: views[0]}), nil
}

// answerRefundIn is the transaction of the answer: a retry of the key answers the stored outcome (and tells again what the first call
// told), a closed question is PASS_CLOSED (the closest reason: it was answered or ended), and the Inspiration goes back through the one
// refund path, which refuses a roller that has one again.
func (s *Service) answerRefundIn(ctx context.Context, tx pgx.Tx, h *hints, c refundCall) (playdb.FavorRefundPrompt, error) {
	q := s.queries.WithTx(tx)
	kind := eventFavorRefundKept
	if c.giveBack {
		kind = eventFavorRefunded
	}
	done, again, err := eventByKey(ctx, q, c.sessionID, c.key, kind, c.hash)
	if err != nil {
		return playdb.FavorRefundPrompt{}, err
	}
	if again {
		h.replay()
		return s.replayRefund(ctx, tx, h, c, done.Payload)
	}
	closed, err := closeRefundPrompt(ctx, q, c)
	if err != nil {
		return playdb.FavorRefundPrompt{}, err
	}
	if !c.giveBack {
		s.tellRefundMaster(ctx, tx, h, closed.CampaignID, closed.CharacterID)
		return closed, s.writeKeyedRefundEvent(ctx, q, c, kind, closed, nil)
	}
	return closed, s.returnRefund(ctx, tx, h, c, closed)
}

// closeRefundPrompt answers the question that is still open, and refuses one that is not.
func closeRefundPrompt(ctx context.Context, q *playdb.Queries, c refundCall) (playdb.FavorRefundPrompt, error) {
	prompt, err := q.GetFavorRefundPrompt(ctx, playdb.GetFavorRefundPromptParams{ID: c.promptID, CampaignID: c.m.CampaignID})
	if err != nil {
		return playdb.FavorRefundPrompt{}, fmt.Errorf("read the question again: %w", err)
	}
	if prompt.Status != refundOpen {
		return playdb.FavorRefundPrompt{}, errFavorPassClosed()
	}
	status := refundKept
	if c.giveBack {
		status = refundReturned
	}
	closed, err := q.AnswerFavorRefundPrompt(ctx, playdb.AnswerFavorRefundPromptParams{Status: status, AnswerKey: &c.key, ID: prompt.ID, CampaignID: prompt.CampaignID})
	if errors.Is(err, pgx.ErrNoRows) {
		return playdb.FavorRefundPrompt{}, errFavorPassClosed()
	}
	if isUniqueViolation(err) { // the answer's key was used in another session of the campaign: the retry check reads the open session's events only
		return playdb.FavorRefundPrompt{}, errKeyReused()
	}
	if err != nil {
		return playdb.FavorRefundPrompt{}, fmt.Errorf("answer the question: %w", err)
	}
	return closed, nil
}

// returnRefund gives the Inspiration back: the roller must have none now (the spend is still unrefunded; ALREADY_HAS otherwise, and the
// answer rolls back with the question still open), then the line of the combat's log when the refund is one, the event and the hints.
func (s *Service) returnRefund(ctx context.Context, tx pgx.Tx, h *hints, c refundCall, closed playdb.FavorRefundPrompt) error {
	q := s.queries.WithTx(tx)
	if err := s.mayHold(ctx, tx, closed.CampaignID, closed.CharacterID, errCannotReceive()); err != nil { // the prompt stays open for a "keep"
		return err
	}
	if err := refundSpentFavor(ctx, q, closed.CampaignID, closed.CharacterID); err != nil {
		return err
	}
	st, inCombat, err := s.favorStage(ctx, q, c.sessionID, closed.CharacterID)
	if err != nil {
		return err
	}
	var stage *favorInCombat
	if inCombat {
		stage = &st
		tellFavorLog(h, closed.CampaignID, st)
	}
	if err := s.writeKeyedRefundEvent(ctx, q, c, eventFavorRefunded, closed, stage); err != nil {
		return err
	}
	return s.tellRefund(ctx, tx, h, closed)
}

// writeKeyedRefundEvent writes the answer's event under its key.
func (s *Service) writeKeyedRefundEvent(ctx context.Context, q *playdb.Queries, c refundCall, kind string, p playdb.FavorRefundPrompt, st *favorInCombat) error {
	ev := keyedEvent{sessionID: c.sessionID, kind: kind, userID: c.m.UserID, key: c.key, hash: c.hash, characterID: &p.CharacterID}
	body := any(favorRefundEvent{PromptID: p.ID, CharacterID: p.CharacterID})
	if st != nil {
		ev.encounterID = &st.EncounterID
		body = favorRefundCombatEvent{favorRefundEvent{PromptID: p.ID, CharacterID: p.CharacterID}, *st}
	}
	return s.appendKeyedEvent(ctx, q, ev, body)
}

// replayRefund answers the retry of an answer: the prompt as it is now, and what the first call told again (the roller's vitals hint when
// it was a refund, the master's when the Inspiration was kept), with no number moved.
func (s *Service) replayRefund(ctx context.Context, tx pgx.Tx, h *hints, c refundCall, payload []byte) (playdb.FavorRefundPrompt, error) {
	var made favorRefundEvent
	if err := json.Unmarshal(payload, &made); err != nil {
		return playdb.FavorRefundPrompt{}, fmt.Errorf("read the refund event: %w", err)
	}
	p, err := s.queries.WithTx(tx).GetFavorRefundPrompt(ctx, playdb.GetFavorRefundPromptParams{ID: made.PromptID, CampaignID: c.m.CampaignID})
	if err != nil {
		return playdb.FavorRefundPrompt{}, fmt.Errorf("find the question: %w", err)
	}
	if c.giveBack {
		return p, s.tellRefund(ctx, tx, h, p)
	}
	s.tellRefundMaster(ctx, tx, h, p.CampaignID, p.CharacterID)
	return p, nil
}

// tellRefund collects the roller's vitals hint (the master and the roller's player read GetFavor again).
func (s *Service) tellRefund(ctx context.Context, tx pgx.Tx, h *hints, p playdb.FavorRefundPrompt) error {
	h.touch()
	found, err := s.roster.SessionCharacters(ctx, tx, p.CampaignID, []string{p.CharacterID})
	if err != nil || len(found) != 1 {
		return err
	}
	return s.tellFavor(ctx, tx, h, p.CampaignID, found[0])
}

// closeRefundPromptsOfSession keeps spent the Inspiration of every prompt still open when the session ends: each writes favor_refund_kept
// (the master's hand, no key). Nothing is told; the end of the session is.
func (s *Service) closeRefundPromptsOfSession(ctx context.Context, tx pgx.Tx, m authz.Membership, sessionID string) error {
	q := s.queries.WithTx(tx)
	closed, err := q.CloseFavorRefundPromptsOfSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("keep the open refund questions: %w", err)
	}
	for _, p := range closed {
		if err := s.writeRefundEvent(ctx, q, sessionID, &m.UserID, eventFavorRefundKept, playdb.FavorRefundPrompt(p), nil); err != nil {
			return err
		}
	}
	return nil
}
