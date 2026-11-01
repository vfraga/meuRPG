package play

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/platform/tablerules"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
	"github.com/PuraFome/meuRPG/backend/internal/rules/reaction"
)

// The attack roll (RollAttack), in phases. rollAttack reads the request, runs the transaction as a
// sequence of phases of an attackFlow, each one of the checks or the writes the attack makes, in
// that order, and answers from what the transaction left in an attackOut. The phases keep the order of the
// reads, the dice and the writes, because the scripted dice of the tests and the golden of
// combat_roll_attack_golden_test.go depend on it.

// attackCall is what RollAttack reads from its request, checked.
type attackCall struct {
	key, encounterID, attackerID, targetID string
	// attackKey is the attack the request names; a throw back replaces it with the caught missile.
	attackKey string
	in        rollInput
	// offerID is the opportunity offer the attack answers, catchID the window of a Deflect Missiles throw
	// back (PM-04).
	offerID, catchID string
	// asReaction says the attack is a reaction: an opportunity attack or a throw back (MR-034).
	asReaction bool
	// useExtra says it uses the extra action Haste gives (SRD 5.1, Haste).
	useExtra bool
}

// opportunity says the attack is an opportunity attack: a reaction that is not a throw back.
func (call attackCall) opportunity() bool { return call.asReaction && call.catchID == "" }

// readAttackCall reads the request, in the order the answers are given: the ids, the roll, the offer.
func readAttackCall(req *playv1.RollAttackRequest) (attackCall, error) {
	call, err := readAttackIDs(req)
	if err != nil {
		return call, err
	}
	if call.in, err = readAttackRoll(req); err != nil {
		return call, err
	}
	if raw := req.GetOpportunityOfferId(); raw != "" {
		if call.offerID, err = parseOfferID(raw); err != nil {
			return call, err
		}
	}
	call.catchID = req.GetCatchWindowId()
	call.asReaction = req.GetAsReaction() || call.offerID != "" || call.catchID != ""
	call.useExtra = req.GetUseExtraAction() && !call.asReaction
	return call, nil
}

// readAttackIDs reads the key, the combat, the two combatants and the attack the request names.
func readAttackIDs(req *playv1.RollAttackRequest) (attackCall, error) {
	var call attackCall
	var err error
	if call.key, call.encounterID, err = parseCall(req.GetIdempotencyKey(), req.GetEncounterId()); err != nil {
		return call, err
	}
	if call.attackerID, err = parseCombatID(req.GetAttackerId(), "combatant"); err != nil {
		return call, err
	}
	if call.targetID, err = parseCombatID(req.GetTargetId(), "combatant"); err != nil {
		return call, err
	}
	call.attackKey = req.GetAttackKey()
	if call.attackKey == "" || len(call.attackKey) > 100 {
		return call, connect.NewError(connect.CodeInvalidArgument, errors.New("attack_key must name one of the attacker's attacks"))
	}
	return call, nil
}

// readAttackRoll reads how the d20 comes: rolled in the app, one typed face, or the typed faces of a pair.
func readAttackRoll(req *playv1.RollAttackRequest) (rollInput, error) {
	var in rollInput
	switch roll := req.GetRoll().(type) {
	case *playv1.RollAttackRequest_RollInApp:
		if !roll.RollInApp {
			return in, connect.NewError(connect.CodeInvalidArgument, errors.New("roll_in_app must be true"))
		}
		in.inApp = true
	case *playv1.RollAttackRequest_D20Face:
		if len(req.GetD20Faces()) > 0 {
			return in, connect.NewError(connect.CodeInvalidArgument, errors.New("set d20_face or d20_faces, not both"))
		}
		in.typed = int(roll.D20Face)
		if in.typed < 1 || in.typed > 20 {
			return in, connect.NewError(connect.CodeInvalidArgument, errors.New("d20_face must be 1 to 20"))
		}
	default:
		var err error
		if in.typedFaces, err = typedD20Faces(req.GetD20Faces()); err != nil {
			return in, err
		}
	}
	return in, nil
}

// attackOut is what the transaction of an attack leaves for its answer.
type attackOut struct {
	made actionEvent
	// shown are the circumstances of the mode, as the caller may read them; reason is why the mode is not
	// the suggestion (the caller's own text, or the master's).
	shown  []*playv1.AdvantageSource
	reason string
	// held is the question a roll that waits for the player's answer asks: Lucky, or a Bardic Inspiration die.
	held *heldOffer
	v    combatViewer
}

