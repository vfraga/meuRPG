package play

import (
	"slices"
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The spells that leave a zone, one by one, as their SRD 5.1 text says: the triggers, the ledger, the
// slowing, the sides of a wall, the camouflage, the silence.

// advanceTo passes the turns (the master ends each one) until the creature is on turn, answering with a
// natural 20 any saving throw a zone asks on the way.
func (c *cave) advanceTo(t *testing.T, label string) *playv1.Encounter {
	t.Helper()
	for range 40 {
		e := c.get(t, c.master)
		for _, s := range e.GetZoneSaves() {
			c.mustAnswer(t, c.master, s.GetId(), 20)
		}
		e = c.get(t, c.master)
		if e.GetCurrentCombatantId() == c.id(t, label) {
			return e
		}
		if _, err := c.endTurn(t, c.master, e, true); err != nil {
			t.Fatalf("EndTurn() while passing to %s: %v", label, err)
		}
	}
	t.Fatalf("%s never came on turn", label)
	return nil
}

// pendingOf lists the pending damages the creature has to roll as the attacker.
func (c *cave) pendingOf(t *testing.T, label string) []*playv1.PendingDamage {
	t.Helper()
	res, err := c.master.combat.GetTurnOptions(t.Context(), connect.NewRequest(&playv1.GetTurnOptionsRequest{
		CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), CombatantId: c.id(t, label),
	}))
	if err != nil {
		t.Fatalf("GetTurnOptions(%s) error = %v", label, err)
	}
	return res.Msg.GetPendingDamages()
}

func pendingFor(c *cave, t *testing.T, list []*playv1.PendingDamage, label string) *playv1.PendingDamage {
	t.Helper()
	id := c.id(t, label)
	for _, p := range list {
		if p.GetTargetId() == id {
			return p
		}
	}
	return nil
}

func TestGreaseMakesEveryoneInItSaveAtTheCastAndEachTimeItEntersOrEndsTheTurnThere(t *testing.T) {
	t.Parallel()
	c := newZoneCave(t)
	c.zoneFight(t)
	// A 10-foot square centered on (22,8): (22,8) (23,8) (22,9) (23,9) before the east wall; Goblin 2 is on (22,8).
	c.castZone(t, c.ana, "Pensantus", greaseSpell, 1, 22, 8)
	z := c.zones(t, c.master)[0]
	if z.GetConcentration() || z.GetDurationRounds() != 10 {
		t.Errorf("grease = concentration %v lasts %d rounds, want no concentration and 1 minute (10 rounds)", z.GetConcentration(), z.GetDurationRounds())
	}
	if n := len(z.GetCells()) / 2; n < 2 || n > 4 {
		t.Errorf("grease covers %d squares, want a 10-foot square (4 squares, 2 beyond the wall of the room)", n)
	}
	save := c.mustSaveOf(t, "Goblin 2")
	if save.GetTriggerKind() != playv1.ZoneTriggerKind_ZONE_TRIGGER_KIND_AT_CAST || save.GetAbility() != "dex" || save.GetOnFail() != playv1.ZoneFail_ZONE_FAIL_PRONE {
		t.Errorf("save = %v, want the Dexterity save at the cast: fall prone", save)
	}
	if c.saveOf(t, "Toren") != nil || c.saveOf(t, "Goblin 1") != nil {
		t.Errorf("a creature outside the grease is asked for a save")
	}
	// The master rolls 4 for the goblin: 4 + 0 against 16, it falls.
	c.mustAnswer(t, c.master, save.GetId(), 4)
	if !hasCondition(c.get(t, c.master), t, "Goblin 2", condProne) {
		t.Errorf("Goblin 2 failed its save and is not prone")
	}

	// Goblin 1 enters the grease: a save each time it enters, with no limit on a turn (the SRD has none).
	c.mustMove(t, c.master, "Goblin 1", 22, 9)
	enter := c.mustSaveOf(t, "Goblin 1")
	if enter.GetTriggerKind() != playv1.ZoneTriggerKind_ZONE_TRIGGER_KIND_ON_ENTER {
		t.Errorf("trigger = %v, want entering the area", enter.GetTriggerKind())
	}
	c.mustAnswer(t, c.master, enter.GetId(), 20)
	c.mustMove(t, c.master, "Goblin 1", 18, 9)
	c.mustMove(t, c.master, "Goblin 1", 22, 9)
	if again := c.saveOf(t, "Goblin 1"); again == nil || again.GetId() == enter.GetId() {
		t.Errorf("Goblin 1 entered the grease a second time on the turn and no save was asked: grease has no first-time limit")
	}
}

