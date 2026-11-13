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
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/platform/names"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The character's creatures in a combat (MR-037). A creature is a
// combatant of kind 'creature': its character_id is its owner's, its user_id
// the owner's player (so the same "may this player act for it" check works),
// and its hit points are on the combatant, like an NPC's. It acts on the
// party's side, never makes death saves (at 0 it is defeated and dismissed), and
// has a turn and an economy of its own, worked out from its stat block.
//
// The creatures themselves live in package characters (table
// character_creatures), reached through CombatRoster. This file has what the
// combat does with them: which sheet a combatant is read from, joining a combat
// (at its start with the owner, or when Conjurar Animais is cast in it), the
// initiative a group of them shares, keeping their hit points and their
// dismissal in step with the combat, and concentration ending. The casting
// outside a combat is in creature_cast.go.

// sheetKey is what a combatant's sheet is looked up by: its character, or, for
// a creature (whose character_id is its owner's), the combatant itself.
func sheetKey(c playdb.Combatant) string {
	if isCreature(c) {
		return "creature:" + c.ID
	}
	return c.CharacterID
}

// sheetOf reads the sheet a combat works a combatant from: its character's, or
// a creature's stat block with what its spell lets it attack with.
func (s *Service) sheetOf(ctx context.Context, tx pgx.Tx, campaignID string, c playdb.Combatant) (link.Sheet, error) {
	if !isCreature(c) {
		sheet, err := s.roster.CombatSheet(ctx, tx, campaignID, c.CharacterID)
		// Mage Armor, cast outside the combat, gives the better of the two armor classes
		// (casting_combat.go).
		if err == nil && c.MageArmorAc != nil && int(*c.MageArmorAc) > sheet.ArmorClass {
			sheet.ArmorClass = int(*c.MageArmorAc)
		}
		return sheet, err
	}
	sheet, ok, err := s.roster.CreatureSheet(ctx, tx, campaignID, conv.OrZero(c.CreatureID), conv.OrZero(c.MonsterKey), conv.OrZero(c.SummonAttack))
	if err != nil {
		return link.Sheet{}, err
	}
	if !ok {
		return link.Sheet{}, errCombatantNotFound()
	}
	return sheet, nil
}

// optionsOf works out what a combatant can do now, from its sheet or its stat
// block and what it used this turn.
func (s *Service) optionsOf(ctx context.Context, tx pgx.Tx, campaignID string, c playdb.Combatant) (*rulesv1.TurnOptions, error) {
	var opts *rulesv1.TurnOptions
	if !isCreature(c) {
		turn := turnOf(c)
		var err error
		if turn.FrenzyReady, err = s.frenzyReadyIn(ctx, tx, c); err != nil {
			return nil, err
		}
		if opts, err = s.roster.CombatTurnOptions(ctx, tx, campaignID, c.CharacterID, turn); err != nil {
			return nil, err
		}
	} else {
		var ok bool
		var err error
		opts, ok, err = s.roster.CreatureTurnOptions(ctx, tx, campaignID, conv.OrZero(c.CreatureID), conv.OrZero(c.MonsterKey), conv.OrZero(c.SummonAttack), turnOf(c))
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, errCombatantNotFound()
		}
	}
	// The movement left is the combatant's own (speedDFt: the conditions that
	// leave no speed, the Dash), the same number its view shows.
	if opts.GetEconomy() != nil {
		opts.Economy.Movement = movementLeftOf(c)
	}
	return opts, nil
}

// movementLeftOf is the combatant's speed, movement used and movement left this
// turn, in tenths of a foot and in feet rounded down (RN-21).
func movementLeftOf(c playdb.Combatant) *rulesv1.MovementLeft {
	speed, left := speedDFt(c), movementLeftDFt(c)
	return &rulesv1.MovementLeft{
		SpeedFt: clamp32(speed/10, 0, math.MaxInt32), UsedFt: clamp32(int(c.MovementUsedDft)/10, 0, math.MaxInt32), LeftFt: clamp32(left/10, 0, math.MaxInt32),
		SpeedDft: clamp32(speed, 0, math.MaxInt32), UsedDft: clamp32(int(c.MovementUsedDft), 0, math.MaxInt32), LeftDft: clamp32(left, 0, math.MaxInt32),
	}
}

