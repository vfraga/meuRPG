package rules

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/PuraFome/meuRPG/backend/internal/rules/formula"
	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// The hit_rider effect: a lasting effect, a forced move, or both, that the target of one attack line's hit takes
// beyond the damage (Thunder Gauntlets of the Guardian, Grasp of Hadar, Lance of Lethargy, Repelling Blast). `line`
// names the hit: a weapon line as for extra_damage (line.go), or the key of a spell ("spell:..."), whose spell
// attack carries it on a hit and whose saving throw carries it on a failed save. The engine only says what each
// rider is and how it ends; the combat offers it on the hit card, rolls its saving throw, applies it and undoes it.

// The lasting effects a rider may be (its `rider`), a closed vocabulary the combat knows how to put on a creature.
const (
	// HitRiderAttackDisadvantage gives the target disadvantage on its attack rolls against anyone but the holder:
	// an attack_disadvantage modifier that spares the effect's source (ExceptSource), the goad's effect.
	HitRiderAttackDisadvantage = "attack_disadvantage_except_holder"
	// HitRiderSpeedReduction lowers the target's speed by SpeedReductionFt: a speed_add modifier of that many feet
	// less.
	HitRiderSpeedReduction = "speed_reduction"
)

// The forced moves a rider may make (its `forced_move`): away from the holder, or toward it.
const (
	HitMovePush = "push"
	HitMovePull = "pull"
)

// How a lasting rider ends when it has no duration in rounds (its `ends_on`): at the start or at the end of the
// holder's next turn.
const (
	HitEndsStartOfHolderTurn = "start_of_holder_turn"
	HitEndsEndOfHolderTurn   = "end_of_holder_turn"
)

var (
	hitRiders     = []string{HitRiderAttackDisadvantage, HitRiderSpeedReduction}
	hitMoves      = []string{HitMovePush, HitMovePull}
	hitRiderEnds  = []string{HitEndsStartOfHolderTurn, HitEndsEndOfHolderTurn}
	hitRiderNames = map[string][2]string{
		HitRiderAttackDisadvantage: {"Desvantagem nos ataques contra os outros", "O alvo tem desvantagem nas jogadas de ataque contra qualquer criatura que não seja quem tem a característica."},
		HitRiderSpeedReduction:     {"Deslocamento reduzido", "O deslocamento do alvo diminui no valor da redução (use o campo de redução do deslocamento)."},
	}
	hitMoveNames = map[string][2]string{
		HitMovePush: {"Empurrar", "O alvo é empurrado para longe de quem tem a característica."},
		HitMovePull: {"Puxar", "O alvo é puxado em direção a quem tem a característica."},
	}
	hitEndsNames = map[string][2]string{
		HitEndsStartOfHolderTurn: {"Até o início do próximo turno de quem tem a característica", "Acaba quando começa o próximo turno de quem tem a característica."},
		HitEndsEndOfHolderTurn:   {"Até o fim do próximo turno de quem tem a característica", "Acaba quando termina o próximo turno de quem tem a característica."},
	}
)

// Bounds of a hit_rider.
const (
	// MinHitRiderFt and MaxHitRiderFt bound the feet of a speed reduction and of a forced move, on the 5 ft grid.
	MinHitRiderFt = 5
	MaxHitRiderFt = 60
	// MaxHitRiderRounds is the longest a lasting rider lasts: one hour of 6-second rounds.
	MaxHitRiderRounds = 600
)

// spellKeyPrefix starts the key of a spell, the value a hit_rider's `line` takes to ride on a spell.
const spellKeyPrefix = "spell:"

// HitRider is a hit_rider effect of a feature the character has, as the sheet lists it. The combat offers it
// after a hit on Line that the rules of its Frequency allow.
type HitRider struct {
	// Key is the feature that gives it, NamePT its name.
	Key, NamePT string
	// Line is the key of the attack line whose hit carries it (LinePact, a weapon key, PseudoWeaponKey,
	// NaturalAttackKey) or of the spell whose attack hit, or failed saving throw, does (a "spell:" key).
	Line string
	// Rider is the lasting effect (a HitRider* constant), empty for a forced move alone, and SpeedReductionFt the feet
	// HitRiderSpeedReduction takes off the speed.
	Rider            string
	SpeedReductionFt int
	// ForcedMove is HitMovePush or HitMovePull, empty for none, and ForcedMoveFt how far, a multiple of 5.
	ForcedMove   string
	ForcedMoveFt int
	// DurationRounds is how many rounds a lasting rider lasts; 0 when EndsOn says when it ends instead (a
	// HitEnds* constant), and for a forced move alone both are empty.
	DurationRounds int
	EndsOn         string
	// Frequency is ExtraDamageOncePerTurn or ExtraDamageEachHit.
	Frequency string
	// SaveAbility is the ability of the target's saving throw, empty when the rider asks none, and SaveDC its DC (8
	// + proficiency + the better modifier of the ability the effect names and, when it names a second one, of that).
	// A spell with a saving throw of its own has none here: its own decides.
	SaveAbility string
	SaveDC      int
	// TextPT is the line the master reads.
	TextPT string
}

