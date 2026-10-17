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

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// The contests and the special actions of a combat (SRD 5.1, "Contests", "Grappling",
// "Shoving a Creature", "Hide", "Help", "Group Checks", "Surprise"): this file is what
// they share, the files next to it are the calls. The rules are pure, in
// rules/combat/contest.go; the state is in the tables of migration 00196.

// The kinds of session_events rows the contests write (session_event_kinds).
const (
	eventContestStarted      = "contest_started"
	eventContestResolved     = "contest_resolved"
	eventContestDeferred     = "contest_deferred"
	eventContestClosed       = "contest_closed"
	eventShoveResolved       = "shove_resolved"
	eventGrappleReleased     = "grapple_released"
	eventHideAttempted       = "hide_attempted"
	eventHideResolved        = "hide_resolved"
	eventHideEnded           = "hide_ended"
	eventHelpGiven           = "help_given"
	eventHelpCleared         = "help_cleared"
	eventGroupCheckRequested = "group_check_requested"
	eventGroupCheckRolled    = "group_check_rolled"
	eventGroupCheckClosed    = "group_check_closed"
	eventSurpriseSet         = "surprise_set"
)

// The values of combat_contests' columns.
const (
	contestKindContest  = "contest"
	contestKindEscapeDC = "escape_dc"

	purposeGrapple = "grapple"
	purposeShove   = "shove"
	purposeEscape  = "escape"

	contestAwaitingDefender = "awaiting_defender"
	contestAwaitingOutcome  = "awaiting_outcome"
	contestResolved         = "resolved"
	contestClosed           = "closed"

	winnerInitiator = "initiator"
	winnerDefender  = "defender"
	winnerTie       = "tie"

	shoveProne  = "prone"
	shovePush   = "push"
	shoveStays  = "stays"
	hidePending = "pending"
	hideApplied = "applied"
	hideRefused = "refused"

	helpCheck  = "check"
	helpAttack = "attack"
)

// The condition keys and the skills the contests read.
const (
	condGrappled   = "condition:grappled"
	condProne      = "condition:prone"
	condPoisoned   = "condition:poisoned"
	condBlinded    = "condition:blinded"
	skillAthletics = "skill:athletics"
	skillAcrobat   = "skill:acrobatics"
	skillStealth   = "skill:stealth"
	skillPercept   = "skill:perception"
	actionHide     = "standard:hide"
	actionHelp     = "standard:help"
)

// maxContestsListed is how many of a combat's latest contests the state returns.
const maxContestsListed = 20

// The Portuguese sentences of the circumstances behind a roll (names_pt.json has the
// conditions' names).
const (
	noteHelp         = "help"
	noteUnseen       = "unseen_attacker"
	notePoisoned     = "poisoned"
	labelUnseen      = "Atacante não visto"
	labelPoisoned    = "Envenenado"
	defaultHideRefus = "Alguém vê você claramente: não dá para se esconder agora."
)

// rollNote is one circumstance behind a roll's mode, as a roll keeps it.
type rollNote struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
	Adv   bool   `json:"adv,omitempty"`
}

func (n rollNote) proto() *playv1.RollNote {
	return &playv1.RollNote{Kind: n.Kind, LabelPt: n.Label, Advantage: n.Adv}
}

func notesProto(notes []rollNote) []*playv1.RollNote {
	var out []*playv1.RollNote
	for _, n := range notes {
		out = append(out, n.proto())
	}
	return out
}

// contestRoll is a d20 check as the tables keep it (JSON): the skill, the faces rolled (the
// pair for advantage or disadvantage), what it added, the total, the circumstances behind
// its mode and who rolled. A deferred answer has no faces: the master rolls it later.
type contestRoll struct {
	Skill    string     `json:"skill,omitempty"`
	Faces    []int32    `json:"faces,omitempty"`
	Modifier int32      `json:"modifier"`
	Total    int32      `json:"total"`
	Physical bool       `json:"physical,omitempty"`
	Mode     string     `json:"mode,omitempty"`
	Notes    []rollNote `json:"notes,omitempty"`
	// Unknown says the roller has no known number for the check: the d20 plus 0.
	Unknown  bool `json:"unknown,omitempty"`
	ByMaster bool `json:"by_master,omitempty"`
	// Deferred says the player left the roll to the master: Skill is their choice and
	// there are no faces.
	Deferred bool `json:"deferred,omitempty"`
	// The Bardic Inspiration die the player added after the d20 (already in Total).
	BonusSides int32 `json:"bonus_sides,omitempty"`
	BonusFace  int32 `json:"bonus_face,omitempty"`
	// The table maneuver's superiority die added after the d20 (already in Total).
	ManeuverKey   string `json:"maneuver_key,omitempty"`
	ManeuverName  string `json:"maneuver_name,omitempty"`
	ManeuverSides int32  `json:"maneuver_sides,omitempty"`
	ManeuverFace  int32  `json:"maneuver_face,omitempty"`
	// Lucky is the d20 the halfling's Lucky rolled again after it showed a 1 (a group check's,
	// inspiration_lucky.go): Faces hold the new face, and this the 1 it replaced and where.
	Lucky *luckyReroll `json:"lucky,omitempty"`
}

