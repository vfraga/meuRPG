package play

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"slices"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// The damage of a hit by parts (PM-06b, SRD 5.1, "Damage Rolls"): the weapon's
// dice, the extras the player may add (Sneak Attack, Divine Smite, Hunter's Mark,
// Colossus Slayer) and the automatic lines (Rage, Improved Divine Smite). The rules
// and the conditions are package combat's; this file reads the combat for the facts,
// stores the parts on the pending damage when the attack hits, rolls them when the
// damage is rolled (the dice of every part double on a critical hit, a fixed number
// never does) and keeps what each rolled for the log.
//
// A pending damage with no parts (a spell's, a trap's, a creature's attack that has
// no extra) is one roll.

// maxIgnoredSources is how many sources of resistance the master may leave out of one damage.
const maxIgnoredSources = 20

// The kinds of a stored part.
const (
	partWeapon = "weapon"
	partExtra  = "extra"
	partAuto   = "auto"
)

// partRecord is a line of the damage as the hit left it, in the pending damage's
// parts column.
type partRecord struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
	// Count d Sides are the dice before a critical hit changes them; Flat is the number
	// added without rolling.
	Count      int32  `json:"count,omitempty"`
	Sides      int32  `json:"sides,omitempty"`
	Flat       int32  `json:"flat,omitempty"`
	DamageType string `json:"damage_type,omitempty"`
	Source     string `json:"source,omitempty"`
	// Choosable, Selected and Available: the player may mark it; it comes marked; its
	// condition holds. Reason says why, in Portuguese.
	Choosable bool   `json:"choosable,omitempty"`
	Selected  bool   `json:"selected,omitempty"`
	Available bool   `json:"available,omitempty"`
	Reason    string `json:"reason,omitempty"`
	NeedsSlot bool   `json:"needs_slot,omitempty"`
	// Conditional: the dice count only when a fact about the target holds, which the
	// player is not told (the extra die of Divine Smite against an undead or a fiend).
	Conditional bool `json:"conditional,omitempty"`
	// Reroll: Great Weapon Fighting rerolls this part's 1s and 2s.
	Reroll bool   `json:"reroll,omitempty"`
	Note   string `json:"note,omitempty"`
	// SlotLevel and Pact are the slot a Divine Smite spent, once rolled.
	SlotLevel int32 `json:"slot_level,omitempty"`
	Pact      bool  `json:"pact,omitempty"`
	// Removed: the master took it out.
	Removed bool `json:"removed,omitempty"`
	// Resource is the resource a table maneuver spends (one use) when this extra is
	// rolled; Rider is the line the master reads for it.
	Resource string `json:"resource,omitempty"`
	Rider    string `json:"rider,omitempty"`
	// ResourceLeft is how many uses of Resource the attacker had left when the hit was
	// rolled (the superiority dice left), so the damage step can say what one costs.
	ResourceLeft int32 `json:"resource_left,omitempty"`
	// Table names the engine's effect type that made a part of the table's content ("extra_damage"):
	// it is not a maneuver, so the one-maneuver-per-attack limit does not count it. Once says the
	// part is once per turn, remembered by Key in the attacker's table_once_turns.
	Table string `json:"table,omitempty"`
	Once  bool   `json:"once,omitempty"`
	// NoCrit: a critical hit does not double this part's dice (an extra_damage that says so).
	NoCrit bool `json:"no_crit,omitempty"`
}

// partRoll is what a part rolled, in the pending damage's part_rolls column and in the
// event of the roll.
type partRoll struct {
	Key        string  `json:"key"`
	Label      string  `json:"label,omitempty"`
	Count      int32   `json:"count,omitempty"`
	Sides      int32   `json:"sides,omitempty"`
	Faces      []int32 `json:"faces,omitempty"`
	Flat       int32   `json:"flat,omitempty"`
	Fixed      int32   `json:"fixed,omitempty"` // what the table's critical rule added without rolling
	Sum        int32   `json:"sum,omitempty"`
	Counted    bool    `json:"counted,omitempty"`
	Physical   bool    `json:"physical,omitempty"`
	DamageType string  `json:"damage_type,omitempty"`
	// Conditional is the part's, kept for the audience of the roll.
	Conditional bool       `json:"conditional,omitempty"`
	Rerolled    []rerolled `json:"rerolled,omitempty"`
	// Resource and Rider are a table maneuver's: the resource it spent and the line only
	// the master reads.
	Resource string `json:"resource,omitempty"`
	Rider    string `json:"rider,omitempty"`
}

type rerolled struct {
	Index int32 `json:"i"`
	From  int32 `json:"from"`
	To    int32 `json:"to"`
}

// stepGroup is the damage of one type and the modifiers that covered it: what the
// steps are worked out from again when the master leaves a source out.
type stepGroup struct {
	DamageType string `json:"damage_type"`
	// Before is the damage of the type before the modifiers.
	Before int32             `json:"before"`
	Mods   []combat.Modifier `json:"mods"`
	// From says where the damage came from: the steps are worked out with it again when
	// the master leaves a source out (a modifier can hold for a nonmagical attack only).
	From combat.Origin `json:"origin,omitzero"`
}

func readParts(raw []byte) []partRecord {
	var out []partRecord
	if len(raw) == 0 || json.Unmarshal(raw, &out) != nil {
		return nil
	}
	return out
}

// rewritesParts says a roll stores a list of parts that is not the one the hit offered (the choice of a
// Divine Smite or a maneuver changes it), which is then kept in the roll's event for the undo.
func rewritesParts(offered []byte, rolled []partRecord) bool {
	return len(rolled) > 0 && !reflect.DeepEqual(readParts(offered), rolled)
}

func readPartRolls(raw []byte) []partRoll {
	var out []partRoll
	if len(raw) == 0 || json.Unmarshal(raw, &out) != nil {
		return nil
	}
	return out
}

func readStepGroups(raw []byte) []stepGroup {
	var out []stepGroup
	if len(raw) == 0 || json.Unmarshal(raw, &out) != nil {
		return nil
	}
	return out
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("[]")
	}
	return b
}

// turnKey names the turn in progress: Sneak Attack and Colossus Slayer are once per
// turn, and a turn is a round and the combatant (or joint group) on it.
func turnKey(e playdb.Encounter) string {
	return fmt.Sprintf("%d:%s", e.Round, conv.OrZero(e.CurrentCombatantID))
}

// stateWordPT is the word the players read for how hurt a combatant is.
func stateWordPT(c playdb.Combatant) string {
	switch stateOf(c) {
	case playv1.CombatantState_COMBATANT_STATE_UNHURT:
		return "Ileso"
	case playv1.CombatantState_COMBATANT_STATE_HURT:
		return "Ferido"
	case playv1.CombatantState_COMBATANT_STATE_BADLY_HURT:
		return "Muito ferido"
	}
	return ""
}

// automaticLines are the lines the server adds with no choice. It is the seam where
// the automatic lines of the other branches join: Dueling's +2 and Brutal Critical
// append an Extra with Auto set here when they land on main.
func automaticLines(scene combat.ExtraScene) []combat.Extra {
	return combat.Automatic(scene)
}

// hitFacts is everything the extras of a hit read.
type hitFacts struct {
	attacker, target playdb.Combatant
	sheet            link.Sheet
	attack           link.Attack
	weaponBonus      int
	mode             combat.RollMode
	cs               []playdb.Combatant
	states           map[string][]playdb.CombatantState
	enc              playdb.Encounter
	viewer           combatViewer
	freeSlots        bool
	disadvantaged    bool // some source gives the attack disadvantage, canceled or not
	targetHurt       bool
	names            func(string) string
	// usesLeft are the uses left of the attacker's resources, for the table maneuvers.
	usesLeft map[string]int32
	// opportunity says an opportunity attack: a melee attack made from the square the mover left.
	opportunity bool
	// planned are the damage parts of a creature's attack after its first, from the stat block's plan
	// (plannedParts): each rolls with its own type.
	planned []partRecord
}

