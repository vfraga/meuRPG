package play

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/play/advantage"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// Advantage and disadvantage on the d20 rolls of a combat (SRD 5.1, "Advantage
// and Disadvantage"). The rules are package combat's (AttackMode, SaveMode); this
// file reads the combat for the facts they need (conditions, states, squares,
// sides, who sees whom), keeps the whole truth apart from what a viewer may read
// (RN-10, RN-20: a circumstance that rests on something the master hides counts in
// the mode and is never listed), and writes the sentence of each source.

// The state kinds of combatant_states.
const (
	stateRage     = "rage"
	stateMark     = "hunters_mark_target"
	stateDodging  = "dodging"
	stateReckless = "reckless"
)

// statesOf groups the states of a combat by the combatant that has them.
func statesOf(states []playdb.CombatantState) map[string][]playdb.CombatantState {
	out := map[string][]playdb.CombatantState{}
	for _, st := range states {
		out[st.CombatantID] = append(out[st.CombatantID], st)
	}
	return out
}

// hasState says whether the combatant is in a state of the kind.
func hasState(states map[string][]playdb.CombatantState, id, kind string) bool {
	return slices.ContainsFunc(states[id], func(s playdb.CombatantState) bool { return s.Kind == kind })
}

// creatureFacts is what the advantage rules read about a combatant. A rage whose
// benefits hold needs the sheet's armor (heavy armor cancels them): traits are the
// sheet's, or the zero value when the caller has no sheet at hand.
func creatureFacts(c playdb.Combatant, states map[string][]playdb.CombatantState, traits link.Traits) combat.Creature {
	attackDisadvantage, spares := effectAttackDisadvantage(states, c.ID)
	return combat.Creature{
		ID:              c.ID,
		Conditions:      c.Conditions,
		Dodging:         hasState(states, c.ID, stateDodging),
		Reckless:        hasState(states, c.ID, stateReckless),
		PackTactics:     traits.PackTactics,
		MagicResistance: traits.MagicResistance,
		// The sun itself (Sunlit) is the combat's fact, set where the combat is read; the traits are the sheet's.
		SunlightSensitivity: traits.SunlightSensitivity, SunlightWeakness: traits.SunlightWeakness,
		Evasion:     traits.Evasion,
		DangerSense: traits.DangerSense,
		Raging:      hasState(states, c.ID, stateRage) && !traits.HeavyArmor,
		// The effects that last (RN-22): exhaustion, Faerie Fire's outline, the advantage on saving
		// throws an effect gives and the disadvantage one gives the creature's attack rolls.
		Exhaustion: int(c.ExhaustionLevel), Outlined: hasEffect(states, c.ID, "spell:faerie-fire"), SaveAdvantage: effectSaveAdvantage(states, c.ID), CheckAdvantage: effectCheckAdvantage(states, c.ID),
		AttackDisadvantage: attackDisadvantage, AttackDisadvantageSpares: spares, AttackAdvantage: effectAttackAdvantage(states, c.ID),
		SaveDisadvantage:  abilitiesOfKind(states, c.ID, rules.ModifierSaveDisadvantage),
		CheckDisadvantage: abilitiesOfKind(states, c.ID, rules.ModifierCheckDisadvantage),
		ArmorUntrained:    traits.ArmorUntrained,
		TaggedSaves:       taggedSavesOf(traits.TaggedSaves),
	}
}

// taggedSavesOf are the racial advantages on saves against a kind of source the sheet has.
func taggedSavesOf(saves []link.TaggedSave) []combat.TaggedSave {
	out := make([]combat.TaggedSave, 0, len(saves))
	for _, t := range saves {
		out = append(out, combat.TaggedSave{Source: t.Source, Against: t.Against, Abilities: t.Abilities, Disadvantage: t.Disadvantage})
	}
	return out
}

// incapacitatedKeys are the conditions that make a creature unable to act or to
// see (SRD 5.1, Conditions): a hostile creature with one of them gives no
// disadvantage to a ranged attacker, and an ally with one gives no Pack Tactics.
var incapacitatedKeys = []string{"condition:incapacitated", conditionParalyzed, "condition:petrified", "condition:stunned", "condition:unconscious"}