// saveOf is a combatant's saving throw bonus against an ability.
func (s *Service) saveOf(ctx context.Context, tx pgx.Tx, campaignID string, c playdb.Combatant, ability string) (link.Save, error) {
	if !isCreature(c) {
		return s.roster.CombatSave(ctx, tx, campaignID, c.CharacterID, ability)
	}
	return s.roster.CreatureSave(ctx, tx, campaignID, conv.OrZero(c.CreatureID), conv.OrZero(c.MonsterKey), ability)
}

// mayAttack checks what the spell that brought a creature lets it do: a
// familiar never takes the Attack action (and has no attacks), the familiar of a
// warlock with the Pact of the Chain attacks only with its reaction. The master
// has no say here: it is the spell's rule, not the economy's.
func mayAttack(c playdb.Combatant, asReaction bool) error {
	if !isCreature(c) {
		return nil
	}
	switch conv.OrZero(c.SummonAttack) {
	case rules.SummonAttackNone:
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_CREATURE_CANNOT_ATTACK, "this creature cannot attack")
	case rules.SummonAttackReaction:
		if !asReaction {
			return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_CREATURE_CANNOT_ATTACK, "this creature attacks only with its reaction")
		}
	}
	return nil
}

// groupRoll is the initiative a group of creatures rolled once (RN-18): the
// total, the d20 and the bonus every member carries.
type groupRoll struct {
	Total int32 `json:"total"`
	Face  int32 `json:"face"`
	Bonus int32 `json:"bonus"`
}

// concentrationSpellOf is the spell a creature's source depends on for
// concentration: Conjurar Animais, and the table spell (summonKey) that made a creature of the source "table".
func concentrationSpellOf(source, summonKey string) string {
	switch source {
	case "conjure_animals":
		return "spell:conjure-animals"
	case summonSourceTable:
		return summonKey
	}
	return ""
}

// squareNearRings is how many squares around its owner a creature may appear.
const squareNearRings = 4

// squareNear finds the nearest free square to (col, row), within four squares,
// where a creature appears next to its owner; false when there is none.
func squareNear(e playdb.Encounter, all []playdb.Combatant, col, row int32) (int32, int32, bool) {
	taken := func(c, r int32) bool {
		return slices.ContainsFunc(all, func(o playdb.Combatant) bool {
			return !o.Defeated && placed(o) && *o.GridCol == c && *o.GridRow == r
		})
	}
	for ring := int32(1); ring <= squareNearRings; ring++ {
		if c, r, ok := freeOnRing(e, col, row, ring, taken); ok {
			return c, r, true
		}
	}
	return 0, 0, false
}

// freeOnRing is the first free square, row by row, at exactly ring squares from
// (col, row) and inside the grid.
func freeOnRing(e playdb.Encounter, col, row, ring int32, taken func(c, r int32) bool) (int32, int32, bool) {
	for dr := -ring; dr <= ring; dr++ {
		for dc := -ring; dc <= ring; dc++ {
			if max(dr, -dr, dc, -dc) != ring {
				continue
			}
			if c, r := col+dc, row+dr; inGrid(e, c, r) && !taken(c, r) {
				return c, r, true
			}
		}
	}
	return 0, 0, false
}