func checkModeKey(m combat.CheckMode) string {
	switch m {
	case combat.CheckAdvantage:
		return "advantage"
	case combat.CheckDisadvantage:
		return "disadvantage"
	case combat.CheckNormal:
	}
	return ""
}

func checkModeProto(m combat.CheckMode) playv1.RollModeKind {
	switch m {
	case combat.CheckAdvantage:
		return playv1.RollModeKind_ROLL_MODE_KIND_ADVANTAGE
	case combat.CheckDisadvantage:
		return playv1.RollModeKind_ROLL_MODE_KIND_DISADVANTAGE
	case combat.CheckNormal:
	}
	return playv1.RollModeKind_ROLL_MODE_KIND_NORMAL
}

func checkModeOfKey(key string) combat.CheckMode {
	switch key {
	case "advantage":
		return combat.CheckAdvantage
	case "disadvantage":
		return combat.CheckDisadvantage
	}
	return combat.CheckNormal
}

// proto is the roll as a view reads it, with the skill's enum for a contest roll.
func (r contestRoll) proto() *playv1.CheckRoll {
	out := &playv1.CheckRoll{
		Skill: skillEnum(r.Skill), SkillKey: r.Skill, Faces: slices.Clone(r.Faces), Modifier: r.Modifier, Total: r.Total, Physical: r.Physical,
		Mode: checkModeProto(checkModeOfKey(r.Mode)), Notes: notesProto(r.Notes), BonusKnown: !r.Unknown, RolledByMaster: r.ByMaster,
		BonusDieSides: r.BonusSides, BonusDieFace: r.BonusFace,
		ManeuverKey: r.ManeuverKey, ManeuverNamePt: r.ManeuverName, ManeuverDieSides: r.ManeuverSides, ManeuverDieFace: r.ManeuverFace,
		Rerolled: r.Lucky.proto(),
	}
	return out
}

func skillEnum(key string) playv1.ContestSkill {
	switch key {
	case skillAthletics:
		return playv1.ContestSkill_CONTEST_SKILL_ATHLETICS
	case skillAcrobat:
		return playv1.ContestSkill_CONTEST_SKILL_ACROBATICS
	}
	return playv1.ContestSkill_CONTEST_SKILL_UNSPECIFIED
}

func skillKeyOf(s playv1.ContestSkill) string {
	switch s {
	case playv1.ContestSkill_CONTEST_SKILL_ATHLETICS:
		return skillAthletics
	case playv1.ContestSkill_CONTEST_SKILL_ACROBATICS:
		return skillAcrobat
	case playv1.ContestSkill_CONTEST_SKILL_UNSPECIFIED:
	}
	return ""
}

// errContest is the failed_precondition of the contest calls, with the ContestBlocked
// detail that tells the app why. edit fills the detail's extra fields.
func errContest(reason playv1.ContestBlockedReason, msg string, edit ...func(*playv1.ContestBlocked)) error {
	err := connect.NewError(connect.CodeFailedPrecondition, errors.New(msg))
	blocked := &playv1.ContestBlocked{Reason: reason}
	for _, e := range edit {
		e(blocked)
	}
	if detail, detailErr := connect.NewErrorDetail(blocked); detailErr == nil {
		err.AddDetail(detail)
	}
	return err
}

// errNotAwaiting is a contest or an attempt that waits for nothing of what the call does.
func errNotAwaiting() error {
	return errContest(playv1.ContestBlockedReason_CONTEST_BLOCKED_REASON_NOT_AWAITING, "the contest does not wait for this")
}

// checkInput is how a check's d20 comes: in the app, or the faces of the physical dice.
type checkInput struct {
	inApp bool
	faces []int
}