// fighting says the combatant is in the fight (not defeated, not dismissed) and
// can act: it is not incapacitated.
func fighting(c playdb.Combatant) bool {
	return !c.Defeated && !c.Dismissed && !slices.ContainsFunc(incapacitatedKeys, func(k string) bool { return slices.Contains(c.Conditions, k) })
}

// attackShape is what an attack is, for the rules of "Ranged Attacks".
type attackShape struct {
	// Melee says a melee attack (a thrown weapon is one too, until it is thrown).
	Melee bool
	// Weapon says a weapon attack (an unarmed strike too), not a spell.
	Weapon bool
	// UsesStrength says it is made with Strength.
	UsesStrength bool
	// Ability is the ability the attack uses ("str", "dex"...), "" for a spell attack.
	Ability string
	// RangeFt is the reach or the normal range, LongRangeFt the long range.
	RangeFt, LongRangeFt int
	// Thrown says a melee weapon that has a throwing range.
	Thrown bool
	// MeleeReachFt is how far it is a melee attack: up to there a thrown weapon is not
	// thrown (5 ft, or the longer reach a weapon rule gives it).
	MeleeReachFt int
}

// rangedAt says the attack is made at range: a ranged one, or a thrown weapon used on
// a target beyond its melee reach (SRD 5.1, "Ranged Attacks"). known says dist is a real distance
// (both on the map, not theatre of the mind): without it a thrown weapon is in melee.
func (s attackShape) rangedAt(dist int32, known bool) bool {
	return !s.Melee || (s.Thrown && known && dist > conv.Int32(s.MeleeReachFt))
}

// physical says the attack is made with Strength or Dexterity.
func (s attackShape) physical() bool { return combat.ArmorSpoilsAbility(s.Ability) }

// shapeOfAttack reads a sheet's attack. A spell attack with a range beyond touch is
// a ranged attack; a touch spell is melee.
func shapeOfAttack(a link.Attack) attackShape {
	shape := attackShape{
		Melee: a.Melee, Weapon: a.Weapon || (!a.Spell && !a.Save), UsesStrength: a.Ability == "str", Ability: a.Ability,
		RangeFt: a.RangeFt, LongRangeFt: a.LongRangeFt, MeleeReachFt: attackMeleeReach(a),
	}
	switch {
	case a.Spell:
		shape.Melee = a.RangeFt <= meleeReachFt
		shape.Weapon = false
	case a.Melee && a.LongRangeFt > shape.MeleeReachFt:
		shape.Thrown = true
	}
	if shape.Melee && !shape.Thrown {
		shape.RangeFt = max(shape.RangeFt, shape.MeleeReachFt)
	}
	return shape
}

// shapeOfSpell reads a spell's attack roll: melee when it is a touch, else ranged
// with the spell's range.
func shapeOfSpell(sp link.Spell) attackShape {
	return attackShape{Melee: sp.AttackType == "melee", RangeFt: sp.RangeFt}
}

// modeFacts is the combat as the advantage rules read it.
type modeFacts struct {
	cs      []playdb.Combatant
	states  map[string][]playdb.CombatantState
	theatre bool
	// hiding are the creatures hidden from others and the helps that hold (combat_hide.go,
	// combat_help.go): a creature that has not noticed its hider cannot see it, and the
	// attack of the creature an ally helped has advantage.
	hiding []playdb.CombatHiding
	helps  []playdb.CombatHelp
	// sight is the fog of the combat's map, nil without one.
	sight *fogSight
	// enc and surprised are the combat and who is surprised now, for the ambush effects.
	enc       playdb.Encounter
	surprised []string
	// down are the player's characters at 0 hit points: unconscious for an attack against them.
	down []string
	// sun is the combat's sunlight, off by default (Sunlight Sensitivity and Weakness).
	sun sunlight
}

