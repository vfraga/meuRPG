package play

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// The group check (SRD 5.1, "Working Together", Group Checks): the master asks the whole
// party for one check; everyone makes it, and if at least half of the group succeeds, the
// whole group succeeds. The count is over the characters asked (the living player
// characters when the master asks), and a character that did not answer when the master
// closes the check counts as failed.
//
// The same request also asks only some characters, and for a saving throw ("save:con") as
// well as a skill or an ability check, at any moment out of combat. When it asks named
// characters, each roll is judged alone (is_group false): there is no group verdict.
// Only the characters asked read the request or may roll it (RN-10): a player who was not
// asked reads no group check at all.
//
// What each side reads (RN-20): a player reads their own roll, and passed or failed only
// when the master shows the DC (the scene checks' rule); the group's verdict, once the
// check is closed, on the same condition. The DC is never a player's. The master reads
// everything, who has not answered, and may roll for a character or close the check.
//
// The check lives in the session, not in a combat: the master asks it in a scene, on the
// trail. One group check is open at a time.

// groupCheckEvent is the payload of the group check's events: IDs only.
type groupCheckEvent struct {
	GroupCheckID string `json:"group_check_id"`
	CharacterID  string `json:"character_id,omitempty"`
}

// groupChange is what one write of the group check does inside the transaction: it changes the
// rows and says the payload of the event, and the character the event is about (empty for none).
type groupChange func(q *playdb.Queries, tx pgx.Tx, session playdb.GameSession) (payload groupCheckEvent, characterID string, err error)

// groupCall is who makes a write of the group check, and the idempotency data of the call.
type groupCall struct {
	m    authz.Membership
	key  string
	hash *string
	kind string
}

// groupWrite runs one change to the open session's group check, as the scene checks do: one
// transaction locks the open session, replays a retry of the key, lets do change the rows
// and appends the event. It returns whether the call was a retry.
func (s *Service) groupWrite(ctx context.Context, m authz.Membership, key string, hash *string, kind string, do groupChange) (groupCheckEvent, bool, error) {
	call := groupCall{m: m, key: key, hash: hash, kind: kind}
	var ev groupCheckEvent
	var repeated bool
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		ev, repeated = groupCheckEvent{}, false
		q := s.queries.WithTx(tx)
		session, err := lockOpenSessionOrNone(ctx, q, m.CampaignID)
		if err != nil {
			return err
		}
		replayed, found, err := groupReplay(ctx, q, session.ID, call)
		if err != nil {
			return err
		}
		if found {
			ev, repeated = replayed, true
			return nil
		}
		ev, err = s.groupApply(ctx, tx, q, session, call, do)
		return err
	})
	return ev, repeated, err
}

// groupReplay finds the event a retry of the call's key already made. found is false when the key
// is new; a key used for another change, kind or caller is refused.
func groupReplay(ctx context.Context, q *playdb.Queries, sessionID string, call groupCall) (ev groupCheckEvent, found bool, err error) {
	done, err := q.GetSessionEventByIdempotencyKey(ctx, playdb.GetSessionEventByIdempotencyKeyParams{GameSessionID: sessionID, IdempotencyKey: &call.key})
	if errors.Is(err, pgx.ErrNoRows) {
		return groupCheckEvent{}, false, nil
	}
	if err != nil {
		return groupCheckEvent{}, false, fmt.Errorf("find the event of this idempotency key: %w", err)
	}
	if done.Kind != call.kind || done.ActorUserID == nil || *done.ActorUserID != call.m.UserID || hashDiffers(done.IdempotencyHash, call.hash) {
		return groupCheckEvent{}, false, errKeyReused()
	}
	// A retry answers what the first call made: the check it names is in the event.
	if err := json.Unmarshal(done.Payload, &ev); err != nil {
		return groupCheckEvent{}, true, fmt.Errorf("read the event of this idempotency key: %w", err)
	}
	return ev, true, nil
}

// groupApply lets do change the rows of a call that is not a retry and appends its event.
func (s *Service) groupApply(ctx context.Context, tx pgx.Tx, q *playdb.Queries, session playdb.GameSession, call groupCall, do groupChange) (groupCheckEvent, error) {
	c, err := s.openTx(ctx, combatTx{tx: tx, q: q, session: session, now: s.now(), hash: call.hash, svc: s})
	if err != nil {
		return groupCheckEvent{}, err
	}
	payload, characterID, err := do(q, tx, session)
	if errors.Is(err, errRollHeld) {
		return groupCheckEvent{}, nil // the roll waits for the player's answer (a die, or Lucky): nothing is written yet
	}
	if err != nil {
		return groupCheckEvent{}, err
	}
	if characterID != "" {
		c.characterID = &characterID
	}
	_, err = insertSceneEvent(ctx, c, call.kind, &call.m.UserID, &call.key, payload)
	return payload, err
}

// errRollHeld is what a write's closure returns when its roll was kept for the player's answer
// about a Bardic Inspiration die or the halfling's Lucky: the change commits with what it kept,
// and writes no event.
var errRollHeld = errors.New("the roll is held")