// scene is the rules' ExtraScene for the hit.
func (h hitFacts) scene() combat.ExtraScene {
	t := h.sheet.Traits
	a := h.attack
	key := turnKey(h.enc)
	// A thrown melee weapon thrown at range (beyond its melee reach) is a ranged weapon attack: Rage and the
	// melee extras do not apply to it; used in melee (within its melee reach) it is still a melee attack.
	melee := a.Melee && !h.thrownAtRange()
	return combat.ExtraScene{
		Weapon: a.Weapon, Melee: melee, Finesse: a.Finesse, Ranged: a.Weapon && !melee, UsesStrength: a.Ability == "str",
		WeaponName: a.Name, Mode: h.mode, WithoutMap: isTheatre(h.enc), EnemyNearTarget: h.enemyNearTarget(), DisadvantageSource: h.disadvantaged,
		SneakAttackDice: t.SneakAttackDice, SneakUsed: conv.OrZero(h.attacker.SneakAttackTurn) == key,
		DivineSmite: t.DivineSmite, SmiteSlotFree: h.freeSlots, ImprovedDivineSmite: t.ImprovedDivineSmite,
		HuntersMarkKnown: t.HuntersMark, HuntersMarkActive: conv.OrZero(h.attacker.ConcentrationSpell) == "spell:hunters-mark",
		TargetMarked:   h.targetMarked(),
		ColossusSlayer: t.ColossusSlayer, ColossusUsed: conv.OrZero(h.attacker.ColossusSlayerTurn) == key,
		TargetHurt: h.targetHurt, TargetState: stateWordPT(h.target), TargetLabel: h.target.Label,
		Raging: hasState(h.states, h.attacker.ID, stateRage) && !t.HeavyArmor, BarbarianLevel: t.BarbarianLevel,
	}
}

// thrownAtRange says the attack is a melee weapon with a throwing range used on a target
// farther than melee reach (SRD 5.1, "Ranged Attacks": it is then a ranged attack).
func (h hitFacts) thrownAtRange() bool {
	return thrownAtRange(h.enc, h.attack, h.attacker, h.target, h.opportunity)
}

// thrownAtRange is hitFacts.thrownAtRange for any attack: an opportunity attack is a melee
// attack whatever the distance, and theatre of the mind has no distances.
func thrownAtRange(enc playdb.Encounter, attack link.Attack, attacker, target playdb.Combatant, opportunity bool) bool {
	if opportunity || isTheatre(enc) {
		return false
	}
	shape := shapeOfAttack(attack)
	d, ok := distanceFt(attacker, target)
	return shape.Thrown && ok && shape.rangedAt(d, true)
}

// targetMarked says the attacker's Hunter's Mark is on the target.
func (h hitFacts) targetMarked() bool {
	return slices.ContainsFunc(h.states[h.target.ID], func(s playdb.CombatantState) bool {
		return s.Kind == stateMark && conv.OrZero(s.SourceID) == h.attacker.ID
	})
}

// enemyNearTarget says a creature that is an enemy of the target, not the attacker,
// not incapacitated, stands within 5 ft of it (SRD 5.1, Rogue, Sneak Attack). A player
// counts only the creatures they see: the master's hidden one never explains why
// the extra holds or not (RN-10, RN-20).
func (h hitFacts) enemyNearTarget() bool {
	for _, c := range h.cs {
		if c.ID == h.attacker.ID || c.ID == h.target.ID || c.Side == h.target.Side || !fighting(c) || !h.viewer.sees(c) {
			continue
		}
		if d, ok := distanceFt(c, h.target); ok && d <= meleeReachFt {
			return true
		}
	}
	return false
}

// buildParts lists the parts of a hit, or nil when the hit is one roll (no extra, no
// automatic line, no rerolling style).
func (h hitFacts) buildParts() []partRecord {
	scene := h.scene()
	extras := combat.Offered(scene)
	autos := automaticLines(scene)
	reroll := h.sheet.Traits.GreatWeaponFighting && h.attack.Weapon && h.attack.Melee && h.attack.TwoHanded
	maneuvers := append(h.maneuverParts(), h.extraDamageParts()...)
	if len(extras) == 0 && len(autos) == 0 && !reroll && len(maneuvers) == 0 && len(h.planned) == 0 {
		return nil
	}
	weapon := partRecord{
		Key: partWeapon, Label: h.attack.Name, Kind: partWeapon, Count: clamp32(h.attack.DiceCount, 0, 100), Sides: clamp32(h.attack.DiceSides, 0, 100),
		Flat: clamp32(h.weaponBonus, -1000, 1000), DamageType: h.attack.DamageType, Reroll: reroll, Available: true,
	}
	if reroll {
		weapon.Note = "Combate com Armas Grandes: os 1 e 2 dos dados da arma são rerrolados"
	}
	out := []partRecord{weapon}
	for _, e := range extras {
		out = append(out, h.fromExtra(e, partExtra))
	}
	for _, e := range autos {
		out = append(out, h.fromExtra(e, partAuto))
	}
	out = append(out, h.planned...)
	return append(out, maneuvers...)
}

// maneuverParts lists, for a player's character, the table maneuvers whose die goes on the
// damage of a weapon attack that hit (a goad also makes the target save): one extra each, which the player may mark (only one
// per attack, checked when the damage is rolled) while the resource has a use.
func (h hitFacts) maneuverParts() []partRecord {
	if h.attacker.Kind != kindPlayer || !h.attack.Weapon {
		return nil
	}
	var out []partRecord
	for _, m := range h.sheet.Traits.Maneuvers {
		if m.Applies != rules.ManeuverDamage && m.Applies != rules.ManeuverGoad {
			continue
		}
		left := h.usesLeft[m.Resource]
		sp := partRecord{
			Key: m.Key, Label: m.NamePT, Kind: partExtra, Count: 1, Sides: clamp32(m.Sides, 0, 100), DamageType: h.attack.DamageType,
			Source: m.Key, Choosable: true, Available: left > 0, Resource: m.Resource, Rider: m.TextPT, ResourceLeft: max(left, 0),
		}
		if left <= 0 {
			sp.Reason = "Sem usos do recurso"
		}
		out = append(out, sp)
	}
	return out
}

// extraDamageParts lists, for a player's character, the damage the table's features add to a weapon hit
// (extra_damage): one choosable extra each, with the fixed damage and the dice the feature says, of its own
// damage type or the weapon's. The fixed part is never doubled by a critical hit, the dice are unless the
// feature says so. It is listed also when it does not hold, off with its reason (the size of a creature and
// the kind of weapon are no secret).
func (h hitFacts) extraDamageParts() []partRecord {
	if h.attacker.Kind != kindPlayer || !h.attack.Weapon {
		return nil
	}
	used := readTableOnce(h.attacker.TableOnceTurns)
	var out []partRecord
	for _, x := range h.sheet.Traits.ExtraDamages {
		sp := partRecord{
			Key: x.Key, Label: x.NamePT, Kind: partExtra, Flat: clamp32(x.Amount, 0, extraDamageMax), DamageType: cmp.Or(x.DamageType, h.attack.DamageType),
			Count: clamp32(x.DiceCount, 0, extraDamageMaxDice), Sides: clamp32(x.DiceSides, 0, 100), NoCrit: x.NoCritDoubling,
			Source: x.Key, Choosable: true, Rider: x.TextPT, Table: tableExtraDamage, Once: x.Frequency != rules.ExtraDamageEachHit, Available: true,
		}
		if x.Resource != "" {
			left := h.usesLeft[x.Resource]
			sp.Resource, sp.ResourceLeft = x.Resource, max(left, 0)
			if left <= 0 {
				sp.Available, sp.Reason = false, "Sem usos do recurso"
			}
		}
		if sp.Available {
			sp.Available, sp.Reason = h.extraDamageHolds(x, sp, used)
		}
		out = append(out, sp)
	}
	return out
}

