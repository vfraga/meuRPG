package play

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// Help (SRD 5.1, "Help" and "Working Together").
//
// The Help action has two forms. Check: the ally has advantage on the next ability check
// it makes for the task, if it makes it before the help ends. Attack: the ally's first
// attack roll against a creature within 5 feet of the helper has advantage. The helper has
// to be able to attempt the task alone (the sheet has a number for the check); the master
// decides when a task admits help.
//
// A Help lasts until it is used, or until the start of the helper's next turn (the SRD's
// words are "before the start of your next turn"). Outside a combat a Help would last until the master
// clears it; the combat is the only place that makes one today. A Help is a source of
// advantage every player reads ("Ajuda de Orla"); the one aimed at a creature a player does
// not see is not theirs to read. A player's creature helps and is helped like anyone: a
// familiar can't attack, but it can take other actions as normal (SRD 5.1, Find Familiar),
// and its Help is the classic one. A Help names the helper's and the ally's combatants
// (00241), because a creature carries its owner's character_id.

// liveHelps are the Helps that hold now in a combat: not used, not cleared, and not past the
// start of the helper's next turn.
func (s *Service) liveHelps(ctx context.Context, q *playdb.Queries, enc playdb.Encounter, cs []playdb.Combatant) ([]playdb.CombatHelp, error) {
	rows, err := q.ListLiveHelps(ctx, enc.GameSessionID)
	if err != nil {
		return nil, fmt.Errorf("list the helps: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	current := -1
	if enc.CurrentCombatantID != nil {
		if c, ok := combatantByID(cs, *enc.CurrentCombatantID); ok {
			current = int(c.OrderIndex)
		}
	}
	var out []playdb.CombatHelp
	for _, h := range rows {
		if h.EncounterID == nil {
			out = append(out, h) // outside a combat: until the master clears it
			continue
		}
		if *h.EncounterID != enc.ID {
			continue
		}
		helper, ok := helpHelper(cs, h)
		if !ok {
			continue
		}
		if combat.HelpLastsThrough(int(num(h.CreatedRound)), int(enc.Round), current, int(helper.OrderIndex)) {
			out = append(out, h)
		}
	}
	return out, nil
}

// helpHelper is the combatant that gave a Help: the one the row names, or, for a Help kept
// before the rows named combatants, the character's own combatant.
func helpHelper(cs []playdb.Combatant, h playdb.CombatHelp) (playdb.Combatant, bool) {
	if h.HelperCombatantID == nil {
		return combatantOfCharacter(cs, h.HelperCharacterID)
	}
	c, ok := combatantByID(cs, *h.HelperCombatantID)
	return c, ok && !c.Dismissed
}

// helpAlly is the combatant a Help is for, read the same way.
func helpAlly(cs []playdb.Combatant, h playdb.CombatHelp) (playdb.Combatant, bool) {
	if h.AllyCombatantID == nil {
		return combatantOfCharacter(cs, h.AllyCharacterID)
	}
	c, ok := combatantByID(cs, *h.AllyCombatantID)
	return c, ok && !c.Dismissed
}

// helpsAlly says whether a Help is for this combatant: a Help for Pensantus is never one for
// his familiar or his wolves, who carry his character_id.
func helpsAlly(h playdb.CombatHelp, c playdb.Combatant) bool {
	if h.AllyCombatantID == nil {
		return h.AllyCharacterID == c.CharacterID && !isCreature(c)
	}
	return *h.AllyCombatantID == c.ID
}

// maxHelpTaskKey is the longest task_key a Help with a check may name.
const maxHelpTaskKey = 100

// helpPlan is a Help request once the checks that need no database are done.
type helpPlan struct {
	kind     playv1.HelpKind
	helperID string
	allyID   string
	targetID string // a Help with an attack
	taskKey  string // a Help with a check
}

// parseHelpPlan reads the ids and the form of a Help, refusing a request whose form does not fit its
// kind. A kind the app does not know passes, as it always has.
func parseHelpPlan(msg *playv1.HelpRequest) (helpPlan, error) {
	var p helpPlan
	var err error
	if p.helperID, err = parseCombatID(msg.GetCombatantId(), "combatant"); err != nil {
		return p, err
	}
	if p.allyID, err = parseCombatID(msg.GetAllyId(), "combatant"); err != nil {
		return p, err
	}
	p.kind, p.taskKey = msg.GetKind(), msg.GetTaskKey()
	switch p.kind {
	case playv1.HelpKind_HELP_KIND_CHECK:
		err = checkHelpWithACheck(msg)
	case playv1.HelpKind_HELP_KIND_ATTACK:
		p.targetID, err = checkHelpWithAnAttack(msg)
	case playv1.HelpKind_HELP_KIND_UNSPECIFIED:
		err = connect.NewError(connect.CodeInvalidArgument, errors.New("kind must be CHECK or ATTACK"))
	}
	return p, err
}

// checkHelpWithACheck refuses a Help with a check that aims at a creature or names no task.
func checkHelpWithACheck(msg *playv1.HelpRequest) error {
	if msg.GetTargetId() != "" {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("target_id is for a Help with an attack"))
	}
	if key := msg.GetTaskKey(); key == "" || len(key) > maxHelpTaskKey {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("task_key must name the check of the task"))
	}
	return nil
}

