package play

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// The payload of the events the actions of a turn write (docs/data.md,
// session_events): IDs and numbers only, never a name or a free text
// (docs/privacy.md), at most 4 KiB. The combat log (combat_log.go) is
// built from these events, and the undo (combat_undo.go) puts back what they
// say was before, so they carry both.

// The values of a pending damage's status (pending_damages_status_valid).
const (
	pendingAwaitingReaction = "awaiting_reaction"
	pendingAwaitingRoll     = "awaiting_roll"
	pendingRolled           = "rolled"
	pendingApplied          = "applied"
	pendingDiscarded        = "discarded"
)

// The outcomes of an attack roll, as stored in an event.
const (
	outcomeHit  = "hit"
	outcomeCrit = "critical"
	outcomeMiss = "miss"
)

// hpState is the hit points of a combatant at one moment: the numbers an
// undo puts back.
type hpState struct {
	HP       int32 `json:"hp"`
	Temp     int32 `json:"temp,omitempty"`
	Defeated bool  `json:"defeated,omitempty"`
	// Shape is the Wild Shape form the druid was in at that moment (nil: its own
	// shape): the beast's pool is a number an undo puts back, like the hit points
	// (MR-037). HP and Temp are the character's own, which wait while the form lasts.
	Shape *shapeState `json:"shape,omitempty"`
}

// shapeState is a druid's beast form and the beast's hit points.
type shapeState struct {
	Beast string `json:"beast"`
	HP    int32  `json:"hp"`
}

// hpStateOf is a player's character's hit points, and its form, in its vitals.
func hpStateOf(v *playv1.CharacterVitals) hpState {
	out := hpState{HP: v.GetHitPointsCurrent(), Temp: v.GetHitPointsTemporary()}
	if w := v.GetWildShape(); w != nil {
		out.Shape = &shapeState{Beast: w.GetBeastKey(), HP: w.GetHitPointsCurrent()}
	}
	return out
}

// slotRef is the spell slot a spell spent: its level, and whether it was a
// pact magic slot. The undo gives it back.
type slotRef struct {
	Level int32 `json:"level"`
	Pact  bool  `json:"pact,omitempty"`
}

// deathState is a combatant's death save counts and whether its turn's save was
// rolled, and whether it was out of the fight (a confirmed death): what an undo
// puts back.
type deathState struct {
	Successes int32 `json:"successes,omitempty"`
	Failures  int32 `json:"failures,omitempty"`
	Rolled    bool  `json:"rolled,omitempty"`
	Dead      bool  `json:"dead,omitempty"`
}

func deathOf(c playdb.Combatant) *deathState {
	return &deathState{Successes: c.DeathSuccesses, Failures: c.DeathFailures, Rolled: c.DeathSaveRolled, Dead: c.Defeated}
}

// saveRoll is a target's saving throw against a spell, as the cast event keeps it.
type saveRoll struct {
	D20   int32 `json:"d20"`
	Bonus int32 `json:"bonus,omitempty"`
	Total int32 `json:"total"`
	DC    int32 `json:"dc"`
	Saved bool  `json:"saved,omitempty"`
	// Unknown says the target is a basic-sheet NPC with no saving throw bonus:
	// the roll is d20 + 0 and the master may overrule it.
	Unknown bool `json:"bonus_unknown,omitempty"`
}