// extraDamageHolds says an extra_damage can be added to this hit, and why not when it cannot.
func (h hitFacts) extraDamageHolds(x link.ExtraDamage, sp partRecord, used map[string]string) (bool, string) {
	ranged := !h.attack.Melee || h.thrownAtRange()
	switch {
	case sp.Once && used[x.Key] == turnKey(h.enc):
		return false, "Já usado neste turno"
	case x.Weapon == rules.ExtraDamageMeleeWeapon && ranged:
		return false, "Só vale com arma corpo a corpo"
	case x.Weapon == rules.ExtraDamageRangedWeapon && !ranged:
		return false, "Só vale com arma à distância"
	case x.Applies == rules.ExtraDamageLargerTarget && combat.SizeRank(h.target.Size) <= combat.SizeRank(h.attacker.Size):
		return false, "O alvo não é maior que você"
	}
	return true, ""
}

// tableExtraDamage is partRecord.Table of an extra_damage part; extraDamageMax bounds its flat damage.
const (
	tableExtraDamage = "extra_damage"
	extraDamageMax   = 999
	// extraDamageMaxDice is the most dice an extra_damage rolls (rules.maxExtraDice).
	extraDamageMaxDice = 20
)

// readTableOnce reads the once-per-turn marks of the table's extras: the turn each feature last used.
func readTableOnce(raw []byte) map[string]string {
	out := map[string]string{}
	if len(raw) == 0 || json.Unmarshal(raw, &out) != nil || out == nil {
		return map[string]string{}
	}
	return out
}

// writeTableOnce is the column's JSON for the marks.
func writeTableOnce(marks map[string]string) []byte {
	b, err := json.Marshal(marks)
	if err != nil {
		return []byte("{}")
	}
	return b
}

// partLabels are the names of the parts, in Portuguese: the game's short name.
func (h hitFacts) label(key string) string {
	switch key {
	case combat.ExtraSneakAttack:
		return h.names("feature:sneak-attack")
	case combat.ExtraDivineSmite:
		return h.names("feature:divine-smite")
	case combat.ExtraDivineSmiteExtra:
		return h.names("feature:divine-smite") + ": dado a mais"
	case combat.ExtraHuntersMark:
		return h.names("spell:hunters-mark")
	case combat.ExtraColossusSlayer:
		return "Matador de Colossos"
	case combat.AutoRage:
		return h.names("feature:rage")
	case combat.AutoImprovedDivineSmite:
		return h.names("feature:improved-divine-smite")
	}
	return key
}

var extraSources = map[string]string{
	combat.ExtraSneakAttack: "feature:sneak-attack", combat.ExtraDivineSmite: "feature:divine-smite",
	combat.ExtraDivineSmiteExtra: "feature:divine-smite", combat.ExtraHuntersMark: "spell:hunters-mark",
	combat.ExtraColossusSlayer: "feature:hunters-prey-colossus-slayer", combat.AutoRage: "feature:rage",
	combat.AutoImprovedDivineSmite: "feature:improved-divine-smite",
}

// fromExtra stores an extra. Sneak Attack, Hunter's Mark and Colossus Slayer are of the
// weapon's damage type; the smite is radiant.
func (h hitFacts) fromExtra(e combat.Extra, kind string) partRecord {
	typ := h.attack.DamageType
	switch e.Key {
	case combat.ExtraDivineSmite, combat.ExtraDivineSmiteExtra, combat.AutoImprovedDivineSmite:
		typ = "damage-type:radiant"
	case combat.AutoRage:
		typ = h.attack.DamageType // a bonus to the weapon's damage
	}
	return partRecord{
		Key: e.Key, Label: h.label(e.Key), Kind: kind, Count: clamp32(e.Count, 0, 100), Sides: clamp32(e.Sides, 0, 100), Flat: clamp32(e.Flat, 0, 100),
		DamageType: typ, Source: extraSources[e.Key], Choosable: !e.Auto && e.Key != combat.ExtraDivineSmiteExtra,
		Selected: e.Selected, Available: e.Available, Reason: e.Reason, NeedsSlot: e.NeedsSlot, Conditional: e.Conditional,
	}
}

// ---- the API's view of the parts ----

// partsProto converts the stored parts and what they rolled. A critical hit shows the
// dice as they are rolled (doubled, or once with the maximum added) and the fixed
// numbers as they are.
func partsProto(p playdb.PendingDamage, rule combat.CriticalRule, damageTypePT func(string) string) []*playv1.DamagePart {
	var out []*playv1.DamagePart
	for _, sp := range readParts(p.Parts) {
		count, fixed := partCriticalDice(sp, p.Critical, rule)
		if sp.Count == 0 {
			count, fixed = 0, 0
		}
		note := sp.Note
		if sp.Key == partWeapon && count > 0 && p.ExtraDice > 0 {
			count += int(p.ExtraDice)
			note = joinNote(note, fmt.Sprintf("inclui %dd%d do %s", p.ExtraDice, sp.Sides, extraDiceName(p.ExtraDice, p.SavageDice)))
		}
		out = append(out, &playv1.DamagePart{
			Key: sp.Key, LabelPt: sp.Label, DiceCount: clamp32(count, 0, 200), DiceSides: sp.Sides, Flat: sp.Flat + clamp32(fixed, 0, 10000),
			DamageTypeKey: sp.DamageType, DamageTypePt: damageTypePT(sp.DamageType), SourceKey: sp.Source,
			Choosable: sp.Choosable, Selected: sp.Selected && !sp.Removed, Available: sp.Available && !sp.Removed, ReasonPt: sp.Reason,
			Auto: sp.Kind == partAuto, Doubled: p.Critical && sp.Count > 0 && !sp.NoCrit, NeedsSlot: sp.NeedsSlot, NotePt: note, Maneuver: sp.Resource != "" && sp.Table == "", ResourceLeft: sp.ResourceLeft,
		})
	}
	return out
}

// partRollsProto converts what each part rolled. The extra die of Divine Smite is a line
// like the others for a player, counted or not: only the master reads whether it counts,
// so nothing tells a player what the target is (RN-10, RN-20).
func partRollsProto(p playdb.PendingDamage, master bool, damageTypePT func(string) string) []*playv1.DamagePartRoll {
	var out []*playv1.DamagePartRoll
	for _, r := range readPartRolls(p.PartRolls) {
		if r.Conditional && !master {
			r.Counted = true // a player reads the same lines whatever the target is
		}
		pr := partRollProto(r, damageTypePT)
		if master {
			pr.RiderPt = r.Rider
		}
		out = append(out, pr)
	}
	return out
}

func partRollProto(r partRoll, damageTypePT func(string) string) *playv1.DamagePartRoll {
	pr := &playv1.DamagePartRoll{
		PartKey: r.Key, LabelPt: r.Label, DiceCount: r.Count, DiceSides: r.Sides, Faces: r.Faces, Flat: r.Flat + r.Fixed, Sum: r.Sum,
		Counted: r.Counted, Physical: r.Physical, DamageTypeKey: r.DamageType,
	}
	for _, rr := range r.Rerolled {
		pr.Rerolled = append(pr.Rerolled, &playv1.Reroll{Index: rr.Index, From: rr.From, To: rr.To})
	}
	_ = damageTypePT
	return pr
}

// stepsProto writes the steps the modifiers take the damage through, in Portuguese.
func stepsProto(groups []stepGroup, ignored []string, names func(string) string, damageTypePT func(string) string) []*playv1.DamageStep {
	var out []*playv1.DamageStep
	for _, g := range groups {
		_, steps := combat.Adjust(int(g.Before), g.DamageType, g.Mods, ignored, g.From)
		for _, st := range steps {
			out = append(out, &playv1.DamageStep{
				Kind: stepKindProto(st.Kind), SourceKeys: st.Sources, LabelPt: stepLabel(st, names, damageTypePT),
				Before: clamp32(st.Before, 0, math.MaxInt32), After: clamp32(st.After, 0, math.MaxInt32), DamageTypeKey: st.DamageType,
			})
		}
		// A source the master left out is still listed, struck.
		for _, ig := range ignored {
			for _, m := range g.Mods {
				if m.Source == ig && m.Covers(g.DamageType, g.From) {
					out = append(out, &playv1.DamageStep{
						Kind: stepKindProto(m.Kind), SourceKeys: []string{m.Source}, LabelPt: stepLabel(combat.Step{Kind: m.Kind, Sources: []string{m.Source}, DamageType: g.DamageType, Qualifier: m.Qualifier}, names, damageTypePT),
						Before: g.Before, After: g.Before, DamageTypeKey: g.DamageType, Ignored: true,
					})
				}
			}
		}
	}
	return out
}