// RollAttack implements playv1connect.CombatServiceHandler.
func (s *Service) RollAttack(
	ctx context.Context,
	req *connect.Request[playv1.RollAttackRequest],
) (*connect.Response[playv1.RollAttackResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	return s.rollAttack(ctx, m, req)
}

// rollAttack is RollAttack for a member already authorized: the replay of a held action calls it
// with the member who made the request (combat_reaction_hold.go).
func (s *Service) rollAttack(ctx context.Context, m authz.Membership, req *connect.Request[playv1.RollAttackRequest]) (*connect.Response[playv1.RollAttackResponse], error) {
	call, err := readAttackCall(req.Msg)
	if err != nil {
		return nil, err
	}
	out := &attackOut{v: viewerOf(m)}
	res, err := s.write(ctx, combatWrite{m: m, key: call.key, hash: idem.Hash(req.Msg), kind: eventAttackRolled, encounterID: call.encounterID, sameHeld: retryOfHeldAttack(req.Msg)}, func(c *combatTx) (any, error) {
		return newAttackFlow(s, c, m, req.Msg, call, out).run(ctx)
	})
	if err != nil {
		return nil, s.dbError(ctx, "roll an attack", err)
	}
	return s.attackResponse(ctx, m, req.Msg, res, out)
}

// attackFlow is one run of the attack's transaction: what each phase reads, and the next one uses.
type attackFlow struct {
	s    *Service
	c    *combatTx
	m    authz.Membership
	req  *playv1.RollAttackRequest
	call attackCall
	out  *attackOut

	v                combatViewer
	sees             combatViewer // who picks the target: the attacker's player when the master acts for it
	cs               []playdb.Combatant
	attacker, target playdb.Combatant
	offer            playdb.OpportunityOffer
	sheet            link.Sheet
	caught           playdb.ReactionWindow
	attack           link.Attack
	attackKey        string
	bonusKind        combat.BonusKind
	// coverTarget is the target on the square the mode and the cover are read at: where an opportunity
	// attack's mover left the reach.
	coverTarget playdb.Combatant
	cp          coverPair
	spec        rollSpec
	modeIn      attackModeInputs
	truth       modeResult
	rolled      attackRolled
	bonusFace   int
	targetAC    int
	acRead      bool
	result      combat.AttackResult
	extra       []effectDie
}

func newAttackFlow(s *Service, c *combatTx, m authz.Membership, req *playv1.RollAttackRequest, call attackCall, out *attackOut) *attackFlow {
	return &attackFlow{s: s, c: c, m: m, req: req, call: call, out: out, attackKey: call.attackKey}
}

// run is the transaction: the checks, the d20, what it did and what it spent. It answers the event the
// write stores, or nil when the roll waits for the player's answer.
func (f *attackFlow) run(ctx context.Context) (any, error) {
	f.out.held = nil
	if err := f.prepare(ctx); err != nil {
		return nil, err
	}
	if held, err := f.holdForWindow(ctx); err != nil {
		return nil, err
	} else if held {
		return f.out.made, nil // a reaction window holds the roll: nothing is rolled yet
	}
	if err := f.rollD20(ctx); err != nil {
		return nil, err
	}
	if f.rolled.Held != nil {
		f.out.held = f.rolled.Held // nothing is resolved, spent or written until the answer
		return nil, nil
	}
	if err := f.writeAttack(ctx); err != nil {
		return nil, err
	}
	return f.out.made, nil
}

// prepare is the part of the attack that checks and reads: who and what, the turn, the attack, the economy
// and the cover. Nothing is written.
func (f *attackFlow) prepare(ctx context.Context) error {
	for _, phase := range []func(context.Context) error{f.findAttacker, f.findTarget, f.checkOffer, f.checkTiming, f.pickAttack} {
		if err := phase(ctx); err != nil {
			return err
		}
	}
	if err := f.checkAttack(); err != nil {
		return err
	}
	if err := f.checkEconomy(ctx); err != nil {
		return err
	}
	return f.readCover(ctx)
}

// writeAttack is what the settled d20 does: the event, the economy, the damage it opens and the offer it answers.
func (f *attackFlow) writeAttack(ctx context.Context) error {
	for _, phase := range []func(context.Context) error{
		f.resolve, f.record, f.spendEconomy, f.closeThrow, f.openHitDamage, f.answerOffer, f.settle,
	} {
		if err := phase(ctx); err != nil {
			return err
		}
	}
	return nil
}

// findAttacker reads the combat's combatants as the caller sees them (the fog: an NPC the player does not see
// is not found) and the attacker, who the caller must be able to act for.
func (f *attackFlow) findAttacker(ctx context.Context) error {
	cs, err := listCombatants(ctx, f.c)
	if err != nil {
		return err
	}
	f.cs = cs
	f.v = f.c.viewer(f.m, cs)
	f.out.v = f.v
	if f.attacker, err = findCombatant(cs, f.call.attackerID, f.v); err != nil {
		return err
	}
	f.sees = f.c.actorSight(f.v, cs, f.attacker)
	return f.v.mayAct(f.attacker)
}

// findTarget reads the target, who cannot be the attacker.
func (f *attackFlow) findTarget(ctx context.Context) error {
	target, err := findCombatant(f.cs, f.call.targetID, f.sees)
	if err != nil && f.call.offerID != "" && connect.CodeOf(err) == connect.CodeNotFound {
		if moved, ok := f.movedTarget(ctx); ok {
			target, err = moved, nil
		}
	}
	if err != nil {
		return err
	}
	if target.ID == f.attacker.ID {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("an attacker cannot be its own target"))
	}
	f.target = target
	return nil
}

