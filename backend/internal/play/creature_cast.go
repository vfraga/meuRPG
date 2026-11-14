package play

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"uuid"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// Casting a summoning spell (MR-037): Convocar Familiar (1 hour, a
// ritual: no slot), Animar Mortos (1 minute) and Conjurar Animais (1 action).
// The long ones are cast outside a combat, here (CastSummon); Conjurar Animais
// is cast in a combat with CastSpell (combat_spells.go), where its creatures
// join the fight with one initiative roll. The rules (what a spell may summon
// with a slot, for which build) are package rules's, reached through
// CombatRoster.CheckSummon; the creatures are stored by package characters.

// summoning is a cast's choice once checked: the creatures to make, with their
// names, and how the group's initiative comes.
type summoning struct {
	creatures []link.CreatureSpec
	in        rollInput
	chk       link.SummonSpell
	slot      int
}

// prepareSummon checks, inside the cast's transaction, what a summoning spell
// cast in a combat needs: the caster is a player's character, the initiative of
// the group is rolled (RN-18: in the app, or typed from a physical die, as the
// campaign's dice setting allows) and the choice is what the spell allows with
// this slot and this character.
func (s *Service) prepareSummon(ctx context.Context, c *combatTx, m authz.Membership, v combatViewer, caster playdb.Combatant, spellKey string, slotLevel int, pick *playv1.SummonChoice, in rollInput, rolled bool, concentration bool, viaGrant bool) (*summoning, error) {
	if err := s.summonRollOK(ctx, c, m, v, caster, in, rolled); err != nil {
		return nil, err
	}
	chk, err := s.checkSummonChoice(ctx, c.tx, m.CampaignID, caster.CharacterID, spellKey, slotLevel, pick, viaGrant)
	if err != nil {
		return nil, err
	}
	if chk.Table && chk.TurnMode == rules.SummonTurnOwnInitiative && len(chk.Creatures) > 1 && !in.inApp {
		return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_SUMMON_NEEDS_INITIATIVE,
			"each creature rolls its own initiative: set roll_in_app, a typed die is one face")
	}
	given, err := summonNames(pick.GetNames(), len(chk.Creatures))
	if err != nil {
		return nil, err
	}
	if err := s.summonHasRoom(ctx, c, m, caster, spellKey, chk, concentration); err != nil {
		return nil, err
	}
	slot := slotLevel
	if chk.Feature {
		slot = 0 // a feature's summon spends no slot: slot() is 0 in what it makes
	}
	return &summoning{creatures: summonSpecs(chk.Creatures, given), in: in, chk: chk, slot: slot}, nil
}

// summonRollOK checks who casts and how the group's initiative comes: a player's
// character, rolled in the app or typed from a physical die, the way the campaign
// lets this caller roll.
func (s *Service) summonRollOK(ctx context.Context, c *combatTx, m authz.Membership, v combatViewer, caster playdb.Combatant, in rollInput, rolled bool) error {
	if caster.Kind != kindPlayer {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("only a player's character summons creatures"))
	}
	if !rolled || in.pool {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_SUMMON_NEEDS_INITIATIVE,
			"set roll_in_app or d20_face: it is the initiative roll of the creatures")
	}
	if v.master {
		return nil
	}
	return s.mustRollThisWay(ctx, c.tx, m, in)
}

// summonHasRoom refuses a cast whose creatures do not fit in the combat (the room check runs before anything
// is spent; the other checks of the cast have their own place). A combat holds 40 combatants, and the creatures the
// cast sends away (the concentration it ends, the creatures of its own spell it
// replaces) leave first.
func (s *Service) summonHasRoom(ctx context.Context, c *combatTx, m authz.Membership, caster playdb.Combatant, spellKey string, chk link.SummonSpell, concentration bool) error {
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return err
	}
	leaving, err := s.replacedLeaving(ctx, c, m, caster, cs, spellKey, chk.ReplacesSame, concentration)
	if err != nil {
		return err
	}
	if len(cs)-leaving+len(chk.Creatures) > maxCombatants {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TOO_MANY_COMBATANTS, "the combat has no room for these creatures")
	}
	return nil
}

