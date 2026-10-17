package play

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// Casting a summoning spell (MR-037, Etapa 9): Convocar Familiar (1 hour, a
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
}

// prepareSummon checks, inside the cast's transaction, what a summoning spell
// cast in a combat needs: the caster is a player's character, the initiative of
// the group is rolled (RN-18: in the app, or typed from a physical die, as the
// campaign's dice setting allows) and the choice is what the spell allows with
// this slot and this character.
func (s *Service) prepareSummon(ctx context.Context, c *combatTx, m authz.Membership, v combatViewer, caster playdb.Combatant, spellKey string, slotLevel int, pick *playv1.SummonChoice, in rollInput, rolled bool, concentration bool) (*summoning, error) {
	if err := s.summonRollOK(ctx, c, m, v, caster, in, rolled); err != nil {
		return nil, err
	}
	chk, err := s.checkSummonChoice(ctx, c.tx, m.CampaignID, caster.CharacterID, spellKey, slotLevel, pick)
	if err != nil {
		return nil, err
	}
	given, err := summonNames(pick.GetNames(), len(chk.Creatures))
	if err != nil {
		return nil, err
	}
	if err := s.summonHasRoom(ctx, c, m, caster, concentration, len(chk.Creatures)); err != nil {
		return nil, err
	}
	return &summoning{creatures: summonSpecs(chk.Creatures, given), in: in}, nil
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

// summonHasRoom refuses a cast whose creatures do not fit in the combat. It runs
// before anything is spent: a combat holds 40 combatants, and the creatures of the
// concentration this one ends leave first.
func (s *Service) summonHasRoom(ctx context.Context, c *combatTx, m authz.Membership, caster playdb.Combatant, concentration bool, bringing int) error {
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the combatants: %w", err)
	}
	leaving := 0
	if concentration {
		if leaving, err = s.concentrationLeaving(ctx, c, m, caster, cs); err != nil {
			return err
		}
	}
	if len(cs)-leaving+bringing > maxCombatants {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TOO_MANY_COMBATANTS, "the combat has no room for these creatures")
	}
	return nil
}

// concentrationLeaving counts the creatures of the caster's concentration that are
// in the combat: they leave when the new concentration starts.
func (s *Service) concentrationLeaving(ctx context.Context, c *combatTx, m authz.Membership, caster playdb.Combatant, cs []playdb.Combatant) (int, error) {
	old, err := s.roster.ConcentrationCreatures(ctx, c.tx, m.CampaignID, caster.CharacterID)
	if err != nil {
		return 0, err
	}
	leaving := 0
	for _, o := range old {
		if slices.ContainsFunc(cs, func(x playdb.Combatant) bool { return deref(x.CreatureID) == o.ID }) {
			leaving++
		}
	}
	return leaving, nil
}

// joinSummon makes the creatures of a cast and puts them in the combat: they
// appear next to the caster and share one initiative roll, so a group with the
// same total takes a joint turn. The cast's event keeps what an undo needs.
func (s *Service) joinSummon(ctx context.Context, c *combatTx, caster playdb.Combatant, spellKey string, concentration bool, sm *summoning, made *actionEvent) error {
	res, err := s.roster.SummonCreatures(ctx, c.tx, link.Summon{
		CampaignID: c.session.CampaignID, CharacterID: caster.CharacterID, Source: summonSource(spellKey), GroupID: made.CastID,
		Concentration: concentration, Creatures: sm.creatures,
	})
	if err != nil {
		return err
	}
	for _, cr := range res.Created {
		made.Created, made.MonsterKeys = append(made.Created, cr.ID), append(made.MonsterKeys, cr.MonsterKey)
	}
	made.Source, made.OwnerCharacter = summonSource(spellKey), caster.CharacterID
	roll, err := s.rollGroup(sm.in, res.Created[0].InitiativeBonus)
	if err != nil {
		return err
	}
	made.SummonRoll = &roll
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the combatants: %w", err)
	}
	order, _, err := s.joinCreatures(ctx, c, cs, res.Created, map[string]groupRoll{made.CastID: roll})
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
		if i := slices.IndexFunc(cs, func(o playdb.Combatant) bool { return deref(o.CreatureID) == id }); i >= 0 {
			out = append(out, cs[i].ID)
		}
	}
	return out
}

// publishCreaturesChanged tells the master and the creatures' owner that their
// lists changed (outside a combat; in one, encounter_changed covers it).
func (s *Service) publishCreaturesChanged(campaignID, ownerUserID string) {
	s.hub.Publish(campaignID, live.Event{
		Audience: live.Audience{Master: true, UserID: ownerUserID},
		Message: &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_CreaturesChanged_{
			CreaturesChanged: &playv1.WatchGameSessionResponse_CreaturesChanged{},
		}},
	})
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
}