// movedTarget is the mover an opportunity attack is made at, on the square it left: a mover the player saw
// leave is theirs to hit even if it is out of their sight now (an NPC that retreated into the dark).
func (f *attackFlow) movedTarget(ctx context.Context) (playdb.Combatant, bool) {
	o, err := pendingOffer(ctx, f.c, f.call.offerID)
	if err != nil || o.ReactorID != f.attacker.ID || o.MoverID != f.call.targetID {
		return playdb.Combatant{}, false
	}
	i := slices.IndexFunc(f.cs, func(x playdb.Combatant) bool { return x.ID == f.call.targetID })
	if i < 0 || !f.v.seesAtOffer(f.cs[i], o) {
		return playdb.Combatant{}, false
	}
	return f.cs[i], true
}

// checkOffer reads the opportunity offer an attack answers: the offer is the reactor's, on the mover, and
// still waits. Gone or answered is aborted (the screen reads the combat again); the wrong person was told
// apart before (mayAct).
func (f *attackFlow) checkOffer(ctx context.Context) error {
	if f.call.offerID == "" {
		return nil
	}
	offer, err := pendingOffer(ctx, f.c, f.call.offerID)
	if err != nil {
		return err
	}
	f.offer = offer
	if offer.ReactorID != f.attacker.ID || offer.MoverID != f.target.ID {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("an opportunity attack is made by the offer's reactor on its mover"))
	}
	if f.attacker.ReactionUsed {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_REACTION_USED, "the reaction is used")
	}
	return nil
}

// checkTiming checks it is the moment of the attack (the action's turn, or a reaction's), that the target
// stands, the way the caller rolls (RN-18) and that the attacker may attack.
func (f *attackFlow) checkTiming(ctx context.Context) error {
	var err error
	if f.call.asReaction {
		err = f.s.mustReactNow(ctx, f.c, f.attacker, f.call.offerID != "" || f.call.catchID != "")
	} else {
		err = f.s.mustActNow(ctx, f.c, f.attacker)
	}
	if err != nil {
		return err
	}
	if f.target.Defeated {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TARGET_DEFEATED, "the target is defeated")
	}
	if !f.v.master {
		if err := f.s.mustRollThisWay(ctx, f.c.tx, f.m, f.call.in); err != nil {
			return err
		}
	}
	return mayAttack(f.attacker, f.call.asReaction)
}

// pickAttack reads the attacker's sheet and the attack it makes: one of its own, or the missile a Deflect
// Missiles throw back catches. A Cutting Words that took a die off the roll lowers its bonus (PM-04).
func (f *attackFlow) pickAttack(ctx context.Context) error {
	sheet, err := f.s.sheetOf(ctx, f.c.tx, f.m.CampaignID, f.attacker)
	if err != nil {
		return err
	}
	f.sheet = sheet
	if f.call.catchID != "" {
		if err := f.catchMissile(ctx); err != nil {
			return err
		}
	}
	i := slices.IndexFunc(f.sheet.Attacks, func(a link.Attack) bool { return a.Key == f.attackKey })
	if i < 0 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("attack_key is not one of the attacker's attacks"))
	}
	f.attack = f.sheet.Attacks[i]
	if f.c.replay != nil {
		f.attack.ToHit -= int(f.c.replay.reduction)
	}
	return nil
}

// catchMissile adds the caught missile to the attacks: a throw back is made under its own key.
func (f *attackFlow) catchMissile(ctx context.Context) error {
	thrown, w, err := f.s.deflectThrow(ctx, f.c, f.call.catchID, f.attacker, f.cs)
	if err != nil {
		return err
	}
	f.caught = w
	f.sheet.Attacks = append(slices.Clone(f.sheet.Attacks), thrown)
	f.attackKey = thrown.Key
	return nil
}

// checkAttack checks the attack is a roll to hit: not a saving throw (CastSpell), and a melee weapon for an
// opportunity attack, thrown or not (a dagger counts), not a bow or a spell.
func (f *attackFlow) checkAttack() error {
	if f.attack.Save {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("this attack asks for a saving throw, not an attack roll: use CastSpell"))
	}
	if f.attack.Spell && f.sheet.Traits.ArmorUntrained { // no spells in armor without proficiency
		return errArmorNotProficient()
	}
	if f.call.opportunity() && !f.attack.Melee {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("an opportunity attack is a melee attack"))
	}
	return nil
}

