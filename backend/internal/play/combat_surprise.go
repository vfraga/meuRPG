package play

import (
	"context"
	"fmt"
	"math"
	"slices"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// Surprise (SRD 5.1, "Surprise").
//
// The master decides who might be surprised, and marks any creature, player characters
// included, one by one: a band of adventurers can be surprised in part. A surprised
// creature cannot move or take an action on its first turn of the combat, and cannot take a
// reaction until that turn ends. The app also takes its bonus action away (the SRD says only
// "an action": the app's reading). The mark is set before the combat begins; it lasts until
// the end of the creature's turn in the first round, and the stored row stays: the state is
// read from the turn that is running.
//
// The app suggests who is surprised and the master decides: each hider's Stealth total is
// compared with each creature's passive Perception (not the best Stealth of the group); a
// creature that notices none of the hiders is suggested, and one that notices a hider is not.

// currentOrder is the place in the order of the turn that is running, -1 when none.
func currentOrder(enc playdb.Encounter, cs []playdb.Combatant) int {
	if enc.CurrentCombatantID == nil {
		return -1
	}
	if c, ok := combatantByID(cs, *enc.CurrentCombatantID); ok {
		return int(c.OrderIndex)
	}
	return -1
}

// stillSurprised says whether a combatant marked surprised is still surprised in this
// combat: before it begins, or until its turn of the first round ends.
func stillSurprised(enc playdb.Encounter, current int, c playdb.Combatant) bool {
	if enc.Status == statusEnded || c.Defeated {
		return false
	}
	return combat.SurprisedUntilTurnEnds(enc.Status == statusActive, int(enc.Round), current, int(c.OrderIndex))
}

// surprisedIDs are the combatants that are surprised now.
func (s *Service) surprisedIDs(_ context.Context, enc playdb.Encounter, cs []playdb.Combatant, rows []playdb.CombatSurprised) ([]string, error) {
	current := currentOrder(enc, cs)
	var out []string
	for _, r := range rows {
		if c, ok := combatantByID(cs, r.CombatantID); ok && stillSurprised(enc, current, c) {
			out = append(out, c.ID)
		}
	}
	return out, nil
}

// surprisedNow says whether one combatant is surprised now. tx is the open transaction, or
// nil for a read.
func (s *Service) surprisedNow(ctx context.Context, tx pgx.Tx, enc playdb.Encounter, c playdb.Combatant) (bool, error) {
	q := s.queriesIn(tx)
	marked, err := q.IsCombatantSurprised(ctx, c.ID)
	if err != nil {
		return false, fmt.Errorf("read the surprise: %w", err)
	}
	if !marked {
		return false, nil
	}
	current := -1
	if enc.CurrentCombatantID != nil {
		if current32, err := q.GetCombatantOrder(ctx, *enc.CurrentCombatantID); err == nil {
			current = int(current32)
		}
	}
	return stillSurprised(enc, current, c), nil
}

// mustNotBeSurprised refuses a combatant that is surprised: it does not act, move or react.
func (s *Service) mustNotBeSurprised(ctx context.Context, c *combatTx, who playdb.Combatant) error {
	surprised, err := s.surprisedNow(ctx, c.tx, c.enc, who)
	if err != nil {
		return err
	}
	if surprised {
		return gateError(rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_SURPRISED)
	}
	return nil
}

// SetSurprised implements playv1connect.ContestServiceHandler.
func (s *Service) SetSurprised(
	ctx context.Context,
	req *connect.Request[playv1.SetSurprisedRequest],
) (*connect.Response[playv1.SetSurprisedResponse], error) {
	m, key, encID, err := masterCaller(ctx, req.Msg.GetCampaignId(), req.Msg.GetIdempotencyKey(), req.Msg.GetEncounterId())
	if err != nil {
		return nil, err
	}
	combatantID, err := parseCombatID(req.Msg.GetCombatantId(), "combatant")
	if err != nil {
		return nil, err
	}
	surprised := req.Msg.GetSurprised()
	var made actionEvent
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventSurpriseSet, encounterID: encID}, func(c *combatTx) (any, error) {
		made = actionEvent{}
		return s.surprisedWrite(ctx, c, combatantID, surprised, &made)
	})
	if err != nil {
		return nil, s.dbError(ctx, "mark a surprise", err)
	}
	ev, err := resultEvent(res, made)
	if err != nil {
		return nil, s.dbError(ctx, "read the surprise", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChangedFor(ctx, m.CampaignID, d, ev.Actor)
	})
	if err != nil {
		return nil, err
	}
	view, err := s.surpriseViewOf(ctx, m, res.encounterID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.SetSurprisedResponse{Encounter: out, Surprise: view}), nil
}

