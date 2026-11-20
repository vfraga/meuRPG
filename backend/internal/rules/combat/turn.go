package combat

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// TurnState is what the character has used in the current turn.
type TurnState struct {
	ActionUsed, BonusActionUsed, ReactionUsed bool
	// MovementUsedFt is the feet walked so far this turn.
	MovementUsedFt int
	// Dashed says the Dash action was taken: the speed counts twice.
	Dashed bool
	// AttacksMade is how many attacks the Attack action made this turn
	// (Extra Attack); the first one spends the action.
	AttacksMade int
	// ActionSurged says Action Surge was used this turn: it is used once per
	// turn, whatever the uses left.
	ActionSurged bool
	// SpellCast says a spell other than a bonus action one and other than a
	// cantrip of 1 action was cast this turn, and BonusSpellCast that a spell was
	// cast with a bonus action: the first makes a bonus action spell illegal and
	// the second every spell but that cantrip (SRD 5.1, Casting Time).
	SpellCast, BonusSpellCast bool
	// LastAttackKey is the attack the Attack action made last this turn ("" if
	// none), and FlurryLeft how many unarmed strikes of Flurry of Blows are
	// left: what the bonus action attack rules read.
	LastAttackKey string
	FlurryLeft    int
	// FrenzyReady says the character is in a frenzied rage that began in an earlier
	// turn (the Berserker's Frenzy, SRD 5.1): a melee weapon attack is its bonus action.
	FrenzyReady bool
}

// actionSurgeResource is the resource of Action Surge.
const actionSurgeResource = "action_surge"

// Reason codes for a disabled option. They are codes, never text: the web
// maps each to Portuguese copy ("Ação já usada", "Sem espaço de 2º nível
// ou maior").
const (
	// ReasonAttacksUsed: the Attack action made all its attacks (Extra
	// Attack); the plain ReasonActionUsed says it for a single attack.
	ReasonAttacksUsed = "ATTACKS_USED"
	// ReasonActionUsed, ReasonBonusActionUsed and ReasonReactionUsed: the
	// economy the option needs is spent.
	ReasonActionUsed      = "ACTION_USED"
	ReasonBonusActionUsed = "BONUS_ACTION_USED"
	ReasonReactionUsed    = "REACTION_USED"
	// ReasonNoSlot: no free slot at the spell's level or above (MinLevel).
	ReasonNoSlot = "NO_SLOT"
	// ReasonNoUses: the feature's resource is spent (Recharge says when it
	// comes back).
	ReasonNoUses = "NO_USES"
	// ReasonAlreadyUsedThisTurn: a feature that can be used once per turn
	// (Action Surge) was used in this turn.
	ReasonAlreadyUsedThisTurn = "ALREADY_USED_THIS_TURN"
	// ReasonBonusActionSpellLimit: a spell cast with a bonus action leaves no
	// other spell for the turn but a cantrip of 1 action, in either order.
	ReasonBonusActionSpellLimit = "BONUS_ACTION_SPELL_LIMIT"
	// ReasonReactionOnlyWhenHit: Shield, which is only cast when an attack
	// hits the caster, never on their own turn.
	ReasonReactionOnlyWhenHit = "REACTION_ONLY_WHEN_HIT"
	// ReasonReactionOnly: any other reaction spell, cast when its trigger
	// happens.
	ReasonReactionOnly = "REACTION_ONLY"
	// ReasonTooLong: the casting time is a minute or more, too long for a
	// fight.
	ReasonTooLong = "CASTING_TIME_TOO_LONG"
	// ReasonAttackActionFirst: Flurry of Blows comes right after the Attack
	// action, and it was not taken with a weapon or an unarmed strike yet.
	ReasonAttackActionFirst = "ATTACK_ACTION_FIRST"
)

// Reason says why an option is disabled.
type Reason struct {
	Code string
	// MinLevel is set for ReasonNoSlot: the lowest slot level that would
	// do.
	MinLevel int
	// Recharge is set for ReasonNoUses: a rules.Recharge* constant.
	Recharge string
}