// groupHold carries what a group check roll needs to be held for the answer about a die, or for
// the halfling's Lucky question (lucky, which comes first).
type groupHold struct {
	m     authz.Membership
	key   string
	raw   proto.Message // the request that rolled: a player's own (RollGroupCheck) or the master's for a player (RollForPlayer)
	offer *playv1.OutsideInspirationOffer
	lucky *playv1.LuckyOffer
	held  *playv1.HeldRollOffer
}

// asking says the roll was kept for one of the player's answers: nothing is written yet.
func (g *groupHold) asking() bool {
	return g.offer != nil || g.lucky != nil || g.held != nil
}

type groupHoldKey struct{}

func groupHoldOf(ctx context.Context) *groupHold {
	g, _ := ctx.Value(groupHoldKey{}).(*groupHold)
	return g
}

// publishGroupCheckChanged tells the table that the group check changed: a hint with no
// content, read again by each as their own role. While a scene open to only some players is
// open, only its players hear it (RN-10).
func (s *Service) publishGroupCheckChanged(ctx context.Context, campaignID string) {
	s.PublishToScene(ctx, campaignID, &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_GroupCheckChanged_{
		GroupCheckChanged: &playv1.WatchGameSessionResponse_GroupCheckChanged{},
	}})
}

// maxGroupCheckDC is the highest DC a group check can ask (0 asks none).
const maxGroupCheckDC = 40

// minGroupSize is the fewest characters a group verdict is over.
const minGroupSize = 2

// groupAsk is a request for a group check that passed the checks that need no database.
type groupAsk struct {
	skill        string
	characterIDs []string
	dc           int32
	showDC       bool
	group        *bool
}

// parseGroupAsk checks what a request for a group check says by itself, in the order the
// refusals are made: the skill, the characters named and the DC.
func (s *Service) parseGroupAsk(msg *playv1.RequestGroupCheckRequest) (groupAsk, error) {
	skill := msg.GetSkillKey()
	if !isCheckKey(skill) || s.roster.SceneCheckName(skill) == "" {
		return groupAsk{}, connect.NewError(connect.CodeInvalidArgument, errors.New("skill_key must be a skill, an ability check or a saving throw"))
	}
	askedIDs := msg.GetCharacterIds()
	if len(askedIDs) > maxCombatants {
		return groupAsk{}, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("character_ids must have at most %d entries", maxCombatants))
	}
	seen := make(map[string]struct{}, len(askedIDs))
	for _, id := range askedIDs {
		if _, err := parseCombatID(id, "character"); err != nil {
			return groupAsk{}, err
		}
		if _, dup := seen[id]; dup {
			return groupAsk{}, connect.NewError(connect.CodeInvalidArgument, errors.New("character_ids names a character twice"))
		}
		seen[id] = struct{}{}
	}
	dc := msg.GetDc()
	if dc < 0 || dc > maxGroupCheckDC {
		return groupAsk{}, connect.NewError(connect.CodeInvalidArgument, errors.New("dc must be 1 to 40, or 0 for none"))
	}
	return groupAsk{skill: skill, characterIDs: askedIDs, dc: dc, showDC: msg.GetShowDc(), group: msg.Group}, nil
}

// RequestGroupCheck implements playv1connect.ContestServiceHandler.
func (s *Service) RequestGroupCheck(
	ctx context.Context,
	req *connect.Request[playv1.RequestGroupCheckRequest],
) (*connect.Response[playv1.RequestGroupCheckResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	ask, err := s.parseGroupAsk(req.Msg)
	if err != nil {
		return nil, err
	}
	ev, repeated, err := s.groupWrite(ctx, m, key, idem.Hash(req.Msg), eventGroupCheckRequested, func(q *playdb.Queries, tx pgx.Tx, session playdb.GameSession) (groupCheckEvent, string, error) {
		ev, err := s.askGroupCheck(ctx, q, tx, m, session, ask)
		return ev, "", err
	})
	if err != nil {
		return nil, s.dbError(ctx, "ask a group check", err)
	}
	if !repeated {
		s.publishGroupCheckChanged(ctx, m.CampaignID)
	}
	view, err := s.groupCheckView(ctx, m, ev.GroupCheckID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.RequestGroupCheckResponse{GroupCheck: view}), nil
}

// askGroupCheck writes the group check the master asked: one open check at a time, over the
// characters asked.
func (s *Service) askGroupCheck(ctx context.Context, q *playdb.Queries, tx pgx.Tx, m authz.Membership, session playdb.GameSession, ask groupAsk) (groupCheckEvent, error) {
	if _, err := q.GetOpenGroupCheck(ctx, session.ID); err == nil {
		return groupCheckEvent{}, errContest(playv1.ContestBlockedReason_CONTEST_BLOCKED_REASON_GROUP_CHECK_OPEN, "a group check is open already")
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return groupCheckEvent{}, fmt.Errorf("find the open group check: %w", err)
	}
	party, err := s.roster.CombatParty(ctx, tx, m.CampaignID)
	if err != nil {
		return groupCheckEvent{}, err
	}
	// While a scene is open to only some players the check goes to its players alone (RN-10).
	if party, err = s.partyInScene(ctx, q, session, party); err != nil {
		return groupCheckEvent{}, err
	}
	if len(party) == 0 {
		return groupCheckEvent{}, connect.NewError(connect.CodeInvalidArgument, errors.New("the campaign has no living player character to ask"))
	}
	asked, err := askedCharacters(party, ask.characterIDs)
	if err != nil {
		return groupCheckEvent{}, err
	}
	isGroup, err := askIsGroup(ask, len(asked))
	if err != nil {
		return groupCheckEvent{}, err
	}
	var dcValue *int32
	if ask.dc > 0 {
		dcValue = &ask.dc
	}
	created, err := q.InsertGroupCheck(ctx, playdb.InsertGroupCheckParams{
		GameSessionID: session.ID, SkillKey: ask.skill, Dc: dcValue, ShowDc: ask.showDC, IsGroup: isGroup, CreatedAt: s.now(),
	})
	if err != nil {
		return groupCheckEvent{}, fmt.Errorf("ask the group check: %w", err)
	}
	for _, ch := range asked {
		if err := q.InsertGroupCheckMember(ctx, playdb.InsertGroupCheckMemberParams{GroupCheckID: created.ID, CharacterID: ch.ID}); err != nil {
			return groupCheckEvent{}, fmt.Errorf("ask a character: %w", err)
		}
	}
	return groupCheckEvent{GroupCheckID: created.ID}, nil
}

// askedCharacters are the characters asked: all of the party, or the ones named (each must be a
// living player character of this campaign: never an NPC, a dead or an unknown one).
func askedCharacters(party []link.Character, named []string) ([]link.Character, error) {
	if len(named) == 0 {
		return party, nil
	}
	asked := make([]link.Character, 0, len(named))
	for _, id := range named {
		i := slices.IndexFunc(party, func(c link.Character) bool { return c.ID == id })
		if i < 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("character_ids must name living player characters of this campaign"))
		}
		asked = append(asked, party[i])
	}
	return asked, nil
}

