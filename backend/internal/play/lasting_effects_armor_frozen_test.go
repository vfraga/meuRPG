package play

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/sessiontell"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// The characterization of GearChanged and ArmorWorn (what the sheet's armor and shield do to
// Mage Armor) and of the armor class of a summoned creature. See lasting_core_frozen_test.go.

type coreArmorCall func(ctx context.Context, tx pgx.Tx, campaignID, characterID string, t sessiontell.Tell) error

// coreArmor runs the call in a transaction the way the characters service does, runs what it
// asked to tell after the commit, and says whether it asked for anything.
func (a *armed) coreArmor(t *testing.T, call coreArmorCall, characterID string) string {
	t.Helper()
	var told hints
	err := db.InTx(t.Context(), a.h.pool, func(tx pgx.Tx) error {
		told = hints{}
		return call(t.Context(), tx, a.campaignID, characterID, &SessionTell{s: a.h.svc, h: &told, campaignID: a.campaignID})
	})
	if err != nil {
		return "error: " + err.Error()
	}
	if len(told.afters) == 0 {
		return "no callback"
	}
	for _, step := range told.afters {
		step(t.Context(), a.h.svc.sayNow)
	}
	return "callback"
}

// coreBaseRow puts a row of an effect with a base armor class on a character out of a combat.
func (a *armed) coreBaseRow(t *testing.T, characterID, modifiers string) {
	t.Helper()
	a.execSQL(t, `INSERT INTO character_effects (campaign_id, character_id, group_id, source_key, source_kind, modifiers, duration_kind, seconds_left)
		VALUES ($1, $2, gen_random_uuid(), 'spell:mage-armor', 'spell', $3, 'rounds', 28800)`, a.campaignID, characterID, modifiers)
}

// The base armor class Mage Armor gives is worked out again from the gear: a row that changes
// is written again (with the combatants' and the vitals'), one that does not, or a character that
// wears armor, leaves everything as it was and asks for no callback.
func TestLastingCoreGearChangedWorksTheBaseOutAgain(t *testing.T) {
	t.Parallel()
	a := newCastingParty(t)
	var l coreLog
	gear := func(label, what string, c *charactersv1.Character) {
		t.Helper()
		l.say("%s: GearChanged -> %s", what, a.coreArmor(t, a.h.svc.GearChanged, c.GetId()))
		l.say("%s\n%s", a.coreVitals(t, label, c), a.coreCharacterEffects(t, c.GetId()))
	}
	gear("Toren", "no effects", a.toren)
	a.coreBaseRow(t, a.pens.GetId(), `[{"kind":"base_ac","value":13},{"kind":"ac_bonus","value":1}]`)
	gear("Pensantus", "a base of 13 for Dexterity 14, and a bonus beside it", a.pens)
	gear("Pensantus", "the same again", a.pens)
	// In a combat the states are read too, and the combatants hold the base.
	e := a.closeFight(t)
	a.mustAddEffect(t, e, mageArmorKey, []string{"Pensantus"}, nil)
	l.say("in a combat, Mage Armor added:\n%s%s", a.coreEffects(t, e.GetId()), a.coreCombatants(t, e.GetId(), "Pensantus"))
	l.say("GearChanged, nothing changed -> %s", a.coreArmor(t, a.h.svc.GearChanged, a.pens.GetId()))
	// The state and the combatant hold a base the gear no longer gives.
	a.execSQL(t, `UPDATE combatant_states SET modifiers = '[{"kind":"base_ac","value":13}]' WHERE kind = 'effect' AND source_key = 'spell:mage-armor'`)
	a.execSQL(t, `UPDATE combatants SET mage_armor_ac = 13 WHERE id = $1`, a.id(t, "Pensantus"))
	l.say("GearChanged -> %s", a.coreArmor(t, a.h.svc.GearChanged, a.pens.GetId()))
	l.say("%s%s", a.coreEffects(t, e.GetId()), a.coreCombatants(t, e.GetId(), "Pensantus"))
	l.say("GearChanged again -> %s", a.coreArmor(t, a.h.svc.GearChanged, a.pens.GetId()))
	// Armor on the character: the base is not touched.
	c := a.bia.character(t, a.bri)
	sheet := c.GetSheet()
	sheet.GetFull().ArmorKey = leatherArmorKey
	if _, err := a.master.characters.UpdateCharacter(t.Context(), connect.NewRequest(&charactersv1.UpdateCharacterRequest{
		CampaignId: a.campaignID, CharacterId: c.GetId(), Revision: c.GetRevision(), Name: c.GetName(), Sheet: sheet,
	})); err != nil {
		t.Fatalf("UpdateCharacter() error = %v", err)
	}
	a.coreBaseRow(t, a.bri.GetId(), `[{"kind":"base_ac","value":13}]`)
	gear("Brisa", "armor worn", a.bri)
	// The combat over and the session closed: the callback has no combat to tell.
	if _, err := a.master.combat.EndEncounter(t.Context(), connect.NewRequest(&playv1.EndEncounterRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey()})); err != nil {
		t.Fatalf("EndEncounter() error = %v", err)
	}
	a.execSQL(t, `UPDATE character_effects SET modifiers = '[{"kind":"base_ac","value":9}]' WHERE character_id = $1`, a.pens.GetId())
	a.master.end(t, a.master.liveSession(t, a.campaignID).GetGameSession())
	l.say("no session open: GearChanged -> %s", a.coreArmor(t, a.h.svc.GearChanged, a.pens.GetId()))
	l.say("%s", a.coreCharacterEffects(t, a.pens.GetId()))
	wantLastingGolden(t, "lasting_core", "armor_gear_changed", a.coreNames(t, l.String()))
}