// checkEconomy is RN-18, then the economy: a player has one action a turn (the Attack action's attacks, with
// Extra Attack), or the reaction for an opportunity attack, and the reach. The master has the last word, and
// an NPC with several attacks is his to run.
func (f *attackFlow) checkEconomy(ctx context.Context) error {
	f.bonusKind = combat.BonusNone
	if f.call.useExtra {
		if err := f.checkExtraAction(ctx); err != nil {
			return err
		}
	}
	if f.v.master {
		return f.routineLeft()
	}
	if err := f.playerEconomy(ctx); err != nil {
		return err
	}
	return f.checkReach()
}

// routineLeft holds the master to the Multiattack of a creature he runs (SRD 5.1): once the Attack
// action made as many attacks as the longest routine has, one more is refused. A creature with no
// Multiattack (one attack per action), an attack that is not of the Attack action (a reaction, the extra
// action) and a player's character the master acts for stay as they were: the master has the last word.
func (f *attackFlow) routineLeft() error {
	a := f.attacker
	if a.Kind != kindNPC || f.call.asReaction || f.call.useExtra || f.attack.Spell {
		return nil
	}
	if f.sheet.AttacksPerAction > 1 && a.ActionUsed && int(a.AttacksMade) >= f.sheet.AttacksPerAction {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ATTACKS_USED, "the Multiattack has no attacks left this turn")
	}
	return nil
}

// checkExtraAction checks the extra action Velocidade gives takes one weapon attack (SRD 5.1, Haste).
func (f *attackFlow) checkExtraAction(ctx context.Context) error {
	if f.attack.Spell {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_EXTRA_ACTION_UNAVAILABLE, "the extra action takes one weapon attack, not a spell")
	}
	return f.s.mustHaveExtraAction(ctx, f.c, f.attacker, "attack")
}

// playerEconomy is what the attack spends of a player's turn and what it says about the bonus action.
func (f *attackFlow) playerEconomy(ctx context.Context) error {
	switch {
	case f.call.asReaction && f.attacker.ReactionUsed && f.call.catchID == "": // the throw is the reaction already spent
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_REACTION_USED, "the reaction is used")
	case f.call.useExtra:
		return nil // spent below, with the extra action: the action and the attacks of the turn stay
	case !f.call.asReaction:
		frenzy, err := f.s.frenzyReadyNow(ctx, f.c, f.attacker)
		if err != nil {
			return err
		}
		f.bonusKind, err = attackEconomy(f.attacker, f.sheet, f.attack, frenzy)
		return err
	}
	return nil
}

// checkReach is RN-21: the reach is a player's limit on a map. Without one (RN-25) nobody has a square and
// the master judges the reach, so nothing is checked. An opportunity attack comes right before the mover
// leaves the reach, so it asks no reach check (the mover has moved already).
func (f *attackFlow) checkReach() error {
	if isTheatre(f.c.enc) {
		return nil
	}
	if !placed(f.attacker) || !placed(f.target) {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_PLACED, "the attacker and the target must be on the map")
	}
	dist, _ := distanceFt(f.attacker, f.target)
	if reach := attackReach(f.attack, f.call.opportunity()); f.call.offerID == "" && dist > reach {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TARGET_OUT_OF_REACH, "the target is beyond the attack's range",
			func(b *playv1.EncounterBlocked) { b.MissingFt = dist - reach })
	}
	return nil
}

// readCover is D4: the cover the target has against this attacker. Total cover (a wall on the line, or the
// master's mark) makes it untargetable for a player; the master has the last word. An opportunity attack
// comes right before the mover leaves the reach: the cover is measured with it on the square it left the
// reach at, not where it stands now (a door it walked through is not between the two).
func (f *attackFlow) readCover(ctx context.Context) error {
	f.coverTarget = f.target
	if f.call.offerID != "" && f.offer.LeftCol != nil && f.offer.LeftRow != nil { // an offer made by hand has no square
		f.coverTarget.GridCol, f.coverTarget.GridRow = f.offer.LeftCol, f.offer.LeftRow
	}
	terrain, err := f.s.terrainOf(ctx, f.c.tx, f.m.CampaignID, f.c.enc)
	if err != nil {
		return err
	}
	known, err := f.c.sight.knownTerrain(ctx, f.c.tx, f.v)
	if err != nil {
		return err
	}
	// The real cover sets the armor class; the player is told, and refused, only by what they know of the
	// map and see of the creatures (RN-10, MR-036).
	cp, err := f.c.coverOf(ctx, f.v, terrain, known, f.attacker, f.coverTarget, f.cs)
	if err != nil {
		return err
	}
	// A table feat's ranged_attack effect: half and three-quarters cover give nothing against the attacker's
	// ranged attack of its kind (an opportunity attack is melee).
	if !f.call.opportunity() {
		shape := shapeOfAttack(f.attack)
		dist, placedBoth := distanceFt(f.attacker, f.coverTarget)
		cp = cp.ignoring(ignoresCover(f.sheet.Traits, shape, shape.rangedAt(dist, placedBoth && !isTheatre(f.c.enc))))
	}
	f.cp = cp
	if cp.shown.total() && !f.v.master {
		return errCoverTotal()
	}
	return nil
}