// checkHelpWithAnAttack refuses a Help with an attack that names a task, and gives its target.
func checkHelpWithAnAttack(msg *playv1.HelpRequest) (string, error) {
	if msg.GetTaskKey() != "" {
		return "", connect.NewError(connect.CodeInvalidArgument, errors.New("task_key is for a Help with a check"))
	}
	return parseCombatID(msg.GetTargetId(), "combatant")
}

// Help implements playv1connect.ContestServiceHandler.
func (s *Service) Help(
	ctx context.Context,
	req *connect.Request[playv1.HelpRequest],
) (*connect.Response[playv1.HelpResponse], error) {
	m, key, encID, err := contestCaller(ctx, req.Msg.GetCampaignId(), req.Msg.GetIdempotencyKey(), req.Msg.GetEncounterId())
	if err != nil {
		return nil, err
	}
	plan, err := parseHelpPlan(req.Msg)
	if err != nil {
		return nil, err
	}

	var made actionEvent
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventHelpGiven, encounterID: encID}, func(c *combatTx) (any, error) {
		made = actionEvent{}
		ev, err := s.giveHelp(ctx, c, m, plan)
		if err != nil {
			return nil, err
		}
		made = ev
		return made, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "help", err)
	}
	ev, err := resultEvent(res, made)
	if err != nil {
		return nil, s.dbError(ctx, "read the help", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChangedFor(ctx, m.CampaignID, d, ev.Actor, ev.Target)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !ev.Secret)
	})
	if err != nil {
		return nil, err
	}
	view, err := s.helpFor(ctx, m, res.encounterID, ev.Contest.HelpID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.HelpResponse{Encounter: out, Help: view}), nil
}

// giveHelp is the change of a Help: who helps whom, what the action costs, and the row kept.
func (s *Service) giveHelp(ctx context.Context, c *combatTx, m authz.Membership, p helpPlan) (actionEvent, error) {
	if err := mustBeRunning(c); err != nil {
		return actionEvent{}, err
	}
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return actionEvent{}, fmt.Errorf("list the combatants: %w", err)
	}
	v := c.viewer(m, cs)
	helper, ally, err := s.helpParties(ctx, c, v, cs, p)
	if err != nil {
		return actionEvent{}, err
	}
	if err := s.mustHaveHelpAction(ctx, c, m, v, helper); err != nil {
		return actionEvent{}, err
	}
	row := playdb.InsertHelpParams{
		GameSessionID: c.session.ID, EncounterID: &c.enc.ID, HelperCharacterID: helper.CharacterID, AllyCharacterID: ally.CharacterID,
		ExpiresRound: new(c.enc.Round + 1), CreatedRound: &c.enc.Round, CreatedAt: c.now,
		HelperCombatantID: &helper.ID, AllyCombatantID: &ally.ID,
	}
	if err := s.aimHelp(ctx, c, v, cs, helper, p, &row); err != nil {
		return actionEvent{}, err
	}
	if _, err := breakRun(ctx, c, helper); err != nil {
		return actionEvent{}, err
	}
	if err := spendAction(ctx, c, helper, v.master); err != nil {
		return actionEvent{}, err
	}
	inserted, err := c.q.InsertHelp(ctx, row)
	if err != nil {
		return actionEvent{}, fmt.Errorf("keep the help: %w", err)
	}
	if err := touchEncounter(ctx, c); err != nil {
		return actionEvent{}, err
	}
	c.characterID = &helper.CharacterID
	return actionEvent{
		Round: c.enc.Round, Secret: secretOf(helper, ally), Actor: helper.ID, Target: ally.ID,
		Contest: &contestEvent{HelpID: inserted.ID, Line: contestLineHelped},
	}, nil
}

