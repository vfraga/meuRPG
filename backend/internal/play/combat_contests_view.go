package play

import (
	"context"
	"fmt"
	"slices"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// What the contests show (RN-10, RN-20): the state a caller may read of the contests, the
// Hide actions, the Helps, the grapples and the surprise of a combat. The master reads
// everything; a player reads what is theirs and what the table sees, never a number or a
// skill of an NPC's check, a fixed escape DC, who noticed a hider, or the other player's
// total.

// contestData is what the contests keep of a combat, read together.
type contestData struct {
	enc       playdb.Encounter
	cs        []playdb.Combatant
	contests  []playdb.CombatContest
	holds     []playdb.CombatHold
	hiding    []playdb.CombatHiding
	attempts  []playdb.CombatHideAttempt
	helps     []playdb.CombatHelp // the ones that hold now
	surprised []playdb.CombatSurprised
}

// loadContestData reads what the contests keep of a combat with the queries given (the
// change's transaction, or a read-only one).
func (s *Service) loadContestData(ctx context.Context, q *playdb.Queries, enc playdb.Encounter, cs []playdb.Combatant) (*contestData, error) {
	d := &contestData{enc: enc, cs: cs}
	var err error
	if d.contests, err = q.ListContests(ctx, playdb.ListContestsParams{EncounterID: enc.ID, Limit: maxContestsListed}); err != nil {
		return nil, fmt.Errorf("list the contests: %w", err)
	}
	if d.holds, err = q.ListHolds(ctx, enc.ID); err != nil {
		return nil, fmt.Errorf("list the holds: %w", err)
	}
	if d.hiding, err = q.ListHiding(ctx, enc.ID); err != nil {
		return nil, fmt.Errorf("list the hiding: %w", err)
	}
	if d.attempts, err = q.ListHideAttempts(ctx, enc.ID); err != nil {
		return nil, fmt.Errorf("list the hide attempts: %w", err)
	}
	if d.helps, err = s.liveHelps(ctx, q, enc, cs); err != nil {
		return nil, err
	}
	if d.surprised, err = q.ListSurprised(ctx, enc.ID); err != nil {
		return nil, fmt.Errorf("list the surprised: %w", err)
	}
	return d, nil
}

// readContestData reads the open session, a combat and what its contests keep, in one
// read-only transaction.
func (s *Service) readContestData(ctx context.Context, campaignID, encounterID string) (*contestData, error) {
	var d *contestData
	err := db.ReadTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		session, err := openSessionWith(ctx, q, campaignID)
		if err != nil {
			return err
		}
		enc, err := encounterInSessionWith(ctx, q, session.ID, encounterID)
		if err != nil {
			return err
		}
		cs, err := q.ListCombatants(ctx, enc.ID)
		if err != nil {
			return fmt.Errorf("list the combatants: %w", err)
		}
		d, err = s.loadContestData(ctx, q, enc, cs)
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "read the contests", err)
	}
	return d, nil
}

// contestReader builds the views of one caller.
type contestReader struct {
	s     *Service
	m     authz.Membership
	v     combatViewer
	sight *fogSight
	d     *contestData
	// terrain and plan are read once, when a view asks for them.
	terrain, plan *grid.Terrain
}

func (s *Service) contestReaderFor(ctx context.Context, m authz.Membership, d *contestData) (*contestReader, error) {
	m = heldViewer(ctx, m) // a held roll's answer renders for who answered
	v, sight, err := s.viewerWith(ctx, m, d.enc, d.cs)
	if err != nil {
		return nil, s.dbError(ctx, "work out what the player sees", err)
	}
	return &contestReader{s: s, m: m, v: v, sight: sight, d: d}, nil
}

func (r *contestReader) terrains(ctx context.Context) (grid.Terrain, grid.Terrain, error) {
	if r.terrain != nil {
		return *r.terrain, *r.plan, nil
	}
	terrain, err := r.s.terrainOf(ctx, nil, r.m.CampaignID, r.d.enc)
	if err != nil {
		return grid.Terrain{}, grid.Terrain{}, err
	}
	known, err := r.sight.knownTerrain(ctx, nil, r.v)
	if err != nil {
		return grid.Terrain{}, grid.Terrain{}, err
	}
	plan := planOn(r.v, terrain, known)
	r.terrain, r.plan = &terrain, &plan
	return terrain, plan, nil
}

