package play

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"uuid"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/reaction"
)

// A readied spell (SRD 5.1, "Ready"). Only a spell whose casting time is 1 action can be readied. The caster casts it on
// its turn: Ready checks it as CastSpell does (castFlow), spends its slot at once and holds its energy with
// concentration until the trigger, whatever the spell's duration. Nothing of the spell happens yet: no target, no roll,
// no effect, no zone. The release is the cast itself, as a reaction: CastSpell off the caster's turn while its READIED
// window is open (or AnswerReaction "Usar", which builds that request), with the slot already spent and the
// concentration already held. The spell is lost, and its slot stays spent, when the concentration ends before the
// release, when the window is let pass, and whenever the row stops waiting otherwise (the next turn, a defeat, a
// cancel).

// concentrationReadied is the concentration of a combatant holding a readied spell. The held spell is in the row, which
// the master and the readier's player read: every other viewer reads no concentration at all (RN-10).
const concentrationReadied = "readied"

// checkSpell checks the spell a SPELL holds as CastSpell checks a cast on the caster's turn, and reads it at the slot it
// is cast with: the caster may cast now, the spell is one it has, its casting time is 1 action, it may be cast now (a
// slot free, no bonus action spell this turn), the slot fits it, and a verbal spell is not cast in a Silence.
func (f *readyFlow) checkSpell(ctx context.Context) error {
	if f.call.kind != readiedSpell {
		return nil
	}
	t := f.take
	cf := f.heldCast()
	if a := spellAttackOf(t.opts, f.call.spellKey); a != nil {
		return f.checkSpellAttack(ctx, cf, a)
	}
	for _, phase := range []func(context.Context) error{cf.findCaster, cf.pickSpell, cf.checkReadyTime, cf.checkEnabled, f.pickReadySlot(cf), cf.readSpellAtSlot, f.checkReadyReleasable(cf), cf.checkSilence} {
		if err := phase(ctx); err != nil {
			return err
		}
	}
	f.spell, f.name = cf, spellOptionName(t.opts, f.call.spellKey)
	return nil
}

// checkReadyTime refuses a spell whose casting time is not 1 action: a bonus action, a reaction, a minute or more.
func (f *castFlow) checkReadyTime(context.Context) error {
	if f.cast.economy != rulesv1.ActionEconomy_ACTION_ECONOMY_ACTION {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_READY_SPELL_TIME, "only a spell with a casting time of 1 action can be readied")
	}
	return nil
}

// checkReadyReleasable refuses a spell no release of the READIED window can carry, decided by what the spell is: the
// answer carries targets and darts, and CastSpell on the map carries a point, a direction and a wall, but neither has a
// picker for a summon, an ability, a damage type or the dead creature of a revival. Such a spell could only be let pass,
// with its slot spent: it is cast on the caster's turn, not held. A spell placed on the map or laying a zone there is held
// and released with CastSpell; an area in a combat with no map is released by the answer with the creatures it picks.
func (f *readyFlow) checkReadyReleasable(cf *castFlow) func(context.Context) error {
	return func(ctx context.Context) error {
		why, err := cf.unreleasable(ctx)
		if err != nil || why == "" {
			return err
		}
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_READY_SPELL_NOT_RELEASABLE, "this spell asks for "+why+": cast it on your turn, it cannot be readied")
	}
}

// laysZoneOnMap says casting the spell lays a zone on the map: it keeps one and the combat has a map. The answer of a
// window refuses such a spell (it carries no point); CastSpell lays it.
func laysZoneOnMap(sp link.Spell, enc playdb.Encounter) bool {
	_, keepsZone := zoneDefOfSpell(sp)
	return keepsZone && !isTheatre(enc)
}

// unreleasable names what the spell asks that no release can carry, "" when it asks nothing of the kind.
func (f *castFlow) unreleasable(ctx context.Context) (string, error) {
	sp := f.sp
	switch {
	case sp.Summon:
		return "the creatures of a summon", nil
	case sp.DamageChoice != "" && len(sp.DamageTypes) > 0:
		return "a damage type", nil
	}
	asks, err := f.s.spellAsksAbility(ctx, f.c, f.call.spellKey)
	if err != nil {
		return "", err
	}
	if asks {
		return "an ability", nil
	}
	if castsAsRevival(f.call.spellKey) {
		return "the dead creature to revive", nil
	}
	return "", nil
}