func TestWallOfFireBurnsOnTheSideTheCasterChoseAndAsksTheSaveOfWhoIsInsideAtTheCast(t *testing.T) {
	t.Parallel()
	c := newZoneCave(t)
	c.zoneFight(t)
	c.aim(t, map[string][2]int32{"Goblin 2": {16, 8}}) // on the wall's line
	// A wall of 3 squares running south from (16,7): (16,7) (16,8) (16,9), across the mouth of the corridor. Its left is
	// east (side A), toward the room.
	south := &playv1.SpellDirection{Dx: 0, Dy: 1}
	res := c.castZone(t, c.ana, "Pensantus", wallOfFire, 4, 16, 7, func(r *playv1.CastSpellRequest) {
		r.ZoneDirection, r.DamageSide, r.ZoneLengthSquares = south, playv1.ZoneSide_ZONE_SIDE_A, 3
	})
	z := res.GetZone()
	if z == nil || z.GetShape() != playv1.ZoneShape_ZONE_SHAPE_WALL || z.GetObscurity() != playv1.ZoneObscurity_ZONE_OBSCURITY_OPAQUE {
		t.Fatalf("zone = %v, want an opaque wall", z)
	}
	master := c.zones(t, c.master)[0]
	if master.GetDamageSide() != playv1.ZoneSide_ZONE_SIDE_A || master.GetReachSquares() != 2 || len(master.GetCells())/2 != 3 {
		t.Errorf("wall = side %v reach %d squares %d, want side A, 2 squares (10 ft), 3 squares", master.GetDamageSide(), master.GetReachSquares(), len(master.GetCells())/2)
	}
	save := c.mustSaveOf(t, "Goblin 2")
	if save.GetTriggerKind() != playv1.ZoneTriggerKind_ZONE_TRIGGER_KIND_AT_CAST || save.GetAbility() != "dex" || !save.GetHalfOnSuccess() || save.GetDamageCount() != 5 || save.GetDamageSides() != 8 || save.GetDamageType() != "damage-type:fire" {
		t.Errorf("save = %v, want 5d8 fire, Dexterity, half on a success", save)
	}
	ans := c.mustAnswer(t, c.master, save.GetId(), 20)
	if !ans.GetSaved() || ans.GetPendingDamageId() == "" {
		t.Fatalf("a success on a Wall of Fire answered %v, want a saved roll and a damage to roll (half)", ans)
	}
	p := pendingFor(c, t, c.pendingOf(t, "Pensantus"), "Goblin 2")
	if p == nil || !p.GetHalf() || p.GetDiceCount() != 5 || p.GetDiceSides() != 8 {
		t.Errorf("pending damage = %v, want 5d8 halved", p)
	}
	// Only the master answers for a goblin: its owner is no player.
	if _, err := c.answer(t, c.bia, save.GetId(), 5); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("a player answering a goblin's save: %v, want not_found", err)
	}

	// Ending a turn within 10 ft of the burning side hurts with no save. Goblin 1 stands at (18,7): 2 squares east of the wall.
	if got := pendingFor(c, t, c.pendingOf(t, "Pensantus"), "Goblin 1"); got != nil {
		t.Fatalf("Goblin 1 is hurt before its turn ends: %v", got)
	}
	c.advanceTo(t, "Goblin 1")
	e := c.get(t, c.master)
	if _, err := c.endTurn(t, c.master, e, true); err != nil {
		t.Fatalf("EndTurn() error = %v", err)
	}
	burn := pendingFor(c, t, c.pendingOf(t, "Pensantus"), "Goblin 1")
	if burn == nil || burn.GetHalf() || burn.GetDiceCount() != 5 || burn.GetDiceSides() != 8 {
		t.Errorf("Goblin 1 ended its turn 10 ft from the burning side: pending = %v, want 5d8 with no save", burn)
	}
}

