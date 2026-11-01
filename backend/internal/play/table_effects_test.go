package play

import (
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// The table entries of these tests are invented: a table race whose traits carry a resource and the
// action or the hook that spends it. Ragna, a fighter of level 5 (44 hit points) of that race, is
// Toren's slot of the usual fixture; Pensantus and Brisa are the party's other two.

const (
	poolKey    = "reserva_de_teste"
	lastGaspPt = "Último Fôlego da Mesa"
)

// fxTrait is a trait of the invented race: a name, its text and its effects.
func fxTrait(name string, effects ...*rulesv1.TableEffect) *rulesv1.TableFeature {
	return &rulesv1.TableFeature{NamePt: name, DescPt: []string{"Texto de teste."}, Effects: effects}
}

// fxPool is a resource of the invented traits.
func fxPool(key, limit, recharge string) *rulesv1.TableEffect {
	return &rulesv1.TableEffect{Type: "resource", Resource: key, Max: limit, Recharge: recharge}
}

// fxTable is a campaign whose Ragna (a level 5 fighter, Caio's) is of an invented race with the traits; the
// session is open and nothing else has started.
func fxTable(t *testing.T, traits ...*rulesv1.TableFeature) *armed {
	t.Helper()
	return fxTableOf(t, "class:fighter", traits...)
}

// fxTableOf is fxTable with Ragna of the class (a caster, for the slots).
func fxTableOf(t *testing.T, class string, traits ...*rulesv1.TableFeature) *armed {
	t.Helper()
	return newArmedWith(t, func(a *armed) {
		res, err := a.master.table.CreateTableEntry(t.Context(), connect.NewRequest(&rulesv1.CreateTableEntryRequest{
			CampaignId: a.campaignID, Body: &rulesv1.CreateTableEntryRequest_TableRace{TableRace: &rulesv1.TableRace{
				NamePt: "Raça de Teste da Mesa", Size: "Medium", SpeedFt: 30, Languages: []string{"language:common"}, Traits: traits,
			}},
		}))
		if err != nil {
			t.Fatalf("CreateTableEntry(race) error = %v", err)
		}
		a.toren = a.caio.hero(t, a.campaignID, "Ragna", class, res.Msg.GetEntry().GetKey(), 5,
			&rulesv1.AbilityScores{Strength: 16, Dexterity: 13, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{greataxe, shortbowKey}, nil)
		a.pens = a.ana.hero(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 1,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8}, nil, []string{fireBolt})
		a.bri = a.bia.hero(t, a.campaignID, "Brisa", "class:fighter", "race:human", 2,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 16, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{rapier}, nil)
	})
}

// fxFight starts the fight.
func fxFight(t *testing.T, a *armed) *playv1.Encounter {
	t.Helper()
	return a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId()}},
		npcRolls: []int{3},
		players:  map[string]int32{"Ragna": 18, "Pensantus": 10, "Brisa": 1},
		reveal:   []string{"Goblin"},
		at:       map[string][2]int32{"Ragna": {3, 3}, "Goblin": {4, 3}, "Pensantus": {10, 3}, "Brisa": {8, 8}},
	})
}

// fxActionKey is the key of the feature action of Ragna with the name.
func fxActionKey(t *testing.T, a *armed, e *playv1.Encounter, name string) string {
	t.Helper()
	for _, f := range a.mustOptions(t, a.caio, e, "Ragna").GetOptions().GetFeatureActions() {
		if f.GetAction().GetNamePt() == name {
			return f.GetAction().GetKey()
		}
	}
	t.Fatalf("no feature action named %q", name)
	return ""
}

// fxOption is the option of the action with the name, for the caller.
func fxOption(t *testing.T, a *armed, e *playv1.Encounter, name string) *rulesv1.ActionOption {
	t.Helper()
	for _, f := range a.mustOptions(t, a.caio, e, "Ragna").GetOptions().GetFeatureActions() {
		if f.GetAction().GetNamePt() == name {
			return f
		}
	}
	t.Fatalf("no feature action named %q", name)
	return nil
}

