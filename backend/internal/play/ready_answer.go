package play

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/reaction"
)

// The answer to a READIED window (FireReadied, ready_fire.go). "Deixar passar" ends the row as ignored and keeps the
// reaction. "Usar" runs the held action through its own path with the reaction's economy, and the row is fired:
//   - an attack is RollAttack with catch_window_id naming the window (combat_attack.go): AnswerReaction USE sends it so,
//     with the target named at Ready, or the one in creature_ids; its roll waits for the windows any attack roll opens
//     (holdAttackRoll), and the use is spent when it rolls;
//   - a move is MoveCombatant by the readier while the window is open (combat_move.go): it needs a square, which
//     AnswerReaction does not carry, so AnswerReaction USE refuses it; it offers opportunity attacks as a walk does;
//   - another standard action is TakeAction's, charged to the reaction (useReadiedOtherTx).
// The use writes a readied_fired line just before the event of the action, which carries the row, the window and the
// line: the undo of the action takes the line back, and the row waits and the window opens again (readiedUseBack).

// readiedUse is a readied action being used: its open window and its row.
type readiedUse struct {
	w   playdb.ReactionWindow
	row playdb.ReadiedAction
}

// readiedUseOf checks that the window is the open READIED window of who, for a readied action of the kind that waits,
// and that who's reaction is free. A window that is not who's is refused as one that does not exist.
func readiedUseOf(ctx context.Context, c *combatTx, windowID string, who playdb.Combatant, kind string) (*readiedUse, error) {
	w, err := c.q.GetReactionWindowForUpdate(ctx, playdb.GetReactionWindowForUpdateParams{EncounterID: c.enc.ID, ID: windowID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (reaction.Kind(w.Kind) != reaction.Readied || conv.OrZero(w.ReactorID) != who.ID)) {
		return nil, errNotYourTurnToAnswer() // as a throw back's window that is not found: another's window is not told apart
	}
	if err != nil {
		return nil, fmt.Errorf("find the readied action's window: %w", err)
	}
	if err := mustBeOpen(w); err != nil {
		return nil, err
	}
	if err := mayUseNow(ctx, c, w, who); err != nil {
		return nil, err
	}
	row, err := c.q.GetWaitingReadiedAction(ctx, playdb.GetWaitingReadiedActionParams{ID: windowTriggerOf(w).Readied, EncounterID: c.enc.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errNotYourTurnToAnswer()
	}
	if err != nil {
		return nil, fmt.Errorf("read the readied action: %w", err)
	}
	if usedAs(row) != kind {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("the readied action holds another kind of action"))
	}
	if who.ReactionUsed {
		return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_REACTION_USED, "the reaction is used")
	}
	return &readiedUse{w: w, row: row}, nil
}

// mayUseNow refuses a use held for other windows or for a die, and one its readier's concentration save or the master's
// question holds (readiedGate). The replay of the held roll, or the run again of a roll held for a die, is the use the
// hold waited for.
func mayUseNow(ctx context.Context, c *combatTx, w playdb.ReactionWindow, who playdb.Combatant) error {
	if _, rerun := heldReplyOf(ctx); c.replay != nil || rerun {
		return nil
	}
	if err := mustNotBeHeld(ctx, c.q, c.enc.ID, w.ID); err != nil {
		return err
	}
	return c.svc.readiedGate(ctx, c, who)
}

// spend is what the use writes besides the action's own economy: the window answered, the row fired, the reaction's
// record and the readied_fired line. made is the action's event: it keeps the row, the window and the line.
func (u *readiedUse) spend(ctx context.Context, c *combatTx, who playdb.Combatant, made *actionEvent) error {
	if _, err := c.svc.closeWindow(ctx, c, u.w, windowAnswered, reaction.ReasonNone, windowOutcome{ByMaster: c.master, Used: true}, reaction.Readied); err != nil {
		return err
	}
	if err := c.q.SetReadiedActionStatus(ctx, playdb.SetReadiedActionStatusParams{ID: u.row.ID, Status: readiedFired}); err != nil {
		return fmt.Errorf("fire the readied action: %w", err)
	}
	if err := recordReaction(ctx, c, who.ID, string(reaction.Readied)); err != nil {
		return err
	}
	line := readiedRowEvent(u.row, who, c.enc.Round)
	line.Readied.Window, line.Readied.Ended = u.w.ID, readiedFired
	id, err := insertEventID(ctx, c, eventReadiedFired, &c.actorUserID, nil, line)
	if err != nil {
		return err
	}
	ev := readiedEventOf(u.row)
	ev.Window, ev.Ended, ev.Line = u.w.ID, readiedFired, id
	made.Readied = ev
	return nil
}