// askIsGroup says whether the request has a group verdict. A group check is the default of asking
// everyone; asking some is judged one by one, unless the master says group.
func askIsGroup(ask groupAsk, askedCount int) (bool, error) {
	isGroup := len(ask.characterIDs) == 0
	if ask.group != nil {
		isGroup = *ask.group
	}
	if isGroup && askedCount < minGroupSize && ask.group != nil {
		return false, connect.NewError(connect.CodeInvalidArgument, errors.New("a group check needs at least two characters; ask one without group"))
	}
	return isGroup, nil
}

// isCheckKey says whether a key can be asked: a skill ("skill:stealth"), an ability check
// ("ability:str") or a saving throw ("save:con"). The roster checks the rest of the key.
func isCheckKey(key string) bool {
	return strings.HasPrefix(key, "skill:") || strings.HasPrefix(key, "ability:") || strings.HasPrefix(key, "save:")
}

// RollGroupCheck implements playv1connect.ContestServiceHandler.
func (s *Service) RollGroupCheck(
	ctx context.Context,
	req *connect.Request[playv1.RollGroupCheckRequest],
) (*connect.Response[playv1.RollGroupCheckResponse], error) {
	m, err := heldRole(ctx, req.Msg.GetCampaignId(), authz.RolePlayer)
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	checkID, err := parseCombatID(req.Msg.GetGroupCheckId(), "group check")
	if err != nil {
		return nil, err
	}
	in, err := parseCheckInput(req.Msg.GetRoll())
	if err != nil {
		return nil, err
	}
	hold := &groupHold{m: m, key: key, raw: req.Msg}
	ctx = context.WithValue(ctx, groupHoldKey{}, hold)
	_, repeated, err := s.groupWrite(ctx, m, key, idem.Hash(req.Msg), eventGroupCheckRolled, func(q *playdb.Queries, tx pgx.Tx, session playdb.GameSession) (groupCheckEvent, string, error) {
		return s.rollOwnGroupCheck(ctx, q, tx, m, session, hold, checkID, in)
	})
	if err != nil {
		return nil, s.dbError(ctx, "roll a group check", err)
	}
	if hold.asking() {
		return connect.NewResponse(&playv1.RollGroupCheckResponse{InspirationOffer: hold.offer, LuckyOffer: hold.lucky, HeldRollOffer: hold.held}), nil
	}
	if !repeated {
		s.publishGroupRolled(m)
	}
	view, err := s.groupCheckView(ctx, m, checkID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.RollGroupCheckResponse{GroupCheck: view}), nil
}

// publishGroupRolled tells the master and the roller that a player rolled: the other players
// have nothing to read of it.
func (s *Service) publishGroupRolled(m authz.Membership) {
	s.hub.Publish(m.CampaignID, live.Event{
		Audience: live.Audience{Master: true, UserID: m.UserID},
		Message: &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_GroupCheckChanged_{
			GroupCheckChanged: &playv1.WatchGameSessionResponse_GroupCheckChanged{},
		}},
	})
}