// modeResult is the mode of one roll: the truth, and what a viewer may read.
type modeResult struct {
	// Mode is the mode the roll has; Sources the circumstances behind it.
	Mode    combat.RollMode
	Sources []combat.Source
	// Shown are the sources the viewer may read, written in Portuguese. A source
	// that rests on something the viewer does not see is in Mode and not in Shown.
	Shown []*playv1.AdvantageSource
	// CriticalOnHit says a hit is a critical hit by the target's condition.
	CriticalOnHit bool
	// Hide says which of Sources the viewer reads as "Outra fonte" (RN-10); nil when they read them all.
	Hide func(combat.Source) bool
}

// attackMode works out the mode of an attack roll. v is the viewer who reads the
// sources: the facts it does not see count in the mode and are left out of Shown.
func (f modeFacts) attackMode(attacker, target playdb.Combatant, attackerTraits link.Traits, shape attackShape, ownTurn bool, v combatViewer, names func(string) string) modeResult {
	truth := f.scene(attacker, target, attackerTraits, shape, ownTurn, nil)
	out := modeResult{Mode: combat.Resolve(combat.AttackMode(truth).Sources), Sources: combat.AttackMode(truth).Sources, CriticalOnHit: combat.AttackMode(truth).CriticalOnHit}
	seen := f.scene(attacker, target, attackerTraits, shape, ownTurn, &v)
	shown := combat.AttackMode(seen)
	// The viewer reads their own side's facts and the public ones; the critical hit
	// of a paralyzed or unconscious target is a condition they see on a target they
	// picked.
	self := v.master || v.owns(attacker)
	ctx := advantage.TextContext{Self: self, Names: names, Attacker: attacker.Label, Target: target.Label, AllyLabel: f.allyLabelNear(attacker, target)}
	if h := f.helperOf(attacker, target); h != nil && (self || v.sees(*h)) {
		ctx.Helper = h.Label
	}
	// What an effect the master hides gives is "Outra fonte" to a player (RN-10).
	hide := f.hiddenFrom(v, attacker, target)
	out.Hide = hide
	for _, src := range shown.Sources {
		out.Shown = append(out.Shown, advantage.SourceProto(src, ctx, hide))
	}
	out.Shown = append(out.Shown, bonusSources(rollBonusesOf(f.states, attacker.ID, rules.RollAppliesAttack), names, v.master, true)...)
	return out
}

// scene fills the rules' AttackScene. view is the viewer whose sight limits the
// facts that rest on other creatures (an enemy near the attacker, an ally near the
// target); nil is the whole truth.
func (f modeFacts) scene(attacker, target playdb.Combatant, traits link.Traits, shape attackShape, ownTurn bool, view *combatViewer) combat.AttackScene {
	dist, known := f.distanceBetween(attacker, target)
	known = known && !f.theatre
	ranged := shape.rangedAt(dist, known)
	reach, long := shape.RangeFt, shape.LongRangeFt
	if shape.Thrown && !ranged {
		reach, long = shape.MeleeReachFt, 0
	}
	s := combat.AttackScene{
		Attacker: f.sun.facts(creatureFacts(attacker, f.states, traits), attacker), Target: f.sun.facts(creatureFacts(target, f.states, link.Traits{}), target),
		Ranged: ranged, StrengthMelee: shape.Weapon && shape.Melee && !ranged && shape.UsesStrength, OwnTurn: ownTurn,
		DistanceKnown: known, DistanceFt: int(dist), ReachFt: reach, LongRangeFt: long,
		// A table feat's ranged_attack effect: the weapon keeps its aim at long range.
		NoLongRangeDisadvantage:   shape.Weapon && traits.RangedWeaponNoLongRangeDisadvantage,
		StrengthOrDexterityWeapon: shape.Weapon && shape.physical(),
	}
	if slices.Contains(f.down, target.ID) {
		s.Target.Conditions = append(slices.Clone(s.Target.Conditions), unconscious)
	}
	s.Helped = f.helperOf(attacker, target) != nil
	s.AttackerUnseen = f.unseen(target, attacker)
	s.TargetUnseen = f.unseen(attacker, target)
	s.AllyNearTarget = known && traits.PackTactics && f.allyNear(attacker, target, view)
	s.HostileNearAttacker = known && ranged && !ignoresHostileNearby(shape, traits) && f.hostileNear(attacker, view)
	f.ambush(&s, target, traits)
	return s
}