// answerReadied answers a READIED window, or says the window is not one (handled false).
func (s *Service) answerReadied(ctx context.Context, m authz.Membership, in answerInput, msg *playv1.AnswerReactionRequest) (*connect.Response[playv1.AnswerReactionResponse], bool, error) {
	w, err := s.queries.GetReactionWindow(ctx, playdb.GetReactionWindowParams{EncounterID: in.encID, ID: in.winID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil // the generic answer refuses it as it refuses any window it does not find
	}
	if err != nil {
		return nil, true, s.dbError(ctx, "find the reaction window", err)
	}
	if reaction.Kind(w.Kind) != reaction.Readied {
		return nil, false, nil
	}
	if err := s.mayAnswerReadied(ctx, m, in.encID, w); err != nil { // before anything the row holds is told
		return nil, true, err
	}
	if in.choice == playv1.ReactionChoice_REACTION_CHOICE_PASS {
		out, err := s.answerReadiedWrite(ctx, m, in, msg, eventReadiedFired, s.passReadiedTx)
		return out, true, err
	}
	row, err := s.queries.GetReadiedAction(ctx, playdb.GetReadiedActionParams{ID: windowTriggerOf(w).Readied, EncounterID: in.encID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, true, errNotYourTurnToAnswer()
	}
	if err != nil {
		return nil, true, s.dbError(ctx, "read the readied action", err)
	}
	if err := releaseFieldsOf(msg, row); err != nil {
		return nil, true, err
	}
	switch row.Kind {
	case readiedAttack:
		out, err := s.useReadiedAttack(ctx, m, in, msg, w, row)
		return out, true, err
	case readiedOther:
		out, err := s.answerReadiedWrite(ctx, m, in, msg, eventActionTaken, s.useReadiedOtherTx)
		return out, true, err
	case readiedMove:
		return nil, true, connect.NewError(connect.CodeInvalidArgument, errors.New("a readied move is made with MoveCombatant while its window is open"))
	case readiedSpell:
		if heldSpellAttackRow(row) {
			out, err := s.useReadiedAttack(ctx, m, in, msg, w, row)
			return out, true, err
		}
		out, err := s.useReadiedSpell(ctx, m, in, msg, row)
		return out, true, err
	}
	return nil, true, connect.NewError(connect.CodeInvalidArgument, errors.New("this readied action cannot be used here"))
}

// mayAnswerReadied refuses a player who does not play the window's reactor as the generic answer refuses a window it
// does not find (RN-10): the master answers any.
func (s *Service) mayAnswerReadied(ctx context.Context, m authz.Membership, encID string, w playdb.ReactionWindow) error {
	if m.Role == authz.RoleMaster {
		return nil
	}
	cs, err := s.queries.ListCombatants(ctx, encID)
	if err != nil {
		return s.dbError(ctx, "list the combatants", err)
	}
	reactor, ok := combatantByID(cs, conv.OrZero(w.ReactorID))
	if !ok || reactor.UserID == nil || *reactor.UserID != m.UserID || isCreature(reactor) {
		return errWindowDenied()
	}
	return nil
}

// fieldsNotUsed refuses the targets and the table's dice where the answer takes none.
func fieldsNotUsed(msg *playv1.AnswerReactionRequest) error {
	if len(msg.GetTargets()) > 0 {
		return badTargets(targetsNotUsed)
	}
	return diceNotUsed(msg)
}

// diceNotUsed refuses the table's dice (pool_sum, d20_faces, a mode) where the answer rolls nothing of its own: a pass,
// another kind of window, a readied action that is not an attack or a spell. An answer never drops what it carries.
func diceNotUsed(msg *playv1.AnswerReactionRequest) error {
	if _, pool := msg.GetRoll().(*playv1.AnswerReactionRequest_PoolSum); pool || len(msg.GetD20Faces()) > 0 || len(msg.GetRayRolls()) > 0 || msg.GetRollMode() != playv1.RollMode_ROLL_MODE_UNSPECIFIED || msg.GetModeReason() != "" {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("pool_sum, d20_faces, ray_rolls and the roll mode are for the release of a readied attack or spell"))
	}
	return nil
}

// releaseFieldsOf checks what an answer carries against the readied action it releases, before the request the release
// builds is checked as the cast or the attack checks it: targets only for a cast, one way of rolling, and no pool for an
// attack.
func releaseFieldsOf(msg *playv1.AnswerReactionRequest, row playdb.ReadiedAction) error {
	kind := usedAs(row)
	if kind != readiedSpell && len(msg.GetTargets()) > 0 { // not released as a cast
		return badTargets(targetsNotUsed)
	}
	if kind != readiedAttack && kind != readiedSpell {
		return diceNotUsed(msg)
	}
	if msg.GetRoll() != nil && len(msg.GetD20Faces()) > 0 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("set one of roll_in_app, typed, pool_sum or d20_faces"))
	}
	if _, pool := msg.GetRoll().(*playv1.AnswerReactionRequest_PoolSum); pool && kind == readiedAttack {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("pool_sum is for a spell that rolls a pool of dice"))
	}
	if kind == readiedAttack && len(msg.GetRayRolls()) > 0 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("ray_rolls are for a spell with several attack rolls (rays)"))
	}
	return rayRollsAlone(len(msg.GetRayRolls()) > 0 && (msg.GetRoll() != nil || len(msg.GetD20Faces()) > 0), "roll_in_app, typed, pool_sum and d20_faces")
}