// joinCreatures inserts the creatures as combatants of the combat c.enc, after
// the combatants in all, and returns all with them and the ones it added. Each
// appears next to its owner's combatant when that is on the map. The
// creatures of a group in rolls take that group's initiative (a casting in a
// running combat, an undo); without one a creature has none yet and its owner's
// player rolls it (a combat in SETUP). A creature of Conjurar Animais seeds
// its owner's concentration when the owner has none. The caller saves the
// order.
func (s *Service) joinCreatures(ctx context.Context, c *combatTx, all []playdb.Combatant, creatures []link.Creature, rolls map[string]groupRoll) (_, added []playdb.Combatant, err error) {
	all = slices.Clone(all)
	taken := make(map[string]bool, len(all))
	for _, e := range all {
		taken[e.Label] = true
	}
	groupBonus := map[string]int32{} // one bonus for the group: the first creature of the casting's
	for _, cr := range creatures {
		if len(all) >= maxCombatants {
			break // a combat holds 40: the master leaves one out by dismissing
		}
		row := creatureRow(c, cr, len(all), taken, groupBonus, rolls)
		owner := ownerOf(all, cr)
		if owner != nil && placed(*owner) {
			if col, r, ok := squareNear(c.enc, all, *owner.GridCol, *owner.GridRow); ok {
				row.GridCol, row.GridRow = &col, &r
			}
		}
		inserted, err := c.q.InsertCreatureCombatant(ctx, row)
		if err != nil {
			return nil, nil, fmt.Errorf("insert a creature combatant: %w", err)
		}
		all = append(all, inserted)
		added = append(added, inserted)
		if err := s.seedConcentration(ctx, c, cr, owner); err != nil {
			return nil, nil, err
		}
	}
	return all, added, nil
}

// creatureRow is the row of a creature that joins as the n-th combatant: its label made
// unique among the taken ones, its stat block, and the initiative of its group.
func creatureRow(c *combatTx, cr link.Creature, n int, taken map[string]bool, groupBonus map[string]int32, rolls map[string]groupRoll) playdb.InsertCreatureCombatantParams {
	label := copyLabels(cr.Name, 1, taken)[0]
	row := playdb.InsertCreatureCombatantParams{
		EncounterID: c.enc.ID, CharacterID: cr.CharacterID, Label: label,
		InitiativeBonus: clamp32(cr.InitiativeBonus, -20, 40), OrderIndex: clamp32(n, 0, math.MaxInt32),
		SpeedFt: clamp32(cr.SpeedFt, 0, 600), CreatedAt: c.now,
		HpCurrent: new(clamp32(max(cr.HitPointsCurrent, 1), 1, math.MaxInt32)), HpMax: new(clamp32(max(cr.HitPointsMax, 1), 1, math.MaxInt32)),
		CreatureID: &cr.ID, MonsterKey: &cr.MonsterKey, SummonAttack: &cr.Attack, SummonGroupID: &cr.GroupID,
		// A creature fights for the party (D7), with its stat block's size, fly speed and
		// jump limits, as a character's are copied when it joins.
		Size: sizeKey(cr.Size), SpeedFlyFt: clamp32(cr.SpeedFlyFt, 0, 600),
		JumpLongDft: clamp32(cr.JumpLongDFt, 0, 6000), JumpHighDft: clamp32(cr.JumpHighDFt, 0, 6000),
		SpeedSwimFt: clamp32(cr.SpeedSwimFt, 0, 600), SpeedClimbFt: clamp32(cr.SpeedClimbFt, 0, 600), SpeedBurrowFt: clamp32(cr.SpeedBurrowFt, 0, 600),
	}
	if cr.OwnerUserID != "" {
		row.UserID = &cr.OwnerUserID
	}
	// Whoever rolls for the group, the bonus is the first creature's of the casting.
	if bonus, ok := groupBonus[cr.GroupID]; ok {
		row.InitiativeBonus = bonus
	} else {
		groupBonus[cr.GroupID] = row.InitiativeBonus
	}
	if roll, ok := rolls[cr.GroupID]; ok {
		row.Initiative, row.InitiativeFace, row.InitiativeBonus = new(roll.Total), new(roll.Face), roll.Bonus
	}
	return row
}

// ownerOf is the combatant of the creature's owner in all, the last one when several
// match; nil when there is none.
func ownerOf(all []playdb.Combatant, cr link.Creature) *playdb.Combatant {
	var owner *playdb.Combatant
	for i := range all {
		if all[i].Kind == kindPlayer && all[i].CharacterID == cr.CharacterID {
			owner = &all[i]
		}
	}
	return owner
}

// seedConcentration makes the creature's owner concentrate on the spell the creature
// depends on, when the owner has no concentration yet.
func (s *Service) seedConcentration(ctx context.Context, c *combatTx, cr link.Creature, owner *playdb.Combatant) error {
	spell := concentrationSpellOf(cr.Source, cr.SummonKey)
	if spell == "" || !cr.DependsOnConcentration || owner == nil || owner.ConcentrationSpell != nil {
		return nil
	}
	if err := c.q.SetCombatantConcentration(ctx, playdb.SetCombatantConcentrationParams{ID: owner.ID, ConcentrationSpell: &spell}); err != nil {
		return fmt.Errorf("seed the owner's concentration: %w", err)
	}
	owner.ConcentrationSpell = &spell
	return nil
}