// holdForWindow is what comes before a roll can be held or rolled. A Cutting Words can change the roll: it
// waits for the windows, and is rolled when they are answered (PM-04, combat_reaction_hold.go). A roll that
// waits for a Lucky or Bardic Inspiration answer comes first: nothing else rolls or opens a window until it
// is answered, and the answer does not open the windows again (they were answered before the d20 was kept,
// combat_inspiration.go). It says the roll is held by a window.
func (f *attackFlow) holdForWindow(ctx context.Context) (bool, error) {
	f.spec = rollSpec{
		Kind: heldWeaponAttack, Key: f.call.key, Window: f.c.replay != nil, Roller: attackRoller(f.m, f.attacker, f.c.enc.Round),
		Lucky:  f.sheet.Traits.Lucky, // a halfling: a d20 of 1 may be rolled again (combat_lucky.go)
		MayAsk: true,
	}
	if err := f.s.heldBlocked(ctx, combatGate(f.c), f.spec); err != nil {
		return false, err
	}
	if _, answering := heldReplyOf(ctx); answering {
		return false, nil
	}
	ev, ok, err := f.s.holdAttackRoll(ctx, f.c, f.m, f.req, f.cs, f.attacker, f.target, f.attackKey)
	if err != nil || !ok {
		return false, err
	}
	f.out.made = ev
	return true, nil
}

// modeShape is the attack as the mode reads it. An opportunity attack is a melee attack made right before the
// mover leaves reach (SRD 5.1): its mode and distance are read with the target on the square it left the
// reach at, never where the move ended (a prone mover that was adjacent is not "beyond 5 ft"), and a
// thrown-capable melee weapon is never ranged with it.
func (f *attackFlow) modeShape() attackShape {
	shape := shapeOfAttack(f.attack)
	if f.call.opportunity() {
		shape.Thrown, shape.RangeFt, shape.LongRangeFt = false, shape.MeleeReachFt, 0
	}
	return shape
}

// rollD20 works the mode out from the combat, never from what the screen showed, settles what the caller
// asked for and rolls one d20 or two (SRD 5.1, advantage and disadvantage): the gate rolls it, or keeps it for
// the question it asks.
func (f *attackFlow) rollD20(ctx context.Context) error {
	modeIn, err := f.s.readModeInputs(ctx, f.c, f.cs)
	if err != nil {
		return err
	}
	names, err := f.s.namerFor(ctx, f.c.tx, f.m.CampaignID)
	if err != nil {
		return err
	}
	f.modeIn = modeIn
	f.truth = modeIn.facts(f.c.enc, f.c.sight).attackMode(f.attacker, f.coverTarget, f.sheet.Traits, f.modeShape(), actsNow(f.c.enc, f.attacker) && !f.call.asReaction, f.v, names)
	if err := f.applyProtection(); err != nil {
		return err
	}
	f.strokeSpec(ctx)
	f.rolled, err = f.s.rollHeldAttack(ctx, f.c, f.v, f.req, f.spec, f.attacker, f.target, f.attackKey, f.truth.Mode, f.call.in, f.attack.ToHit)
	if err != nil || f.rolled.Held != nil {
		return err
	}
	f.attack.ToHit = f.rolled.ToHit
	f.out.shown, f.out.reason = f.truth.Shown, f.rolled.Choice.Reason
	if f.rolled.Bonus != nil {
		f.bonusFace = int(f.rolled.Bonus.Face) // the die the player added after rolling
	}
	return nil
}

// resolve is what the roll did against the target's armor class (with the +5 of an active Escudo and the
// cover). The class stays on the server (RN-20).
func (f *attackFlow) resolve(ctx context.Context) error {
	if _, err := f.armorClass(ctx); err != nil {
		return err
	}
	// Improved Critical and Superior Critical are for weapon attacks only.
	criticalFrom := f.criticalFrom()
	// Bênção and Perdição (SRD 5.1): a d4 added to or taken from the attack roll.
	extra, extraTotal, err := f.s.rollEffectDice(f.call.in, effectDiceFor(f.modeIn.states, f.attacker.ID, rules.RollAppliesAttack), f.req.GetExtraDieFaces())
	if err != nil {
		return err
	}
	f.extra = extra
	f.result = combat.ResolveAttackFrom(f.attack.ToHit+f.bonusFace+extraTotal, f.targetAC, f.rolled.D20.Face(), criticalFrom)
	f.strokeHit() // Stroke of Luck: the miss the player turned into a hit
	// A hit on a paralyzed or unconscious creature within 5 ft is a critical hit whatever the die says (SRD 5.1,
	// Conditions).
	if f.result.Hit && f.truth.CriticalOnHit {
		f.result.Critical = true
	}
	if err := closeRequestsOf(ctx, f.c, f.attacker.ID); err != nil {
		return err
	}
	return f.keepRage(ctx)
}