// A character that dons armor loses the effects that set a base armor class, in a combat or out
// of it, and the casting that held them ends.
func TestLastingCoreArmorWornEndsTheEffectsThatNeedNone(t *testing.T) {
	t.Parallel()
	a := newCastingParty(t)
	var l coreLog
	l.say("no effects: ArmorWorn -> %s", a.coreArmor(t, a.h.svc.ArmorWorn, a.toren.GetId()))
	cast := a.mageArmorOnPensantus(t)
	a.mustCastOut(t, a.bia, a.bri, blessKey, slotOfLevel(1), false, []*charactersv1.Character{a.pens})
	a.coreBaseRow(t, a.pens.GetId(), `[{"kind":"base_ac","value":13}]`) // a row with no casting behind it
	l.say("before:\n%s%s", a.coreVitals(t, "Pensantus", a.pens), a.coreCharacterEffects(t, a.pens.GetId()))
	l.say("ArmorWorn -> %s", a.coreArmor(t, a.h.svc.ArmorWorn, a.pens.GetId()))
	l.say("%s\n%scast status %v", a.coreVitals(t, "Pensantus", a.pens), a.coreCharacterEffects(t, a.pens.GetId()), a.castByID(t, a.master, cast.GetId()).GetStatus())
	l.say("ArmorWorn again -> %s", a.coreArmor(t, a.h.svc.ArmorWorn, a.pens.GetId()))
	// In a combat.
	e := a.closeFight(t)
	a.mustAddEffect(t, e, mageArmorKey, []string{"Pensantus"}, nil)
	a.mustAddEffect(t, e, "spell:haste", []string{"Pensantus"}, a.rounds(t, 5, "Pensantus"))
	l.say("in a combat:\n%s%s", a.coreEffects(t, e.GetId()), a.coreCombatants(t, e.GetId(), "Pensantus"))
	l.say("ArmorWorn -> %s", a.coreArmor(t, a.h.svc.ArmorWorn, a.pens.GetId()))
	l.say("%s%s", a.coreEffects(t, e.GetId()), a.coreCombatants(t, e.GetId(), "Pensantus"))
	// With no session open.
	if _, err := a.master.combat.EndEncounter(t.Context(), connect.NewRequest(&playv1.EndEncounterRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey()})); err != nil {
		t.Fatalf("EndEncounter() error = %v", err)
	}
	a.coreBaseRow(t, a.pens.GetId(), `[{"kind":"base_ac","value":13}]`)
	a.master.end(t, a.master.liveSession(t, a.campaignID).GetGameSession())
	l.say("no session open: ArmorWorn -> %s", a.coreArmor(t, a.h.svc.ArmorWorn, a.pens.GetId()))
	l.say("%s", a.coreCharacterEffects(t, a.pens.GetId()))
	wantLastingGolden(t, "lasting_core", "armor_worn", a.coreNames(t, l.String()))
}

// FINDING, frozen as it is: a creature with no sheet (a summoned one) would have a base of 13
// plus its initiative bonus, but every combatant row names a character (combatants.character_id
// is NOT NULL), so the branch is not reachable through the calls; it is called here with the
// combatant the way the code reads it.
func TestLastingCoreTheBaseArmorClassOfACreatureWithNoSheet(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	var l coreLog
	a.coreInTx(t, "", func(ctx context.Context, c *combatTx) error {
		content, err := a.h.svc.contentOf(ctx, c)
		if err != nil {
			return err
		}
		def, _ := content.CombatSpellEffect(mageArmorKey)
		for _, bonus := range []int32{3, -2, 40, -20} {
			mods, ok, err := a.h.svc.modifiersForTarget(ctx, c, def, playdb.Combatant{InitiativeBonus: bonus}, castOpts{})
			l.say("initiative bonus %d -> %v %v %v", bonus, mods, ok, err)
		}
		mods, ok, err := a.h.svc.modifiersForTarget(ctx, c, def, playdb.Combatant{CharacterID: a.pens.GetId()}, castOpts{})
		l.say("a wizard of Dexterity 14 -> %v %v %v", mods, ok, err)
		mods, ok, err = a.h.svc.modifiersForTarget(ctx, c, def, playdb.Combatant{CharacterID: a.toren.GetId()}, castOpts{})
		l.say("a fighter -> %v %v %v", mods, ok, err)
		return nil
	})
	wantLastingGolden(t, "lasting_core", "armor_no_sheet", a.coreNames(t, l.String()))
}
