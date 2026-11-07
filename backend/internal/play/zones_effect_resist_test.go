package play

import (
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/prototext"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The damage of a zone takes the resistances, immunities and vulnerabilities of the lasting effects on the creature it hurts,
// as the damage of an attack does (RN-37, RN-10). These tests need the database (MEURPG_TEST_DATABASE_URL). The cave is the
// zone cave: Toren (a cleric) casts a table spell that leaves the modifier on the ogre, then Pensantus lays the zone on it.

const wardKey = "spell:guarda-de-nanquim@mesa"

func damageMod(kind rulesv1.TableResistanceKind, types ...string) *tableMod {
	return &tableMod{Kind: rulesv1.TableEffectModifierKind_TABLE_EFFECT_MODIFIER_KIND_RESISTANCE, ResistanceKind: kind, DamageTypes: types}
}

// wardCave is the zone cave where Toren prepared a table spell whose effect is mods.
func wardCave(t *testing.T, mods ...*tableMod) *cave {
	t.Helper()
	return newCaveFrom(t, func(a *armed) {
		ward := tableSpell("Guarda de Nanquim", 1, "class:cleric", rulesv1.SpellRangeKind_SPELL_RANGE_KIND_RANGED, tableTarget(rulesv1.TableSpellTargetKind_TABLE_SPELL_TARGET_KIND_CREATURE))
		ward.Concentration = true
		ward.Duration = &rulesv1.TableSpellDuration{Kind: rulesv1.SpellDurationKind_SPELL_DURATION_KIND_TIMED, Amount: 1, Unit: rulesv1.SpellDurationUnit_SPELL_DURATION_UNIT_MINUTE, UpTo: true}
		ward.Effect = &rulesv1.TableSpellEffect{Modifiers: mods, Visibility: rulesv1.TableEffectAudience_TABLE_EFFECT_AUDIENCE_PUBLIC}
		if _, err := a.master.table.CreateTableEntry(t.Context(), connect.NewRequest(&rulesv1.CreateTableEntryRequest{
			CampaignId: a.campaignID, Body: &rulesv1.CreateTableEntryRequest_TableSpell{TableSpell: ward},
		})); err != nil {
			t.Fatalf("CreateTableEntry(Guarda de Nanquim) error = %v", err)
		}
		a.toren = a.caio.caster(t, a.campaignID, "Toren", "class:cleric", "race:human", 7, abilities(14, 12, 14, 16, 10),
			[]string{maceKey}, []string{sacredFlame}, []string{wardKey}, []string{wardKey})
		book := []string{wallOfFireKey, cloudkillKey}
		a.pens = a.ana.caster(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 9, &rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8},
			nil, []string{fireBolt}, book, book)
		a.bri = a.bia.caster(t, a.campaignID, "Brisa", "class:cleric", "race:halfling", 2,
			&rulesv1.AbilityScores{Strength: 8, Dexterity: 16, Constitution: 14, Intelligence: 10, Wisdom: 16, Charisma: 8}, []string{maceKey}, []string{sacredFlame}, nil, []string{cureWounds})
	})
}

// wardOgre has Toren (first in the order) put the effect on the ogre and leaves Pensantus on turn.
func (c *cave) wardOgre(t *testing.T) *playv1.Encounter {
	t.Helper()
	e := c.zoneFight(t)
	c.mustCast(t, c.caio, e, "Toren", wardKey, slotOfLevel(1), c.at(t, "Ogro"), noCastRoll)
	c.passTo(t, e, "Pensantus")
	return e
}

func TestAnEffectsImmunityStopsAPoisonZone(t *testing.T) {
	t.Parallel()
	c := wardCave(t, damageMod(rulesv1.TableResistanceKind_TABLE_RESISTANCE_KIND_IMMUNE, "damage-type:poison"))
	c.wardOgre(t)
	c.h.roller.queue(5, 4, 4, 4, 4, 4) // the ogre's Constitution save fails, then 5d8 = 20
	c.mustCastArea(t, c.ana, "Pensantus", cloudkillKey, slotOfLevel(5), at(7, 7), nil)
	c.passTo(t, c.get(t, c.master), "Ogro")
	if cur, _, _ := c.hp(t, "Ogro"); cur != 59 {
		t.Errorf("the ogre has %d hit points, want 59: the effect makes it immune to poison", cur)
	}
}