// surprisedWrite is the transaction of SetSurprised: the mark is the master's until the
// combatant's first turn has ended.
func (s *Service) surprisedWrite(ctx context.Context, c *combatTx, combatantID string, surprised bool, made *actionEvent) (any, error) {
	if err := notEnded(c.enc); err != nil {
		return nil, err
	}
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return nil, err
	}
	who, ok := combatantByID(cs, combatantID)
	if !ok {
		return nil, errCombatantNotFound()
	}
	// Surprise is the start of the encounter: until the creature's first turn ends.
	if !stillSurprised(c.enc, currentOrder(c.enc, cs), playdb.Combatant{OrderIndex: who.OrderIndex}) {
		return nil, errContest(playv1.ContestBlockedReason_CONTEST_BLOCKED_REASON_COMBAT_BEGUN, "the combatant's first turn has ended")
	}
	if err := markSurprise(ctx, c, who, surprised); err != nil {
		return nil, err
	}
	if err := touchEncounter(ctx, c); err != nil {
		return nil, err
	}
	c.characterID = &who.CharacterID
	*made = actionEvent{Round: c.enc.Round, Secret: true, Actor: who.ID, Contest: &contestEvent{Surprised: surprised}}
	return *made, nil
}

// markSurprise sets or takes off the mark of a surprised combatant.
func markSurprise(ctx context.Context, c *combatTx, who playdb.Combatant, surprised bool) error {
	var err error
	if surprised {
		err = c.q.UpsertSurprised(ctx, playdb.UpsertSurprisedParams{CombatantID: who.ID, EncounterID: c.enc.ID, CreatedAt: c.now})
	} else {
		err = c.q.DeleteSurprised(ctx, who.ID)
	}
	if err != nil {
		return fmt.Errorf("mark the surprise: %w", err)
	}
	return nil
}

// surpriseViewOf reads who is surprised, as the caller may read it.
func (s *Service) surpriseViewOf(ctx context.Context, m authz.Membership, encounterID string) (*playv1.SurpriseView, error) {
	d, err := s.readContestData(ctx, m.CampaignID, encounterID)
	if err != nil {
		return nil, err
	}
	r, err := s.contestReaderFor(ctx, m, d)
	if err != nil {
		return nil, err
	}
	view, err := r.surpriseView(ctx)
	if err != nil {
		return nil, s.dbError(ctx, "read the surprise", err)
	}
	return view, nil
}

// stealthTotals are the Stealth totals of the hiders the suggestion compares: the latest
// group check of Stealth of the session (the party sneaking up on the camp), and the Hide
// actions the master applied in the combat, which are newer when there are both.
func (s *Service) stealthTotals(ctx context.Context, q *playdb.Queries, enc playdb.Encounter, cs []playdb.Combatant, attempts []playdb.CombatHideAttempt) (map[string]int, error) {
	out := map[string]int{}
	if err := groupCheckTotals(ctx, q, enc, cs, out); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, a := range attempts { // newest first
		if a.Status != hideApplied || seen[a.HiderID] {
			continue
		}
		seen[a.HiderID] = true
		if roll, err := decodeRoll(a.Roll); err == nil && roll != nil {
			out[a.HiderID] = int(roll.Total)
		}
	}
	return out, nil
}

// groupCheckTotals adds to out the totals of the members of the latest group check of Stealth.
func groupCheckTotals(ctx context.Context, q *playdb.Queries, enc playdb.Encounter, cs []playdb.Combatant, out map[string]int) error {
	checks, err := q.ListGroupChecksOfSkill(ctx, playdb.ListGroupChecksOfSkillParams{GameSessionID: enc.GameSessionID, SkillKey: skillStealth, Limit: 1})
	if err != nil {
		return fmt.Errorf("read the latest Stealth check: %w", err)
	}
	if len(checks) == 0 {
		return nil
	}
	members, err := q.ListGroupCheckMembers(ctx, checks[0].ID)
	if err != nil {
		return fmt.Errorf("read the members of the check: %w", err)
	}
	for _, mb := range members {
		roll, err := decodeRoll(mb.Roll)
		if err != nil || roll == nil {
			continue
		}
		if c, ok := combatantOfCharacter(cs, mb.CharacterID); ok {
			out[c.ID] = int(roll.Total)
		}
	}
	return nil
}

