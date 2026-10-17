package play

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// Grapple, shove and escape (SRD 5.1, "Contests", "Grappling", "Shoving a Creature").
//
// A grapple or a shove is a special melee attack of the Attack action: it replaces one
// attack, and instead of an attack roll the two sides roll Strength (Athletics) against
// Strength (Athletics) or Dexterity (Acrobatics), the defender's choice. The initiator
// rolls first (StartContest, which spends the attack in the same transaction); the
// defender answers (RespondContest): the player for their own character, the master for
// an NPC. The higher total wins and a tie changes nothing. A won grapple makes the
// target Grappled (the condition, and combat_holds says who holds it); a won shove waits
// for its winner's choice between knocking prone and pushing 5 feet (ResolveShove); an
// escape is the grappled creature's action, against the grappler's Athletics or against
// the fixed escape DC of a grapple that came from an attack.
//
// What each side reads (RN-10, RN-20): the player reads their own roll and who won,
// never the total, the skill or the escape DC of an NPC, nor the other player's total.
//
// The contest waits without a timeout; the master may answer for anyone or close it.
// The wait is a reaction window of kind CONTEST (combat_contests_window.go).

// contestCaller reads what every contest call starts with: the caller, the idempotency
// key and the combat.
func contestCaller(ctx context.Context, campaignID, keyRaw, encounterRaw string) (m authz.Membership, key, encID string, err error) {
	if m, err = authz.RequireCampaignMember(ctx, campaignID); err != nil {
		return m, "", "", err
	}
	if key, err = parseKey(keyRaw); err != nil {
		return m, "", "", err
	}
	if encID, err = parseCombatID(encounterRaw, "encounter"); err != nil {
		return m, "", "", err
	}
	return m, key, encID, nil
}

// mustBeRunning refuses a contest call when the combat is not running.
func mustBeRunning(c *combatTx) error {
	if err := notEnded(c.enc); err != nil {
		return err
	}
	if c.enc.Status != statusActive {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_ACTIVE, "the combat is not running")
	}
	return nil
}

// spendAttack spends one attack of the Attack action for a grapple or a shove: the action
// when it is the first, the next attack of the Attack action when Extra Attack leaves one
// (SRD 5.1, "Grappling": the special attack replaces one of them). A player is held to
// it; the master has the last word on an NPC and runs its attacks as he wants, as
// RollAttack does.
func spendAttack(ctx context.Context, c *combatTx, who playdb.Combatant, sheet link.Sheet, master bool) error {
	perAction := max(sheet.AttacksPerAction, 1)
	last, hasLast := lastAttack(sheet, who)
	castCantrip := hasLast && last.Spell // the action went to a cantrip: no Attack action attacks left
	turn := combat.TurnState{ActionUsed: who.ActionUsed, AttacksMade: int(who.AttacksMade)}
	if !master && (castCantrip || combat.AttacksLeft(perAction, turn) == 0) {
		if !castCantrip && who.AttacksMade > 0 && perAction > 1 {
			return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ATTACKS_USED, "the Attack action made all its attacks")
		}
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ACTION_USED, "the action of this turn is used")
	}
	if err := c.q.SetCombatantEconomy(ctx, playdb.SetCombatantEconomyParams{
		ID: who.ID, ActionUsed: true, BonusActionUsed: who.BonusActionUsed, ReactionUsed: who.ReactionUsed, Dashed: who.Dashed,
	}); err != nil {
		return fmt.Errorf("spend the action: %w", err)
	}
	if err := c.q.SetCombatantAttacksMade(ctx, playdb.SetCombatantAttacksMadeParams{ID: who.ID, AttacksMade: who.AttacksMade + 1}); err != nil {
		return fmt.Errorf("count the attack: %w", err)
	}
	if who.ActionAttackKey == nil {
		// The Attack action was taken (Flurry of Blows may follow); a later weapon attack
		// replaces this mark with its own.
		contestKey := combat.ContestAttackKey
		if err := c.q.SetCombatantAttackState(ctx, playdb.SetCombatantAttackStateParams{ID: who.ID, ActionAttackKey: &contestKey, BonusAttacksLeft: who.BonusAttacksLeft}); err != nil {
			return fmt.Errorf("mark the Attack action: %w", err)
		}
	}
	return nil
}

// spendAction spends the action of a combatant that tries to escape; a player with the
// action spent is refused.
func spendAction(ctx context.Context, c *combatTx, who playdb.Combatant, master bool) error {
	if who.ActionUsed && !master {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ACTION_USED, "the action of this turn is used")
	}
	if err := c.q.SetCombatantEconomy(ctx, playdb.SetCombatantEconomyParams{
		ID: who.ID, ActionUsed: true, BonusActionUsed: who.BonusActionUsed, ReactionUsed: who.ReactionUsed, Dashed: who.Dashed,
	}); err != nil {
		return fmt.Errorf("spend the action: %w", err)
	}
	return nil
}

// applyGrapple makes the target Grappled by the grappler, and keeps who holds it and the
// escape DC of a grapple that came from an attack.
func applyGrapple(ctx context.Context, c *combatTx, grappler, target playdb.Combatant, escapeDC *int32) error {
	if err := setConditionsOf(ctx, c, target, withCondition(target.Conditions, condGrappled)); err != nil {
		return err
	}
	if err := c.q.UpsertHold(ctx, playdb.UpsertHoldParams{
		GrappledID: target.ID, EncounterID: c.enc.ID, GrapplerID: grappler.ID, EscapeDc: escapeDC, CreatedAt: c.now,
	}); err != nil {
		return fmt.Errorf("keep the hold: %w", err)
	}
	return nil
}

// releaseHold ends a grapple: the condition and the hold go.
func releaseHold(ctx context.Context, c *combatTx, grappled playdb.Combatant) error {
	if err := setConditionsOf(ctx, c, grappled, withoutCondition(grappled.Conditions, condGrappled)); err != nil {
		return err
	}
	if err := c.q.DeleteHold(ctx, grappled.ID); err != nil {
		return fmt.Errorf("end the hold: %w", err)
	}
	return nil
}

// winnerKey is a contest's winner as the table stores it.
func winnerKey(w combat.ContestWinner) string {
	switch w {
	case combat.ContestInitiator:
		return winnerInitiator
	case combat.ContestDefender:
		return winnerDefender
	case combat.ContestTie:
	}
	return winnerTie
}

// settleContest applies what a contest decided: the grapple, the escape, or the shove that
// waits for its choice. It returns the status the contest has now and the log line (empty
// when there is none yet).
func settleContest(ctx context.Context, c *combatTx, row playdb.CombatContest, winner string, initiator, defender playdb.Combatant) (status, line string, err error) {
	won := winner == winnerInitiator
	switch row.Purpose {
	case purposeGrapple:
		if !won {
			return contestResolved, contestLineGrappleFailed, nil
		}
		return contestResolved, contestLineGrappled, applyGrapple(ctx, c, initiator, defender, row.EscapeDc)
	case purposeEscape:
		if !won {
			return contestResolved, contestLineEscapeFailed, nil
		}
		return contestResolved, contestLineEscaped, releaseHold(ctx, c, initiator)
	case purposeShove:
		if !won {
			return contestResolved, contestLineShoveFailed, nil
		}
		return contestAwaitingOutcome, "", nil
	}
	return contestResolved, "", nil
}