// Slot is the state of one part of the action economy.
type Slot struct {
	Used, Available bool
}

// Movement is the walking left in the turn.
type Movement struct {
	// SpeedFt is the speed for this turn: doubled after the Dash action.
	SpeedFt, UsedFt, LeftFt int
}

// Economy is the turn's action economy.
type Economy struct {
	Action, BonusAction, Reaction Slot
	Movement                      Movement
	// AttacksPerAction is how many attacks the Attack action makes, and
	// AttacksLeft how many remain: all of them before the first attack, none
	// once the action is spent on something else.
	AttacksPerAction, AttacksLeft int
}

// Option is something the character may try. When Enabled is false, Reason
// says why.
type Option struct {
	Enabled bool
	Reason  *Reason
}

// AttackOption is a line of Derived.Attacks.
type AttackOption struct {
	Option
	Attack rules.Attack
	// Bonus is the rule that makes it a bonus action attack now (BonusNone while
	// the Attack action has attacks left), and FlurryLeft the strikes of Flurry
	// of Blows left when Bonus is BonusFlurry. Attack.Damage is already the
	// damage the server rolls: without the ability modifier for BonusTwoWeapon.
	Bonus      BonusKind
	FlurryLeft int
	// DropsModifier says the off-hand damage leaves out the ability modifier.
	DropsModifier bool
	// BeamsLeft is how many beams of a cantrip cast with the action are still to
	// fire (Eldritch Blast); the option is enabled while any is left.
	BeamsLeft int
}

// SlotChoice is a slot the spell can be cast with.
type SlotChoice struct {
	// Level is the slot level; Pact says it is a pact magic slot.
	Level int
	Pact  bool
	// Free is how many slots of this kind are free, so the app can warn
	// about the last one.
	Free int
	// Total is how many slots of this kind the character has, spent or not.
	Total int
}

// SpellOption is a spell the character can cast.
type SpellOption struct {
	Option
	Spell rules.SpellEntry
	// Economy is what casting costs: rules.EconomyAction, EconomyBonusAction
	// or EconomyReaction; "" for a casting time too long for a fight.
	Economy string
	// Slots are the slot levels it can be cast with: at least the spell's
	// level and with a free slot. Empty for a cantrip, and for a spell with
	// no slot (then the option is disabled with ReasonNoSlot).
	Slots []SlotChoice
	// Grant is set for a spell a granted_spell effect gives (grants.go): it is cast
	// without a slot, spending a use of the grant's resource unless it is at will.
	Grant *SpellGrant
	// FeatureSummon is set for the summon of a table feature, which is no spell: the limit of the turn's spells
	// does not apply to it.
	FeatureSummon bool
}

// ActionOption is a standard action or a feature's action.
type ActionOption struct {
	Option
	Action rules.Action
	// UsesLeft is how many uses of the action's resource remain, when it
	// has one.
	UsesLeft int
}

// TurnOptions is everything "Sua vez" offers (MR-014).
type TurnOptions struct {
	Economy         Economy
	Attacks         []AttackOption
	Spells          []SpellOption
	StandardActions []ActionOption
	FeatureActions  []ActionOption
}

// shieldKey is the one reaction spell with its own reason.
const shieldKey = "spell:shield"