// fxUse calls TakeAction as Caio for Ragna.
func fxUse(t *testing.T, a *armed, e *playv1.Encounter, key string, edit func(*playv1.TakeActionRequest)) (*playv1.TakeActionResponse, error) {
	t.Helper()
	req := &playv1.TakeActionRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Ragna"), ActionKey: key, IdempotencyKey: newKey()}
	if edit != nil {
		edit(req)
	}
	res, err := a.caio.combat.TakeAction(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func fxSpend(n int32) func(*playv1.TakeActionRequest) {
	return func(r *playv1.TakeActionRequest) {
		r.Spend = n
		r.Roll = &playv1.TakeActionRequest_RollInApp{RollInApp: true}
	}
}

// fxSpendOf sets the resource's spent uses, as the master's hand does.
func fxSpendOf(t *testing.T, a *armed, key string, used int32) {
	t.Helper()
	a.correct(t, a.toren, func(r *playv1.AdjustCharacterVitalsRequest) {
		r.ResourcesUsed = []*playv1.ResourceUsed{{Key: key, Used: used}}
	})
}

// fxNextTurn ends the turn on course and comes back to Ragna's.
func fxNextTurn(t *testing.T, a *armed, e *playv1.Encounter) {
	t.Helper()
	if _, err := a.endTurn(t, a.master, e, true); err != nil {
		t.Fatalf("EndTurn() error = %v", err)
	}
	a.passTo(t, e, "Ragna")
}

func wantFxCode(t *testing.T, err error, want connect.Code, what string) {
	t.Helper()
	if err == nil || connect.CodeOf(err) != want {
		t.Errorf("%s: error = %v, want code %v", what, err, want)
	}
}

// A table feature that spends 2 points of its pool to give a bonus action: the action is off when fewer than its cost are
// left, and each use spends exactly the cost.
func TestAnActionThatCostsTwoSpendsTwoOfThePool(t *testing.T) {
	t.Parallel()
	a := fxTable(t, fxTrait("Surto da Mesa",
		fxPool(poolKey, "4", "long_rest"),
		&rulesv1.TableEffect{Type: "grant_action", Economy: "bonus_action", Resource: poolKey, Cost: "2", TextPt: "Surto de teste."},
	))
	e := fxFight(t, a)
	key := fxActionKey(t, a, e, "Surto da Mesa")
	if o := fxOption(t, a, e, "Surto da Mesa"); !o.GetEnabled() || o.GetUsesLeft() != 4 || o.GetAction().GetCost() != 2 {
		t.Fatalf("the option = enabled %v, %d left, cost %d; want enabled, 4 left, cost 2", o.GetEnabled(), o.GetUsesLeft(), o.GetAction().GetCost())
	}
	fxSpendOf(t, a, poolKey, 3) // one left: less than the cost
	if o := fxOption(t, a, e, "Surto da Mesa"); o.GetEnabled() || o.GetReason().GetCode() != rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_NO_USES {
		t.Errorf("with 1 left the option = enabled %v (%v), want off for NO_USES", o.GetEnabled(), o.GetReason().GetCode())
	}
	if _, err := fxUse(t, a, e, key, nil); err == nil {
		t.Fatal("the action was taken with 1 use left and a cost of 2")
	}
	if got := usedOf(a.vitals(t, a.toren), poolKey); got != 3 {
		t.Errorf("the refused action changed the pool: %d spent, want 3", got)
	}
	fxSpendOf(t, a, poolKey, 0)
	res, err := fxUse(t, a, e, key, nil)
	if err != nil {
		t.Fatalf("TakeAction() error = %v", err)
	}
	if got := usedOf(a.vitals(t, a.toren), poolKey); got != 2 || res.GetSpent() != 2 {
		t.Errorf("after the use %d spent (answer says %d), want 2", got, res.GetSpent())
	}
	if !byLabel(t, a.get(t, a.caio), "Ragna").GetBonusActionUsed() {
		t.Error("the bonus action was not spent")
	}
	fxNextTurn(t, a, e)
	if _, err := fxUse(t, a, e, key, nil); err != nil {
		t.Fatalf("second TakeAction() error = %v", err)
	}
	if got := usedOf(a.vitals(t, a.toren), poolKey); got != 4 {
		t.Errorf("after the second use %d spent, want 4", got)
	}
	fxNextTurn(t, a, e)
	if o := fxOption(t, a, e, "Surto da Mesa"); o.GetEnabled() {
		t.Error("the action is on with the pool empty")
	}
}

// A table feature that spends 1 to 3 points to heal 1d6 for each: the server reads the points left in the use's
// transaction and refuses a number outside the range (0 takes the least); a valid pick spends exactly that many.
func TestAChosenSpendHealsPerPointAndIsValidatedOnTheServer(t *testing.T) {
	t.Parallel()
	a := fxTable(t, fxTrait("Fôlego da Mesa",
		fxPool(poolKey, "5", "long_rest"),
		&rulesv1.TableEffect{Type: "grant_action", Economy: "action", Resource: poolKey, Cost: "1", CostMax: "3", Heal: "1d6", ValuePerPoint: "1d6", TextPt: "Cura de teste."},
	))
	e := fxFight(t, a)
	a.hurt(t, a.toren, 1)
	key := fxActionKey(t, a, e, "Fôlego da Mesa")
	if o := fxOption(t, a, e, "Fôlego da Mesa"); o.GetAction().GetCost() != 1 || o.GetAction().GetCostMax() != 3 || o.GetAction().GetHeal().GetSides() != 6 {
		t.Fatalf("the action = %v, want cost 1 to 3 healing a d6", o.GetAction())
	}
	for name, n := range map[string]int32{"4 points (above cost_max)": 4, "9 points": 9, "a negative number": -1} {
		_, err := fxUse(t, a, e, key, fxSpend(n))
		wantFxCode(t, err, connect.CodeInvalidArgument, name)
	}
	if v := a.vitals(t, a.toren); usedOf(v, poolKey) != 0 || v.GetHitPointsCurrent() != 1 || byLabel(t, a.get(t, a.caio), "Ragna").GetActionUsed() {
		t.Fatalf("a refused pick changed something: %d spent, %d hit points, action used %v", usedOf(v, poolKey), v.GetHitPointsCurrent(), byLabel(t, a.get(t, a.caio), "Ragna").GetActionUsed())
	}

	a.h.roller.queue(4, 5, 6)
	res, err := fxUse(t, a, e, key, fxSpend(3))
	if err != nil {
		t.Fatalf("TakeAction(3) error = %v", err)
	}
	if v := a.vitals(t, a.toren); usedOf(v, poolKey) != 3 || v.GetHitPointsCurrent() != 16 {
		t.Errorf("after 3 points %d spent and %d hit points, want 3 and 1 + 4 + 5 + 6 = 16", usedOf(v, poolKey), v.GetHitPointsCurrent())
	}
	if res.GetSpent() != 3 || res.GetHealed() != 15 || res.GetRoll().GetDiceCount() != 3 || res.GetRoll().GetDiceSides() != 6 {
		t.Errorf("answer = spent %d, healed %d, roll %v; want 3, 15, 3d6", res.GetSpent(), res.GetHealed(), res.GetRoll())
	}

	// With 2 left the range ends at 2: 3 is more than is left, 0 takes the least.
	fxNextTurn(t, a, e)
	_, err = fxUse(t, a, e, key, fxSpend(3))
	wantFxCode(t, err, connect.CodeInvalidArgument, "3 points with 2 left")
	if got := usedOf(a.vitals(t, a.toren), poolKey); got != 3 {
		t.Fatalf("the refused pick spent: %d, want 3", got)
	}
	a.h.roller.queue(2)
	if _, err := fxUse(t, a, e, key, fxSpend(0)); err != nil {
		t.Fatalf("TakeAction(0) error = %v", err)
	}
	if got := usedOf(a.vitals(t, a.toren), poolKey); got != 4 {
		t.Errorf("0 took %d points in all, want the least (1): 4 spent", got)
	}
	fxNextTurn(t, a, e)
	a.h.roller.queue(1, 1)
	if _, err := fxUse(t, a, e, key, fxSpend(2)); err == nil {
		t.Error("2 points were taken with 1 left")
	}
}

// The heal and the points go to the actor or to one ally; the line of the log shows the numbers to the master and to the
// actor's player only; the undo gives the target its hit points and the pool its points back.
func TestAHealingActionTargetsAnAllyAndUndoes(t *testing.T) {
	t.Parallel()
	a := fxTable(t, fxTrait("Fôlego da Mesa",
		fxPool(poolKey, "5", "long_rest"),
		&rulesv1.TableEffect{Type: "grant_action", Economy: "action", Resource: poolKey, Cost: "1", CostMax: "3", Heal: "1d6", ValuePerPoint: "1d6", TextPt: "Cura de teste."},
	))
	e := fxFight(t, a)
	a.hurt(t, a.bri, 3)
	key := fxActionKey(t, a, e, "Fôlego da Mesa")
	_, err := fxUse(t, a, e, key, func(r *playv1.TakeActionRequest) {
		fxSpend(2)(r)
		r.HealTargetCombatantId = a.id(t, "Goblin")
	})
	wantFxCode(t, err, connect.CodeInvalidArgument, "an NPC as the target")
	if got := usedOf(a.vitals(t, a.toren), poolKey); got != 0 {
		t.Fatalf("the refused target spent %d points", got)
	}
	a.undoes(t, "the ally's heal", func() {
		a.h.roller.queue(3, 6)
		if _, err := fxUse(t, a, e, key, func(r *playv1.TakeActionRequest) {
			fxSpend(2)(r)
			r.HealTargetCombatantId = a.id(t, "Brisa")
		}); err != nil {
			t.Fatalf("TakeAction() error = %v", err)
		}
		if v := a.vitals(t, a.bri); v.GetHitPointsCurrent() != 12 {
			t.Fatalf("Brisa has %d hit points, want 3 + 3 + 6 = 12", v.GetHitPointsCurrent())
		}
		if got := usedOf(a.vitals(t, a.toren), poolKey); got != 2 {
			t.Fatalf("Ragna's pool has %d spent, want 2", got)
		}
		for name, u := range map[string]*user{"master": a.master, "Ragna's player": a.caio} {
			if d := healLine(t, a, u, e); d == nil || d.GetAmount() != 9 {
				t.Errorf("%s reads the heal line %v, want 9 hit points", name, d)
			}
		}
		if d := healLine(t, a, a.bia, e); d != nil && d.GetAmount() != 0 {
			t.Errorf("Brisa's player reads %d hit points in the line of Ragna's action: only the master and the actor's player do", d.GetAmount())
		}
	})
}

// healLine is the damage block of the log line of Ragna's action that heals, as the user reads it.
func healLine(t *testing.T, a *armed, u *user, e *playv1.Encounter) *playv1.CombatLogDamage {
	t.Helper()
	var out *playv1.CombatLogDamage
	for _, r := range a.log(t, u, e).GetRounds() {
		for _, en := range r.GetEntries() {
			if en.GetActorLabel() == "Ragna" && en.GetDamage().GetHealing() {
				out = en.GetDamage()
			}
		}
	}
	return out
}

// A table feature that gives temporary hit points equal to the level: they do not stack (the larger number is kept).
func TestAnActionGivesTemporaryHitPointsEqualToTheLevel(t *testing.T) {
	t.Parallel()
	feat := fxTrait("Vigor de Nível", &rulesv1.TableEffect{Type: "grant_action", Economy: "action", TempHp: "level()", TextPt: "Vigor de teste."})
	t.Run("a fresh gift", func(t *testing.T) {
		t.Parallel()
		a := fxTable(t, feat)
		e := fxFight(t, a)
		res, err := fxUse(t, a, e, fxActionKey(t, a, e, "Vigor de Nível"), nil)
		if err != nil {
			t.Fatalf("TakeAction() error = %v", err)
		}
		if v := a.vitals(t, a.toren); v.GetHitPointsTemporary() != 5 || res.GetTempHpGained() != 5 {
			t.Errorf("temporary hit points %d (answer says %d), want 5: the level", v.GetHitPointsTemporary(), res.GetTempHpGained())
		}
		if d := healLine(t, a, a.master, e); d == nil || d.GetTempHpGained() != 5 {
			t.Errorf("the master reads %v in the log, want 5 temporary hit points", d)
		}
	})
	t.Run("they do not stack", func(t *testing.T) {
		t.Parallel()
		a := fxTable(t, feat)
		e := fxFight(t, a)
		a.correct(t, a.toren, func(r *playv1.AdjustCharacterVitalsRequest) { r.HitPointsTemporary = new(int32(8)) })
		res, err := fxUse(t, a, e, fxActionKey(t, a, e, "Vigor de Nível"), nil)
		if err != nil {
			t.Fatalf("TakeAction() error = %v", err)
		}
		if v := a.vitals(t, a.toren); v.GetHitPointsTemporary() != 8 || res.GetTempHpGained() != 0 {
			t.Errorf("temporary hit points %d (gained %d), want the 8 kept and 0 gained", v.GetHitPointsTemporary(), res.GetTempHpGained())
		}
	})
}

// A pool that returns one use when its owner rolls initiative, only when none is left.
func TestAPoolReturnsOneUseOnInitiativeWhenNoneIsLeft(t *testing.T) {
	t.Parallel()
	pool := fxTrait("Prontidão da Mesa", fxPool(poolKey, "2", "initiative"))
	for _, tc := range []struct {
		name      string
		used, got int32
	}{
		{"none left", 2, 1},
		{"one left", 1, 1},
		{"all left", 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := fxTable(t, pool)
			fxSpendOf(t, a, poolKey, tc.used)
			fxFight(t, a)
			if got := usedOf(a.vitals(t, a.toren), poolKey); got != tc.got {
				t.Errorf("after the initiative %d spent, want %d", got, tc.got)
			}
		})
	}
}

// A rest gives the owner's temporary hit points of the hook, after the long rest ended the old ones, and restores uses without ever
// giving back more than was spent: a rest taken twice changes nothing the second time.
func TestARestRunsTheOwnersHookInPlay(t *testing.T) {
	t.Parallel()
	rest := func(t *testing.T, a *armed, kind playv1.RestKind) {
		t.Helper()
		if _, err := a.master.resource.TakeRest(t.Context(), connect.NewRequest(&playv1.TakeRestRequest{CampaignId: a.campaignID, Kind: kind, IdempotencyKey: newKey()})); err != nil {
			t.Fatalf("TakeRest() error = %v", err)
		}
	}
	t.Run("temporary hit points on a long rest", func(t *testing.T) {
		t.Parallel()
		a := fxTable(t, fxTrait("Vigília da Mesa", &rulesv1.TableEffect{Type: "on_event", Event: "long_rest", TempHp: "level()", TextPt: "Vigília de teste."}))
		a.correct(t, a.toren, func(r *playv1.AdjustCharacterVitalsRequest) { r.HitPointsTemporary = new(int32(2)) })
		rest(t, a, playv1.RestKind_REST_KIND_LONG)
		if got := a.vitals(t, a.toren).GetHitPointsTemporary(); got != 5 {
			t.Fatalf("temporary hit points after a long rest = %d, want 5 (the level), given after the rest cleared the 2", got)
		}
		rest(t, a, playv1.RestKind_REST_KIND_LONG)
		if got := a.vitals(t, a.toren).GetHitPointsTemporary(); got != 5 {
			t.Errorf("temporary hit points after a second long rest = %d, want 5", got)
		}
		if got := a.vitals(t, a.pens).GetHitPointsTemporary(); got != 0 {
			t.Errorf("Pensantus has %d temporary hit points: the hook is its owner's alone", got)
		}
	})
	t.Run("a restore never gives back more than was spent", func(t *testing.T) {
		t.Parallel()
		a := fxTable(t, fxTrait("Reserva da Vigília",
			fxPool(poolKey, "3", "none"),
			&rulesv1.TableEffect{Type: "on_event", Event: "short_rest", Restore: poolKey, Value: "2", TextPt: "Reserva de teste."},
		))
		fxSpendOf(t, a, poolKey, 3)
		for i, want := range []int32{1, 0, 0} {
			rest(t, a, playv1.RestKind_REST_KIND_SHORT)
			if got := usedOf(a.vitals(t, a.toren), poolKey); got != want {
				t.Fatalf("rest %d: %d spent, want %d", i+1, got, want)
			}
		}
	})
}

// dyingRagna is the table of a hook "death_save_failed": the character has one use of a once-per-rest resource, is at 0 hit points,
// and the save is due on its turn.
func dyingRagna(t *testing.T) (*armed, *playv1.Encounter) {
	t.Helper()
	a := fxTable(t, fxTrait(lastGaspPt,
		fxPool("folego_final", "1", "long_rest"),
		&rulesv1.TableEffect{Type: "on_event", Event: "death_save_failed", Resource: "folego_final", HealPercentMax: 50, TextPt: "Último fôlego de teste."},
	))
	e := fxFight(t, a)
	a.correct(t, a.toren, hpIs(0))
	fxNextTurn(t, a, e)
	return a, e
}

func ragnaCounts(t *testing.T, a *armed) (successes, failures int32) {
	t.Helper()
	c := byLabel(t, a.get(t, a.master), "Ragna")
	return c.GetDeathSuccesses(), c.GetDeathFailures()
}

// A failed death save with the resource stands the character at half the maximum and spends the use; without the resource it
// does not (the failure counts); a natural 1 fires the hook once.
func TestAFailedDeathSaveRunsTheHookOnlyWithTheResource(t *testing.T) {
	t.Parallel()
	t.Run("with the resource", func(t *testing.T) {
		t.Parallel()
		a, e := dyingRagna(t)
		res, err := a.deathSave(t, a.caio, e, "Ragna", deathFace(5))
		if err != nil {
			t.Fatalf("RollDeathSave() error = %v", err)
		}
		v := a.vitals(t, a.toren)
		if v.GetHitPointsCurrent() != v.GetHitPointsMax()/2 || usedOf(v, "folego_final") != 1 {
			t.Errorf("Ragna has %d of %d hit points and %d uses spent; want half the maximum and 1", v.GetHitPointsCurrent(), v.GetHitPointsMax(), usedOf(v, "folego_final"))
		}
		if s, f := ragnaCounts(t, a); s != 0 || f != 0 {
			t.Errorf("the counts are %d/%d, want 0/0: she stood up", s, f)
		}
		if h := res.GetDeathSave().GetHook(); h.GetSourceNamePt() != lastGaspPt || h.GetHealed() != v.GetHitPointsMax()/2 || h.GetSpentResource() != "folego_final" {
			t.Errorf("the answer's hook = %v, want %q healing half the maximum", h, lastGaspPt)
		}
		if s := combatantState(a.get(t, a.caio), "Ragna"); s == playv1.CombatantState_COMBATANT_STATE_DOWN {
			t.Error("Ragna is still down")
		}
	})
	t.Run("without the resource", func(t *testing.T) {
		t.Parallel()
		a, e := dyingRagna(t)
		fxSpendOf(t, a, "folego_final", 1)
		res, err := a.deathSave(t, a.caio, e, "Ragna", deathFace(5))
		if err != nil {
			t.Fatalf("RollDeathSave() error = %v", err)
		}
		if v := a.vitals(t, a.toren); v.GetHitPointsCurrent() != 0 || usedOf(v, "folego_final") != 1 {
			t.Errorf("Ragna has %d hit points and %d uses spent; want 0 and 1 (the hook did not run)", v.GetHitPointsCurrent(), usedOf(v, "folego_final"))
		}
		if s, f := ragnaCounts(t, a); s != 0 || f != 1 || res.GetDeathSave().GetHook() != nil {
			t.Errorf("counts %d/%d and hook %v; want 0/1 and none", s, f, res.GetDeathSave().GetHook())
		}
	})
	t.Run("a natural 1 fires it once, before the failures", func(t *testing.T) {
		t.Parallel()
		a, e := dyingRagna(t)
		if _, err := a.deathSave(t, a.caio, e, "Ragna", deathFace(1)); err != nil {
			t.Fatalf("RollDeathSave() error = %v", err)
		}
		v := a.vitals(t, a.toren)
		if s, f := ragnaCounts(t, a); s != 0 || f != 0 || usedOf(v, "folego_final") != 1 || v.GetHitPointsCurrent() == 0 {
			t.Errorf("after a natural 1: counts %d/%d, %d uses spent, %d hit points; want 0/0, 1 and above 0", s, f, usedOf(v, "folego_final"), v.GetHitPointsCurrent())
		}
	})
	t.Run("the hook before the third failure keeps her alive", func(t *testing.T) {
		t.Parallel()
		a, e := dyingRagna(t)
		fxSpendOf(t, a, "folego_final", 1) // out of uses: two failures count
		for range 2 {
			if _, err := a.deathSave(t, a.caio, e, "Ragna", deathFace(5)); err != nil {
				t.Fatalf("RollDeathSave() error = %v", err)
			}
			fxNextTurn(t, a, e)
		}
		if _, f := ragnaCounts(t, a); f != 2 {
			t.Fatalf("failures = %d, want 2", f)
		}
		fxSpendOf(t, a, "folego_final", 0) // the use is back
		if _, err := a.deathSave(t, a.caio, e, "Ragna", deathFace(5)); err != nil {
			t.Fatalf("RollDeathSave() error = %v", err)
		}
		if _, f := ragnaCounts(t, a); f != 0 {
			t.Errorf("failures = %d, want 0: the hook stood her up before the third failure was added", f)
		}
		if v := a.vitals(t, a.toren); v.GetHitPointsCurrent() == 0 {
			t.Error("Ragna is still at 0 hit points")
		}
	})
}

// A hook on dropping to 0 hit points that heals keeps the character up, spending its use; massive damage kills outright whatever
// the hook is; without the use the damage drops her.
func TestDroppingToZeroRunsTheHookExceptForMassiveDamage(t *testing.T) {
	t.Parallel()
	setup := func(t *testing.T) (*armed, *playv1.Encounter) {
		t.Helper()
		a := fxTable(t, fxTrait("Teimosia da Mesa",
			fxPool("teimosia", "1", "long_rest"),
			&rulesv1.TableEffect{Type: "on_event", Event: "drop_to_zero", Resource: "teimosia", HealPercentMax: 25, TextPt: "Teimosia de teste."},
		))
		e := fxFight(t, a)
		a.hurt(t, a.toren, 5)
		e = a.passTo(t, e, "Goblin")
		return a, e
	}
	t.Run("the hook keeps her up", func(t *testing.T) {
		t.Parallel()
		a, e := setup(t)
		id := goblinHitsRagna(t, a, e, 3) // 3 + 2 = 5: exactly to 0
		if _, err := a.settle(t, a.master, e, id, true); err != nil {
			t.Fatalf("ApplyPendingDamage() error = %v", err)
		}
		v := a.vitals(t, a.toren)
		if v.GetHitPointsCurrent() != v.GetHitPointsMax()/4 || usedOf(v, "teimosia") != 1 {
			t.Errorf("Ragna has %d of %d hit points and %d uses spent; want a quarter of the maximum and 1", v.GetHitPointsCurrent(), v.GetHitPointsMax(), usedOf(v, "teimosia"))
		}
		if s := combatantState(a.get(t, a.caio), "Ragna"); s == playv1.CombatantState_COMBATANT_STATE_DOWN {
			t.Error("Ragna is down after the hook")
		}
	})
	t.Run("massive damage is not prevented", func(t *testing.T) {
		t.Parallel()
		a, e := setup(t)
		id := goblinHitsRagna(t, a, e, 3)
		hpMax := a.vitals(t, a.toren).GetHitPointsMax()
		if _, err := a.master.combat.ApplyPendingDamage(t.Context(), connect.NewRequest(&playv1.ApplyPendingDamageRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), PendingDamageId: id, IdempotencyKey: newKey(), Amount: new(5 + hpMax),
		})); err != nil {
			t.Fatalf("ApplyPendingDamage() error = %v", err)
		}
		if v := a.vitals(t, a.toren); v.GetHitPointsCurrent() != 0 || usedOf(v, "teimosia") != 0 {
			t.Errorf("Ragna has %d hit points and %d uses spent; want 0 and 0: instant death is not a drop the hook answers", v.GetHitPointsCurrent(), usedOf(v, "teimosia"))
		}
	})
	t.Run("without the use the damage drops her", func(t *testing.T) {
		t.Parallel()
		a, e := setup(t)
		fxSpendOf(t, a, "teimosia", 1)
		id := goblinHitsRagna(t, a, e, 3)
		if _, err := a.settle(t, a.master, e, id, true); err != nil {
			t.Fatalf("ApplyPendingDamage() error = %v", err)
		}
		if v := a.vitals(t, a.toren); v.GetHitPointsCurrent() != 0 {
			t.Errorf("Ragna has %d hit points, want 0", v.GetHitPointsCurrent())
		}
	})
}

