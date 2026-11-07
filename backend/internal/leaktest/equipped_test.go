package leaktest

import (
	"testing"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// Whether a player's free object interaction is spent this turn (Combatant.object_interaction_used) is the owner's and the
// master's. Another player reads no word of it: the owner's and the master's reads are the positive controls.
func TestAPlayersSpentObjectInteractionIsTheOwnersAndTheMastersAlone(t *testing.T) {
	w := newWorld(t)
	ctx := t.Context()
	m := w.master
	sheet := w.fullSheet("class:fighter", "race:human", 3)
	sheet.GetFull().WeaponKeys = []string{"equipment:dagger"}
	must(m.characters.UpdateCharacter(ctx, rq(&charactersv1.UpdateCharacterRequest{
		CampaignId: w.campaign, CharacterId: w.toren.GetId(), Revision: w.toren.GetRevision(), Name: w.toren.GetName(), Sheet: sheet,
	})))
	toren := w.combatant(w.toren)
	for range 40 {
		cur := must(m.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
		if cur.GetCurrentCombatantId() == toren.GetId() {
			w.encounter = cur
			break
		}
		must(m.combat.EndTurn(ctx, rq(&playv1.EndTurnRequest{CampaignId: w.campaign, EncounterId: cur.GetId(), IdempotencyKey: newKey(), ExpectedCombatantId: cur.GetCurrentCombatantId(), ExpectedRound: cur.GetRound()})))
	}
	if w.encounter.GetCurrentCombatantId() != toren.GetId() {
		t.Fatal("it never became Toren's turn")
	}
	// Caio stows the dagger: the first weapon of the turn is free, and the free interaction is spent.
	must(w.caio.characters.SetEquipped(ctx, rq(&charactersv1.SetEquippedRequest{
		CampaignId: w.campaign, CharacterId: w.toren.GetId(), Key: "equipment:dagger", Equipped: false,
	})))
	w.secrets.add(&canary{needle: `"objectInteractionUsed"`, kind: "object-interaction-used"})

	read := func(p *person) *playv1.Combatant {
		t.Helper()
		e := must(p.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
		return w.combatantOf(e, w.toren.GetId())
	}
	for _, p := range []*person{w.caio, m} {
		if !read(p).GetObjectInteractionUsed() {
			t.Errorf("%s reads the free object interaction as unspent, want spent", p.name)
		}
	}
	if read(w.ana).GetObjectInteractionUsed() {
		t.Error("Ana reads Toren's spent object interaction")
	}
	for _, p := range []*person{w.ana, w.pending} {
		for _, r := range w.reads(p) {
			for _, f := range w.inspect(p, r, nil) {
				t.Errorf("%s: %s", p.name, f)
			}
		}
	}
	found := false
	for _, r := range w.reads(m) {
		found = found || contains(r.body, `"objectInteractionUsed"`)
	}
	if !found {
		t.Error("the master's reads do not hold the field: the canary is unreachable")
	}
}

// The gear a sheet lists (DerivedSheet.gear) and the armor class a change would give (armor_class_if_toggled) are the
// owner's and the master's, as the sheet is. Another player and a pending member read no word of it: the field names
// of the list are the needles, and the owner's and the master's reads are the positive controls.
func TestAPlayersGearListIsTheOwnersAndTheMastersAlone(t *testing.T) {
	w := newWorld(t)
	ctx := t.Context()
	m := w.master
	sheet := w.fullSheet("class:fighter", "race:human", 3)
	// Body armor, a shield and a two-handed bow in hand: the bow's stowing would give the shield back, so the list holds
	// an armor_class_if_toggled that is not zero.
	sheet.GetFull().ArmorKey, sheet.GetFull().Shield = "equipment:chain-mail", true
	sheet.GetFull().WeaponKeys = []string{"equipment:dagger", "equipment:longbow"}
	must(m.characters.UpdateCharacter(ctx, rq(&charactersv1.UpdateCharacterRequest{
		CampaignId: w.campaign, CharacterId: w.toren.GetId(), Revision: w.toren.GetRevision(), Name: w.toren.GetName(), Sheet: sheet,
	})))

	reads := func(p *person) []reply {
		return append(w.reads(p),
			p.call(charactersv1connect.CharacterServiceGetCharacterProcedure, &charactersv1.GetCharacterRequest{CampaignId: w.campaign, CharacterId: w.toren.GetId()}),
			p.call(charactersv1connect.CharacterServiceListCharactersProcedure, &charactersv1.ListCharactersRequest{CampaignId: w.campaign}))
	}
	holds := func(r reply) bool {
		return contains(r.body, `"armorClassIfToggled"`) && contains(r.body, `"equipment:longbow"`)
	}
	for _, p := range []*person{w.caio, m} {
		found := false
		for _, r := range reads(p) {
			found = found || holds(r)
		}
		if !found {
			t.Errorf("%s: the gear list is not in any read: the canary is unreachable", p.name)
		}
	}
	for _, p := range []*person{w.ana, w.pending} {
		for _, r := range reads(p) {
			if contains(r.body, `"armorClassIfToggled"`) || contains(r.body, `"equipment:longbow"`) || contains(r.body, `"twoHanded"`) {
				t.Errorf("%s reads Toren's gear", p.name)
			}
			for _, f := range w.inspect(p, r, nil) {
				t.Errorf("%s: %s", p.name, f)
			}
		}
	}
}
