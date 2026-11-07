package leaktest

import (
	"testing"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// What a persistent NPC kept of its uses a day (npc_spell_use.creature_spent) is the master's alone, as the creature's
// state in a combat is (RN-20, RN-29): a copy of the NPC that joins a combat with the uses already spent shows nothing of
// them to a player or the name of the NPC in a rest preview, kept (LimitedAction.kept) included: another player's copy has no limited action to carry it. The master reads the copy's uses left (the positive control).
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
	} else if !l.GetKept() {
		t.Errorf("the master reads kept = false on a persistent NPC's per-day action, want true (the positive control of kept)")
	}
	for _, p := range []*person{w.ana, w.caio} {
		for _, c := range p.currentEncounter(w).GetCombatants() {
			if len(c.GetLimitedActions()) != 0 {
				t.Errorf("%s reads %v on %s", p.name, c.GetLimitedActions(), c.GetLabel())
			}
		}
	}
	// The master's rest preview names the NPC whose kept uses a long rest gives back (the positive control) and, as the kept use is
	// a use a day, not one a short rest gives back; a player's call is refused, so no player copy carries the names.
	var name string
	if err := w.pool.QueryRow(ctx, `SELECT name FROM characters WHERE id = $1`, char).Scan(&name); err != nil {
		t.Fatalf("name of the deva: %v", err)
	}
	preview := func(p *person, kind playv1.RestKind) (*playv1.GetRestPreviewResponse, error) {
		res, err := p.resources.GetRestPreview(ctx, rq(&playv1.GetRestPreviewRequest{CampaignId: w.campaign, Kind: kind}))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	}
	if res, err := preview(w.master, playv1.RestKind_REST_KIND_LONG); err != nil || len(res.GetNpcsBack()) != 1 || res.GetNpcsBack()[0].GetName() != name || res.GetNpcsBack()[0].GetCharacterId() != char {
		t.Errorf("the master's long-rest preview names %v (err %v), want %s", res.GetNpcsBack(), err, name)
	}
	if res, err := preview(w.master, playv1.RestKind_REST_KIND_SHORT); err != nil || len(res.GetNpcsBack()) != 0 {
		t.Errorf("the master's short-rest preview names %v (err %v), want none: a use a day waits for the long rest", res.GetNpcsBack(), err)
	}
	if _, err := w.pool.Exec(ctx, `UPDATE npc_spell_use SET creature_spent = '{"rest:x":1}'::jsonb WHERE character_id = $1`, char); err != nil {
		t.Fatalf("keep a rest action: %v", err)
	}
	if res, err := preview(w.master, playv1.RestKind_REST_KIND_SHORT); err != nil || len(res.GetNpcsBack()) != 1 || res.GetNpcsBack()[0].GetName() != name {
		t.Errorf("the master's short-rest preview names %v (err %v), want %s: a rest action comes back on a short rest", res.GetNpcsBack(), err, name)
	}
	for _, p := range []*person{w.ana, w.caio} {
		if res, err := preview(p, playv1.RestKind_REST_KIND_LONG); err == nil || len(res.GetNpcsBack()) != 0 {
			t.Errorf("%s reads the rest preview of the table (err %v)", p.name, err)
		}
	}
	w.checkNoStatBlock(t, []reply{use}, nil, needles[0], needles[1], w.canaryNamed("creature-action-key"))
}

// A creature a player controls (a deva given to Caio's paladin) never keeps what it spent: its use a day ends with the combat, so
// Caio, who controls it, reads the action with its uses left and kept = false, as the master does; the other player reads no limited
// action on it at all (RN-29). Kept belongs only to a persistent NPC, and only the master reads that (the test above).
func TestTheControllerOfACreatureReadsItsUsesNeverKeptAndNoOtherPlayerReadsThem(t *testing.T) {
	rt := newRollsTableFog(t, true, func(rt *rollsTable) {
		w := rt.world
		must(w.master.characters.GiveCreature(t.Context(), rq(&charactersv1.GiveCreatureRequest{
			CampaignId: w.campaign, CharacterId: w.toren.GetId(), MonsterKey: "monster:deva", Name: "Devo",
		})))
	})
	w := rt.world
	w.encounter = rt.encounter
	const key = "monster:deva#healing-touch"
	devo := rt.combatantOf(w.master, "Devo")
	// A controlled creature's stat-block actions are not used through the API (only an NPC uses them), so the use is spent in the
	// creature's state, as the Web is in the roll tests.
	if _, err := w.pool.Exec(t.Context(), `UPDATE combatants SET creature_spent = jsonb_build_object($2::text, 1) WHERE id = $1`, devo.GetId(), "use:"+key); err != nil {
		t.Fatalf("spend a use: %v", err)
	}
	for _, p := range []*person{w.master, w.caio} {
		if l := w.limitedOf(p, devo, key); l == nil || l.GetUsesLeft() != 2 {
			t.Errorf("%s reads %v on the creature, want 2 uses left", p.name, l)
		} else if l.GetKept() {
			t.Errorf("%s reads kept on a controlled creature, which never keeps its uses", p.name)
		}
	}
	for _, c := range w.ana.currentEncounter(w).GetCombatants() {
		if len(c.GetLimitedActions()) != 0 {
			t.Errorf("Ana reads %v on %s", c.GetLimitedActions(), c.GetLabel())
		}
	}
}
