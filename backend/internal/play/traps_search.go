package play

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	maplink "github.com/PuraFome/meuRPG/backend/internal/maps/link"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/platform/logging"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// Procurar armadilhas (MR-035): the player's search for traps, in phases. SearchForTraps reads and
// validates the request, the transaction finds the searcher's place, rolls the d20 and asks the
// maps module what the roll finds, and the answer is built from the event the history keeps.

// The faces of a d20.
const (
	searchMinFace = 1
	searchMaxFace = 20
)

// trapSearch is one SearchForTraps call: the request as it was read, and what its transaction
// works out for the answer and for the streams after the commit.
type trapSearch struct {
	m      authz.Membership
	key    string
	hash   *string
	req    *playv1.SearchForTrapsRequest
	who    link.Character
	skill  string
	in     rollInput
	typed2 int // the face typed for the second die of a Perception search

	ev       actionEvent
	shown    []*playv1.AdvantageSource // the circumstances of the roll's mode, for the searcher
	repeated bool
	told     []string
	place    searchPlace
	offer    *playv1.OutsideInspirationOffer // the d20 was rolled and kept for the Bardic Inspiration question
	lucky    *playv1.LuckyOffer              // or for the halfling's Lucky question, which comes first
}

// searchTurn is what the transaction of a search works out before the dice: the character's numbers
// for the check, its mode and the answer of the held-roll gate.
type searchTurn struct {
	c        *combatTx
	opt      link.SceneOption
	cm       checkMode
	names    func(string) string
	bonus    int
	reliable bool
	step     gateResult
	dieFace  int // the die the player added after rolling: it counts in every total of the search
}

// searchDice is the d20 of a search and the totals it is answered with.
type searchDice struct {
	face, face2, counted int
	roll                 dice.Result
	totals               []int
}

// SearchForTraps implements playv1connect.PlayServiceHandler.
func (s *Service) SearchForTraps(
	ctx context.Context,
	req *connect.Request[playv1.SearchForTrapsRequest],
) (*connect.Response[playv1.SearchForTrapsResponse], error) {
	ts, err := s.readTrapSearch(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error { return s.searchInTx(ctx, tx, ts) })
	if err != nil {
		return nil, s.dbError(ctx, "search for traps", err)
	}
	if ts.offer != nil || ts.lucky != nil { // nothing was written: the question is the whole answer
		return connect.NewResponse(&playv1.SearchForTrapsResponse{InspirationOffer: ts.offer, LuckyOffer: ts.lucky}), nil
	}
	if !ts.repeated {
		s.tellTrapSearch(ctx, ts)
	}
	return connect.NewResponse(trapSearchAnswer(ts)), nil
}