// pickReadySlot sets the slot the request's slot_level names on the cast: a free slot of that level, a Warlock's pact
// slot when it has no other; none for a cantrip. readSpellAtSlot, which runs next, checks it as CastSpell's slot.
func (f *readyFlow) pickReadySlot(cf *castFlow) func(context.Context) error {
	return func(context.Context) error {
		level := f.call.slotLevel
		if cf.cast.level == 0 && level == 0 {
			return nil
		}
		slot := &playv1.SpellSlot{Level: level}
		pact := slices.IndexFunc(cf.cast.slots, func(s *rulesv1.SlotChoice) bool { return s.GetLevel() == level && s.GetResource() == "" && s.GetPact() })
		plain := slices.ContainsFunc(cf.cast.slots, func(s *rulesv1.SlotChoice) bool {
			return s.GetLevel() == level && s.GetResource() == "" && !s.GetPact()
		})
		slot.Pact = !plain && pact >= 0
		cf.req.Slot = slot
		return nil
	}
}

// spellOptionName is the Portuguese name of one of the caster's spells.
func spellOptionName(opts *rulesv1.TurnOptions, key string) string {
	for _, sp := range opts.GetSpells() {
		if sp.GetSpell().GetKey() == key {
			return sp.GetSpell().GetNamePt()
		}
	}
	return ""
}

// holdSpell is what a SPELL spends at Ready, besides the action: the slot (as CastSpell spends it), the spell cast this
// turn, and the concentration, which ends the one the caster had with its creatures, its areas and its effects. The
// event keeps what the undo puts back.
func (f *readyFlow) holdSpell(ctx context.Context) error {
	cf := f.spell
	if cf == nil {
		return nil
	}
	cf.out.made = actionEvent{CastID: uuid.New().String(), Slot: cf.slot}
	cf.sp.Concentration = true // a readied spell holds concentration whatever its duration
	for _, phase := range []func(context.Context) error{cf.spendCastSlot, cf.noteSpellCast, func(ctx context.Context) error { return cf.concentrateOn(ctx, concentrationReadied) }, cf.markHunters} {
		if err := phase(ctx); err != nil {
			return err
		}
	}
	held, made := cf.out.made, &f.take.made
	made.Slot, made.SlotCounted, made.CastID = held.Slot, held.SlotCounted, held.CastID
	made.Concentrate, made.ConcBefore, made.ConcEnded, made.Dismissed, made.MarksBefore = true, held.ConcBefore, held.ConcEnded, held.Dismissed, held.MarksBefore
	made.SpellCastBefore, made.BonusSpellBefore = cf.caster.SpellCast, cf.caster.BonusSpellCast
	f.vitals = cf.out.vitals
	return nil
}

// mayCastNow checks the caster may cast now: on its turn, or, for the spell it readied, as the reaction its open
// READIED window offers. Metamagic shapes a cast, and a readied spell was cast at Ready: its release takes none.
func (f *castFlow) mayCastNow(ctx context.Context) error {
	if err := f.findReadied(ctx); err != nil {
		return err
	}
	if f.readied == nil {
		return f.s.mustActNow(ctx, f.c, f.caster)
	}
	if len(f.req.GetMetamagic()) > 0 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("a readied spell was cast at Ready: its release takes no Metamagic"))
	}
	return f.s.mustReactNowOrOnTurn(ctx, f.c, f.caster)
}

// findReadied finds the readied spell the cast releases: the caster's open READIED window holds this spell.
func (f *castFlow) findReadied(ctx context.Context) error {
	w, _, found, err := heldByWindow(ctx, f.c.q, f.c.enc.ID, f.caster.ID, f.call.spellKey)
	if err != nil {
		return err
	}
	if !found {
		if f.call.answering { // the answer's window is closed: the spell is not cast for it
			return errNotYourTurnToAnswer()
		}
		return nil
	}
	if f.req.GetGrantedBy() != "" { // a spell is readied from the caster's classes: a grant's charge is no release's to spend
		return connect.NewError(connect.CodeInvalidArgument, errors.New("a readied spell is released from the source it was readied with: name no grant"))
	}
	f.readied, err = readiedUseOf(ctx, f.c, w.ID, f.caster, readiedSpell)
	return err
}

