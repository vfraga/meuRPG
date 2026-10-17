package play

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// Casting outside a combat (SRD 5.1, "Spellcasting"): rituals, spells that take
// minutes or hours, healing between fights and the spells that last. The rules are
// package rules's (casting time, rituals, durations) and the sheet's reads are
// CombatRoster's (OutsideSpell, CastingOptions); this file records the casts in
// spell_casts and applies what the server knows how to apply. The combat has its own
// CastSpell (combat_spells.go): a character that is in a combat casts there.

// The kinds of session event the casts write (migration 00196). Their payloads hold
// ids and numbers only.
const (
	eventSpellCastOutside     = "spell_cast_outside"
	eventSpellCastStarted     = "spell_cast_started"
	eventSpellCastFinished    = "spell_cast_finished"
	eventSpellCastInterrupted = "spell_cast_interrupted"
	eventSpellCastEnded       = "spell_cast_ended"
)

// The states and endings of a cast, as spell_casts keeps them.
const (
	castCasting = "casting"
	castActive  = "active"
	castEnded   = "ended"
	castFailed  = "failed"

	endInstant       = "instant"
	endDismissed     = "dismissed"
	endConcentration = "concentration"
	endRest          = "rest"
	endInterrupted   = "interrupted"
	endCasterGone    = "caster_gone"
)

// What a cast did to a target, as spell_casts keeps it.
const (
	castHeal      = link.EffectHeal
	castTempHP    = link.EffectTempHP
	castMaxHP     = link.EffectMaxHP
	castArmor     = link.EffectArmorClass
	castNarrated  = link.EffectNarrated
	maxCastsShown = 100 // the log of ListSpellCasts
)

// castTarget is what a cast did to one target. It is stored with the cast.
type castTarget struct {
	ID     string `json:"id"`
	Effect string `json:"effect,omitempty"`
	// Amount is the hit points regained (after Disciple of Life), gained as temporary
	// hit points, or added to the maximum; Before and After are the hit points around it.
	Amount int32 `json:"amount,omitempty"`
	Before int32 `json:"before,omitempty"`
	After  int32 `json:"after,omitempty"`
	// Extra is what Disciple of Life added to the healing of this target.
	Extra int32 `json:"extra,omitempty"`
	// AC is the armor class Mage Armor gave.
	AC int32 `json:"ac,omitempty"`
	// Blessed marks the caster's own regained hit points of Blessed Healer, which are
	// not a target the caster picked.
	Blessed bool `json:"blessed,omitempty"`
}

// castEvent is the payload of the session events the casts write.
type castEvent struct {
	CastID string `json:"cast_id"`
	Caster string `json:"caster"`
	Key    string `json:"key"`
	// Secret says the master alone reads the cast.
	Secret bool     `json:"secret,omitempty"`
	Slot   *slotRef `json:"slot,omitempty"`
	Ritual bool     `json:"ritual,omitempty"`
	// Minutes is how long the casting takes.
	Minutes   int32        `json:"minutes,omitempty"`
	Targets   []castTarget `json:"targets,omitempty"`
	Ended     []string     `json:"ended,omitempty"`
	Created   []string     `json:"created,omitempty"`
	Dismissed []string     `json:"dismissed,omitempty"`
	Reason    string       `json:"reason,omitempty"`
}

func readCastEvent(payload []byte) (castEvent, error) {
	var ev castEvent
	if err := json.Unmarshal(payload, &ev); err != nil {
		return castEvent{}, fmt.Errorf("read the cast event: %w", err)
	}
	return ev, nil
}

// errCasting is CastingService's failed_precondition, with the CastingBlocked detail
// that tells the app why.
func errCasting(reason playv1.CastingBlockedReason, msg string, edit ...func(*playv1.CastingBlocked)) error {
	err := connect.NewError(connect.CodeFailedPrecondition, errors.New(msg))
	blocked := &playv1.CastingBlocked{Reason: reason}
	for _, e := range edit {
		e(blocked)
	}
	if detail, detailErr := connect.NewErrorDetail(blocked); detailErr == nil {
		err.AddDetail(detail)
	}
	return err
}

func errCastNotFound() error {
	return connect.NewError(connect.CodeNotFound, errors.New("cast not found"))
}