// targetsNotUsed refuses targets where the answer does not cast with them: only the use of a readied spell does.
const targetsNotUsed = "targets are for the use of a readied spell"

// answerReadiedWrite runs an answer of a READIED window in its own change of the kind, and answers as AnswerReaction does.
func (s *Service) answerReadiedWrite(ctx context.Context, m authz.Membership, in answerInput, msg *playv1.AnswerReactionRequest, kind string,
	do func(ctx context.Context, c *combatTx, m authz.Membership, winID string) (actionEvent, error),
) (*connect.Response[playv1.AnswerReactionResponse], error) {
	var made actionEvent
	res, err := s.write(ctx, combatWrite{m: m, key: in.key, hash: idem.Hash(msg), kind: kind, encounterID: in.encID}, func(c *combatTx) (any, error) {
		ev, err := do(ctx, c, m, in.winID)
		made = ev
		return ev, err
	})
	if err != nil {
		return nil, s.dbError(ctx, "answer a readied action", err)
	}
	enc, err := s.readiedAnswer(ctx, m, res, made)
	if err != nil {
		return nil, err
	}
	used := kind != eventReadiedFired
	return connect.NewResponse(&playv1.AnswerReactionResponse{Encounter: enc, Result: &playv1.ReactionResult{
		Kind: playv1.ReactionKind_REACTION_KIND_READIED, ByMaster: m.Role == authz.RoleMaster, Used: used,
	}}), nil
}

