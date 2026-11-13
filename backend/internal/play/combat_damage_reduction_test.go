package play

import (
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// A table feat that takes a flat amount off each weapon attack that hits (#572), end to end on Ragna of the
// table's race (whose trait halves nonmagical weapons): invented content, the engine reads only the effects.

// reductionFeat makes a table feat with one weapon damage reduction modifier and returns its key.
func (f *frostTable) reductionFeat(t *testing.T, name, target, amount string) string {
	t.Helper()
	res, err := f.master.table.CreateTableEntry(t.Context(), connect.NewRequest(&rulesv1.CreateTableEntryRequest{
		CampaignId: f.campaignID,
		Body: &rulesv1.CreateTableEntryRequest_TableFeat{TableFeat: &rulesv1.TableFeat{
			NamePt: name, DescPt: []string{"Um talento da mesa."},
			Effects: []*rulesv1.TableEffect{{Type: "modifier", Target: target, Mode: "add", Value: amount}},
		}},
	}))
	if err != nil {
		t.Fatalf("CreateTableEntry(%s) error = %v", name, err)
	}
	return res.Msg.GetEntry().GetKey()
}

// ragnaWith gives Ragna the table's feats of the keys.
func (f *frostTable) ragnaWith(t *testing.T, keys ...string) {
	t.Helper()
	f.edit(t, func(full *charactersv1.FullSheet) { full.FeatKeys = keys })
}

// attackLineOf is the damage of the first attack line of the log as u reads it.
func (f *frostTable) attackLineOf(t *testing.T, u *user, e *playv1.Encounter) *playv1.CombatLogDamage {
	t.Helper()
	for _, r := range f.log(t, u, e).GetRounds() {
		for _, en := range r.GetEntries() {
			if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_ATTACK {
				return en.GetDamage()
			}
		}
	}
	t.Fatal("the log has no attack line")
	return nil
}

func TestATableFeatReductionTakesItOffAWeaponHitBeforeTheRaceResistance(t *testing.T) {
	t.Parallel()
	f, e := newFrostTable(t)
	thick := f.reductionFeat(t, "Couro grosso", "damage.reduction.weapon", "3")
	f.ragnaWith(t, thick)
	e = f.passTo(t, e, "Goblin")
	dmg := f.hitRagna(t, e, 6) // 1d6 + 2 = 8 slashing
	steps := dmg.GetSteps()
	if len(steps) != 2 {
		t.Fatalf("steps = %v, want the reduction and then the resistance", steps)
	}
	if r := steps[0]; r.GetKind() != playv1.DamageStepKind_DAMAGE_STEP_KIND_REDUCTION || r.GetBefore() != 8 || r.GetAfter() != 5 ||
		!slices.Equal(r.GetSourceKeys(), []string{thick}) || !strings.HasPrefix(r.GetLabelPt(), "Redução de dano de arma (Couro grosso)") {
		t.Errorf("first step = %v, want the reduction 8 to 5 from the feat", r)
	}
	if r := steps[1]; r.GetKind() != playv1.DamageStepKind_DAMAGE_STEP_KIND_RESISTANCE || r.GetBefore() != 5 || r.GetAfter() != 2 {
		t.Errorf("second step = %v, want the resistance 5 to 2", r)
	}
	if dmg.GetAmountAfterSteps() != 2 {
		t.Errorf("the damage after the steps = %d, want 2", dmg.GetAmountAfterSteps())
	}
	hpMax := f.vitals(t, f.toren).GetHitPointsMax()
	if _, err := f.settle(t, f.master, e, dmg.GetId(), true); err != nil {
		t.Fatalf("ApplyPendingDamage() error = %v", err)
	}
	if got := f.vitals(t, f.toren).GetHitPointsCurrent(); got != hpMax-2 {
		t.Errorf("Ragna has %d hit points, want %d", got, hpMax-2)
	}
}

func TestTwoTableFeatsReductionsAddAndBothAreNamedInOneStep(t *testing.T) {
	t.Parallel()
	f, e := newFrostTable(t)
	thick := f.reductionFeat(t, "Couro grosso", "damage.reduction.weapon", "3")
	tanned := f.reductionFeat(t, "Pele curtida", "damage.reduction.nonmagical_weapon", "2")
	f.ragnaWith(t, thick, tanned)
	e = f.passTo(t, e, "Goblin")
	dmg := f.hitRagna(t, e, 6)
	steps := dmg.GetSteps()
	if len(steps) == 0 || steps[0].GetKind() != playv1.DamageStepKind_DAMAGE_STEP_KIND_REDUCTION || steps[0].GetBefore() != 8 || steps[0].GetAfter() != 3 {
		t.Fatalf("steps = %v, want one reduction 8 to 3 first", steps)
	}
	if got := steps[0].GetSourceKeys(); len(got) != 2 || !slices.Contains(got, thick) || !slices.Contains(got, tanned) {
		t.Errorf("the reduction names %v, want both feats", got)
	}
	if !strings.Contains(steps[0].GetLabelPt(), "Couro grosso e Pele curtida") {
		t.Errorf("the label reads %q, want both feats named", steps[0].GetLabelPt())
	}
}

func TestATableFeatReductionDoesNotTouchASpell(t *testing.T) {
	t.Parallel()
	f, e := newFrostTable(t)
	f.ragnaWith(t, f.reductionFeat(t, "Couro grosso", "damage.reduction.weapon", "3"))
	e = f.passTo(t, e, "Pensantus")
	hpMax := f.vitals(t, f.toren).GetHitPointsMax()
	hit := f.mustAttack(t, f.ana, e, "Pensantus", fireBolt, "Ragna", d20(15))
	dmg := f.mustDamage(t, f.ana, e, hit.GetPendingDamage().GetId(), typedDamage(6)).GetPendingDamage()
	if len(dmg.GetSteps()) != 0 {
		t.Errorf("steps = %v, want none for a spell", dmg.GetSteps())
	}
	if _, err := f.settle(t, f.master, e, dmg.GetId(), true); err != nil {
		t.Fatalf("ApplyPendingDamage() error = %v", err)
	}
	if got := f.vitals(t, f.toren).GetHitPointsCurrent(); got != hpMax-6 {
		t.Errorf("Ragna has %d hit points, want %d: a spell is no weapon", got, hpMax-6)
	}
}

// RN-20: the steps of the damage a player's character takes reach the master and that character's player, and no
// other player.
func TestAWeaponReductionStepReachesOnlyTheMasterAndTheCharactersPlayer(t *testing.T) {
	t.Parallel()
	f, e := newFrostTable(t)
	f.ragnaWith(t, f.reductionFeat(t, "Couro grosso", "damage.reduction.weapon", "3"))
	e = f.passTo(t, e, "Goblin")
	dmg := f.hitRagna(t, e, 6)
	if _, err := f.settle(t, f.master, e, dmg.GetId(), true); err != nil {
		t.Fatalf("ApplyPendingDamage() error = %v", err)
	}
	reductions := func(u *user) int {
		n := 0
		for _, s := range f.attackLineOf(t, u, e).GetSteps() {
			if s.GetKind() == playv1.DamageStepKind_DAMAGE_STEP_KIND_REDUCTION {
				n++
			}
		}
		return n
	}
	for _, c := range []struct {
		who  string
		u    *user
		want int
	}{{"the master", f.master, 1}, {"Ragna's player", f.caio, 1}, {"another player", f.ana, 0}, {"a third player", f.bia, 0}} {
		if got := reductions(c.u); got != c.want {
			t.Errorf("%s reads %d reduction step(s), want %d", c.who, got, c.want)
		}
	}
}

// The steps of a damage already rolled stay in the log when the master later archives the feat that gave the reduction:
// the master and the owner read them as they were, and no other player reads one. The sheet has no source the master hides
// from its own player (only a lasting effect can be hidden, and that case has its own test).
func TestAnArchivedFeatsReductionStepStaysInTheLogForTheMasterAndTheOwnerOnly(t *testing.T) {
	t.Parallel()
	f, e := newFrostTable(t)
	thick := f.reductionFeat(t, "Couro grosso", "damage.reduction.weapon", "3")
	f.ragnaWith(t, thick)
	e = f.passTo(t, e, "Goblin")
	dmg := f.hitRagna(t, e, 6)
	if _, err := f.settle(t, f.master, e, dmg.GetId(), true); err != nil {
		t.Fatalf("ApplyPendingDamage() error = %v", err)
	}
	if _, err := f.master.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: f.campaignID, Key: thick})); err != nil {
		t.Fatalf("ArchiveTableEntry() error = %v", err)
	}
	labels := func(u *user) string {
		var out []string
		for _, s := range f.attackLineOf(t, u, e).GetSteps() {
			out = append(out, s.GetLabelPt()+"|"+strings.Join(s.GetSourceKeys(), ","))
		}
		return strings.Join(out, ";")
	}
	for _, c := range []struct {
		who string
		u   *user
	}{{"the master", f.master}, {"Ragna's player", f.caio}} {
		if got := labels(c.u); !strings.Contains(got, "Redução de dano de arma (Couro grosso)|"+thick) {
			t.Errorf("%s reads %q, want the reduction as it was", c.who, got)
		}
	}
	if got := labels(f.ana); got != "" {
		t.Errorf("another player reads %q, want no step", got)
	}
}