// readTrapSearch checks who calls and the request, and finds the caller's character.
func (s *Service) readTrapSearch(ctx context.Context, msg *playv1.SearchForTrapsRequest) (*trapSearch, error) {
	m, err := authz.RequireCampaignRole(ctx, msg.GetCampaignId(), authz.RolePlayer)
	if err != nil {
		return nil, err
	}
	key, err := parseKey(msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	skill, err := s.searchSkillOf(msg)
	if err != nil {
		return nil, err
	}
	in, typed2, err := searchRollOf(msg, skill)
	if err != nil {
		return nil, err
	}
	if s.traps == nil {
		return nil, errNoTraps()
	}
	who, has, err := s.myCharacter(ctx, nil, m)
	if err != nil {
		return nil, s.dbError(ctx, "find the caller's character", err)
	}
	if !has {
		return nil, errScene(playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_NO_CHARACTER, "you have no living character to search with")
	}
	return &trapSearch{m: m, key: key, hash: idem.Hash(msg), req: msg, who: who, skill: skill, in: in, typed2: typed2}, nil
}

// searchSkillOf is the skill the request searches with: its key for the check, or the SRD skill of
// OTHER.
func (s *Service) searchSkillOf(msg *playv1.SearchForTrapsRequest) (string, error) {
	var skill string
	switch msg.GetSkill() {
	case playv1.TrapSearchSkill_TRAP_SEARCH_SKILL_PERCEPTION:
		skill = searchPerception
	case playv1.TrapSearchSkill_TRAP_SEARCH_SKILL_INVESTIGATION:
		skill = searchInvestigation
	case playv1.TrapSearchSkill_TRAP_SEARCH_SKILL_OTHER:
		// Any other skill of the SRD: the player's menu is the same for every trap, and the
		// server counts the skill only for the traps that allow it.
		key := msg.GetOtherSkillKey()
		if key == searchCheckKey[searchPerception] || key == searchCheckKey[searchInvestigation] || !strings.HasPrefix(key, "skill:") || s.roster.SceneCheckName(key) == "" {
			return "", connect.NewError(connect.CodeInvalidArgument, errors.New("other_skill_key must be a skill of the SRD that is neither Perception nor Investigation"))
		}
		skill = key
	default:
		return "", connect.NewError(connect.CodeInvalidArgument, errors.New("skill must be PERCEPTION, INVESTIGATION or OTHER"))
	}
	if msg.GetSkill() != playv1.TrapSearchSkill_TRAP_SEARCH_SKILL_OTHER && msg.GetOtherSkillKey() != "" {
		return "", connect.NewError(connect.CodeInvalidArgument, errors.New("other_skill_key is only for a search with OTHER"))
	}
	return skill, nil
}

// searchRollOf is how the d20 comes: in the app or typed, with the face of the second die of a
// Perception search that a real die typed.
func searchRollOf(msg *playv1.SearchForTrapsRequest, skill string) (rollInput, int, error) {
	var in rollInput
	in.extraFaces = msg.GetExtraDieFaces()
	switch roll := msg.GetRoll().(type) {
	case *playv1.SearchForTrapsRequest_RollInApp:
		if !roll.RollInApp {
			return in, 0, connect.NewError(connect.CodeInvalidArgument, errors.New("roll_in_app must be true"))
		}
		in.inApp = true
	case *playv1.SearchForTrapsRequest_D20Face:
		in.typed = int(roll.D20Face)
		if in.typed < searchMinFace || in.typed > searchMaxFace {
			return in, 0, connect.NewError(connect.CodeInvalidArgument, errors.New("d20_face must be 1 to 20"))
		}
	default:
		var err error
		if in.typedFaces, err = typedD20Faces(msg.GetD20Faces()); err != nil {
			return in, 0, err
		}
	}
	typed2, err := searchSecondFace(msg, in, skill)
	return in, typed2, err
}

// searchSecondFace is d20_face_2, read only where it counts: a Perception search with a typed die.
func searchSecondFace(msg *playv1.SearchForTrapsRequest, in rollInput, skill string) (int, error) {
	f := msg.D20Face_2
	if f == nil || in.inApp || skill != searchPerception {
		return 0, nil
	}
	typed2 := int(*f)
	if typed2 < searchMinFace || typed2 > searchMaxFace {
		return 0, connect.NewError(connect.CodeInvalidArgument, errors.New("d20_face_2 must be 1 to 20"))
	}
	return typed2, nil
}

// searchInTx is the change: the retry of a search already made, the checks of the state, the place,
// the d20, what the roll finds and the event.
func (s *Service) searchInTx(ctx context.Context, tx pgx.Tx, ts *trapSearch) error {
	q := s.queries.WithTx(tx)
	ts.repeated, ts.told, ts.place, ts.offer, ts.lucky = false, nil, searchPlace{}, nil, nil
	session, err := lockOpenSession(ctx, q, ts.m.CampaignID)
	if err != nil {
		return err
	}
	if done, err := s.searchRetried(ctx, q, session, ts); done || err != nil {
		return err
	}
	opt, err := s.searchGuards(ctx, tx, q, session, ts)
	if err != nil {
		return err
	}
	c, err := s.openTx(ctx, combatTx{tx: tx, q: q, session: session, now: s.now(), characterID: &ts.who.ID, kind: eventTrapSearched, actorUserID: ts.m.UserID, hash: ts.hash})
	if err != nil {
		return err
	}
	if ts.place, err = s.searcherPlace(ctx, c, ts.who); err != nil {
		return err
	}
	r := &searchTurn{c: c, opt: opt, reliable: opt.ReliableTalent}
	held, err := s.prepareSearchRoll(ctx, tx, q, ts, r)
	if err != nil || held {
		return err
	}
	d, err := s.rollSearch(ts, r)
	if err != nil {
		return err
	}
	return s.writeTrapSearch(ctx, ts, r, d)
}

// searchRetried answers a retry of a search already made with that search, and changes nothing. It
// says whether the change is over.
func (s *Service) searchRetried(ctx context.Context, q *playdb.Queries, session playdb.GameSession, ts *trapSearch) (bool, error) {
	done, err := q.GetSessionEventByIdempotencyKey(ctx, playdb.GetSessionEventByIdempotencyKeyParams{GameSessionID: session.ID, IdempotencyKey: &ts.key})
	switch {
	case err == nil:
		if done.Kind != eventTrapSearched || done.ActorUserID == nil || *done.ActorUserID != ts.m.UserID || hashDiffers(done.IdempotencyHash, ts.hash) {
			return true, errKeyReused()
		}
		ts.repeated = true
		return true, json.Unmarshal(done.Payload, &ts.ev)
	case !errors.Is(err, pgx.ErrNoRows):
		return true, fmt.Errorf("find the event of this idempotency key: %w", err)
	}
	return false, nil
}

// searchGuards are the refusals of a search that come from the state: a combat without a map, the
// dice the campaign allows and the numbers the character has. It returns the character's option for
// the check.
func (s *Service) searchGuards(ctx context.Context, tx pgx.Tx, q *playdb.Queries, session playdb.GameSession, ts *trapSearch) (link.SceneOption, error) {
	if theatre, err := theatreRunning(ctx, q, session.ID); err != nil {
		return link.SceneOption{}, err
	} else if theatre {
		return link.SceneOption{}, errNeedsAMap() // a combat without a map has no traps to search for (RN-25)
	}
	force, err := s.dice.ForcedDice(ctx, tx, ts.m.CampaignID, ts.m.UserID)
	if err != nil {
		return link.SceneOption{}, err
	}
	if force.refuses(ts.in.inApp) {
		return link.SceneOption{}, errScene(playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_WRONG_DICE_MODE, "this is not how the campaign has you roll your dice")
	}
	options, err := s.roster.SceneOptions(ctx, tx, ts.m.CampaignID, ts.who.ID, []string{checkKeyOf(ts.skill)})
	if err != nil {
		return link.SceneOption{}, err
	}
	if len(options) != 1 || !options[0].Known {
		return link.SceneOption{}, errScene(playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_NO_CHARACTER, "your character has no numbers for this check")
	}
	return options[0], nil
}

// prepareSearchRoll works out the mode and the bonus of the check, asks the held-roll gate and spends
// what the roll uses up. It says whether the d20 was kept for a question: then nothing else is
// written.
func (s *Service) prepareSearchRoll(ctx context.Context, tx pgx.Tx, q *playdb.Queries, ts *trapSearch, r *searchTurn) (bool, error) {
	ability, isCheck := checkKey(searchCheckKey[ts.skill])
	cm, err := s.checkModeOf(ctx, tx, ts.m.CampaignID, r.c.session.ID, ts.who.ID, searchCheckKey[ts.skill], ability, isCheck)
	// Reliable Talent: a d20 of 9 or lower counts as 10 for a skill the character is
	// proficient in, each die of a search with disadvantage before the lower is
	// chosen. Not for the light's penalty: that is the second die, as before.
	if err != nil {
		return false, err
	}
	r.cm = cm
	if r.names, err = s.namerFor(ctx, tx, ts.m.CampaignID); err != nil {
		return false, err
	}
	ts.shown = shownSources(r.cm.Sources, r.names)
	if r.bonus, err = s.withEffectDice(ts.in, ts.in.extraFaces, &r.cm, r.opt.Bonus); err != nil {
		return false, err
	}
	if held, err := s.askSearchGate(ctx, tx, q, ts, r); held || err != nil {
		return held, err
	}
	if r.step.Bonus != nil {
		r.dieFace = int(r.step.Bonus.Face)
	}
	if err := s.spendOnceEffects(ctx, r.c, r.cm.Rolled); err != nil {
		return false, err
	}
	if err := s.spendCheckHelps(ctx, r.c, r.cm.Helps); err != nil {
		return false, err
	}
	ts.shown = r.cm.shownCheck(r.names)
	return false, nil
}

// askSearchGate keeps the d20 until the player answers when a character out of a combat holds a
// Bardic Inspiration die, before anything is spent or written (the Search action of a combat has spent
// the action by now, and rolls as always), and so for a halfling's d20 that shows a 1, for the Lucky
// question. The answer brings the d20 back. It says whether a question is open.
func (s *Service) askSearchGate(ctx context.Context, tx pgx.Tx, q *playdb.Queries, ts *trapSearch, r *searchTurn) (bool, error) {
	if ts.place.spent {
		return false, nil
	}
	held := ts.in
	if len(ts.in.typedFaces) > 0 && r.cm.Mode == combat.ModeNormal { // the second die of a Perception search is not the held one
		held.typed, held.typedFaces = ts.in.typedFaces[0], nil
	}
	step, err := s.heldGate(ctx, s.outsideGate(tx, q), rollSpec{
		Kind: heldTrapSearch, Key: ts.key, Request: ts.req, Roller: heldRoller{CampaignID: ts.m.CampaignID, CharacterID: ts.who.ID, UserID: ts.m.UserID},
		In: held, Modifier: r.bonus, Mode: r.cm.Mode, Reliable: r.reliable, Lucky: r.opt.Lucky, MayAsk: true,
	})
	if err != nil {
		return false, err
	}
	r.step = step
	if step.Held == nil {
		return false, nil
	}
	ts.offer, ts.lucky = s.outsideOffers(ctx, tx, step.Held)
	return true, nil
}

// rollSearch rolls the d20 of the search, or takes the one the gate decided.
func (s *Service) rollSearch(ts *trapSearch, r *searchTurn) (searchDice, error) {
	if r.cm.Mode != combat.ModeNormal {
		return s.rollSearchWithMode(ts, r)
	}
	return s.rollSearchNormal(ts, r)
}

// rollSearchWithMode is a roll under advantage or disadvantage: two dice, the better or the worse
// counts. Where the light is dim to a Perception searcher, that disadvantage cancels an advantage
// and doubles up with another disadvantage: the second total is the one for the dim squares (SRD 5.1,
// "Advantage and Disadvantage").
func (s *Service) rollSearchWithMode(ts *trapSearch, r *searchTurn) (searchDice, error) {
	rolled := r.step.Roll
	if !r.step.Settled {
		var err error
		if rolled, err = s.d20With(ts.in, r.bonus, r.cm.Mode); err != nil {
			return searchDice{}, err
		}
		rolled = reliableD20(rolled, r.bonus, r.reliable)
	}
	d := searchDice{
		face: rolled.Faces[0], face2: rolled.Faces[1], counted: rolled.Index,
		roll: dice.Result{Total: rolled.Total, Physical: rolled.Physical}, totals: []int{rolled.Total},
	}
	if ts.skill != searchPerception {
		return d, nil
	}
	if r.cm.Mode == combat.ModeAdvantage {
		d.totals = append(d.totals, countedFace(d.face, r.reliable)+r.bonus+r.dieFace)
	} else {
		d.totals = append(d.totals, rolled.Total)
	}
	return d, nil
}

// rollSearchNormal is a roll with one d20; a Perception search is rolled twice in the app (the lower
// counts where the light is dim to the searcher), and with a real die the second one is typed.
func (s *Service) rollSearchNormal(ts *trapSearch, r *searchTurn) (searchDice, error) {
	in, typed2 := ts.in, ts.typed2
	if len(in.typedFaces) > 0 {
		in.typed = in.typedFaces[0]
		if len(in.typedFaces) > 1 && ts.skill == searchPerception {
			typed2 = in.typedFaces[1]
		}
	}
	var d searchDice
	if r.step.Settled {
		d.face, d.roll = r.step.Roll.Face(), dice.Result{Total: r.step.Roll.Total, Physical: r.step.Roll.Physical}
	} else {
		var err error
		if d.face, d.roll, err = s.checkD20(in, r.bonus, r.reliable); err != nil {
			return d, err
		}
	}
	d.totals = []int{d.roll.Total}
	switch {
	case ts.skill != searchPerception:
	case in.inApp:
		face2, roll2, err := s.checkD20(rollInput{inApp: true}, r.opt.Bonus, r.reliable)
		if err != nil {
			return d, err
		}
		d.face2 = face2
		d.totals = append(d.totals, roll2.Total+r.dieFace)
	case typed2 > 0:
		d.face2 = typed2
		d.totals = append(d.totals, countedFace(typed2, r.reliable)+r.opt.Bonus+r.dieFace)
	}
	return d, nil
}

// writeTrapSearch asks the maps module what the roll finds and writes the event. Only the master
// and the searcher hear of the roll (RN-20); the history keeps the ids and the numbers, never the DC.
func (s *Service) writeTrapSearch(ctx context.Context, ts *trapSearch, r *searchTurn, d searchDice) error {
	c := r.c
	found, err := s.traps.SearchTraps(ctx, c.tx, ts.m.CampaignID, ts.place.mapID, maplink.Observer{CharacterID: ts.who.ID, At: ts.place.at}, ts.skill, d.totals, c.now)
	if errors.Is(err, maplink.ErrSearchNeedsTwoDice) {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_SEARCH_NEEDS_TWO_DICE, "a Perception search in dim light has disadvantage: roll a second die")
	}
	if err != nil {
		return err
	}
	ts.told = found.Users
	ts.ev = trapSearchEvent(ts, r, d, found)
	if ts.place.spent {
		if err = touchEncounter(ctx, c); err != nil {
			return err
		}
		ts.place.enc = c.enc
	}
	return insertEvent(ctx, c, eventTrapSearched, &ts.m.UserID, &ts.key, ts.ev)
}