// replacedLeaving counts the creatures in the combat that a new cast sends away: those of the caster's
// concentration when the spell concentrates, and those of the same table spell when it replaces its own.
func (s *Service) replacedLeaving(ctx context.Context, c *combatTx, m authz.Membership, caster playdb.Combatant, cs []playdb.Combatant, spellKey string, same, concentration bool) (int, error) {
	if !same && !concentration {
		return 0, nil
	}
	mine, err := s.roster.CharacterCreatures(ctx, c.tx, m.CampaignID, []string{caster.CharacterID})
	if err != nil {
		return 0, err
	}
	leaving := 0
	for _, o := range mine {
		goes := concentration && o.DependsOnConcentration || same && o.Source == summonSourceTable && o.SummonKey == spellKey
		if goes && slices.ContainsFunc(cs, func(x playdb.Combatant) bool { return conv.OrZero(x.CreatureID) == o.ID }) {
			leaving++
		}
	}
	return leaving, nil
}

// joinSummon makes the creatures of a cast and puts them in the combat: they
// appear next to the caster and share one initiative roll, so a group with the
// same total takes a joint turn; the creatures of a table spell that rolls its own
// initiative each roll theirs. The cast's event keeps what an undo needs.
func (s *Service) joinSummon(ctx context.Context, c *combatTx, caster playdb.Combatant, spellKey string, sm *summoning, made *actionEvent) error {
	res, err := s.roster.SummonCreatures(ctx, c.tx, summonOf(c.session.CampaignID, caster.CharacterID, spellKey, made.CastID, sm.slot, sm.chk, sm.creatures))
	if err != nil {
		return err
	}
	for _, cr := range res.Created {
		made.Created, made.MonsterKeys = append(made.Created, cr.ID), append(made.MonsterKeys, cr.MonsterKey)
	}
	gone, err := s.dropReplaced(ctx, c, caster, res.Replaced)
	if err != nil {
		return err
	}
	made.Dismissed = append(made.Dismissed, gone...)
	made.Source, made.OwnerCharacter = summonSourceOf(spellKey, sm.chk), caster.CharacterID
	rolls, roll, err := s.summonRolls(sm.in, res.Created)
	if err != nil {
		return err
	}
	made.SummonRoll = &roll
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return err
	}
	order, _, err := s.joinCreatures(ctx, c, cs, res.Created, rolls)
	if err != nil {
		return err
	}
	if _, err := saveOrder(ctx, c.q, order, orderCombatants(order)); err != nil {
		return err
	}
	return s.creatureEvent(ctx, c, eventCreatureSummoned, caster, actionEvent{
		Round: c.enc.Round, Actor: caster.ID, OwnerCharacter: caster.CharacterID, Created: made.Created, MonsterKeys: made.MonsterKeys,
		Source: made.Source, Key: spellKey, SummonRoll: &roll,
	})
}

// summonRolls rolls the initiative of a casting's creatures: one roll for each group among them (the creatures
// of a casting share one, a creature that rolls its own initiative has a group of its own). The first creature's
// roll is the one the cast's event keeps.
func (s *Service) summonRolls(in rollInput, created []link.Creature) (map[string]groupRoll, groupRoll, error) {
	rolls := make(map[string]groupRoll, len(created))
	var first groupRoll
	for i, cr := range created {
		if _, done := rolls[cr.GroupID]; done {
			continue
		}
		roll, err := s.rollGroup(in, cr.InitiativeBonus)
		if err != nil {
			return nil, first, err
		}
		rolls[cr.GroupID] = roll
		if i == 0 {
			first = roll
		}
	}
	return rolls, first, nil
}

// combatantsOfCreatures are the combatants of the creatures, in the order of ids,
// in the combat: what CastSpell answers with. A creature that is not in it (the
// combat held 40 already) is left out.
func (s *Service) combatantsOfCreatures(ctx context.Context, res combatResult, ids []string) []string {
	if len(ids) == 0 || res.encounterID == "" {
		return nil
	}
	cs, err := s.queries.ListCreatureCombatants(ctx, res.encounterID)
	if err != nil {
		return nil
	}
	var out []string
	for _, id := range ids {
		if i := slices.IndexFunc(cs, func(o playdb.Combatant) bool { return conv.OrZero(o.CreatureID) == id }); i >= 0 {
			out = append(out, cs[i].ID)
		}
	}
	return out
}