// passReadiedTx is "Deixar passar": the row is ignored, the reaction stays free, and the line says so.
func (s *Service) passReadiedTx(ctx context.Context, c *combatTx, m authz.Membership, winID string) (actionEvent, error) {
	if err := notEnded(c.enc); err != nil {
		return actionEvent{}, err
	}
	w, reactor, err := s.windowToAnswer(ctx, c, m, winID)
	if err != nil {
		return actionEvent{}, err
	}
	if err := mustNotBeHeld(ctx, c.q, c.enc.ID, w.ID); err != nil {
		return actionEvent{}, err
	}
	row, err := c.q.GetWaitingReadiedAction(ctx, playdb.GetWaitingReadiedActionParams{ID: windowTriggerOf(w).Readied, EncounterID: c.enc.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return actionEvent{}, errNotYourTurnToAnswer()
	}
	if err != nil {
		return actionEvent{}, fmt.Errorf("read the readied action: %w", err)
	}
	if _, err := s.closeWindow(ctx, c, w, windowAnswered, reaction.ReasonNone, windowOutcome{ByMaster: c.master}, ""); err != nil {
		return actionEvent{}, err
	}
	if err := c.q.SetReadiedActionStatus(ctx, playdb.SetReadiedActionStatusParams{ID: row.ID, Status: readiedIgnored}); err != nil {
		return actionEvent{}, fmt.Errorf("let the readied action pass: %w", err)
	}
	if err := touch(ctx, c); err != nil {
		return actionEvent{}, err
	}
	c.characterID = &reactor.CharacterID
	ev := readiedRowEvent(row, reactor, c.enc.Round)
	ev.Readied.Window, ev.Readied.Ended = w.ID, readiedIgnored
	if row.Kind == readiedSpell { // a spell let pass is lost, with its slot (settleReadiedSpells ends its concentration)
		lost := readiedRowEvent(row, reactor, c.enc.Round)
		lost.Readied.Window, lost.Readied.Ended = w.ID, readiedIgnored
		if ev.Readied.Line, err = insertEventID(ctx, c, eventReadiedLost, &c.actorUserID, nil, lost); err != nil {
			return actionEvent{}, err
		}
	}
	return ev, nil
}

// useReadiedAttack is "Usar" on a readied attack: RollAttack with the window, the held attack, the target named at Ready
// (or the request's creature_ids[0]) and the answer's die, under the answer's key.
func (s *Service) useReadiedAttack(ctx context.Context, m authz.Membership, in answerInput, msg *playv1.AnswerReactionRequest, w playdb.ReactionWindow, row playdb.ReadiedAction) (*connect.Response[playv1.AnswerReactionResponse], error) {
	target := conv.OrZero(row.TargetID)
	if ids := msg.GetCreatureIds(); len(ids) > 0 {
		target = ids[0]
	}
	if target == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("a readied attack needs a target: the creature named at Ready, or creature_ids"))
	}
	req := &playv1.RollAttackRequest{
		CampaignId: msg.GetCampaignId(), EncounterId: in.encID, AttackerId: row.CombatantID, AttackKey: conv.OrZero(row.ActionKey),
		TargetId: target, IdempotencyKey: in.key, CatchWindowId: w.ID, Roll: &playv1.RollAttackRequest_RollInApp{RollInApp: true},
	}
	if typed, ok := msg.GetRoll().(*playv1.AnswerReactionRequest_Typed); ok {
		req.Roll = &playv1.RollAttackRequest_D20Face{D20Face: typed.Typed}
	}
	if len(msg.GetD20Faces()) > 0 { // the table's dice instead of the app's: the attack reads them as it reads its own
		req.Roll, req.D20Faces = nil, msg.GetD20Faces()
	}
	req.RollMode, req.ModeReason = msg.GetRollMode(), msg.GetModeReason()
	res, err := s.rollAttack(ctx, m, connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.AnswerReactionResponse{
		Encounter: res.Msg.GetEncounter(),
		Result: &playv1.ReactionResult{
			Kind: playv1.ReactionKind_REACTION_KIND_READIED, ByMaster: m.Role == authz.RoleMaster,
			Used: !res.Msg.GetRoll().GetHeldForReaction(), // a roll held by a Cutting Words or a Protection is used once it rolls
		},
		HeldRollOffer: res.Msg.GetHeldRollOffer(),
	}), nil
}

// useReadiedOtherTx is "Usar" on another standard action: TakeAction's steps, off the readier's turn, charged to the
// reaction. The action is the one readied whatever the options say of the turn's action (it was spent to ready it).
func (s *Service) useReadiedOtherTx(ctx context.Context, c *combatTx, m authz.Membership, winID string) (actionEvent, error) {
	if err := notEnded(c.enc); err != nil {
		return actionEvent{}, err
	}
	w, reactor, err := s.windowToAnswer(ctx, c, m, winID)
	if err != nil {
		return actionEvent{}, err
	}
	use, err := readiedUseOf(ctx, c, w.ID, reactor, readiedOther)
	if err != nil {
		return actionEvent{}, err
	}
	f := newTakeActionFlow(s, c, m, takeActionCall{encounterID: c.enc.ID, combatantID: reactor.ID, actionKey: conv.OrZero(use.row.ActionKey)}, &takeActionOut{})
	if err := f.locate(ctx); err != nil {
		return actionEvent{}, err
	}
	if err := f.chooseReadied(ctx); err != nil {
		return actionEvent{}, err
	}
	for _, step := range []func(context.Context) error{f.begin, f.spend, f.perform} {
		if err := step(ctx); err != nil {
			return actionEvent{}, err
		}
	}
	if err := use.spend(ctx, c, f.who, &f.made); err != nil {
		return actionEvent{}, err
	}
	if err := touch(ctx, c); err != nil {
		return actionEvent{}, err
	}
	c.characterID = &f.who.CharacterID
	return f.made, nil
}