func TestTheOtherSideOfTheWallOfFireDealsNothing(t *testing.T) {
	t.Parallel()
	c := newZoneCave(t)
	c.zoneFight(t)
	south := &playv1.SpellDirection{Dx: 0, Dy: 1}
	c.castZone(t, c.ana, "Pensantus", wallOfFire, 4, 16, 7, func(r *playv1.CastSpellRequest) {
		r.ZoneDirection, r.DamageSide, r.ZoneLengthSquares = south, playv1.ZoneSide_ZONE_SIDE_B, 3 // the west side burns
	})
	c.advanceTo(t, "Goblin 1")
	e := c.get(t, c.master)
	if _, err := c.endTurn(t, c.master, e, true); err != nil {
		t.Fatalf("EndTurn() error = %v", err)
	}
	if got := pendingFor(c, t, c.pendingOf(t, "Pensantus"), "Goblin 1"); got != nil {
		t.Errorf("Goblin 1 is on the side of the wall that deals no damage and is hurt: %v", got)
	}
}

func TestTheWallOfFireNeedsASideAndADirection(t *testing.T) {
	t.Parallel()
	c := newZoneCave(t)
	c.zoneFight(t)
	if _, err := c.castArea(t, c.ana, "Pensantus", wallOfFire, slotOfLevel(4), at(16, 5), nil); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a wall with no side: %v, want invalid_argument", err)
	}
	if _, err := c.castArea(t, c.ana, "Pensantus", wallOfFire, slotOfLevel(4), at(16, 5), nil, func(r *playv1.CastSpellRequest) { r.DamageSide = playv1.ZoneSide_ZONE_SIDE_A }); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a wall with no direction: %v, want invalid_argument", err)
	}
	if _, err := c.castArea(t, c.ana, "Pensantus", webSpell, slotOfLevel(2), at(20, 7), nil, func(r *playv1.CastSpellRequest) { r.DamageSide = playv1.ZoneSide_ZONE_SIDE_A }); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a side for a web: %v, want invalid_argument", err)
	}
	// Nothing was spent by the refusals.
	if got := c.zones(t, c.master); len(got) != 0 {
		t.Errorf("the refusals left %d zones", len(got))
	}
}

func TestSpikeGrowthIsCamouflagedAndHurtsForEveryFiveFeetTravelledInIt(t *testing.T) {
	t.Parallel()
	c := newZoneCave(t)
	c.zoneFight(t)
	c.castZone(t, c.ana, "Pensantus", spikeGrowth, 2, 20, 7)
	z := c.zones(t, c.master)[0]
	if !z.GetCamouflaged() || z.GetVisibleToPlayers() || !z.GetDifficult() {
		t.Errorf("spike growth = camouflaged %v visible %v difficult %v, want camouflaged, hidden from players, difficult terrain", z.GetCamouflaged(), z.GetVisibleToPlayers(), z.GetDifficult())
	}
	// The party saw it cast and knows it; a creature that did not does not: Brisa sees the zone (her side), then the
	// master takes her recognition back and she does not.
	if len(c.zones(t, c.bia)) != 1 {
		t.Fatalf("Brisa watched the spell being cast and is not told of its zone")
	}
	if _, err := c.master.combat.SetMapZoneKnown(t.Context(), connect.NewRequest(&playv1.SetMapZoneKnownRequest{
		CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), ZoneId: z.GetId(), CombatantId: c.id(t, "Brisa"), Known: false, IdempotencyKey: newKey(),
	})); err != nil {
		t.Fatalf("SetMapZoneKnown() error = %v", err)
	}
	if got := c.zones(t, c.bia); len(got) != 0 {
		t.Errorf("Brisa did not recognise the spikes and is told of the zone: %v", got)
	}
	// Goblin 1 walks (18,7) -> (22,7): 4 squares of the zone, 2d4 each: 8d4 piercing, in one damage.
	c.mustMove(t, c.master, "Goblin 1", 22, 7)
	p := pendingFor(c, t, c.pendingOf(t, "Pensantus"), "Goblin 1")
	if p == nil || p.GetDiceCount() != 8 || p.GetDiceSides() != 4 || p.GetDamageTypeKey() != "damage-type:piercing" {
		t.Errorf("pending damage = %v, want 8d4 piercing (4 squares at 2d4)", p)
	}
	// A move far from the spikes costs nothing.
	c.mustMove(t, c.master, "Goblin 3", 20, 2)
	if got := pendingFor(c, t, c.pendingOf(t, "Pensantus"), "Goblin 3"); got != nil {
		t.Errorf("Goblin 3 moved one square: pending = %v", got)
	}
}