// OnSpell says the rider rides on a spell (Line is a spell's key) and not on a weapon line.
func (r HitRider) OnSpell() bool { return strings.HasPrefix(r.Line, spellKeyPrefix) }

// hitRiderShape names the field of a hit_rider that is wrong (its value, its limit or its pairing with another
// field), why in the reason's words and the message; field is "" when the effect is right. It reads the effect's
// own fields only: the line and the spell it names are the content's to check.
func hitRiderShape(e *Effect) (field, reason, msg string) {
	for _, check := range []func(*Effect) (string, string, string){hitRiderWhat, hitRiderDuration, hitRiderSave} {
		if field, reason, msg = check(e); field != "" {
			return field, reason, msg
		}
	}
	return "", "", ""
}

// hitRiderWhat checks what the rider does: its lasting effect, its speed reduction and its forced move.
func hitRiderWhat(e *Effect) (field, reason, msg string) {
	switch {
	case e.Rider != "" && !slices.Contains(hitRiders, e.Rider):
		return "rider", ReasonValue, fmt.Sprintf("rider %q is not one of %s", e.Rider, strings.Join(hitRiders, ", "))
	case e.ForcedMove != "" && !slices.Contains(hitMoves, e.ForcedMove):
		return "forced_move", ReasonValue, fmt.Sprintf("forced_move is %s", strings.Join(hitMoves, " or "))
	case e.ForcedMove == "" && e.ForcedMoveFt != 0:
		return "forced_move_ft", ReasonValue, "forced_move_ft needs a forced_move"
	case e.ForcedMove != "" && !hitRiderFeet(e.ForcedMoveFt):
		return "forced_move_ft", ReasonLimit, hitRiderFeetMsg("forced_move_ft")
	case e.Rider == "" && e.ForcedMove == "":
		return "rider", ReasonValue, "a hit_rider has a rider, a forced move or both"
	case e.Rider == HitRiderSpeedReduction && !hitRiderFeet(e.SpeedReductionFt):
		return "speed_reduction_ft", ReasonLimit, hitRiderFeetMsg("speed_reduction_ft")
	case e.Rider != HitRiderSpeedReduction && e.SpeedReductionFt != 0:
		return "speed_reduction_ft", ReasonValue, "speed_reduction_ft belongs to the " + HitRiderSpeedReduction + " rider"
	}
	return "", "", ""
}

// hitRiderFeet says a number of feet is one a rider takes: from MinHitRiderFt to MaxHitRiderFt in steps of 5.
func hitRiderFeet(ft int) bool {
	return ft >= MinHitRiderFt && ft <= MaxHitRiderFt && ft%feetStep == 0
}

func hitRiderFeetMsg(field string) string {
	return fmt.Sprintf("%s is %d to %d feet, in steps of %d", field, MinHitRiderFt, MaxHitRiderFt, feetStep)
}

// hitRiderDuration checks how long a lasting rider lasts and how often a hit offers it. A lasting rider has a
// duration in rounds or one way to end, never both; a forced move alone lasts no time.
func hitRiderDuration(e *Effect) (field, reason, msg string) {
	switch {
	case e.Frequency != "" && !slices.Contains(extraDamageFrequencies, e.Frequency):
		return "frequency", ReasonValue, fmt.Sprintf("unknown frequency %q", e.Frequency)
	case e.Rider == "" && e.DurationRounds != "":
		return "duration_rounds", ReasonValue, "a forced move alone lasts no time: duration_rounds belongs to a rider"
	case e.Rider == "" && len(e.EndsOn) > 0:
		return "ends_on", ReasonValue, "a forced move alone lasts no time: ends_on belongs to a rider"
	case e.Rider == "":
		return "", "", ""
	case e.DurationRounds != "" && len(e.EndsOn) > 0:
		return "ends_on", ReasonValue, "a rider lasts for duration_rounds or until ends_on, not both"
	case e.DurationRounds == "" && len(e.EndsOn) == 0:
		return "duration_rounds", ReasonValue, "a rider needs a duration in rounds or an ends_on"
	case len(e.EndsOn) > 1 || (len(e.EndsOn) == 1 && !slices.Contains(hitRiderEnds, e.EndsOn[0])):
		return "ends_on", ReasonValue, "ends_on is one of " + strings.Join(hitRiderEnds, ", ")
	}
	return "", "", ""
}

