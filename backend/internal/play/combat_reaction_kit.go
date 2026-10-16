package play

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/reaction"
)

// The reactions of the window (PM-04) and what each one needs from a combatant:
// this file reads who can react with what. A player's character spends from its
// vitals, as a cast always did; an NPC's slots and uses are counted on the
// combatant (combatants.slots_used) because its sheet keeps no vitals.

// The spells and resources the reactions use.
const (
	spellShield        = shield
	spellCounterspell  = "spell:counterspell"
	spellHellishRebuke = "spell:hellish-rebuke"
	spellFeatherFall   = "spell:feather-fall"

	resInfernalLegacy = "infernal_legacy"
	resBardic         = "bardic_inspiration"
	resKi             = "ki"

	// usedPact and usedResource name the keys of slots_used besides the slot levels.
	usedPact     = "pact"
	usedResource = "res:"
)

// reactionSpellLevel is the spell level of each reaction spell, the lowest slot that casts it.
var reactionSpellLevel = map[string]int32{spellShield: 1, spellCounterspell: reaction.CounterspellLevel, spellHellishRebuke: 1, spellFeatherFall: 1}

// incapacitating are the conditions that take a creature's reactions away (SRD,
// Incapacitated and what includes it). Blinded does not: a blind creature reacts
// to what it can still perceive, and the sight of a trigger is checked apart.
var incapacitating = []string{
	"condition:incapacitated", "condition:paralyzed", "condition:petrified", "condition:stunned", "condition:unconscious",
}

// kit is what a combatant can react with.
type kit struct {
	who  playdb.Combatant
	st   link.ReactionStats
	opts *rulesv1.TurnOptions
	vit  *playv1.CharacterVitals
	// down says a player's character is at 0 hit points.
	down bool
	// surprised says the combatant is surprised and takes no reaction yet.
	surprised bool
}

// kitOf reads a combatant's reactions, inside tx (nil for a read). ok is false for
// a combatant that has none: a creature of a character, or a sheet that is gone.
func (s *Service) kitOf(ctx context.Context, tx pgx.Tx, campaignID string, who playdb.Combatant) (kit, bool, error) {
	if isCreature(who) {
		return kit{}, false, nil
	}
	st, err := s.roster.ReactionStats(ctx, tx, campaignID, who.CharacterID)
	if connect.CodeOf(err) == connect.CodeNotFound {
		return kit{}, false, nil
	}
	if err != nil {
		return kit{}, false, err
	}
	k := kit{who: who, st: st}
	if who.Kind == kindPlayer {
		if k.opts, err = s.roster.CombatTurnOptions(ctx, tx, campaignID, who.CharacterID, turnOf(who)); err != nil {
			return kit{}, false, err
		}
		if k.vit, err = s.vitals.GetVitalsTx(ctx, tx, campaignID, who.CharacterID); err != nil {
			return kit{}, false, err
		}
		k.down = isDownIn(k.vit)
	}
	// A surprised combatant takes no reaction until its first turn ends (SRD 5.1, "Surprise").
	if k.surprised, err = s.surprisedReactor(ctx, tx, who); err != nil {
		return kit{}, false, err
	}
	return k, true, nil
}

// canReact says the combatant has its reaction and is able to use it: not spent,
// not out of the fight, not down and not incapacitated.
func (k kit) canReact() bool {
	return !k.who.ReactionUsed && !k.who.EffectNoReaction && !k.who.Defeated && !k.down && !k.surprised && !slices.ContainsFunc(k.who.Conditions, func(c string) bool { return slices.Contains(incapacitating, c) })
}

// slotsUsedOf reads what an NPC's fight spent.
func slotsUsedOf(who playdb.Combatant) map[string]int {
	out := map[string]int{}
	if len(who.SlotsUsed) > 0 {
		_ = json.Unmarshal(who.SlotsUsed, &out) // a value this package wrote
	}
	return out
}

// slotsFor are the free slots a reaction spell can be cast with, from minLevel
// (at least the spell's own level) up: empty when the combatant does not have the
// spell or no slot is free.
func (k kit) slotsFor(spell string) []*rulesv1.SlotChoice {
	level := reactionSpellLevel[spell]
	if k.who.Kind == kindPlayer {
		if k.opts == nil {
			return nil
		}
		for _, sp := range k.opts.GetSpells() {
			if sp.GetSpell().GetKey() == spell {
				return slices.DeleteFunc(slices.Clone(sp.GetSlots()), func(s *rulesv1.SlotChoice) bool { return s.GetLevel() < level })
			}
		}
		return nil
	}
	if !slices.Contains(k.st.Spells, spell) {
		return nil
	}
	used := slotsUsedOf(k.who)
	var out []*rulesv1.SlotChoice
	for l := int(level); l <= 9; l++ {
		if free := k.st.SlotsTotal[l-1] - used[strconv.Itoa(l)]; free > 0 {
			out = append(out, &rulesv1.SlotChoice{Level: int32(l), Free: int32(free)}) //nolint:gosec // 1 to 9, a count of slots
		}
	}
	if free := k.st.PactSlots - used[usedPact]; free > 0 && int32(k.st.PactLevel) >= level { //nolint:gosec // a pact slot level
		out = append(out, &rulesv1.SlotChoice{Level: int32(k.st.PactLevel), Pact: true, Free: int32(free)}) //nolint:gosec // a count of slots
	}
	return out
}

