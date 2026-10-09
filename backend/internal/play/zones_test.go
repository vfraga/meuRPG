package play

import (
	"slices"
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// Zones on the map (W7-Z, SRD 5.1: the text of each spell, "Areas of Effect", "Vision and
// Light"). These tests need the database (MEURPG_TEST_DATABASE_URL). The fixture is the cave of
// the area tests with stronger casters: Pensantus (wizard 9: DC 15) casts the zones of the
// wizard's list, Brisa (cleric 5: DC 14) Spirit Guardians, Toren stands in the room as an
// ally, the goblins are in the room to the east.

const (
	fogCloud      = "spell:fog-cloud"
	stinkingCloud = "spell:stinking-cloud"
	wallOfFire    = "spell:wall-of-fire"
	cloudkill     = "spell:cloudkill"
	silenceSpell  = "spell:silence"
	darknessSpell = "spell:darkness"
	spikeGrowth   = "spell:spike-growth"
	greaseSpell   = "spell:grease"
	entangle      = "spell:entangle"
	spiritGuards  = "spell:spirit-guardians"
	moonbeam      = "spell:moonbeam"
)

// newZoneCave is the cave with the casters of the zones. verbal spells are in Pensantus's book.
func newZoneCave(t *testing.T) *cave {
	t.Helper()
	spells := []string{webSpell, fogCloud, stinkingCloud, wallOfFire, cloudkill, silenceSpell, darknessSpell, spikeGrowth, greaseSpell, entangle, magicMissileSpell}
	a := newArmedWith(t, func(a *armed) {
		scores := func(str, dex, con, intl, wis int32) *rulesv1.AbilityScores {
			return &rulesv1.AbilityScores{Strength: str, Dexterity: dex, Constitution: con, Intelligence: intl, Wisdom: wis, Charisma: 8}
		}
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 4, scores(15, 13, 14, 10, 10), []string{battleaxe}, nil)
		a.pens = a.ana.caster(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 9, scores(10, 14, 12, 16, 10), nil, []string{fireBolt}, spells, spells)
		a.bri = a.bia.caster(t, a.campaignID, "Brisa", "class:cleric", "race:human", 5, scores(8, 16, 14, 10, 16), []string{maceKey}, []string{sacredFlame}, nil,
			[]string{cureWounds, spiritGuards})
	})
	c := &cave{armed: a, terrain: &caveTerrain{}}
	a.h.svc.terrain = c.terrain
	c.goblins = a.master.sizedNPC(t, a.campaignID, "Goblin", 7, 12, rulesv1.CreatureSize_CREATURE_SIZE_SMALL)
	c.ogre = a.master.sizedNPC(t, a.campaignID, "Ogro", 59, 11, rulesv1.CreatureSize_CREATURE_SIZE_LARGE)
	a.mapID = a.h.newMapOf(a.campaignID, 24, 1200, 800)
	if _, err := a.master.play.SetCurrentMap(t.Context(), connect.NewRequest(&playv1.SetCurrentMapRequest{CampaignId: a.campaignID, MapId: a.mapID})); err != nil {
		t.Fatalf("SetCurrentMap() error = %v", err)
	}
	return c
}

// zoneFight starts the combat with Pensantus on turn, in the corridor: Goblin 1 (18,7), Goblin 2
// (22,8) and Goblin 3 (20,3) in the room, Toren (21,8) with them, Brisa at the entrance.
func (c *cave) zoneFight(t *testing.T) *playv1.Encounter {
	t.Helper()
	e := c.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: c.goblins.GetId(), Count: 3}},
		npcRolls: []int{2, 2, 2},
		players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1},
		reveal:   []string{"Goblin 1", "Goblin 2", "Goblin 3"},
		at: map[string][2]int32{
			"Pensantus": {12, 7}, "Toren": {21, 8}, "Brisa": {4, 7}, "Goblin 1": {18, 7}, "Goblin 2": {22, 8}, "Goblin 3": {20, 3},
		},
	})
	return c.passTo(t, e, "Pensantus")
}