func badCast(msg string) error { return connect.NewError(connect.CodeInvalidArgument, errors.New(msg)) }

// castRoll is how the dice of a cast come: in the app, or the sum of physical dice.
func castRoll(inApp *bool, poolSum *int32) (rollInput, bool, error) {
	switch {
	case inApp != nil && *inApp:
		return rollInput{inApp: true}, true, nil
	case inApp != nil:
		return rollInput{}, false, badCast("roll_in_app must be true")
	case poolSum != nil:
		return rollInput{typed: int(*poolSum), pool: true}, true, nil
	}
	return rollInput{}, false, nil
}

// castGuard is who is casting and the characters it is about, read in the
// transaction.
type castGuard struct {
	caster  link.Character
	targets []link.Character
	// stage are the NPCs on the stage now.
	stage map[string]bool
	// secret says a character the players do not see is in the cast.
	secret bool
}

// stageMap is the NPCs on the stage of the session: a player never learns an NPC's
// character ID (RN-20), so what a player names an NPC by is its place on the stage.
type stageMap struct {
	// byChar maps a character ID to its place's ID, and byEntry the other way.
	byChar, byEntry map[string]string
}

// stageMapOf reads the stage of the session.
func stageMapOf(ctx context.Context, q *playdb.Queries, sessionID string) (stageMap, error) {
	rows, err := q.ListStage(ctx, sessionID)
	if err != nil {
		return stageMap{}, fmt.Errorf("list the stage: %w", err)
	}
	out := stageMap{byChar: make(map[string]string, len(rows)), byEntry: make(map[string]string, len(rows))}
	for _, r := range rows {
		out.byChar[r.CharacterID], out.byEntry[r.ID] = r.ID, r.CharacterID
	}
	return out, nil
}

// stageOf reads the NPCs on the stage of the session.
func stageOf(ctx context.Context, q *playdb.Queries, sessionID string) (map[string]bool, error) {
	sm, err := stageMapOf(ctx, q, sessionID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(sm.byChar))
	for id := range sm.byChar {
		out[id] = true
	}
	return out, nil
}

// inCombat says the character is a combatant of the session's combat that is not ended.
func (s *Service) inCombat(ctx context.Context, q *playdb.Queries, sessionID, characterID string) (bool, error) {
	_, cs, found, err := openCombatants(ctx, q, sessionID)
	if err != nil || !found {
		return false, err
	}
	return slices.ContainsFunc(cs, undismissedOf(characterID)), nil
}

// guardCast reads the caster and the targets of a cast and checks who may cast for
// whom and pick whom (RN-10). The caster is a player's character (its player or the
// master casts) or an NPC (the master). A player picks the player characters and the
// NPCs on the stage; the master any living character but the NPCs kept for monsters.
// A character the caller may not pick is not found, as one that does not exist.
func (s *Service) guardCast(ctx context.Context, c *combatTx, m authz.Membership, casterID string, targetIDs []string) (castGuard, error) {
	g := castGuard{}
	sm, err := stageMapOf(ctx, c.q, c.session.ID)
	if err != nil {
		return g, err
	}
	targetIDs, viaStage := namedOnStage(sm, m.Role == authz.RoleMaster, targetIDs)
	ids := append([]string{casterID}, targetIDs...)
	chars, err := s.roster.CombatCharacters(ctx, c.tx, m.CampaignID, ids)
	if err != nil {
		return g, err
	}
	byID := make(map[string]link.Character, len(chars))
	for _, ch := range chars {
		byID[ch.ID] = ch
	}
	g.stage = make(map[string]bool, len(sm.byChar))
	for id := range sm.byChar {
		g.stage[id] = true
	}
	caster, ok := byID[casterID]
	if !ok || caster.CombatOnly {
		return g, errCharacterNotFound()
	}
	if err := mayCastFor(caster, m); err != nil {
		return g, err
	}
	g.caster = caster
	g.secret = !caster.Player && !g.stage[caster.ID]
	err = g.pickTargets(byID, targetIDs, viaStage, m.Role == authz.RoleMaster)
	return g, err
}