// creatureStates are the hit points and the defeat of the creatures in cs, for
// the characters module to keep.
func creatureStates(cs []playdb.Combatant) []link.CreatureState {
	var out []link.CreatureState
	for _, cb := range cs {
		if isCreature(cb) && cb.CreatureID != nil {
			out = append(out, link.CreatureState{ID: *cb.CreatureID, HP: int(conv.OrZero(cb.HpCurrent)), Defeated: cb.Defeated || conv.OrZero(cb.HpCurrent) <= 0})
		}
	}
	return out
}

// syncCreatures keeps the creatures in step with the combat after a change,
// inside its transaction: a creature the change defeated (0 hit points) is
// dismissed from its owner's list, and one that was dismissed as defeated and
// is up again (a heal, the master's undo) is back. Every change to a
// combatant's hit points goes through here, so the damage, the heal, the
// master's hand and the undo need no code of their own for it. The dismissal of
// a defeated creature writes no event of its own: the damage that defeated it
// is in the history, and an event after it would close the undo of the action
// before.
func (s *Service) syncCreatures(ctx context.Context, c *combatTx) error {
	if c.enc.ID == "" || c.enc.Status == statusEnded {
		return nil
	}
	cs, err := c.q.ListCreatureCombatants(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the creatures of the combat: %w", err)
	}
	if len(cs) == 0 {
		return nil
	}
	if _, err := s.roster.SyncCreatures(ctx, c.tx, c.session.CampaignID, creatureStates(cs), c.now); err != nil {
		return err
	}
	return nil
}

// writeBackCreatures writes the hit points of the creatures in cs back to their
// owners' lists: the combat ended, or they leave it.
func (s *Service) writeBackCreatures(ctx context.Context, c *combatTx, cs []playdb.Combatant) error {
	return s.roster.WriteBackCreatures(ctx, c.tx, c.session.CampaignID, creatureStates(cs), c.now)
}

// dropCombatants takes the combatants out of the combat c.enc inside its
// transaction: each leaves the turn first (the turn passes when nobody who acts
// is left), its row goes, and the order is closed up. cs are the combatants as
// they are, in order; it returns the ones that stay and whether the turn passed.
//
// With dismiss, the rows stay and only hide (see endSummons).
func (s *Service) dropCombatants(ctx context.Context, c *combatTx, cs []playdb.Combatant, drop []playdb.Combatant, dismiss bool) (rest []playdb.Combatant, turnPassed bool, err error) {
	rest = slices.Clone(cs)
	for _, who := range drop {
		// What it cast or carried ends with it (RN-22).
		if err := s.endEffectsOfLeaver(ctx, c, rest, who); err != nil {
			return nil, false, err
		}
		passed, err := leaveTurn(ctx, c, rest, who)
		if err != nil {
			return nil, false, err
		}
		turnPassed = turnPassed || passed || inTurn(c.enc, who)
		if dismiss {
			if _, err := c.q.SetCreatureCombatantsDismissed(ctx, playdb.SetCreatureCombatantsDismissedParams{EncounterID: c.enc.ID, CreatureIds: []string{conv.OrZero(who.CreatureID)}, Dismissed: true}); err != nil {
				return nil, false, fmt.Errorf("dismiss the combatant: %w", err)
			}
		} else if err := c.q.DeleteCombatant(ctx, who.ID); err != nil {
			return nil, false, fmt.Errorf("delete the combatant: %w", err)
		}
		rest = slices.DeleteFunc(rest, func(o playdb.Combatant) bool { return o.ID == who.ID })
	}
	if _, err := saveOrder(ctx, c.q, cs, rest); err != nil {
		return nil, false, err
	}
	return rest, turnPassed, nil
}