// hitRiderSave checks the saving throw of a rider: its ability, and the ability or the two abilities its DC reads.
func hitRiderSave(e *Effect) (field, reason, msg string) {
	_, abilityOK := abilityIndex[Ability(e.Ability)]
	_, altOK := abilityIndex[Ability(e.DCAbilityAlt)]
	switch {
	case e.SaveAbility == "" && e.Ability != "":
		return "ability", ReasonValue, "ability is the ability of a saving throw's DC: it needs save_ability"
	case e.SaveAbility == "" && e.DCAbilityAlt != "":
		return "dc_ability_alt", ReasonValue, "dc_ability_alt is for the DC of a saving throw: it needs save_ability"
	case e.SaveAbility == "":
		return "", "", ""
	case !isAbilityKey(e.SaveAbility):
		return "save_ability", ReasonValue, fmt.Sprintf("save_ability %q is not an ability", e.SaveAbility)
	case !abilityOK:
		return "ability", ReasonValue, "a saving throw needs ability, the ability whose modifier is in its DC"
	case e.DCAbilityAlt != "" && !altOK:
		return "dc_ability_alt", ReasonValue, fmt.Sprintf("dc_ability_alt %q is not an ability", e.DCAbilityAlt)
	case e.DCAbilityAlt == e.Ability:
		return "dc_ability_alt", ReasonValue, "dc_ability_alt is a second ability: it cannot be the ability"
	}
	return "", "", ""
}

func isAbilityKey(s string) bool {
	_, ok := abilityIndex[Ability(s)]
	return ok
}

// riderSpell says the content has the spell and what its hit is: "attack" for a spell attack roll (the rider rides on a
// hit), "save" for a saving throw (on a failed save). An empty kind is a spell with neither.
func riderSpell(spells map[string]*srd51.Spell, key string) (kind string, ok bool) {
	sp, found := spells[key]
	switch {
	case !found:
		return "", false
	case sp.SaveAbility != "":
		return "save", true
	case sp.AttackType != "":
		return "attack", true
	}
	return "", true
}

// hitRiderSpellProblem is what is wrong with the spell a hit_rider names: it has none of that key, it has neither an
// attack roll nor a saving throw, or it has a saving throw of its own and the rider asks another.
func hitRiderSpellProblem(e *Effect, spells map[string]*srd51.Spell) (field, reason, msg string) {
	kind, ok := riderSpell(spells, e.Line)
	switch {
	case !ok:
		return "line", ReasonReference, fmt.Sprintf("line %q names no spell of the content", e.Line)
	case kind == "":
		return "line", ReasonValue, fmt.Sprintf("line %q names a spell with no attack roll and no saving throw: there is no hit to ride on", e.Line)
	case kind == "save" && e.SaveAbility != "":
		return "save_ability", ReasonValue, "a spell with a saving throw of its own decides the rider: save_ability is refused"
	}
	return "", "", ""
}

// hitRider is the compile-time check of a hit_rider: the shape of its fields, its duration and its line. The table's
// menu has checked them at their fields already (checkHitRider), so this guards the SRD load.
func (x effectCompiler) hitRider() error {
	e := x.e
	if field, _, msg := hitRiderShape(e); field != "" {
		return x.fail("%s", msg)
	}
	var err error
	if e.durationRounds, err = x.compile(e.DurationRounds, formula.Int); err != nil {
		return err
	}
	if e.Line == "" {
		return x.fail("a hit_rider needs a line")
	}
	if strings.HasPrefix(e.Line, spellKeyPrefix) {
		if field, _, msg := hitRiderSpellProblem(e, x.c.spells); field != "" {
			return x.fail("%s", msg)
		}
		return nil
	}
	return x.line()
}