// keepRage keeps the rage of a raging barbarian that attacks a hostile creature (SRD 5.1).
func (f *attackFlow) keepRage(ctx context.Context) error {
	a := f.attacker
	if !hasState(f.modeIn.states, a.ID, stateRage) || f.target.Side == a.Side || (a.AttackedHostile && !a.RageEndPending) {
		return nil
	}
	if err := f.c.q.SetCombatantRageFlags(ctx, playdb.SetCombatantRageFlagsParams{ID: a.ID, AttackedHostile: true, TookDamage: a.TookDamage}); err != nil {
		return fmt.Errorf("note the attack on a hostile creature: %w", err)
	}
	return nil
}

// record is the event of the attack, with what it needs to be undone, and what the attack gives away: the
// attacker's position, hit or miss, and the Help aimed at its target (SRD 5.1, "Unseen Attackers and
// Targets", "Help").
func (f *attackFlow) record(ctx context.Context) error {
	var run int32
	if !f.call.asReaction {
		var err error
		if run, err = breakRun(ctx, f.c, f.attacker); err != nil {
			return err
		}
	}
	f.out.made = f.attackEvent(run)
	return f.s.afterAttack(ctx, f.c, f.attacker, f.target, f.cs, &f.out.made)
}

// attackEvent is the event an attack writes, before its hit is opened: the d20, the mode, the armor class it
// was compared with and what the attacker had spent before.
func (f *attackFlow) attackEvent(run int32) actionEvent {
	a, d20, choice, cover := f.attacker, f.rolled.D20, f.rolled.Choice, f.cp.real
	made := actionEvent{
		RunBefore: run, Round: f.c.enc.Round, Secret: secretOf(a, f.target), Actor: a.ID, Target: f.target.ID, Key: f.attackKey,
		Extra: f.extra, ExtraUsed: f.call.useExtra,
		D20: clamp32(d20.Face(), 1, 20), Modifier: conv.Int32(f.attack.ToHit + f.bonusFace), Total: conv.Int32(f.result.Total),
		Physical: d20.Physical, Outcome: outcomeMiss, AsReaction: f.call.asReaction, CriticalMaxRule: f.c.rules.CriticalMaxPlusRoll,
		D20B: clamp32(d20.otherFace(), 0, 20), Counted: clamp32(d20.Index, 0, 1), RollMode: modeKey(choice.Mode), SuggestedMode: modeKey(choice.Suggested),
		ReasonID: choice.ReasonID, RequestID: choice.RequestID, CritOnHit: f.truth.CriticalOnHit && f.result.Hit,
		ByMaster:     choice.ByMaster && choice.changed(),
		ActionBefore: a.ActionUsed, BonusBefore: a.BonusActionUsed, ReactionBefore: a.ReactionUsed, AttacksBefore: a.AttacksMade,
		AsBonus: f.bonusKind != combat.BonusNone, AttackKeyBefore: conv.OrZero(a.ActionAttackKey), FlurryBefore: a.BonusAttacksLeft,
		// The armor class the roll was compared with (an active Escudo's +5 included): the master's answer
		// shows it, a player's never does.
		TargetAC: clamp32(f.targetAC, 0, math.MaxInt32),
		Cover:    cover.key(), CoverSource: cover.sourceKey(), CoverBonus: clamp32(cover.bonus(), 0, 5),
		CoverRestricted: f.cp.restricted, CoverSeenBy: f.cp.seenBy,
	}
	if b := f.rolled.Bonus; b != nil {
		made.Res = &resourceEvent{Kind: resBardicUse, Sides: b.Sides, Face: b.Face, FromID: b.From, ExpiresRound: b.ExpiresRound}
	}
	made.Lucky = f.rolled.Lucky   // the d20 Lucky rolled again: the log says "Sortudo: 1 → 14"
	made.Stroke = f.rolled.Stroke // the use of Stroke of Luck: the log says the miss became a hit, the undo gives it back
	made.ProtectedBy = f.protectedBy()
	return made
}

// spendEconomy spends what the attack costs: the reaction of an opportunity attack, the extra action, a
// strike of Flurry of Blows, the bonus action or the action of the turn.
func (f *attackFlow) spendEconomy(ctx context.Context) error {
	after := f.attacker
	if err := f.spendTurn(ctx, &after); err != nil {
		return err
	}
	if err := f.c.q.SetCombatantEconomy(ctx, playdb.SetCombatantEconomyParams{
		ID: f.attacker.ID, ActionUsed: after.ActionUsed, BonusActionUsed: after.BonusActionUsed, ReactionUsed: after.ReactionUsed, Dashed: after.Dashed,
	}); err != nil {
		return fmt.Errorf("spend the action: %w", err)
	}
	return nil
}