// helpParties finds the helper, who may act now, and the ally, who must be another standing
// creature of the helper's side.
func (s *Service) helpParties(ctx context.Context, c *combatTx, v combatViewer, cs []playdb.Combatant, p helpPlan) (helper, ally playdb.Combatant, err error) {
	if helper, err = findCombatant(cs, p.helperID, v); err != nil {
		return helper, ally, err
	}
	if err := v.mayAct(helper); err != nil {
		return helper, ally, err
	}
	if err := s.mustActNow(ctx, c, helper); err != nil {
		return helper, ally, err
	}
	if ally, err = findCombatant(cs, p.allyID, v); err != nil {
		return helper, ally, err
	}
	if ally.ID == helper.ID || ally.Defeated || ally.Side != helper.Side {
		return helper, ally, errContest(playv1.ContestBlockedReason_CONTEST_BLOCKED_REASON_NOT_AN_ALLY, "that is not an ally the combatant can help")
	}
	return helper, ally, nil
}

// mustHaveHelpAction checks that Help is an action of the helper's sheet and that the action is
// free (the master is not stopped by a spent action).
func (s *Service) mustHaveHelpAction(ctx context.Context, c *combatTx, m authz.Membership, v combatViewer, helper playdb.Combatant) error {
	opts, err := s.optionsOf(ctx, c.tx, m.CampaignID, helper)
	if err != nil {
		return err
	}
	list := opts.GetStandardActions()
	i := slices.IndexFunc(list, func(a *rulesv1.ActionOption) bool { return a.GetAction().GetKey() == actionHelp })
	switch {
	case i < 0:
		return errContest(playv1.ContestBlockedReason_CONTEST_BLOCKED_REASON_NOT_AVAILABLE, "the combatant has no Help action")
	case !list[i].GetEnabled() && !v.master:
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ACTION_USED, "the action of this turn is used")
	}
	return nil
}

// aimHelp says on the row what the Help is for: the task of a check, or the creature of an attack.
func (s *Service) aimHelp(ctx context.Context, c *combatTx, v combatViewer, cs []playdb.Combatant, helper playdb.Combatant, p helpPlan, row *playdb.InsertHelpParams) error {
	switch p.kind {
	case playv1.HelpKind_HELP_KIND_CHECK:
		if err := s.mustBeATask(ctx, c, helper, p.taskKey); err != nil {
			return err
		}
		row.Kind, row.Task = helpCheck, &p.taskKey
	case playv1.HelpKind_HELP_KIND_ATTACK:
		target, err := helpTarget(c, v, cs, helper, p.targetID)
		if err != nil {
			return err
		}
		row.Kind, row.TargetID = helpAttack, &target.ID
	}
	return nil
}

// helpTarget finds the creature a Help with an attack aims at: a standing enemy of the helper's
// side, within 5 feet of the helper unless the master gives it or the combat is a theatre.
func helpTarget(c *combatTx, v combatViewer, cs []playdb.Combatant, helper playdb.Combatant, targetID string) (playdb.Combatant, error) {
	target, err := findCombatant(cs, targetID, v)
	if err != nil {
		return target, err
	}
	if target.Defeated || target.Side == helper.Side || target.ID == helper.ID {
		return target, errContest(playv1.ContestBlockedReason_CONTEST_BLOCKED_REASON_NOT_AN_ALLY, "the target is not a creature the ally can attack")
	}
	if !v.master && !isTheatre(c.enc) {
		return target, mustBeWithinHelpReach(helper, target)
	}
	return target, nil
}

// mustBeWithinHelpReach checks that both are on the map and the helper is within 5 feet of the target.
func mustBeWithinHelpReach(helper, target playdb.Combatant) error {
	if !placed(helper) || !placed(target) {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_PLACED, "the combatants must be on the map")
	}
	if dist, _ := distanceFt(helper, target); dist > combat.MeleeReachFt {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TARGET_OUT_OF_REACH, "the target is beyond 5 feet of the helper",
			func(b *playv1.EncounterBlocked) { b.MissingFt = dist - combat.MeleeReachFt })
	}
	return nil
}