// zones is the zones a user is told of.
func (c *cave) zones(t *testing.T, u *user) []*playv1.MapZone {
	t.Helper()
	res, err := u.combat.ListMapZones(t.Context(), connect.NewRequest(&playv1.ListMapZonesRequest{CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId()}))
	if err != nil {
		t.Fatalf("ListMapZones() error = %v", err)
	}
	return res.Msg.GetZones()
}

// castZone casts a zone spell as the caster's player at a point.
func (c *cave) castZone(t *testing.T, u *user, caster, spell string, level int32, col, row int32, edit ...func(*playv1.CastSpellRequest)) *playv1.CastSpellResponse {
	t.Helper()
	return c.mustCastArea(t, u, caster, spell, slotOfLevel(level), at(col, row), nil, edit...)
}

func hasLabel(e *playv1.Encounter, label string) bool {
	return slices.ContainsFunc(e.GetCombatants(), func(c *playv1.Combatant) bool { return c.GetLabel() == label })
}

func TestFogCloudHidesWhoIsInsideFromEveryPlayerOutsideItAndLeavesTheirPlace(t *testing.T) {
	t.Parallel()
	c := newZoneCave(t)
	c.zoneFight(t)
	res := c.castZone(t, c.ana, "Pensantus", fogCloud, 1, 20, 7)

	// The master sees the zone with everything: the caster, the duration, the clock.
	master := c.zones(t, c.master)
	if len(master) != 1 {
		t.Fatalf("the master is told of %d zones, want 1", len(master))
	}
	z := master[0]
	if z.GetSpellKey() != fogCloud || z.GetShape() != playv1.ZoneShape_ZONE_SHAPE_SPHERE || z.GetObscurity() != playv1.ZoneObscurity_ZONE_OBSCURITY_HEAVY {
		t.Errorf("zone = %v %v %v, want the fog: a sphere, heavily obscured", z.GetSpellKey(), z.GetShape(), z.GetObscurity())
	}
	if z.GetCasterId() != c.id(t, "Pensantus") || !z.GetConcentration() || z.GetDurationRounds() != 600 || z.GetRemainingRounds() != 600 {
		t.Errorf("zone = caster %q concentration %v duration %d remaining %d; want Pensantus, concentration, 600 rounds (1 hour)", z.GetCasterId(), z.GetConcentration(), z.GetDurationRounds(), z.GetRemainingRounds())
	}
	if n := len(z.GetCells()) / 2; n < 30 || n > 49 { // a sphere of 20 ft radius is 49 squares; the walls of the room cut some
		t.Errorf("the fog covers %d squares, want a sphere of 4 squares of radius with the walls cut", n)
	}
	if res.GetZone().GetId() != z.GetId() {
		t.Errorf("the cast's zone = %q, want %q", res.GetZone().GetId(), z.GetId())
	}

	// Brisa, outside, sees the zone and none of what is inside: the goblins are not in her combat, Toren is
	// in the order but has no square.
	brisa := c.get(t, c.bia)
	for _, g := range []string{"Goblin 1", "Goblin 2", "Goblin 3"} {
		if hasLabel(brisa, g) {
			t.Errorf("Brisa is told of %s, who is inside the fog", g)
		}
		if !hasLabel(c.get(t, c.master), g) {
			t.Errorf("the master is not told of %s", g)
		}
	}
	toren := byLabel(t, brisa, "Toren")
	if toren.GetPlaced() {
		t.Errorf("Brisa is told where Toren is, inside the fog: (%d, %d)", toren.GetCol(), toren.GetRow())
	}
	pz := c.zones(t, c.bia)
	if len(pz) != 1 || pz[0].GetCasterId() != "" || pz[0].GetSaveDc() != 0 || pz[0].GetDurationRounds() != 0 || len(pz[0].GetTriggers()) != 0 {
		t.Errorf("a player's zone = %v, want the shape and the obscurity only", pz)
	}
	if pz[0].GetSpellKey() != fogCloud || len(pz[0].GetCells()) == 0 {
		t.Errorf("a player's zone = %v, want the spell and its squares", pz[0])
	}

	// Toren, inside, sees nobody else on the map: the goblins are gone from his combat too, and so is
	// Pensantus's square.
	torenView := c.get(t, c.caio)
	if hasLabel(torenView, "Goblin 1") || hasLabel(torenView, "Goblin 2") {
		t.Errorf("Toren is inside the fog and is told of the goblins")
	}
	if p := byLabel(t, torenView, "Pensantus"); p.GetPlaced() {
		t.Errorf("Toren is told where Pensantus is: he is inside the fog and she is not")
	}
	if me := byLabel(t, torenView, "Toren"); !me.GetPlaced() {
		t.Errorf("Toren does not see himself on the map")
	}

	// The cast's list of who is inside names only those the caster sees: nobody, with the whole fog hiding them.
	for _, tg := range res.GetCast().GetTargets() {
		t.Errorf("the cast result names %s, who is inside the fog", tg.GetCombatantId())
	}
}