// heldByWindow finds the readied spell a combatant's open READIED window holds: the window and its row. A held spell attack is
// not one (its release is a roll). It matches the spell's key alone: the caller checks the source (a release takes Ready's).
func heldByWindow(ctx context.Context, q *playdb.Queries, encID, casterID, spellKey string) (playdb.ReactionWindow, playdb.ReadiedAction, bool, error) {
	w, row, found, err := openReadied(ctx, q, encID, casterID)
	return w, row, found && row.Kind == readiedSpell && !heldSpellAttackRow(row) && conv.OrZero(row.ActionKey) == spellKey, err
}

// chosenSlot is the slot of the cast: the request's, or the one a readied spell spent at Ready. A release names no slot or
// the one it was held with: another level is refused.
func (f *castFlow) chosenSlot() (*slotRef, error) {
	if f.readied == nil {
		return slotOf(f.req.GetSlot(), f.cast)
	}
	held := f.readied.row.SlotLevel
	if in := f.req.GetSlot(); in != nil && in.GetLevel() != held {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("a readied spell is released with the slot it was held with (level %d): name no slot or that level", held))
	}
	if held == 0 {
		return nil, nil
	}
	return &slotRef{Level: held}, nil
}

// castID is the id of the cast: a readied spell's is the one it got at Ready.
func (f *castFlow) castID() string {
	if f.readied != nil && f.readied.row.SpellCastID != nil {
		return *f.readied.row.SpellCastID
	}
	return uuid.New().String()
}

// releaseHold ends the concentration a readied spell that needs none held until its release.
func (f *castFlow) releaseHold(ctx context.Context) error {
	if f.readied == nil || f.sp.Concentration {
		return nil
	}
	if err := f.c.q.SetCombatantConcentration(ctx, playdb.SetCombatantConcentrationParams{ID: f.caster.ID}); err != nil {
		return fmt.Errorf("release the readied spell: %w", err)
	}
	return nil
}

// spendReadied writes the use of a readied spell: its window answered, its row fired, the reaction's record and the
// readied_fired line.
func (f *castFlow) spendReadied(ctx context.Context) error {
	if f.readied == nil {
		return nil
	}
	return f.readied.spend(ctx, f.c, f.caster, &f.out.made)
}

// heldRelease says a held cast (reaction_holds, kind "cast") is the release of the readied spell of the READIED window:
// its caster is the window's reactor and its spell the one the window holds.
func heldRelease(ctx context.Context, q *playdb.Queries, encID, windowID string, request []byte) (bool, error) {
	var req playv1.CastSpellRequest
	if err := proto.Unmarshal(request, &req); err != nil {
		return false, fmt.Errorf("read a held cast: %w", err)
	}
	w, err := q.GetReactionWindow(ctx, playdb.GetReactionWindowParams{EncounterID: encID, ID: windowID})
	if err != nil {
		return false, fmt.Errorf("find the readied action's window: %w", err)
	}
	t := windowTriggerOf(w)
	return req.GetCasterId() == conv.OrZero(w.ReactorID) && req.GetSpellKey() == t.Key, nil
}