// A reduction whose source is marked hidden reads "Outra fonte" and names no key to a player, and the source to the master.
func TestAHiddenReductionSourceReadsOutraFonteToAPlayer(t *testing.T) {
	t.Parallel()
	mods := []combat.Modifier{{Kind: combat.StepReduction, Source: "feat:couro-grosso@mesa", Amount: 3, Hidden: true}}
	groups := settleSteps(map[string]int{"damage-type:slashing": 8}, mods, nil, combat.Origin{Attack: true, Weapon: true}).groups
	names := func(k string) string { return map[string]string{"feat:couro-grosso@mesa": "Couro grosso"}[k] }
	player := stepsProto(groups, nil, names, dtPT, false)
	if len(player) != 1 || player[0].GetLabelPt() != "Redução de dano de arma (Outra fonte)" || len(player[0].GetSourceKeys()) != 0 {
		t.Errorf("the player reads %v, want the reduction from Outra fonte and no key", player)
	}
	master := stepsProto(groups, nil, names, dtPT, true)
	if len(master) != 1 || master[0].GetLabelPt() != "Redução de dano de arma (Couro grosso)" || len(master[0].GetSourceKeys()) != 1 {
		t.Errorf("the master reads %v, want the feat and its key", master)
	}
	if master[0].GetBefore() != 8 || master[0].GetAfter() != 5 {
		t.Errorf("the step goes %d to %d, want 8 to 5", master[0].GetBefore(), master[0].GetAfter())
	}
}