func TestAWebHoldsWhoStartsTheTurnInItAndTheMasterAnswersForTheGoblin(t *testing.T) {
	t.Parallel()
	c := newZoneCave(t)
	e := c.zoneFight(t)
	c.castZone(t, c.ana, "Pensantus", webSpell, 2, 20, 7)
	z := c.zones(t, c.master)[0]
	if !z.GetDifficult() || z.GetObscurity() != playv1.ZoneObscurity_ZONE_OBSCURITY_LIGHT || len(z.GetCells())/2 != 16 {
		t.Errorf("the web = difficult %v obscurity %v squares %d, want difficult terrain, lightly obscured, a 20-foot cube (16 squares)", z.GetDifficult(), z.GetObscurity(), len(z.GetCells())/2)
	}
	if z.GetDurationRounds() != 600 || !z.GetAnchored() {
		t.Errorf("the web lasts %d rounds anchored %v, want an hour, anchored", z.GetDurationRounds(), z.GetAnchored())
	}
	// Nobody rolls at the cast: the web asks at the start of a turn in it, or on entering.
	if got := c.get(t, c.master).GetZoneSaves(); len(got) != 0 {
		t.Fatalf("the cast opened %d saves, want none (Web asks when a creature starts its turn in it or enters it)", len(got))
	}

	// The turn passes to the goblins (a joint turn): Goblin 2 starts it inside the web.
	e = c.passTo(t, c.get(t, c.master), "Goblin 1")
	saves := e.GetZoneSaves()
	if len(saves) != 1 || saves[0].GetReactorId() != c.id(t, "Goblin 2") || saves[0].GetAbility() != "dex" {
		t.Fatalf("zone saves = %v, want one: Dexterity, Goblin 2", saves)
	}
	if saves[0].GetTriggerKind() != playv1.ZoneTriggerKind_ZONE_TRIGGER_KIND_START_OF_TURN {
		t.Errorf("the save's trigger = %v, want the start of the turn", saves[0].GetTriggerKind())
	}
	if saves[0].GetDc() != 16 { // 8 + proficiency 4 + Intelligence 4 (16, +2 for the gnome)
		t.Errorf("the master reads DC %d, want 16 (Pensantus, wizard 9)", saves[0].GetDc())
	}
	// The turn waits for it: a goblin cannot move meanwhile, and the table reads "Esperando o mestre".
	if _, err := c.move(t, c.master, "Goblin 2", 22, 9); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("a move while the save waits: %v, want failed_precondition", err)
	}
	if !c.get(t, c.bia).GetTurnHeld() {
		t.Errorf("the table is not told the turn waits")
	}
	if got := c.get(t, c.bia).GetZoneSaves(); len(got) != 0 {
		t.Errorf("Brisa is told of a save asked of a goblin: %v", got)
	}

	// The master rolls a 3 for the goblin: total 3 + 0 against 16, it fails and is restrained.
	res, err := c.master.combat.AnswerZoneSave(t.Context(), connect.NewRequest(&playv1.AnswerZoneSaveRequest{
		CampaignId: c.campaignID, EncounterId: e.GetId(), ZoneSaveId: saves[0].GetId(), IdempotencyKey: newKey(),
		Roll: &playv1.AnswerZoneSaveRequest_D20Face{D20Face: 3},
	}))
	if err != nil {
		t.Fatalf("AnswerZoneSave() error = %v", err)
	}
	if res.Msg.GetSaved() || res.Msg.GetD20() != 3 || res.Msg.GetFailedWith() != playv1.ZoneFail_ZONE_FAIL_RESTRAINED {
		t.Errorf("answer = %v, want a failure that restrains", res.Msg)
	}
	e = c.get(t, c.master)
	if !hasCondition(e, t, "Goblin 2", condRestrained) {
		t.Errorf("Goblin 2 is not restrained after the failed save: %v", byLabel(t, e, "Goblin 2").GetConditions())
	}
	if len(e.GetZoneSaves()) != 0 || e.GetTurnHeld() {
		t.Errorf("the save is answered and the turn still waits: %v held %v", e.GetZoneSaves(), e.GetTurnHeld())
	}

	// Ending the web (the master's "Encerrar") lets the goblin go.
	if _, err := c.master.combat.EndMapZone(t.Context(), connect.NewRequest(&playv1.EndMapZoneRequest{
		CampaignId: c.campaignID, EncounterId: e.GetId(), ZoneId: z.GetId(), IdempotencyKey: newKey(),
	})); err != nil {
		t.Fatalf("EndMapZone() error = %v", err)
	}
	e = c.get(t, c.master)
	if hasCondition(e, t, "Goblin 2", condRestrained) {
		t.Errorf("Goblin 2 is still restrained after the web ended")
	}
	if got := c.zones(t, c.master); len(got) != 0 {
		t.Errorf("the master is still told of %d zones after ending the web", len(got))
	}
	if byLabel(t, e, "Pensantus").GetConcentrationSpell() != "" {
		t.Errorf("Pensantus still concentrates on %q after her web ended", byLabel(t, e, "Pensantus").GetConcentrationSpell())
	}
}