func TestTheMovePreviewNeverShowsTheCostOfASpikeGrowthThePlayerHasNotRecognised(t *testing.T) {
	t.Parallel()
	c := newZoneCave(t)
	c.zoneFight(t)
	c.castZone(t, c.ana, "Pensantus", spikeGrowth, 2, 12, 7) // under Pensantus herself, in the corridor
	z := c.zones(t, c.master)[0]
	c.passTo(t, c.get(t, c.master), "Brisa")
	// Brisa (4,7) knows it (her side); the preview tells the cost of the squares in it, with where it comes from.
	known, err := c.options(t, c.bia, "Brisa")
	if err != nil {
		t.Fatalf("GetMoveOptions() error = %v", err)
	}
	if len(known.GetCostPerSquare()) == 0 || known.GetCostPerSquare()[0].GetSpellKey() != spikeGrowth || !known.GetCostPerSquare()[0].GetDifficult() {
		t.Errorf("a known spike growth's cost per square = %v, want its squares as difficult terrain", known.GetCostPerSquare())
	}
	if _, err := c.master.combat.SetMapZoneKnown(t.Context(), connect.NewRequest(&playv1.SetMapZoneKnownRequest{
		CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), ZoneId: z.GetId(), CombatantId: c.id(t, "Brisa"), Known: false, IdempotencyKey: newKey(),
	})); err != nil {
		t.Fatalf("SetMapZoneKnown() error = %v", err)
	}
	unknown, err := c.options(t, c.bia, "Brisa")
	if err != nil {
		t.Fatalf("GetMoveOptions() error = %v", err)
	}
	if len(unknown.GetCostPerSquare()) != 0 || len(unknown.GetZoneWarnings()) != 0 {
		t.Errorf("an unrecognised spike growth shows cost %v warnings %v, want nothing", unknown.GetCostPerSquare(), unknown.GetZoneWarnings())
	}
	// The preview is exactly what a plain corridor would show: the same reach as if there were no zone.
	c.endZone(t, z.GetId())
	plain, err := c.options(t, c.bia, "Brisa")
	if err != nil {
		t.Fatalf("GetMoveOptions() error = %v", err)
	}
	if len(plain.GetReachable()) != len(unknown.GetReachable()) {
		t.Errorf("the preview of an unrecognised zone reaches %d squares, the corridor without it %d: it tells the secret", len(unknown.GetReachable()), len(plain.GetReachable()))
	}
}

func TestSilenceStopsASpellWithAVerbalComponentDeafensWhoIsInsideAndHasNoVisibleForm(t *testing.T) {
	t.Parallel()
	c := newZoneCave(t)
	c.zoneFight(t)
	c.castZone(t, c.ana, "Pensantus", silenceSpell, 2, 12, 7) // centered on her, radius 20 ft
	z := c.zones(t, c.master)[0]
	if z.GetVisibleToPlayers() || len(z.GetRules()) != 3 {
		t.Errorf("silence = visible %v rules %v, want no visible form and the three rules", z.GetVisibleToPlayers(), z.GetRules())
	}
	e := c.get(t, c.master)
	if !hasCondition(e, t, "Pensantus", condDeafened) {
		t.Errorf("Pensantus is entirely inside the silence and is not deafened: %v", byLabel(t, e, "Pensantus").GetConditions())
	}
	// Whoever stands inside knows; whoever stands outside is told nothing of the zone.
	if got := c.zones(t, c.ana); len(got) != 1 {
		t.Errorf("Pensantus, inside, is told of %d zones, want her silence", len(got))
	}
	if got := c.zones(t, c.bia); len(got) != 0 {
		t.Errorf("Brisa, outside, is told of the silence: %v", got)
	}
	// The spell she cast needed a verbal component; the next one with one cannot be cast inside.
	_, err := c.castArea(t, c.master, "Pensantus", fogCloud, slotOfLevel(1), at(20, 7), nil)
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("a spell with a verbal component inside the silence: %v, want failed_precondition", err)
	}
	c.advanceToRound(t, "Pensantus", 2) // a new turn: the action is free again
	opts := c.mustOptions(t, c.ana, c.get(t, c.master), "Pensantus")
	for _, sp := range opts.GetOptions().GetSpells() {
		if sp.GetSpell().GetKey() == fogCloud && (sp.GetEnabled() || sp.GetReason().GetCode() != rulesSilenced) {
			t.Errorf("fog cloud in the silence = enabled %v reason %v, want unavailable: SILENCED", sp.GetEnabled(), sp.GetReason().GetCode())
		}
	}
	// She leaves it: the deafness goes (the master walks her out; the radius is 4 squares).
	c.mustMove(t, c.master, "Pensantus", 7, 7)
	if hasCondition(c.get(t, c.master), t, "Pensantus", condDeafened) {
		t.Errorf("Pensantus left the silence and is still deafened")
	}
}