// The escape DC a master may give a grapple that came from an attack (SRD 5.1: the
// monsters' "escape DC" runs from the low teens to the low twenties; the table's ceiling is
// the highest DC of any check).
const (
	minEscapeDC = 1
	maxEscapeDC = 40
)

// contestStart is a StartContest request read and checked: everything the transaction needs
// that does not come from the database.
type contestStart struct {
	m            authz.Membership
	key, encID   string
	initiatorID  string
	targetID     string // empty for an escape
	purpose      playv1.ContestPurpose
	purposeKey   string
	byAttack     bool // a grapple that came from an attack: the master says the DC, nobody rolls
	escapeDC     int32
	skill        playv1.ContestSkill
	maneuverKey  string
	maneuverFace int32
	in           checkInput
}

// purposeKeyOf is a contest's purpose as the table stores it.
func purposeKeyOf(purpose playv1.ContestPurpose) (string, error) {
	switch purpose {
	case playv1.ContestPurpose_CONTEST_PURPOSE_GRAPPLE:
		return purposeGrapple, nil
	case playv1.ContestPurpose_CONTEST_PURPOSE_SHOVE:
		return purposeShove, nil
	case playv1.ContestPurpose_CONTEST_PURPOSE_ESCAPE:
		return purposeEscape, nil
	case playv1.ContestPurpose_CONTEST_PURPOSE_UNSPECIFIED:
	}
	return "", connect.NewError(connect.CodeInvalidArgument, errors.New("purpose must be GRAPPLE, SHOVE or ESCAPE"))
}

// checkContestKind says whether the request is a grapple that came from an attack (kind
// ESCAPE_DC) and refuses the kinds and DCs that do not go together.
func checkContestKind(st *contestStart, kind playv1.ContestKind) error {
	st.byAttack = kind == playv1.ContestKind_CONTEST_KIND_ESCAPE_DC
	if st.byAttack && (st.purpose != playv1.ContestPurpose_CONTEST_PURPOSE_GRAPPLE || st.m.Role != authz.RoleMaster) {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("kind ESCAPE_DC is the master's grapple that came from an attack"))
	}
	if st.byAttack && (st.escapeDC < minEscapeDC || st.escapeDC > maxEscapeDC) {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("escape_dc must be 1 to 40"))
	}
	if !st.byAttack && st.escapeDC != 0 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("escape_dc is only for kind ESCAPE_DC"))
	}
	return nil
}

// checkContestSkill defaults the skill to Athletics and refuses what a purpose cannot use:
// only an escape may roll Acrobatics, and only a rolled grapple starts a maneuver.
func checkContestSkill(st *contestStart) error {
	if st.skill == playv1.ContestSkill_CONTEST_SKILL_UNSPECIFIED {
		st.skill = playv1.ContestSkill_CONTEST_SKILL_ATHLETICS
	}
	if st.skill == playv1.ContestSkill_CONTEST_SKILL_ACROBATICS && st.purpose != playv1.ContestPurpose_CONTEST_PURPOSE_ESCAPE {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("a grapple or a shove is a Strength (Athletics) check"))
	}
	if st.maneuverKey != "" && (st.purpose != playv1.ContestPurpose_CONTEST_PURPOSE_GRAPPLE || st.byAttack) {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("a maneuver starts a GRAPPLE that is rolled"))
	}
	return nil
}

// parseContestStart is the first phase of StartContest: read and validate the request, in
// the order the refusals have always come.
func parseContestStart(ctx context.Context, msg *playv1.StartContestRequest) (contestStart, error) {
	m, key, encID, err := contestCaller(ctx, msg.GetCampaignId(), msg.GetIdempotencyKey(), msg.GetEncounterId())
	if err != nil {
		return contestStart{}, err
	}
	st := contestStart{m: m, key: key, encID: encID, purpose: msg.GetPurpose(), escapeDC: msg.GetEscapeDc(), skill: msg.GetSkill(), maneuverKey: msg.GetManeuverKey(), maneuverFace: msg.GetManeuverFace()}
	if st.initiatorID, err = parseCombatID(msg.GetInitiatorId(), "combatant"); err != nil {
		return contestStart{}, err
	}
	kind := msg.GetKind()
	if kind == playv1.ContestKind_CONTEST_KIND_UNSPECIFIED {
		kind = playv1.ContestKind_CONTEST_KIND_CONTEST
	}
	if st.purposeKey, err = purposeKeyOf(st.purpose); err != nil {
		return contestStart{}, err
	}
	if err := checkContestKind(&st, kind); err != nil {
		return contestStart{}, err
	}
	if st.purpose != playv1.ContestPurpose_CONTEST_PURPOSE_ESCAPE {
		if st.targetID, err = parseCombatID(msg.GetTargetId(), "combatant"); err != nil {
			return contestStart{}, err
		}
	}
	if err := checkContestSkill(&st); err != nil {
		return contestStart{}, err
	}
	if !st.byAttack {
		if st.in, err = parseCheckInput(msg.GetRoll()); err != nil {
			return contestStart{}, err
		}
	}
	return st, nil
}

// contestRun is what one StartContest transaction knows as it goes: the request, the combat
// as read, the sides, the roll and the row it will write.
type contestRun struct {
	st        contestStart
	c         *combatTx
	cs        []playdb.Combatant
	v         combatViewer
	holds     []playdb.CombatHold
	initiator playdb.Combatant
	defender  playdb.Combatant
	row       playdb.InsertContestParams
	roll      *contestRoll
	man       link.Maneuver
	usedMan   bool
}