// stubbornTable is the table of a "drop_to_zero" hook: Ragna has one use of a once-per-long-rest resource and, dropped to 0 hit
// points by damage, stands with a quarter of the maximum instead.
func stubbornTable(t *testing.T) *armed {
	t.Helper()
	return fxTable(t, fxTrait("Teimosia da Mesa",
		fxPool("teimosia", "1", "long_rest"),
		&rulesv1.TableEffect{Type: "on_event", Event: "drop_to_zero", Resource: "teimosia", HealPercentMax: 25, TextPt: "Teimosia de teste."},
	))
}

// trapHits puts a trap damage of amount points on Ragna in the open session, waiting for the master, and applies it.
func trapHits(t *testing.T, a *armed, amount int32) {
	t.Helper()
	session := a.master.liveSession(t, a.campaignID).GetGameSession()
	d, err := a.h.svc.queries.InsertTrapDamage(t.Context(), playdb.InsertTrapDamageParams{
		GameSessionID: session.GetId(), TrapPointID: uuid.New().String(), FireID: uuid.New().String(), CharacterID: a.toren.GetId(),
		DiceCount: 0, DiceSides: 0, DiceBonus: amount, DamageType: "damage-type:piercing", Faces: []int32{}, RollTotal: amount, Amount: amount, CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("InsertTrapDamage() error = %v", err)
	}
	if _, err := a.master.play.ApplyTrapDamage(t.Context(), connect.NewRequest(&playv1.ApplyTrapDamageRequest{CampaignId: a.campaignID, TrapDamageId: d.ID, IdempotencyKey: newKey()})); err != nil {
		t.Fatalf("ApplyTrapDamage() error = %v", err)
	}
}

// Trap damage is damage: in a combat or outside one, a trap that drops the character to 0 hit points runs its drop_to_zero hook
// (the character stands with its result and the use is spent); damage that kills outright, or that finds the character already
// at 0, does not.
func TestTrapDamageRunsTheDropToZeroHook(t *testing.T) {
	t.Parallel()
	for _, inCombat := range []bool{true, false} {
		name := "outside a combat"
		if inCombat {
			name = "in a combat"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			a := stubbornTable(t)
			if inCombat {
				fxFight(t, a)
			}
			a.hurt(t, a.toren, 5)
			trapHits(t, a, 5)
			v := a.vitals(t, a.toren)
			if v.GetHitPointsCurrent() != v.GetHitPointsMax()/4 || usedOf(v, "teimosia") != 1 {
				t.Errorf("after the trap Ragna has %d of %d hit points and %d uses spent; want a quarter of the maximum and 1", v.GetHitPointsCurrent(), v.GetHitPointsMax(), usedOf(v, "teimosia"))
			}
		})
	}
	t.Run("massive damage kills outright", func(t *testing.T) {
		t.Parallel()
		a := stubbornTable(t)
		a.hurt(t, a.toren, 5)
		trapHits(t, a, 5+a.vitals(t, a.toren).GetHitPointsMax())
		if v := a.vitals(t, a.toren); v.GetHitPointsCurrent() != 0 || usedOf(v, "teimosia") != 0 {
			t.Errorf("after massive trap damage Ragna has %d hit points and %d uses spent; want 0 and 0", v.GetHitPointsCurrent(), usedOf(v, "teimosia"))
		}
	})
	t.Run("a character already at 0 is not raised by a hook", func(t *testing.T) {
		t.Parallel()
		a := stubbornTable(t)
		fxFight(t, a)
		a.correct(t, a.toren, hpIs(0))
		if _, err := a.h.pool.Exec(t.Context(), `UPDATE combatants SET death_failures = 3 WHERE id = $1`, a.id(t, "Ragna")); err != nil {
			t.Fatalf("set the failed saves: %v", err)
		}
		trapHits(t, a, 4)
		if v := a.vitals(t, a.toren); v.GetHitPointsCurrent() != 0 || usedOf(v, "teimosia") != 0 {
			t.Errorf("a dying Ragna has %d hit points and %d uses spent after the trap; want 0 and 0", v.GetHitPointsCurrent(), usedOf(v, "teimosia"))
		}
	})
}

