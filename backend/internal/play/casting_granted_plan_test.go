package play

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
)

// The plan of a cast through a granted_spell effect, and how the combat's cast reads the option of
// one. The names are invented; these tests need no database.

const planFeat = "feat:graca@mesa"

// grantRoster answers the grant's own numbers.
type grantRoster struct{ noRoster }

func (grantRoster) GrantedCombatSpell(_ context.Context, _ pgx.Tx, _, _, key, _ string, _ int, _ string) (link.Spell, error) {
	return link.Spell{Key: key, ToHit: 7, SaveDC: 15, CasterDC: 15}, nil
}

func grantPlan(grants ...link.OutsideGrant) castPlan {
	return castPlan{
		g:   castGuard{caster: link.Character{ID: "c", Player: true}},
		osp: link.OutsideSpell{Spell: link.Spell{Key: "spell:bless", Level: 1}, Ritual: true, CastMinutes: 0, RitualMinutes: 10, Grants: grants},
	}
}

func readyGrant(edit ...func(*link.OutsideGrant)) link.OutsideGrant {
	g := link.OutsideGrant{Source: planFeat, AtWill: true, Ready: true}
	for _, e := range edit {
		e(&g)
	}
	return g
}

func planOf(t *testing.T, plan castPlan, source string, slot *playv1.SpellSlot) (castPlan, error) {
	t.Helper()
	s := &Service{roster: grantRoster{}}
	return s.planGranted(t.Context(), &combatTx{}, authz.Membership{}, plan, source, slot)
}

func TestAGrantedCastTakesNoSlotAndUsesTheGrantsNumbers(t *testing.T) {
	t.Parallel()
	plan, err := planOf(t, grantPlan(readyGrant(func(g *link.OutsideGrant) { g.AtWill, g.Resource, g.UsesLeft = false, "usos", 1 })), planFeat, nil)
	if err != nil {
		t.Fatalf("planGranted() error = %v", err)
	}
	if plan.slot != nil || plan.grant == nil || plan.grant.Resource != "usos" || plan.osp.Spell.SaveDC != 15 || plan.osp.Spell.ToHit != 7 {
		t.Errorf("plan = %+v, want no slot, the grant to spend and the grant's numbers", plan)
	}
}

func TestAGrantedCastRefusesWhatTheGrantDoesNotAllow(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		grants []link.OutsideGrant
		source string
		slot   *playv1.SpellSlot
		ritual bool
		code   connect.Code
	}{
		{"a grant that is not one", []link.OutsideGrant{readyGrant()}, "feat:outra@mesa", nil, false, connect.CodeInvalidArgument},
		{"a slot the grant does not allow", []link.OutsideGrant{readyGrant()}, planFeat, &playv1.SpellSlot{Level: 1}, false, connect.CodeInvalidArgument},
		{"a ritual a grant does not cast", []link.OutsideGrant{readyGrant()}, planFeat, nil, true, connect.CodeInvalidArgument},
		{"a normal cast of a ritual-only grant", []link.OutsideGrant{readyGrant(func(g *link.OutsideGrant) { g.RitualOnly = true })}, planFeat, nil, false, connect.CodeInvalidArgument},
		{"no use left", []link.OutsideGrant{readyGrant(func(g *link.OutsideGrant) { g.AtWill, g.Ready, g.Resource = false, false, "usos" })}, planFeat, nil, false, connect.CodeFailedPrecondition},
	}
	for _, tc := range cases {
		plan := grantPlan(tc.grants...)
		plan.ritual = tc.ritual
		if _, err := planOf(t, plan, tc.source, tc.slot); connect.CodeOf(err) != tc.code {
			t.Errorf("%s: code = %v (%v), want %v", tc.name, connect.CodeOf(err), err, tc.code)
		}
	}
}

func TestAGrantWithASlotSpendsNoUse(t *testing.T) {
	t.Parallel()
	plan := grantPlan(readyGrant(func(g *link.OutsideGrant) { g.AlsoWithSlot, g.AtWill, g.Resource = true, false, "usos" }))
	plan.osp.Slots = []link.CastSlot{{Level: 1, Free: 2}}
	got, err := planOf(t, plan, planFeat, &playv1.SpellSlot{Level: 1})
	if err != nil {
		t.Fatalf("planGranted() error = %v", err)
	}
	if got.slot == nil || got.slot.Level != 1 || got.grant != nil {
		t.Errorf("plan = %+v, want the slot and no grant to spend", got)
	}
}

func TestARitualOnlyGrantCastsAsARitual(t *testing.T) {
	t.Parallel()
	plan := grantPlan(readyGrant(func(g *link.OutsideGrant) { g.RitualOnly = true }))
	plan.ritual = true
	got, err := planOf(t, plan, planFeat, nil)
	if err != nil {
		t.Fatalf("planGranted() error = %v", err)
	}
	if got.minutes != 10 || got.slot != nil {
		t.Errorf("plan = %+v, want the ritual's 10 minutes and no slot", got)
	}
	plan.osp.Ritual = false
	if _, err := planOf(t, plan, planFeat, nil); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("a grant on a spell with no ritual tag: code = %v, want failed_precondition", connect.CodeOf(err))
	}
}

func TestACantripThroughAGrantTakesNoSlot(t *testing.T) {
	t.Parallel()
	plan := grantPlan(readyGrant(func(g *link.OutsideGrant) { g.AlsoWithSlot = true }))
	plan.osp.Spell.Level = 0
	if _, err := planOf(t, plan, planFeat, &playv1.SpellSlot{Level: 1}); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a cantrip with a slot: code = %v, want invalid_argument", connect.CodeOf(err))
	}
}