// publishCreaturesChanged tells the master and the creatures' owner that their
// lists changed (outside a combat; in one, encounter_changed covers it).
func (s *Service) publishCreaturesChanged(campaignID, ownerUserID string) {
	s.send(campaignID, creaturesChangedEvent(ownerUserID))
}

// creaturesChangedEvent is the hint that the creature lists changed, for the master and the owner's player.
func creaturesChangedEvent(ownerUserID string) live.Event {
	return live.Event{
		Audience: live.Audience{Master: true, UserID: ownerUserID},
		Message: &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_CreaturesChanged_{
			CreaturesChanged: &playv1.WatchGameSessionResponse_CreaturesChanged{},
		}},
	}
}

// publishCreaturesOfCombat tells the owners of the creatures in cs (and the
// master) that their lists changed: a combat ended and wrote the hit points back.
func (s *Service) publishCreaturesOfCombat(campaignID string, cs []playdb.Combatant) {
	var owners []string
	for _, c := range cs {
		if isCreature(c) && c.UserID != nil && !slices.Contains(owners, *c.UserID) {
			owners = append(owners, *c.UserID)
		}
	}
	for _, o := range owners {
		s.publishCreaturesChanged(campaignID, o)
	}
}

// summonKeyMaxLen is the longest spell_key CastSummon takes.
const summonKeyMaxLen = 100

// summonCall is a CastSummon request once its fields are checked.
type summonCall struct {
	key         string
	characterID string
	spellKey    string
	pick        *playv1.SummonChoice
	ritual      bool
	slot        *playv1.SpellSlot
	// grantedBy is the feature that gives the summon ("" for a spell of the character's own): the casting spends
	// a use of the feature's resource.
	grantedBy string
}

// parseSummonCall checks the fields of a CastSummon request, in the order the
// caller learns of them.
func parseSummonCall(msg *playv1.CastSummonRequest) (summonCall, error) {
	key, err := parseKey(msg.GetIdempotencyKey())
	if err != nil {
		return summonCall{}, err
	}
	characterID, ok := uid.Canonical(msg.GetCharacterId())
	if !ok {
		return summonCall{}, errCharacterNotFound()
	}
	spellKey := msg.GetSpellKey()
	if spellKey == "" || len(spellKey) > summonKeyMaxLen {
		return summonCall{}, connect.NewError(connect.CodeInvalidArgument, errors.New("spell_key must name a summoning spell"))
	}
	pick := msg.GetSummon()
	if pick == nil {
		return summonCall{}, connect.NewError(connect.CodeInvalidArgument, errors.New("summon must say what the spell brings"))
	}
	ritual, grantedBy := msg.GetRitual(), msg.GetGrantedBy()
	switch {
	case len(grantedBy) > summonKeyMaxLen:
		return summonCall{}, connect.NewError(connect.CodeInvalidArgument, errors.New("granted_by must name the feature that gives the summon"))
	case grantedBy != "" && (ritual || msg.GetSlot() != nil):
		return summonCall{}, connect.NewError(connect.CodeInvalidArgument, errors.New("a summon given by a feature spends a use of the feature: it is neither a ritual nor cast with a slot"))
	case grantedBy == "" && ritual != (msg.GetSlot() == nil):
		return summonCall{}, connect.NewError(connect.CodeInvalidArgument, errors.New("a ritual is cast with no slot, and any other casting needs one"))
	}
	return summonCall{key: key, characterID: characterID, spellKey: spellKey, pick: pick, ritual: ritual, slot: msg.GetSlot(), grantedBy: grantedBy}, nil
}

// summonRun is what a CastSummon works with inside its transaction, and what the
// answer reads after it.
type summonRun struct {
	call   summonCall
	m      authz.Membership
	v      combatViewer
	made   actionEvent
	vitals *playv1.CharacterVitals
	owner  string // the character's player, for the stream
}