func stepKindProto(kind string) playv1.DamageStepKind {
	switch kind {
	case combat.StepResistance:
		return playv1.DamageStepKind_DAMAGE_STEP_KIND_RESISTANCE
	case combat.StepVulnerability:
		return playv1.DamageStepKind_DAMAGE_STEP_KIND_VULNERABILITY
	case combat.StepImmunity:
		return playv1.DamageStepKind_DAMAGE_STEP_KIND_IMMUNITY
	}
	return playv1.DamageStepKind_DAMAGE_STEP_KIND_UNSPECIFIED
}

// stepLabel writes a step as the screen reads it: "Resistência a fogo (tiefling)",
// "Vulnerabilidade a concussão (Esqueleto)".
func stepLabel(st combat.Step, names func(string) string, damageTypePT func(string) string) string {
	var head string
	switch st.Kind {
	case combat.StepResistance:
		head = "Resistência a "
	case combat.StepVulnerability:
		head = "Vulnerabilidade a "
	case combat.StepImmunity:
		head = "Imunidade a "
	}
	var sources []string
	for _, s := range st.Sources {
		sources = append(sources, sourceNamePT(s, names))
	}
	out := head + damageTypePT(st.DamageType)
	if st.Qualifier == combat.QualifierNonmagical {
		out += " de ataque não mágico"
	}
	if len(sources) > 0 {
		out += " (" + joinPT(sources) + ")"
	}
	return out
}

// joinPT joins words with commas and "e".
func joinPT(words []string) string {
	switch len(words) {
	case 0:
		return ""
	case 1:
		return words[0]
	}
	return fmt.Sprintf("%s e %s", joinWith(words[:len(words)-1], ", "), words[len(words)-1])
}

func joinWith(words []string, sep string) string {
	out := ""
	for i, w := range words {
		if i > 0 {
			out += sep
		}
		out += w
	}
	return out
}

// sourceNamePT is the name of a source of a resistance: the race for a racial trait
// ("tiefling"), "fúria", the creature's name for a stat block.
func sourceNamePT(source string, names func(string) string) string {
	switch source {
	case "feature:rage":
		return "fúria"
	case "trait:hellish-resistance":
		return "tiefling"
	case "trait:dwarven-resilience":
		return "anão"
	}
	if len(source) > 25 && source[:25] == "trait:draconic-ancestry-" {
		return "draconato"
	}
	if n := names(source); n != "" {
		return n
	}
	return source
}

// ---- rolling ----

// partsRoll is the result of rolling the parts of a damage.
type partsRoll struct {
	// parts are the parts as rolled: the player's marks applied.
	parts []partRecord
	rolls []partRoll
	// total is the sum of the parts that count, never below 0; byType is the same
	// for each damage type, for the steps.
	total  int
	byType map[string]int
	// adjust is what the answers of a held damage's windows do to the total a player reads.
	adjust func(int) int
	// faces are the weapon's faces, for the legacy fields of the event.
	physical bool
}

// extraChoice is what the player marked.
type extraChoice struct {
	chosen bool
	marks  map[string]*playv1.SelectedExtra
}

func choiceOf(req *playv1.RollDamageRequest) extraChoice {
	c := extraChoice{chosen: req.GetExtrasChosen() || len(req.GetSelectedExtras()) > 0, marks: map[string]*playv1.SelectedExtra{}}
	for _, m := range req.GetSelectedExtras() {
		c.marks[m.GetKey()] = m
	}
	return c
}

// rollPartsInput is what rolling the parts needs.
type rollPartsInput struct {
	p        playdb.PendingDamage
	parts    []partRecord
	rule     combat.CriticalRule
	choice   extraChoice
	in       rollInput
	typed    map[string]int
	target   link.Traits // the target's traits: its creature type
	selectOK func(part partRecord, mark *playv1.SelectedExtra) error
}

// rollParts rolls the damage of a hit by parts. The extras marked are checked by
// selectOK (the conditions hold now, a slot is free). In the app the server rolls
// every die; with physical dice the player typed one sum for each group of dice.
func (s *Service) rollParts(r rollPartsInput) (partsRoll, error) {
	out := partsRoll{byType: map[string]int{}, physical: !r.in.inApp}
	final, err := r.markedParts()
	if err != nil {
		return partsRoll{}, err
	}
	linkDivineSmite(final)
	diceParts := 0
	for _, sp := range final {
		if r.counts(sp) && sp.Count > 0 && sp.Sides > 0 {
			diceParts++
		}
	}
	for _, sp := range final {
		if !r.counts(sp) {
			continue
		}
		pr, err := s.rollPart(sp, r, diceParts)
		if err != nil {
			return partsRoll{}, err
		}
		out.rolls = append(out.rolls, pr)
		out.parts = append(out.parts, sp)
	}
	out.sumUp()
	return out, nil
}

// markedParts applies the player's marks to the choosable extras: the ones marked are
// checked now, and the slot a mark names goes with its part.
func (r rollPartsInput) markedParts() ([]partRecord, error) {
	var final []partRecord
	chosen := 0
	for _, sp := range r.parts {
		marked, err := r.markedPart(sp, &chosen)
		if err != nil {
			return nil, err
		}
		final = append(final, marked)
	}
	return final, nil
}

// markedPart is a part with the player's mark applied (a part that is no choosable extra
// stays as it is). chosen counts the maneuvers marked so far.
func (r rollPartsInput) markedPart(sp partRecord, chosen *int) (partRecord, error) {
	if sp.Kind != partExtra || !sp.Choosable {
		return sp, nil
	}
	mark := r.choice.marks[sp.Key]
	selected := sp.Selected
	if r.choice.chosen {
		selected = mark != nil
	}
	if selected {
		if err := r.checkMarked(sp, mark, chosen); err != nil {
			return sp, err
		}
	}
	sp.Selected = selected
	if mark != nil {
		sp.SlotLevel, sp.Pact = mark.GetSlotLevel(), mark.GetPact()
	}
	return sp, nil
}

// checkMarked refuses a second maneuver for the attack and an extra that does not hold now.
func (r rollPartsInput) checkMarked(sp partRecord, mark *playv1.SelectedExtra, chosen *int) error {
	if sp.Resource != "" && sp.Table == "" {
		// One maneuver per attack.
		if *chosen++; *chosen > 1 {
			return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_EXTRA_NOT_AVAILABLE, "only one maneuver for each attack")
		}
	}
	return r.selectOK(sp, mark)
}

// linkDivineSmite makes the extra die of Divine Smite follow the smite, and sets the
// smite's dice by the slot it spends: 2d8 for a 1st level slot, one more for each level above.
func linkDivineSmite(final []partRecord) {
	smite := slices.IndexFunc(final, func(sp partRecord) bool { return sp.Key == combat.ExtraDivineSmite && sp.Selected })
	for i := range final {
		if final[i].Key == combat.ExtraDivineSmiteExtra {
			final[i].Selected = smite >= 0
			if smite >= 0 {
				final[i].SlotLevel, final[i].Pact = final[smite].SlotLevel, final[smite].Pact
			}
		}
	}
	if smite >= 0 {
		final[smite].Count = clamp32(combat.SmiteDice(int(final[smite].SlotLevel)), 0, 100)
	}
}