// resourceLeft is how many uses of a resource the combatant has left (Bardic
// Inspiration, ki, the Infernal Legacy): a player's character from its vitals, an NPC's
// from its sheet less what the fight spent.
func (k kit) resourceLeft(key string) int32 {
	if k.who.Kind == kindPlayer {
		for _, r := range k.vit.GetResources() {
			if r.GetKey() == key {
				return max(r.GetTotal()-r.GetUsed(), 0)
			}
		}
		return 0
	}
	return int32(max(k.st.ResourceMax[key]-slotsUsedOf(k.who)[usedResource+key], 0)) //nolint:gosec // a count of uses
}

// legacyLeft is how many uses of the Infernal Legacy the combatant has left.
func (k kit) legacyLeft() int32 {
	if !k.st.InfernalLegacy {
		return 0
	}
	return k.resourceLeft(resInfernalLegacy)
}

// spendReactionSlot spends a slot of a reaction spell: a player's through its
// vitals, an NPC's on the combatant. It returns the player's vitals after.
func (s *Service) spendReactionSlot(ctx context.Context, c *combatTx, who playdb.Combatant, slot slotRef) (*playv1.CharacterVitals, error) {
	if who.Kind == kindPlayer {
		return s.spendSlot(ctx, c, who.CharacterID, slot, 1)
	}
	k, ok, err := s.kitOf(ctx, c.tx, c.session.CampaignID, who)
	if err != nil {
		return nil, err
	}
	key := strconv.Itoa(int(slot.Level))
	if slot.Pact {
		key = usedPact
	}
	free := k.st.SlotsTotal[max(min(int(slot.Level), 9), 1)-1] - slotsUsedOf(who)[key]
	if slot.Pact {
		free = k.st.PactSlots - slotsUsedOf(who)[usedPact]
	}
	if !ok || free <= 0 {
		return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_SLOT, "there is no free spell slot of that level",
			func(b *playv1.EncounterBlocked) { b.MinLevel = slot.Level })
	}
	return nil, s.bumpUsed(ctx, c, who, key, 1)
}

// bumpUsed adds delta to what an NPC's fight spent under key (a slot level, "pact"
// or a resource), never below 0.
func (s *Service) bumpUsed(ctx context.Context, c *combatTx, who playdb.Combatant, key string, delta int) error {
	used := slotsUsedOf(who)
	used[key] = max(used[key]+delta, 0)
	body, err := json.Marshal(used)
	if err != nil {
		return fmt.Errorf("encode the slots used: %w", err)
	}
	if err := c.q.SetCombatantSlotsUsed(ctx, playdb.SetCombatantSlotsUsedParams{ID: who.ID, SlotsUsed: body}); err != nil {
		return fmt.Errorf("count the slot spent: %w", err)
	}
	return nil
}

// spendReactionResource spends one use of a resource (the Infernal Legacy, Bardic
// Inspiration, ki): a player's through its vitals, an NPC's counted on the combatant.
func (s *Service) spendReactionResource(ctx context.Context, c *combatTx, who playdb.Combatant, key string) (*playv1.CharacterVitals, error) {
	if who.Kind == kindPlayer {
		return s.spendResource(ctx, c, who.CharacterID, key, 1)
	}
	return nil, s.bumpUsed(ctx, c, who, usedResource+key, 1)
}

// pickSlot checks the slot an answer chose against the free ones; invalid_argument
// when it is not one of them.
func pickSlot(in *playv1.SpellSlot, free []*rulesv1.SlotChoice) (slotRef, error) {
	if in == nil || !slices.ContainsFunc(free, func(s *rulesv1.SlotChoice) bool { return s.GetLevel() == in.GetLevel() && s.GetPact() == in.GetPact() }) {
		return slotRef{}, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("slot is not one of the free slots the window offers"))
	}
	return slotRef{Level: in.GetLevel(), Pact: in.GetPact()}, nil
}

// giveBackNPCSlot takes a slot an NPC spent back (the undo of its Shield).
func (s *Service) giveBackNPCSlot(ctx context.Context, c *combatTx, who playdb.Combatant, slot slotRef) error {
	key := strconv.Itoa(int(slot.Level))
	if slot.Pact {
		key = usedPact
	}
	return s.bumpUsed(ctx, c, who, key, -1)
}

// reopenWindows opens again the windows an undone change closed, as they were: the same
// reactor, the same trigger and its place among the windows to answer. Like the undo of an
// answer (reopenWindowOf), it leaves the clock to the master's screen, which starts the
// window's wait again when it sees it open.
func reopenWindows(ctx context.Context, c *combatTx, ids []string) error {
	for _, id := range ids {
		if err := c.q.ReopenReactionWindow(ctx, id); err != nil {
			return fmt.Errorf("open the reaction window again: %w", err)
		}
	}
	return nil
}

// reopenWindowOf opens again the window an undone answer had closed.
func (s *Service) reopenWindowOf(ctx context.Context, c *combatTx, ev actionEvent) error {
	if ev.Reaction == nil || ev.Reaction.Window == "" {
		return nil
	}
	if err := c.q.ReopenReactionWindow(ctx, ev.Reaction.Window); err != nil {
		return fmt.Errorf("open the reaction window again: %w", err)
	}
	return nil
}
