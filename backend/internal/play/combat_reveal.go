package play

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/platform/tablerules"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// Hidden creatures an area spell hits (RN-10, RN-20, table rule
// "Criaturas escondidas atingidas por uma área"). The SRD gives an area's effect to
// every creature in it and says nothing of what that does to hiding (its text on
// unseen creatures is about attacks), so the table decides, in campaign_table_rules:
//
//   - reveal (the default): the creature appears to the players, as when the master
//     reveals it;
//   - keep_hidden: it takes the effect and stays hidden;
//   - ask: a player's spell holds the turn (hidden_reveals) until the master answers
//     with ResolveHiddenReveal. The master's own spell asks nothing when the request
//     carries his choice (CastSpellRequest.reveal_hidden).
//
// Until a creature is revealed nothing a player receives names or counts it: the
// cast's answer, its line of the log, the stream. The pending question is the
// master's alone, and the hold on the turn reads the same to a player as any other
// wait for the master.

// errOnlyMasterReveals is the refusal of a player who sends what only the master
// may: the same for every request, so that it tells nothing of what is open.
func errOnlyMasterReveals() error {
	return connect.NewError(connect.CodePermissionDenied, errors.New("only the master decides whether hidden creatures are revealed"))
}

// revealDecision says what a cast does to the hidden creatures it hit: reveal them,
// or ask the master. Neither means they stay hidden. The master's own choice wins;
// without it the table's rule decides, and a rule that asks makes the cast hold the
// turn, whoever casts.
func revealDecision(rule tablerules.HiddenAreaHitRule, choice *bool) (reveal, ask bool) {
	if choice != nil {
		return *choice, false
	}
	switch rule {
	case tablerules.HiddenAreaHitsKeepHidden:
		return false, false
	case tablerules.HiddenAreaHitsAsk:
		return false, true
	}
	return true, false
}

// settleHidden is what a placed cast does to the hiding of the hidden creatures it
// hit: it marks them in the cast's event (the players' view never lists them) and
// reveals them, keeps them or opens the master's question. It runs inside the cast's
// transaction, before the cast's own event is written.
func (s *Service) settleHidden(ctx context.Context, c *combatTx, m authz.Membership, v combatViewer, plan areaPlan, spellKey string, choice *bool, made *actionEvent) error {
	var hit []string
	for i := range made.Hits {
		if j := slices.IndexFunc(plan.targets, func(t areaTarget) bool { return t.who.ID == made.Hits[i].Target }); j >= 0 && plan.targets[j].who.Hidden {
			made.Hits[i].HiddenAtCast = true
			hit = append(hit, made.Hits[i].Target)
		}
	}
	if len(hit) == 0 {
		return nil
	}
	if !v.master {
		choice = nil // a player never chooses (a request that tried was refused at its start)
	}
	reveal, ask := revealDecision(c.rules.HiddenAreaHits, choice)
	switch {
	case ask:
		seq, err := c.q.NextHiddenRevealSeq(ctx, c.enc.ID)
		if err != nil {
			return fmt.Errorf("number the hidden reveal: %w", err)
		}
		flat := make([]int32, 0, 2*len(plan.squares))
		for _, sq := range plan.squares {
			flat = append(flat, clamp32(sq.Col, 0, 1<<30), clamp32(sq.Row, 0, 1<<30))
		}
		q, err := c.q.InsertHiddenReveal(ctx, playdb.InsertHiddenRevealParams{
			EncounterID: c.enc.ID, CasterID: made.Actor, SpellKey: spellKey, CombatantIds: hit,
			OriginCol: clamp32(plan.origin.Col, 0, 1<<30), OriginRow: clamp32(plan.origin.Row, 0, 1<<30), Squares: flat, Seq: seq, CreatedAt: c.now,
		})
		if err != nil {
			return fmt.Errorf("open the hidden reveal: %w", err)
		}
		made.PendingReveal = q.ID
	case reveal:
		if err := s.revealCombatants(ctx, c, m, hit); err != nil {
			return err
		}
		made.Revealed = hit
	}
	return nil
}

// revealCombatants makes hidden creatures appear to the players, with the line of
// the log an area's reveal has ("foi revelado"). A creature that is not hidden any
// more, or left the combat, is left alone.
func (s *Service) revealCombatants(ctx context.Context, c *combatTx, m authz.Membership, ids []string) error {
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the combatants: %w", err)
	}
	for _, id := range ids {
		i := slices.IndexFunc(cs, func(o playdb.Combatant) bool { return o.ID == id })
		if i < 0 || !cs[i].Hidden {
			continue
		}
		if err := c.q.SetCombatantHidden(ctx, playdb.SetCombatantHiddenParams{ID: id, Hidden: false}); err != nil {
			return fmt.Errorf("reveal the combatant: %w", err)
		}
		if err := insertEvent(ctx, c, eventCombatantHiddenSet, &m.UserID, nil, actionEvent{Round: c.enc.Round, Actor: id, ByArea: true}); err != nil {
			return err
		}
	}
	return nil
}

// mustNotHold refuses what the turn cannot do while a question waits for the
// master: it is the same for everybody, the master's turn passing included. The
// reason is the one a wait for the master has (HIDDEN_REVEAL_PENDING); what the
// player's screen says is "Esperando o mestre" and nothing of why.
func (s *Service) mustNotHold(ctx context.Context, c *combatTx) error {
	if c.enc.ID == "" || c.enc.Status != statusActive {
		return nil
	}
	pending, err := c.q.ListPendingHiddenReveals(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the hidden reveals: %w", err)
	}
	if len(pending) > 0 {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_HIDDEN_REVEAL_PENDING, "the turn waits for the master")
	}
	// A saving throw a zone asked also holds the turn, until its answer (zones_save.go).
	return s.mustNotWaitForZone(ctx, c)
}