// rollOwnGroupCheck rolls the caller's own character for the group check, in the dice mode the
// campaign has them roll in. A roll kept for an answer (hold.asking) writes no event.
func (s *Service) rollOwnGroupCheck(ctx context.Context, q *playdb.Queries, tx pgx.Tx, m authz.Membership, session playdb.GameSession, hold *groupHold, checkID string, in checkInput) (groupCheckEvent, string, error) {
	hold.offer, hold.lucky = nil, nil
	who, has, err := s.myCharacter(ctx, tx, m)
	if err != nil {
		return groupCheckEvent{}, "", err
	}
	if !has {
		return groupCheckEvent{}, "", errContest(playv1.ContestBlockedReason_CONTEST_BLOCKED_REASON_NOT_IN_GROUP_CHECK, "you have no living character in this check")
	}
	if outside, err := outsideOpenScene(ctx, q, session, m.UserID); err != nil || outside {
		if err != nil {
			return groupCheckEvent{}, "", err
		}
		return groupCheckEvent{}, "", errContest(playv1.ContestBlockedReason_CONTEST_BLOCKED_REASON_NOT_IN_GROUP_CHECK, "you have no living character in this check")
	}
	force, err := s.dice.ForcedDice(ctx, tx, m.CampaignID, m.UserID)
	if err != nil {
		return groupCheckEvent{}, "", err
	}
	if force.refuses(in.inApp) {
		return groupCheckEvent{}, "", errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_WRONG_DICE_MODE, "this is not how the campaign has you roll your dice")
	}
	if err := s.rollMember(ctx, q, tx, m.CampaignID, session, checkID, who.ID, in, false); err != nil {
		return groupCheckEvent{}, "", err
	}
	if hold.asking() {
		return groupCheckEvent{}, "", errRollHeld
	}
	return groupCheckEvent{GroupCheckID: checkID, CharacterID: who.ID}, who.ID, nil
}

// rollMember rolls a character's check for an open group check: its number for the skill
// and a normal d20 (the check has no combat, so no circumstance changes the roll).
func (s *Service) rollMember(ctx context.Context, q *playdb.Queries, tx pgx.Tx, campaignID string, session playdb.GameSession, checkID, characterID string, in checkInput, byMaster bool) error {
	check, err := openCheckOfMember(ctx, q, session.ID, checkID, characterID)
	if err != nil {
		return err
	}
	options, err := s.roster.SceneOptions(ctx, tx, campaignID, characterID, []string{check.SkillKey})
	if err != nil {
		return err
	}
	spec := memberRoll{check: check, characterID: characterID, in: in, byMaster: byMaster}
	if len(options) == 1 && options[0].Known {
		spec.bonus, spec.known, spec.lucky, spec.reliable = options[0].Bonus, true, options[0].Lucky, options[0].ReliableTalent
	}
	// The same circumstances as any ability check: Poisoned, Frightened, exhaustion, the
	// Rage on Strength, armor on Stealth and a Help (SRD 5.1, "Group Checks" are ability checks).
	ability, isCheck := checkKey(check.SkillKey)
	spec.mode, err = s.checkModeOf(ctx, tx, campaignID, session.ID, characterID, check.SkillKey, ability, isCheck)
	if err != nil {
		return err
	}
	roll, held, err := s.memberRollOf(ctx, q, tx, spec)
	if err != nil || held {
		return err
	}
	return s.keepMemberRoll(ctx, q, tx, session, spec, roll)
}

// openCheckOfMember is the open group check the character may still answer: it exists, is open,
// names the character and the character did not answer yet.
func openCheckOfMember(ctx context.Context, q *playdb.Queries, sessionID, checkID, characterID string) (playdb.GroupCheck, error) {
	check, err := q.GetGroupCheck(ctx, playdb.GetGroupCheckParams{GameSessionID: sessionID, ID: checkID})
	if errors.Is(err, pgx.ErrNoRows) {
		return playdb.GroupCheck{}, connect.NewError(connect.CodeNotFound, errors.New("group check not found"))
	}
	if err != nil {
		return playdb.GroupCheck{}, fmt.Errorf("find the group check: %w", err)
	}
	if check.Status != "open" {
		return playdb.GroupCheck{}, errContest(playv1.ContestBlockedReason_CONTEST_BLOCKED_REASON_GROUP_CHECK_CLOSED, "the group check is closed")
	}
	members, err := q.ListGroupCheckMembers(ctx, check.ID)
	if err != nil {
		return playdb.GroupCheck{}, fmt.Errorf("list the members: %w", err)
	}
	var member *playdb.GroupCheckMember
	for i := range members {
		if members[i].CharacterID == characterID {
			member = &members[i]
		}
	}
	switch {
	case member == nil:
		return playdb.GroupCheck{}, errContest(playv1.ContestBlockedReason_CONTEST_BLOCKED_REASON_NOT_IN_GROUP_CHECK, "the character is not in this check")
	case member.RolledAt != nil:
		return playdb.GroupCheck{}, errContest(playv1.ContestBlockedReason_CONTEST_BLOCKED_REASON_ALREADY_ANSWERED, "the character answered already")
	}
	return check, nil
}

// memberRoll is what rolling one character's group check needs: the check, who rolls and how,
// the character's modifier for the skill and the circumstances of the roll.
type memberRoll struct {
	check       playdb.GroupCheck
	characterID string
	in          checkInput
	byMaster    bool
	bonus       int
	known       bool
	lucky       bool
	reliable    bool // Reliable Talent counts a low d20 of this check as 10
	mode        checkMode
}