// namedOnStage turns what a player named an NPC by, its place on the stage, into the NPC's character
// ID (the character ID of an NPC is the master's, RN-20, and a player who sends it gets what a made-up
// one gets). It returns the IDs and which of them came through the stage. The master names characters.
func namedOnStage(sm stageMap, master bool, targetIDs []string) ([]string, map[string]bool) {
	viaStage := map[string]bool{}
	if master {
		return targetIDs, viaStage
	}
	targetIDs = slices.Clone(targetIDs)
	for i, id := range targetIDs {
		if ch, ok := sm.byEntry[id]; ok {
			targetIDs[i], viaStage[ch] = ch, true
		}
	}
	return targetIDs, viaStage
}

// mayCastFor checks the caller may cast for the character: its player or the master for a player's
// character, the master alone for an NPC (RN-10; a player's NPC is not found).
func mayCastFor(caster link.Character, m authz.Membership) error {
	master := m.Role == authz.RoleMaster
	switch {
	case caster.Player && !master && caster.PlayerUserID != m.UserID:
		return connect.NewError(connect.CodePermissionDenied, errors.New("only the character's player or the master may cast for it"))
	case !caster.Player && !master:
		return errCharacterNotFound() // an NPC is not a player's to cast for (RN-10)
	}
	return nil
}

// pickTargets adds the characters the cast is aimed at to the guard, and says the cast is secret when
// the master aims at an NPC the players do not see.
func (g *castGuard) pickTargets(byID map[string]link.Character, targetIDs []string, viaStage map[string]bool, master bool) error {
	for _, id := range targetIDs {
		t, ok := byID[id]
		if !ok || t.CombatOnly || (!t.Player && !master && !viaStage[t.ID]) {
			return errCharacterNotFound()
		}
		if !t.Player && !g.stage[t.ID] {
			g.secret = true // the master aims at an NPC the players do not see
		}
		g.targets = append(g.targets, t)
	}
	return nil
}