// trapSearchEvent is what the history keeps of the search.
func trapSearchEvent(ts *trapSearch, r *searchTurn, d searchDice, found maplink.SearchResult) actionEvent {
	ev := actionEvent{
		Round: r.c.enc.Round, Actor: ts.place.combatantID, Key: ts.skill, D20: clampInt32(d.face), Modifier: clampInt32(r.opt.Bonus + r.dieFace), Total: clampInt32(d.roll.Total),
		Physical: d.roll.Physical, Found: found.Found, OnTurn: ts.place.spent, D20B: clampInt32(d.face2), ReliableTalent: r.reliable,
		Lucky: r.step.Lucky, // the held d20 Lucky rolled again: the first of a Perception search's two
	}
	if r.cm.Mode != combat.ModeNormal {
		ev.RollMode, ev.Counted = modeKey(r.cm.Mode), clampInt32(d.counted)
	}
	if r.step.Bonus != nil {
		ev.Res = &resourceEvent{Kind: resBardicUse, Sides: r.step.Bonus.Sides, Face: r.step.Bonus.Face}
	}
	return ev
}

// tellTrapSearch tells the streams of a search that was just written.
func (s *Service) tellTrapSearch(ctx context.Context, ts *trapSearch) {
	pctx, stop := afterCommit(ctx) // the change is committed: a caller that hangs up must not leave the players unheard
	defer stop()
	if len(ts.ev.Found) > 0 {
		logging.Event(ctx, s.logger, "trap.noticed", slog.String("map_id", ts.place.mapID), slog.Int("found", len(ts.ev.Found)), slog.Bool("physical_dice", ts.ev.Physical))
	}
	if len(ts.told) > 0 {
		s.traps.Told(pctx, ts.m.CampaignID, ts.place.mapID, ts.told)
	}
	if ts.place.spent {
		s.publishEncounterChanged(pctx, ts.m.CampaignID, ts.place.enc)
		s.publishLogChanged(pctx, ts.m.CampaignID, ts.place.enc.ID, false) // the master's log has the search
	}
	s.Publish(ts.m.CampaignID, false, mapChangedHint(ts.place.mapID)) // the master's activity read has a new line: a hint with no content
}