// memberRollOf makes the d20 of the member. held says the roll was kept for the player's answer
// (a Bardic Inspiration die, or the halfling's Lucky): nothing is written then.
//
// A character that holds a Bardic Inspiration die has the d20 kept until the player
// answers whether to use it, and so has a halfling's d20 that shows a 1, for the Lucky
// question (a roll the master makes for the player waits for no one).
func (s *Service) memberRollOf(ctx context.Context, q *playdb.Queries, tx pgx.Tx, spec memberRoll) (roll contestRoll, held bool, err error) {
	if gh := groupHoldOf(ctx); gh != nil && s.mayAskMember(ctx, tx, gh, spec) {
		var settled bool
		roll, settled, held, err = s.gatedMemberRoll(ctx, q, tx, gh, spec)
		if err != nil || held {
			return contestRoll{}, held, err
		}
		if settled {
			return roll, false, nil
		}
	}
	roll, err = s.rollCheck(spec.in, checkModeOfRoll(spec.mode.Mode), spec.bonus, spec.check.SkillKey, nil, !spec.known, spec.reliable)
	return roll, false, err
}

// mayAskMember says the member's roll may wait for an answer: the player's own always, the master's for a player's
// character only while a player plays it (a roll for a character nobody plays waits for nobody).
func (s *Service) mayAskMember(ctx context.Context, tx pgx.Tx, gh *groupHold, spec memberRoll) bool {
	if !spec.byMaster {
		return true
	}
	cs, err := s.roster.CombatCharacters(ctx, tx, gh.m.CampaignID, []string{spec.characterID})
	return err == nil && len(cs) == 1 && cs[0].PlayerUserID != ""
}

// gatedMemberRoll puts the member's d20 through the held-roll gate. It says whether the gate
// settled the roll (settled), or kept it for an answer (held).
func (s *Service) gatedMemberRoll(ctx context.Context, q *playdb.Queries, tx pgx.Tx, gh *groupHold, spec memberRoll) (roll contestRoll, settled, held bool, err error) {
	step, err := s.heldGate(ctx, s.outsideGate(tx, q), rollSpec{
		Kind: heldGroupCheck, Key: gh.key, Request: gh.raw, Roller: heldRoller{CampaignID: gh.m.CampaignID, CharacterID: spec.characterID, UserID: gh.m.UserID},
		In: rollInput{inApp: spec.in.inApp, typedFaces: spec.in.faces}, Modifier: spec.bonus, Mode: spec.mode.Mode, Reliable: spec.reliable, Lucky: spec.lucky, MayAsk: true,
		Stroke: groupStrokeRule(spec.check),
	})
	if err != nil {
		return contestRoll{}, false, false, err
	}
	if step.Held != nil {
		gh.offer, gh.lucky, gh.held = s.outsideOffers(ctx, tx, step.Held)
		return contestRoll{}, false, true, nil
	}
	if !step.Settled {
		return contestRoll{}, false, false, nil
	}
	roll = contestRoll{
		Skill: spec.check.SkillKey, Faces: faces32(step.Roll.Faces), Modifier: conv.Int32(spec.bonus),
		Total: conv.Int32(step.Roll.Total), Physical: step.Roll.Physical,
		Mode: checkModeKey(checkModeOfRoll(spec.mode.Mode)), Unknown: !spec.known, Lucky: step.Lucky, Stroke: step.Stroke,
		CountedAs: countedAs(step.Roll.Face(), countedFace(step.Roll.Face(), spec.reliable)),
	}
	if step.Bonus != nil {
		roll.BonusSides, roll.BonusFace = step.Bonus.Sides, step.Bonus.Face
	}
	return roll, true, false, nil
}

// keepMemberRoll spends the Helps the roll used and keeps the roll on the member.
func (s *Service) keepMemberRoll(ctx context.Context, q *playdb.Queries, tx pgx.Tx, session playdb.GameSession, spec memberRoll, roll contestRoll) error {
	roll.Notes = notesOfSources(spec.mode.Sources, spec.mode.hide)
	c, err := s.openTx(ctx, combatTx{tx: tx, q: q, session: session, now: s.now(), characterID: &spec.characterID})
	if err != nil {
		return err
	}
	if err := s.spendCheckHelps(ctx, c, spec.mode.Helps); err != nil {
		return err
	}
	roll.ByMaster = spec.byMaster
	stored, err := encodeRoll(roll)
	if err != nil {
		return err
	}
	now := s.now()
	if _, err := q.SetGroupCheckMemberRoll(ctx, playdb.SetGroupCheckMemberRollParams{
		GroupCheckID: spec.check.ID, CharacterID: spec.characterID, Roll: stored, RolledByMaster: spec.byMaster, RolledAt: &now,
	}); err != nil {
		return fmt.Errorf("keep the roll: %w", err)
	}
	return nil
}