// Options works out what the character can do now, from their sheet, what
// they used this turn and what they spent since the last rest. It is
// computed on every read and never stored.
func Options(d rules.Derived, turn TurnState, u Usage) TurnOptions {
	out := TurnOptions{Economy: economyOf(d, turn)}
	// A cantrip cast with the action is no Attack action: Extra Attack has nothing
	// left to give after it.
	castCantrip := castWithCantrip(d, turn)
	if castCantrip {
		out.Economy.AttacksLeft = 0
	}
	// Attacks cost an action. The damaging cantrips are in Derived.Attacks
	// already, so they are left out of the spells below.
	cantripAttacks := map[string]bool{}
	for _, a := range d.Attacks {
		if a.Kind == "spell" {
			cantripAttacks[a.Key] = true
		}
		out.Attacks = append(out.Attacks, sheetAttackOption(d, turn, out.Economy, castCantrip, a))
	}
	for _, cs := range d.Spells {
		if !cs.Prepared || cantripAttacks[cs.Spell.Key] {
			continue
		}
		out.Spells = append(out.Spells, spellOption(d, turn, u, cs.Spell))
	}
	out.Spells = append(out.Spells, grantedOptions(d, turn, u)...)
	sortSpells(out.Spells)
	for _, a := range d.StandardActions {
		out.StandardActions = append(out.StandardActions, actionOption(d, turn, u, a))
	}
	for _, a := range d.Actions {
		out.FeatureActions = append(out.FeatureActions, flurryOption(d, turn, actionOption(d, turn, u, a)))
	}
	return out
}

// dashFactor is how many times the speed counts after the Dash action.
const dashFactor = 2

// economyOf is the turn's economy before a cantrip is taken into account.
func economyOf(d rules.Derived, turn TurnState) Economy {
	speed := d.SpeedWalkFt
	if turn.Dashed {
		speed *= dashFactor
	}
	e := Economy{
		Action:      Slot{Used: turn.ActionUsed, Available: !turn.ActionUsed},
		BonusAction: Slot{Used: turn.BonusActionUsed, Available: !turn.BonusActionUsed},
		Reaction:    Slot{Used: turn.ReactionUsed, Available: !turn.ReactionUsed},
		Movement:    Movement{SpeedFt: speed, UsedFt: turn.MovementUsedFt, LeftFt: max(speed-turn.MovementUsedFt, 0)},
	}
	e.AttacksPerAction = max(d.AttacksPerAction, 1)
	e.AttacksLeft = AttacksLeft(e.AttacksPerAction, turn)
	return e
}

// sheetAttackOption is the option of one sheet attack.
func sheetAttackOption(d rules.Derived, turn TurnState, e Economy, castCantrip bool, a rules.Attack) AttackOption {
	// Extra Attack belongs to the Attack action, that is, to weapon attacks: a
	// cantrip is cast with the whole action (SRD 5.1).
	opt := attackOption(e.AttacksPerAction, turn)
	if a.Kind == "spell" {
		opt = economyOption(rules.EconomyAction, turn)
	}
	ao := AttackOption{Option: opt, Attack: a}
	switch {
	case a.Kind == "spell":
		// The beams of the cast that spent the action are still its own.
		if left := beamsLeft(d, turn, a); left > 0 {
			ao.Option, ao.BeamsLeft = Option{Enabled: true}, left
		}
	case castCantrip:
		// The cantrip took the whole action: no attack of the Attack action is
		// left, and no bonus action attack follows it.
		ao.Option = Option{Reason: &Reason{Code: ReasonActionUsed}}
	case e.AttacksLeft == 0:
		ao = bonusAttackOption(d, turn, ao)
	}
	return ao
}

// FlurryOfBlowsKey is the monk's bonus action that spends 1 ki point for two
// unarmed strikes.
const FlurryOfBlowsKey = "feature:flurry-of-blows"

// bonusTraits is what the bonus action rules read of a sheet attack.
func bonusTraits(a rules.Attack) AttackTraits {
	return AttackTraits{Spell: a.Kind == "spell", Melee: a.Melee, Light: a.Light, Unarmed: a.Key == rules.UnarmedStrikeKey, MartialArts: a.MartialArts}
}

// ContestAttackKey marks that the Attack action was taken with a grapple or a shove
// (SRD 5.1, "Grappling": the special melee attack replaces one attack of the Attack
// action) and no weapon attack has been made yet. It is no sheet attack: it counts as the
// Attack action for Flurry of Blows ("immediately after you take the Attack action") but,
// being neither an unarmed strike nor a monk weapon, never unlocks the Martial Arts bonus
// strike or Two-Weapon Fighting.
const ContestAttackKey = "special:contest"