// ignoresHostileNearby says a table feature's ignore_hostile_nearby flag lifts the disadvantage of
// a ranged weapon attack (not a spell's).
func ignoresHostileNearby(shape attackShape, traits link.Traits) bool {
	return shape.Weapon && traits.HasFlag(rules.FlagIgnoreHostileNearby)
}

// ambush fills what the attacker's ambush effects ask of the target: that it has not had its
// turn yet (advantage) and that it is surprised (a hit is critical).
func (f modeFacts) ambush(s *combat.AttackScene, target playdb.Combatant, traits link.Traits) {
	if traits.UnactedAdvantage != "" && combat.HasNotActed(f.enc.Status == statusActive, int(f.enc.Round), currentOrder(f.enc, f.cs), int(target.OrderIndex)) {
		s.UnactedAdvantage = traits.UnactedAdvantage
	}
	s.SurprisedCritical = traits.SurprisedCritical != "" && slices.Contains(f.surprised, target.ID)
}

// unseen says the observer cannot see the subject: a fog map hides it from the
// observer's player. The master's "hidden" mark is no in-fiction fact (it only keeps
// a combatant from the players' screens), so it changes no roll. A player's character
// is never unseen, and an observer that is an NPC sees every player's character.
func (f modeFacts) unseen(observer, subject playdb.Combatant) bool {
	if slices.ContainsFunc(f.hiding, func(h playdb.CombatHiding) bool {
		return h.HiderID == subject.ID && h.ObserverID == observer.ID && !h.Noticed
	}) {
		return true // a hider the observer has not noticed (SRD 5.1, "Hiding")
	}
	if subject.Kind != kindNPC || subject.Side == observer.Side {
		return false
	}
	if f.sight != nil && observer.UserID != nil && observer.Kind == kindPlayer {
		return !f.sight.seesNPC(*observer.UserID, subject)
	}
	return false
}

// helperOf is the combatant whose Help holds for the attacker against the target: the
// Help of an ally aimed at that target, not used and not past its helper's next turn.
func (f modeFacts) helperOf(attacker, target playdb.Combatant) *playdb.Combatant {
	for _, h := range f.helps {
		if h.Kind != helpAttack || !helpsAlly(h, attacker) || conv.OrZero(h.TargetID) != target.ID {
			continue
		}
		if helper, ok := helpHelper(f.cs, h); ok {
			return &helper
		}
	}
	return nil
}

// distanceBetween is the distance in feet and whether both have a square.
func (f modeFacts) distanceBetween(a, b playdb.Combatant) (int32, bool) {
	return distanceFt(a, b)
}

// allyNear says an ally of the attacker, not incapacitated, stands within 5 ft of
// the target (Pack Tactics). With a view, only an ally the viewer sees counts.
func (f modeFacts) allyNear(attacker, target playdb.Combatant, view *combatViewer) bool {
	return f.allyOf(attacker, target, view) != nil
}

func (f modeFacts) allyOf(attacker, target playdb.Combatant, view *combatViewer) *playdb.Combatant {
	for i := range f.cs {
		c := f.cs[i]
		if c.ID == attacker.ID || c.ID == target.ID || c.Side != attacker.Side || !fighting(c) || (view != nil && !view.sees(c)) {
			continue
		}
		if d, ok := distanceFt(c, target); ok && d <= meleeReachFt {
			return &f.cs[i]
		}
	}
	return nil
}

func (f modeFacts) allyLabelNear(attacker, target playdb.Combatant) string {
	if c := f.allyOf(attacker, target, nil); c != nil {
		return c.Label
	}
	return ""
}