// RollForPlayer implements playv1connect.ContestServiceHandler.
func (s *Service) RollForPlayer(
	ctx context.Context,
	req *connect.Request[playv1.RollForPlayerRequest],
) (*connect.Response[playv1.RollForPlayerResponse], error) {
	m, err := heldRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	checkID, err := parseCombatID(req.Msg.GetGroupCheckId(), "group check")
	if err != nil {
		return nil, err
	}
	characterID, err := parseCombatID(req.Msg.GetCharacterId(), "character")
	if err != nil {
		return nil, err
	}
	in, err := parseCheckInput(req.Msg.GetRoll())
	if err != nil {
		return nil, err
	}
	// The master's roll for a player asks like the player's own (E4-10): the master is asked in his own sheet, the
	// player reads the same question, and the first answer wins (AnswerHeldRoll).
	hold := &groupHold{m: m, key: key, raw: req.Msg}
	ctx = context.WithValue(ctx, groupHoldKey{}, hold)
	_, repeated, err := s.groupWrite(ctx, m, key, idem.Hash(req.Msg), eventGroupCheckRolled, func(q *playdb.Queries, tx pgx.Tx, session playdb.GameSession) (groupCheckEvent, string, error) {
		hold.offer, hold.lucky, hold.held = nil, nil, nil
		if err := s.rollMember(ctx, q, tx, m.CampaignID, session, checkID, characterID, in, true); err != nil {
			return groupCheckEvent{}, "", err
		}
		if hold.asking() {
			return groupCheckEvent{}, "", errRollHeld
		}
		return groupCheckEvent{GroupCheckID: checkID, CharacterID: characterID}, characterID, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "roll for a player", err)
	}
	if hold.asking() {
		return connect.NewResponse(&playv1.RollForPlayerResponse{InspirationOffer: hold.offer, LuckyOffer: hold.lucky, HeldRollOffer: hold.held}), nil
	}
	if !repeated {
		s.publishGroupCheckChanged(ctx, m.CampaignID)
	}
	view, err := s.groupCheckView(ctx, m, checkID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.RollForPlayerResponse{GroupCheck: view}), nil
}

// CloseGroupCheck implements playv1connect.ContestServiceHandler.
func (s *Service) CloseGroupCheck(
	ctx context.Context,
	req *connect.Request[playv1.CloseGroupCheckRequest],
) (*connect.Response[playv1.CloseGroupCheckResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	checkID, err := parseCombatID(req.Msg.GetGroupCheckId(), "group check")
	if err != nil {
		return nil, err
	}
	_, repeated, err := s.groupWrite(ctx, m, key, idem.Hash(req.Msg), eventGroupCheckClosed, func(q *playdb.Queries, _ pgx.Tx, session playdb.GameSession) (groupCheckEvent, string, error) {
		ev, err := s.closeGroupCheckRows(ctx, q, session.ID, checkID)
		return ev, "", err
	})
	if err != nil {
		return nil, s.dbError(ctx, "close a group check", err)
	}
	if !repeated {
		s.publishGroupCheckChanged(ctx, m.CampaignID)
	}
	view, err := s.groupCheckView(ctx, m, checkID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.CloseGroupCheckResponse{GroupCheck: view}), nil
}

// closeGroupCheckRows closes an open group check, with the group's verdict when there is a DC and a
// group to judge.
func (s *Service) closeGroupCheckRows(ctx context.Context, q *playdb.Queries, sessionID, checkID string) (groupCheckEvent, error) {
	check, err := q.GetGroupCheck(ctx, playdb.GetGroupCheckParams{GameSessionID: sessionID, ID: checkID})
	if errors.Is(err, pgx.ErrNoRows) {
		return groupCheckEvent{}, connect.NewError(connect.CodeNotFound, errors.New("group check not found"))
	}
	if err != nil {
		return groupCheckEvent{}, fmt.Errorf("find the group check: %w", err)
	}
	if check.Status != "open" {
		return groupCheckEvent{}, errContest(playv1.ContestBlockedReason_CONTEST_BLOCKED_REASON_GROUP_CHECK_CLOSED, "the group check is closed")
	}
	members, err := q.ListGroupCheckMembers(ctx, check.ID)
	if err != nil {
		return groupCheckEvent{}, fmt.Errorf("list the members: %w", err)
	}
	if _, err := q.CloseGroupCheck(ctx, playdb.CloseGroupCheckParams{GameSessionID: sessionID, ID: check.ID, Passed: groupVerdictOf(check, members), ClosedAt: new(s.now())}); err != nil {
		return groupCheckEvent{}, fmt.Errorf("close the group check: %w", err)
	}
	return groupCheckEvent{GroupCheckID: check.ID}, nil
}

// groupVerdictOf is the verdict a group check closes with: a character that did not answer counts
// as failed. It is nil when there is no DC, or the rolls are judged one by one.
func groupVerdictOf(check playdb.GroupCheck, members []playdb.GroupCheckMember) *bool {
	if check.Dc == nil || !check.IsGroup {
		return nil
	}
	passed := 0
	for _, mb := range members {
		if roll, err := decodeRoll(mb.Roll); err == nil && roll != nil && int(roll.Total) >= int(*check.Dc) {
			passed++
		}
	}
	return new(combat.GroupCheckPasses(passed, len(members)))
}

// GetGroupCheck implements playv1connect.ContestServiceHandler.
func (s *Service) GetGroupCheck(
	ctx context.Context,
	req *connect.Request[playv1.GetGroupCheckRequest],
) (*connect.Response[playv1.GetGroupCheckResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	view, err := s.latestGroupCheckView(ctx, m)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.GetGroupCheckResponse{GroupCheck: view}), nil
}

// latestGroupCheckView is the open group check as the caller reads it, or the latest one.
func (s *Service) latestGroupCheckView(ctx context.Context, m authz.Membership) (*playv1.GroupCheckView, error) {
	var view *playv1.GroupCheckView
	var check playdb.GroupCheck
	var members []playdb.GroupCheckMember
	err := db.ReadTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		session, err := openSessionWith(ctx, q, m.CampaignID)
		if err != nil {
			return err
		}
		check, err = q.GetOpenGroupCheck(ctx, session.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			check, err = q.GetLatestGroupCheck(ctx, session.ID)
		}
		if errors.Is(err, pgx.ErrNoRows) {
			check = playdb.GroupCheck{}
			return nil
		}
		if err != nil {
			return fmt.Errorf("find the group check: %w", err)
		}
		members, err = q.ListGroupCheckMembers(ctx, check.ID)
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "read the group check", err)
	}
	if check.ID == "" {
		return nil, nil
	}
	if view, err = s.buildGroupCheckView(ctx, m, check, members); err != nil {
		return nil, s.dbError(ctx, "read the group check", err)
	}
	return view, nil
}