// CastSummon implements playv1connect.PlayServiceHandler.
func (s *Service) CastSummon(
	ctx context.Context,
	req *connect.Request[playv1.CastSummonRequest],
) (*connect.Response[playv1.CastSummonResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	call, err := parseSummonCall(req.Msg)
	if err != nil {
		return nil, err
	}
	r := &summonRun{call: call, m: m, v: viewerOf(m)}
	res, err := s.write(ctx, combatWrite{m: m, key: call.key, hash: idem.Hash(req.Msg), kind: eventCreatureSummoned}, func(c *combatTx) (any, error) {
		return s.castSummonTx(ctx, c, r)
	})
	if err != nil {
		return nil, s.dbError(ctx, "cast a summoning spell", err)
	}
	ev, err := resultEvent(res, r.made)
	if err != nil {
		return nil, s.dbError(ctx, "read the casting", err)
	}
	if !res.repeated {
		s.publishVitals(ctx, m.CampaignID, r.vitals)
		s.publishCreaturesChanged(m.CampaignID, r.owner)
	}
	return connect.NewResponse(&playv1.CastSummonResponse{CreatureIds: ev.Created, DismissedCreatureIds: ev.Dismissed, Vitals: r.vitals}), nil
}

// castSummonTx is the CastSummon transaction: who casts, the slot, the choice,
// then the slot spent and the creatures made.
func (s *Service) castSummonTx(ctx context.Context, c *combatTx, r *summonRun) (any, error) {
	r.vitals, r.owner = nil, ""
	if err := s.summonCaster(ctx, c, r); err != nil {
		return nil, err
	}
	if err := s.summonNotInCombat(ctx, c, r.call.characterID); err != nil {
		return nil, err
	}
	cost, err := s.summonCost(ctx, c, r)
	if err != nil {
		return nil, err
	}
	chk, err := s.checkSummonChoice(ctx, c.tx, r.m.CampaignID, r.call.characterID, r.call.spellKey, cost.circle, r.call.pick, r.call.grantedBy != "")
	if err != nil {
		return nil, err
	}
	if r.call.ritual && (!chk.CanRitual || !chk.Ritual) {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("the character cannot cast this spell as a ritual"))
	}
	given, err := summonNames(r.call.pick.GetNames(), len(chk.Creatures))
	if err != nil {
		return nil, err
	}
	if err := s.summonWrite(ctx, c, r, cost, chk, given); err != nil {
		return nil, err
	}
	return r.made, nil
}

// summonCaster checks that the character is a player's and that the caller may
// cast for it, and notes the player for the stream. A beast form casts no spell
// (MR-037).
func (s *Service) summonCaster(ctx context.Context, c *combatTx, r *summonRun) error {
	characterID := r.call.characterID
	chars, err := s.roster.CombatCharacters(ctx, c.tx, r.m.CampaignID, []string{characterID})
	if err != nil {
		return err
	}
	if len(chars) != 1 || !chars[0].Player {
		return errCharacterNotFound()
	}
	if !r.v.master && chars[0].PlayerUserID != r.m.UserID {
		return connect.NewError(connect.CodePermissionDenied, errors.New("only the character's player or the master may cast for it"))
	}
	r.owner = chars[0].PlayerUserID
	if chars[0].ArmorUntrained {
		return errArmorNotProficient()
	}
	now, err := s.vitals.GetVitalsTx(ctx, c.tx, r.m.CampaignID, characterID)
	if err != nil {
		return err
	}
	return noSpellsIn(now, chars[0].CastsInBeastForm)
}

// summonNotInCombat refuses the cast of a character that is in a combat: there it
// is CastSpell's, with the initiative roll.
func (s *Service) summonNotInCombat(ctx context.Context, c *combatTx, characterID string) error {
	_, cs, found, err := openCombatants(ctx, c.q, c.session.ID)
	if err != nil || !found {
		return err
	}
	if slices.ContainsFunc(cs, func(o playdb.Combatant) bool { return o.Kind == kindPlayer && o.CharacterID == characterID }) {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_SUMMON_IN_COMBAT,
			"the character is in a combat: cast the spell there")
	}
	return nil
}

// summonPrice is what a CastSummon spends: a slot of the circle, or a use of a resource of the feature that gives the
// summon (resource), or nothing for a ritual (circle 0).
type summonPrice struct {
	slot     *slotRef
	circle   int
	resource string
	uses     int32
}