func TestStinkingCloudAsksOnlyWhoIsCompletelyInsideAtTheStartOfItsTurnAndTakesTheAction(t *testing.T) {
	t.Parallel()
	c := newZoneCave(t)
	c.zoneFight(t)
	c.castZone(t, c.ana, "Pensantus", stinkingCloud, 3, 20, 7)
	z := c.zones(t, c.master)[0]
	if z.GetObscurity() != playv1.ZoneObscurity_ZONE_OBSCURITY_HEAVY || z.GetDurationRounds() != 10 || len(z.GetWinds()) != 2 {
		t.Errorf("stinking cloud = obscurity %v lasts %d winds %v, want heavily obscured, 1 minute, both winds", z.GetObscurity(), z.GetDurationRounds(), z.GetWinds())
	}
	if got := c.get(t, c.master).GetZoneSaves(); len(got) != 0 {
		t.Fatalf("the cast opened %d saves; the cloud asks at the start of a turn", len(got))
	}
	c.advanceToSave(t, "Goblin 1")
	save := c.mustSaveOf(t, "Goblin 1")
	if save.GetAbility() != "con" || save.GetTriggerKind() != playv1.ZoneTriggerKind_ZONE_TRIGGER_KIND_START_OF_TURN || save.GetOnFail() != playv1.ZoneFail_ZONE_FAIL_LOSE_ACTION {
		t.Errorf("save = %v, want Constitution at the start of the turn, losing the action", save)
	}
	c.mustAnswer(t, c.master, save.GetId(), 2)
	e := c.get(t, c.master)
	if !byLabel(t, e, "Goblin 1").GetActionUsed() {
		t.Errorf("Goblin 1 failed and still has its action: the cloud makes it spend it retching")
	}
	// A creature standing outside the cloud is not asked.
	if c.saveOf(t, "Brisa") != nil {
		t.Errorf("Brisa is outside the cloud and is asked for a save")
	}
}

func TestCloudkillAsksOnEnteringTheFirstTimeInATurnAndStartingTheTurnThereNotAtTheCast(t *testing.T) {
	t.Parallel()
	c := newZoneCave(t)
	c.zoneFight(t)
	c.aim(t, map[string][2]int32{"Goblin 2": {22, 5}})
	c.castZone(t, c.ana, "Pensantus", cloudkill, 5, 18, 7)
	if got := c.get(t, c.master).GetZoneSaves(); len(got) != 0 {
		t.Fatalf("the cast opened %d saves; the fog asks on entering or starting a turn in it", len(got))
	}
	z := c.zones(t, c.master)[0]
	if z.GetMovesWith() != playv1.ZoneMovesWith_ZONE_MOVES_WITH_SELF_AT_TURN_START || z.GetDurationRounds() != 100 {
		t.Errorf("cloudkill = moves %v lasts %d rounds, want it to walk at the start of the caster's turn, 10 minutes", z.GetMovesWith(), z.GetDurationRounds())
	}
	// A moderate wind does not disperse it; a strong one does, at once.
	if _, err := c.master.combat.DisperseMapZone(t.Context(), connect.NewRequest(&playv1.DisperseMapZoneRequest{
		CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), ZoneId: z.GetId(), Wind: playv1.ZoneWind_ZONE_WIND_MODERATE, IdempotencyKey: newKey(),
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a moderate wind on a cloudkill: %v, want invalid_argument", err)
	}
	// At the start of Pensantus's next turn the fog walks 2 squares away from her: (18,7) -> (20,7), and Goblin 2 at (22,5) enters it.
	c.advanceToRound(t, "Pensantus", 2)
	moved := c.zones(t, c.master)[0]
	if moved.GetOrigin().GetCol() != 20 || moved.GetOrigin().GetRow() != 7 {
		t.Errorf("the cloud is at (%d,%d), want (20,7) after walking 10 ft away from Pensantus", moved.GetOrigin().GetCol(), moved.GetOrigin().GetRow())
	}
	enter := c.mustSaveOf(t, "Goblin 2")
	if enter.GetTriggerKind() != playv1.ZoneTriggerKind_ZONE_TRIGGER_KIND_ON_ENTER_FIRST_TIME_ON_A_TURN || enter.GetAbility() != "con" || enter.GetDamageCount() != 5 || enter.GetDamageSides() != 8 {
		t.Errorf("save = %v, want Constitution on entering, 5d8", enter)
	}
	// The strong wind ends it at once.
	if _, err := c.master.combat.DisperseMapZone(t.Context(), connect.NewRequest(&playv1.DisperseMapZoneRequest{
		CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), ZoneId: z.GetId(), Wind: playv1.ZoneWind_ZONE_WIND_STRONG, IdempotencyKey: newKey(),
	})); err != nil {
		t.Fatalf("DisperseMapZone() error = %v", err)
	}
	e := c.get(t, c.master)
	if len(c.zones(t, c.master)) != 0 || len(e.GetZoneSaves()) != 0 {
		t.Errorf("a strong wind left %d zones and %d saves; the window closes with the zone", len(c.zones(t, c.master)), len(e.GetZoneSaves()))
	}
	if byLabel(t, e, "Pensantus").GetConcentrationSpell() != "" {
		t.Errorf("a strong wind ended the spell and Pensantus still concentrates")
	}
}