// hostileNear says a hostile creature that can see the attacker and is not
// incapacitated stands within 5 ft of it (SRD, "Ranged Attacks"). A creature the
// attacker has made invisible cannot see it, and a blinded one cannot. With a view,
// only a creature the viewer sees counts.
func (f modeFacts) hostileNear(attacker playdb.Combatant, view *combatViewer) bool {
	if slices.Contains(attacker.Conditions, "condition:invisible") {
		return false
	}
	for _, c := range f.cs {
		if c.ID == attacker.ID || c.Side == attacker.Side || !fighting(c) || slices.Contains(c.Conditions, "condition:blinded") || (view != nil && !view.sees(c)) {
			continue
		}
		if d, ok := distanceFt(c, attacker); ok && d <= meleeReachFt {
			return true
		}
	}
	return false
}

// ---- reading the combat ----

// readStates reads the states of the combat.
func (s *Service) readStates(ctx context.Context, tx pgx.Tx, encID string) (map[string][]playdb.CombatantState, error) {
	rows, err := s.queriesIn(tx).ListCombatantStates(ctx, encID)
	if err != nil {
		return nil, fmt.Errorf("list the states: %w", err)
	}
	return statesOf(rows), nil
}

// d20Roll is a d20 roll with the mode: the faces rolled, the die that counts and the
// total.
type d20Roll struct {
	// Faces are the d20 rolled or typed: one, or two with advantage or disadvantage.
	Faces []int
	// Index is the die that counts.
	Index int
	// Total is the number the die counts as plus the modifier.
	Total    int
	Physical bool
	// CountedValue is the number a floor made the die that counts count as.
	// It is 0 when the die counts as it came up.
	// It is a number, unlike the Index of a pair.
	CountedValue int
	// CountedBy is the content key of the feature that gave the floor.
	CountedBy string
}

// Face is the die that counts as it came up: what the roll shows.
func (r d20Roll) Face() int { return r.Faces[r.Index] }

// Value is the number the die that counts counts as: the face, or the floor that raised it.
// Every rule that reads the die reads this one.
func (r d20Roll) Value() int {
	if r.CountedValue > 0 {
		return r.CountedValue
	}
	return r.Face()
}

// otherFace is the die that does not count, 0 when there was only one.
func (r d20Roll) otherFace() int {
	if len(r.Faces) < pairDice {
		return 0
	}
	return r.Faces[1-r.Index]
}

// typedD20Faces checks the faces of a physical d20 pair: one or two, 1 to 20 each.
func typedD20Faces(raw []int32) ([]int, error) {
	if len(raw) < 1 || len(raw) > 2 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("set roll_in_app, d20_face or d20_faces"))
	}
	out := make([]int, len(raw))
	for i, f := range raw {
		if f < 1 || f > 20 {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("d20_faces must be 1 to 20 each"))
		}
		out[i] = int(f)
	}
	return out, nil
}

// d20With rolls the d20 of a roll with the mode: one die, or two with advantage or
// disadvantage (SRD 5.1). In the app the server rolls them; with physical dice the
// player typed as many faces as the mode needs, and a different number is refused.
// The die that counts is the higher with advantage and the lower with disadvantage.
func (s *Service) d20With(in rollInput, modifier int, mode combat.RollMode) (d20Roll, error) {
	n := mode.Dice()
	var faces []int
	switch {
	case in.inApp:
		res, err := dice.Roll(s.roller, dice.Expr{Count: n, Sides: d20Sides})
		if err != nil {
			return d20Roll{}, fmt.Errorf("roll the d20: %w", err)
		}
		faces = res.Faces
	case len(in.typedFaces) > 0:
		faces = in.typedFaces
	default:
		faces = []int{in.typed}
	}
	if len(faces) != n {
		return d20Roll{}, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("the roll takes %d d20: type %d face(s) in d20_faces", n, n))
	}
	for _, f := range faces {
		if f < 1 || f > 20 {
			return d20Roll{}, connect.NewError(connect.CodeInvalidArgument, errors.New("d20_face must be 1 to 20"))
		}
	}
	idx := mode.Pick(faces)
	return d20Roll{Faces: faces, Index: idx, Total: faces[idx] + modifier, Physical: !in.inApp}, nil
}