// optionOf is what a combatant's check of a skill would roll, for a view.
func (r *contestReader) optionOf(ctx context.Context, who playdb.Combatant, skill string) (checkNumbers, []rollNote, error) {
	n, err := r.s.numbersOf(ctx, nil, r.m.CampaignID, who, skill)
	if err != nil {
		return n, nil, err
	}
	notes, err := r.s.checkSourcesRead(ctx, r.m.CampaignID, r.d.enc, who, skill, r.d.cs)
	return n, notes, err
}

// baseContestView is a contest as every reader sees it: its facts, with a creature the
// player does not see (a hidden NPC) unnamed, as its id is the master's (RN-10).
func (r *contestReader) baseContestView(row playdb.CombatContest, initiator, defender playdb.Combatant) *playv1.ContestView {
	out := &playv1.ContestView{
		Id: row.ID, Kind: kindProto(row.Kind), Purpose: purposeProto(row.Purpose), Status: statusProto(row.Status),
		InitiatorId: row.InitiatorID, DefenderId: row.DefenderID, Round: row.Round, CreatedAt: timestamppb.New(row.CreatedAt),
	}
	if !r.v.master {
		if !r.v.sees(initiator) {
			out.InitiatorId = ""
		}
		if !r.v.sees(defender) {
			out.DefenderId = ""
		}
	}
	if row.Winner != nil && (row.Status == contestResolved || row.Status == contestAwaitingOutcome) {
		out.Winner = winnerProto(*row.Winner)
	}
	if row.ShoveOutcome != nil {
		out.ShoveOutcome = shoveProto(*row.ShoveOutcome)
	}
	return out
}

// fillContestRolls adds the rolls the caller may read. The rolls are their roller's and the
// master's (RN-20): a player never reads the other side's total.
func (r *contestReader) fillContestRolls(out *playv1.ContestView, row playdb.CombatContest, iRoll, dRoll *contestRoll, initiator, defender playdb.Combatant) {
	if iRoll != nil && (r.v.master || r.v.owns(initiator)) {
		out.InitiatorRoll = iRoll.proto()
		if r.v.master {
			out.InitiatorBonusKnown = !iRoll.Unknown
		}
	}
	if dRoll != nil && dRoll.Deferred {
		out.Deferred, out.DeferredSkill = true, skillEnum(dRoll.Skill)
	} else if dRoll != nil && (r.v.master || r.v.owns(defender)) {
		out.DefenderRoll = dRoll.proto()
	}
	if r.v.master && row.EscapeDc != nil {
		out.EscapeDc = *row.EscapeDc
	}
}

// fillAwaitingDefender says who the contest waits for and, for the one that answers, what
// each skill would roll.
func (r *contestReader) fillAwaitingDefender(ctx context.Context, out *playv1.ContestView, row playdb.CombatContest, defender playdb.Combatant) error {
	out.WaitingFor, out.WaitingCombatantId = playv1.ContestWaitFor_CONTEST_WAIT_FOR_MASTER, ""
	if defender.UserID != nil && !out.Deferred {
		out.WaitingFor, out.WaitingCombatantId = playv1.ContestWaitFor_CONTEST_WAIT_FOR_PLAYER, defender.ID
	}
	out.YouAnswer = row.Kind == contestKindContest && (r.v.master || (r.v.owns(defender) && !out.Deferred))
	if !out.YouAnswer {
		return nil
	}
	var err error
	out.AnswerOptions, err = r.answerOptions(ctx, row, defender, out.Deferred, out.DeferredSkill)
	return err
}

// fillAwaitingOutcome says who the won shove waits for and, for the winner, what it may do.
func (r *contestReader) fillAwaitingOutcome(ctx context.Context, out *playv1.ContestView, initiator, defender playdb.Combatant) error {
	out.WaitingFor, out.WaitingCombatantId = playv1.ContestWaitFor_CONTEST_WAIT_FOR_MASTER, ""
	if initiator.UserID != nil {
		out.WaitingFor, out.WaitingCombatantId = playv1.ContestWaitFor_CONTEST_WAIT_FOR_PLAYER, initiator.ID
	}
	if !r.v.master && !r.v.owns(initiator) {
		return nil
	}
	out.YouChoose = true
	var err error
	out.ShoveChoice, err = r.shoveChoice(ctx, initiator, defender)
	return err
}