// trapSearchAnswer is the response for the event of the search.
func trapSearchAnswer(ts *trapSearch) *playv1.SearchForTrapsResponse {
	ev := ts.ev
	out := &playv1.SearchForTrapsResponse{
		Roll:          checkRoll(ev.D20, ev.Modifier, ev.Physical, ev.ReliableTalent),
		FoundPointIds: ev.Found,
		SpentAction:   ev.OnTurn,
		Mode:          modeToProto[modeOfKey(ev.RollMode)],
		Sources:       ts.shown,
		BonusDice:     bonusDiceOf(ev),
	}
	if ev.RollMode != "" {
		out.Roll = diceRoll(2, 20, []int32{ev.D20, ev.D20B}, ev.Modifier, ev.Total, ev.Physical)
		out.Roll.CountedIndex = ev.Counted
		markTreated(out.Roll, ev.Modifier, ev.Total)
	} else if ev.D20B != 0 {
		out.SecondRoll = checkRoll(ev.D20B, ev.Modifier, ev.Physical, ev.ReliableTalent)
	}
	out.Roll.Rerolled = ev.Lucky.proto() // "Sortudo: 1 → 14", with the d20 it changed
	return out
}

// searchPlace is where a character searches from, and what the search costs.
type searchPlace struct {
	mapID string
	at    grid.Square
	// enc is the combat the character is in and combatantID its combatant; spent
	// says the search took the action of its turn.
	enc         playdb.Encounter
	combatantID string
	spent       bool
}