// ContestAttack is the stand-in attack the key stands for: a melee attack that is not
// light, not unarmed and not a spell.
func ContestAttack() rules.Attack {
	return rules.Attack{Key: ContestAttackKey, Melee: true}
}

// lastAttackOf is the sheet attack the Attack action made last this turn.
func lastAttackOf(d rules.Derived, turn TurnState) (rules.Attack, bool) {
	if turn.LastAttackKey == "" {
		return rules.Attack{}, false
	}
	if turn.LastAttackKey == ContestAttackKey {
		return ContestAttack(), true
	}
	i := slices.IndexFunc(d.Attacks, func(a rules.Attack) bool { return a.Key == turn.LastAttackKey })
	if i < 0 {
		return rules.Attack{}, false
	}
	return d.Attacks[i], true
}

// castWithCantrip says the action of this turn went to a cantrip.
func castWithCantrip(d rules.Derived, turn TurnState) bool {
	last, ok := lastAttackOf(d, turn)
	return ok && turn.ActionUsed && last.Kind == "spell"
}

// beamsLeft is how many beams of the cantrip such a cast still has: the cast
// that spent the action fired it, and made fewer beams than the cantrip has.
// 0 when the action has not been spent on this cantrip.
func beamsLeft(d rules.Derived, turn TurnState, a rules.Attack) int {
	last, ok := lastAttackOf(d, turn)
	if !ok || !turn.ActionUsed || last.Key != a.Key || a.Beams <= 1 {
		return 0
	}
	return max(a.Beams-turn.AttacksMade, 0)
}

// attackActionTaken says the Attack action was taken this turn with a weapon or
// an unarmed strike (not a cantrip): the condition of every bonus action
// attack.
func attackActionTaken(d rules.Derived, turn TurnState) bool {
	last, ok := lastAttackOf(d, turn)
	return ok && turn.ActionUsed && turn.AttacksMade > 0 && last.Kind != "spell"
}

// bonusAttackOption turns an attack the Attack action cannot make any more into
// a bonus action attack, when a rule lets the character make it now: enabled
// while the bonus action is free (Flurry of Blows spent it already), disabled
// with the bonus action's reason when it is spent (the rule is worked out as if
// it were free, so the reason is the bonus action's, not the action's). The off-hand attack shows the
// damage the server rolls.
func bonusAttackOption(d rules.Derived, turn TurnState, ao AttackOption) AttackOption {
	last, _ := lastAttackOf(d, turn)
	kind := BonusAttack(BonusAttackTurn{
		AttackAction: attackActionTaken(d, turn), FlurryLeft: turn.FlurryLeft, Last: bonusTraits(last), FrenzyReady: turn.FrenzyReady,
		NoSecondLight: SecondLightWeaponMissing(countLight(d.Attacks, func(a rules.Attack) bool { return a.Melee && a.Light })),
	}, bonusTraits(ao.Attack))
	if kind == BonusNone {
		return ao
	}
	ao.Bonus = kind
	switch {
	case kind == BonusFlurry:
		ao.FlurryLeft = turn.FlurryLeft
		ao.Option = Option{Enabled: true}
	case turn.BonusActionUsed:
		ao.Option = Option{Reason: &Reason{Code: ReasonBonusActionUsed}}
	default:
		ao.Option = Option{Enabled: true}
	}
	if kind == BonusTwoWeapon {
		dice := ao.Attack.DamageDice
		kept := OffHandBonus(dice.Bonus, ao.Attack.AbilityMod, d.TwoWeaponFighting)
		ao.DropsModifier = kept != dice.Bonus
		dice.Bonus = kept
		ao.Attack.DamageDice, ao.Attack.Damage = dice, diceText(dice)
	}
	return ao
}