func TestSpiritGuardiansHurtsOncePerTurnLeavesTheExcludedAloneAndSlowsWhoIsInside(t *testing.T) {
	t.Parallel()
	c := newZoneCave(t)
	c.zoneFight(t)
	c.aim(t, map[string][2]int32{"Brisa": {17, 7}, "Toren": {19, 8}})
	c.passTo(t, c.get(t, c.master), "Brisa")
	res := c.mustCastArea(t, c.bia, "Brisa", spiritGuards, slotOfLevel(3), nil, nil, func(r *playv1.CastSpellRequest) {
		r.ExcludedCombatantIds = []string{c.id(t, "Toren")}
	})
	z := res.GetZone()
	if z == nil || z.GetHalvesSpeed() != true {
		t.Fatalf("zone = %v, want one that halves the speed", z)
	}
	m := c.zones(t, c.master)[0]
	if m.GetMovesWith() != playv1.ZoneMovesWith_ZONE_MOVES_WITH_CASTER || !slices.Contains(m.GetExcludedCombatantIds(), c.id(t, "Toren")) || m.GetDamageType() != "damage-type:radiant" || m.GetDamageCount() != 3 || m.GetDamageSides() != 8 {
		t.Errorf("zone = moves %v excluded %v damage %dd%d %s, want it to follow Brisa, leave Toren out, 3d8 radiant", m.GetMovesWith(), m.GetExcludedCombatantIds(), m.GetDamageCount(), m.GetDamageSides(), m.GetDamageType())
	}
	if got := c.get(t, c.master).GetZoneSaves(); len(got) != 0 {
		t.Fatalf("the cast opened %d saves; the spirits ask on entering or starting a turn", len(got))
	}
	// The excluded Toren stands inside, and is never asked and not slowed; the goblin is.
	c.advanceToSave(t, "Goblin 1")
	save := c.mustSaveOf(t, "Goblin 1")
	if save.GetAbility() != "wis" || !save.GetHalfOnSuccess() || save.GetDc() != 14 {
		t.Errorf("save = %v, want Wisdom, half on a success, DC 14 (Brisa, cleric 5)", save)
	}
	if c.saveOf(t, "Toren") != nil {
		t.Errorf("Toren was left out of the spell and is asked for a save")
	}
	c.mustAnswer(t, c.master, save.GetId(), 3)
	// Once on the turn: out and in again, no second save.
	c.mustMove(t, c.master, "Goblin 1", 21, 7)
	c.mustMove(t, c.master, "Goblin 1", 18, 7)
	if again := c.saveOf(t, "Goblin 1"); again != nil {
		t.Errorf("Goblin 1 re-entered the spirits on the same turn and was asked again: %v", again)
	}
	// The speed is halved inside: a square of the zone costs twice (a goblin of 30 ft).
	opts, err := c.options(t, c.master, "Goblin 1")
	if err != nil {
		t.Fatalf("GetMoveOptions() error = %v", err)
	}
	found := false
	for _, cost := range opts.GetCostPerSquare() {
		if cost.GetSpellKey() == spiritGuards && cost.GetHalvesSpeed() && cost.GetCostDft() == 100 {
			found = true
		}
	}
	if !found {
		t.Errorf("the preview does not tell the spirits double each square (100 tenths): %v", opts.GetCostPerSquare())
	}
	// The spirits go with the caster: moving Brisa east carries the zone, and Goblin 2 at (22,8) is inside after 5 squares.
	c.mustMove(t, c.master, "Brisa", 21, 7)
	if o := c.zones(t, c.master)[0].GetOrigin(); o.GetCol() != 21 || o.GetRow() != 7 {
		t.Errorf("the spirits are at (%d,%d), want (21,7) with Brisa", o.GetCol(), o.GetRow())
	}
}