// errSearchNotOnMap is the refusal of a search by a character with no square on a map with a grid.
func errSearchNotOnMap() error {
	return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TRAP_NOT_ON_MAP, "your character is not on a map with a grid")
}

// searcherPlace finds where the character searches from, inside the change's
// transaction. While a combat is running and the character is one of its
// combatants it is the SRD's Search action: the character's own turn, one action,
// from its combatant's square on the combat's map; the action is spent here. Any
// other time it is the session's current map and the character's token on it.
func (s *Service) searcherPlace(ctx context.Context, c *combatTx, who link.Character) (searchPlace, error) {
	latest, err := c.q.GetLatestEncounter(ctx, c.session.ID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return searchPlace{}, fmt.Errorf("find the session's combat: %w", err)
	}
	if err == nil && latest.Status != statusEnded {
		if place, in, err := s.searchFromCombat(ctx, c, latest, who); in || err != nil {
			return place, err
		}
	}
	return s.searchFromMap(ctx, c, who)
}

// searchFromCombat is the Search action of a combatant of the running combat. It says whether the
// character is one of its combatants.
func (s *Service) searchFromCombat(ctx context.Context, c *combatTx, latest playdb.Encounter, who link.Character) (searchPlace, bool, error) {
	cs, err := c.q.ListCombatants(ctx, latest.ID)
	if err != nil {
		return searchPlace{}, false, fmt.Errorf("list the combatants: %w", err)
	}
	i := slices.IndexFunc(cs, func(o playdb.Combatant) bool { return o.Kind == kindPlayer && o.CharacterID == who.ID })
	if i < 0 {
		return searchPlace{}, false, nil
	}
	me := cs[i]
	if latest.Status != statusActive {
		return searchPlace{}, true, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TRAP_SEARCH_NOT_NOW, "the combat has not begun: search when it is your turn")
	}
	c.enc = latest
	if err := s.spendSearchAction(ctx, c, latest, me); err != nil {
		return searchPlace{}, true, err
	}
	return searchPlace{mapID: *latest.MapID, at: squareOfCombatant(me), enc: latest, combatantID: me.ID, spent: true}, true, nil
}