// summonCost is the price of the cast, through the character's options: the spell must be one it has prepared, and a
// slot of that circle free; a feature's summon, one it has with a use of its resource left.
func (s *Service) summonCost(ctx context.Context, c *combatTx, r *summonRun) (summonPrice, error) {
	if r.call.ritual {
		return summonPrice{}, nil
	}
	opts, err := s.roster.CombatTurnOptions(ctx, c.tx, r.m.CampaignID, r.call.characterID, link.Turn{})
	if err != nil {
		return summonPrice{}, err
	}
	cast, err := castableOf(opts, r.call.spellKey, r.call.grantedBy)
	if err != nil {
		return summonPrice{}, err
	}
	// The casting time and the economy do not matter outside a combat.
	if !cast.enabled && cast.reason.GetCode() != rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_CASTING_TIME_TOO_LONG {
		if err := castError(cast.reason, true); err != nil {
			return summonPrice{}, err
		}
	}
	if r.call.grantedBy != "" {
		return grantedSummonPrice(cast)
	}
	slot, err := slotOf(r.call.slot, cast)
	if err != nil {
		return summonPrice{}, err
	}
	if slot == nil {
		return summonPrice{}, connect.NewError(connect.CodeInvalidArgument, errors.New("this spell needs a slot"))
	}
	return summonPrice{slot: slot, circle: int(slot.Level)}, nil
}

// grantedSummonPrice is the price of a feature's summon: one use of the grant's resource, at the spell's own circle.
func grantedSummonPrice(cast castable) (summonPrice, error) {
	g := cast.grant
	if g == nil || g.GetAtWill() || g.GetResource() == "" {
		return summonPrice{}, connect.NewError(connect.CodeInvalidArgument, errors.New("granted_by is not a feature that gives this summon"))
	}
	if g.GetUsesLeft() < max(g.GetUsesCost(), 1) {
		return summonPrice{}, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_USES, "the feature has no use left")
	}
	return summonPrice{circle: int(cast.level), resource: g.GetResource(), uses: max(g.GetUsesCost(), 1)}, nil
}

// summonWrite spends the slot or the use, makes the creatures and writes what the event
// keeps for an undo, and the dismissal of the ones they replace.
func (s *Service) summonWrite(ctx context.Context, c *combatTx, r *summonRun, cost summonPrice, chk link.SummonSpell, given []string) error {
	characterID, spellKey, slot := r.call.characterID, r.call.spellKey, cost.slot
	var err error
	switch {
	case slot != nil:
		r.vitals, err = s.spendSlot(ctx, c, characterID, *slot, 1)
	case cost.resource != "":
		r.vitals, err = s.spendResource(ctx, c, characterID, cost.resource, max(cost.uses, 1))
	}
	if err != nil {
		return grantUseError(err)
	}
	made := 0
	if !chk.Feature {
		made = int(castSlotLevel(slot))
	}
	sum, err := s.roster.SummonCreatures(ctx, c.tx, summonOf(r.m.CampaignID, characterID, spellKey, uuid.New().String(), made, chk, summonSpecs(chk.Creatures, given)))
	if err != nil {
		return err
	}
	r.made = actionEvent{OwnerCharacter: characterID, Source: summonSourceOf(spellKey, chk), Ritual: r.call.ritual, Key: spellKey, Slot: slot, GrantUse: cost.resource, GrantCost: grantCostOf(cost.uses)}
	for _, cr := range sum.Created {
		r.made.Created, r.made.MonsterKeys = append(r.made.Created, cr.ID), append(r.made.MonsterKeys, cr.MonsterKey)
	}
	c.characterID = &characterID
	return s.dismissReplaced(ctx, c, r, sum.Replaced)
}

// dismissReplaced writes the dismissal of each creature a casting replaced, with the reason the roster
// dismissed it for.
func (s *Service) dismissReplaced(ctx context.Context, c *combatTx, r *summonRun, replaced []link.Creature) error {
	for _, cr := range replaced {
		r.made.Dismissed = append(r.made.Dismissed, cr.ID)
		if err := insertEvent(ctx, c, eventCreatureDismissed, &r.m.UserID, nil, actionEvent{OwnerCharacter: r.call.characterID, Created: []string{cr.ID}, Reason: cr.DismissedReason}); err != nil {
			return err
		}
	}
	return nil
}