// groupCheckView reads a group check back as the caller may read it.
func (s *Service) groupCheckView(ctx context.Context, m authz.Membership, checkID string) (*playv1.GroupCheckView, error) {
	m = heldViewer(ctx, m) // a held roll's answer renders for who answered
	var check playdb.GroupCheck
	var members []playdb.GroupCheckMember
	err := db.ReadTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		session, err := openSessionWith(ctx, q, m.CampaignID)
		if err != nil {
			return err
		}
		if check, err = q.GetGroupCheck(ctx, playdb.GetGroupCheckParams{GameSessionID: session.ID, ID: checkID}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return connect.NewError(connect.CodeNotFound, errors.New("group check not found"))
			}
			return fmt.Errorf("find the group check: %w", err)
		}
		members, err = q.ListGroupCheckMembers(ctx, check.ID)
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "read the group check", err)
	}
	view, err := s.buildGroupCheckView(ctx, m, check, members)
	if err != nil {
		return nil, s.dbError(ctx, "read the group check", err)
	}
	return view, nil
}

// groupViewer is who reads a group check: the master, or a player and their character.
type groupViewer struct {
	master  bool
	mine    link.Character
	hasMine bool
}

// buildGroupCheckView is a group check as the caller may read it: the master everything;
// a player their own roll, and passed or failed only when the master shows the DC.
func (s *Service) buildGroupCheckView(ctx context.Context, m authz.Membership, check playdb.GroupCheck, members []playdb.GroupCheckMember) (*playv1.GroupCheckView, error) {
	viewer, asked, err := s.groupViewerOf(ctx, m, members)
	if err != nil || !asked {
		return nil, err
	}
	out := s.groupCheckHeader(check, len(members), viewer.master)
	passed, err := s.addGroupMembers(ctx, out, m.CampaignID, check, members, viewer)
	if err != nil {
		return nil, err
	}
	setGroupVerdict(out, check, viewer.master, passed, len(members))
	return out, nil
}

// addGroupMembers adds to the view the members the viewer may read (all for the master, the
// viewer's own for a player) and counts the ones who passed, of all the members.
func (s *Service) addGroupMembers(ctx context.Context, out *playv1.GroupCheckView, campaignID string, check playdb.GroupCheck, members []playdb.GroupCheckMember, viewer groupViewer) (int, error) {
	names := s.groupMemberNames(ctx, campaignID, members)
	passed := 0
	for _, mb := range members {
		roll, err := decodeRoll(mb.Roll)
		if err != nil {
			return 0, err
		}
		didPass := roll != nil && check.Dc != nil && int(roll.Total) >= int(*check.Dc)
		if didPass {
			passed++
		}
		isMine := viewer.hasMine && mb.CharacterID == viewer.mine.ID
		if !viewer.master && !isMine {
			continue // a player reads their own roll only
		}
		out.Members = append(out.Members, groupMemberViewOf(check, mb, roll, names[mb.CharacterID], viewer.master, didPass))
		if isMine && roll == nil {
			s.offerOwnRoll(ctx, out, campaignID, check, viewer.mine.ID)
		}
	}
	return passed, nil
}

// offerOwnRoll tells the player that their character may roll the open check now, and what the roll adds.
func (s *Service) offerOwnRoll(ctx context.Context, out *playv1.GroupCheckView, campaignID string, check playdb.GroupCheck, characterID string) {
	if check.Status != "open" {
		return
	}
	out.YouRoll = true
	out.YourOption = s.groupYourOption(ctx, campaignID, check, characterID)
}