// chooseReadied finds the readied standard action among the readier's, charged to the reaction, and checks the readier
// may react now.
func (f *takeActionFlow) chooseReadied(ctx context.Context) error {
	list := f.opts.GetStandardActions()
	i := slices.IndexFunc(list, func(a *rulesv1.ActionOption) bool { return a.GetAction().GetKey() == f.call.actionKey })
	if i < 0 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("the readied action is not one of the readier's standard actions"))
	}
	f.action, f.economy = list[i], rulesv1.ActionEconomy_ACTION_ECONOMY_REACTION
	return f.s.mustReactNowOrOnTurn(ctx, f.c, f.who)
}

// readiedAttackOf is the READIED window a RollAttack's catch_window_id names, when it is one (ok false for a Deflect
// Missiles throw back): the held attack, by its attacker, with its reaction free.
func (f *attackFlow) readiedAttackOf(ctx context.Context) (bool, error) {
	id, err := uuid.Parse(f.call.catchID)
	if err != nil {
		return true, connect.NewError(connect.CodeInvalidArgument, errors.New("catch_window_id must be a UUID"))
	}
	w, err := f.c.q.GetReactionWindow(ctx, playdb.GetReactionWindowParams{EncounterID: f.c.enc.ID, ID: id.String()})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil // the throw back refuses a window it does not find
	}
	if err != nil {
		return false, fmt.Errorf("find the window of the attack: %w", err)
	}
	if reaction.Kind(w.Kind) != reaction.Readied {
		return false, nil
	}
	use, err := readiedUseOf(ctx, f.c, w.ID, f.attacker, readiedAttack)
	if err != nil {
		return true, err
	}
	if conv.OrZero(use.row.ActionKey) != f.attackKey {
		return true, connect.NewError(connect.CodeInvalidArgument, errors.New("attack_key is not the readied attack"))
	}
	f.readied = use
	return true, nil
}

// mustNotBeHeld refuses an answer to a READIED window whose use is held for other windows: an attack's roll held for the
// windows any attack roll opens (holdAttackRoll), a spell's cast held for a Counterspell (holdCast). The held action is
// the use, once they are answered. It is the refusal of any action while a window waits.
func mustNotBeHeld(ctx context.Context, q *playdb.Queries, encID, windowID string) error {
	held, err := readiedHeld(ctx, q, encID, windowID)
	if err != nil {
		return err
	}
	if held {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_REACTION_PENDING, "the readied action waits for other reactions")
	}
	return nil
}

// readiedHeld says a held action (reaction_holds, still held) is the use of the READIED window: an attack that names it
// in its catch_window_id, or the cast of the readied spell by its readier (heldRelease).
func readiedHeld(ctx context.Context, q *playdb.Queries, encID, windowID string) (bool, error) {
	if held, err := readiedHeldForDie(ctx, q, encID, windowID); held || err != nil {
		return held, err
	}
	return readiedHeldForWindows(ctx, q, encID, windowID)
}

// readiedHeldForWindows says the use of a READIED window waits for other reaction windows (reaction_holds): an
// attack's roll that catches it, or a cast that releases its spell.
func readiedHeldForWindows(ctx context.Context, q *playdb.Queries, encID, windowID string) (bool, error) {
	holds, err := q.ListHeldReactionHolds(ctx, encID)
	if err != nil {
		return false, fmt.Errorf("list the held actions: %w", err)
	}
	for _, h := range holds {
		if h.Kind == "cast" {
			if held, err := heldRelease(ctx, q, encID, windowID, h.Request); held || err != nil {
				return held, err
			}
		}
		if h.Kind != "attack" {
			continue
		}
		var req playv1.RollAttackRequest
		if err := proto.Unmarshal(h.Request, &req); err != nil {
			return false, fmt.Errorf("read a held attack: %w", err)
		}
		if req.GetCatchWindowId() == windowID {
			return true, nil
		}
	}
	return false, nil
}