// A weapon hit of two damage types loses the reduction once, not once per type (#572).
func TestAReductionIsTakenOnceFromAHitOfTwoDamageTypes(t *testing.T) {
	t.Parallel()
	cut, fire := "damage-type:slashing", "damage-type:fire"
	mods := []combat.Modifier{{Kind: combat.StepReduction, Source: "feat:couro-grosso@mesa", Amount: 3}}
	from := combat.Origin{Attack: true, Weapon: true}
	land := settleSteps(map[string]int{cut: 8, fire: 4}, mods, nil, from)
	if land.amount != 9 {
		t.Errorf("the landing is %d, want 9: 12 minus 3 once", land.amount)
	}
	steps := stepsProto(land.groups, nil, func(string) string { return "Couro grosso" }, dtPT, true)
	if len(steps) != 1 || steps[0].GetBefore() != 12 || steps[0].GetAfter() != 9 || steps[0].GetDamageTypeKey() != "" {
		t.Errorf("steps = %v, want one reduction of the whole attack, 12 to 9, with no damage type", steps)
	}
	if got := reapply(12, land.groups, nil); got != 9 {
		t.Errorf("reapply = %d, want 9", got)
	}
	if got := reapply(12, land.groups, []string{"feat:couro-grosso@mesa"}); got != 12 {
		t.Errorf("reapply without the source = %d, want 12", got)
	}
	struck := stepsProto(land.groups, []string{"feat:couro-grosso@mesa"}, func(string) string { return "Couro grosso" }, dtPT, true)
	if len(struck) != 1 || !struck[0].GetIgnored() {
		t.Errorf("the struck steps = %v, want one, ignored", struck)
	}
	// The master edits a part: the damage is worked out again from the stored groups.
	again := settleSteps(map[string]int{cut: 8, fire: 2}, modsOfGroups(land.groups), nil, originOfGroups(land.groups))
	if again.amount != 7 {
		t.Errorf("the landing after an edit is %d, want 7: 10 minus 3 once", again.amount)
	}
}