// castHit is what a cast did to one target.
type castHit struct {
	Target string `json:"target_id"`
	Darts  int32  `json:"darts,omitempty"`
	// A spell attack: the d20, the bonus, the total and the outcome.
	Outcome  string    `json:"outcome,omitempty"`
	D20      int32     `json:"d20,omitempty"`
	Modifier int32     `json:"modifier,omitempty"`
	Total    int32     `json:"total,omitempty"`
	Physical bool      `json:"physical,omitempty"`
	Save     *saveRoll `json:"save,omitempty"`
	// Pending is the pending damage or heal the cast opened for the target.
	Pending string `json:"pending_id,omitempty"`
	// More are the other pending damages the cast opened for the target, one for
	// each further damage type of the spell, in the spell's order.
	More []string `json:"more_pending_ids,omitempty"`
	// The cover the target had against the caster for a spell attack or a Dexterity
	// save (see actionEvent.Cover), and, for a spell attack, the armor class the
	// total was compared with (the master's alone).
	Cover       string `json:"cover,omitempty"`
	CoverSource string `json:"cover_source,omitempty"`
	CoverBonus  int32  `json:"cover_bonus,omitempty"`
	TargetAC    int32  `json:"target_ac,omitempty"`
	// On a map with the fog of war, a cover the map gave is told to a player only if
	// they knew its squares and saw its creatures: see actionEvent.CoverRestricted.
	// A cast keeps them as a bit of the event's CoverUsers (CoverSeenMask) so that the
	// ids are not repeated for each of ten targets; an older event keeps the list.
	CoverRestricted bool     `json:"cover_restricted,omitempty"`
	CoverSeenBy     []string `json:"cover_seen_by,omitempty"`
	CoverSeenMask   uint64   `json:"cover_seen_mask,omitempty"`
	// HiddenAtCast says the target was a hidden creature when an area spell hit it:
	// the players' view of the cast never lists it, even after it is revealed, and
	// the master's marks it.
	HiddenAtCast bool `json:"hidden_at_cast,omitempty"`

	// A spell that reads hit points (combat_spells_hp.go): whether it reached the
	// target (the fx* values below), why not, the target's hit points when it did,
	// its place in the pool's order and what was left of the pool after it, and the
	// hit points regained by a heal.
	Fx       string `json:"fx,omitempty"`
	FxReason string `json:"fx_reason,omitempty"`
	HPBefore int32  `json:"hp_before,omitempty"`
	Order    int32  `json:"order,omitempty"`
	Left     int32  `json:"left,omitempty"`
	Healed   *int32 `json:"healed,omitempty"`
	// Gain is what Aid gave the target (the gain* values): its maximum, temporary
	// hit points or current hit points.
	Gain string `json:"gain,omitempty"`
	// What an undo puts back: the hit points and death saves when the spell
	// changed them (a heal, a death, Estabilizar), the conditions when it
	// changed them.
	Restore *hpState `json:"restore,omitempty"`
	// MaxBefore is the maximum hit points an NPC had before a spell raised them
	// (Ajuda), for the undo.
	MaxBefore   *int32      `json:"max_before,omitempty"`
	DeathBefore *deathState `json:"death_before,omitempty"`
	CondSet     bool        `json:"cond_set,omitempty"`
	CondBefore  []string    `json:"cond_before,omitempty"`
}

// What a spell that reads hit points did to a target, as a cast event stores it.
const (
	fxAffected   = "affected"
	fxUnaffected = "unaffected"
)

// damageHit is what a damage roll did to one pending damage of a cast: the
// amount that landed (half for a target that saved), and the numbers an undo
// puts back.
type damageHit struct {
	Pending     string      `json:"pending_id"`
	Target      string      `json:"target_id"`
	Amount      int32       `json:"amount"`
	Half        bool        `json:"half,omitempty"`
	Applied     bool        `json:"applied,omitempty"`
	Before      *hpState    `json:"before,omitempty"`
	After       *hpState    `json:"after,omitempty"`
	DeathBefore *deathState `json:"death_before,omitempty"`
	// ConcentrationDC is the save DC a damage that landed threatens a
	// concentration with (RN-22), 0 when it does not.
	ConcentrationDC int32 `json:"concentration_dc,omitempty"`
}