// discardDamageOf drops, inside the change's transaction, the damage still to roll or to
// apply that names one of the combatants (as the attacker or as the target) before they
// leave the combat, with a damage_discarded event for each as passing a turn over it
// writes: the row goes with the combatant, and the log would keep an attack whose damage
// never lands. cs are the combatants of the combat.
func (s *Service) discardDamageOf(ctx context.Context, c *combatTx, cs, leaving []playdb.Combatant) error {
	open, err := c.q.ListOpenPendingDamages(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the pending damage: %w", err)
	}
	named := func(id string) bool {
		return slices.ContainsFunc(leaving, func(o playdb.Combatant) bool { return o.ID == id })
	}
	for _, p := range open {
		if !named(p.TargetID) && !named(conv.OrZero(p.AttackerID)) {
			continue
		}
		if _, err := c.q.SetPendingDamageStatus(ctx, playdb.SetPendingDamageStatusParams{ID: p.ID, Status: pendingDiscarded, ResolvedAt: &c.now}); err != nil {
			return fmt.Errorf("discard the pending damage: %w", err)
		}
		attacker, _ := findCombatant(cs, conv.OrZero(p.AttackerID), combatViewer{master: true})
		target, _ := findCombatant(cs, p.TargetID, combatViewer{master: true})
		if err := insertEvent(ctx, c, eventDamageDiscarded, &c.actorUserID, nil, actionEvent{
			Round: c.enc.Round, Secret: secretOf(attacker, target), Actor: attacker.ID, Target: p.TargetID, Pending: p.ID, Key: p.AttackKey,
			Amount: conv.OrZero(p.Amount), PrevStatus: p.Status,
		}); err != nil {
			return err
		}
	}
	return nil
}

// creatureIDsOf are the creature IDs of the combatants.
func creatureIDsOf(cs []playdb.Combatant) []string {
	var out []string
	for _, cb := range cs {
		if cb.CreatureID != nil {
			out = append(out, *cb.CreatureID)
		}
	}
	return out
}

// endSummons ends what a concentration on a summoning spell brought: the
// caster's creatures that last only while it concentrates leave their owner's
// list, and their combatants are dismissed (hidden from the order, the map and
// the turns, but kept whole: the same ids, hit points and conditions come back
// with an undo). A creature_dismissed event is written for each, before the
// change's own. It returns their IDs. Nothing happens when there are none.
func (s *Service) endSummons(ctx context.Context, c *combatTx, caster playdb.Combatant) (dismissed []string, err error) {
	if caster.Kind != kindPlayer {
		return nil, nil
	}
	creatures, err := s.roster.ConcentrationCreatures(ctx, c.tx, c.session.CampaignID, caster.CharacterID)
	if err != nil || len(creatures) == 0 {
		return nil, err
	}
	ids := make([]string, 0, len(creatures))
	for _, cr := range creatures {
		ids = append(ids, cr.ID)
	}
	if err := s.dropCreatureCombatants(ctx, c, ids); err != nil {
		return nil, err
	}
	if dismissed, err = s.roster.DismissCreatures(ctx, c.tx, c.session.CampaignID, ids, "concentration", c.now); err != nil {
		return nil, err
	}
	for _, id := range dismissed {
		if err := s.creatureEvent(ctx, c, eventCreatureDismissed, caster, actionEvent{
			Round: c.enc.Round, Actor: caster.ID, OwnerCharacter: caster.CharacterID, Created: []string{id}, Reason: "concentration",
		}); err != nil {
			return nil, err
		}
	}
	return dismissed, nil
}

// dropCreatureCombatants takes the combatants of the creatures out of the order and the map, the way a
// dismissal does; an undo brings them back (rejoinCreatures).
func (s *Service) dropCreatureCombatants(ctx context.Context, c *combatTx, ids []string) error {
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the combatants: %w", err)
	}
	leaving := slices.DeleteFunc(slices.Clone(cs), func(o playdb.Combatant) bool {
		return o.CreatureID == nil || !slices.Contains(ids, *o.CreatureID)
	})
	_, _, err = s.dropCombatants(ctx, c, cs, leaving, true)
	return err
}