// readiedMoveOf is the readied move the mover makes: a plain walk off its turn while its READIED window of a move is open. A
// player's own is the release; the master's is a free token move unless the request says it is (release_readied). Any other
// move (on its turn, forced, a jump, a placement, a legendary move) is not one.
func (s *Service) readiedMoveOf(ctx context.Context, a *moveAttempt) error {
	if a.v.master && !a.req.release {
		return nil
	}
	var (
		w   playdb.ReactionWindow
		row playdb.ReadiedAction
		err error
	)
	found := a.req.walking() && !a.req.place && a.req.legendary == "" && !actsNow(a.c.enc, a.target)
	if found {
		if w, row, found, err = openReadied(ctx, a.c.q, a.c.enc.ID, a.target.ID); err != nil {
			return err
		}
	}
	if !found || row.Kind != readiedMove { // another kind of readied action: the move is an ordinary one, and no release
		if a.v.master {
			return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_YOUR_TURN, "the combatant has no readied move to release")
		}
		return nil
	}
	if a.readied, err = readiedUseOf(ctx, a.c, w.ID, a.target, readiedMove); err != nil {
		return err
	}
	return s.mustReactNowOrOnTurn(ctx, a.c, a.target)
}

// openReadied finds the open READIED window of a combatant (it has at most one) and the row it holds.
func openReadied(ctx context.Context, q *playdb.Queries, encID, whoID string) (w playdb.ReactionWindow, row playdb.ReadiedAction, found bool, err error) {
	open, err := q.ListOpenReactionWindows(ctx, encID)
	if err != nil {
		return w, row, false, fmt.Errorf("list the reaction windows: %w", err)
	}
	i := slices.IndexFunc(open, func(w playdb.ReactionWindow) bool {
		return reaction.Kind(w.Kind) == reaction.Readied && conv.OrZero(w.ReactorID) == whoID
	})
	if i < 0 {
		return w, row, false, nil
	}
	if row, err = q.GetReadiedAction(ctx, playdb.GetReadiedActionParams{ID: windowTriggerOf(open[i]).Readied, EncounterID: encID}); err != nil {
		return w, row, false, fmt.Errorf("read the readied action: %w", err)
	}
	return open[i], row, true, nil
}

// openReadiedMove is the readied move of who whose READIED window is open off its turn, if any: GetMoveOptions works it out
// at that move's own budget, as MoveCombatant walks it (readiedMoveOf).
func (s *Service) openReadiedMove(ctx context.Context, d *encounterData, who playdb.Combatant) (*playdb.ReadiedAction, error) {
	if actsNow(d.enc, who) || !slices.ContainsFunc(d.readied, func(r playdb.ReadiedAction) bool { return r.CombatantID == who.ID && r.Kind == readiedMove }) {
		return nil, nil // no readied move waits for it: the query of the open windows is not run
	}
	_, row, found, err := openReadied(ctx, s.queries, d.enc.ID, who.ID)
	if err != nil {
		return nil, s.dbError(ctx, "read the readied move", err)
	}
	if !found || row.Kind != readiedMove {
		return nil, nil
	}
	return &row, nil
}

// spendReadiedMove spends the reaction of a readied move and writes its use. A walk that went nowhere (stopped before its
// first square) is no use: moveInTx stops it before anything is written (idle), so nothing is spent, the row waits and its
// window stays open.
func (s *Service) spendReadiedMove(ctx context.Context, a *moveAttempt) error {
	if a.readied == nil || a.length == 0 {
		return nil
	}
	a.made.ReactionBefore = a.target.ReactionUsed
	if err := spendReactionAs(ctx, a.c, a.target, string(reaction.Readied)); err != nil {
		return err
	}
	return a.readied.spend(ctx, a.c, a.target, &a.made)
}

// readiedMoveDFt is the most a readied move may walk on any surface, in tenths of a foot (readiedSpeedsOf).
func (a *moveAttempt) readiedMoveDFt() int {
	return mostLeftDFt(readiedSpeedsOf(a.target, a.readied.row.MoveFt))
}