// parseSummonCall checks the fields of a CastSummon request, in the order the
// caller learns of them.
func parseSummonCall(msg *playv1.CastSummonRequest) (summonCall, error) {
	key, err := parseKey(msg.GetIdempotencyKey())
	if err != nil {
		return summonCall{}, err
	}
	characterID, ok := parseID(msg.GetCharacterId())
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
	ritual := msg.GetRitual()
	if ritual != (msg.GetSlot() == nil) {
		return summonCall{}, connect.NewError(connect.CodeInvalidArgument, errors.New("a ritual is cast with no slot, and any other casting needs one"))
	}
	return summonCall{key: key, characterID: characterID, spellKey: spellKey, pick: pick, ritual: ritual, slot: msg.GetSlot()}, nil
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
		s.publishVitals(m.CampaignID, r.vitals)
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
	slot, circle, err := s.summonSlot(ctx, c, r)
	if err != nil {
		return nil, err
	}
	chk, err := s.checkSummonChoice(ctx, c.tx, r.m.CampaignID, r.call.characterID, r.call.spellKey, circle, r.call.pick)
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
	if err := s.summonWrite(ctx, c, r, slot, chk, given); err != nil {
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
	now, err := s.vitals.GetVitalsTx(ctx, c.tx, r.m.CampaignID, characterID)
	if err != nil {
		return err
	}
	return noSpellsIn(now, chars[0].CastsInBeastForm)
}

// summonNotInCombat refuses the cast of a character that is in a combat: there it
// is CastSpell's, with the initiative roll.
func (s *Service) summonNotInCombat(ctx context.Context, c *combatTx, characterID string) error {
	enc, err := c.q.GetOpenEncounter(ctx, c.session.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("find the open encounter: %w", err)
	}
	cs, err := c.q.ListCombatants(ctx, enc.ID)
	if err != nil {
		return fmt.Errorf("list the combatants: %w", err)
	}
	if slices.ContainsFunc(cs, func(o playdb.Combatant) bool { return o.Kind == kindPlayer && o.CharacterID == characterID }) {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_SUMMON_IN_COMBAT,
			"the character is in a combat: cast the spell there")
	}
	return nil
}

// summonSlot is the slot (none for a ritual) and its circle, through the
// character's options: the spell must be one it has prepared, and a slot of that
// circle free.
func (s *Service) summonSlot(ctx context.Context, c *combatTx, r *summonRun) (*slotRef, int, error) {
	if r.call.ritual {
		return nil, 0, nil
	}
	opts, err := s.roster.CombatTurnOptions(ctx, c.tx, r.m.CampaignID, r.call.characterID, link.Turn{})
	if err != nil {
		return nil, 0, err
	}
	cast, err := castableOf(opts, r.call.spellKey)
	if err != nil {
		return nil, 0, err
	}
	// The casting time and the economy do not matter outside a combat.
	if !cast.enabled && cast.reason.GetCode() != rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_CASTING_TIME_TOO_LONG {
		if err := castError(cast.reason, true); err != nil {
			return nil, 0, err
		}
	}
	slot, err := slotOf(r.call.slot, cast)
	if err != nil {
		return nil, 0, err
	}
	if slot == nil {
		return nil, 0, connect.NewError(connect.CodeInvalidArgument, errors.New("this spell needs a slot"))
	}
	return slot, int(slot.Level), nil
}

// summonWrite spends the slot, makes the creatures and writes what the event
// keeps for an undo, and the dismissal of the ones they replace.
func (s *Service) summonWrite(ctx context.Context, c *combatTx, r *summonRun, slot *slotRef, chk link.SummonSpell, given []string) error {
	characterID, spellKey := r.call.characterID, r.call.spellKey
	var err error
	if slot != nil {
		if r.vitals, err = s.spendSlot(ctx, c, characterID, *slot, 1); err != nil {
			return err
		}
	}
	sum, err := s.roster.SummonCreatures(ctx, c.tx, link.Summon{
		CampaignID: r.m.CampaignID, CharacterID: characterID, Source: summonSource(spellKey), GroupID: uuid.New().String(),
		Concentration: chk.Concentration, Creatures: summonSpecs(chk.Creatures, given),
	})
	if err != nil {
		return err
	}
	r.made = actionEvent{OwnerCharacter: characterID, Source: summonSource(spellKey), Ritual: r.call.ritual, Key: spellKey, Slot: slot}
	for _, cr := range sum.Created {
		r.made.Created, r.made.MonsterKeys = append(r.made.Created, cr.ID), append(r.made.MonsterKeys, cr.MonsterKey)
	}
	c.characterID = &characterID
	return s.dismissReplaced(ctx, c, r, sum.Replaced)
}

// dismissReplaced writes the dismissal of each creature a casting replaced: a
// familiar is replaced by the next one, the others end with the concentration.
func (s *Service) dismissReplaced(ctx context.Context, c *combatTx, r *summonRun, replaced []link.Creature) error {
	for _, cr := range replaced {
		r.made.Dismissed = append(r.made.Dismissed, cr.ID)
		reason := "concentration"
		if cr.Source == "familiar" {
			reason = "replaced"
		}
		if err := insertEvent(ctx, c, eventCreatureDismissed, &r.m.UserID, nil, actionEvent{OwnerCharacter: r.call.characterID, Created: []string{cr.ID}, Reason: reason}); err != nil {
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
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	encID, err := parseCombatID(req.Msg.GetEncounterId(), "encounter")
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
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return nil, fmt.Errorf("list the combatants: %w", err)
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
	if err := touchEncounter(ctx, c); err != nil {
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
// the event.
func (s *Service) stopConcentrating(ctx context.Context, c *combatTx, target playdb.Combatant, ev *actionEvent) error {
	if err := c.q.SetCombatantConcentration(ctx, playdb.SetCombatantConcentrationParams{ID: target.ID}); err != nil {
		return fmt.Errorf("end the concentration: %w", err)
	}
	ev.ConcBefore, ev.ConcEnded = *target.ConcentrationSpell, *target.ConcentrationSpell
	if *target.ConcentrationSpell == huntersMark {
		if err := s.clearHuntersMark(ctx, c, target, nil); err != nil { // only a cast's undo puts a mark back (MarksBefore)
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