// spendTurn marks in after what the attack spends and writes the counters that are not the economy's own.
func (f *attackFlow) spendTurn(ctx context.Context, after *playdb.Combatant) error {
	a := f.attacker
	switch {
	case f.call.asReaction:
		after.ReactionUsed = true
	case f.call.useExtra:
		if err := f.c.q.SetCombatantExtraActionUsed(ctx, playdb.SetCombatantExtraActionUsedParams{ID: a.ID, ExtraActionUsed: true}); err != nil {
			return fmt.Errorf("spend the extra action: %w", err)
		}
	case f.bonusKind == combat.BonusFlurry:
		// The bonus action went to Flurry of Blows: one of its strikes.
		if err := f.c.q.SetCombatantAttackState(ctx, playdb.SetCombatantAttackStateParams{ID: a.ID, ActionAttackKey: a.ActionAttackKey, BonusAttacksLeft: a.BonusAttacksLeft - 1}); err != nil {
			return fmt.Errorf("count the flurry strike: %w", err)
		}
	case f.bonusKind != combat.BonusNone:
		after.BonusActionUsed = true
	default:
		after.ActionUsed = true
		return f.countAttack(ctx)
	}
	return nil
}

// countAttack counts the attack of the Attack action. A cantrip is no attack of the Attack action, but its
// beams are counted (Eldritch Blast), so a cast makes no more than its beams.
func (f *attackFlow) countAttack(ctx context.Context) error {
	a := f.attacker
	if !f.attack.Spell || f.attack.Beams > 1 {
		if err := f.c.q.SetCombatantAttacksMade(ctx, playdb.SetCombatantAttacksMadeParams{ID: a.ID, AttacksMade: a.AttacksMade + 1}); err != nil {
			return fmt.Errorf("count the attack: %w", err)
		}
	}
	if err := f.c.q.SetCombatantAttackState(ctx, playdb.SetCombatantAttackStateParams{ID: a.ID, ActionAttackKey: &f.attack.Key, BonusAttacksLeft: a.BonusAttacksLeft}); err != nil {
		return fmt.Errorf("remember the attack: %w", err)
	}
	return nil
}

// closeThrow spends the ki and closes the window of a Deflect Missiles throw back.
func (f *attackFlow) closeThrow(ctx context.Context) error {
	if f.call.catchID == "" {
		return nil
	}
	if _, err := f.s.spendResourceOf(ctx, f.c, f.attacker, resKi); err != nil {
		return err
	}
	ct := windowTriggerOf(f.caught)
	_, err := f.s.closeWindow(ctx, f.c, f.caught, windowAnswered, reaction.ReasonNone, windowOutcome{ByMaster: f.c.master, Used: true, Reduction: ct.Reduced, Before: ct.Damage, After: 0}, "")
	return err
}

// openHitDamage opens the damage of a hit: the dice of the weapon, what a critical hit adds, the parts of the
// damage and the riders the hit offers.
func (f *attackFlow) openHitDamage(ctx context.Context) error {
	if !f.result.Hit {
		return nil
	}
	f.out.made.Outcome = outcomeHit
	if f.result.Critical {
		f.out.made.Outcome = outcomeCrit
	}
	bonus := f.damageBonus()
	savage := savageAttacksDice(f.attacker, f.attack, f.sheet, f.result.Critical)
	p, err := f.s.openHit(ctx, f.c, f.attacker, f.target, f.attackKey,
		link.Dice{Count: f.attack.DiceCount, Sides: f.attack.DiceSides, Bonus: bonus, DamageType: f.attack.DamageType}, f.result.Critical,
		brutalCriticalDice(f.attacker, f.attack, f.sheet, f.result.Critical)+savage, savage, f.result.Total, f.targetAC)
	if err != nil {
		return err
	}
	if err := f.writeHitParts(ctx, bonus, p.ID); err != nil {
		return err
	}
	f.out.made.Pending = p.ID
	if err := f.s.offerRiders(ctx, f.c, f.attacker, f.target, f.attack, f.sheet, f.bonusKind == combat.BonusFlurry); err != nil {
		return err
	}
	return markMeleeHit(ctx, f.c, f.attacker, f.attack, &f.out.made)
}

// damageBonus is the bonus of the weapon's damage. The off-hand attack adds no ability modifier to its damage
// (unless it is negative, or the character fights with two weapons).
func (f *attackFlow) damageBonus() int {
	if f.bonusKind == combat.BonusTwoWeapon {
		return combat.OffHandBonus(f.attack.DiceBonus, f.attack.AbilityMod, f.sheet.TwoWeaponFighting)
	}
	return f.attack.DiceBonus
}

