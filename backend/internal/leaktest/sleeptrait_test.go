package leaktest

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// Why Sleep goes past a creature (SRD 5.1, RN-10, RN-20): an undead, a creature immune to being charmed and an elf (Fey Ancestry:
// "magic can't put you to sleep"). The type, the immunity and the trait are the master's: a player reads only that the creature was
// not affected, for every reason alike; the creature's own player reads the trait of her own character. Ana's wizard casts Sleep over
// the skeleton, the armor (a construct immune to being charmed), Toren (an elf of Caio's) and an elf the master wrote as an NPC; the
// master casts the same and reads why (the positive controls).

const (
	sleepUndeadReason              = "Sem efeito em mortos-vivos"
	sleepCharmFreeReason           = "Sem efeito em quem é imune a ser enfeitiçado"
	feyAncestryWords               = "magia não faz você dormir"
	feyAncestryFullWords           = "Vantagem em testes de resistência contra ficar enfeitiçado; magia não faz você dormir."
	cannotSeeReason                = "Sem efeito em quem não enxerga"
	sleepSpellKey                  = "spell:sleep"
	colorSpraySpellKey             = "spell:color-spray"
	armorLabel, elfLabel           = "Armadura", "Elfo"
	sleepOriginCol, sleepOriginRow = 3, 5
)

// poolCast casts a spell of the pool as p for the caster over the area at the origin, rolling in the app, and answers the results by the
// target's label.
func (rt *rollsTable) poolCast(t *testing.T, p *person, caster, spell string, slot int32, area *playv1.CastSpellRequest) map[string]*playv1.SpellTargetResult {
	t.Helper()
	area.CampaignId, area.EncounterId, area.CasterId, area.SpellKey, area.Slot, area.IdempotencyKey = rt.campaign, rt.encounter.GetId(), rt.ids[caster], spell, &playv1.SpellSlot{Level: slot}, newKey()
	area.Roll = &playv1.CastSpellRequest_RollInApp{RollInApp: true}
	res := must(p.combat.CastSpell(t.Context(), rq(area)))
	body, err := protojson.Marshal(res)
	if err != nil {
		t.Fatalf("marshal the cast: %v", err)
	}
	r := reply{status: 200, body: body, msg: res}
	rt.got.keep(p, r)
	if p != rt.master {
		for _, f := range rt.inspect(p, r, nil) {
			t.Errorf("%s reads the cast of %s: %s", p.name, spell, f)
		}
	}
	out := map[string]*playv1.SpellTargetResult{}
	for _, tg := range res.GetCast().GetTargets() {
		for label, id := range rt.ids {
			if id == tg.GetCombatantId() {
				out[label] = tg
			}
		}
	}
	return out
}

// targetIsOwn is true for the result of a spell on the combatant of the caller's own character: its player reads the trait of it.
func targetIsOwn(w *world, p *person, m protoreflect.Message) bool {
	fd := m.Descriptor().Fields().ByName("combatant_id")
	if fd == nil || w.encounter == nil || p == w.master {
		return false
	}
	for _, c := range w.encounter.GetCombatants() {
		if c.GetId() == m.Get(fd).String() {
			return c.GetKind() == playv1.CombatantKind_COMBATANT_KIND_PLAYER && c.GetCharacterId() == w.ownCharacter(p).GetId()
		}
	}
	return false
}