// GetSurpriseSuggestion implements playv1connect.ContestServiceHandler.
func (s *Service) GetSurpriseSuggestion(
	ctx context.Context,
	req *connect.Request[playv1.GetSurpriseSuggestionRequest],
) (*connect.Response[playv1.GetSurpriseSuggestionResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	encID, err := parseCombatID(req.Msg.GetEncounterId(), "encounter")
	if err != nil {
		return nil, err
	}
	d, err := s.readContestData(ctx, m.CampaignID, encID)
	if err != nil {
		return nil, err
	}
	totals, err := s.stealthTotalsRead(ctx, d)
	if err != nil {
		return nil, s.dbError(ctx, "read the Stealth totals", err)
	}
	marked, err := s.surprisedIDs(ctx, d.enc, d.cs, d.surprised)
	if err != nil {
		return nil, s.dbError(ctx, "read the surprise", err)
	}
	out := &playv1.GetSurpriseSuggestionResponse{}
	for _, c := range d.cs {
		if c.Defeated {
			continue
		}
		sg, err := s.suggestionFor(ctx, m.CampaignID, d, c, totals, slices.Contains(marked, c.ID))
		if err != nil {
			return nil, err
		}
		out.Suggestions = append(out.Suggestions, sg)
	}
	return connect.NewResponse(out), nil
}

// suggestionFor says whether the combatant should be surprised: each hider's Stealth total
// against its passive Perception.
func (s *Service) suggestionFor(ctx context.Context, campaignID string, d *contestData, c playdb.Combatant, totals map[string]int, surprised bool) (*playv1.SurpriseSuggestion, error) {
	n, err := s.numbersOf(ctx, nil, campaignID, c, skillPercept)
	if err != nil {
		return nil, s.dbError(ctx, "read the Perception of a combatant", err)
	}
	notes, err := s.checkSourcesRead(ctx, campaignID, d.enc, c, skillPercept, d.cs)
	if err != nil {
		return nil, s.dbError(ctx, "read the Perception of a combatant", err)
	}
	passive := passivePerceptionOf(n, notesMode(notes))
	sg := &playv1.SurpriseSuggestion{
		CombatantId: c.ID, PassivePerception: clamp32(passive, math.MinInt32, math.MaxInt32), Surprised: surprised,
	}
	hiders, notHiding := hidersAgainst(d, c, totals, passive, sg)
	switch {
	case len(hiders) == 0:
		sg.Reason = playv1.SurpriseSuggestionReason_SURPRISE_SUGGESTION_REASON_NO_HIDERS
	case notHiding > 0: // a creature that is not hiding is noticed (SRD 5.1, "Surprise": a threat noticed)
		sg.Reason = playv1.SurpriseSuggestionReason_SURPRISE_SUGGESTION_REASON_NOTICES
	case combat.NoticesNoThreat(passive, hiders):
		sg.Suggested, sg.Reason = true, playv1.SurpriseSuggestionReason_SURPRISE_SUGGESTION_REASON_HIDERS_BEAT
	default:
		sg.Reason = playv1.SurpriseSuggestionReason_SURPRISE_SUGGESTION_REASON_NOTICES
	}
	return sg, nil
}

// hidersAgainst compares the Stealth total of each creature of the other side with the
// passive Perception of c, adds the totals to sg, and returns the hiders and how many
// creatures of the other side have no total: they are in plain sight.
func hidersAgainst(d *contestData, c playdb.Combatant, totals map[string]int, passive int, sg *playv1.SurpriseSuggestion) (hiders []combat.HideTotals, notHiding int) {
	for _, h := range d.cs {
		if h.ID == c.ID || h.Defeated || h.Dismissed || !oppositeSide(h, c) {
			continue
		}
		total, ok := totals[h.ID]
		if !ok {
			notHiding++
			continue
		}
		// A creature the master said sees the hider clearly notices it, whatever the numbers.
		seesClearly := slices.ContainsFunc(d.hiding, func(r playdb.CombatHiding) bool { return r.HiderID == h.ID && r.ObserverID == c.ID && r.Noticed })
		beats := !combat.NoticesHider(total, passive) && !seesClearly
		hiders = append(hiders, combat.HideTotals{ID: h.ID, Total: total})
		sg.Hiders = append(sg.Hiders, &playv1.HiderTotal{CombatantId: h.ID, StealthTotal: clamp32(total, math.MinInt32, math.MaxInt32), Beats: beats})
		if seesClearly {
			hiders[len(hiders)-1].Total = passive // it notices this one
		}
	}
	return hiders, notHiding
}

// stealthTotalsRead is stealthTotals in a read-only transaction.
func (s *Service) stealthTotalsRead(ctx context.Context, d *contestData) (map[string]int, error) {
	var out map[string]int
	err := dbReadTx(ctx, s, func(q *playdb.Queries) error {
		var err error
		out, err = s.stealthTotals(ctx, q, d.enc, d.cs, d.attempts)
		return err
	})
	return out, err
}

// dbReadTx runs a read in one read-only transaction, with the queries on it.
func dbReadTx(ctx context.Context, s *Service, fn func(q *playdb.Queries) error) error {
	return db.ReadTx(ctx, s.pool, func(tx pgx.Tx) error { return fn(s.queries.WithTx(tx)) })
}

// surprisedReactor says whether a combatant that could react is surprised: the mark is read
// first, and the combat only when there is one (the reactors of a move are many, the surprised
// few).
func (s *Service) surprisedReactor(ctx context.Context, tx pgx.Tx, r playdb.Combatant) (bool, error) {
	q := s.queriesIn(tx)
	marked, err := q.IsCombatantSurprised(ctx, r.ID)
	if err != nil {
		return false, fmt.Errorf("read the surprise: %w", err)
	}
	if !marked {
		return false, nil
	}
	enc, err := q.GetEncounterByID(ctx, r.EncounterID)
	if err != nil {
		return false, fmt.Errorf("find the encounter: %w", err)
	}
	return s.surprisedNow(ctx, tx, enc, r)
}
