package leaktest

import (
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The sunlight of a combat (#704, RN-20, RN-29) is the master's alone: whether it is on, where it reaches, the squares
// painted, which creature stands in it and which trait the creature has. The players read none of it, on a map they see
// in part or not at all, in their reads, their stream and the log; the one trace they may read is the "Outra fonte" of a
// roll that carries the source.
func TestTheSunlightIsTheMastersAloneOnAFoggedMap(t *testing.T) {
	z := newZoneTable(t, false)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("%v", r)
		}
	}()
	ctx := t.Context()
	master := z.master
	// A drow, which has Sunlight Sensitivity, in a square no player sees.
	z.encounter = z.enc
	e, drow := z.addMonsterWithATotalOfItsOwn("monster:drow", z.secrets.public("drow-name"), false)
	z.enc = e
	z.forceMove(drow.GetId(), casterCol, casterRow)

	needles := []string{"Luz do sol", "Sensibilidade à Luz do Sol", "Fraqueza à Luz do Sol", "SUNLIGHT_TRAIT_SENSITIVITY", "SUNLIGHT_ATTACK", "sunlight_attack", "sunlight_perception", "sunlight_save", "sunlightOn", "sunlightSquares", "inSunlight", "sunlightTrait", "sunlight_set"}
	watchers := z.streamsOf()

	set := func(on bool, scope playv1.SunlightScope, add ...*playv1.SunlightSquare) {
		t.Helper()
		must(master.combat.SetSunlight(ctx, rq(&playv1.SetSunlightRequest{
			CampaignId: z.campaign, EncounterId: z.enc.GetId(), IdempotencyKey: newKey(), On: on, Scope: scope, AddSquares: add,
		})))
	}
	// The whole map first, then the painted squares: one that the players see and the drow's own, which they do not.
	set(true, playv1.SunlightScope_SUNLIGHT_SCOPE_MAP)
	set(true, playv1.SunlightScope_SUNLIGHT_SCOPE_SQUARES,
		&playv1.SunlightSquare{Col: wizardCol + 1, Row: wizardRow}, &playv1.SunlightSquare{Col: casterCol, Row: casterRow})

	for _, p := range []*person{z.ana, z.caio} {
		text := z.read(p)
		requireClean(t, p.name, "the reads with the sun on", text, needles)
		enc := must(p.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: z.campaign}))).GetEncounter()
		if enc.GetSunlightOn() || enc.GetSunlightScope() != playv1.SunlightScope_SUNLIGHT_SCOPE_UNSPECIFIED || len(enc.GetSunlightSquares()) > 0 {
			t.Errorf("%s reads the sunlight of the combat: on=%v scope=%v squares=%v", p.name, enc.GetSunlightOn(), enc.GetSunlightScope(), enc.GetSunlightSquares())
		}
		for _, c := range enc.GetCombatants() {
			if c.GetInSunlight() || c.GetSunlightTrait() != playv1.SunlightTrait_SUNLIGHT_TRAIT_UNSPECIFIED {
				t.Errorf("%s reads the sunlight of %s: in=%v trait=%v", p.name, c.GetId(), c.GetInSunlight(), c.GetSunlightTrait())
			}
		}
	}

	// The master's positive control: all of it is there.
	menc := must(master.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: z.campaign}))).GetEncounter()
	if !menc.GetSunlightOn() || menc.GetSunlightScope() != playv1.SunlightScope_SUNLIGHT_SCOPE_SQUARES || len(menc.GetSunlightSquares()) != 2 {
		t.Fatalf("the master does not read the sunlight: on=%v scope=%v squares=%v", menc.GetSunlightOn(), menc.GetSunlightScope(), menc.GetSunlightSquares())
	}
	var seen bool
	for _, c := range menc.GetCombatants() {
		if c.GetId() == drow.GetId() {
			seen = true
			if !c.GetInSunlight() || c.GetSunlightTrait() != playv1.SunlightTrait_SUNLIGHT_TRAIT_SENSITIVITY {
				t.Errorf("the master reads the drow with in=%v trait=%v, want in the sun with Sensitivity", c.GetInSunlight(), c.GetSunlightTrait())
			}
		}
	}
	if !seen {
		t.Fatal("the master does not read the drow")
	}
	opts := jsonOf(must(master.combat.GetTurnOptions(ctx, rq(&playv1.GetTurnOptionsRequest{CampaignId: z.campaign, EncounterId: z.enc.GetId(), CombatantId: drow.GetId()}))))
	requireHolds(t, "the master", "the drow's turn options", opts, []string{"SUNLIGHT_ATTACK"})

	// The sun goes off: the players read nothing of that either.
	set(false, playv1.SunlightScope_SUNLIGHT_SCOPE_UNSPECIFIED)
	for _, p := range []*person{z.ana, z.caio} {
		requireClean(t, p.name, "the reads with the sun off", z.read(p), needles)
	}

	events := z.endAndCollect(watchers)
	for _, p := range []*person{z.ana, z.caio} {
		requireClean(t, p.name, "the stream", bytesOf(events[p]), needles)
	}
	requireHolds(t, "the master", "the stream", bytesOf(events[master]), []string{"encounterChanged"})
}