// flurryOption disables Flurry of Blows until the Attack action was taken with
// a weapon or an unarmed strike. A spent bonus action says so first.
func flurryOption(d rules.Derived, turn TurnState, o ActionOption) ActionOption {
	if o.Action.Key != FlurryOfBlowsKey || attackActionTaken(d, turn) {
		return o
	}
	if o.Enabled || (o.Reason != nil && o.Reason.Code == ReasonNoUses) {
		o.Enabled, o.Reason = false, &Reason{Code: ReasonAttackActionFirst}
	}
	return o
}

// diceText writes a formula as the sheet shows it: "1d6+2", "2d8-1".
func diceText(f rules.DiceFormula) string {
	text := strconv.Itoa(f.Count) + "d" + strconv.Itoa(f.Sides)
	switch {
	case f.Bonus > 0:
		return text + "+" + strconv.Itoa(f.Bonus)
	case f.Bonus < 0:
		return text + strconv.Itoa(f.Bonus)
	}
	return text
}

// AttacksLeft is how many attacks the Attack action can still make this turn:
// all of them while the action is free, the rest of them after the first
// attack spent it (Extra Attack), none when something else spent it. perAction
// is Derived.AttacksPerAction.
func AttacksLeft(perAction int, turn TurnState) int {
	perAction = max(perAction, 1)
	switch {
	case !turn.ActionUsed:
		return perAction
	case turn.AttacksMade > 0:
		return max(perAction-turn.AttacksMade, 0)
	}
	return 0
}

// attackOption is an attack's option: it costs the action, but a second attack
// of the same Attack action (Extra Attack) does not cost another one.
func attackOption(perAction int, turn TurnState) Option {
	if AttacksLeft(perAction, turn) > 0 {
		return Option{Enabled: true}
	}
	if turn.AttacksMade > 0 && perAction > 1 {
		return Option{Reason: &Reason{Code: ReasonAttacksUsed}}
	}
	return Option{Reason: &Reason{Code: ReasonActionUsed}}
}

// economyOption is enabled while the economy is free. Free and movement
// actions are always free.
func economyOption(economy string, turn TurnState) Option {
	switch {
	case economy == rules.EconomyAction && turn.ActionUsed:
		return Option{Reason: &Reason{Code: ReasonActionUsed}}
	case economy == rules.EconomyBonusAction && turn.BonusActionUsed:
		return Option{Reason: &Reason{Code: ReasonBonusActionUsed}}
	case economy == rules.EconomyReaction && turn.ReactionUsed:
		return Option{Reason: &Reason{Code: ReasonReactionUsed}}
	}
	return Option{Enabled: true}
}

// spellEconomy maps a casting time to an economy, "" when it is too long.
func spellEconomy(ct rules.CastingTime) string { return ct.Economy() }

// sortSpells puts the spells in the order "O que você pode fazer" lists them
// (MR-014): the ones that can be cast now first, then the rest;
// in each group the cantrips first, then by circle, then by Portuguese name.
// Shield is never castable on the character's own turn (a reaction when hit), so
// it sorts with the rest. The order is stable, so spells that tie keep the
// order of the sheet.
func sortSpells(spells []SpellOption) {
	slices.SortStableFunc(spells, func(a, b SpellOption) int {
		if a.Enabled != b.Enabled {
			if a.Enabled {
				return -1
			}
			return 1
		}
		if c := cmp.Compare(a.Spell.Level, b.Spell.Level); c != 0 {
			return c
		}
		return strings.Compare(sortName(a.Spell), sortName(b.Spell))
	})
}

// ptFold takes the accents off the letters Portuguese names use, so "Ânimo"
// sorts with the As, not after the Zs.
var ptFold = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "é", "e", "ê", "e", "í", "i",
	"ó", "o", "ô", "o", "õ", "o", "ú", "u", "ü", "u", "ç", "c",
)

// sortName is the name a spell sorts by: Portuguese, lowercase, without accents.
func sortName(sp rules.SpellEntry) string {
	name := sp.NamePT
	if name == "" {
		name = sp.Name
	}
	return ptFold.Replace(strings.ToLower(name))
}