// answer answers a zone's saving throw as u with the face of a physical d20.
func (c *cave) answer(t *testing.T, u *user, saveID string, face int32) (*playv1.AnswerZoneSaveResponse, error) {
	t.Helper()
	res, err := u.combat.AnswerZoneSave(t.Context(), connect.NewRequest(&playv1.AnswerZoneSaveRequest{
		CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), ZoneSaveId: saveID, IdempotencyKey: newKey(),
		Roll: &playv1.AnswerZoneSaveRequest_D20Face{D20Face: face},
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// mustAnswer answers a zone's saving throw that must work.
func (c *cave) mustAnswer(t *testing.T, u *user, saveID string, face int32) *playv1.AnswerZoneSaveResponse {
	t.Helper()
	res, err := c.answer(t, u, saveID, face)
	if err != nil {
		t.Fatalf("AnswerZoneSave() error = %v", err)
	}
	return res
}

// saveOf is the master's open zone save asked of the creature, nil when none is.
func (c *cave) saveOf(t *testing.T, label string) *playv1.ZoneSave {
	t.Helper()
	id := c.id(t, label)
	for _, s := range c.get(t, c.master).GetZoneSaves() {
		if s.GetReactorId() == id {
			return s
		}
	}
	return nil
}

// mustSaveOf is saveOf for a save that must be open.
func (c *cave) mustSaveOf(t *testing.T, label string) *playv1.ZoneSave {
	t.Helper()
	s := c.saveOf(t, label)
	if s == nil {
		t.Fatalf("no zone save is open for %s: %v", label, c.get(t, c.master).GetZoneSaves())
	}
	return s
}

// endZoneAs ends a zone as the master.
func (c *cave) endZone(t *testing.T, zoneID string) {
	t.Helper()
	if _, err := c.master.combat.EndMapZone(t.Context(), connect.NewRequest(&playv1.EndMapZoneRequest{
		CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), ZoneId: zoneID, IdempotencyKey: newKey(),
	})); err != nil {
		t.Fatalf("EndMapZone() error = %v", err)
	}
}

func (c *cave) pending(t *testing.T) []*playv1.PendingDamage {
	t.Helper()
	res, err := c.master.combat.GetTurnOptions(t.Context(), connect.NewRequest(&playv1.GetTurnOptionsRequest{
		CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), CombatantId: c.id(t, "Pensantus"),
	}))
	if err != nil {
		t.Fatalf("GetTurnOptions() error = %v", err)
	}
	return res.Msg.GetPendingDamages()
}