// useReadiedSpell is "Usar" on a readied spell: CastSpell by the readier, under the answer's key, with the targets the
// answer names in targets (with their darts) or in creature_ids, else the creature named at Ready (for a spell that
// takes named targets), and the answer's die, checked as on the caster's turn. The use is spent once the spell is cast: a cast held by a Counterspell is
// used when it happens.
func (s *Service) useReadiedSpell(ctx context.Context, m authz.Membership, in answerInput, msg *playv1.AnswerReactionRequest, row playdb.ReadiedAction) (*connect.Response[playv1.AnswerReactionResponse], error) {
	req := &playv1.CastSpellRequest{
		CampaignId: msg.GetCampaignId(), EncounterId: in.encID, CasterId: row.CombatantID, SpellKey: conv.OrZero(row.ActionKey),
		IdempotencyKey: in.key, Roll: &playv1.CastSpellRequest_RollInApp{RollInApp: true},
	}
	if len(msg.GetTargets()) > 0 && len(msg.GetCreatureIds()) > 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name the targets of a readied spell in targets or in creature_ids, not both"))
	}
	switch {
	case len(msg.GetTargets()) > 0:
		req.Targets = msg.GetTargets()
	case len(msg.GetCreatureIds()) > 0:
		for _, id := range msg.GetCreatureIds() {
			req.Targets = append(req.Targets, &playv1.SpellTarget{CombatantId: id})
		}
	case row.TargetID != nil:
		var err error
		if req.Targets, err = s.namedAtReady(ctx, msg.GetCampaignId(), in.encID, row); err != nil {
			return nil, err
		}
	}
	switch roll := msg.GetRoll().(type) {
	case *playv1.AnswerReactionRequest_Typed:
		req.Roll = &playv1.CastSpellRequest_D20Face{D20Face: roll.Typed}
	case *playv1.AnswerReactionRequest_PoolSum:
		req.Roll = &playv1.CastSpellRequest_PoolSum{PoolSum: roll.PoolSum}
	}
	if len(msg.GetD20Faces()) > 0 { // the table's dice instead of the app's: the cast reads them as it reads its own
		req.Roll, req.D20Faces = nil, msg.GetD20Faces()
	}
	if len(msg.GetRayRolls()) > 0 { // the table's dice for each ray: the cast checks them as it checks its own
		req.Roll, req.RayRolls = nil, msg.GetRayRolls()
	}
	req.RollMode, req.ModeReason = msg.GetRollMode(), msg.GetModeReason()
	res, err := s.castSpell(ctx, m, connect.NewRequest(req), true)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.AnswerReactionResponse{
		Encounter: res.Msg.GetEncounter(),
		Result: &playv1.ReactionResult{
			Kind: playv1.ReactionKind_REACTION_KIND_READIED, ByMaster: m.Role == authz.RoleMaster,
			Used: res.Msg.GetCast() != nil, // a cast held by a Counterspell is used once it happens
		},
		HeldRollOffer: res.Msg.GetHeldRollOffer(),
		Cast:          res.Msg.GetCast(), // what CastSpell shows this caller
	}), nil
}

// heldSpellView is the concentration of a combatant holding a readied spell as the viewer reads it: the held spell for
// the master and the readier's player (readied is their copy of the row), nothing for anyone else (RN-10), who reads no
// spell, no level and not even that a spell is held.
func heldSpellView(out *playv1.Combatant, readied *playv1.ReadiedAction, c playdb.Combatant) {
	if conv.OrZero(c.ConcentrationSpell) != concentrationReadied {
		return
	}
	out.ConcentrationSpell, out.ConcentrationSpellNamePt = "", ""
	if readied.GetKind() == playv1.ReadiedKind_READIED_KIND_SPELL {
		out.ConcentrationSpell, out.ConcentrationSpellNamePt = readied.GetActionKey(), readied.GetActionNamePt()
	}
}

// concentrationEndedFor is the concentration a line says ended, as the viewer reads it: the one that held a readied
// spell is the master's and the readier's player's (RN-10).
func concentrationEndedFor(key string, v combatViewer, who playdb.Combatant) string {
	if key == concentrationReadied && !v.master && !v.owns(who) {
		return ""
	}
	return key
}

// heldSpellOf is the spell the combatant holds readied when its concentration is the one that holds it, else "".
func heldSpellOf(ctx context.Context, c *combatTx, who playdb.Combatant) (string, error) {
	if conv.OrZero(who.ConcentrationSpell) != concentrationReadied {
		return "", nil
	}
	rows, err := c.q.ListWaitingReadiedActions(ctx, c.enc.ID)
	if err != nil {
		return "", fmt.Errorf("list the readied actions: %w", err)
	}
	for _, row := range rows {
		if row.CombatantID == who.ID && row.Kind == readiedSpell {
			return conv.OrZero(row.ActionKey), nil
		}
	}
	return "", nil
}