// readContestRun is the read-and-validate phase of the transaction: the combat runs, the
// initiator is the caller's to move now, and it has no contest waiting.
func (s *Service) readContestRun(ctx context.Context, c *combatTx, st contestStart) (*contestRun, error) {
	if err := mustBeRunning(c); err != nil {
		return nil, err
	}
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return nil, fmt.Errorf("list the combatants: %w", err)
	}
	v := c.viewer(st.m, cs) // the fog: an NPC the player does not see is not found
	initiator, err := findCombatant(cs, st.initiatorID, v)
	if err != nil {
		return nil, err
	}
	if err := v.mayAct(initiator); err != nil {
		return nil, err
	}
	open, err := c.q.ListUnansweredContestsOf(ctx, playdb.ListUnansweredContestsOfParams{EncounterID: c.enc.ID, InitiatorID: initiator.ID})
	if err != nil {
		return nil, fmt.Errorf("list the open contests: %w", err)
	}
	if len(open) > 0 {
		return nil, errContest(playv1.ContestBlockedReason_CONTEST_BLOCKED_REASON_CONTEST_OPEN, "a contest of this combatant waits already")
	}
	if err := s.mustActNow(ctx, c, initiator); err != nil {
		return nil, err
	}
	holds, err := c.q.ListHolds(ctx, c.enc.ID)
	if err != nil {
		return nil, fmt.Errorf("list the holds: %w", err)
	}
	return &contestRun{
		st: st, c: c, cs: cs, v: v, holds: holds, initiator: initiator,
		row: playdb.InsertContestParams{
			EncounterID: c.enc.ID, InitiatorID: initiator.ID, Round: c.enc.Round, CreatedAt: c.now, Purpose: st.purposeKey, Kind: contestKindContest,
		},
	}, nil
}

// resolveEscape picks the grappler as the defender of an escape and spends the action; a
// hold that came from an attack makes the contest a fixed DC.
func (s *Service) resolveEscape(ctx context.Context, r *contestRun) error {
	hold, ok := holdOn(holdsOf(r.holds), r.initiator)
	if !ok {
		return errContest(playv1.ContestBlockedReason_CONTEST_BLOCKED_REASON_NOT_GRAPPLED, "the combatant is not grappled")
	}
	grappler, ok := combatantByID(r.cs, hold.GrapplerID)
	if !ok {
		return errContest(playv1.ContestBlockedReason_CONTEST_BLOCKED_REASON_NOT_GRAPPLED, "the combatant is not grappled")
	}
	r.defender = grappler
	if err := spendAction(ctx, r.c, r.initiator, r.v.master); err != nil {
		return err
	}
	if hold.EscapeDc != nil {
		r.row.Kind, r.row.EscapeDc = contestKindEscapeDC, hold.EscapeDc
	}
	return nil
}

// spendContestAttack pays for a grapple or a shove: one attack of the Attack action, or the
// bonus action and a use when a table maneuver starts the grapple.
func (s *Service) spendContestAttack(ctx context.Context, r *contestRun, sheet link.Sheet) error {
	if r.st.maneuverKey == "" {
		return spendAttack(ctx, r.c, r.initiator, sheet, r.v.master)
	}
	// A table maneuver after a melee hit: the bonus action and a use, not an attack.
	man, err := s.useGrappleManeuver(ctx, r.c, r.st.m.CampaignID, r.initiator, sheet, r.st.maneuverKey, r.v.master)
	if err != nil {
		return err
	}
	r.man, r.usedMan = man, true
	return nil
}

// noteContestAttack records that a grapple or a shove is an attack on the target: it keeps
// a rage going (SRD 5.1, Rage: the rage ends if the turn ends without having "attacked a
// hostile creature") and gives a hider's position away ("Unseen Attackers and Targets").
func (s *Service) noteContestAttack(ctx context.Context, r *contestRun) error {
	c, initiator := r.c, r.initiator
	if oppositeSide(initiator, r.defender) {
		states, err := s.readStates(ctx, c.tx, c.enc.ID)
		if err != nil {
			return err
		}
		if hasState(states, initiator.ID, stateRage) && (!initiator.AttackedHostile || initiator.RageEndPending) {
			if err := c.q.SetCombatantRageFlags(ctx, playdb.SetCombatantRageFlagsParams{ID: initiator.ID, AttackedHostile: true, TookDamage: initiator.TookDamage}); err != nil {
				return fmt.Errorf("note the attack on a hostile creature: %w", err)
			}
		}
	}
	_, err := s.endHiding(ctx, c, initiator)
	return err
}

// resolveAttackContest picks the defender of a grapple or a shove and, unless the grapple
// came from an attack that was rolled already, spends what it costs.
func (s *Service) resolveAttackContest(ctx context.Context, r *contestRun) error {
	defender, err := findCombatant(r.cs, r.st.targetID, r.v)
	if err != nil {
		return err
	}
	r.defender = defender
	switch {
	case defender.ID == r.initiator.ID:
		return connect.NewError(connect.CodeInvalidArgument, errors.New("a combatant cannot contest itself"))
	case defender.Defeated:
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TARGET_DEFEATED, "the target is defeated")
	}
	if r.st.byAttack {
		// A grapple that came from an attack: the attack was rolled already, so nothing is
		// spent and nobody rolls.
		escapeDC := r.st.escapeDC
		r.row.Kind, r.row.EscapeDc = contestKindEscapeDC, &escapeDC
		return nil
	}
	if err := s.mustBeInReach(r.c, r.v, r.initiator, defender); err != nil {
		return err
	}
	sheet, err := s.sheetOf(ctx, r.c.tx, r.st.m.CampaignID, r.initiator)
	if err != nil {
		return err
	}
	if err := mayAttack(r.initiator, false); err != nil {
		return err
	}
	if err := s.spendContestAttack(ctx, r, sheet); err != nil {
		return err
	}
	return s.noteContestAttack(ctx, r)
}

// rollContestInitiator rolls (or reads the typed dice of) the initiator's side, adds the
// maneuver die when one started the grapple, and keeps the roll on the row.
func (s *Service) rollContestInitiator(ctx context.Context, r *contestRun) error {
	if !r.v.master {
		if err := s.mustRollThisWay(ctx, r.c.tx, r.st.m, r.st.in.rollInput()); err != nil {
			return err
		}
	}
	roll, err := s.rollFor(ctx, r.c, r.initiator, skillKeyOf(r.st.skill), r.st.in, r.cs)
	if err != nil {
		return err
	}
	roll.ByMaster = r.v.master && !initiatorIsNPC(r.initiator)
	if r.usedMan {
		face, err := s.rollManeuverDie(r.man, r.st.in.inApp, r.st.maneuverFace)
		if err != nil {
			return err
		}
		addManeuverDie(&roll, r.man, face)
	}
	r.roll = &roll
	if r.row.InitiatorRoll, err = encodeRoll(roll); err != nil {
		return err
	}
	return nil
}

// instantWinner says who won a contest that nothing waits for, or "" when the defender
// still has to answer: a grapple that came from an attack is won by its initiator, and an
// escape against a fixed DC by whoever the roll puts ahead.
func (r *contestRun) instantWinner() string {
	switch {
	case r.st.byAttack:
		return winnerInitiator
	case r.row.Kind == contestKindEscapeDC:
		if combat.MeetsEscapeDC(int(r.roll.Total), int(*r.row.EscapeDc)) {
			return winnerInitiator
		}
		return winnerDefender
	}
	return ""
}