// rollPart rolls one part that counts.
func (s *Service) rollPart(sp partRecord, r rollPartsInput, diceParts int) (partRoll, error) {
	count, fixed := r.diceOf(sp)
	pr := partRoll{
		Key: sp.Key, Label: sp.Label, Count: clamp32(count, 0, 200), Sides: sp.Sides, Flat: sp.Flat, Fixed: clamp32(fixed, 0, 10000),
		DamageType: sp.DamageType, Conditional: sp.Conditional, Counted: true, Physical: !r.in.inApp,
		Resource: sp.Resource, Rider: sp.Rider,
	}
	if count > 0 {
		if err := s.rollPartDice(&pr, sp, r, diceParts); err != nil {
			return partRoll{}, err
		}
	}
	// The extra die of Divine Smite counts against an undead or a fiend only.
	if sp.Key == combat.ExtraDivineSmiteExtra && r.target.CreatureType != "undead" && r.target.CreatureType != "fiend" {
		pr.Counted = false
	}
	return pr, nil
}

// sumUp adds the rolls that count, by damage type and all together, never below 0.
func (r *partsRoll) sumUp() {
	for _, pr := range r.rolls {
		if !pr.Counted {
			continue
		}
		n := int(pr.Sum) + int(pr.Flat) + int(pr.Fixed)
		r.byType[pr.DamageType] += n
	}
	for typ, n := range r.byType {
		r.byType[typ] = max(n, 0)
	}
	for _, n := range r.byType {
		r.total += n
	}
}

// partCriticalDice is the dice a part rolls on a hit and the fixed number that comes with them: a critical
// hit doubles them (or adds the maximum), except those of a part that keeps its dice.
func partCriticalDice(sp partRecord, critical bool, rule combat.CriticalRule) (count, fixed int) {
	if sp.NoCrit {
		return int(sp.Count), 0
	}
	return combat.CriticalDice(rules.DiceFormula{Count: int(sp.Count), Sides: int(sp.Sides)}, critical, rule)
}

// diceOf is the dice a part rolls and the fixed number that comes without rolling: the
// critical's rule applied to its dice, and, for the weapon, the extra dice of a feature
// (Brutal Critical), always rolled after the critical's own.
func (r rollPartsInput) diceOf(sp partRecord) (count, fixed int) {
	if sp.Count > 0 && sp.Sides > 0 {
		count, fixed = partCriticalDice(sp, r.p.Critical, r.rule)
	}
	if sp.Key == partWeapon && count > 0 {
		count += int(r.p.ExtraDice)
	}
	return count, fixed
}

// counts says a part is part of the damage: the weapon, a marked extra, an automatic
// line, and not one the master took out.
func (r rollPartsInput) counts(sp partRecord) bool {
	switch {
	case sp.Removed:
		return false
	case sp.Kind == partExtra:
		return sp.Selected
	}
	return true
}

// rollPartDice rolls the dice of one part in the app, or reads the sum typed for it.
func (s *Service) rollPartDice(pr *partRoll, sp partRecord, r rollPartsInput, diceParts int) error {
	expr := dice.Expr{Count: int(pr.Count), Sides: int(pr.Sides)}
	if r.in.inApp {
		return s.rollInApp(pr, sp, expr)
	}
	typed, ok := r.typed[sp.Key]
	if !ok && diceParts == 1 {
		typed, ok = r.in.typed, true // typed_sum: a damage of one group of dice
	}
	if !ok {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("typed_parts needs a sum for %s", sp.Key))
	}
	if _, err := dice.Physical(expr, typed); err != nil {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("the sum of %s must be %d to %d", sp.Key, expr.Count, expr.Count*expr.Sides))
	}
	pr.Sum = clamp32(typed, 0, 100000)
	return nil
}

// rollInApp rolls the dice of a part with the server's roller, rerolling the low faces of
// a weapon that does so.
func (s *Service) rollInApp(pr *partRoll, sp partRecord, expr dice.Expr) error {
	res, err := dice.Roll(s.roller, expr)
	if err != nil {
		return fmt.Errorf("roll the damage: %w", err)
	}
	faces := res.Faces
	if sp.Reroll {
		kept, rerolls, err := combat.GreatWeaponFighting(faces, int(pr.Sides), s.roller.Roll)
		if err != nil {
			return fmt.Errorf("reroll the damage: %w", err)
		}
		faces = kept
		for _, rr := range rerolls {
			pr.Rerolled = append(pr.Rerolled, rerolled{Index: clamp32(rr.Index, 0, 200), From: clamp32(rr.From, 0, 100), To: clamp32(rr.To, 0, 100)})
		}
	}
	pr.Faces = faces32(faces)
	for _, f := range faces {
		pr.Sum += clamp32(f, 0, 100)
	}
	return nil
}

// typedPartsOf reads the typed sums by part key, refusing a key twice.
func typedPartsOf(raw []*playv1.TypedPart) (map[string]int, error) {
	out := map[string]int{}
	for _, t := range raw {
		if _, dup := out[t.GetPartKey()]; dup || t.GetPartKey() == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("typed_parts must name each part once"))
		}
		out[t.GetPartKey()] = int(t.GetSum())
	}
	return out, nil
}

// storePartsRoll writes what a roll left on the pending damage.
func storePartsRoll(ctx context.Context, c *combatTx, id string, parts []partRecord, rolls []partRoll, groups []stepGroup, afterSteps *int32, landedBefore *hpState) error {
	var landed []byte
	if landedBefore != nil {
		landed = mustJSON(landedBefore)
	}
	var rolled []byte // NULL: a roll with no parts of its own leaves the damage's list as it is
	if len(parts) > 0 {
		rolled = mustJSON(parts)
	}
	if err := c.q.SetPendingDamageDetail(ctx, playdb.SetPendingDamageDetailParams{
		ID: id, Parts: rolled, PartRolls: mustJSON(rolls), Steps: mustJSON(groups), AfterSteps: afterSteps, LandedBefore: landed,
	}); err != nil {
		return fmt.Errorf("keep the parts of the damage: %w", err)
	}
	return nil
}

// ---- reading the combat for the parts of a hit ----

// freeSmiteSlots lists the spell slots a character has free, with how many dice a
// Divine Smite rolls with each (pact magic slots too: they are spell slots).
func freeSmiteSlots(v *playv1.CharacterVitals) []*playv1.SlotOption {
	var out []*playv1.SlotOption
	for _, u := range v.GetSpellSlots() {
		if free := u.GetTotal() - u.GetUsed(); free > 0 {
			out = append(out, &playv1.SlotOption{Level: u.GetLevel(), Free: free, Max: u.GetTotal(), DiceCount: clamp32(combat.SmiteDice(int(u.GetLevel())), 0, 100)})
		}
	}
	if p := v.GetPactSlots(); p != nil && p.GetTotal()-p.GetUsed() > 0 {
		out = append(out, &playv1.SlotOption{
			Level: p.GetSlotLevel(), Pact: true, Free: p.GetTotal() - p.GetUsed(), Max: p.GetTotal(), DiceCount: clamp32(combat.SmiteDice(int(p.GetSlotLevel())), 0, 100),
		})
	}
	return out
}

// hitParts works out the parts of the hit of a weapon attack and stores them on the
// pending damage the attack opened. A hit with no extra and no automatic line has none.
func (s *Service) hitParts(ctx context.Context, c *combatTx, campaignID string, v combatViewer, in attackModeInputs, attacker, target playdb.Combatant,
	sheet link.Sheet, attack link.Attack, weaponBonus int, mode combat.RollMode, disadvantaged, opportunity bool, pendingID string,
) error {
	if !attack.Weapon {
		return nil
	}
	names, err := s.namerFor(ctx, c.tx, campaignID)
	if err != nil {
		return err
	}
	h := hitFacts{
		attacker: attacker, target: target, sheet: sheet, attack: attack, weaponBonus: weaponBonus, mode: mode, disadvantaged: disadvantaged,
		cs: in.cs, states: in.states, enc: c.enc, viewer: v, names: names, opportunity: opportunity,
	}
	if h.targetHurt, err = s.isHurt(ctx, c, campaignID, target); err != nil {
		return err
	}
	plan, planned, err := s.planOfAttack(ctx, c, attacker, attack.Key)
	if err != nil {
		return err
	}
	if planned {
		h.planned = plannedParts(plan)
	}
	if sheet.Traits.DivineSmite && attacker.Kind == kindPlayer {
		vit, err := s.vitals.GetVitalsTx(ctx, c.tx, campaignID, attacker.CharacterID)
		if err != nil {
			return err
		}
		h.freeSlots = len(freeSmiteSlots(vit)) > 0
	}
	if attacker.Kind == kindPlayer && (len(sheet.Traits.Maneuvers) > 0 || len(sheet.Traits.ExtraDamages) > 0) {
		vit, err := s.vitals.GetVitalsTx(ctx, c.tx, campaignID, attacker.CharacterID)
		if err != nil {
			return err
		}
		h.usesLeft = usesLeftOf(vit)
	}
	parts := h.buildParts()
	if parts == nil {
		return nil
	}
	return c.q.SetPendingDamageParts(ctx, playdb.SetPendingDamagePartsParams{ID: pendingID, Parts: mustJSON(parts)})
}