// dropReplaced sends away the creatures a cast replaced (the old ones of its own spell, the old concentration's):
// their combatants leave and each has its line, with the reason the roster dismissed it for. It returns their ids.
func (s *Service) dropReplaced(ctx context.Context, c *combatTx, caster playdb.Combatant, replaced []link.Creature) ([]string, error) {
	if len(replaced) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(replaced))
	for _, cr := range replaced {
		ids = append(ids, cr.ID)
	}
	if err := s.dropCreatureCombatants(ctx, c, ids); err != nil {
		return nil, err
	}
	for _, cr := range replaced {
		if err := s.creatureEvent(ctx, c, eventCreatureDismissed, caster, actionEvent{
			Round: c.enc.Round, Actor: caster.ID, OwnerCharacter: caster.CharacterID, Created: []string{cr.ID}, Reason: cr.DismissedReason,
		}); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

// creatureEvent writes a creature event (ids only) to the history, inside the
// change's transaction, as an event of the caster's character.
func (s *Service) creatureEvent(ctx context.Context, c *combatTx, kind string, who playdb.Combatant, ev actionEvent) error {
	was := c.characterID
	c.characterID = &who.CharacterID
	err := insertEvent(ctx, c, kind, &c.actorUserID, nil, ev)
	c.characterID = was
	return err
}

// rejoinCreatures brings back the creatures an undo gave back to their owner
// (the ones a concentration ended or a cast replaced): they are alive again and
// their dismissed combatants return to the order, the same ones, with the hit
// points, conditions and initiative they had.
func (s *Service) rejoinCreatures(ctx context.Context, c *combatTx, ids []string) error {
	var revived []string
	for _, reason := range []string{"concentration", "replaced"} { // what a cast dismisses for; ids are the event's own
		back, err := s.roster.ReviveCreatures(ctx, c.tx, c.session.CampaignID, ids, reason)
		if err != nil {
			return err
		}
		revived = append(revived, back...)
	}
	if len(revived) == 0 {
		return nil
	}
	if _, err := c.q.SetCreatureCombatantsDismissed(ctx, playdb.SetCreatureCombatantsDismissedParams{EncounterID: c.enc.ID, CreatureIds: revived, Dismissed: false}); err != nil {
		return fmt.Errorf("bring the creatures back: %w", err)
	}
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the combatants: %w", err)
	}
	_, err = saveOrder(ctx, c.q, cs, orderCombatants(cs))
	return err
}

// removeCreatureCombatants takes the combatants of the creatures out of the
// combat and deletes the creatures a casting made (the master's undo of it).
func (s *Service) removeCreatureCombatants(ctx context.Context, c *combatTx, ids []string) error {
	cs, err := c.q.ListCombatants(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the combatants: %w", err)
	}
	leaving := slices.DeleteFunc(slices.Clone(cs), func(o playdb.Combatant) bool {
		return o.CreatureID == nil || !slices.Contains(ids, *o.CreatureID)
	})
	if _, _, err := s.dropCombatants(ctx, c, cs, leaving, false); err != nil {
		return err
	}
	return s.roster.DeleteCreatures(ctx, c.tx, c.session.CampaignID, ids)
}

// The initiative of the creatures a cast brings (RN-18): one roll for the
// group, by the owner's player in the app or from a physical die, and the master
// for anyone.

// rollGroup rolls the initiative of the creatures of one casting from the
// cast's d20: the face and the total with the group's bonus, which is the
// first creature's Dexterity modifier (ours, said in the docs: the creatures of
// a group may differ, and the group has one total).
func (s *Service) rollGroup(in rollInput, bonus int) (groupRoll, error) {
	face, roll, err := s.d20(in, bonus)
	if err != nil {
		return groupRoll{}, err
	}
	return groupRoll{Total: conv.Int32(roll.Total), Face: clamp32(face, 1, 20), Bonus: clamp32(bonus, -20, 40)}, nil
}

// summonErr turns the rules' refusal of a choice into the API's.
func summonErr(err error) error {
	switch {
	case errors.Is(err, rules.ErrNotSummonSpell):
		return connect.NewError(connect.CodeInvalidArgument, errors.New("this spell does not summon creatures"))
	case errors.Is(err, rules.ErrSummonCircle), errors.Is(err, rules.ErrSummonOption), errors.Is(err, rules.ErrSummonCount), errors.Is(err, rules.ErrSummonCreature):
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_SUMMON_CHOICE_INVALID, "that is not a choice the spell allows")
	}
	return err
}