// writeContest is the write phase: the contest row and, when nothing waits, its effect (the
// grapple or the release) and the settled state. It returns the contest, the log line and
// who won ("" while the defender has to answer).
func (s *Service) writeContest(ctx context.Context, r *contestRun) (contestID, line, winner string, err error) {
	c := r.c
	r.row.Status = contestAwaitingDefender
	winner = r.instantWinner()
	if winner != "" {
		r.row.Winner = &winner
		r.row.ResolvedAt = &c.now
	}
	inserted, err := c.q.InsertContest(ctx, r.row)
	if err != nil {
		return "", "", "", fmt.Errorf("open the contest: %w", err)
	}
	if winner != "" {
		// A fixed DC settles a grapple or an escape: nothing waits, whatever settleContest says.
		_, line, err = settleContest(ctx, c, inserted, winner, r.initiator, r.defender)
		if err != nil {
			return "", "", "", err
		}
		if inserted, err = c.q.SetContestState(ctx, playdb.SetContestStateParams{EncounterID: c.enc.ID, ID: inserted.ID, Status: contestResolved, ShoveOutcome: nil, ResolvedAt: &c.now}); err != nil {
			return "", "", "", fmt.Errorf("settle the contest: %w", err)
		}
	}
	if err := touchEncounter(ctx, c); err != nil {
		return "", "", "", err
	}
	return inserted.ID, line, winner, nil
}

// startContestTx is the whole change of StartContest: read and validate, resolve the
// sides and what they spend, roll, write.
func (s *Service) startContestTx(ctx context.Context, c *combatTx, st contestStart) (actionEvent, error) {
	r, err := s.readContestRun(ctx, c, st)
	if err != nil {
		return actionEvent{}, err
	}
	if st.purpose == playv1.ContestPurpose_CONTEST_PURPOSE_ESCAPE {
		err = s.resolveEscape(ctx, r)
	} else {
		err = s.resolveAttackContest(ctx, r)
	}
	if err != nil {
		return actionEvent{}, err
	}
	r.row.DefenderID = r.defender.ID
	if !st.byAttack {
		if err := s.rollContestInitiator(ctx, r); err != nil {
			return actionEvent{}, err
		}
	}
	contestID, line, winner, err := s.writeContest(ctx, r)
	if err != nil {
		return actionEvent{}, err
	}
	c.characterID = &r.initiator.CharacterID
	made := actionEvent{
		Round: c.enc.Round, Secret: secretOf(r.initiator, r.defender), Actor: r.initiator.ID, Target: r.defender.ID,
		Contest: &contestEvent{ContestID: contestID, Purpose: st.purposeKey, Line: line, Waiting: winner == ""},
	}
	if r.usedMan {
		made.ManeuverSpent = []string{r.man.Resource}
	}
	return made, nil
}

// StartContest implements playv1connect.ContestServiceHandler.
func (s *Service) StartContest(
	ctx context.Context,
	req *connect.Request[playv1.StartContestRequest],
) (*connect.Response[playv1.StartContestResponse], error) {
	st, err := parseContestStart(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	var made actionEvent
	res, err := s.write(ctx, combatWrite{m: st.m, key: st.key, hash: idem.Hash(req.Msg), kind: eventContestStarted, encounterID: st.encID}, func(c *combatTx) (any, error) {
		made = actionEvent{}
		var err error
		if made, err = s.startContestTx(ctx, c, st); err != nil {
			return nil, err
		}
		return made, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "start a contest", err)
	}
	ev, err := resultEvent(res, made)
	if err != nil {
		return nil, s.dbError(ctx, "read the contest", err)
	}
	return contestResponse(ctx, s, st.m, res, ev, func(out *playv1.Encounter, view *playv1.ContestView) *playv1.StartContestResponse {
		return &playv1.StartContestResponse{Encounter: out, Contest: view}
	})
}

// initiatorIsNPC says the combatant is not a player's.
func initiatorIsNPC(c playdb.Combatant) bool { return c.UserID == nil }

// mustBeInReach checks, for a player, that the target is a size the combatant may grapple
// or shove and within its melee reach (SRD 5.1, "Grappling"). The master has the last
// word. Without a map nobody has a square and the master judges the reach.
func (s *Service) mustBeInReach(c *combatTx, v combatViewer, who, target playdb.Combatant) error {
	if v.master {
		return nil
	}
	if !combat.CanGrappleOrShove(who.Size, target.Size) {
		return errContest(playv1.ContestBlockedReason_CONTEST_BLOCKED_REASON_TARGET_TOO_BIG, "the target is more than one size larger")
	}
	if isTheatre(c.enc) {
		return nil
	}
	if !placed(who) || !placed(target) {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_PLACED, "the combatants must be on the map")
	}
	if dist, _ := distanceFt(who, target); dist > combat.MeleeReachFt {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TARGET_OUT_OF_REACH, "the target is beyond your reach",
			func(b *playv1.EncounterBlocked) { b.MissingFt = dist - combat.MeleeReachFt })
	}
	return nil
}

// contestAnswer is a RespondContest request read and checked: everything the transaction
// needs that does not come from the database.
type contestAnswer struct {
	m         authz.Membership
	key       string
	encID     string
	contestID string
	skill     playv1.ContestSkill
	// leave says the player leaves the roll to the master (no roll is read then).
	leave bool
	in    checkInput
}

// parseContestAnswer is the first phase of RespondContest: read and validate the request, in
// the order the refusals have always come.
func parseContestAnswer(ctx context.Context, msg *playv1.RespondContestRequest) (contestAnswer, error) {
	m, key, encID, err := contestCaller(ctx, msg.GetCampaignId(), msg.GetIdempotencyKey(), msg.GetEncounterId())
	if err != nil {
		return contestAnswer{}, err
	}
	contestID, err := parseCombatID(msg.GetContestId(), "contest")
	if err != nil {
		return contestAnswer{}, err
	}
	ans := contestAnswer{m: m, key: key, encID: encID, contestID: contestID, skill: msg.GetSkill(), leave: msg.GetDeferToMaster()}
	if ans.skill == playv1.ContestSkill_CONTEST_SKILL_UNSPECIFIED {
		ans.skill = playv1.ContestSkill_CONTEST_SKILL_ATHLETICS
	}
	switch {
	case !ans.leave:
		if ans.in, err = parseCheckInput(msg.GetRoll()); err != nil {
			return contestAnswer{}, err
		}
	case m.Role == authz.RoleMaster:
		return contestAnswer{}, connect.NewError(connect.CodeInvalidArgument, errors.New("the master rolls himself: leaveto_master is the player's"))
	}
	return ans, nil
}

// answerRun is what one RespondContest transaction knows as it goes: the request, the combat
// as read, the contest and its sides.
type answerRun struct {
	ans       contestAnswer
	c         *combatTx
	cs        []playdb.Combatant
	v         combatViewer
	row       playdb.CombatContest
	initiator playdb.Combatant
	defender  playdb.Combatant
	// skillKey is the skill the defender rolls (or the one it chose, when it leaves the roll).
	skillKey string
}