// spendSearchAction checks that it is the combatant's turn, with the action free and a square on the
// combat's map, and spends the action.
func (s *Service) spendSearchAction(ctx context.Context, c *combatTx, latest playdb.Encounter, me playdb.Combatant) error {
	if err := s.mustActNow(ctx, c, me); err != nil {
		return err
	}
	if me.ActionUsed {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ACTION_USED, "the action of this turn is used")
	}
	if latest.MapID == nil || !placed(me) {
		return errSearchNotOnMap()
	}
	run, err := breakRun(ctx, c, me)
	if err != nil {
		return err
	}
	_ = run // a search is no undoable action: the running start is simply lost
	if err := c.q.SetCombatantEconomy(ctx, playdb.SetCombatantEconomyParams{
		ID: me.ID, ActionUsed: true, BonusActionUsed: me.BonusActionUsed, ReactionUsed: me.ReactionUsed, Dashed: me.Dashed,
	}); err != nil {
		return fmt.Errorf("spend the action: %w", err)
	}
	return nil
}

// searchFromMap is the search of a character the combat does not hold: from its token on the
// session's current map.
func (s *Service) searchFromMap(ctx context.Context, c *combatTx, who link.Character) (searchPlace, error) {
	if c.session.CurrentMapID == nil {
		return searchPlace{}, errSearchNotOnMap()
	}
	mapID := *c.session.CurrentMapID
	g, err := s.maps.MapGrid(ctx, c.tx, c.session.CampaignID, mapID)
	if err != nil {
		return searchPlace{}, err
	}
	tokens, err := s.maps.MapTokens(ctx, c.tx, mapID)
	if err != nil {
		return searchPlace{}, err
	}
	i := slices.IndexFunc(tokens, func(t link.TokenPosition) bool { return t.CharacterID == who.ID })
	if !g.OK() || i < 0 {
		return searchPlace{}, errSearchNotOnMap()
	}
	return searchPlace{mapID: mapID, at: grid.Grid{Columns: int(g.Columns), Rows: int(g.Rows)}.SquareOf(int(tokens[i].XBP), int(tokens[i].YBP))}, nil
}