// The reduction spills to the next type, and resistance then works on what is left of each type.
func TestAReductionLargerThanTheBiggestTypeSpillsAndThenResistanceApplies(t *testing.T) {
	t.Parallel()
	cut, fire := "damage-type:slashing", "damage-type:fire"
	mods := []combat.Modifier{
		{Kind: combat.StepReduction, Source: "feat:couro-grosso@mesa", Amount: 5},
		{Kind: combat.StepResistance, Source: "feat:sangue-frio@mesa", DamageTypes: []string{fire}},
	}
	land := settleSteps(map[string]int{cut: 3, fire: 6}, mods, nil, combat.Origin{Attack: true, Weapon: true})
	// fire is the largest: 6 - 5 = 1, halved to 0; slashing stays 3.
	if land.amount != 3 {
		t.Errorf("the landing is %d, want 3", land.amount)
	}
	spill := settleSteps(map[string]int{cut: 3, fire: 1}, mods[:1], nil, combat.Origin{Attack: true, Weapon: true})
	if spill.amount != 0 {
		t.Errorf("the landing is %d, want 0: 4 minus 5 stops at 0", spill.amount)
	}
}

// A creature's save action (a breath, a tail swipe) is no weapon attack: the weapon reduction of the target takes
// nothing off its damage, and the origin it stores keeps Weapon false for a later edit of the parts (#572).
func TestAWeaponReductionDoesNotTouchACreaturesSaveAction(t *testing.T) {
	t.Parallel()
	a, e, hound := houndFight(t, true)
	f := &frostTable{armed: a}
	f.ragnaWith(t, f.reductionFeat(t, "Couro grosso", "damage.reduction.weapon", "3"), f.reductionFeat(t, "Pele curtida", "damage.reduction.nonmagical_weapon", "2"))
	a.h.roller.queue(1) // Toren fails the save
	res := a.mustUseAction(t, a.master, e, hound, fireBreath, []string{"Toren"})
	pend := res.GetResult().GetPendingDamages()
	if len(pend) != 1 {
		t.Fatalf("the breath opened %d pending damages, want 1", len(pend))
	}
	dmg := a.mustDamage(t, a.master, e, pend[0].GetId(), typedDamage(12)).GetPendingDamage()
	for _, s := range dmg.GetSteps() {
		if s.GetKind() == playv1.DamageStepKind_DAMAGE_STEP_KIND_REDUCTION {
			t.Errorf("steps = %v, want no weapon reduction on a save action", dmg.GetSteps())
		}
	}
	if len(dmg.GetSteps()) != 0 || dmg.GetAmount() != 12 {
		t.Errorf("the damage = %d with steps %v, want 12 and no step", dmg.GetAmount(), dmg.GetSteps())
	}
}

// originOf of a pending damage of a creature's action: Weapon only for an attack that is no spell; Attack stays true
// for all of them (the resistances to nonmagical attacks are unchanged), and Magical is the spell attack.
func TestOriginOfACreaturesActionIsAWeaponOnlyForAWeaponAttack(t *testing.T) {
	t.Parallel()
	a, e, hound := houndFight(t, true)
	cs, err := a.h.svc.queries.ListCombatants(t.Context(), e.GetId())
	if err != nil {
		t.Fatalf("ListCombatants() error = %v", err)
	}
	attacker := cs[slices.IndexFunc(cs, func(c playdb.Combatant) bool { return c.Label == hound })]
	tx, err := a.h.pool.Begin(t.Context())
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	defer func() { _ = tx.Rollback(t.Context()) }()
	q := a.h.svc.queries.WithTx(tx)
	session, err := q.GetOpenGameSession(t.Context(), a.campaignID)
	if err != nil {
		t.Fatalf("GetOpenGameSession() error = %v", err)
	}
	flow := &damageFlow{s: a.h.svc, c: &combatTx{tx: tx, q: q, session: session}, attacker: attacker}
	cast := "cast-id"
	for _, c := range []struct {
		name, key string
		want      combat.Origin
	}{
		{"a save action", fireBreath, combat.Origin{Attack: true}},
		{"an action that is not in the stat block", "monster:hell-hound#nothing", combat.Origin{Attack: true}},
		{"a weapon attack", "monster:hell-hound#bite", combat.Origin{Attack: true, Weapon: true}},
	} {
		got, err := flow.originOf(t.Context(), playdb.PendingDamage{CastID: &cast, AttackKey: c.key})
		if err != nil || got != c.want {
			t.Errorf("%s: originOf() = %+v, %v; want %+v", c.name, got, err, c.want)
		}
	}
}