// sidesOfContest finds the two sides of a contest and refuses a caller who owns neither
// the side that acts (the defender for an answer, the initiator for a shove's choice): a
// player who is not in the contest does not know it exists.
func sidesOfContest(cs []playdb.Combatant, row playdb.CombatContest, v combatViewer, ownerIsDefender bool, deniedMsg string) (initiator, defender playdb.Combatant, err error) {
	initiator, okI := combatantByID(cs, row.InitiatorID)
	defender, okD := combatantByID(cs, row.DefenderID)
	if !okI || !okD {
		return initiator, defender, errNotAwaiting()
	}
	owner, other := initiator, defender
	if ownerIsDefender {
		owner, other = defender, initiator
	}
	if !v.master && !v.owns(owner) {
		if !v.owns(other) {
			return initiator, defender, connect.NewError(connect.CodeNotFound, errors.New("contest not found"))
		}
		return initiator, defender, connect.NewError(connect.CodePermissionDenied, errors.New(deniedMsg))
	}
	return initiator, defender, nil
}

// answerSkillKey is the skill the defender rolls: an escape is contested by the grappler's
// Athletics, and a roll the player left to the master uses the player's choice.
func answerSkillKey(row playdb.CombatContest, skill playv1.ContestSkill, previous *contestRoll, deferred bool) string {
	switch {
	case row.Purpose == purposeEscape:
		return skillAthletics
	case deferred:
		return previous.Skill
	}
	return skillKeyOf(skill)
}

// readAnswerRun is the read-and-validate phase of the transaction: the combat runs, the
// contest is the caller's to answer and waits for an answer from them.
func (s *Service) readAnswerRun(ctx context.Context, c *combatTx, ans contestAnswer) (*answerRun, error) {
	if err := mustBeRunning(c); err != nil {
		return nil, err
	}
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return nil, fmt.Errorf("list the combatants: %w", err)
	}
	v := c.viewer(ans.m, cs)
	row, err := c.q.GetContest(ctx, playdb.GetContestParams{EncounterID: c.enc.ID, ID: ans.contestID})
	if err != nil {
		return nil, contestNotFound(err)
	}
	initiator, defender, err := sidesOfContest(cs, row, v, true, "only the one that answers or the master may do this")
	if err != nil {
		return nil, err
	}
	if row.Status != contestAwaitingDefender || row.Kind != contestKindContest {
		return nil, errNotAwaiting()
	}
	previous, err := decodeRoll(row.DefenderRoll)
	if err != nil {
		return nil, err
	}
	deferred := previous != nil && previous.Deferred
	if deferred && !v.master {
		return nil, errNotAwaiting() // it waits for the master now
	}
	return &answerRun{
		ans: ans, c: c, cs: cs, v: v, row: row, initiator: initiator, defender: defender,
		skillKey: answerSkillKey(row, ans.skill, previous, deferred),
	}, nil
}

// leaveRollToMaster keeps the player's choice of skill and leaves the roll to the master.
func leaveRollToMaster(ctx context.Context, r *answerRun) error {
	stored, err := encodeRoll(contestRoll{Skill: r.skillKey, Deferred: true})
	if err != nil {
		return err
	}
	if _, err := r.c.q.SetContestAnswered(ctx, playdb.SetContestAnsweredParams{
		EncounterID: r.c.enc.ID, ID: r.row.ID, DefenderRoll: stored, Winner: nil, Status: contestAwaitingDefender, ResolvedAt: nil,
	}); err != nil {
		return fmt.Errorf("leave the roll to the master: %w", err)
	}
	return nil
}

// rollDefender rolls the defender's side, settles the contest and writes the answer. It
// returns the contest part of the event.
func (s *Service) rollDefender(ctx context.Context, r *answerRun) (*contestEvent, error) {
	c := r.c
	if !r.v.master {
		if err := s.mustRollThisWay(ctx, c.tx, r.ans.m, r.ans.in.rollInput()); err != nil {
			return nil, err
		}
	}
	roll, err := s.rollFor(ctx, c, r.defender, r.skillKey, r.ans.in, r.cs)
	if err != nil {
		return nil, err
	}
	roll.ByMaster = r.v.master && !initiatorIsNPC(r.defender)
	stored, err := encodeRoll(roll)
	if err != nil {
		return nil, err
	}
	iRoll, err := decodeRoll(r.row.InitiatorRoll)
	if err != nil || iRoll == nil {
		return nil, fmt.Errorf("read the initiator's roll of contest %s: %w", r.row.ID, err)
	}
	winner := winnerKey(combat.CompareContest(int(iRoll.Total), int(roll.Total)))
	status, line, err := settleContest(ctx, c, r.row, winner, r.initiator, r.defender)
	if err != nil {
		return nil, err
	}
	resolvedAt := &c.now
	if status == contestAwaitingOutcome {
		resolvedAt = nil
	}
	if _, err := c.q.SetContestAnswered(ctx, playdb.SetContestAnsweredParams{
		EncounterID: c.enc.ID, ID: r.row.ID, DefenderRoll: stored, Winner: &winner, Status: status, ResolvedAt: resolvedAt,
	}); err != nil {
		return nil, fmt.Errorf("answer the contest: %w", err)
	}
	return &contestEvent{Line: line, Waiting: status == contestAwaitingOutcome}, nil
}

// answerContestTx is the whole change of RespondContest: read and validate, then the
// deferral or the roll, then the event.
func (s *Service) answerContestTx(ctx context.Context, c *combatTx, ans contestAnswer) (actionEvent, error) {
	r, err := s.readAnswerRun(ctx, c, ans)
	if err != nil {
		return actionEvent{}, err
	}
	part := &contestEvent{Waiting: true}
	if ans.leave {
		err = leaveRollToMaster(ctx, r)
	} else {
		part, err = s.rollDefender(ctx, r)
	}
	if err != nil {
		return actionEvent{}, err
	}
	if err := touchEncounter(ctx, c); err != nil {
		return actionEvent{}, err
	}
	c.characterID = &r.defender.CharacterID
	part.ContestID, part.Purpose = r.row.ID, r.row.Purpose
	return actionEvent{
		Round: c.enc.Round, Secret: secretOf(r.initiator, r.defender), Actor: r.initiator.ID, Target: r.defender.ID, Contest: part,
	}, nil
}