// actionEvent is the payload of every event of this slice, and of the 6.3
// move: one shape, each kind fills what it needs. Reading an event back
// (the log, the undo) decodes the same struct, so a field absent from the
// payload is its zero value.
type actionEvent struct {
	// Round is the combat's round when it happened.
	Round int32 `json:"round,omitempty"`
	// Secret says a hidden combatant was in it when it happened: the players
	// never get its line of the log, even after the master reveals the
	// combatant (RN-10, RN-20).
	Secret bool `json:"secret,omitempty"`
	// AttackerHidden says the master had hidden the attacker of the blow a reaction
	// answered: the line is the reactor's player's and the master's alone, even after a
	// reveal, or it would tell the others a hidden NPC attacked (RN-10).
	AttackerHidden bool `json:"attacker_hidden,omitempty"`
	// Fogged says the event happened on a map with the fog of war on, with an NPC
	// in it, and SeenBy lists the players (user IDs) who saw every NPC in it when it
	// happened: the log gives the line to them and to nobody else, and never works it
	// out again from what is seen later (MR-036, combat_fog.go). An event without
	// Fogged follows the older rule alone.
	Fogged bool     `json:"fogged,omitempty"`
	SeenBy []string `json:"seen_by,omitempty"`
	// Actor is who did it (attacker, mover, the one that took the action or
	// whose hit points changed), Target who it was done to.
	Actor  string `json:"combatant_id,omitempty"`
	Target string `json:"target_id,omitempty"`
	// NowHidden is the new state of the combatant a combatant_hidden_set event
	// is about.
	NowHidden bool `json:"hidden,omitempty"`
	// Pending is the pending damage the attack opened and its damage follows.
	Pending string `json:"pending_id,omitempty"`
	// Key is the attack or the action.
	Key string `json:"key,omitempty"`

	// The attack roll: the d20, the bonus, the total and the outcome.
	D20      int32  `json:"d20,omitempty"`
	Modifier int32  `json:"modifier,omitempty"`
	Total    int32  `json:"total,omitempty"`
	Physical bool   `json:"physical,omitempty"`
	Outcome  string `json:"outcome,omitempty"`
	// CriticalMax says the table's critical rule was "máximo mais uma rolagem" when
	// the attack was rolled (RN-24): the answer of a retry says the same.
	CriticalMaxRule bool `json:"critical_max_rule,omitempty"`
	// TargetAC is the armor class the total was compared with: only the
	// master's answer carries it (RN-20), and a retry reads it back from here.
	TargetAC int32 `json:"target_ac,omitempty"`

	// The damage roll: the dice, their faces, the total and the type. Applied
	// says an NPC took it at once.
	DiceCount  int32   `json:"dice_count,omitempty"`
	DiceSides  int32   `json:"dice_sides,omitempty"`
	Faces      []int32 `json:"faces,omitempty"`
	Amount     int32   `json:"amount,omitempty"`
	DamageType string  `json:"damage_type,omitempty"`
	Critical   bool    `json:"critical,omitempty"`
	Applied    bool    `json:"applied,omitempty"`
	// CriticalMax is what a critical hit added to the damage without rolling (the
	// dice's maximum, under the table's rule "máximo mais uma rolagem"): part of
	// Modifier, kept apart so the log can say where it came from.
	CriticalMax int32 `json:"critical_max,omitempty"`

	// Before and After are the target's hit points around a damage or the
	// master's hand.
	Before *hpState `json:"before,omitempty"`
	After  *hpState `json:"after,omitempty"`

	// A spell cast (spell_cast) and a reaction: the cast, the slot spent, what it
	// did to each target and the concentration it set or ended; a feature action:
	// the resource a use of which was spent. For a damage roll of a cast, Settled
	// is every pending damage the one roll settled.
	CastID     string      `json:"cast_id,omitempty"`
	Slot       *slotRef    `json:"slot,omitempty"`
	Resource   string      `json:"resource,omitempty"`
	Hits       []castHit   `json:"hits,omitempty"`
	CoverUsers []string    `json:"cover_users,omitempty"` // the players the hits' CoverSeenMask counts, bit by bit
	Settled    []damageHit `json:"settled,omitempty"`
	// Placed says the server worked out who an area spell hit, from the point or the
	// direction the caster chose (AreaCol and AreaRow, Dx and Dy): the line of the
	// cast is the players', with the hits on the hidden left out, instead of being
	// the master's alone because a hidden creature is in it. The answer of a retry
	// draws the area again from them.
	Placed  bool  `json:"placed,omitempty"`
	AreaCol int32 `json:"area_col,omitempty"`
	AreaRow int32 `json:"area_row,omitempty"`
	Dx      int32 `json:"dx,omitempty"`
	Dy      int32 `json:"dy,omitempty"`
	// Revealed are the hidden creatures the cast made appear, and PendingReveal the
	// question it opened for the master: what an undo of the cast puts back.
	Revealed      []string `json:"revealed,omitempty"`
	PendingReveal string   `json:"pending_reveal,omitempty"`
	// ByArea says a combatant_hidden_set event revealed the creature because of an
	// area spell (the table's rule, the master's choice or his answer): the players
	// get its line, "foi revelado".
	ByArea      bool `json:"by_area,omitempty"`
	Heal        bool `json:"heal,omitempty"`
	Concentrate bool `json:"concentrate,omitempty"`
	// ConcBefore is the spell the caster concentrated on before (empty: none),
	// and ConcEnded the one the cast stopped (the same, when it replaced it).
	ConcBefore string `json:"conc_before,omitempty"`
	ConcEnded  string `json:"conc_ended,omitempty"`
	// A spell that reads hit points: its kind (rules.SpellKind*), the condition it
	// gives, the limit of a threshold and the pool roll, which uses DiceCount,
	// DiceSides, Faces, Total and Physical above.
	FxKind      string `json:"fx_kind,omitempty"`
	FxCondition string `json:"fx_condition,omitempty"`
	FxLimit     int32  `json:"fx_limit,omitempty"`
	// Escudo: the +5 the target had before, the reaction and what it did.
	ACBonusBefore int32 `json:"ac_bonus_before,omitempty"`
	Stopped       bool  `json:"stopped,omitempty"`
	// AlsoStopped are the other hits on the same target that the Escudo's armor
	// class stopped too, with the status each one had.
	AlsoStopped []stoppedHit `json:"also_stopped,omitempty"`
	// Extra Attack and the opportunity attack.
	AttacksBefore int32 `json:"attacks_before,omitempty"`
	AsReaction    bool  `json:"as_reaction,omitempty"`
	// AsBonus says the attack was a bonus action attack (Two-Weapon Fighting,
	// Martial Arts, Flurry of Blows). AttackKeyBefore and FlurryBefore are the
	// attack of the action and the Flurry strikes left before the event.
	AsBonus         bool   `json:"as_bonus,omitempty"`
	AttackKeyBefore string `json:"attack_key_before,omitempty"`
	FlurryBefore    int32  `json:"flurry_before,omitempty"`

	// A death save, or damage at 0 hit points: the counts before and after,
	// the outcome and the failures the damage caused.
	Death         *deathState `json:"death,omitempty"`
	DeathBefore   *deathState `json:"death_before,omitempty"`
	DeathOutcome  string      `json:"death_outcome,omitempty"`
	FailuresAdded int32       `json:"failures_added,omitempty"`
	// DeathHidden says the table hid the death saves when this event was written
	// (RN-24): the owner and the master read its rolls and failures, nobody else, and
	// a later change of the rule never rewrites it, as the fog's SeenBy does not.
	DeathHidden bool `json:"death_hidden,omitempty"`
	// The master applied another amount than the rolled one (Rolled), and the
	// damage threatened a concentration (ConcentrationDC).
	Overridden      bool  `json:"overridden,omitempty"`
	Rolled          int32 `json:"rolled,omitempty"`
	ConcentrationDC int32 `json:"concentration_dc,omitempty"`
	// The conditions the master set (Conditions, with CondSet true even when
	// empty) and the ones there were.
	CondSet    bool     `json:"cond_set,omitempty"`
	Conditions []string `json:"conditions,omitempty"`
	CondBefore []string `json:"cond_before,omitempty"`

	// What the undo of an action puts back.
	ActionBefore   bool `json:"action_before,omitempty"`
	BonusBefore    bool `json:"bonus_before,omitempty"`
	ReactionBefore bool `json:"reaction_before,omitempty"`
	DashedBefore   bool `json:"dashed_before,omitempty"`
	// SpellCastBefore and BonusSpellBefore are the kinds of spell cast before a
	// cast (the bonus action spell limit).
	SpellCastBefore  bool   `json:"spell_cast_before,omitempty"`
	BonusSpellBefore bool   `json:"bonus_spell_before,omitempty"`
	PrevStatus       string `json:"prev_status,omitempty"`
	// Mode is how the master changed an NPC's hit points ("damage", "heal",
	// "set"), and Delta the change.
	Mode  string `json:"mode,omitempty"`
	Delta int32  `json:"delta,omitempty"`

	// A move: the square, what it cost the combatant and how far it went, in feet
	// (rounded down) and in tenths of a foot, which is the exact number (RN-21;
	// events written before Etapa 9 have only the feet).
	Col         int32 `json:"col,omitempty"`
	Row         int32 `json:"row,omitempty"`
	CostFt      int32 `json:"cost_ft,omitempty"`
	CostDFt     int32 `json:"cost_dft,omitempty"`
	DistanceFt  int32 `json:"distance_ft,omitempty"`
	DistanceDFt int32 `json:"distance_dft,omitempty"`
	OnTurn      bool  `json:"on_turn,omitempty"`
	// A jump: its kind ("long", "high"), the height of a high one, and whether a
	// long one landed in difficult terrain (the master's log reminds the
	// Acrobatics check, D3). From is where the combatant stood and what it had
	// walked: the undo puts it back.
	Jump             string     `json:"jump,omitempty"`
	HeightDFt        int32      `json:"height_dft,omitempty"`
	LandingDifficult bool       `json:"landing_difficult,omitempty"`
	From             *moveState `json:"from,omitempty"`
	// StoppedEarly says a creature the mover did not see cut the move short.
	StoppedEarly bool `json:"stopped_early,omitempty"`
	// LockedDoor says a locked door stopped the move (RN-26); a replay of the move
	// under the same key says it again.
	LockedDoor bool `json:"locked_door,omitempty"`

	// Opportunity attacks (MR-034, RN-21). MoveID is set on a move that made
	// offers (they share it: the undo of the move deletes them). OfferID is the
	// offer an event is about: opportunity_offered (Actor the mover, Target the
	// reactor, Col and Row the square the mover left the reach at), the
	// opportunity attack (attack_rolled) and its answer without an attack
	// (reaction_declined, Skipped when it was the master's "Seguir sem esperar").
	// ReturnedFrom is where the mover stood when an opportunity attack's damage
	// dropped it to 0 hit points and it went back to the square it left the reach
	// at: the undo puts it there again.
	MoveID       string     `json:"move_id,omitempty"`
	OfferID      string     `json:"offer_id,omitempty"`
	Skipped      bool       `json:"skipped,omitempty"`
	ReturnedFrom *moveState `json:"returned_from,omitempty"`
	// ReturnBlocked says the square to go back to was taken, so the mover stayed.
	ReturnBlocked bool `json:"return_blocked,omitempty"`
	// ByHand says the master made the offer (OfferOpportunity, a combat without a
	// grid): it has no square and its undo is a withdrawal. Withdrawn says the
	// reaction_declined event is the master taking an offer back.
	ByHand    bool `json:"by_hand,omitempty"`
	Withdrawn bool `json:"withdrawn,omitempty"`

	// The Disengage action, which an undo of the action takes back.
	DisengagedBefore bool `json:"disengaged_before,omitempty"`
	SurgedBefore     bool `json:"surged_before,omitempty"`
	// RunBefore is the running start (tenths of a foot) an action, an attack or a
	// spell broke: the undo puts it back.
	RunBefore int32 `json:"run_before,omitempty"`

	// The side an NPC was set to ("party", "enemy") and the one before, and the
	// cover the master marked ("none", "half", "three_quarters", "total") and the
	// one before.
	Side            string `json:"side,omitempty"`
	SideBefore      string `json:"side_before,omitempty"`
	CoverMark       string `json:"cover_mark,omitempty"`
	CoverMarkBefore string `json:"cover_mark_before,omitempty"`

	// The cover the target had against an attack: its degree ("half",
	// "three_quarters", "total"), where it came from ("map", "mark") and the armor
	// class it added (TargetAC already includes it).
	Cover       string `json:"cover,omitempty"`
	CoverSource string `json:"cover_source,omitempty"`
	CoverBonus  int32  `json:"cover_bonus,omitempty"`
	// On a map with the fog of war, a cover the map gave is the master's and, among
	// the players, the one of CoverSeenBy (user IDs) alone: the others knew neither the
	// squares nor the creatures that made it, and read no cover (combat_fog.go).
	CoverRestricted bool     `json:"cover_restricted,omitempty"`
	CoverSeenBy     []string `json:"cover_seen_by,omitempty"`

	// An undo: the event it took back.
	Undone     string `json:"undone_id,omitempty"`
	UndoneKind string `json:"undone_kind,omitempty"`
	// UndoneAlso are the older events taken back with it: the other parts of a
	// trap's firing that was written as several events.
	UndoneAlso []string `json:"undone_also,omitempty"`

	// The character's creatures (MR-037): the owner's character, the creatures a
	// casting made (Created, with their MonsterKeys) and the ones it dismissed
	// (Dismissed: a concentration the cast replaced or ended, with the initiative
	// their group had, which an undo gives back), where they came from (Source),
	// whether it was a ritual and why a creature was dismissed (Reason). IDs and
	// keys only.
	OwnerCharacter string     `json:"character_id,omitempty"`
	Created        []string   `json:"creature_ids,omitempty"`
	MonsterKeys    []string   `json:"monster_keys,omitempty"`
	Dismissed      []string   `json:"dismissed_ids,omitempty"`
	SummonRoll     *groupRoll `json:"summon_roll,omitempty"`
	Source         string     `json:"source,omitempty"`
	Ritual         bool       `json:"ritual,omitempty"`
	Reason         string     `json:"reason,omitempty"`

	// Wild Shape and the familiar's eyes (MR-037, MR-036): the beast's key, the damage
	// the beast's fall carried over to the character, the form before a change an
	// undo puts back (ShapeBefore, with ShapeSet true even when it was the
	// character's own shape), the creature the player looks through and "start" or
	// "stop", the conditions the sight gave, and the combat the change was made in
	// (empty outside one). IDs, keys and numbers only.
	Beast       string      `json:"beast,omitempty"`
	Carried     int32       `json:"carried_damage,omitempty"`
	ShapeSet    bool        `json:"shape_set,omitempty"`
	ShapeBefore *shapeState `json:"shape_before,omitempty"`
	Creature    string      `json:"creature_id,omitempty"`
	Sight       string      `json:"sight,omitempty"`
	SightConds  []string    `json:"sight_conditions,omitempty"`
	EncounterID string      `json:"encounter_id,omitempty"`
	// Spent says the change spent the combatant's action or bonus action (the
	// economy fields above are then what an undo puts back).
	Spent bool `json:"spent,omitempty"`
	// A trap that fired (`trap_triggered`, MR-035): what it did and what an undo
	// puts back. A search (`trap_searched`) keeps its roll in the same fields as an
	// attack (D20, Modifier, Total, Physical), Key the skill ("perception" or
	// "investigation") and Found the traps it revealed (point IDs).
	Trap *trapFireEvent `json:"trap,omitempty"`
	// Monsters is what AddMonsters did: the parameters (for a retry to be checked
	// against) and the new combatants (the master's log line).
	Monsters *monstersEvent `json:"monsters,omitempty"`
	// D20B is the second d20 of a Perception search with disadvantage.
	D20B  int32    `json:"d20_b,omitempty"`
	Found []string `json:"found,omitempty"`
	// Zone is what a line about a zone tells (zones_engine.go): ids and numbers only.
	Zone *zoneNote `json:"zone,omitempty"`
	// ZonesEnded are the zones a concentration that ended took with it: an undo brings them back.
	ZonesEnded []string `json:"zones_ended,omitempty"`
}