// The master's own hit point adjustment ("Ajustar PV") is a correction, not something that happened in the fiction: it never runs the hook.
func TestTheMastersAdjustmentToZeroDoesNotRunTheHook(t *testing.T) {
	t.Parallel()
	// AdjustCombatantHitPoints refuses a player's character: AdjustCharacterVitals is the only correction there is.
	t.Run("the vitals", func(t *testing.T) {
		t.Parallel()
		a := stubbornTable(t)
		fxFight(t, a)
		a.hurt(t, a.toren, 5)
		a.hurt(t, a.toren, 0)
		if v := a.vitals(t, a.toren); v.GetHitPointsCurrent() != 0 || usedOf(v, "teimosia") != 0 {
			t.Errorf("Ragna has %d hit points and %d uses spent after the correction; want 0 and 0", v.GetHitPointsCurrent(), usedOf(v, "teimosia"))
		}
	})
}

// A dead ally is not healed by a table action: the use is refused with TARGET_DEAD and nothing is spent.
func TestAHealingActionRefusesADeadAlly(t *testing.T) {
	t.Parallel()
	a := fxTable(t, fxTrait("Fôlego da Mesa",
		fxPool(poolKey, "5", "long_rest"),
		&rulesv1.TableEffect{Type: "grant_action", Economy: "action", Resource: poolKey, Cost: "1", Heal: "1d6", TextPt: "Cura de teste."},
	))
	e := fxFight(t, a)
	a.correct(t, a.bri, hpIs(0))
	if _, err := a.h.pool.Exec(t.Context(), `UPDATE combatants SET death_failures = 3 WHERE id = $1`, a.id(t, "Brisa")); err != nil {
		t.Fatalf("set the failed saves: %v", err)
	}
	_, err := fxUse(t, a, e, fxActionKey(t, a, e, "Fôlego da Mesa"), func(r *playv1.TakeActionRequest) {
		fxSpend(0)(r)
		r.HealTargetCombatantId = a.id(t, "Brisa")
	})
	wantEncounterBlocked(t, err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TARGET_DEAD)
	if got := usedOf(a.vitals(t, a.toren), poolKey); got != 0 {
		t.Errorf("the refused heal spent %d points", got)
	}
	if byLabel(t, a.get(t, a.caio), "Ragna").GetActionUsed() {
		t.Error("the refused heal spent the action")
	}
}