// mustBeATask checks that a Help with a check names a check the app knows (a skill or an
// ability check, never a saving throw) that the helper could attempt alone: its sheet has a
// number for it.
func (s *Service) mustBeATask(ctx context.Context, c *combatTx, helper playdb.Combatant, key string) error {
	if !strings.HasPrefix(key, "skill:") && !strings.HasPrefix(key, "ability:") {
		return errContest(playv1.ContestBlockedReason_CONTEST_BLOCKED_REASON_TASK_NOT_AVAILABLE, "the task is not a skill or an ability check")
	}
	if s.roster.SceneCheckName(key) == "" {
		return errContest(playv1.ContestBlockedReason_CONTEST_BLOCKED_REASON_TASK_NOT_AVAILABLE, "the task is not a check of the SRD")
	}
	n, err := s.numbersOf(ctx, c.tx, c.session.CampaignID, helper, key)
	if err != nil {
		return err
	}
	if !n.Known {
		return errContest(playv1.ContestBlockedReason_CONTEST_BLOCKED_REASON_TASK_NOT_AVAILABLE, "the helper could not attempt the task alone")
	}
	return nil
}

// helpFor reads a Help back as the caller may read it.
func (s *Service) helpFor(ctx context.Context, m authz.Membership, encounterID, helpID string) (*playv1.HelpView, error) {
	d, err := s.readContestData(ctx, m.CampaignID, encounterID)
	if err != nil {
		return nil, err
	}
	r, err := s.contestReaderFor(ctx, m, d)
	if err != nil {
		return nil, err
	}
	for _, h := range r.helpViews() {
		if h.GetId() == helpID {
			return h, nil
		}
	}
	return nil, nil
}

// ClearHelp implements playv1connect.ContestServiceHandler.
func (s *Service) ClearHelp(
	ctx context.Context,
	req *connect.Request[playv1.ClearHelpRequest],
) (*connect.Response[playv1.ClearHelpResponse], error) {
	m, key, encID, err := masterCaller(ctx, req.Msg.GetCampaignId(), req.Msg.GetIdempotencyKey(), req.Msg.GetEncounterId())
	if err != nil {
		return nil, err
	}
	helpID, err := parseCombatID(req.Msg.GetHelpId(), "help")
	if err != nil {
		return nil, err
	}
	var made actionEvent
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventHelpCleared, encounterID: encID}, func(c *combatTx) (any, error) {
		made = actionEvent{}
		ev, err := clearHelp(ctx, c, helpID)
		if err != nil {
			return nil, err
		}
		made = ev
		return made, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "clear a help", err)
	}
	out, err := s.finish(ctx, m, res, s.changed(m.CampaignID))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.ClearHelpResponse{Encounter: out}), nil
}

// clearHelp is the change of ClearHelp: a Help that is still awaited is taken back for good.
func clearHelp(ctx context.Context, c *combatTx, helpID string) (actionEvent, error) {
	if err := notEnded(c.enc); err != nil {
		return actionEvent{}, err
	}
	help, err := c.q.GetHelp(ctx, playdb.GetHelpParams{GameSessionID: c.session.ID, ID: helpID})
	if errors.Is(err, pgx.ErrNoRows) {
		return actionEvent{}, connect.NewError(connect.CodeNotFound, errors.New("help not found"))
	}
	if err != nil {
		return actionEvent{}, fmt.Errorf("find the help: %w", err)
	}
	if help.ConsumedAt != nil || help.ClearedAt != nil {
		return actionEvent{}, errNotAwaiting()
	}
	if err := c.q.SetHelpCleared(ctx, playdb.SetHelpClearedParams{ID: help.ID, ClearedAt: &c.now}); err != nil {
		return actionEvent{}, fmt.Errorf("clear the help: %w", err)
	}
	if err := touchEncounter(ctx, c); err != nil {
		return actionEvent{}, err
	}
	return actionEvent{Round: c.enc.Round, Contest: &contestEvent{HelpID: help.ID}}, nil
}