// readEvent decodes an event's payload. A payload of this module never fails
// to decode; an event that does is skipped by its readers.
func readEvent(payload []byte) (actionEvent, error) {
	var ev actionEvent
	if err := json.Unmarshal(payload, &ev); err != nil {
		return actionEvent{}, fmt.Errorf("read an event payload: %w", err)
	}
	return ev, nil
}

// secretOf says whether a hidden combatant is one of the combatants.
func secretOf(cs ...playdb.Combatant) bool {
	return slices.ContainsFunc(cs, func(c playdb.Combatant) bool { return c.Hidden })
}

// touchesHidden says whether the attacker or the target of any of the
// pending damages is hidden now.
func touchesHidden(cs []playdb.Combatant, pending []playdb.PendingDamage) bool {
	hidden := map[string]bool{}
	for _, c := range cs {
		hidden[c.ID] = c.Hidden
	}
	return slices.ContainsFunc(pending, func(p playdb.PendingDamage) bool { return hidden[deref(p.AttackerID)] || hidden[p.TargetID] })
}

// hpOf is a combatant's hit points as the undo stores them.
func hpOf(c playdb.Combatant) hpState {
	return hpState{HP: num(c.HpCurrent), Temp: num(c.HpTemp), Defeated: c.Defeated}
}