// parseCheckInput reads a request's CheckRollInput: exactly one way, the faces 1 to 20.
func parseCheckInput(in *playv1.CheckRollInput) (checkInput, error) {
	switch r := in.GetRoll().(type) {
	case *playv1.CheckRollInput_RollInApp:
		if !r.RollInApp {
			return checkInput{}, connect.NewError(connect.CodeInvalidArgument, errors.New("roll_in_app must be true"))
		}
		return checkInput{inApp: true}, nil
	case *playv1.CheckRollInput_D20Faces:
		faces := r.D20Faces.GetFaces()
		if len(faces) < 1 || len(faces) > 2 {
			return checkInput{}, connect.NewError(connect.CodeInvalidArgument, errors.New("d20_faces must have one or two faces"))
		}
		out := checkInput{}
		for _, f := range faces {
			if f < 1 || f > 20 {
				return checkInput{}, connect.NewError(connect.CodeInvalidArgument, errors.New("each d20 face must be 1 to 20"))
			}
			out.faces = append(out.faces, int(f))
		}
		return out, nil
	}
	return checkInput{}, connect.NewError(connect.CodeInvalidArgument, errors.New("set roll_in_app or d20_faces"))
}

// rollInputOf is the checkInput as RN-18's check reads it.
func (in checkInput) rollInput() rollInput { return rollInput{inApp: in.inApp} }

// rollCheck rolls a check's d20 (or the pair the mode asks for), or checks the faces typed
// from physical dice: the dice must match the mode the server worked out, so a stale screen
// is told to read the options again (aborted).
func (s *Service) rollCheck(in checkInput, mode combat.CheckMode, modifier int, skill string, notes []rollNote, unknown bool) (contestRoll, error) {
	faces := in.faces
	physical := !in.inApp
	if in.inApp {
		res, err := dice.Roll(s.roller, dice.Expr{Count: mode.Dice(), Sides: d20Sides})
		if err != nil {
			return contestRoll{}, fmt.Errorf("roll the d20: %w", err)
		}
		faces = res.Faces
	} else if len(faces) != mode.Dice() {
		return contestRoll{}, connect.NewError(connect.CodeAborted, errors.New("the roll's mode changed: read the options again"))
	}
	kept := mode.PickD20(faces)
	out := contestRoll{
		Skill: skill, Modifier: clamp32(modifier, math.MinInt32, math.MaxInt32), Total: clamp32(kept+modifier, math.MinInt32, math.MaxInt32),
		Physical: physical, Mode: checkModeKey(mode), Notes: notes, Unknown: unknown,
	}
	for _, f := range faces {
		out.Faces = append(out.Faces, clamp32(f, 1, 20))
	}
	return out, nil
}

func encodeRoll(r contestRoll) ([]byte, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return nil, fmt.Errorf("encode a roll: %w", err)
	}
	return b, nil
}

func decodeRoll(b []byte) (*contestRoll, error) {
	if len(b) == 0 {
		return nil, nil
	}
	var r contestRoll
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("read a roll: %w", err)
	}
	return &r, nil
}

// checkNumbers is what a combatant adds to a check, and its passive score for the skills
// that have one.
type checkNumbers struct {
	Bonus int
	// Known is false for a combatant with no number for the check (a basic-sheet NPC made
	// by hand): it rolls the d20 plus 0 and the master's log says so.
	Known   bool
	Passive int
	// HasPassive says Passive is the sheet's or the stat block's; without one a passive
	// Perception is 10 plus the bonus.
	HasPassive bool
}

// numbersOf reads a combatant's bonus in a skill or ability check (a character's sheet,
// an NPC made from a creature's stat block, a creature's stat block).
func (s *Service) numbersOf(ctx context.Context, tx pgx.Tx, campaignID string, c playdb.Combatant, key string) (checkNumbers, error) {
	var o link.SceneOption
	var err error
	if isCreature(c) {
		o, err = s.roster.CreatureCheck(ctx, tx, campaignID, deref(c.MonsterKey), key)
	} else {
		var opts []link.SceneOption
		if opts, err = s.roster.SceneOptions(ctx, tx, campaignID, c.CharacterID, []string{key}); err == nil && len(opts) == 1 {
			o = opts[0]
		}
	}
	if err != nil {
		return checkNumbers{}, err
	}
	return checkNumbers{Bonus: o.Bonus, Known: o.Known, Passive: o.Passive, HasPassive: o.HasPassive}, nil
}