// writeHitParts writes the parts of the damage of a hit.
func (f *attackFlow) writeHitParts(ctx context.Context, bonus int, pendingID string) error {
	disadvantage := slices.ContainsFunc(f.truth.Sources, func(s combat.Source) bool { return s.Effect == combat.ModeDisadvantage })
	return f.s.hitParts(ctx, f.c, f.m.CampaignID, f.v, f.modeIn, f.attacker, f.coverTarget, f.sheet, f.attack, bonus, f.rolled.Choice.Mode, disadvantage, f.call.opportunity(), pendingID)
}

// answerOffer answers the opportunity offer the attack was made on, with the damage it opened (the 0 hit
// points rule finds the offer by it).
func (f *attackFlow) answerOffer(ctx context.Context) error {
	if f.call.offerID == "" {
		return nil
	}
	var damage *string
	if f.out.made.Pending != "" {
		damage = &f.out.made.Pending
	}
	if _, err := f.c.q.SetOpportunityOfferState(ctx, playdb.SetOpportunityOfferStateParams{ID: f.offer.ID, State: offerAttacked, AttackPendingID: damage, AnsweredAt: &f.c.now}); err != nil {
		return fmt.Errorf("answer the opportunity offer: %w", err)
	}
	f.out.made.OfferID = f.offer.ID
	if f.offer.LeftCol != nil && f.offer.LeftRow != nil {
		f.out.made.Col, f.out.made.Row = *f.offer.LeftCol, *f.offer.LeftRow // where the mover was when it was attacked
	}
	return nil
}

// settle moves the combat's revision and says whose character acted.
func (f *attackFlow) settle(ctx context.Context) error {
	if err := touch(ctx, f.c); err != nil {
		return err
	}
	f.c.characterID = &f.attacker.CharacterID
	return nil
}

// attackResponse answers from what the transaction left: the question of a roll that waits, the fact that a
// reaction window holds the roll, or the attack with its pending damage.
func (s *Service) attackResponse(ctx context.Context, m authz.Membership, req *playv1.RollAttackRequest, res combatResult, out *attackOut) (*connect.Response[playv1.RollAttackResponse], error) {
	v, err := s.viewerAfter(ctx, m, res, out.v) // a replay never ran the closure: the fog filter still holds
	if err != nil {
		return nil, s.dbError(ctx, "work out what the player sees", err)
	}
	if out.held != nil {
		return s.heldAttackResponse(ctx, m, res, out.held, req)
	}
	ev, err := resultEvent(res, out.made)
	if err != nil {
		return nil, s.dbError(ctx, "read the attack", err)
	}
	enc, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !ev.Secret)
	})
	if err != nil {
		return nil, err
	}
	if ev.Reaction != nil && ev.Reaction.Hold { // a reaction window holds the roll: nothing was rolled yet
		return connect.NewResponse(&playv1.RollAttackResponse{Encounter: enc, Roll: &playv1.AttackRoll{
			AttackerId: ev.Actor, TargetId: ev.Target, AttackKey: ev.Key, HeldForReaction: true,
		}}), nil
	}
	pending, err := s.pendingFor(ctx, res, ev.Pending, v)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.RollAttackResponse{Encounter: enc, PendingDamage: pending, Roll: s.attackRollOf(ctx, m, ev, v, out)}), nil
}

// attackRollOf is the roll of an attack as the caller may read it: the d20, the outcome, the mode and why,
// the cover and, for the master only, the armor class it was compared with.
func (s *Service) attackRollOf(ctx context.Context, m authz.Membership, ev actionEvent, v combatViewer, out *attackOut) *playv1.AttackRoll {
	roll := &playv1.AttackRoll{
		AttackerId: ev.Actor, TargetId: ev.Target, AttackKey: ev.Key,
		D20: d20Proto(ev), Outcome: outcomeToProto[ev.Outcome],
		CriticalRule: criticalRuleProto(tablerules.Rules{CriticalMaxPlusRoll: ev.CriticalMaxRule}),
		Mode:         modeToProto[modeOfKey(ev.RollMode)], SuggestedMode: modeToProto[modeOfKey(ev.SuggestedMode)], CriticalOnHit: ev.CritOnHit,
		BonusDice: bonusDiceOf(ev),
	}
	roll.Sources, roll.ModeReason = out.shown, out.reason
	roll.D20.ExtraDice = extraDiceProto(ev.Extra, v.master, s.namesFor(ctx, m.CampaignID))
	coverKey, coverSource := ev.coverFor(v)
	roll.Cover, roll.CoverSource = coverDegreeProto(coverKey), coverSourceProto(coverSource)
	if v.master {
		roll.TargetArmorClass = new(ev.TargetAC) // "Acertou contra CA 18": never a player's
	}
	return roll
}
