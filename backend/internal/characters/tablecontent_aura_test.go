package characters

import (
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// auraSpell is a self spell that lasts a minute and keeps an aura of allies around the caster that gives +1 on attack rolls.
func auraSpell(name string, mutate func(*rulesv1.TableSpell)) *rulesv1.TableSpell {
	s := effectTestSpell(name, &rulesv1.TableSpellEffect{
		Aura:          &rulesv1.TableEffectAura{RadiusFt: 15, Affects: rulesv1.TableAuraAffects_TABLE_AURA_AFFECTS_ALLIES},
		PlayerLabelPt: "Brasa em volta", Visibility: rulesv1.TableEffectAudience_TABLE_EFFECT_AUDIENCE_OWNER,
		Modifiers: []*rulesv1.TableEffectModifier{{
			Kind: rulesv1.TableEffectModifierKind_TABLE_EFFECT_MODIFIER_KIND_ROLL_BONUS, Value: 1,
			AppliesTo: []rulesv1.TableEffectRollKind{rulesv1.TableEffectRollKind_TABLE_EFFECT_ROLL_KIND_ATTACK},
		}},
	})
	s.Target = &rulesv1.TableSpellTarget{Kind: rulesv1.TableSpellTargetKind_TABLE_SPELL_TARGET_KIND_SELF}
	s.Range = &rulesv1.TableSpellRange{Kind: rulesv1.SpellRangeKind_SPELL_RANGE_KIND_SELF}
	if mutate != nil {
		mutate(s)
	}
	return s
}

// An aura is written, stored and read back as the master wrote it; a malformed one is refused at its own field (the
// positive control of each row is the good spell the round trip wrote first).
func TestTableContentSpellAuraRoundTripAndRefusals(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Samuel")
	campaign := h.newCampaign(master, "Mirathel")
	good := auraSpell("Couraça de brasa", nil)
	created := master.addEntry(t, campaign, good)
	if got := bodyMessage(master.entries(t, campaign)[created.GetKey()]).(*rulesv1.TableSpell).GetEffect(); !proto.Equal(got, good.GetEffect()) {
		t.Errorf("stored effect = %v, want %v", got, good.GetEffect())
	}
	enemies := rulesv1.TableAuraAffects_TABLE_AURA_AFFECTS_ENEMIES
	for _, tt := range []struct {
		name   string
		mutate func(*rulesv1.TableSpell)
		field  string
	}{
		{"a radius of zero", func(s *rulesv1.TableSpell) { s.Effect.Aura.RadiusFt = 0 }, "table_spell.effect.aura.radius_ft"},
		{"a radius over 120 ft", func(s *rulesv1.TableSpell) { s.Effect.Aura.RadiusFt = 125 }, "table_spell.effect.aura.radius_ft"},
		{"a radius off the squares", func(s *rulesv1.TableSpell) { s.Effect.Aura.RadiusFt = 12 }, "table_spell.effect.aura.radius_ft"},
		{"no side", func(s *rulesv1.TableSpell) { s.Effect.Aura.Affects = rulesv1.TableAuraAffects_TABLE_AURA_AFFECTS_UNSPECIFIED }, "table_spell.effect.aura.affects"},
		{"a side this build does not know", func(s *rulesv1.TableSpell) { s.Effect.Aura.Affects = 99 }, "table_spell.effect.aura.affects"},
		{"an instantaneous spell", func(s *rulesv1.TableSpell) {
			s.Duration = &rulesv1.TableSpellDuration{Kind: rulesv1.SpellDurationKind_SPELL_DURATION_KIND_INSTANTANEOUS}
		}, "table_spell.effect.aura"},
		{"a spell on another creature", func(s *rulesv1.TableSpell) {
			s.Target = &rulesv1.TableSpellTarget{Kind: rulesv1.TableSpellTargetKind_TABLE_SPELL_TARGET_KIND_CREATURE}
			s.Range = &rulesv1.TableSpellRange{Kind: rulesv1.SpellRangeKind_SPELL_RANGE_KIND_RANGED, DistanceFt: 30}
		}, "table_spell.effect.aura"},
		{"a condition", func(s *rulesv1.TableSpell) { s.Effect.Conditions = []string{"condition:frightened"} }, "table_spell.effect.conditions"},
		{"armor class", func(s *rulesv1.TableSpell) {
			s.Effect.Modifiers = []*rulesv1.TableEffectModifier{{Kind: rulesv1.TableEffectModifierKind_TABLE_EFFECT_MODIFIER_KIND_AC_BONUS, Value: 1}}
		}, "table_spell.effect.modifiers[0].kind"},
		{"temporary hit points", func(s *rulesv1.TableSpell) {
			s.Effect.Modifiers = []*rulesv1.TableEffectModifier{{Kind: rulesv1.TableEffectModifierKind_TABLE_EFFECT_MODIFIER_KIND_TEMP_HP, Value: 3}}
		}, "table_spell.effect.modifiers[0].kind"},
		{"speed", func(s *rulesv1.TableSpell) {
			s.Effect.Modifiers = []*rulesv1.TableEffectModifier{{Kind: rulesv1.TableEffectModifierKind_TABLE_EFFECT_MODIFIER_KIND_SPEED_ADD, Value: 10}}
		}, "table_spell.effect.modifiers[0].kind"},
	} {
		_, err := master.table.CreateTableEntry(t.Context(), connect.NewRequest(createReq(campaign, auraSpell("Brasa "+tt.name, tt.mutate))))
		v := violationsOfErr(t, err)
		if connect.CodeOf(err) != connect.CodeInvalidArgument || len(v) == 0 || v[0].GetField() != tt.field {
			t.Errorf("%s: error = %v, violations = %v, want one at %s", tt.name, err, v, tt.field)
		}
	}
	// Every side is accepted (the positive control of the unknown one).
	for _, side := range []rulesv1.TableAuraAffects{rulesv1.TableAuraAffects_TABLE_AURA_AFFECTS_SELF, rulesv1.TableAuraAffects_TABLE_AURA_AFFECTS_ALLIES, enemies} {
		master.addEntry(t, campaign, auraSpell("Brasa "+side.String(), func(s *rulesv1.TableSpell) { s.Effect.Aura.Affects = side }))
	}
}

// RN-10/RN-23: a player's copy of a table spell has no word of who sees its effect on the creature and no label for
// the other players (the master's choices), and keeps the rest; the master reads both (the positive control).
func TestAPlayersCopyOfATableSpellLeavesOutWhoSeesItsEffectAndItsLabel(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, ana := h.newUser("Samuel"), h.newUser("Ana")
	campaign := h.newCampaign(master, "Mirathel", ana)
	created := master.addEntry(t, campaign, auraSpell("Couraça de brasa", nil))
	effectOf := func(u *user) *rulesv1.TableSpellEffect {
		e := u.entries(t, campaign)[created.GetKey()]
		if e == nil {
			t.Fatalf("the spell is not listed for the reader")
		}
		return bodyMessage(e).(*rulesv1.TableSpell).GetEffect()
	}
	m := effectOf(master)
	if m.GetVisibility() != rulesv1.TableEffectAudience_TABLE_EFFECT_AUDIENCE_OWNER || m.GetPlayerLabelPt() != "Brasa em volta" {
		t.Fatalf("the master reads visibility %v and label %q, want both (the positive control)", m.GetVisibility(), m.GetPlayerLabelPt())
	}
	p := effectOf(ana)
	if p.GetVisibility() != rulesv1.TableEffectAudience_TABLE_EFFECT_AUDIENCE_UNSPECIFIED || p.GetPlayerLabelPt() != "" {
		t.Errorf("the player reads visibility %v and label %q, want neither", p.GetVisibility(), p.GetPlayerLabelPt())
	}
	if len(p.GetModifiers()) != len(m.GetModifiers()) || p.GetAura().GetRadiusFt() != 15 {
		t.Errorf("the player's copy lost what the spell does: %v", p)
	}
	if got := effectOf(master); got.GetVisibility() != m.GetVisibility() {
		t.Errorf("the master's copy changed after the player read it: %v", got)
	}
}