// passivePerceptionOf is a combatant's passive Wisdom (Perception) with the circumstances
// that give its Perception advantage or disadvantage counted (SRD 5.1, "Passive Checks").
func passivePerceptionOf(n checkNumbers, mode combat.CheckMode) int {
	if n.HasPassive {
		return n.Passive + modeShift(mode)
	}
	return combat.PassivePerceptionOf(n.Bonus, mode)
}

func modeShift(m combat.CheckMode) int {
	return combat.PassivePerceptionOf(0, m) - combat.PassivePerceptionOf(0, combat.CheckNormal)
}

// checkSourceNotes are the notes of the circumstances SaveMode lists for an ability check.
var checkSourceNotes = map[string]rollNote{
	combat.SourcePoisonedCheck:   {Kind: notePoisoned, Label: labelPoisoned},
	combat.SourceFrightenedCheck: {Kind: "frightened", Label: "Amedrontado"},
	combat.SourceRageStrength:    {Kind: "rage", Label: "Fúria", Adv: true},
	combat.SourceStealthArmor:    {Kind: "armor_stealth", Label: "Armadura que atrapalha a furtividade"},
	combat.SourceExhaustionCheck: {Kind: "exhaustion", Label: "Exaustão"},
	combat.SourceEffectCheck:     {Kind: "effect", Label: "Efeito ativo", Adv: true},
}

// checkAbilityOf is the ability a skill's check rolls: Strength for Athletics, Wisdom for
// Perception, Dexterity for the rest the contests use.
func checkAbilityOf(skill string) string {
	switch skill {
	case skillAthletics:
		return "str"
	case skillPercept:
		return "wis"
	}
	return "dex"
}

// stateNotes are the notes of the conditions and the effects that give a combatant's ability
// check advantage or disadvantage now (SaveMode, for an ability check).
func (s *Service) stateNotes(ctx context.Context, tx pgx.Tx, campaignID string, q *playdb.Queries, enc playdb.Encounter, who playdb.Combatant, skill string) ([]rollNote, error) {
	// The sheet's traits: heavy armor takes the Rage's benefits away, and some armor gives
	// disadvantage on Dexterity (Stealth) (SRD 5.1, "Armor").
	sheet, err := s.sheetOf(ctx, tx, campaignID, who)
	if err != nil {
		return nil, err
	}
	states, err := q.ListCombatantStates(ctx, enc.ID)
	if err != nil {
		return nil, fmt.Errorf("list the states: %w", err)
	}
	scene := combat.SaveScene{
		Creature: creatureFacts(who, statesOf(states), sheet.Traits), Ability: checkAbilityOf(skill), Check: true,
		StealthArmor: skill == skillStealth && sheet.Traits.StealthDisadvantage,
	}
	var out []rollNote
	for _, src := range combat.SaveMode(scene) {
		if note, ok := checkSourceNotes[src.Kind]; ok {
			out = append(out, note)
		}
	}
	return out, nil
}

// checkSources are the circumstances that give a combatant's ability check advantage or
// disadvantage now: the rules of advantage's SaveMode for an ability check (Poisoned and
// Frightened, SRD 5.1 Conditions; a raging barbarian's Strength) and the Helps that hold for
// the task (SRD 5.1, "Help").
func (s *Service) checkSources(ctx context.Context, tx pgx.Tx, campaignID string, q *playdb.Queries, enc playdb.Encounter, who playdb.Combatant, skill string, cs []playdb.Combatant) ([]rollNote, error) {
	out, err := s.stateNotes(ctx, tx, campaignID, q, enc, who, skill)
	if err != nil {
		return nil, err
	}
	helps, err := s.liveHelps(ctx, q, enc, cs)
	if err != nil {
		return nil, err
	}
	for _, h := range helps {
		if h.Kind == helpCheck && helpsAlly(h, who) && deref(h.Task) == skill {
			out = append(out, rollNote{Kind: noteHelp, Label: "Ajuda de " + helperLabel(cs, h), Adv: true})
		}
	}
	return out, nil
}

// checkSourcesRead is checkSources for a read that holds no transaction.
func (s *Service) checkSourcesRead(ctx context.Context, campaignID string, enc playdb.Encounter, who playdb.Combatant, skill string, cs []playdb.Combatant) ([]rollNote, error) {
	return s.checkSources(ctx, nil, campaignID, s.queries, enc, who, skill, cs)
}