// d20Faces are the dice of an attack roll in the order they were rolled.
func (ev actionEvent) d20Faces() []int32 {
	switch {
	case ev.D20B == 0:
		return []int32{ev.D20}
	case ev.Counted == 0:
		return []int32{ev.D20, ev.D20B}
	}
	return []int32{ev.D20B, ev.D20}
}

// d20Proto is the d20 of an attack event as a DiceRoll, with the die Lucky rolled again.
func d20Proto(ev actionEvent) *playv1.DiceRoll {
	roll := diceRoll(clamp32(len(ev.d20Faces()), 1, 2), 20, ev.d20Faces(), ev.Modifier, ev.Total, ev.Physical)
	roll.CountedIndex = ev.Counted
	roll.Rerolled = strokeRerolls(ev.Lucky, ev.Stroke)
	return ev.mark(roll)
}

// turnModes is what GetTurnOptions needs to write the mode of each attack against
// each target: the combatant's sheet and the combat as the rules read it.
type turnModes struct {
	who     playdb.Combatant
	sheet   link.Sheet
	facts   modeFacts
	viewer  combatViewer
	names   func(string) string
	ownTurn bool
}

// turnModes reads the combat for the modes of a combatant's attacks.
func (s *Service) turnModes(ctx context.Context, campaignID string, d *encounterData, sight *fogSight, v combatViewer, who playdb.Combatant) (turnModes, error) {
	sheet, err := s.sheetOf(ctx, nil, campaignID, who)
	if err != nil {
		return turnModes{}, err
	}
	states, err := s.readStatesAt(ctx, nil, d.enc.ID, &v)
	if err != nil {
		return turnModes{}, err
	}
	hiding, helps, err := s.contestFacts(ctx, s.queries, d.enc, d.cs)
	if err != nil {
		return turnModes{}, err
	}
	surprised, err := s.surprisedFacts(ctx, nil, campaignID, s.queries, d.enc, d.cs)
	if err != nil {
		return turnModes{}, err
	}
	down, err := s.downCharacters(ctx, nil, campaignID, d.cs)
	if err != nil {
		return turnModes{}, err
	}
	sun, err := s.sunlightOf(ctx, s.queries, d.enc)
	if err != nil {
		return turnModes{}, err
	}
	return turnModes{
		who: who, sheet: sheet, viewer: v, names: s.namesFor(ctx, campaignID), ownTurn: actsNow(d.enc, who),
		facts: modeFacts{cs: d.cs, states: states, theatre: isTheatre(d.enc), sight: sight, hiding: hiding, helps: helps, enc: d.enc, surprised: surprised, down: down, sun: sun},
	}, nil
}

// annotate fills the mode, the sources and the critical hit of each target of an
// attack. The mode is the server's suggestion: the one RollAttack takes unless the
// caller changes it.
func (t turnModes) annotate(attackKey string, targets []*playv1.TargetInReach) {
	i := slices.IndexFunc(t.sheet.Attacks, func(a link.Attack) bool { return a.Key == attackKey })
	if i < 0 {
		t.annotateShape(nil, targets)
		return
	}
	shape := shapeOfAttack(t.sheet.Attacks[i])
	t.annotateShape(&shape, targets)
}

// dropIgnoredCover takes off an attack's targets the half and three-quarters cover the
// attacker's own feature ignores for it (a table feat's ranged_attack effect), as
// RollAttack does: a thrown weapon only on the targets it is thrown at, beyond its melee reach.
func (t turnModes) dropIgnoredCover(attackKey string, targets []*playv1.TargetInReach) {
	i := slices.IndexFunc(t.sheet.Attacks, func(a link.Attack) bool { return a.Key == attackKey })
	if i < 0 {
		return
	}
	shape := shapeOfAttack(t.sheet.Attacks[i])
	for _, tg := range targets {
		known := tg.DistanceFt != nil && !t.facts.theatre
		if ignoresCover(t.sheet.Traits, shape, shape.rangedAt(tg.GetDistanceFt(), known)) {
			clearPartialCover(tg)
		}
	}
}