// contestView is a contest as the caller reads it; nil when it is not theirs to read: the
// master reads all, a player the contests of their own characters.
func (r *contestReader) contestView(ctx context.Context, row playdb.CombatContest) (*playv1.ContestView, error) {
	initiator, okI := combatantByID(r.d.cs, row.InitiatorID)
	defender, okD := combatantByID(r.d.cs, row.DefenderID)
	if !okI || !okD || (!r.v.master && !r.v.owns(initiator) && !r.v.owns(defender)) {
		return nil, nil
	}
	iRoll, err := decodeRoll(row.InitiatorRoll)
	if err != nil {
		return nil, err
	}
	dRoll, err := decodeRoll(row.DefenderRoll)
	if err != nil {
		return nil, err
	}
	out := r.baseContestView(row, initiator, defender)
	r.fillContestRolls(out, row, iRoll, dRoll, initiator, defender)
	switch row.Status {
	case contestAwaitingDefender:
		err = r.fillAwaitingDefender(ctx, out, row, defender)
	case contestAwaitingOutcome:
		err = r.fillAwaitingOutcome(ctx, out, initiator, defender)
	default:
		out.WaitingFor = playv1.ContestWaitFor_CONTEST_WAIT_FOR_NONE
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

// answerOptions are the two skills the defender may answer with, each with what it would
// roll; the master also reads the suggestion (the higher modifier) and whether the number
// is known. An escape is contested by the grappler's Athletics alone.
func (r *contestReader) answerOptions(ctx context.Context, row playdb.CombatContest, defender playdb.Combatant, deferred bool, deferredSkill playv1.ContestSkill) ([]*playv1.ContestSkillOption, error) {
	skills := []playv1.ContestSkill{playv1.ContestSkill_CONTEST_SKILL_ATHLETICS, playv1.ContestSkill_CONTEST_SKILL_ACROBATICS}
	if row.Purpose == purposeEscape {
		skills = skills[:1]
	}
	if deferred {
		skills = []playv1.ContestSkill{deferredSkill}
	}
	var out []*playv1.ContestSkillOption
	best := -1
	for i, sk := range skills {
		n, notes, err := r.optionOf(ctx, defender, skillKeyOf(sk))
		if err != nil {
			return nil, err
		}
		opt := &playv1.ContestSkillOption{
			Skill: sk, Modifier: conv.Int32(n.Bonus), Mode: checkModeProto(notesMode(notes)), Notes: notesProto(notes),
			Known: n.Known || !r.v.master,
		}
		out = append(out, opt)
		if best < 0 || opt.Modifier > out[best].Modifier {
			best = i
		}
	}
	if r.v.master && best >= 0 && len(out) > 1 {
		out[best].Suggested = true
	}
	return out, nil
}

// shoveChoice is what a won shove may do, as its winner reads it.
func (r *contestReader) shoveChoice(ctx context.Context, shover, target playdb.Combatant) (*playv1.ShoveChoice, error) {
	out := &playv1.ShoveChoice{ProneAvailable: true, PushAvailable: true}
	terrain, plan, err := r.terrains(ctx)
	if err != nil {
		return nil, err
	}
	push := r.s.pushPlanOf(terrain, plan, r.d.enc, r.d.cs, shover, target, r.v)
	if push.blocked {
		out.PushAvailable, out.PushBlocked = false, push.why
	}
	return out, nil
}

// hideAttemptView is a Hide action as the caller reads it; nil when it is not theirs.
func (r *contestReader) hideAttemptView(ctx context.Context, a playdb.CombatHideAttempt) (*playv1.HideAttemptView, error) {
	hider, ok := combatantByID(r.d.cs, a.HiderID)
	if !ok || (!r.v.master && !r.v.owns(hider)) {
		return nil, nil
	}
	roll, err := decodeRoll(a.Roll)
	if err != nil || roll == nil {
		return nil, fmt.Errorf("read the roll of hide attempt %s: %w", a.ID, err)
	}
	out := &playv1.HideAttemptView{Id: a.ID, HiderId: a.HiderID, Status: hideStatusProto(a.Status), Roll: roll.proto(), Round: a.Round}
	if a.Status == hideRefused {
		out.Refusal = defaultHideRefus
		if a.Refusal != nil {
			out.Refusal = *a.Refusal
		}
	}
	if r.v.master {
		if out.Observers, err = r.observers(ctx, a, hider, roll); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// observers are the creatures that could see a hider, with what the app compares: the
// stored states of an applied attempt, the suggestion for a pending one. Only the master.
func (r *contestReader) observers(ctx context.Context, a playdb.CombatHideAttempt, hider playdb.Combatant, roll *contestRoll) ([]*playv1.HideObserver, error) {
	stored := map[string]playdb.CombatHiding{}
	for _, h := range r.d.hiding {
		if h.HiderID == hider.ID {
			stored[h.ObserverID] = h
		}
	}
	var out []*playv1.HideObserver
	for _, o := range r.d.cs {
		if o.ID == hider.ID || o.Defeated || !oppositeSide(hider, o) {
			continue
		}
		if h, ok := stored[o.ID]; ok && a.Status == hideApplied {
			out = append(out, &playv1.HideObserver{CombatantId: o.ID, PassivePerception: h.Passive, Known: true, Noticed: h.Noticed})
			continue
		}
		view, err := r.s.observerOf(ctx, nil, r.m.CampaignID, r.d.enc, o, int(roll.Total), r.d.cs, nil)
		if err != nil {
			return nil, err
		}
		out = append(out, &playv1.HideObserver{CombatantId: o.ID, PassivePerception: conv.Int32(view.passive), Known: view.known, Noticed: view.noticed})
	}
	return out, nil
}

// helpViewOf is a Help that holds, as the caller reads it; false when the caller does not
// read it: a Help between creatures a player does not see names a hidden creature.
func (r *contestReader) helpViewOf(h playdb.CombatHelp) (*playv1.HelpView, bool) {
	helper, okH := helpHelper(r.d.cs, h)
	ally, okA := helpAlly(r.d.cs, h)
	if !okH || !okA || (!r.v.master && (!r.v.sees(helper) || !r.v.sees(ally))) {
		return nil, false
	}
	view := &playv1.HelpView{Id: h.ID, HelperId: helper.ID, AllyId: ally.ID}
	switch h.Kind {
	case helpCheck:
		view.Kind, view.TaskKey = playv1.HelpKind_HELP_KIND_CHECK, conv.OrZero(h.Task)
		view.TaskNamePt = r.s.roster.SceneCheckName(view.TaskKey)
	case helpAttack:
		target, ok := combatantByID(r.d.cs, conv.OrZero(h.TargetID))
		if !ok || (!r.v.master && !r.v.sees(target)) {
			return nil, false
		}
		view.Kind, view.TargetId = playv1.HelpKind_HELP_KIND_ATTACK, target.ID
	}
	if h.ExpiresRound != nil {
		view.ExpiresRound = *h.ExpiresRound
	}
	return view, true
}

// helpViews are the Helps that hold, for the caller: every one, except the ones aimed at a
// creature a player does not see.
func (r *contestReader) helpViews() []*playv1.HelpView {
	var out []*playv1.HelpView
	for _, h := range r.d.helps {
		if view, ok := r.helpViewOf(h); ok {
			out = append(out, view)
		}
	}
	return out
}

// combatantOfCharacter is the combatant of a character in the combat: its own, never one of
// its creatures (a creature carries its owner's character_id).
func combatantOfCharacter(cs []playdb.Combatant, characterID string) (playdb.Combatant, bool) {
	i := slices.IndexFunc(cs, func(c playdb.Combatant) bool {
		return c.CharacterID == characterID && !c.Dismissed && !isCreature(c)
	})
	if i < 0 {
		return playdb.Combatant{}, false
	}
	return cs[i], true
}

// grappleViews say who holds whom: the master all; a player the grapples whose two creatures
// they see.
func (r *contestReader) grappleViews() []*playv1.GrappleView {
	var out []*playv1.GrappleView
	for _, h := range r.d.holds {
		grappled, okG := combatantByID(r.d.cs, h.GrappledID)
		grappler, okH := combatantByID(r.d.cs, h.GrapplerID)
		if !okG || !okH || !isGrappled(grappled) {
			continue
		}
		if !r.v.master && (!r.v.sees(grappled) || !r.v.sees(grappler)) {
			continue
		}
		view := &playv1.GrappleView{GrappledId: h.GrappledID, GrapplerId: h.GrapplerID}
		if r.v.master && h.EscapeDc != nil {
			view.EscapeDc = *h.EscapeDc
		}
		out = append(out, view)
	}
	return out
}

// hiddenIDs are the hidden combatants: the master every hider, a player their own.
func (r *contestReader) hiddenIDs() []string {
	var out []string
	for _, c := range r.d.cs {
		if !isHidden(r.d.hiding, c.ID) || (!r.v.master && !r.v.owns(c)) {
			continue
		}
		out = append(out, c.ID)
	}
	return out
}

// isHidden says whether a combatant is hidden from at least one creature.
func isHidden(hiding []playdb.CombatHiding, id string) bool {
	return slices.ContainsFunc(hiding, func(h playdb.CombatHiding) bool { return h.HiderID == id && !h.Noticed })
}

// surpriseView is who is surprised: the master every combatant, a player their own.
func (r *contestReader) surpriseView(ctx context.Context) (*playv1.SurpriseView, error) {
	out := &playv1.SurpriseView{}
	ids, err := r.s.surprisedIDs(ctx, r.d.enc, r.d.cs, r.d.surprised)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		c, ok := combatantByID(r.d.cs, id)
		if ok && (r.v.master || r.v.owns(c)) {
			out.CombatantIds = append(out.CombatantIds, id)
		}
	}
	return out, nil
}

// contestViews are the contests the caller may read, newest first as the table keeps them.
func (r *contestReader) contestViews(ctx context.Context) ([]*playv1.ContestView, error) {
	var out []*playv1.ContestView
	for _, row := range r.d.contests {
		view, err := r.contestView(ctx, row)
		if err != nil {
			return nil, err
		}
		if view != nil {
			out = append(out, view)
		}
	}
	return out, nil
}

// hideAttemptViews are the Hide actions the caller may read: the pending ones and the latest
// decided of each hider.
func (r *contestReader) hideAttemptViews(ctx context.Context) ([]*playv1.HideAttemptView, error) {
	var out []*playv1.HideAttemptView
	seen := map[string]bool{}
	for _, a := range r.d.attempts { // newest first
		if a.Status != hidePending && seen[a.HiderID] {
			continue
		}
		if a.Status != hidePending {
			seen[a.HiderID] = true
		}
		view, err := r.hideAttemptView(ctx, a)
		if err != nil {
			return nil, err
		}
		if view != nil {
			out = append(out, view)
		}
	}
	return out, nil
}

// state is every contest fact the caller may read.
func (r *contestReader) state(ctx context.Context) (*playv1.GetContestStateResponse, error) {
	var err error
	out := &playv1.GetContestStateResponse{}
	if out.Contests, err = r.contestViews(ctx); err != nil {
		return nil, err
	}
	if out.HideAttempts, err = r.hideAttemptViews(ctx); err != nil {
		return nil, err
	}
	out.HiddenIds = r.hiddenIDs()
	out.Helps = r.helpViews()
	out.Grapples = r.grappleViews()
	if out.Surprise, err = r.surpriseView(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

// GetContestState implements playv1connect.ContestServiceHandler.
func (s *Service) GetContestState(
	ctx context.Context,
	req *connect.Request[playv1.GetContestStateRequest],
) (*connect.Response[playv1.GetContestStateResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
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
	r, err := s.contestReaderFor(ctx, m, d)
	if err != nil {
		return nil, err
	}
	out, err := r.state(ctx)
	if err != nil {
		return nil, s.dbError(ctx, "read the contests", err)
	}
	return connect.NewResponse(out), nil
}

// contestResponse finishes a call that answers with the combat and the contest: it tells
// the streams, builds the answer and reads the contest back as the caller may read it.
func contestResponse[T any](ctx context.Context, s *Service, m authz.Membership, res combatResult, ev actionEvent, build func(*playv1.Encounter, *playv1.ContestView) *T) (*connect.Response[T], error) {
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChangedFor(ctx, m.CampaignID, d, ev.Actor, ev.Target)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !ev.Secret)
	})
	if err != nil {
		return nil, err
	}
	var view *playv1.ContestView
	if ev.Contest != nil && ev.Contest.ContestID != "" {
		if view, err = s.contestViewFor(ctx, m, res.encounterID, ev.Contest.ContestID); err != nil {
			return nil, err
		}
	}
	return connect.NewResponse(build(out, view)), nil
}

// contestViewFor reads a contest back as the caller may read it.
func (s *Service) contestViewFor(ctx context.Context, m authz.Membership, encounterID, contestID string) (*playv1.ContestView, error) {
	d, err := s.readContestData(ctx, m.CampaignID, encounterID)
	if err != nil {
		return nil, err
	}
	r, err := s.contestReaderFor(ctx, m, d)
	if err != nil {
		return nil, err
	}
	for _, row := range d.contests {
		if row.ID == contestID {
			view, err := r.contestView(ctx, row)
			if err != nil {
				return nil, s.dbError(ctx, "read the contest", err)
			}
			return view, nil
		}
	}
	return nil, nil
}

func kindProto(k string) playv1.ContestKind {
	if k == contestKindEscapeDC {
		return playv1.ContestKind_CONTEST_KIND_ESCAPE_DC
	}
	return playv1.ContestKind_CONTEST_KIND_CONTEST
}

func purposeProto(p string) playv1.ContestPurpose {
	switch p {
	case purposeGrapple:
		return playv1.ContestPurpose_CONTEST_PURPOSE_GRAPPLE
	case purposeShove:
		return playv1.ContestPurpose_CONTEST_PURPOSE_SHOVE
	case purposeEscape:
		return playv1.ContestPurpose_CONTEST_PURPOSE_ESCAPE
	}
	return playv1.ContestPurpose_CONTEST_PURPOSE_UNSPECIFIED
}

func statusProto(s string) playv1.ContestStatus {
	switch s {
	case contestAwaitingDefender:
		return playv1.ContestStatus_CONTEST_STATUS_AWAITING_DEFENDER
	case contestAwaitingOutcome:
		return playv1.ContestStatus_CONTEST_STATUS_AWAITING_OUTCOME
	case contestResolved:
		return playv1.ContestStatus_CONTEST_STATUS_RESOLVED
	case contestClosed:
		return playv1.ContestStatus_CONTEST_STATUS_CLOSED
	}
	return playv1.ContestStatus_CONTEST_STATUS_UNSPECIFIED
}

func winnerProto(w string) playv1.ContestWinner {
	switch w {
	case winnerInitiator:
		return playv1.ContestWinner_CONTEST_WINNER_INITIATOR
	case winnerDefender:
		return playv1.ContestWinner_CONTEST_WINNER_DEFENDER
	case winnerTie:
		return playv1.ContestWinner_CONTEST_WINNER_TIE
	}
	return playv1.ContestWinner_CONTEST_WINNER_UNSPECIFIED
}

func shoveProto(o string) playv1.ShoveOutcome {
	switch o {
	case shoveProne:
		return playv1.ShoveOutcome_SHOVE_OUTCOME_PRONE
	case shovePush:
		return playv1.ShoveOutcome_SHOVE_OUTCOME_PUSH
	case shoveStays:
		return playv1.ShoveOutcome_SHOVE_OUTCOME_STAYS
	}
	return playv1.ShoveOutcome_SHOVE_OUTCOME_UNSPECIFIED
}

func hideStatusProto(s string) playv1.HideAttemptStatus {
	switch s {
	case hidePending:
		return playv1.HideAttemptStatus_HIDE_ATTEMPT_STATUS_PENDING
	case hideApplied:
		return playv1.HideAttemptStatus_HIDE_ATTEMPT_STATUS_APPLIED
	case hideRefused:
		return playv1.HideAttemptStatus_HIDE_ATTEMPT_STATUS_REFUSED
	}
	return playv1.HideAttemptStatus_HIDE_ATTEMPT_STATUS_UNSPECIFIED
}