// RespondContest implements playv1connect.ContestServiceHandler.
func (s *Service) RespondContest(
	ctx context.Context,
	req *connect.Request[playv1.RespondContestRequest],
) (*connect.Response[playv1.RespondContestResponse], error) {
	ans, err := parseContestAnswer(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	kind := eventContestResolved
	if ans.leave {
		kind = eventContestDeferred
	}
	var made actionEvent
	res, err := s.write(ctx, combatWrite{m: ans.m, key: ans.key, hash: idem.Hash(req.Msg), kind: kind, encounterID: ans.encID}, func(c *combatTx) (any, error) {
		made = actionEvent{}
		var err error
		if made, err = s.answerContestTx(ctx, c, ans); err != nil {
			return nil, err
		}
		return made, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "answer a contest", err)
	}
	ev, err := resultEvent(res, made)
	if err != nil {
		return nil, s.dbError(ctx, "read the contest", err)
	}
	return contestResponse(ctx, s, ans.m, res, ev, func(out *playv1.Encounter, view *playv1.ContestView) *playv1.RespondContestResponse {
		return &playv1.RespondContestResponse{Encounter: out, Contest: view}
	})
}

// contestNotFound is a contest that is not in the combat.
func contestNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return connect.NewError(connect.CodeNotFound, errors.New("contest not found"))
	}
	return fmt.Errorf("find the contest: %w", err)
}

// pushPlan is where a shove's push would send the target, and whether the square is free.
type pushPlan struct {
	// has says there is a square to push to (a map, both on it).
	has bool
	to  grid.Square
	// blocked says the square is not free as the viewer knows the map and the creatures;
	// why says by what.
	blocked bool
	why     playv1.ShoveBlockedReason
	// reallyBlocked says it is not free at all, whatever the viewer knows.
	reallyBlocked bool
}

// pushPlanOf works out the push of a shove from the shover to the target: the square 5
// feet straight away from the shover, free when it is on the grid, not a wall or a column
// and not held by another creature. A player is told only what they know of the map and
// see of the creatures (RN-10): a push that really is blocked by something they do not
// see is offered and does not move the target.
func (s *Service) pushPlanOf(terrain, plan grid.Terrain, enc playdb.Encounter, cs []playdb.Combatant, shover, target playdb.Combatant, v combatViewer) pushPlan {
	if isTheatre(enc) || !placed(shover) || !placed(target) {
		return pushPlan{}
	}
	col, row := combat.PushStep(int(*shover.GridCol), int(*shover.GridRow), int(*target.GridCol), int(*target.GridRow))
	to := grid.Square{Col: col, Row: row}
	out := pushPlan{has: true, to: to}
	from := squareOfCombatant(target)
	mover := moverOf(target)
	wallIn := func(t grid.Terrain) bool { return t.Move(from, to, grid.OccupantMap{}, mover).Blocked }
	creatureAt := func(visible bool) bool {
		for _, o := range cs {
			if o.ID == target.ID || o.Defeated || !placed(o) || squareOfCombatant(o) != to {
				continue
			}
			if !visible || v.sees(o) {
				return true
			}
		}
		return false
	}
	switch {
	case wallIn(plan):
		out.blocked, out.why = true, playv1.ShoveBlockedReason_SHOVE_BLOCKED_REASON_WALL
	case creatureAt(true):
		out.blocked, out.why = true, playv1.ShoveBlockedReason_SHOVE_BLOCKED_REASON_CREATURE
	}
	out.reallyBlocked = out.blocked || wallIn(terrain) || creatureAt(false)
	return out
}

// shoveChoiceReq is a ResolveShove request read and checked.
type shoveChoiceReq struct {
	m         authz.Membership
	key       string
	encID     string
	contestID string
	outcome   playv1.ShoveOutcome
}

// parseShoveChoice is the first phase of ResolveShove: read and validate the request, in the
// order the refusals have always come.
func parseShoveChoice(ctx context.Context, msg *playv1.ResolveShoveRequest) (shoveChoiceReq, error) {
	m, key, encID, err := contestCaller(ctx, msg.GetCampaignId(), msg.GetIdempotencyKey(), msg.GetEncounterId())
	if err != nil {
		return shoveChoiceReq{}, err
	}
	contestID, err := parseCombatID(msg.GetContestId(), "contest")
	if err != nil {
		return shoveChoiceReq{}, err
	}
	outcome := msg.GetOutcome()
	if outcome == playv1.ShoveOutcome_SHOVE_OUTCOME_UNSPECIFIED {
		return shoveChoiceReq{}, connect.NewError(connect.CodeInvalidArgument, errors.New("outcome must be PRONE, PUSH or STAYS"))
	}
	return shoveChoiceReq{m: m, key: key, encID: encID, contestID: contestID, outcome: outcome}, nil
}

// shoveRun is what one ResolveShove transaction knows as it goes: the request, the combat as
// read, the shove, its sides and where the push would send the target.
type shoveRun struct {
	req    shoveChoiceReq
	c      *combatTx
	row    playdb.CombatContest
	shover playdb.Combatant
	target playdb.Combatant
	push   pushPlan
}

// readShoveRun is the read-and-validate phase of the transaction: the combat runs, the shove
// is the caller's to resolve and waits for its winner, and the push is worked out as the
// caller knows the map.
func (s *Service) readShoveRun(ctx context.Context, c *combatTx, req shoveChoiceReq) (*shoveRun, error) {
	if err := mustBeRunning(c); err != nil {
		return nil, err
	}
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return nil, fmt.Errorf("list the combatants: %w", err)
	}
	v := c.viewer(req.m, cs)
	row, err := c.q.GetContest(ctx, playdb.GetContestParams{EncounterID: c.enc.ID, ID: req.contestID})
	if err != nil {
		return nil, contestNotFound(err)
	}
	shover, target, err := sidesOfContest(cs, row, v, false, "only the one that won the shove or the master may do this")
	if err != nil {
		return nil, err
	}
	if row.Status != contestAwaitingOutcome || row.Purpose != purposeShove {
		return nil, errNotAwaiting()
	}
	terrain, err := s.terrainOf(ctx, c.tx, req.m.CampaignID, c.enc)
	if err != nil {
		return nil, err
	}
	known, err := c.sight.knownTerrain(ctx, c.tx, v)
	if err != nil {
		return nil, err
	}
	push := s.pushPlanOf(terrain, planOn(v, terrain, known), c.enc, cs, shover, target, v)
	return &shoveRun{req: req, c: c, row: row, shover: shover, target: target, push: push}, nil
}

// shoveKnocksProne is the outcome PRONE: the target falls.
func shoveKnocksProne(ctx context.Context, r *shoveRun) (stored, line string, err error) {
	err = setConditionsOf(ctx, r.c, r.target, withCondition(r.target.Conditions, condProne))
	return shoveProne, contestLineShoveProne, err
}

// shovePushes is the outcome PUSH: the target goes to the square behind it, unless something
// the shover does not see holds the square (then it stays, and the line says so).
func shovePushes(ctx context.Context, r *shoveRun) (stored, line string, err error) {
	push := r.push
	if push.blocked {
		return "", "", errContest(playv1.ContestBlockedReason_CONTEST_BLOCKED_REASON_PUSH_BLOCKED, "the square behind the target is blocked")
	}
	if !push.has {
		return shovePush, contestLineShovePushed, nil
	}
	if push.reallyBlocked {
		return shoveStays, contestLineShoveStays, nil
	}
	if err := r.c.q.SetCombatantPlace(ctx, playdb.SetCombatantPlaceParams{
		ID: r.target.ID, GridCol: new(clamp32(push.to.Col, 0, math.MaxInt32)), GridRow: new(clamp32(push.to.Row, 0, math.MaxInt32)), CoverMark: "none",
	}); err != nil {
		return "", "", fmt.Errorf("push the target: %w", err)
	}
	return shovePush, contestLineShovePushed, nil
}