// summonNames reads the names a casting gives its creatures: none, or one for
// each, each 1 to 40 characters on one line (a blank one takes the creature's
// name).
func summonNames(raw []string, n int) ([]string, error) {
	if len(raw) == 0 {
		return make([]string, n), nil
	}
	if len(raw) != n {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("summon.names must be empty or have one name for each creature"))
	}
	out := make([]string, n)
	for i, r := range raw {
		if r == "" {
			continue
		}
		name, err := names.Clean(r, 40)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("summon.names[%d] %w", i, err))
		}
		out[i] = name
	}
	return out, nil
}

// summonSourceTable is the source of the creatures of a table spell's summon.
const summonSourceTable = "table"

// summonSourceOf is where a creature of the spell came from, as the table says: a table spell's summon (chk.Table)
// or one of the SRD's spells.
func summonSourceOf(spellKey string, chk link.SummonSpell) string {
	if chk.Table {
		return summonSourceTable
	}
	switch spellKey {
	case "spell:find-familiar":
		return "familiar"
	case "spell:animate-dead":
		return "animate_dead"
	case "spell:conjure-animals":
		return "conjure_animals"
	}
	return ""
}

// summonOf is the casting to record in the roster: the creatures of the check, from the slot of the circle
// (0 for a ritual), with the source and, for a table spell, its key and rule.
func summonOf(campaignID, characterID, spellKey, groupID string, slot int, chk link.SummonSpell, specs []link.CreatureSpec) link.Summon {
	sm := link.Summon{
		CampaignID: campaignID, CharacterID: characterID, Source: summonSourceOf(spellKey, chk), GroupID: groupID, Slot: slot,
		Concentration: chk.Concentration, Creatures: specs,
	}
	if chk.Table {
		sm.SummonKey, sm.OwnInitiative, sm.MaxLive, sm.ReplacesSame = spellKey, chk.TurnMode == rules.SummonTurnOwnInitiative, chk.MaxLive, chk.ReplacesSame
	}
	return sm
}

// summonSpecs are the creatures of a choice, with their names.
func summonSpecs(made []link.SummonedForm, given []string) []link.CreatureSpec {
	out := make([]link.CreatureSpec, len(made))
	for i, m := range made {
		out[i] = link.CreatureSpec{MonsterKey: m.MonsterKey, Name: given[i], Attack: m.Attack}
	}
	return out
}

// checkSummonChoice asks the roster whether the character may cast the spell
// with this slot and this choice, and returns what the spell says and the
// creatures. viaGrant says the cast goes through a feature's grant with no slot.
func (s *Service) checkSummonChoice(ctx context.Context, tx pgx.Tx, campaignID, characterID, spellKey string, circle int, pick *playv1.SummonChoice, viaGrant bool) (link.SummonSpell, error) {
	chk, err := s.roster.CheckSummon(ctx, tx, campaignID, characterID, spellKey, circle, int(pick.GetOption()), pick.GetCreatureKeys())
	if err == nil && chk.Feature && !viaGrant {
		// The summon of a feature is cast through the feature's grant, with a use: never with a slot, a ritual or as a known spell.
		return chk, connect.NewError(connect.CodeInvalidArgument, errors.New("this summon belongs to a feature: cast it with granted_by"))
	}
	if err == nil && viaGrant && !chk.Feature {
		// A summoning spell that a grant gives is cast with the character's classes, not through the grant.
		return chk, connect.NewError(connect.CodeInvalidArgument, errors.New("a summoning spell is cast with the character's classes, not through a grant"))
	}
	return chk, summonErr(err)
}

// A player's character joining at a combat's start brings its creatures: the
// ones the owner has when the combat is set up (participants with the party).
// playerCharacterIDs are the player characters among the participants.
func playerCharacterIDs(parts []planned) []string {
	var out []string
	for _, p := range parts {
		if p.char.Player {
			out = append(out, p.char.ID)
		}
	}
	return out
}