func notesMode(notes []rollNote) combat.CheckMode {
	sources := make([]combat.CheckSource, 0, len(notes))
	for _, n := range notes {
		sources = append(sources, combat.CheckSource{Kind: n.Kind, Advantage: n.Adv})
	}
	return combat.ResolveCheckMode(sources)
}

// helperLabel is the name of the combatant that gave a Help.
func helperLabel(cs []playdb.Combatant, h playdb.CombatHelp) string {
	if c, ok := helpHelper(cs, h); ok {
		return c.Label
	}
	return ""
}

// consumeCheckHelp uses the Help that gave the combatant advantage on a check of the
// task: the next check of the task is the one it covers (SRD 5.1, "Help").
func (s *Service) consumeCheckHelp(ctx context.Context, c *combatTx, who playdb.Combatant, skill string, cs []playdb.Combatant) error {
	helps, err := s.liveHelps(ctx, c.q, c.enc, cs)
	if err != nil {
		return err
	}
	for _, h := range helps {
		if h.Kind == helpCheck && helpsAlly(h, who) && deref(h.Task) == skill {
			if err := c.q.SetHelpConsumed(ctx, playdb.SetHelpConsumedParams{ID: h.ID, ConsumedAt: &c.now}); err != nil {
				return fmt.Errorf("use the help: %w", err)
			}
		}
	}
	return nil
}

// rollFor rolls a combatant's check of a skill: its number, the circumstances behind the
// mode, the d20 or the pair; and uses the Help that covered it.
func (s *Service) rollFor(ctx context.Context, c *combatTx, who playdb.Combatant, skill string, in checkInput, cs []playdb.Combatant) (contestRoll, error) {
	n, err := s.numbersOf(ctx, c.tx, c.session.CampaignID, who, skill)
	if err != nil {
		return contestRoll{}, err
	}
	notes, err := s.checkSources(ctx, c.tx, c.session.CampaignID, c.q, c.enc, who, skill, cs)
	if err != nil {
		return contestRoll{}, err
	}
	roll, err := s.rollCheck(in, notesMode(notes), n.Bonus, skill, notes, !n.Known)
	if err != nil {
		return contestRoll{}, err
	}
	if err := s.consumeCheckHelp(ctx, c, who, skill, cs); err != nil {
		return contestRoll{}, err
	}
	return roll, nil
}

// contestEvent is what a contest or a special action adds to the event it writes: the
// contest or attempt, and what the log says. IDs and keys only.
type contestEvent struct {
	// ContestID is the contest, the hide attempt or the group check the event is about.
	ContestID string `json:"contest_id,omitempty"`
	Purpose   string `json:"purpose,omitempty"`
	// Line is the log line the event is, a contestLine* value; empty when it is none.
	Line string `json:"line,omitempty"`
	// Waiting says the contest waits for its defender after this event.
	Waiting bool `json:"waiting,omitempty"`
	// HelpID is the Help the event made or cleared.
	HelpID string `json:"help_id,omitempty"`
	// Surprised is the state a surprise_set event set.
	Surprised bool `json:"surprised,omitempty"`
}

// The log lines of a contest (ContestLogLine), as an event keeps them.
const (
	contestLineGrappled        = "grappled"
	contestLineGrappleFailed   = "grapple_failed"
	contestLineShoveProne      = "shove_prone"
	contestLineShovePushed     = "shove_pushed"
	contestLineShoveStays      = "shove_stays"
	contestLineShoveFailed     = "shove_failed"
	contestLineEscaped         = "escaped"
	contestLineEscapeFailed    = "escape_failed"
	contestLineClosed          = "closed"
	contestLineReleased        = "released"
	contestLineHideTried       = "hide_tried"
	contestLineHideApplied     = "hide_applied"
	contestLineHideRefused     = "hide_refused"
	contestLineHelped          = "helped"
	contestLineSurprised       = "surprised"
	contestLineSurpriseCleared = "surprise_cleared"
)