// usesLeftOf is the uses left of each resource of a character's vitals.
func usesLeftOf(v *playv1.CharacterVitals) map[string]int32 {
	out := map[string]int32{}
	for _, r := range v.GetResources() {
		out[r.GetKey()] = r.GetTotal() - r.GetUsed()
	}
	return out
}

// isHurt says the combatant is below its hit point maximum.
func (s *Service) isHurt(ctx context.Context, c *combatTx, campaignID string, who playdb.Combatant) (bool, error) {
	if holdsHP(who) {
		return conv.OrZero(who.HpCurrent) < conv.OrZero(who.HpMax), nil
	}
	vit, err := s.vitals.GetVitalsTx(ctx, c.tx, campaignID, who.CharacterID)
	if err != nil {
		return false, err
	}
	return vit.GetHitPointsCurrent() < vit.GetHitPointsMax(), nil
}

// extraCheck is what the check of a marked extra reads of the combat when the damage is
// rolled: the attacker, and the turn the once-per-turn marks are compared with.
type extraCheck struct {
	c          *combatTx
	campaignID string
	attacker   playdb.Combatant
	turn       string
}

// errExtraNotAvailable is the refusal of an extra that does not hold now.
func errExtraNotAvailable() error {
	return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_EXTRA_NOT_AVAILABLE, "that extra does not hold now")
}

// selectionChecker returns the check a marked extra passes when the damage is rolled:
// its condition still holds (the hit stored it; the once-per-turn marks and the
// concentration are read again), and a Divine Smite names a free slot. It also
// remembers the slot vitals it read.
func (s *Service) selectionChecker(ctx context.Context, c *combatTx, campaignID string, attacker playdb.Combatant) func(sp partRecord, mark *playv1.SelectedExtra) error {
	e := extraCheck{c: c, campaignID: campaignID, attacker: attacker, turn: turnKey(c.enc)}
	return func(sp partRecord, mark *playv1.SelectedExtra) error {
		return s.checkExtra(ctx, e, sp, mark)
	}
}

// checkExtra is the check of one marked extra.
func (s *Service) checkExtra(ctx context.Context, e extraCheck, sp partRecord, mark *playv1.SelectedExtra) error {
	if !sp.Available {
		return errExtraNotAvailable()
	}
	if sp.Once && readTableOnce(e.attacker.TableOnceTurns)[sp.Key] == e.turn {
		return errExtraNotAvailable()
	}
	if sp.Resource != "" {
		return s.checkManeuverUses(ctx, e, sp)
	}
	switch sp.Key {
	case combat.ExtraSneakAttack:
		if conv.OrZero(e.attacker.SneakAttackTurn) == e.turn {
			return errExtraNotAvailable()
		}
	case combat.ExtraColossusSlayer:
		if conv.OrZero(e.attacker.ColossusSlayerTurn) == e.turn {
			return errExtraNotAvailable()
		}
	case combat.ExtraHuntersMark:
		if conv.OrZero(e.attacker.ConcentrationSpell) != "spell:hunters-mark" {
			return errExtraNotAvailable()
		}
	case combat.ExtraDivineSmite:
		return s.checkSmiteSlot(ctx, e, mark)
	}
	return nil
}

// checkManeuverUses checks a table maneuver: the attacker's own, with a use left now.
func (s *Service) checkManeuverUses(ctx context.Context, e extraCheck, sp partRecord) error {
	if e.attacker.Kind != kindPlayer {
		return errExtraNotAvailable()
	}
	vit, err := s.vitals.GetVitalsTx(ctx, e.c.tx, e.campaignID, e.attacker.CharacterID)
	if err != nil {
		return err
	}
	if usesLeftOf(vit)[sp.Resource] <= 0 {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_USES, "no uses left")
	}
	return nil
}

// checkSmiteSlot checks that a Divine Smite names a slot level and that the attacker has
// a free slot of it.
func (s *Service) checkSmiteSlot(ctx context.Context, e extraCheck, mark *playv1.SelectedExtra) error {
	if mark == nil || mark.GetSlotLevel() < 1 || mark.GetSlotLevel() > 9 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("a Divine Smite names the slot it spends"))
	}
	vit, err := s.vitals.GetVitalsTx(ctx, e.c.tx, e.campaignID, e.attacker.CharacterID)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(freeSmiteSlots(vit), func(o *playv1.SlotOption) bool {
		return o.GetLevel() == mark.GetSlotLevel() && o.GetPact() == mark.GetPact()
	}) {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_SLOT, "there is no free spell slot of that level",
			func(b *playv1.EncounterBlocked) { b.MinLevel = mark.GetSlotLevel() })
	}
	return nil
}

// ---- resistance, vulnerability and immunity ----

// modifiersOf lists the resistances, vulnerabilities and immunities that apply to the
// damage a combatant takes: a creature's or an NPC's from its stat block, a player's
// character's from its race, its features and its rage (SRD 5.1, "Damage Resistance
// and Vulnerability"). Rage's resistance holds only while the state lasts and no heavy
// armor is worn.
func (s *Service) modifiersOf(ctx context.Context, c *combatTx, campaignID string, states map[string][]playdb.CombatantState, who playdb.Combatant) ([]combat.Modifier, error) {
	if holdsHP(who) {
		return s.statBlockModifiers(ctx, c, campaignID, who)
	}
	sheet, err := s.sheetOf(ctx, c.tx, campaignID, who)
	if err != nil {
		return nil, err
	}
	var out []combat.Modifier
	raging := hasState(states, who.ID, stateRage) && !sheet.Traits.HeavyArmor
	for _, r := range sheet.Traits.Resistances {
		if !rules.ResistanceActive(r.While, raging) {
			continue
		}
		out = append(out, traitModifier(r))
	}
	return out, nil
}

// traitModifier is the modifier a sheet's resistance, immunity or vulnerability makes. A trait of the
// content that names no kind is a resistance.
func traitModifier(r link.Resistance) combat.Modifier {
	m := combat.Modifier{Kind: combat.StepResistance, Source: r.Source, DamageTypes: r.DamageTypes}
	switch r.Kind {
	case rules.ResistanceKindImmune:
		m.Kind = combat.StepImmunity
	case rules.ResistanceKindVulnerable:
		m.Kind = combat.StepVulnerability
	}
	if r.Nonmagical {
		m.Qualifier = combat.QualifierNonmagical
	}
	return m
}

// statBlockModifiers are the immunities, resistances and vulnerabilities of a creature
// or an NPC, from its stat block.
func (s *Service) statBlockModifiers(ctx context.Context, c *combatTx, campaignID string, who playdb.Combatant) ([]combat.Modifier, error) {
	mods, err := s.roster.DamageModifiers(ctx, c.tx, campaignID, who.CharacterID, conv.OrZero(who.MonsterKey))
	if err != nil {
		return nil, err
	}
	source := mods.Source
	if source == "" {
		source = "stat-block"
	}
	var out []combat.Modifier
	for _, g := range []struct {
		kind      string
		types     []string
		qualifier string
	}{
		{combat.StepImmunity, mods.Immune, ""},
		{combat.StepResistance, mods.Resistant, ""},
		{combat.StepVulnerability, mods.Vulnerable, ""},
		{combat.StepImmunity, mods.ImmuneNonmagical, combat.QualifierNonmagical},
		{combat.StepResistance, mods.ResistantNonmagical, combat.QualifierNonmagical},
	} {
		if len(g.types) > 0 {
			out = append(out, combat.Modifier{Kind: g.kind, Source: source, DamageTypes: g.types, Qualifier: g.qualifier})
		}
	}
	return out, nil
}