// groupViewerOf says who reads the check. asked is false for a player who was not asked, who
// reads no group check at all (RN-10).
func (s *Service) groupViewerOf(ctx context.Context, m authz.Membership, members []playdb.GroupCheckMember) (viewer groupViewer, asked bool, err error) {
	viewer.master = m.Role == authz.RoleMaster
	if viewer.master {
		return viewer, true, nil
	}
	if viewer.mine, viewer.hasMine, err = s.myCharacter(ctx, nil, m); err != nil {
		return groupViewer{}, false, err
	}
	asked = viewer.hasMine && slices.ContainsFunc(members, func(mb playdb.GroupCheckMember) bool { return mb.CharacterID == viewer.mine.ID })
	if !asked {
		return viewer, false, nil
	}
	// A player out of the scene open to only some players reads no group check (RN-10).
	session, err := s.queries.GetOpenGameSession(ctx, m.CampaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return viewer, asked, nil
	}
	if err != nil {
		return groupViewer{}, false, fmt.Errorf("find the open session: %w", err)
	}
	outside, err := outsideOpenScene(ctx, s.queries, session, m.UserID)
	return viewer, asked && !outside, err
}

// groupCheckHeader is the part of the view that does not depend on the rolls: the master alone
// reads the DC and how many were asked.
func (s *Service) groupCheckHeader(check playdb.GroupCheck, askedCount int, master bool) *playv1.GroupCheckView {
	out := &playv1.GroupCheckView{
		Id: check.ID, SkillKey: check.SkillKey, SkillNamePt: s.roster.SceneCheckName(check.SkillKey), Open: check.Status == "open",
		ShowDc: check.ShowDc, CreatedAt: timestamppb.New(check.CreatedAt),
		Group: check.IsGroup, Save: strings.HasPrefix(check.SkillKey, "save:"),
	}
	if master {
		out.AskedCount = clamp32(askedCount, 0, math.MaxInt32)
		if check.Dc != nil {
			out.Dc = *check.Dc
		}
	}
	return out
}

// groupMemberNames are the names of the characters of the check, by ID. A roster that cannot
// say leaves the names empty.
func (s *Service) groupMemberNames(ctx context.Context, campaignID string, members []playdb.GroupCheckMember) map[string]string {
	names := map[string]string{}
	ids := make([]string, 0, len(members))
	for _, mb := range members {
		ids = append(ids, mb.CharacterID)
	}
	if chars, err := s.roster.CombatCharacters(ctx, nil, campaignID, ids); err == nil {
		for _, ch := range chars {
			names[ch.ID] = ch.Name
		}
	}
	return names
}

// groupMemberViewOf is one member as the viewer reads it: the roll once answered, and whether it
// passed only to the master, or to the player when the master shows the DC.
func groupMemberViewOf(check playdb.GroupCheck, mb playdb.GroupCheckMember, roll *contestRoll, name string, master, didPass bool) *playv1.GroupCheckMemberView {
	view := &playv1.GroupCheckMemberView{CharacterId: mb.CharacterID, Name: name, Answered: roll != nil}
	if roll == nil {
		return view
	}
	view.Roll = roll.proto()
	if !master {
		view.Roll.BonusKnown = true
	}
	if check.Dc != nil && (master || check.ShowDc) {
		view.PassedKnown, view.Passed = true, didPass
	}
	return view
}

// groupYourOption is what the player's own character would roll, or nil when the roster knows no
// number for it.
func (s *Service) groupYourOption(ctx context.Context, campaignID string, check playdb.GroupCheck, characterID string) *playv1.CheckOption {
	options, err := s.roster.SceneOptions(ctx, nil, campaignID, characterID, []string{check.SkillKey})
	if err != nil || len(options) != 1 {
		return nil
	}
	// The mode the roll will have, so a player with physical dice types the right
	// number of faces.
	mode := playv1.RollModeKind_ROLL_MODE_KIND_NORMAL
	ability, isCheck := checkKey(check.SkillKey)
	if cm, err := s.checkModeOf(ctx, nil, campaignID, check.GameSessionID, characterID, check.SkillKey, ability, isCheck); err == nil {
		mode = checkModeProto(checkModeOfRoll(cm.Mode))
	}
	return &playv1.CheckOption{Modifier: conv.Int32(options[0].Bonus), Known: options[0].Known, Mode: mode}
}

// setGroupVerdict sets the group's count and verdict on the view: none when each roll is judged
// alone; the master always (with a DC); the players only when the check is closed with the DC shown.
func setGroupVerdict(out *playv1.GroupCheckView, check playdb.GroupCheck, master bool, passed, askedCount int) {
	switch {
	case !check.IsGroup:
		// Each roll is judged alone: no count and no group verdict.
	case master:
		out.PassedCount = clamp32(passed, 0, math.MaxInt32)
		out.Needed = clamp32(combat.GroupCheckNeeded(askedCount), 0, math.MaxInt32)
		if check.Dc != nil {
			out.VerdictKnown, out.GroupPassed = true, combat.GroupCheckPasses(passed, askedCount)
		}
	case check.Status != "open" && check.ShowDc && check.Passed != nil:
		out.VerdictKnown, out.GroupPassed = true, *check.Passed
	}
}