// shoveStaysPut is the outcome STAYS: for a push that is blocked, the target keeps its place.
func shoveStaysPut(_ context.Context, r *shoveRun) (stored, line string, err error) {
	if !r.push.has || !r.push.reallyBlocked {
		return "", "", connect.NewError(connect.CodeInvalidArgument, errors.New("outcome STAYS is for a push that is blocked"))
	}
	return shoveStays, contestLineShoveStays, nil
}

// shoveOutcomes say what each outcome writes and what the stored outcome and the log line
// are.
var shoveOutcomes = map[playv1.ShoveOutcome]func(context.Context, *shoveRun) (string, string, error){
	playv1.ShoveOutcome_SHOVE_OUTCOME_PRONE: shoveKnocksProne,
	playv1.ShoveOutcome_SHOVE_OUTCOME_PUSH:  shovePushes,
	playv1.ShoveOutcome_SHOVE_OUTCOME_STAYS: shoveStaysPut,
}

// resolveShoveTx is the whole change of ResolveShove: read and validate, apply the outcome,
// settle the shove, write the event.
func (s *Service) resolveShoveTx(ctx context.Context, c *combatTx, req shoveChoiceReq) (actionEvent, error) {
	r, err := s.readShoveRun(ctx, c, req)
	if err != nil {
		return actionEvent{}, err
	}
	var stored, line string
	if apply, ok := shoveOutcomes[req.outcome]; ok {
		if stored, line, err = apply(ctx, r); err != nil {
			return actionEvent{}, err
		}
	}
	if _, err := c.q.SetContestState(ctx, playdb.SetContestStateParams{EncounterID: c.enc.ID, ID: r.row.ID, Status: contestResolved, ShoveOutcome: &stored, ResolvedAt: &c.now}); err != nil {
		return actionEvent{}, fmt.Errorf("settle the shove: %w", err)
	}
	if err := touchEncounter(ctx, c); err != nil {
		return actionEvent{}, err
	}
	c.characterID = &r.shover.CharacterID
	return actionEvent{
		Round: c.enc.Round, Secret: secretOf(r.shover, r.target), Actor: r.shover.ID, Target: r.target.ID,
		Contest: &contestEvent{ContestID: r.row.ID, Purpose: purposeShove, Line: line},
	}, nil
}

// ResolveShove implements playv1connect.ContestServiceHandler.
func (s *Service) ResolveShove(
	ctx context.Context,
	req *connect.Request[playv1.ResolveShoveRequest],
) (*connect.Response[playv1.ResolveShoveResponse], error) {
	choice, err := parseShoveChoice(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	var made actionEvent
	res, err := s.write(ctx, combatWrite{m: choice.m, key: choice.key, hash: idem.Hash(req.Msg), kind: eventShoveResolved, encounterID: choice.encID}, func(c *combatTx) (any, error) {
		made = actionEvent{}
		var err error
		if made, err = s.resolveShoveTx(ctx, c, choice); err != nil {
			return nil, err
		}
		return made, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "resolve a shove", err)
	}
	ev, err := resultEvent(res, made)
	if err != nil {
		return nil, s.dbError(ctx, "read the shove", err)
	}
	return contestResponse(ctx, s, choice.m, res, ev, func(out *playv1.Encounter, view *playv1.ContestView) *playv1.ResolveShoveResponse {
		return &playv1.ResolveShoveResponse{Encounter: out, Contest: view}
	})
}

// contestClose is a CloseContest request read and checked.
type contestClose struct {
	m         authz.Membership
	key       string
	encID     string
	contestID string
}

// parseContestClose is the first phase of CloseContest: only the master closes, and the
// refusals come in the order they always have.
func parseContestClose(ctx context.Context, msg *playv1.CloseContestRequest) (contestClose, error) {
	m, err := authz.RequireCampaignRole(ctx, msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return contestClose{}, err
	}
	key, err := parseKey(msg.GetIdempotencyKey())
	if err != nil {
		return contestClose{}, err
	}
	encID, err := parseCombatID(msg.GetEncounterId(), "encounter")
	if err != nil {
		return contestClose{}, err
	}
	contestID, err := parseCombatID(msg.GetContestId(), "contest")
	if err != nil {
		return contestClose{}, err
	}
	return contestClose{m: m, key: key, encID: encID, contestID: contestID}, nil
}

// closeContestTx is the whole change of CloseContest: a contest that waits for its defender
// or for its winner's choice ends CLOSED, with what was decided so far kept.
func (s *Service) closeContestTx(ctx context.Context, c *combatTx, req contestClose) (actionEvent, error) {
	if err := notEnded(c.enc); err != nil {
		return actionEvent{}, err
	}
	row, err := c.q.GetContest(ctx, playdb.GetContestParams{EncounterID: c.enc.ID, ID: req.contestID})
	if err != nil {
		return actionEvent{}, contestNotFound(err)
	}
	if row.Status != contestAwaitingDefender && row.Status != contestAwaitingOutcome {
		return actionEvent{}, errNotAwaiting()
	}
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return actionEvent{}, fmt.Errorf("list the combatants: %w", err)
	}
	initiator, _ := combatantByID(cs, row.InitiatorID)
	defender, _ := combatantByID(cs, row.DefenderID)
	if _, err := c.q.SetContestState(ctx, playdb.SetContestStateParams{EncounterID: c.enc.ID, ID: row.ID, Status: contestClosed, ShoveOutcome: row.ShoveOutcome, ResolvedAt: &c.now}); err != nil {
		return actionEvent{}, fmt.Errorf("close the contest: %w", err)
	}
	if err := touchEncounter(ctx, c); err != nil {
		return actionEvent{}, err
	}
	return actionEvent{
		Round: c.enc.Round, Secret: secretOf(initiator, defender), Actor: initiator.ID, Target: defender.ID,
		Contest: &contestEvent{ContestID: row.ID, Purpose: row.Purpose, Line: contestLineClosed},
	}, nil
}

// CloseContest implements playv1connect.ContestServiceHandler.
func (s *Service) CloseContest(
	ctx context.Context,
	req *connect.Request[playv1.CloseContestRequest],
) (*connect.Response[playv1.CloseContestResponse], error) {
	cl, err := parseContestClose(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	var made actionEvent
	res, err := s.write(ctx, combatWrite{m: cl.m, key: cl.key, hash: idem.Hash(req.Msg), kind: eventContestClosed, encounterID: cl.encID}, func(c *combatTx) (any, error) {
		made = actionEvent{}
		var err error
		if made, err = s.closeContestTx(ctx, c, cl); err != nil {
			return nil, err
		}
		return made, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "close a contest", err)
	}
	out, err := s.finish(ctx, cl.m, res, s.changed(cl.m.CampaignID))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.CloseContestResponse{Encounter: out}), nil
}