// landing is what a damage comes to once its modifiers apply.
type landing struct {
	// amount is the damage that lands, groups the damage of the types the modifiers
	// changed, and changed says any did.
	amount  int32
	groups  []stepGroup
	changed bool
}

// settleSteps takes the damage of each type through the target's modifiers. byType is
// the damage by type (one entry for a damage of one type).
func settleSteps(byType map[string]int, mods []combat.Modifier, ignored []string, from combat.Origin) landing {
	var out landing
	types := make([]string, 0, len(byType))
	for t := range byType {
		types = append(types, t)
	}
	slices.Sort(types)
	for _, t := range types {
		final, steps := combat.Adjust(byType[t], t, mods, ignored, from)
		if len(steps) > 0 {
			group := stepGroup{DamageType: t, Before: clamp32(byType[t], 0, math.MaxInt32), From: from}
			for _, m := range mods {
				if m.Covers(t, from) {
					group.Mods = append(group.Mods, combat.Modifier{Kind: m.Kind, Source: m.Source, DamageTypes: []string{t}, Qualifier: m.Qualifier})
				}
			}
			out.groups = append(out.groups, group)
			out.changed = true
		}
		out.amount += clamp32(final, 0, math.MaxInt32)
	}
	return out
}

// reapply works the landing out again from the stored groups, leaving out the sources
// the master told the app to ignore. total is the damage before any modifier.
func reapply(total int32, groups []stepGroup, ignored []string) int32 {
	out := total
	for _, g := range groups {
		after, _ := combat.Adjust(int(g.Before), g.DamageType, g.Mods, ignored, g.From)
		out += clamp32(after, 0, math.MaxInt32) - g.Before
	}
	return max(out, 0)
}

// ---- what the API says of a pending damage ----

// criticalRuleOf is the rule a pending damage was made under.
func ruleOfPending(p playdb.PendingDamage) combat.CriticalRule {
	if p.CriticalMaxRule {
		return combat.CriticalMaxPlusRoll
	}
	return combat.CriticalDoubledDice
}

// dtPT writes a damage type in Portuguese.
func dtPT(key string) string { return damageTypePT[key] }

// pendingView builds the PendingDamage the caller reads: the damage with its parts, the
// slots a Divine Smite may spend, what each part rolled and the steps resistance took
// it through. Only the master and the owner of the player's character that takes the
// damage get the steps; a creature's or an NPC's are the master's alone (RN-20).
func (s *Service) pendingView(ctx context.Context, campaignID string, p playdb.PendingDamage, cs []playdb.Combatant, v combatViewer) *playv1.PendingDamage {
	out := pendingProto(p, cs)
	if j := slices.IndexFunc(cs, func(c playdb.Combatant) bool { return c.ID == conv.OrZero(p.AttackerID) }); j >= 0 {
		out.AttackKey = shownKey(v, cs[j], p.AttackKey) // a stat block action's key names the creature's type (RN-20)
	}
	out.Parts = partsProto(p, ruleOfPending(p), dtPT)
	i := slices.IndexFunc(cs, func(c playdb.Combatant) bool { return c.ID == p.TargetID })
	if i >= 0 && !v.master && holdsHP(cs[i]) && !v.owns(cs[i]) {
		showAsRolled(out, p)
	}
	out.PartRolls = partRollsProto(p, v.master, dtPT)
	if i >= 0 && (v.master || (cs[i].Kind == kindPlayer && v.owns(cs[i]))) {
		if groups := readStepGroups(p.Steps); len(groups) > 0 {
			out.Steps = stepsProto(groups, nil, s.namesFor(ctx, campaignID), dtPT)
			out.AmountAfterSteps = p.AfterSteps
		}
	}
	if p.Status == pendingAwaitingRoll && slices.ContainsFunc(out.Parts, func(d *playv1.DamagePart) bool { return d.GetNeedsSlot() }) {
		s.addSlotOptions(ctx, campaignID, out, p, cs)
	}
	return out
}

// showAsRolled makes a player read the damage as rolled: not what the target's modifiers
// made of it, nor whether a conditional die counted.
func showAsRolled(out *playv1.PendingDamage, p playdb.PendingDamage) {
	if shown, ok := shownOfPending(p); ok {
		out.Amount = shown
		if out.Roll != nil {
			out.Roll.Total = shown
		}
	}
}

// addSlotOptions gives the parts that need a slot (a Divine Smite) the slots the attacker,
// a player's character, has free.
func (s *Service) addSlotOptions(ctx context.Context, campaignID string, out *playv1.PendingDamage, p playdb.PendingDamage, cs []playdb.Combatant) {
	j := slices.IndexFunc(cs, func(c playdb.Combatant) bool { return c.ID == conv.OrZero(p.AttackerID) })
	if j < 0 || cs[j].Kind != kindPlayer {
		return
	}
	vit, err := s.vitals.GetVitals(ctx, campaignID, cs[j].CharacterID)
	if err != nil {
		return
	}
	slots := freeSmiteSlots(vit)
	for _, d := range out.Parts {
		if d.GetNeedsSlot() {
			d.SlotOptions = slots
		}
	}
}

// settleLanding is the damage that lands, and the steps that made it, for a target
// that takes a damage by type: the target's own modifiers apply (an NPC's or a creature's
// at once, a player's character's as the preview the master applies from).
func (s *Service) settleLanding(ctx context.Context, c *combatTx, states map[string][]playdb.CombatantState, tgt playdb.Combatant, byType map[string]int, from combat.Origin) (landing, error) {
	mods, err := s.modifiersOf(ctx, c, c.session.CampaignID, states, tgt)
	if err != nil {
		return landing{}, err
	}
	return settleSteps(byType, mods, nil, from), nil
}

// legacy is the roll as the one-roll damage reads it: the total, the weapon's faces and
// whether the dice were typed.
func (r *partsRoll) legacy() dice.Result {
	res := dice.Result{Total: r.total, Physical: r.physical}
	for _, pr := range r.rolls {
		if pr.Key == partWeapon {
			for _, f := range pr.Faces {
				res.Faces = append(res.Faces, int(f))
			}
			res.Modifier = int(pr.Flat) + int(pr.Fixed)
		}
	}
	return res
}