// annotateShape is annotate for an attack of a known shape; a nil shape leaves the
// targets at the normal mode. A spell attack uses it with the shape CastSpell rolls
// with (shapeOfSpell), so the cast sheet's suggestion is the mode the cast takes.
func (t turnModes) annotateShape(shape *attackShape, targets []*playv1.TargetInReach) {
	for _, tg := range targets {
		tg.RollMode = playv1.RollMode_ROLL_MODE_NORMAL
		if shape == nil {
			continue
		}
		j := slices.IndexFunc(t.facts.cs, func(c playdb.Combatant) bool { return c.ID == tg.GetCombatantId() })
		if j < 0 {
			continue
		}
		r := t.facts.attackMode(t.who, t.facts.cs[j], t.sheet.Traits, *shape, t.ownTurn, t.viewer, t.names)
		tg.RollMode, tg.Sources, tg.CriticalOnHit = advantage.ModeToProto[r.Mode], r.Shown, r.CriticalOnHit
	}
}

// spellModes is what the d20 rolls of a cast need, and keeps the circumstances of each
// target's roll for the answer.
type spellModes struct {
	in attackModeInputs
	v  combatViewer
	// reader is who the sources shown are for: v, unless a held roll's answer is rendered for another.
	reader combatViewer
	ask    modeAsk
	names  func(string) string
	traits link.Traits
	// attack and save are the circumstances by target, as the caller may read them.
	attack, save map[string][]*playv1.AdvantageSource
	// pre is the d20 of a spell attack settled before the cast spent anything, when the caster's
	// player held a Bardic Inspiration die (inspiration_held.go); nil otherwise.
	pre *spellAttackPre
	// spend and key are the request's spend_favor and its idempotency key as parsed (parseKey: the one every hold, event and prompt keeps); favor is what the Inspiration did to the cast's spell attack
	// (favor_spend.go), decided in the first of the two places that work the attack's mode out.
	spend bool
	key   string
	favor favorUse
}

// spellModes reads the combat for the d20 rolls of a cast.
func (s *Service) spellModes(ctx context.Context, c *combatTx, m authz.Membership, v combatViewer, cs []playdb.Combatant, caster playdb.Combatant, req *playv1.CastSpellRequest, key string) (*spellModes, error) {
	in, err := s.readModeInputs(ctx, c, cs, &v)
	if err != nil {
		return nil, err
	}
	names, err := s.namerFor(ctx, c.tx, m.CampaignID)
	if err != nil {
		return nil, err
	}
	sheet, err := s.sheetOf(ctx, c.tx, m.CampaignID, caster)
	if err != nil {
		return nil, err
	}
	reader := v
	if who, ok := heldCallerOf(ctx, m.CampaignID); ok {
		reader = c.viewer(who, cs)
	}
	return &spellModes{
		in: in, v: v, reader: reader, names: names, traits: sheet.Traits, ask: modeAsk{want: req.GetRollMode(), reason: req.GetModeReason()},
		spend: req.GetSpendFavor(), key: key,
		attack: map[string][]*playv1.AdvantageSource{}, save: map[string][]*playv1.AdvantageSource{},
	}, nil
}

// attach puts the circumstances of each roll in the answer to the cast. A retry of a
// cast the closure never ran for has none.
func (m *spellModes) attach(cast *playv1.SpellCast) {
	if m == nil || cast == nil {
		return
	}
	for _, t := range cast.GetTargets() {
		if sources := m.attack[t.GetCombatantId()]; len(sources) > 0 && t.GetAttackRoll() != nil {
			t.AttackSources = sources
		}
		if sources := m.save[t.GetCombatantId()]; len(sources) > 0 && t.GetSave().GetRoll() != nil {
			t.Save.Sources = sources
		}
	}
}

// The d20 and the pair of them a roll with advantage or disadvantage takes.
const pairDice = 2