// num is the value of an optional number, 0 when unset.
func num(n *int32) int32 {
	if n == nil {
		return 0
	}
	return *n
}

// pendingOf returns the damage the combatant's attacks still hold open:
// still to roll, or rolled and waiting for the master.
func (c *combatTx) pendingOf(ctx context.Context, attackerID string) ([]playdb.PendingDamage, error) {
	open, err := c.q.ListOpenPendingDamages(ctx, c.enc.ID)
	if err != nil {
		return nil, fmt.Errorf("list the pending damage: %w", err)
	}
	return slices.DeleteFunc(open, func(p playdb.PendingDamage) bool { return deref(p.AttackerID) != attackerID }), nil
}

// linesSeenBy is who may see at least one of the lines a change wrote, on a map with
// the fog of war: the union of their seen_by. It says scoped false when there is no
// line or one of them is for every player (it has no NPC out of sight in it), and the
// hint then goes to all of them.
func linesSeenBy(lines []actionEvent) (seers []string, scoped bool) {
	if len(lines) == 0 {
		return nil, false
	}
	for _, ev := range lines {
		if !ev.Fogged {
			return nil, false
		}
		for _, u := range ev.SeenBy {
			if !slices.Contains(seers, u) {
				seers = append(seers, u)
			}
		}
	}
	return seers, true
}