func TestACombatCastPicksTheOptionOfItsGrant(t *testing.T) {
	t.Parallel()
	opts := &rulesv1.TurnOptions{Spells: []*rulesv1.SpellOption{
		{Spell: &rulesv1.Spell{Key: "spell:bless", Level: 1}, Enabled: true, Slots: []*rulesv1.SlotChoice{{Level: 1, Free: 2}}},
		{Spell: &rulesv1.Spell{Key: "spell:bless", Level: 1}, Enabled: true, Grant: &rulesv1.SpellGrantOption{SourceKey: planFeat, UsesLeft: 1, Resource: "usos"}},
	}}
	own, err := castableOf(opts, "spell:bless", "")
	if err != nil || own.grant != nil || len(own.slots) != 1 {
		t.Errorf("the class's option = %+v (%v), want the one with slots and no grant", own, err)
	}
	via, err := castableOf(opts, "spell:bless", planFeat)
	if err != nil || via.grant.GetResource() != "usos" {
		t.Errorf("the grant's option = %+v (%v), want the grant's", via, err)
	}
	if _, err := castableOf(opts, "spell:bless", "feat:outra@mesa"); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a grant that is not one: code = %v, want invalid_argument", connect.CodeOf(err))
	}
}

func TestACombatCastThroughAGrantIsCastAtItsOwnLevelWithNoSlot(t *testing.T) {
	t.Parallel()
	via := castable{level: 2, grant: &rulesv1.SpellGrantOption{SourceKey: planFeat}}
	if slot, err := slotOf(nil, via); err != nil || slot != nil {
		t.Errorf("no slot: slot = %v, err = %v, want a cast with none", slot, err)
	}
	if _, err := slotOf(&playv1.SpellSlot{Level: 2}, via); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a slot the grant does not allow: code = %v, want invalid_argument", connect.CodeOf(err))
	}
	via.grant.AlsoWithSlot, via.slots = true, []*rulesv1.SlotChoice{{Level: 3, Free: 1}}
	if slot, err := slotOf(&playv1.SpellSlot{Level: 3}, via); err != nil || slot == nil || slot.Level != 3 {
		t.Errorf("a slot the grant allows: slot = %v, err = %v, want the 3rd level", slot, err)
	}
	// A spell of the class's own still needs its slot.
	if _, err := slotOf(nil, castable{level: 2}); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a leveled spell of the class with no slot: code = %v, want invalid_argument", connect.CodeOf(err))
	}
}

// A cast with no slot through a grant that has a cast level reads the spell at that level, and the level is the
// plan's: the grant's cast level, the slot's when a slot casts it, the spell's own when the grant sets none.
func TestAGrantedCastReadsTheSpellAtTheGrantsCastLevel(t *testing.T) {
	t.Parallel()
	roster := &levelRoster{}
	s := &Service{roster: roster}
	plan := grantPlan(readyGrant(func(g *link.OutsideGrant) { g.CastLevel = 3 }))
	got, err := s.planGranted(t.Context(), &combatTx{}, authz.Membership{}, plan, planFeat, nil)
	if err != nil {
		t.Fatalf("planGranted() error = %v", err)
	}
	if roster.level != 3 || levelOf(got) != 3 {
		t.Errorf("the spell was read at level %d and the plan's level is %d, want 3 for both", roster.level, levelOf(got))
	}
	own, err := s.planGranted(t.Context(), &combatTx{}, authz.Membership{}, grantPlan(readyGrant()), planFeat, nil)
	if err != nil || roster.level != 1 || levelOf(own) != 1 {
		t.Errorf("a grant with no cast level: read at %d, plan level %d (%v), want the spell's own 1", roster.level, levelOf(own), err)
	}
	withSlot := grantPlan(readyGrant(func(g *link.OutsideGrant) { g.CastLevel, g.AlsoWithSlot = 2, true }))
	withSlot.osp.Slots = []link.CastSlot{{Level: 3, Free: 1}}
	slotted, err := s.planGranted(t.Context(), &combatTx{}, authz.Membership{}, withSlot, planFeat, &playv1.SpellSlot{Level: 3})
	if err != nil || levelOf(slotted) != 3 || roster.level != 3 {
		t.Errorf("a slot cast of a grant: plan level %d, read at %d (%v), want the slot's 3 for both", levelOf(slotted), roster.level, err)
	}
}

// levelRoster remembers the level a granted spell was read at.
type levelRoster struct {
	grantRoster
	level int
}

func (r *levelRoster) GrantedCombatSpell(_ context.Context, _ pgx.Tx, _, _, key, _ string, level int, _ string) (link.Spell, error) {
	r.level = level
	return link.Spell{Key: key, Level: 1}, nil
}

// The level of a combat cast through a grant: the grant's cast level with no slot, the slot's with one, the
// spell's own when the grant sets none.
func TestACombatCastThroughAGrantHasTheGrantsCastLevel(t *testing.T) {
	t.Parallel()
	via := castable{level: 1, grant: &rulesv1.SpellGrantOption{SourceKey: planFeat, CastLevel: 2, AlsoWithSlot: true}}
	if got := via.levelWith(nil); got != 2 {
		t.Errorf("no slot: level = %d, want the grant's 2", got)
	}
	if got := via.levelWith(&slotRef{Level: 4}); got != 4 {
		t.Errorf("a slot: level = %d, want the slot's 4", got)
	}
	via.grant.CastLevel = 0
	if got := via.levelWith(nil); got != 1 {
		t.Errorf("a grant with no cast level: level = %d, want the spell's own 1", got)
	}
	if got := (castable{level: 2}).levelWith(nil); got != 2 {
		t.Errorf("a spell of the class: level = %d, want its own 2", got)
	}
}