// releaseTx is the whole change of ReleaseGrapple: the grappler lets go whenever it likes,
// with no action (SRD 5.1, "Grappling").
func (s *Service) releaseTx(ctx context.Context, c *combatTx, m authz.Membership, grappledID string) (actionEvent, error) {
	if err := notEnded(c.enc); err != nil {
		return actionEvent{}, err
	}
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return actionEvent{}, fmt.Errorf("list the combatants: %w", err)
	}
	v := c.viewer(m, cs)
	grappled, err := findCombatant(cs, grappledID, v)
	if err != nil {
		return actionEvent{}, err
	}
	holds, err := c.q.ListHolds(ctx, c.enc.ID)
	if err != nil {
		return actionEvent{}, fmt.Errorf("list the holds: %w", err)
	}
	hold, ok := holdOn(holdsOf(holds), grappled)
	if !ok {
		return actionEvent{}, errContest(playv1.ContestBlockedReason_CONTEST_BLOCKED_REASON_NOT_GRAPPLED, "the combatant is not grappled")
	}
	grappler, ok := combatantByID(cs, hold.GrapplerID)
	if !ok {
		return actionEvent{}, errContest(playv1.ContestBlockedReason_CONTEST_BLOCKED_REASON_NOT_GRAPPLED, "the combatant is not grappled")
	}
	if err := v.mayAct(grappler); err != nil {
		return actionEvent{}, err
	}
	if err := releaseHold(ctx, c, grappled); err != nil {
		return actionEvent{}, err
	}
	if err := touchEncounter(ctx, c); err != nil {
		return actionEvent{}, err
	}
	c.characterID = &grappler.CharacterID
	return actionEvent{
		Round: c.enc.Round, Secret: secretOf(grappler, grappled), Actor: grappler.ID, Target: grappled.ID,
		Contest: &contestEvent{Purpose: purposeGrapple, Line: contestLineReleased},
	}, nil
}

// ReleaseGrapple implements playv1connect.ContestServiceHandler.
func (s *Service) ReleaseGrapple(
	ctx context.Context,
	req *connect.Request[playv1.ReleaseGrappleRequest],
) (*connect.Response[playv1.ReleaseGrappleResponse], error) {
	m, key, encID, err := contestCaller(ctx, req.Msg.GetCampaignId(), req.Msg.GetIdempotencyKey(), req.Msg.GetEncounterId())
	if err != nil {
		return nil, err
	}
	grappledID, err := parseCombatID(req.Msg.GetGrappledId(), "combatant")
	if err != nil {
		return nil, err
	}
	var made actionEvent
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventGrappleReleased, encounterID: encID}, func(c *combatTx) (any, error) {
		made = actionEvent{}
		var err error
		if made, err = s.releaseTx(ctx, c, m, grappledID); err != nil {
			return nil, err
		}
		return made, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "release a grapple", err)
	}
	ev, err := resultEvent(res, made)
	if err != nil {
		return nil, s.dbError(ctx, "read the release", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChangedFor(ctx, m.CampaignID, d, ev.Actor, ev.Target)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !ev.Secret)
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.ReleaseGrappleResponse{Encounter: out}), nil
}

// holdBroken says a hold's creatures are gone from the fight, the grappled one lost the
// condition, or the grappler is defeated or incapacitated (SRD 5.1, Conditions, Grappled).
func holdBroken(by map[string]playdb.Combatant, h playdb.CombatHold) bool {
	grappled, okG := by[h.GrappledID]
	grappler, okR := by[h.GrapplerID]
	return !okG || !okR || !isGrappled(grappled) || grappler.Defeated || grappled.Defeated ||
		slices.ContainsFunc(grappler.Conditions, func(k string) bool { return slices.Contains(incapacitating, k) })
}

// playerDown says one of two combatants is a player's character at 0 hit points, dead or
// not yet revived: it holds nobody and is held by nobody (SRD 5.1, "Grappled": the grappler
// is incapacitated).
func (s *Service) playerDown(ctx context.Context, c *combatTx, pair ...playdb.Combatant) (bool, error) {
	for _, who := range pair {
		if who.Kind != kindPlayer {
			continue
		}
		down, err := s.isDown(ctx, c.tx, c.session.CampaignID, who)
		if err != nil {
			return false, err
		}
		if down {
			return true, nil
		}
	}
	return false, nil
}

// holdEnds says whether a hold no longer holds: it is broken, a player's character is down,
// or the grappled creature is out of the grappler's reach.
func (s *Service) holdEnds(ctx context.Context, c *combatTx, by map[string]playdb.Combatant, h playdb.CombatHold) (bool, error) {
	if holdBroken(by, h) {
		return true, nil
	}
	grappled, grappler := by[h.GrappledID], by[h.GrapplerID]
	down, err := s.playerDown(ctx, c, grappler, grappled)
	if err != nil || down {
		return down, err
	}
	if isTheatre(c.enc) || !placed(grappled) || !placed(grappler) {
		return false, nil
	}
	dist, _ := distanceFt(grappler, grappled)
	return dist > combat.MeleeReachFt, nil
}

// pruneHolds ends the grapples that no longer hold, after any change of a combat (SRD 5.1,
// Conditions, Grappled): the condition ends when the grappler is incapacitated (or out of the
// fight), or when an effect removes the grappled creature from the grappler's reach (a push, a
// forced move, a teleport); and when the master took the condition off, the hold goes with it.
func (s *Service) pruneHolds(ctx context.Context, c *combatTx) error {
	if c.enc.ID == "" {
		return nil
	}
	holds, err := c.q.ListHolds(ctx, c.enc.ID)
	if err != nil || len(holds) == 0 {
		return err
	}
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the combatants: %w", err)
	}
	by := combatantsByID(cs)
	for _, h := range holds {
		ends, err := s.holdEnds(ctx, c, by, h)
		if err != nil {
			return err
		}
		if !ends {
			continue
		}
		if err := endHold(ctx, c, by, h); err != nil {
			return err
		}
	}
	return nil
}

// endHold lets go of a hold that ended: the condition goes when the creature still carries
// it, and the row always does.
func endHold(ctx context.Context, c *combatTx, by map[string]playdb.Combatant, h playdb.CombatHold) error {
	if grappled, ok := by[h.GrappledID]; ok && isGrappled(grappled) {
		return releaseHold(ctx, c, grappled)
	}
	if err := c.q.DeleteHold(ctx, h.GrappledID); err != nil {
		return fmt.Errorf("end the hold: %w", err)
	}
	return nil
}