// The control of the test above: with the same ward and no immunity the same cloud and dice hurt the ogre, so the 59 is the
// immunity and not a cloud that never caught it.
func TestAZoneHurtsAWardedOgreWithoutTheImmunity(t *testing.T) {
	t.Parallel()
	c := wardCave(t, damageMod(rulesv1.TableResistanceKind_TABLE_RESISTANCE_KIND_RESIST, "damage-type:cold"))
	c.wardOgre(t)
	c.h.roller.queue(5, 4, 4, 4, 4, 4)
	c.mustCastArea(t, c.ana, "Pensantus", cloudkillKey, slotOfLevel(5), at(7, 7), nil)
	c.passTo(t, c.get(t, c.master), "Ogro")
	if cur, _, _ := c.hp(t, "Ogro"); cur != 59-20 {
		t.Errorf("the ogre has %d hit points, want 39: a cold ward does not touch poison", cur)
	}
}

func TestAnEffectsResistanceHalvesAFireZone(t *testing.T) {
	t.Parallel()
	c := wardCave(t, damageMod(rulesv1.TableResistanceKind_TABLE_RESISTANCE_KIND_RESIST, "damage-type:fire"))
	c.wardOgre(t)
	c.h.roller.queue(5, 5, 5, 5, 5, 5) // the ogre's Dexterity save fails, then 5d8 = 25
	if _, err := c.castWall(t, 4, northWall(playv1.WallSide_WALL_SIDE_LEFT)); err != nil {
		t.Fatalf("CastSpell() error = %v", err)
	}
	if cur, _, _ := c.hp(t, "Ogro"); cur != 59-12 {
		t.Errorf("the ogre has %d hit points, want 47: the effect halves the 25 fire damage", cur)
	}
}

// torenInTheCloud puts the effect on Toren (who casts it on himself), has Pensantus lay Cloudkill on him and has his player fail
// the save, so a poison damage waits for the master. It answers the id of that damage.
func (c *cave) torenInTheCloud(t *testing.T, hide bool) (*playv1.Encounter, string) {
	t.Helper()
	e := c.zoneFight(t)
	c.mustCast(t, c.caio, e, "Toren", wardKey, slotOfLevel(1), c.at(t, "Toren"), noCastRoll)
	if hide {
		id := c.rpcEffectID(t, wardKey, "Toren")
		if _, err := c.rpcVisibility(t, c.master, e, id, func(r *playv1.SetLastingEffectVisibilityRequest) { r.PlayerVisible = false }); err != nil {
			t.Fatalf("SetLastingEffectVisibility() error = %v", err)
		}
	}
	c.passTo(t, e, "Pensantus")
	c.mustMove(t, c.master, "Toren", 15, 7) // far from Brisa and the ogre, so only Toren's player has a question to answer
	c.mustCastArea(t, c.ana, "Pensantus", cloudkillKey, slotOfLevel(5), at(15, 7), nil)
	c.passRaw(t, "Toren")
	w := c.windowOf(t, c.caio, playv1.ReactionKind_REACTION_KIND_EFFECT_SAVE)
	if w == nil {
		t.Fatal("Toren's player has no question")
	}
	if _, err := c.rollEffectSave(t, c.caio, c.get(t, c.master), w.GetId(), effectSaveFace(2)); err != nil {
		t.Fatalf("RollEffectSave() error = %v", err)
	}
	var id string
	if err := c.h.pool.QueryRow(t.Context(), `SELECT id FROM pending_damages WHERE target_id = $1 AND effect_source_key = $2`, c.id(t, "Toren"), cloudkillKey).Scan(&id); err != nil {
		t.Fatalf("read the zone's damage: %v", err)
	}
	return e, id
}

func stepsOf(p *playv1.PendingDamage) string {
	var b strings.Builder
	for _, s := range p.GetSteps() {
		b.WriteString(s.GetLabelPt() + " " + strings.Join(s.GetSourceKeys(), ",") + "|")
	}
	return b.String()
}

// A character's player is the one asked for the save; the damage then waits for the master, who reads it with the steps the
// effect gives, as the damage of an attack shows them (and gets 0 to apply: the effect makes the character immune).
func TestAnEffectsImmunityShowsInTheWaitingDamageOfAPlayersCharacter(t *testing.T) {
	t.Parallel()
	c := wardCave(t, damageMod(rulesv1.TableResistanceKind_TABLE_RESISTANCE_KIND_IMMUNE, "damage-type:poison"))
	e, id := c.torenInTheCloud(t, false)
	p, err := c.settle(t, c.master, e, id, false)
	if err != nil {
		t.Fatalf("DiscardPendingDamage() error = %v", err)
	}
	if p.GetAmount() <= 0 || p.GetAmountAfterSteps() != 0 || !strings.Contains(stepsOf(p), wardKey) {
		t.Errorf("pending = amount %d after the steps %d, steps %q; want the roll, 0 and the effect", p.GetAmount(), p.GetAmountAfterSteps(), stepsOf(p))
	}
}