// ResolveHiddenReveal implements playv1connect.CombatServiceHandler.
func (s *Service) ResolveHiddenReveal(
	ctx context.Context,
	req *connect.Request[playv1.ResolveHiddenRevealRequest],
) (*connect.Response[playv1.ResolveHiddenRevealResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	// Before the ids are looked at: a player gets this for any id, one that names an
	// open question and one that does not, or the answer would say whether a
	// player's spell hit a hidden creature (RN-10).
	if m.Role != authz.RoleMaster {
		return nil, errOnlyMasterReveals()
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	encID, err := parseCombatID(req.Msg.GetEncounterId(), "encounter")
	if err != nil {
		return nil, err
	}
	askID, err := uuid.Parse(req.Msg.GetPendingRevealId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("pending_reveal_id must be a UUID"))
	}
	answer, other := hiddenRevealKept, hiddenRevealRevealed
	if req.Msg.GetReveal() {
		answer, other = hiddenRevealRevealed, hiddenRevealKept
	}

	var revealed []string
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventHiddenRevealAnswered, encounterID: encID}, func(c *combatTx) (any, error) {
		revealed = nil
		q, err := c.q.GetHiddenReveal(ctx, playdb.GetHiddenRevealParams{EncounterID: c.enc.ID, ID: askID.String()})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, connect.NewError(connect.CodeNotFound, errors.New("hidden reveal not found"))
			}
			return nil, fmt.Errorf("find the hidden reveal: %w", err)
		}
		if q.State == other {
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("the question was answered the other way already"))
		}
		made := actionEvent{Round: c.enc.Round, Actor: q.CasterID, PendingReveal: q.ID, NowHidden: answer == hiddenRevealKept}
		if q.State == answer { // the same answer again: nothing changes
			return made, nil
		}
		pending, err := c.q.ListPendingHiddenReveals(ctx, c.enc.ID)
		if err != nil {
			return nil, fmt.Errorf("list the hidden reveals: %w", err)
		}
		if len(pending) == 0 || pending[0].ID != q.ID {
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("answer the oldest question first"))
		}
		if answer == hiddenRevealRevealed {
			if err := s.revealCombatants(ctx, c, m, q.CombatantIds); err != nil {
				return nil, err
			}
			revealed = q.CombatantIds
			made.Revealed = q.CombatantIds
		}
		if _, err := c.q.AnswerHiddenReveal(ctx, playdb.AnswerHiddenRevealParams{EncounterID: c.enc.ID, ID: q.ID, State: answer, AnsweredAt: &c.now}); err != nil {
			return nil, fmt.Errorf("answer the hidden reveal: %w", err)
		}
		if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		return made, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "answer a hidden reveal", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		if len(revealed) > 0 {
			s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, true)
			if anyInTurn(d, revealed) {
				s.publishTurnChanged(ctx, m.CampaignID, d)
			}
		}
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.ResolveHiddenRevealResponse{Encounter: out}), nil
}

// anyInTurn says whether one of the combatants is in the group on turn.
func anyInTurn(d *encounterData, ids []string) bool {
	return slices.ContainsFunc(d.cs, func(o playdb.Combatant) bool { return slices.Contains(ids, o.ID) && inTurn(d.enc, o) })
}

// The states of a hidden reveal (hidden_reveals.state).
const (
	hiddenRevealPending  = "pending"
	hiddenRevealRevealed = "revealed"
	hiddenRevealKept     = "kept"
)

// hiddenRevealsView is the questions the viewer is told of: the master's all, in the
// order they are answered; a player's none, and nothing says there were any.
func (s *Service) hiddenRevealsView(ctx context.Context, d *encounterData, v combatViewer) ([]*playv1.HiddenRevealQuestion, bool, error) {
	if d.enc.Status != statusActive {
		return nil, false, nil
	}
	pending, err := s.queries.ListPendingHiddenReveals(ctx, d.enc.ID)
	if err != nil {
		return nil, false, s.dbError(ctx, "list the hidden reveals", err)
	}
	if !v.master {
		return nil, len(pending) > 0, nil
	}
	var out []*playv1.HiddenRevealQuestion
	for _, q := range pending {
		ids := slices.DeleteFunc(slices.Clone(q.CombatantIds), func(id string) bool {
			return !slices.ContainsFunc(d.cs, func(o playdb.Combatant) bool { return o.ID == id })
		})
		area := &playv1.SpellArea{Origin: &playv1.SpellOrigin{Col: q.OriginCol, Row: q.OriginRow}}
		for i := 0; i+1 < len(q.Squares); i += 2 {
			area.Squares = append(area.Squares, &playv1.SpellOrigin{Col: q.Squares[i], Row: q.Squares[i+1]})
		}
		out = append(out, &playv1.HiddenRevealQuestion{Id: q.ID, CasterId: q.CasterID, SpellKey: q.SpellKey, CombatantIds: ids, Area: area})
	}
	return out, len(out) > 0, nil
}

// publishHiddenHitPending tells the master a question waits: the master's stream
// alone, and it names nothing (he reads the question from the combat).
func (s *Service) publishHiddenHitPending(campaignID, encounterID, questionID string) {
	s.hub.Publish(campaignID, live.Event{Audience: live.Audience{Master: true}, Message: &playv1.WatchGameSessionResponse{
		Event: &playv1.WatchGameSessionResponse_HiddenHitPending_{
			HiddenHitPending: &playv1.WatchGameSessionResponse_HiddenHitPending{EncounterId: encounterID, PendingRevealId: questionID},
		},
	}})
}