// strayHitRider refuses the fields of hit_rider on an effect of another type, and the long range on one that is no
// pseudo_weapon (the fields it shares with other types are checked where they are).
func (x effectCompiler) strayHitRider() error {
	e := x.e
	if e.Type != "hit_rider" && (e.Rider != "" || e.SpeedReductionFt != 0 || e.ForcedMove != "" || e.ForcedMoveFt != 0 || e.SaveAbility != "") {
		return x.fail("rider, speed_reduction_ft, forced_move, forced_move_ft and save_ability belong to hit_rider")
	}
	if e.Type != "hit_rider" && e.Type != "superiority_die" && e.DCAbilityAlt != "" {
		return x.fail("dc_ability_alt belongs to hit_rider and superiority_die")
	}
	if e.Type != "pseudo_weapon" && e.LongRangeFt != 0 {
		return x.fail("long_range_ft belongs to pseudo_weapon")
	}
	return nil
}

// checkHitRider checks a hit_rider of the table's content at its fields: the shape, the line (a spell or a weapon
// line of the entry) and, for a spell with a saving throw of its own, that the rider asks none.
func (b *overlayBuilder) checkHitRider(site effectSite, e *Effect) error {
	if e.Line == "" {
		return site.fail(".line", ReasonValue, "a hit_rider needs the line whose hits carry it")
	}
	if strings.HasPrefix(e.Line, spellKeyPrefix) {
		if field, reason, msg := hitRiderSpellProblem(e, b.n.spells); field != "" {
			return site.fail("."+field, reason, "%s", msg)
		}
	}
	if field, reason, msg := hitRiderShape(e); field != "" {
		return site.fail("."+field, reason, "%s", msg)
	}
	if strings.HasPrefix(e.Line, spellKeyPrefix) {
		return nil
	}
	return b.checkLine(site, e)
}

// hitRiders fills Derived.HitRiders from the hit_rider effects the character has, with the saving throw's DC and
// the rounds worked out. A rider whose rounds the formula cannot give is left out.
func (x *deriver) hitRiders() {
	for _, a := range x.active {
		e := a.effect
		if e.Type != "hit_rider" || !x.applies(a) {
			continue
		}
		rounds, ok := x.hitRiderRounds(e)
		if !ok {
			x.issue(IssueFormula, "", "Um efeito de %s foi ignorado: a duração não é válida.", x.c.namePT(a.owner))
			continue
		}
		r := HitRider{
			Key: a.owner, NamePT: x.c.namePT(a.owner), Line: e.Line, Rider: e.Rider, SpeedReductionFt: e.SpeedReductionFt,
			ForcedMove: e.ForcedMove, ForcedMoveFt: e.ForcedMoveFt, DurationRounds: rounds,
			Frequency: cmp.Or(e.Frequency, ExtraDamageOncePerTurn), SaveAbility: e.SaveAbility, TextPT: e.TextPT,
		}
		if len(e.EndsOn) == 1 {
			r.EndsOn = e.EndsOn[0]
		}
		if e.SaveAbility != "" {
			r.SaveDC = x.saveDC(e.Ability, e.DCAbilityAlt)
		}
		x.d.HitRiders = append(x.d.HitRiders, r)
	}
}

// hitRiderRounds is the duration of a rider in rounds, from 1 to MaxHitRiderRounds; 0 for a rider that ends by
// ends_on and for a forced move alone. ok is false when the formula gives nothing the table can use.
func (x *deriver) hitRiderRounds(e *Effect) (rounds int, ok bool) {
	if e.durationRounds == nil {
		return 0, true
	}
	n, err := e.durationRounds.Int(x.env)
	if err != nil || n < 1 {
		return 0, false
	}
	return min(n, MaxHitRiderRounds), true
}

// saveDC is the DC of a saving throw the character's feature asks: 8 plus the proficiency bonus and the modifier of
// the ability, or of the better of it and a second one when alt is set. An ability the sheet has not is 0.
func (x *deriver) saveDC(ability, alt string) int {
	best, found := 0, false
	for _, k := range []string{ability, alt} {
		if k == "" {
			continue
		}
		for _, a := range x.d.Abilities {
			if string(a.Ability) == k && (!found || a.Modifier > best) {
				best, found = a.Modifier, true
			}
		}
	}
	if !found {
		return 0
	}
	return goadBaseDC + x.d.ProficiencyBonus + best
}

func hitRiderTypeValues() []MenuValue {
	return namedMenuHint(hitRiders, hitRiderNames)
}

func hitMoveValues() []MenuValue {
	return namedMenuHint(hitMoves, hitMoveNames)
}

func hitEndsValues() []MenuValue {
	return namedMenuHint(hitRiderEnds, hitEndsNames)
}