// EndConcentration implements playv1connect.CombatServiceHandler.
func (s *Service) EndConcentration(
	ctx context.Context,
	req *connect.Request[playv1.EndConcentrationRequest],
) (*connect.Response[playv1.EndConcentrationResponse], error) {
	m, key, encID, err := memberCall(ctx, req.Msg.GetCampaignId(), req.Msg.GetIdempotencyKey(), req.Msg.GetEncounterId())
	if err != nil {
		return nil, err
	}
	combID, err := parseCombatID(req.Msg.GetCombatantId(), "combatant")
	if err != nil {
		return nil, err
	}

	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventConditionsSet, encounterID: encID}, func(c *combatTx) (any, error) {
		return s.endConcentrationTx(ctx, c, m, combID)
	})
	if err != nil {
		return nil, s.dbError(ctx, "end a concentration", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishConcentrationEnded(ctx, m.CampaignID, combID, d)
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.EndConcentrationResponse{Encounter: out}), nil
}

// endConcentrationTx is the EndConcentration transaction: the combatant is found
// as the caller sees it, a creature does not concentrate, and the concentration
// it holds ends, with what it held.
func (s *Service) endConcentrationTx(ctx context.Context, c *combatTx, m authz.Membership, combID string) (any, error) {
	if err := notEnded(c.enc); err != nil {
		return nil, err
	}
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return nil, err
	}
	v := c.viewer(m, cs) // the fog: an NPC the player does not see is not found
	target, err := findCombatant(cs, combID, v)
	if err != nil {
		return nil, err
	}
	if err := v.mayAct(target); err != nil {
		return nil, err
	}
	if isCreature(target) {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("a creature does not concentrate"))
	}
	if target.ConcentrationSpell == nil {
		return nil, nil // nothing to end: no event
	}
	ev := actionEvent{Round: c.enc.Round, Secret: target.Hidden, Actor: target.ID}
	if err := s.stopConcentrating(ctx, c, target, &ev); err != nil {
		return nil, err
	}
	if err := touch(ctx, c); err != nil {
		return nil, err
	}
	c.characterID = &target.CharacterID
	ev.Round = c.enc.Round
	return ev, nil
}

// publishConcentrationEnded tells what an ended concentration changed: the combat,
// its log (unless the combatant is hidden) and the owner of the creatures that
// went with it.
func (s *Service) publishConcentrationEnded(ctx context.Context, campaignID, combID string, d *encounterData) {
	s.publishEncounterChanged(ctx, campaignID, d.enc)
	i := slices.IndexFunc(d.cs, func(c playdb.Combatant) bool { return c.ID == combID })
	s.publishLogChanged(ctx, campaignID, d.enc.ID, i >= 0 && !d.cs[i].Hidden)
	if i >= 0 && d.cs[i].UserID != nil {
		s.publishCreaturesChanged(campaignID, *d.cs[i].UserID)
	}
}

// stopConcentrating ends the combatant's concentration and the creatures that
// lasted only while it did (RN-22, MR-037), and notes what an undo puts back in
// the event: the concentration, the creatures and the Hunter's Mark.
func (s *Service) stopConcentrating(ctx context.Context, c *combatTx, target playdb.Combatant, ev *actionEvent) error {
	if err := c.q.SetCombatantConcentration(ctx, playdb.SetCombatantConcentrationParams{ID: target.ID}); err != nil {
		return fmt.Errorf("end the concentration: %w", err)
	}
	ev.ConcBefore, ev.ConcEnded = *target.ConcentrationSpell, *target.ConcentrationSpell
	if *target.ConcentrationSpell == huntersMark {
		if err := s.clearHuntersMark(ctx, c, target, ev); err != nil { // the undo puts the mark back (MarksBefore)
			return err
		}
	}
	var err error
	if ev.Dismissed, err = s.endSummons(ctx, c, target); err != nil {
		return err
	}
	// The effects the concentration held end with it (RN-22).
	return s.endConcentrationEffects(ctx, c, target, endConcentration)
}