// rollPendingParts checks and rolls the parts of a pending damage: the extras the player
// marked, with their conditions checked again, and the dice of the counted parts.
func (s *Service) rollPendingParts(ctx context.Context, c *combatTx, campaignID string, p playdb.PendingDamage, attacker, target playdb.Combatant,
	in rollInput, typed map[string]int, choice extraChoice,
) (*partsRoll, error) {
	tsheet, err := s.sheetOf(ctx, c.tx, campaignID, target)
	if err != nil {
		return nil, err
	}
	out, err := s.rollParts(rollPartsInput{
		p: p, parts: readParts(p.Parts), rule: ruleOfPending(p), choice: choice, in: in, typed: typed, target: tsheet.Traits,
		selectOK: s.selectionChecker(ctx, c, campaignID, attacker),
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// settleParts does what a roll with parts leaves besides the damage: the slot a Divine
// Smite spent, the once-per-turn marks of Sneak Attack and Colossus Slayer, and the
// parts for the event (the log, the undo).
func (s *Service) settleParts(ctx context.Context, c *combatTx, attacker playdb.Combatant, pr *partsRoll, ev *actionEvent) (*playv1.CharacterVitals, error) {
	ev.Parts = pr.rolls
	var vit *playv1.CharacterVitals
	once := onceTurn{key: turnKey(c.enc), sneak: attacker.SneakAttackTurn, colossus: attacker.ColossusSlayerTurn, table: readTableOnce(attacker.TableOnceTurns)}
	for i, r := range pr.rolls {
		if !r.Counted && !r.Conditional {
			continue
		}
		v, spent, err := s.spendForPart(ctx, c, attacker, pr.parts[i], r.Key, ev)
		if err != nil {
			return nil, err
		}
		if spent {
			vit = v
		}
		// A table's extra that spends a use is once per turn too: the mark does not wait for a part that spends nothing.
		once.mark(r.Key)
		if pr.parts[i].Once {
			once.markTable(r.Key)
		}
	}
	if err := s.keepOnceMarks(ctx, c, attacker, once, ev); err != nil {
		return nil, err
	}
	return vit, nil
}

// onceTurn is the once-per-turn marks of the attacker while the parts of a roll settle.
type onceTurn struct {
	key             string // the turn
	sneak, colossus *string
	// table are the marks of the table's extras by feature key; tableChanged says a roll set one.
	table        map[string]string
	tableChanged bool
}

// markTable notes that the table's extra of the feature key was used this turn.
func (o *onceTurn) markTable(featureKey string) {
	if o.table[featureKey] != o.key {
		o.table[featureKey] = o.key
		o.tableChanged = true
	}
}

// mark notes that the part of the key was used this turn.
func (o *onceTurn) mark(partKey string) {
	switch partKey {
	case combat.ExtraSneakAttack:
		o.sneak = &o.key
	case combat.ExtraColossusSlayer:
		o.colossus = &o.key
	}
}

// keepOnceMarks stores the once-per-turn marks the roll changed, and the ones before in the
// event for the undo.
func (s *Service) keepOnceMarks(ctx context.Context, c *combatTx, attacker playdb.Combatant, once onceTurn, ev *actionEvent) error {
	srdChanged := once.sneak != attacker.SneakAttackTurn || once.colossus != attacker.ColossusSlayerTurn
	if !srdChanged && !once.tableChanged {
		return nil
	}
	ev.OnceBefore = &onceMarks{Sneak: conv.OrZero(attacker.SneakAttackTurn), Colossus: conv.OrZero(attacker.ColossusSlayerTurn)}
	if srdChanged {
		if err := c.q.SetCombatantOncePerTurn(ctx, playdb.SetCombatantOncePerTurnParams{ID: attacker.ID, SneakAttackTurn: once.sneak, ColossusSlayerTurn: once.colossus}); err != nil {
			return fmt.Errorf("mark the once-per-turn damage: %w", err)
		}
	}
	if once.tableChanged {
		ev.OnceBefore.Table, ev.OnceBefore.TableChanged = readTableOnce(attacker.TableOnceTurns), true
		if err := c.q.SetCombatantTableOnceTurns(ctx, playdb.SetCombatantTableOnceTurnsParams{ID: attacker.ID, TableOnceTurns: writeTableOnce(once.table)}); err != nil {
			return fmt.Errorf("mark the table's once-per-turn damage: %w", err)
		}
	}
	return nil
}

// spendForPart spends what a part that counted costs: the use of a maneuver or the slot of
// a Divine Smite, noted in the event for the undo. spent says it cost one.
func (s *Service) spendForPart(ctx context.Context, c *combatTx, attacker playdb.Combatant, part partRecord, key string, ev *actionEvent) (vit *playv1.CharacterVitals, spent bool, err error) {
	if part.Resource != "" && part.Kind == partExtra {
		if vit, err = s.spendResource(ctx, c, attacker.CharacterID, part.Resource, 1); err != nil {
			return nil, false, err
		}
		ev.ManeuverSpent = append(ev.ManeuverSpent, part.Resource)
		return vit, true, nil
	}
	if key != combat.ExtraDivineSmite {
		return nil, false, nil
	}
	ref := slotRef{Level: part.SlotLevel, Pact: part.Pact}
	if vit, err = s.spendSlot(ctx, c, attacker.CharacterID, ref, 1); err != nil {
		return nil, false, err
	}
	ev.Slot = &ref
	return vit, true, nil
}

// onceMarks are the once-per-turn marks of a combatant before a damage roll, for the undo.
type onceMarks struct {
	Sneak    string `json:"sneak,omitempty"`
	Colossus string `json:"colossus,omitempty"`
	// Table are the table's marks before, when the roll changed them (TableChanged: an empty map is a
	// state to put back too).
	Table        map[string]string `json:"table,omitempty"`
	TableChanged bool              `json:"table_changed,omitempty"`
}

// keepRollDetail stores what a roll left on a pending damage: the parts and their rolls
// (the roll's own pending), the steps and the damage after them, and the hit points an
// NPC had before the damage landed on it.
func (s *Service) keepRollDetail(ctx context.Context, c *combatTx, g, p playdb.PendingDamage, pr *partsRoll, steps landing, hit damageHit) error {
	var parts []partRecord
	var rolls []partRoll
	if pr != nil && g.ID == p.ID {
		parts, rolls = pr.parts, pr.rolls
	}
	var after *int32
	if steps.changed {
		after = &steps.amount
	}
	var landed *hpState
	if hit.Applied && hit.Before != nil {
		landed = hit.Before
	}
	if len(parts) == 0 && len(rolls) == 0 && !steps.changed && landed == nil {
		return nil
	}
	return storePartsRoll(ctx, c, g.ID, parts, rolls, steps.groups, after, landed)
}

// shownTotal is the damage as the roll made it for a player who is not the target's: every
// die counts, also the conditional one that the target's kind keeps out of the real damage,
// so the total tells nothing of what the target is.
func (r partsRoll) shownTotal() int {
	total := shownTotalOf(r.rolls)
	if r.adjust != nil {
		total = r.adjust(total)
	}
	return total
}

func shownTotalOf(rolls []partRoll) int {
	byType := map[string]int{}
	for _, pr := range rolls {
		if pr.Counted || pr.Conditional {
			byType[pr.DamageType] += int(pr.Sum) + int(pr.Flat) + int(pr.Fixed)
		}
	}
	total := 0
	for _, n := range byType {
		total += max(n, 0)
	}
	return total
}

// shownOfPending is the damage of a pending damage as rolled, for a player who is not
// the target's, when the target's modifiers or a conditional die made the real one differ.
func shownOfPending(p playdb.PendingDamage) (int32, bool) {
	if rolls := readPartRolls(p.PartRolls); len(rolls) > 0 {
		return clamp32(shownTotalOf(rolls), 0, math.MaxInt32), true
	}
	if len(readStepGroups(p.Steps)) > 0 && p.RollTotal != nil {
		total := *p.RollTotal
		if p.Half {
			total = clamp32(combat.HalfDamage(int(total)), 0, math.MaxInt32)
		}
		return total, true
	}
	return 0, false
}

// heldPartsRoll rebuilds the roll of a damage with extras that a reaction window held.
func heldPartsRoll(parts []partRecord, rolls []partRoll, physical bool) *partsRoll {
	out := &partsRoll{parts: parts, rolls: rolls, byType: map[string]int{}, physical: physical}
	for _, pr := range rolls {
		if pr.Counted {
			out.byType[pr.DamageType] += int(pr.Sum) + int(pr.Flat) + int(pr.Fixed)
		}
	}
	for typ, n := range out.byType {
		out.byType[typ] = max(n, 0)
		out.total += out.byType[typ]
	}
	return out
}

// scale puts what the reactions took off a held damage on its damage types, in proportion,
// and on the total a player reads, which a reaction changes in the same way.
func (r *partsRoll) scale(from, to int, st *replayState) {
	r.total = to
	if from > 0 && from != to {
		left := to
		var last string
		for typ, n := range r.byType {
			scaled := n * to / from
			r.byType[typ] = scaled
			left -= scaled
			last = typ
		}
		if last != "" { // the rounding goes to one type
			r.byType[last] += left
		}
	}
	r.adjust = func(shown int) int { return adjustedDamage(st, shown) }
}

// joinNote puts two notes of a part in one line.
func joinNote(a, b string) string {
	if a == "" {
		return b
	}
	return a + "; " + b
}