// concentrationSpell is the spell a concentration window is for: the readied spell its concentration holds, when it
// holds one.
func (t windowTrigger) concentrationSpell() string {
	if t.HeldSpell != "" {
		return t.HeldSpell
	}
	return t.Spell
}

// readiedGate refuses the fire and the use of a readied action while its readier owes a concentration save (it may
// lose what it holds) or a question about a hidden creature waits for the master (nobody's action goes on).
func (s *Service) readiedGate(ctx context.Context, c *combatTx, who playdb.Combatant) error {
	if err := s.mustNotHold(ctx, c); err != nil {
		return err
	}
	open, err := openWindowsOf(ctx, c)
	if err != nil {
		return err
	}
	if slices.ContainsFunc(open, func(w playdb.ReactionWindow) bool {
		return reaction.Kind(w.Kind) == reaction.Concentration && conv.OrZero(w.ReactorID) == who.ID
	}) {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_CONCENTRATION_SAVE_PENDING, "the readier's concentration save waits for its answer")
	}
	return nil
}

// readiedHeldForDie says the use of a READIED window waits for its roller's answer about a die (a Bardic Inspiration
// die, Lucky, Stroke of Luck: inspiration_holds): a weapon attack that catches the window, or a spell attack that
// releases its spell. The answer is the use.
func readiedHeldForDie(ctx context.Context, q *playdb.Queries, encID, windowID string) (bool, error) {
	holds, err := q.ListOpenInspirationHoldsOfEncounter(ctx, &encID)
	if err != nil {
		return false, fmt.Errorf("list the held rolls: %w", err)
	}
	for _, h := range holds {
		switch heldKind(h.Kind) {
		case heldWeaponAttack:
			var req playv1.RollAttackRequest
			if err := proto.Unmarshal(h.Request, &req); err != nil {
				return false, fmt.Errorf("read a held attack: %w", err)
			}
			if req.GetCatchWindowId() == windowID {
				return true, nil
			}
		case heldSpellAttack:
			if held, err := heldRelease(ctx, q, encID, windowID, h.Request); held || err != nil {
				return held, err
			}
		}
	}
	return false, nil
}

// concentrationSpell is the spell a concentration save was on: the readied spell, when its concentration held one.
func (re *reactionEvent) concentrationSpell() string {
	if re.Held != "" {
		return re.Held
	}
	return re.Spell
}

// heldEntry is the spell a combatant holds readied, as the options list a spell it can cast now: the release is a cast at the
// slot spent at Ready, so the window can pick its targets and split its darts with the rows of the cast screens. A held
// spell attack has none: its release is a roll, not a pick. A combatant has at most one waiting row.
func heldEntry(waiting []playdb.ReadiedAction, who playdb.Combatant) []spellTargetEntry {
	for _, row := range waiting {
		if row.CombatantID == who.ID && row.Kind == readiedSpell && !heldSpellAttackRow(row) {
			return []spellTargetEntry{{key: conv.OrZero(row.ActionKey), level: int(row.SlotLevel), slots: []*rulesv1.SlotChoice{{Level: row.SlotLevel}}}}
		}
	}
	return nil
}

// namedAtReady is the creature named at Ready as the target of a release that names none, for a spell that takes named
// targets. A spell that reaches its caster alone, or an area, names nobody on purpose: the creature said at Ready is not
// put in its place.
func (s *Service) namedAtReady(ctx context.Context, campaignID, encID string, row playdb.ReadiedAction) ([]*playv1.SpellTarget, error) {
	cs, err := s.queries.ListCombatants(ctx, encID)
	if err != nil {
		return nil, s.dbError(ctx, "list the combatants", err)
	}
	i := slices.IndexFunc(cs, func(c playdb.Combatant) bool { return c.ID == row.CombatantID })
	if i < 0 {
		return nil, nil
	}
	sp, err := s.roster.CombatSpell(ctx, nil, campaignID, cs[i].CharacterID, conv.OrZero(row.ActionKey), int(row.SlotLevel), "")
	if err != nil {
		return nil, s.dbError(ctx, "read the spell", err)
	}
	if selfOnly(sp) || sp.Area {
		return nil, nil
	}
	return []*playv1.SpellTarget{{CombatantId: *row.TargetID}}, nil
}