// SpellLimited says the turn's spells forbid this one: after a spell cast with a
// bonus action only a cantrip with a casting time of 1 action may follow, and a
// spell cast with a bonus action may not follow any other spell but that cantrip
// (SRD 5.1, Casting Time). economy is the spell's, level its own.
func SpellLimited(turn TurnState, economy string, level int) bool {
	if IsFreeCantrip(economy, level) {
		return false
	}
	return turn.BonusSpellCast || (economy == rules.EconomyBonusAction && turn.SpellCast)
}

// ScrollReadReason is why the spell of a Spell Scroll could not be cast in this turn, by the two checks a spell's option makes after
// what the spell is: the economy of its casting time, then the limit a bonus action spell puts on the turn's spells. nil when it
// could. economy is the casting time's ("" for a longer one, which spends none) and level the spell's.
func ScrollReadReason(turn TurnState, economy string, level int) *Reason {
	if eo := economyOption(economy, turn); !eo.Enabled {
		return eo.Reason
	}
	if SpellLimited(turn, economy, level) {
		return &Reason{Code: ReasonBonusActionSpellLimit}
	}
	return nil
}

// IsFreeCantrip says a spell is a cantrip with a casting time of 1 action, the
// one spell the bonus action limit allows beside a bonus action spell.
func IsFreeCantrip(economy string, level int) bool {
	return level == 0 && economy == rules.EconomyAction
}

func spellOption(d rules.Derived, turn TurnState, u Usage, sp rules.SpellEntry) SpellOption {
	o := SpellOption{Spell: sp, Economy: spellEconomy(sp.CastingTime)}
	if sp.Level > 0 {
		o.Slots = slotChoices(d, u, sp.Level)
	}
	o.Reason = spellReason(turn, o, sp)
	o.Enabled = o.Reason == nil
	return o
}

// spellReason is why the spell cannot be cast now, or nil. The first reason
// that applies wins: what the spell is, then the economy, then the slots.
func spellReason(turn TurnState, o SpellOption, sp rules.SpellEntry) *Reason {
	switch {
	case o.Economy == "":
		return &Reason{Code: ReasonTooLong}
	case o.Economy == rules.EconomyReaction && sp.Key == shieldKey:
		return &Reason{Code: ReasonReactionOnlyWhenHit}
	case o.Economy == rules.EconomyReaction:
		return &Reason{Code: ReasonReactionOnly}
	}
	if eo := economyOption(o.Economy, turn); !eo.Enabled {
		return eo.Reason
	}
	if !o.FeatureSummon && SpellLimited(turn, o.Economy, sp.Level) {
		return &Reason{Code: ReasonBonusActionSpellLimit}
	}
	if o.Grant != nil {
		return grantReason(o)
	}
	if sp.Level > 0 && len(o.Slots) == 0 {
		return &Reason{Code: ReasonNoSlot, MinLevel: sp.Level}
	}
	return nil
}

func actionOption(d rules.Derived, turn TurnState, u Usage, a rules.Action) ActionOption {
	o := ActionOption{Action: a, Option: economyOption(a.Economy, turn)}
	if a.Resource == "" {
		return o
	}
	left, _ := ResourceLeft(d, u, a.Resource)
	o.UsesLeft = left
	if o.Enabled && left < a.UnitCost() { // fewer uses than one use costs: the table's content may set a cost above 1
		o.Enabled = false
		o.Reason = &Reason{Code: ReasonNoUses, Recharge: rechargeOf(d, a.Resource)}
	} else if o.Enabled && a.Resource == actionSurgeResource && turn.ActionSurged {
		o.Enabled = false
		o.Reason = &Reason{Code: ReasonAlreadyUsedThisTurn}
	}
	return o
}

func rechargeOf(d rules.Derived, key string) string {
	for _, r := range d.Resources {
		if r.Key == key {
			return r.Recharge
		}
	}
	return ""
}

// countLight counts the attacks that match.
func countLight(attacks []rules.Attack, match func(rules.Attack) bool) int {
	n := 0
	for _, a := range attacks {
		if match(a) {
			n++
		}
	}
	return n
}