// readiedUseBack: the undo of the action a readied action's use ran puts the use back too: the row waits, its window is
// open again, and the reaction a move spent is free (the attack's and the action's undo give theirs back).
func (u *undoer) readiedUseBack(ctx context.Context, kind string, ev actionEvent) error {
	r := ev.Readied
	if r == nil || r.Line == "" || (kind != eventAttackRolled && kind != eventCombatantMoved && kind != eventActionTaken && kind != eventSpellCast) {
		return nil
	}
	if err := u.readiedWaitsAgain(ctx, r); err != nil {
		return err
	}
	if err := u.holdAgain(ctx, r, ev.Actor); err != nil { // a released spell is held again
		return err
	}
	if who, ok := u.find(ev.Actor); ok && kind == eventCombatantMoved {
		return u.setEconomy(ctx, who, who.ActionUsed, who.BonusActionUsed, ev.ReactionBefore, who.Dashed)
	}
	return nil
}

// readiedFiredBack is the undo of a readied_fired event: of the fire, the window it opened goes (unless it was answered:
// the answer's own event comes first, or a use of it is held: REACTION_PENDING) and the row keeps waiting; of "Deixar
// passar", the row waits and its window is open again.
func (u *undoer) readiedFiredBack(ctx context.Context, ev actionEvent) error {
	r := ev.Readied
	switch {
	case r != nil && r.Opened:
		// A use the window holds (a roll waiting for a die or for other windows) names the window: it would be left naming one
		// that is gone, so the undo waits for it.
		if err := mustNotBeHeld(ctx, u.c.q, u.c.enc.ID, r.Window); err != nil {
			return err
		}
		n, err := u.c.q.DeleteUnansweredReadiedWindow(ctx, playdb.DeleteUnansweredReadiedWindowParams{ID: r.Window, EncounterID: u.c.enc.ID})
		if err != nil {
			return fmt.Errorf("take the readied action's window back: %w", err)
		}
		if n == 1 {
			return nil
		}
	case r != nil && r.Ended == readiedIgnored:
		if err := u.readiedWaitsAgain(ctx, r); err != nil {
			return err
		}
		return u.holdAgain(ctx, r, ev.Actor) // a spell let pass is held again
	}
	return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOTHING_TO_UNDO, "this readied action cannot be undone")
}

// readiedWaitsAgain makes the row wait and opens its window again, with its wait clean: the mark of the reactor's player
// and the beginning of the wait the window had before its use are gone, so the next mark starts a new wait.
func (u *undoer) readiedWaitsAgain(ctx context.Context, r *readiedEvent) error {
	if err := u.c.q.ReopenReadiedAction(ctx, r.ID); err != nil {
		return fmt.Errorf("make the readied action wait again: %w", err)
	}
	if err := reopenWindows(ctx, u.c, []string{r.Window}); err != nil {
		return err
	}
	w, err := u.c.q.GetReactionWindow(ctx, playdb.GetReactionWindowParams{EncounterID: u.c.enc.ID, ID: r.Window})
	if err != nil {
		return fmt.Errorf("read the readied window: %w", err)
	}
	t := windowTriggerOf(w)
	if !t.Rolling && t.WaitBegan == nil {
		return nil
	}
	t.Rolling, t.WaitBegan = false, nil
	return setWindowTrigger(ctx, u.c.q, w.ID, t)
}

// chainReadied: a Ready whose row was lost since (its readier was defeated or dismissed: that line never closes the
// chain) cannot be undone: the lost line and the row stay as they are.
func chainReadied(c chainEvent) chainVerdict {
	if c.row.Kind != eventActionReadied {
		return chainGoOn
	}
	if ev, ok := c.payload(); ok && ev.Readied != nil && readiedLostSince(c, ev.Readied.ID) {
		return chainClose
	}
	return chainGoOn
}

// readiedLostSince says a readied_lost line of the row was written after the event the walk looks at.
func readiedLostSince(c chainEvent, id string) bool {
	for _, e := range c.recent {
		if e.ID == c.row.ID {
			return false
		}
		if e.Kind != eventReadiedLost {
			continue
		}
		if ev, err := readEvent(e.Payload); err == nil && ev.Readied != nil && ev.Readied.ID == id {
			return true
		}
	}
	return false
}