func TestEntangleRestrainsWhoIsInTheAreaAtTheCastUntilTheSpellEnds(t *testing.T) {
	t.Parallel()
	c := newZoneCave(t)
	c.zoneFight(t)
	c.castZone(t, c.ana, "Pensantus", entangle, 1, 20, 7)
	save := c.mustSaveOf(t, "Goblin 2")
	if save.GetAbility() != "str" || save.GetOnFail() != playv1.ZoneFail_ZONE_FAIL_RESTRAINED || save.GetTriggerKind() != playv1.ZoneTriggerKind_ZONE_TRIGGER_KIND_AT_CAST {
		t.Errorf("save = %v, want Strength at the cast: restrained", save)
	}
	c.mustAnswer(t, c.master, save.GetId(), 1)
	if !hasCondition(c.get(t, c.master), t, "Goblin 2", condRestrained) {
		t.Errorf("Goblin 2 failed and is not restrained")
	}
	// It does not ask again at the start of a turn: Entangle only asks at the cast.
	c.advanceToSave(t, "Goblin 1")
	if again := c.saveOf(t, "Goblin 2"); again != nil {
		t.Errorf("Entangle asked again at the start of a turn: %v", again)
	}
	// Pensantus's concentration ends with the spell: the plants wilt, the goblin is free.
	if _, err := c.master.combat.EndConcentration(t.Context(), connect.NewRequest(&playv1.EndConcentrationRequest{
		CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), CombatantId: c.id(t, "Pensantus"), IdempotencyKey: newKey(),
	})); err != nil {
		t.Fatalf("EndConcentration() error = %v", err)
	}
	e := c.get(t, c.master)
	if hasCondition(e, t, "Goblin 2", condRestrained) || len(c.zones(t, c.master)) != 0 {
		t.Errorf("Pensantus stopped concentrating: zones %d, Goblin 2 restrained %v; want none and free", len(c.zones(t, c.master)), hasCondition(e, t, "Goblin 2", condRestrained))
	}
}

// advanceToSave passes the turns until the creature's save is open, or the creature is on turn.
func (c *cave) advanceToSave(t *testing.T, label string) {
	t.Helper()
	for range 40 {
		e := c.get(t, c.master)
		if c.saveOf(t, label) != nil || e.GetCurrentCombatantId() == c.id(t, label) {
			return
		}
		for _, s := range e.GetZoneSaves() {
			c.mustAnswer(t, c.master, s.GetId(), 20)
		}
		if _, err := c.endTurn(t, c.master, c.get(t, c.master), true); err != nil {
			t.Fatalf("EndTurn() while passing to %s: %v", label, err)
		}
	}
	t.Fatalf("%s never came on turn", label)
}

// advanceToRound passes the turns until the creature is on turn in the round, answering with a natural 20 any
// save but the ones of the creature in the label's turn start.
func (c *cave) advanceToRound(t *testing.T, label string, round int32) {
	t.Helper()
	for range 60 {
		e := c.get(t, c.master)
		if e.GetRound() == round && e.GetCurrentCombatantId() == c.id(t, label) {
			return
		}
		if e.GetRound() > round {
			t.Fatalf("round %d passed without %s", round, label)
		}
		for _, s := range e.GetZoneSaves() {
			if e.GetRound() == round-1 || e.GetCurrentCombatantId() != c.id(t, label) {
				c.mustAnswer(t, c.master, s.GetId(), 20)
			}
		}
		e = c.get(t, c.master)
		if e.GetRound() == round && e.GetCurrentCombatantId() == c.id(t, label) {
			return
		}
		if _, err := c.endTurn(t, c.master, e, true); err != nil {
			t.Fatalf("EndTurn() while passing to %s: %v", label, err)
		}
	}
	t.Fatalf("%s never came on turn in round %d", label, round)
}

// rulesSilenced is the reason a spell the silence stops is unavailable with.
const rulesSilenced = rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_SILENCED
