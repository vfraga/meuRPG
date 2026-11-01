package leaktest

import (
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// What a persistent NPC kept of its uses a day (npc_spell_use.creature_spent) is the master's alone, as the creature's
// state in a combat is (RN-20, RN-29): a copy of the NPC that joins a combat with the uses already spent shows nothing of
// them to a player. The master reads the copy's uses left (the positive control).
func TestAPersistentNPCsKeptUsesAreTheMastersAlone(t *testing.T) {
	w := newWorld(t)
	needles := w.limitedNeedles()
	const monster, action = "monster:deva", "healing-touch"
	w.statBlockNeedles(monster, action)
	deva := w.monsterOnTurn(monster, w.secrets.public("kept-uses-name"), false)
	ctx := t.Context()
	var char string
	if err := w.pool.QueryRow(ctx, `SELECT character_id FROM combatants WHERE id = $1`, deva.GetId()).Scan(&char); err != nil {
		t.Fatalf("character of the deva: %v", err)
	}
	if _, err := w.pool.Exec(ctx, `UPDATE characters SET sheet = jsonb_set(sheet, '{basic,combat_only}', 'false') WHERE id = $1`, char); err != nil {
		t.Fatalf("make the deva a persistent NPC: %v", err)
	}
	use := w.useAction(w.master, deva, monster+"#"+action)
	if !use.ok() {
		t.Fatalf("the use: status %d %s", use.status, use.body)
	}
	var kept bool
	if err := w.pool.QueryRow(ctx, `SELECT creature_spent <> '{}' FROM npc_spell_use WHERE character_id = $1`, char).Scan(&kept); err != nil || !kept {
		t.Fatalf("the NPC kept nothing (err %v): the test has nothing to hide", err)
	}

	hidden := false
	w.scriptD20(ownFace)
	e := must(w.master.combat.AddCombatants(ctx, rq(&playv1.AddCombatantsRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), IdempotencyKey: newKey(), Participants: []*playv1.Participant{{CharacterId: char, Hidden: &hidden}},
	}))).GetEncounter()
	var copyID string
	for _, c := range e.GetCombatants() {
		if c.GetCharacterId() == char && c.GetId() != deva.GetId() {
			copyID = c.GetId()
		}
	}
	if copyID == "" {
		t.Fatal("the second deva did not join")
	}
	w.encounter = e
	if l := w.limitedOf(w.master, &playv1.Combatant{Id: copyID}, monster+"#"+action); l == nil || l.GetUsesLeft() != 2 {
		t.Fatalf("the master reads %v on the copy, want 2 uses left", l)
	}
	for _, p := range []*person{w.ana, w.caio} {
		for _, c := range p.currentEncounter(w).GetCombatants() {
			if len(c.GetLimitedActions()) != 0 {
				t.Errorf("%s reads %v on %s", p.name, c.GetLimitedActions(), c.GetLabel())
			}
		}
	}
	w.checkNoStatBlock(t, []reply{use}, nil, needles[0], needles[1], w.canaryNamed("creature-action-key"))
}