var contestLineProto = map[string]playv1.ContestLogLine{
	contestLineGrappled: playv1.ContestLogLine_CONTEST_LOG_LINE_GRAPPLED, contestLineGrappleFailed: playv1.ContestLogLine_CONTEST_LOG_LINE_GRAPPLE_FAILED,
	contestLineShoveProne: playv1.ContestLogLine_CONTEST_LOG_LINE_SHOVE_PRONE, contestLineShovePushed: playv1.ContestLogLine_CONTEST_LOG_LINE_SHOVE_PUSHED,
	contestLineShoveStays: playv1.ContestLogLine_CONTEST_LOG_LINE_SHOVE_STAYS, contestLineShoveFailed: playv1.ContestLogLine_CONTEST_LOG_LINE_SHOVE_FAILED,
	contestLineEscaped: playv1.ContestLogLine_CONTEST_LOG_LINE_ESCAPED, contestLineEscapeFailed: playv1.ContestLogLine_CONTEST_LOG_LINE_ESCAPE_FAILED,
	contestLineClosed: playv1.ContestLogLine_CONTEST_LOG_LINE_CLOSED, contestLineReleased: playv1.ContestLogLine_CONTEST_LOG_LINE_RELEASED,
	contestLineHideTried: playv1.ContestLogLine_CONTEST_LOG_LINE_HIDE_TRIED, contestLineHideApplied: playv1.ContestLogLine_CONTEST_LOG_LINE_HIDE_APPLIED,
	contestLineHideRefused: playv1.ContestLogLine_CONTEST_LOG_LINE_HIDE_REFUSED, contestLineHelped: playv1.ContestLogLine_CONTEST_LOG_LINE_HELPED,
	contestLineSurprised: playv1.ContestLogLine_CONTEST_LOG_LINE_SURPRISED, contestLineSurpriseCleared: playv1.ContestLogLine_CONTEST_LOG_LINE_SURPRISE_CLEARED,
}

// masterOnlyLines are the log lines only the master reads: what the master decided of a
// hide, and the surprise (RN-10, RN-20).
var masterOnlyLines = []string{contestLineHideTried, contestLineHideApplied, contestLineHideRefused, contestLineSurprised, contestLineSurpriseCleared, contestLineClosed}

// withCondition is the conditions with one more, or the same list when it is there already.
func withCondition(conditions []string, key string) []string {
	if slices.Contains(conditions, key) {
		return conditions
	}
	return append(slices.Clone(conditions), key)
}

// withoutCondition is the conditions without one.
func withoutCondition(conditions []string, key string) []string {
	return slices.DeleteFunc(slices.Clone(conditions), func(k string) bool { return k == key })
}

// setConditionsOf writes a combatant's conditions in the change's transaction.
func setConditionsOf(ctx context.Context, c *combatTx, who playdb.Combatant, conditions []string) error {
	if slices.Equal(who.Conditions, conditions) {
		return nil
	}
	if err := c.q.SetCombatantConditions(ctx, playdb.SetCombatantConditionsParams{ID: who.ID, Conditions: conditions}); err != nil {
		return fmt.Errorf("set the conditions: %w", err)
	}
	return nil
}

// isGrappled says whether a combatant carries the Grappled condition.
func isGrappled(c playdb.Combatant) bool { return slices.Contains(c.Conditions, condGrappled) }

// holdsOf maps the encounter's holds by the grappled combatant.
func holdsOf(holds []playdb.CombatHold) map[string]playdb.CombatHold {
	out := make(map[string]playdb.CombatHold, len(holds))
	for _, h := range holds {
		out[h.GrappledID] = h
	}
	return out
}

// holdOn is the hold on a grappled combatant: the stored one, when the combatant still
// carries the condition.
func holdOn(holds map[string]playdb.CombatHold, c playdb.Combatant) (playdb.CombatHold, bool) {
	h, ok := holds[c.ID]
	return h, ok && isGrappled(c)
}

// oppositeSide says whether two combatants are on different sides.
func oppositeSide(a, b playdb.Combatant) bool { return a.Side != b.Side }

// trimReason cleans the master's one sentence: trimmed, one line, 1 to 120 characters.
func trimReason(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", nil
	}
	if len([]rune(s)) > 120 || strings.ContainsAny(s, "\r\n") {
		return "", connect.NewError(connect.CodeInvalidArgument, errors.New("the reason must be 1 to 120 characters on one line"))
	}
	return s, nil
}

// contestLogEntry turns an event of the contests into a line of the combat log: the kind, and
// whether only the master reads it. It says false for an event that is no line.
func contestLogEntry(entry *logEntry, ev actionEvent) bool {
	if ev.Contest == nil || ev.Contest.Line == "" {
		return false
	}
	entry.kind = playv1.CombatLogKind_COMBAT_LOG_KIND_CONTEST
	entry.masterOnly = slices.Contains(masterOnlyLines, ev.Contest.Line)
	return true
}