// publishLogChanged tells the streams to read the combat log again: the
// master's always, the players' only when the change touches a line they may
// see.
//
// On a map with the fog of war, a line that was stamped with who could see it goes
// only to those players (actionEvent.SeenBy), so a player does not even hear that
// something happened out of their sight.
func (s *Service) publishLogChanged(ctx context.Context, campaignID, encounterID string, players bool) {
	msg := &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_CombatLogChanged_{
		CombatLogChanged: &playv1.WatchGameSessionResponse_CombatLogChanged{EncounterId: encounterID},
	}}
	s.hub.Publish(campaignID, live.Event{Audience: live.Audience{Master: true}, Message: msg})
	if !players {
		return
	}
	if memo := fogMemoOf(ctx); memo != nil {
		if seers, scoped := linesSeenBy(memo.lines); scoped {
			for _, u := range seers {
				s.hub.Publish(campaignID, live.Event{Audience: live.Audience{UserID: u}, Message: msg})
			}
			return
		}
	}
	s.hub.Publish(campaignID, live.Event{Audience: live.Audience{Players: true}, Message: msg})
}

// stoppedHit is a pending damage that a later Escudo stopped: its id and the
// status the undo puts back.
type stoppedHit struct {
	Pending    string `json:"pending"`
	PrevStatus string `json:"prev_status"`
}