// RN-10: an effect the master hides is not named to anybody but the master: the character's own player, who read the save, and
// another player read nothing of it, in the combat or the log, while the master reads the steps with the spell.
func TestAHiddenEffectsStepOfAZoneDamageReachesTheMasterAlone(t *testing.T) {
	t.Parallel()
	c := wardCave(t, damageMod(rulesv1.TableResistanceKind_TABLE_RESISTANCE_KIND_IMMUNE, "damage-type:poison"))
	e, id := c.torenInTheCloud(t, true)
	effectsOnToren := func(u *user) []*playv1.LastingEffect {
		return byLabel(t, c.get(t, u), "Toren").GetEffects()
	}
	if len(effectsOnToren(c.master)) == 0 {
		t.Fatal("the master reads no effect on Toren: the row proves nothing")
	}
	for _, u := range []*user{c.caio, c.ana} {
		if got := effectsOnToren(u); len(got) != 0 {
			t.Errorf("a player reads the hidden effect on Toren: %v", got)
		}
		for _, r := range c.log(t, u, c.get(t, c.master)).GetRounds() {
			for _, en := range r.GetEntries() {
				if en.GetDamage() != nil && strings.Contains(prototext.Format(en.GetDamage()), wardKey) {
					t.Errorf("a player's log names the hidden effect in a damage: %v", en)
				}
			}
		}
	}
	p, err := c.settle(t, c.master, e, id, false)
	if err != nil {
		t.Fatalf("DiscardPendingDamage() error = %v", err)
	}
	if !strings.Contains(stepsOf(p), wardKey) {
		t.Errorf("the master reads the steps %q, want the effect (the positive control)", stepsOf(p))
	}
}

func saveBonus(n int32) *tableMod {
	return &tableMod{
		Kind: rulesv1.TableEffectModifierKind_TABLE_EFFECT_MODIFIER_KIND_ROLL_BONUS, Value: n,
		AppliesTo: []rulesv1.TableEffectRollKind{rulesv1.TableEffectRollKind_TABLE_EFFECT_ROLL_KIND_SAVE},
	}
}

// The flat bonus of a roll_bonus effect counts in the saving throw a zone asks of an NPC, which the server rolls: a 12 and +5
// beat the DC 16 of Cloudkill (the ogre's own bonus is +0), and the pass halves the damage.
func TestARollBonusCountsInAZoneSaveOfAnNPC(t *testing.T) {
	t.Parallel()
	c := wardCave(t, saveBonus(5))
	c.wardOgre(t)
	c.h.roller.queue(12, 4, 4, 4, 4, 4) // the save, then 5d8 = 20
	c.mustCastArea(t, c.ana, "Pensantus", cloudkillKey, slotOfLevel(5), at(7, 7), nil)
	c.passTo(t, c.get(t, c.master), "Ogro")
	if cur, _, _ := c.hp(t, "Ogro"); cur != 59-10 {
		t.Errorf("the ogre has %d hit points, want 49: 12 and the bonus of 5 pass the save, so it takes half of 20", cur)
	}
}

// ...and in the question a zone asks of a player's character, answered with a typed die: the total adds the bonus.
func TestARollBonusCountsInAZoneSaveOfAPlayersCharacter(t *testing.T) {
	t.Parallel()
	c := wardCave(t, saveBonus(5))
	e := c.zoneFight(t)
	c.mustCast(t, c.caio, e, "Toren", wardKey, slotOfLevel(1), c.at(t, "Toren"), noCastRoll)
	c.passTo(t, e, "Pensantus")
	c.mustMove(t, c.master, "Toren", 15, 7)
	c.mustCastArea(t, c.ana, "Pensantus", cloudkillKey, slotOfLevel(5), at(15, 7), nil)
	c.passRaw(t, "Toren")
	w := c.windowOf(t, c.caio, playv1.ReactionKind_REACTION_KIND_EFFECT_SAVE)
	if w == nil {
		t.Fatal("Toren's player has no question")
	}
	res, err := c.rollEffectSave(t, c.caio, c.get(t, c.master), w.GetId(), effectSaveFace(12))
	if err != nil {
		t.Fatalf("RollEffectSave() error = %v", err)
	}
	if r := res.GetResult(); r.GetD20() != 12 || r.GetTotal() != 12+r.GetModifier() || r.GetModifier() < 5 {
		t.Errorf("result = %v, want the modifier to include the bonus of 5", r)
	}
}