func TestWhySleepGoesPastACreatureIsTheMastersAlone(t *testing.T) {
	rt := newRollsTableWith(t, func(rt *rollsTable) {
		w, ctx := rt.world, rt.t.Context()
		toren, pens := w.toren, w.pens
		full := toren.GetSheet().GetFull()
		full.RaceKey = "race:elf" // an elf of Caio's: Fey Ancestry
		full.FeatureChoiceKeys = nil
		must(w.caio.characters.UpdateCharacter(ctx, rq(&charactersv1.UpdateCharacterRequest{
			CampaignId: w.campaign, CharacterId: toren.GetId(), Revision: toren.GetRevision(), Name: toren.GetName(),
			Sheet: &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: full}},
		})))
		wizard := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
			BaseScores: &rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 14, Intelligence: 16, Wisdom: 10, Charisma: 10}, RaceKey: "race:human",
			Classes:        []*charactersv1.ClassLevel{{ClassKey: "class:wizard", Level: 7}},
			KnownSpellKeys: []string{sleepSpellKey, colorSpraySpellKey}, PreparedSpellKeys: []string{sleepSpellKey, colorSpraySpellKey}, WeaponKeys: []string{dagger},
		}}}
		must(w.ana.characters.UpdateCharacter(ctx, rq(&charactersv1.UpdateCharacterRequest{
			CampaignId: w.campaign, CharacterId: pens.GetId(), Revision: pens.GetRevision(), Name: pens.GetName(), Sheet: wizard,
		})))
	})
	w, m, caio, ana := rt.world, rt.master, rt.caio, rt.ana
	rt.buildRows()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("the table: %v", r)
		}
	}()
	ctx := t.Context()
	for _, n := range []string{sleepUndeadReason, sleepCharmFreeReason, feyAncestryWords, feyAncestryFullWords, cannotSeeReason} {
		w.secrets.add(&canary{needle: n, kind: "sleep-reason"})
	}
	// The armor (a construct immune to being charmed) and an elf the master wrote join the combat, next to the skeleton.
	rt.encounter = rt.encounterOf(m)
	armor := must(m.combat.AddMonsters(ctx, rq(&playv1.AddMonstersRequest{
		CampaignId: w.campaign, EncounterId: rt.encounter.GetId(), IdempotencyKey: newKey(), CreatureKey: "monster:animated-armor", Count: 1, Name: w.secrets.public("vis-" + armorLabel), Hidden: new(false),
	}))).GetCombatantIds()[0]
	elf := must(m.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
		CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_ENEMY, Name: w.secrets.public("vis-" + elfLabel),
		Sheet: &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
			BaseScores: &rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 10, Wisdom: 10, Charisma: 8}, RaceKey: "race:elf",
			Classes: []*charactersv1.ClassLevel{{ClassKey: "class:fighter", Level: 1}}, WeaponKeys: []string{dagger},
		}}},
	}))).GetCharacter()
	added := must(m.combat.AddCombatants(ctx, rq(&playv1.AddCombatantsRequest{
		CampaignId: w.campaign, EncounterId: rt.encounter.GetId(), IdempotencyKey: newKey(), Participants: []*playv1.Participant{{CharacterId: elf.GetId(), Hidden: new(false)}},
	}))).GetEncounter()
	rt.ids[armorLabel] = armor
	for _, c := range added.GetCombatants() {
		if c.GetCharacterId() == elf.GetId() {
			rt.ids[elfLabel] = c.GetId()
		}
	}
	for label, sq := range map[string][2]int32{armorLabel: {3, 6}, elfLabel: {4, 4}, "EscondidoA": {20, 12}} { // the hidden one is out of the sphere: a hidden hit has its own row (areaspell_test)
		must(m.combat.MoveCombatant(ctx, rq(&playv1.MoveCombatantRequest{CampaignId: w.campaign, EncounterId: rt.encounter.GetId(), CombatantId: rt.ids[label], IdempotencyKey: newKey(), Col: sq[0], Row: sq[1], Forced: true})))
	}
	for _, label := range []string{labelToren, labelPens, "Esqueleto", armorLabel, elfLabel} {
		w.secrets.id("sleep-combatant", rt.ids[label], ana, caio)
	}
	over := func() *playv1.CastSpellRequest {
		return &playv1.CastSpellRequest{Area: &playv1.CastSpellRequest_Origin{Origin: &playv1.SpellOrigin{Col: sleepOriginCol, Row: sleepOriginRow}}}
	}

	// Ana's Sleep: every creature the pool went past reads "not affected", with no reason.
	rt.toTurn(labelPens)
	rt.dice.ScriptFor(t, 8, 8, 8, 8, 8, 8, 8, 8, 8)
	ours := rt.poolCast(t, ana, labelPens, sleepSpellKey, 3, over())
	skipped := []string{"Esqueleto", armorLabel, labelToren, elfLabel}
	for _, label := range skipped {
		got := ours[label]
		if got == nil {
			t.Fatalf("Ana's cast does not list %s: %v", label, ours)
		}
		if got.GetEffect().GetOutcome() != playv1.SpellEffectOutcome_SPELL_EFFECT_OUTCOME_NOT_AFFECTED || got.GetNoEffectReasonMaster() != "" {
			t.Errorf("Ana reads %s as %v, reason %q; want not affected with no reason", label, got.GetEffect().GetOutcome(), got.GetNoEffectReasonMaster())
		}
	}
	rt.ask(t, "Sleep over an undead, a construct and two elves")
	rt.undoLast(t)

	// The master casts it again for Pensantus and reads why, for each.
	rt.dice.ScriptFor(t, 8, 8, 8, 8, 8, 8, 8, 8, 8)
	theirs := rt.poolCast(t, m, labelPens, sleepSpellKey, 3, over())
	for label, want := range map[string]string{"Esqueleto": sleepUndeadReason, armorLabel: sleepCharmFreeReason, labelToren: feyAncestryFullWords, elfLabel: feyAncestryFullWords} {
		if got := theirs[label].GetNoEffectReasonMaster(); got != want {
			t.Errorf("the master reads %q for %s, want %q (the positive control)", got, label, want)
		}
	}
	rt.ask(t, "the master cast Sleep over an undead, a construct and two elves")
	rt.undoLast(t)

	// Color Spray over a creature that cannot see: the armor is immune to being blinded, and has no sight. The cone comes out of Pensantus,
	// moved next to the armor.
	must(m.combat.MoveCombatant(ctx, rq(&playv1.MoveCombatantRequest{CampaignId: w.campaign, EncounterId: rt.encounter.GetId(), CombatantId: rt.ids[labelPens], IdempotencyKey: newKey(), Col: 6, Row: 6, Forced: true})))
	over = func() *playv1.CastSpellRequest {
		return &playv1.CastSpellRequest{Area: &playv1.CastSpellRequest_Direction{Direction: &playv1.SpellDirection{Dx: -1}}}
	}
	rt.dice.ScriptFor(t, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10)
	if got := rt.poolCast(t, ana, labelPens, colorSpraySpellKey, 3, over())[armorLabel]; got != nil && (got.GetNoEffectReasonMaster() != "" || got.GetEffect().GetOutcome() != playv1.SpellEffectOutcome_SPELL_EFFECT_OUTCOME_NOT_AFFECTED) {
		t.Errorf("Ana reads the armor under Color Spray as %v, want not affected with no reason", got)
	}
	rt.ask(t, "Color Spray over a construct that cannot see")
	rt.undoLast(t)
	rt.dice.ScriptFor(t, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10)
	if got := rt.poolCast(t, m, labelPens, colorSpraySpellKey, 3, over())[armorLabel].GetNoEffectReasonMaster(); got != cannotSeeReason {
		t.Errorf("the master reads %q for the armor under Color Spray, want %q (the positive control)", got, cannotSeeReason)
	}
	rt.ask(t, "the master cast Color Spray over a construct that cannot see")

	for _, p := range []*person{ana, caio} {
		for what, body := range map[string]string{"the combat": protojson.Format(rt.encounterOf(p)), "the log": protojson.Format(rt.logOf(p))} {
			for _, word := range []string{sleepUndeadReason, sleepCharmFreeReason, feyAncestryWords, cannotSeeReason} {
				if strings.Contains(body, word) {
					t.Errorf("%s reads %q in %s", p.name, word, what)
				}
			}
		}
	}
	t.Run("canaries are reachable", func(t *testing.T) { checkCanariesAreReachable(t, w, rt.got) })
}