// CastSpellOutsideCombat implements playv1connect.CastingServiceHandler.
func (s *Service) CastSpellOutsideCombat(
	ctx context.Context,
	req *connect.Request[playv1.CastSpellOutsideCombatRequest],
) (*connect.Response[playv1.CastSpellOutsideCombatResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	in, err := readCastRequest(req.Msg)
	if err != nil {
		return nil, err
	}
	var made castOutcome
	res, err := s.write(ctx, combatWrite{
		m: m, key: key, hash: idem.Hash(req.Msg), kind: eventSpellCastOutside, altKind: eventSpellCastStarted,
	}, func(c *combatTx) (any, error) {
		made = castOutcome{}
		out, err := s.castOutside(ctx, c, m, req.Msg, in)
		if err != nil {
			return nil, err
		}
		made = out
		return out.ev, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "cast a spell outside a combat", err)
	}
	return s.castAnswer(ctx, m, res, made, func(cast *playv1.OutsideCast, vitals []*playv1.CharacterVitals, o castEvent) *playv1.CastSpellOutsideCombatResponse {
		return &playv1.CastSpellOutsideCombatResponse{
			Cast: cast, Vitals: vitals, CreatureIds: o.Created, DismissedCreatureIds: o.Dismissed, EndedCastIds: o.Ended,
		}
	})
}

// castOutside is the write of CastSpellOutsideCombat, in the transaction: it checks the cast, records
// it, and either takes effect at once or starts a casting that takes time.
func (s *Service) castOutside(ctx context.Context, c *combatTx, m authz.Membership, msg *playv1.CastSpellOutsideCombatRequest, in castRequest) (castOutcome, error) {
	plan, err := s.planOutside(ctx, c, m, msg, in)
	if err != nil {
		return castOutcome{}, err
	}
	long := plan.minutes > 0
	if plan.row, err = s.insertCast(ctx, c, m, plan, long); err != nil {
		return castOutcome{}, err
	}
	if long {
		return s.startLongCast(ctx, c, plan)
	}
	out, err := s.takeEffect(ctx, c, m, plan)
	if err != nil {
		return castOutcome{}, err
	}
	out.guard = plan.g
	c.characterID = &plan.g.caster.ID
	return out, nil
}

// planOutside reads the caster and the targets, plans the cast against the sheet and checks
// everything the request asks for, before the cast is written.
func (s *Service) planOutside(ctx context.Context, c *combatTx, m authz.Membership, msg *playv1.CastSpellOutsideCombatRequest, in castRequest) (castPlan, error) {
	g, err := s.guardCast(ctx, c, m, in.casterID, in.targetIDs)
	if err != nil {
		return castPlan{}, err
	}
	if err := s.mayCastNow(ctx, c, g.caster); err != nil {
		return castPlan{}, err
	}
	plan, err := s.planCast(ctx, c, m, g, in.spell, in.ritual, msg.GetSlot())
	if err != nil {
		return castPlan{}, err
	}
	plan.in, plan.rolled, plan.pick = in.roll, in.rolled, msg.GetSummon()
	if plan.ability, err = s.abilityChoiceOf(ctx, c, in.spell, msg.GetAbilityKey()); err != nil {
		return castPlan{}, err
	}
	return plan, s.checkOutsideCast(ctx, c, m, plan)
}

// checkOutsideCast checks the targets, the reach and, for a cast that takes effect at once, what
// the spell needs from the request.
func (s *Service) checkOutsideCast(ctx context.Context, c *combatTx, m authz.Membership, plan castPlan) error {
	long := plan.minutes > 0
	if long && plan.pick != nil {
		return badCast("summon goes in FinishCast for a spell that takes time")
	}
	if err := s.checkCastTargets(ctx, c, m, plan); err != nil {
		return err
	}
	if err := s.checkReach(ctx, c, plan, m.Role == authz.RoleMaster); err != nil {
		return err
	}
	if long {
		return nil
	}
	return s.checkCastInput(ctx, c, m, plan)
}

// insertCast writes the cast's row, in the casting state.
func (s *Service) insertCast(ctx context.Context, c *combatTx, m authz.Membership, plan castPlan, long bool) (playdb.SpellCast, error) {
	row, err := c.q.InsertSpellCast(ctx, playdb.InsertSpellCastParams{
		CampaignID: m.CampaignID, GameSessionID: c.session.ID, CasterID: plan.g.caster.ID, SpellKey: plan.spell, Ritual: plan.ritual,
		SlotLevel: castSlotLevel(plan.slot), SlotPact: plan.slot != nil && plan.slot.Pact, Status: castCasting, Concentrating: long,
		CastingMinutes: clamp32(plan.minutes, 0, 1_000_000), Lasts: plan.osp.Lasts,
		DurationSeconds: durationOf(plan.osp), RestEnds: restOf(plan.osp), Secret: plan.g.secret, StartedAt: c.now,
	})
	if err != nil {
		return playdb.SpellCast{}, fmt.Errorf("insert the cast: %w", err)
	}
	return row, nil
}

// startLongCast starts a casting that takes time: it keeps the chosen targets for FinishCast and
// takes the caster's concentration from the start.
func (s *Service) startLongCast(ctx context.Context, c *combatTx, plan castPlan) (castOutcome, error) {
	g, row := plan.g, plan.row
	// The targets chosen now are the ones FinishCast takes effect on.
	chosen := make([]castTarget, 0, len(g.targets))
	for _, t := range g.targets {
		chosen = append(chosen, castTarget{ID: t.ID})
	}
	body, err := json.Marshal(chosen)
	if err != nil {
		return castOutcome{}, fmt.Errorf("encode the cast's targets: %w", err)
	}
	if err := c.q.SetSpellCastTargets(ctx, playdb.SetSpellCastTargetsParams{ID: row.ID, Targets: body}); err != nil {
		return castOutcome{}, fmt.Errorf("keep the cast's targets: %w", err)
	}
	// Casting takes the caster's concentration from the start: it ends any
	// concentration spell (SRD 5.1, "Longer Casting Times").
	ended, dismissed, vitals, err := s.endOtherConcentration(ctx, c, g.caster, row.ID)
	if err != nil {
		return castOutcome{}, err
	}
	made := castOutcome{row: row, ended: ended, dismissed: dismissed, vitals: vitals, guard: g}
	made.ev = castEvent{
		CastID: row.ID, Caster: g.caster.ID, Key: plan.spell, Secret: g.secret, Slot: plan.slot, Ritual: plan.ritual,
		Minutes: clamp32(plan.minutes, 0, 1_000_000), Ended: ended, Dismissed: dismissed,
	}
	c.kind, c.characterID = eventSpellCastStarted, &g.caster.ID
	return made, nil
}

// castSlotLevel is the slot level a cast records: 0 without a slot.
func castSlotLevel(slot *slotRef) int32 {
	if slot == nil {
		return 0
	}
	return slot.Level
}

// durationOf is the timed duration a cast records: nil when the spell has none.
func durationOf(osp link.OutsideSpell) *int32 {
	if osp.DurationSeconds <= 0 {
		return nil
	}
	return new(clamp32(osp.DurationSeconds, 1, 1<<30))
}

// restOf is the rest that ends the spell, nil for none.
func restOf(osp link.OutsideSpell) *string {
	if osp.RestEnds == "" {
		return nil
	}
	return &osp.RestEnds
}

// castOutcome is what a change to a cast leaves for the handler: the cast as stored,
// the characters it changed and who it touched.
type castOutcome struct {
	row                       playdb.SpellCast
	vitals                    []*playv1.CharacterVitals
	ended, created, dismissed []string
	targets                   []castTarget
	guard                     castGuard
	reason                    string
	// ev is the payload of the session event the change wrote.
	ev castEvent
}

// event is the payload of the cast's session event.
func (o castOutcome) event() castEvent {
	return castEvent{
		CastID: o.row.ID, Caster: o.row.CasterID, Key: o.row.SpellKey, Secret: o.row.Secret, Ritual: o.row.Ritual,
		Slot: slotOfRow(o.row), Minutes: o.row.CastingMinutes, Targets: o.targets, Ended: o.ended, Created: o.created,
		Dismissed: o.dismissed, Reason: o.reason,
	}
}

func slotOfRow(r playdb.SpellCast) *slotRef {
	if r.SlotLevel == 0 {
		return nil
	}
	return &slotRef{Level: r.SlotLevel, Pact: r.SlotPact}
}

// mayCastNow checks the caster is not in a combat (it casts there), is not in a beast
// form and has no cast going.
func (s *Service) mayCastNow(ctx context.Context, c *combatTx, caster link.Character) error {
	if caster.Player {
		now, err := s.vitals.GetVitalsTx(ctx, c.tx, c.session.CampaignID, caster.ID)
		if err != nil {
			return err
		}
		if err := noSpellsIn(now, caster.CastsInBeastForm); err != nil { // no spells in a beast form (MR-037)
			return err
		}
	}
	fighting, err := s.inCombat(ctx, c.q, c.session.ID, caster.ID)
	if err != nil {
		return err
	}
	if fighting {
		return errCasting(playv1.CastingBlockedReason_CASTING_BLOCKED_REASON_IN_COMBAT, "the character is in a combat: cast the spell there")
	}
	live, err := c.q.ListLiveSpellCastsOfCaster(ctx, caster.ID)
	if err != nil {
		return fmt.Errorf("list the caster's casts: %w", err)
	}
	if slices.ContainsFunc(live, func(r playdb.SpellCast) bool { return r.Status == castCasting }) {
		return errCasting(playv1.CastingBlockedReason_CASTING_BLOCKED_REASON_CAST_IN_PROGRESS, "the character is already casting a spell: finish it or interrupt it first")
	}
	return nil
}

// castPlan is a cast once checked: who, what, with which slot.
type castPlan struct {
	g       castGuard
	osp     link.OutsideSpell
	spell   string
	ritual  bool
	slot    *slotRef
	minutes int
	in      rollInput
	rolled  bool
	pick    *playv1.SummonChoice
	// ability is the ability an Enhance Ability is cast for (ability_key).
	ability string
	row     playdb.SpellCast
}

// planCast checks the spell and the slot against the caster's sheet: the spell is one
// it can cast, the slot a free one of at least the spell's level, the ritual one the
// caster's class casts as a ritual, and works out how long the casting takes
// (SRD 5.1, "Rituals" and "Casting Time").
func (s *Service) planCast(ctx context.Context, c *combatTx, m authz.Membership, g castGuard, spellKey string, ritual bool, in *playv1.SpellSlot) (castPlan, error) {
	slotLevel := int(in.GetLevel())
	osp, err := s.roster.OutsideSpell(ctx, c.tx, m.CampaignID, g.caster.ID, spellKey, slotLevel)
	if err != nil {
		if connect.CodeOf(err) == connect.CodeNotFound {
			return castPlan{}, badCast("spell_key is not one of the caster's spells")
		}
		return castPlan{}, err
	}
	plan := castPlan{g: g, osp: osp, spell: spellKey, ritual: ritual}
	switch {
	case ritual:
		err = plan.planRitual()
	case !osp.Prepared:
		err = badCast("spell_key is not one of the caster's spells")
	case osp.Spell.Level == 0:
		err = plan.planCantrip(in)
	default:
		err = plan.planSlotted(in)
	}
	return plan, err
}

// planRitual plans a ritual: it spends no slot and cannot be cast at a higher level.
func (p *castPlan) planRitual() error {
	if !p.osp.Ritual {
		return errCasting(playv1.CastingBlockedReason_CASTING_BLOCKED_REASON_NOT_A_RITUAL, "this spell does not have the ritual tag")
	}
	if !p.osp.CanRitual {
		return errCasting(playv1.CastingBlockedReason_CASTING_BLOCKED_REASON_CLASS_CANNOT_RITUAL, "the character cannot cast this spell as a ritual")
	}
	p.minutes = p.osp.RitualMinutes
	return nil
}

// planCantrip plans a cantrip: it is cast with no slot.
func (p *castPlan) planCantrip(in *playv1.SpellSlot) error {
	if in != nil {
		return badCast("a cantrip is cast with no slot")
	}
	p.minutes = p.osp.CastMinutes
	return nil
}

// planSlotted plans a spell cast with a slot: one of at least the spell's level that the caster has free.
func (p *castPlan) planSlotted(in *playv1.SpellSlot) error {
	if in == nil || in.GetLevel() < int32(p.osp.Spell.Level) || in.GetLevel() > 9 { //nolint:gosec // a spell level is 1 to 9
		return badCast("slot must be a spell slot of at least the spell's level")
	}
	i := slices.IndexFunc(p.osp.Slots, func(sl link.CastSlot) bool { return sl.Level == int(in.GetLevel()) && sl.Pact == in.GetPact() })
	if i < 0 {
		return badCast("slot is not one the caster has free")
	}
	p.slot = &slotRef{Level: in.GetLevel(), Pact: in.GetPact()}
	p.minutes = p.osp.CastMinutes
	return nil
}

// checkCastTargets checks how many targets the spell takes. The master is not held to the
// number; a spell that reaches only the caster takes the caster by itself.
func (s *Service) checkCastTargets(_ context.Context, _ *combatTx, m authz.Membership, plan castPlan) error {
	sp := plan.osp.Spell
	level := int(castSlotLevel(plan.slot))
	if level == 0 {
		level = sp.Level
	}
	switch {
	case plan.osp.Effect == link.EffectSummon && !plan.osp.NPC:
		if len(plan.g.targets) > 0 {
			return badCast("a summoning spell takes no targets: the creatures appear next to the caster")
		}
	case selfOnly(sp):
		if slices.ContainsFunc(plan.g.targets, func(t link.Character) bool { return t.ID != plan.g.caster.ID }) {
			return badCast("this spell reaches the caster alone: leave the targets out")
		}
	default:
		if !sp.Area && len(plan.g.targets) == 0 && plan.osp.Effect != link.EffectNarrated {
			return badCast("this spell needs a target")
		}
		if limit := maxTargetsOf(sp, level); limit > 0 && len(plan.g.targets) > limit && m.Role != authz.RoleMaster {
			return badCast(fmt.Sprintf("this spell takes at most %d targets with this slot", limit))
		}
	}
	return nil
}

// checkCastInput checks what a spell that takes effect needs from the request: the
// dice of a spell that rolls them (RN-18), the summoning choice of a summoning spell
// and no armor on a Mage Armor target.
func (s *Service) checkCastInput(ctx context.Context, c *combatTx, m authz.Membership, plan castPlan) error {
	switch plan.osp.Effect {
	case link.EffectHeal, link.EffectTempHP:
		if rollsDice(plan.osp) {
			if err := s.mustRollPool(ctx, c.tx, m, plan.in, plan.rolled); err != nil {
				return err
			}
		}
	case link.EffectArmorClass:
		if err := s.checkNoArmor(ctx, c, m, plan); err != nil {
			return err
		}
	}
	if plan.pick != nil != (plan.osp.Effect == link.EffectSummon && !plan.osp.NPC) {
		return badCast("summon is for a summoning spell, and a summoning spell needs it")
	}
	return nil
}

// checkNoArmor checks no Mage Armor target wears armor.
func (s *Service) checkNoArmor(ctx context.Context, c *combatTx, m authz.Membership, plan castPlan) error {
	for _, t := range s.effectTargets(plan) {
		mage, err := s.roster.MageArmorAC(ctx, c.tx, m.CampaignID, t.ID)
		if err != nil {
			return err
		}
		if mage.Applies && mage.Wears {
			return errCasting(playv1.CastingBlockedReason_CASTING_BLOCKED_REASON_TARGET_WEARS_ARMOR, "Mage Armor asks for a creature that wears no armor")
		}
	}
	return nil
}

// rollsDice says the spell rolls dice the caster must provide: a healing spell with
// dice (Supreme Healing uses the highest number instead of rolling) and a spell that
// gives temporary hit points.
func rollsDice(osp link.OutsideSpell) bool {
	switch osp.Effect {
	case link.EffectHeal:
		return osp.Spell.Heal != nil && osp.Spell.Heal.Count > 0 && !osp.Healing.Supreme
	case link.EffectTempHP:
		return osp.Spell.HP != nil && osp.Spell.HP.Pool.Count > 0
	}
	return false
}

// effectTargets are the characters the server applies the spell's effect to: the
// player characters among the targets (an NPC keeps no hit points or armor class
// outside a combat), and the caster for a spell that reaches only the caster.
func (s *Service) effectTargets(plan castPlan) []link.Character {
	targets := plan.g.targets
	if len(targets) == 0 && selfOnly(plan.osp.Spell) {
		targets = []link.Character{plan.g.caster}
	}
	return slices.DeleteFunc(slices.Clone(targets), func(t link.Character) bool { return !t.Player })
}

// castRequest is what a CastSpellOutsideCombat request says, checked for its shape.
type castRequest struct {
	casterID, spell string
	targetIDs       []string
	ritual, rolled  bool
	roll            rollInput
}

// readCastRequest checks the shape of a request: the ids, the spell, the targets (no more than
// the most a cast takes, none repeated) and the roll.
func readCastRequest(msg *playv1.CastSpellOutsideCombatRequest) (castRequest, error) {
	var in castRequest
	var ok bool
	if in.casterID, ok = parseID(msg.GetCasterCharacterId()); !ok {
		return in, errCharacterNotFound()
	}
	in.spell = msg.GetSpellKey()
	if in.spell == "" || len(in.spell) > 100 {
		return in, badCast("spell_key must name one of the caster's spells")
	}
	if len(msg.GetTargetIds()) > maxSpellTargets {
		return in, badCast(fmt.Sprintf("target_ids must have at most %d entries", maxSpellTargets))
	}
	for i, raw := range msg.GetTargetIds() {
		id, ok := parseID(raw)
		if !ok {
			return in, errCharacterNotFound()
		}
		if slices.Contains(in.targetIDs, id) {
			return in, badCast(fmt.Sprintf("target_ids[%d] repeats a character", i))
		}
		in.targetIDs = append(in.targetIDs, id)
	}
	var inApp *bool
	var typed *int32
	switch roll := msg.GetRoll().(type) {
	case *playv1.CastSpellOutsideCombatRequest_RollInApp:
		inApp = &roll.RollInApp
	case *playv1.CastSpellOutsideCombatRequest_TypedSum:
		typed = &roll.TypedSum
	}
	var err error
	if in.roll, in.rolled, err = castRoll(inApp, typed); err != nil {
		return in, err
	}
	if in.ritual = msg.GetAsRitual(); in.ritual && msg.GetSlot() != nil {
		return in, badCast("a ritual is cast with no slot")
	}
	return in, nil
}
