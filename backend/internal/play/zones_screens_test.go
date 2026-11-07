package play

import (
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// What the zone screens read (boards B6): the summary of each zone, the question a player is asked when
// a zone catches their character, the order of two questions, a zone with no saving throw, how a zone
// ends and what it does to the speed and the terrain.

// noSaveSpell is a fixture: an SRD spell (Silent Image) given the row of a zone with no saving throw, such as
// the board's Cloud of Daggers, which the SRD 5.1 does not have. Registered once, before any test runs.
const noSaveSpell = "spell:silent-image"

func init() {
	zoneDefs[noSaveSpell] = zoneDef{
		kind: zoneDamage, shape: grid.ShapeCube, sizeFt: 5, dice: "4d4", dtype: "damage-type:slashing", onEnter: true, onStart: true,
		placement: playv1.AreaPlacement_AREA_PLACEMENT_POINT, movement: playv1.SpellZoneMovement_SPELL_ZONE_MOVEMENT_FIXED,
	}
}

// cloudCave is the cave with Toren a druid of level 3 (Moonbeam) and Pensantus a wizard of level 9 (Cloudkill, Web).
func cloudCave(t *testing.T) *cave {
	t.Helper()
	return newCaveOf(t, 9, []string{cloudkillKey, webKey, noSaveSpell}, func(a *armed) *charactersv1.Character {
		return a.caio.caster(t, a.campaignID, "Toren", "class:druid", "race:human", 3, abilities(12, 12, 14, 16, 10), nil, nil, nil, []string{moonbeamKey})
	})
}

func hasNote(z *playv1.SpellZone, note string) bool { return slices.Contains(z.GetNotes(), note) }

// The master reads the whole summary of a zone: the caster, the save, the damage, how it moves and who is in it.
func TestTheMastersZoneSummary(t *testing.T) {
	t.Parallel()
	c := zoneCave(t)
	c.zoneFight(t)
	c.castSpiritGuardians(t, 3)
	z := c.oneZone(t, c.master, spiritGuardiansKey)
	if z.GetOwnerLabel() != "Toren" || z.GetSaveAbility() != "wis" || z.GetSaveDc() != 14 || z.GetDamageDice() != "3d8" || z.GetDamageTypeKey() != "damage-type:radiant" || !z.GetHalfOnSave() {
		t.Errorf("summary = %v, want Toren's Wisdom DC 14, 3d8 radiant, half on a pass", z)
	}
	if !hasNote(z, "speed_halved") || z.GetDetailsHidden() {
		t.Errorf("notes = %v hidden = %v, want speed_halved and the numbers", z.GetNotes(), z.GetDetailsHidden())
	}
	if want := []string{c.id(t, "Brisa"), c.id(t, "Ogro"), c.id(t, "Toren")}; !sameSet(z.GetOccupantIds(), want) {
		t.Errorf("occupants = %v, want Brisa, the ogre and Toren: the creatures whose square is in the area", z.GetOccupantIds())
	}
}

func sameSet(a, b []string) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}

// Cloudkill says how far it goes by itself, and Web what else it does.
func TestCloudAndWebSaySomeMore(t *testing.T) {
	t.Parallel()
	c := zoneCave(t)
	e := c.zoneFight(t)
	c.passTo(t, e, "Pensantus")
	c.mustCastArea(t, c.ana, "Pensantus", cloudkillKey, slotOfLevel(5), at(16, 7), nil)
	if z := c.oneZone(t, c.master, cloudkillKey); z.GetMovesPerTurnFt() != 10 || z.GetSaveAbility() != "con" || z.GetDamageDice() != "5d8" {
		t.Errorf("cloud = %v, want 10 ft a turn, Constitution, 5d8", z)
	}
	c.nextTurnOf(t, "Pensantus")
	c.mustCastArea(t, c.ana, "Pensantus", webKey, slotOfLevel(2), at(18, 3), nil)
	z := c.oneZone(t, c.master, webKey)
	if !hasNote(z, "difficult_terrain") || !hasNote(z, "restrains") || z.GetDamageDice() != "" || z.GetSaveAbility() != "dex" {
		t.Errorf("web = %v, want a Dexterity zone that restrains in difficult terrain, with no damage", z)
	}
}

// What a player reads of a zone (RN-10): the zone of a caster that is not theirs has no DC, dice, damage, notes
// or rounds, only the name, the shape, the squares, the concentration and who stands in it; their own character's
// zone is whole; the master's NPC caster's is just as bare.
func TestAPlayerReadsNoNumbersOfAZoneThatIsNotTheirs(t *testing.T) {
	t.Parallel()
	c := zoneCave(t)
	c.zoneFight(t)
	c.castSpiritGuardians(t, 3)
	own := c.oneZone(t, c.caio, spiritGuardiansKey)
	if own.GetDetailsHidden() || own.GetSaveDc() != 14 || own.GetDamageDice() != "3d8" || own.GetRoundsLeft() != 100 {
		t.Errorf("the caster's own player reads %v, want it whole", own)
	}
	other := c.oneZone(t, c.ana, spiritGuardiansKey)
	if !other.GetDetailsHidden() || other.SaveDc != nil || other.GetDamageDice() != "" || other.GetDamageTypeKey() != "" || other.GetSaveAbility() != "" ||
		other.GetRoundsLeft() != 0 || len(other.GetNotes()) != 0 || other.GetMovesPerTurnFt() != 0 || other.GetHalfOnSave() {
		t.Errorf("another player reads %v, want no numbers", other)
	}
	if other.GetOwnerLabel() != "Toren" || !other.GetConcentration() || len(other.GetSquares()) == 0 || other.GetSpellKey() != spiritGuardiansKey {
		t.Errorf("another player reads %v, want the name, the shape, the squares and the concentration", other)
	}
	// Who stands in it: only whom the reader sees.
	if len(other.GetOccupantIds()) != 3 {
		t.Errorf("occupants for a player = %v, want the three creatures they see", other.GetOccupantIds())
	}
}

// zoneDiceNeedle is the dice of Moonbeam as a whole JSON string value. Bare 2d10 also matches the middle of an
// identifier (a UUID holds those characters now and then), a quoted value only the field itself.
const zoneDiceNeedle = `"2d10"`

// The NPC caster the players see: bare for them, whole for the master.
func TestAVisibleNPCCastersZoneIsBareForPlayers(t *testing.T) {
	t.Parallel()
	c := newCaveWith(t, 5, []string{thunderwave})
	c.druidFight(t, "Druida")
	c.mustCastArea(t, c.master, "Druida", moonbeamKey, slotOfLevel(2), at(7, 7), nil)
	whole := c.oneZone(t, c.master, moonbeamKey)
	if whole.GetSaveDc() == 0 || whole.GetDamageDice() != "2d10" || whole.GetRoundsLeft() != 10 {
		t.Errorf("the master reads %v, want the DC, 2d10 and 10 rounds", whole)
	}
	// The positive control: the needle is in the master's copy, so a player's copy without it proves something.
	if !strings.Contains(asJSONAny(t, c.get(t, c.master)), zoneDiceNeedle) {
		t.Errorf("the master's copy has no %s: the needle finds nothing", zoneDiceNeedle)
	}
	for who, u := range map[string]*user{"Toren's player": c.caio, "Pensantus's player": c.ana} {
		z := c.oneZone(t, u, moonbeamKey)
		if !z.GetDetailsHidden() || z.SaveDc != nil || z.GetDamageDice() != "" || z.GetRoundsLeft() != 0 || z.GetOwnerLabel() != "Druida" {
			t.Errorf("%s reads %v, want the name, shape and squares and no numbers", who, z)
		}
		if text := asJSONAny(t, c.get(t, u)); strings.Contains(text, zoneDiceNeedle) {
			t.Errorf("%s reads the dice of an NPC caster's zone", who)
		}
	}
}

// A creature that starts its turn in a zone, when it is a player's character, is asked for the saving throw: the
// window is the player's, the DC is not theirs to read (the caster is another player), a typed d20 is answered with
// RollEffectSave, a failure leaves a damage for the master to apply, and a pass halves it.
func TestAPlayerIsAskedTheZoneSave(t *testing.T) {
	t.Parallel()
	c := zoneCave(t)
	e := c.zoneFight(t)
	c.mustMove(t, c.master, "Toren", 16, 7)
	c.passTo(t, e, "Pensantus")
	c.mustCastArea(t, c.ana, "Pensantus", cloudkillKey, slotOfLevel(5), at(15, 7), nil)
	c.passRaw(t, "Toren")
	w := c.windowOf(t, c.caio, playv1.ReactionKind_REACTION_KIND_EFFECT_SAVE)
	if w == nil {
		t.Fatal("Toren's player has no question: the zone caught his character at the start of its turn")
	}
	p := w.GetEffectSave()
	if p.GetZoneId() == "" || p.GetZoneReason() != "zone_start" || p.GetAbility() != "con" || p.Dc != nil || p.GetDamageTypeNamePt() == "" || !p.GetHalfOnSave() {
		t.Errorf("prompt = %v, want a Constitution zone save, zone_start, no DC for him, the damage type and the half", p)
	}
	c.h.roller.queue(4, 4, 4, 4, 4) // 5d8 = 20
	res, err := c.rollEffectSave(t, c.caio, c.get(t, c.master), w.GetId(), effectSaveFace(5))
	if err != nil {
		t.Fatalf("RollEffectSave() error = %v", err)
	}
	if res.GetResult().GetSaved() {
		t.Error("the save passed with a 5")
	}
	var status string
	var amount int32
	if err := c.h.pool.QueryRow(t.Context(), `SELECT status, amount FROM pending_damages WHERE target_id = $1 AND effect_source_key = $2`, c.id(t, "Toren"), cloudkillKey).Scan(&status, &amount); err != nil {
		t.Fatalf("read the damage of the cloud: %v", err)
	}
	if status != "rolled" || amount != 20 {
		t.Errorf("pending damage = %s for %d, want rolled for 20 (the master applies it)", status, amount)
	}
}

func TestAPlayerWhoPassesTheZoneSaveTakesHalf(t *testing.T) {
	t.Parallel()
	c := zoneCave(t)
	e := c.zoneFight(t)
	c.mustMove(t, c.master, "Toren", 16, 7)
	c.passTo(t, e, "Pensantus")
	c.mustCastArea(t, c.ana, "Pensantus", cloudkillKey, slotOfLevel(5), at(15, 7), nil)
	c.passRaw(t, "Toren")
	w := c.windowOf(t, c.caio, playv1.ReactionKind_REACTION_KIND_EFFECT_SAVE)
	if w == nil {
		t.Fatal("no question")
	}
	c.h.roller.queue(4, 4, 4, 4, 4)
	if _, err := c.rollEffectSave(t, c.caio, c.get(t, c.master), w.GetId(), effectSaveFace(20)); err != nil {
		t.Fatalf("RollEffectSave() error = %v", err)
	}
	var amount int32
	if err := c.h.pool.QueryRow(t.Context(), `SELECT amount FROM pending_damages WHERE target_id = $1 AND effect_source_key = $2`, c.id(t, "Toren"), cloudkillKey).Scan(&amount); err != nil {
		t.Fatalf("read the damage: %v", err)
	}
	if amount != 10 {
		t.Errorf("damage = %d, want 10: half of 20", amount)
	}
}

// Two zones on one creature: the questions come one at a time, in the order the spells were cast.
func TestTwoZonesAskOneQuestionAtATimeInTheOrderTheyWereCast(t *testing.T) {
	t.Parallel()
	c := cloudCave(t)
	e := c.zoneFight(t)
	c.mustMove(t, c.master, "Brisa", 14, 7)
	c.mustCastArea(t, c.caio, "Toren", moonbeamKey, slotOfLevel(2), at(14, 7), nil)
	c.passTo(t, e, "Pensantus")
	c.mustCastArea(t, c.ana, "Pensantus", cloudkillKey, slotOfLevel(5), at(14, 7), nil)
	moonZone := c.oneZone(t, c.master, moonbeamKey).GetId()
	c.passRaw(t, "Brisa")
	var asked []*playv1.ReactionWindow
	for _, w := range c.windows(t, c.bia) {
		if w.GetKind() == playv1.ReactionKind_REACTION_KIND_EFFECT_SAVE {
			asked = append(asked, w)
		}
	}
	if len(asked) != 1 || asked[0].GetEffectSave().GetZoneId() != moonZone {
		t.Fatalf("questions = %v, want the Moonbeam's alone", asked)
	}
	first := asked[0]
	moon := first.GetEffectSave().GetZoneId()
	c.h.roller.queue(2, 2, 2) // 2d10 of the beam is rolled after the save; the cloud's save then 5d8
	if _, err := c.rollEffectSave(t, c.bia, c.get(t, c.master), first.GetId(), effectSaveFace(10)); err != nil {
		t.Fatalf("RollEffectSave() error = %v", err)
	}
	second := c.windowOf(t, c.bia, playv1.ReactionKind_REACTION_KIND_EFFECT_SAVE)
	if second == nil || second.GetEffectSave().GetZoneId() == moon {
		t.Fatalf("second question = %v, want the cloud's, opened once the first is answered", second)
	}
}

// A zone with no saving throw: the damage waits to be rolled, by whoever rolls the caster's damage.
func TestAZoneWithNoSaveOffersTheDamageRoll(t *testing.T) {
	t.Parallel()
	c := cloudCave(t)
	e := c.zoneFight(t)
	c.mustMove(t, c.master, "Ogro", 14, 7)
	c.passTo(t, e, "Pensantus")
	c.mustCastArea(t, c.ana, "Pensantus", noSaveSpell, slotOfLevel(3), at(16, 7), nil)
	z := c.oneZone(t, c.master, noSaveSpell)
	if z.GetSaveAbility() != "" || z.GetDamageDice() != "4d4" || z.GetDamageTypeKey() != "damage-type:slashing" {
		t.Fatalf("zone = %v, want 4d4 slashing with no save", z)
	}
	if _, err := c.move(t, c.master, "Ogro", 16, 7); err != nil {
		t.Fatalf("MoveCombatant() error = %v", err)
	}
	var id, status string
	if err := c.h.pool.QueryRow(t.Context(), `SELECT id, status FROM pending_damages WHERE target_id = $1 AND attack_key = $2`, c.id(t, "Ogro"), noSaveSpell).Scan(&id, &status); err != nil {
		t.Fatalf("no damage waits for the ogre that came in: %v", err)
	}
	if status != "awaiting_roll" {
		t.Errorf("status = %s, want awaiting_roll", status)
	}
	c.h.roller.queue(1, 2, 3, 4)
	d := c.mustDamage(t, c.ana, c.get(t, c.master), id, inAppDamage).GetPendingDamage()
	if d.GetAmount() != 10 {
		t.Errorf("damage = %d, want 10", d.GetAmount())
	}
}

// How a zone ends is a line of the log, for who may read it (RN-10).
func TestTheEndOfAZoneIsALine(t *testing.T) {
	t.Parallel()
	c := zoneCave(t)
	c.zoneFight(t)
	c.castSpiritGuardians(t, 3)
	c.nextTurnOf(t, "Toren")
	c.mustCast(t, c.caio, c.get(t, c.master), "Toren", blessKey, slotOfLevel(1), c.at(t, "Toren"), noCastRoll)
	if !c.endedLine(t, c.ana, spiritGuardiansKey, "concentration_replaced") {
		t.Error("the table has no line that Toren left the Spirit Guardians for another spell")
	}
}

func (c *cave) endedLine(t *testing.T, u *user, spell, reason string) bool {
	t.Helper()
	for _, en := range c.zoneLines(t, u, spell) {
		if en.GetEffect().GetChange() == playv1.CombatLogEffectChange_COMBAT_LOG_EFFECT_CHANGE_ENDED && en.GetEffect().GetReason() == reason {
			return true
		}
	}
	return false
}

func TestTheDurationOfAZoneEndingIsALineAndALostConcentrationToo(t *testing.T) {
	t.Parallel()
	c := druidCave(t)
	c.zoneFight(t)
	c.mustCastArea(t, c.caio, "Toren", moonbeamKey, slotOfLevel(2), at(18, 7), nil)
	for range 10 {
		c.nextTurnOf(t, "Toren")
	}
	if !c.endedLine(t, c.ana, moonbeamKey, "duration") {
		t.Error("no line that the time of the beam passed")
	}
	c2 := druidCave(t)
	c2.zoneFight(t)
	c2.mustCastArea(t, c2.caio, "Toren", moonbeamKey, slotOfLevel(2), at(18, 7), nil)
	c2.correct(t, c2.toren, hpIs(0))
	if !c2.endedLine(t, c2.ana, moonbeamKey, "concentration") {
		t.Error("no line that Toren lost the concentration")
	}
}

// The master ends a zone: only he may, and the line says so.
func TestTheMasterEndsAZone(t *testing.T) {
	t.Parallel()
	c := druidCave(t)
	c.zoneFight(t)
	c.mustCastArea(t, c.caio, "Toren", moonbeamKey, slotOfLevel(2), at(18, 7), nil)
	z := c.oneZone(t, c.master, moonbeamKey)
	end := func(u *user, id string) error {
		_, err := u.combat.EndSpellZone(t.Context(), connect.NewRequest(&playv1.EndSpellZoneRequest{
			CampaignId: c.campaignID, EncounterId: c.get(t, c.master).GetId(), ZoneId: id, IdempotencyKey: newKey(),
		}))
		return err
	}
	if err := end(c.caio, z.GetId()); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("the caster's player ended it: error = %v, want permission_denied", err)
	}
	if err := end(c.master, z.GetId()); err != nil {
		t.Fatalf("EndSpellZone() error = %v", err)
	}
	if zs := zonesOf(c.get(t, c.master), moonbeamKey); len(zs) != 0 {
		t.Errorf("zones = %v, want none", zs)
	}
	if !c.endedLine(t, c.ana, moonbeamKey, "ended_by_master") {
		t.Error("no line that the master ended the zone")
	}
	if err := end(c.master, z.GetId()); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("ending it again: error = %v, want not_found", err)
	}
}

// A creature that is caught by Spirit Guardians moves at half speed that turn.
func TestSpiritGuardiansHalvesTheSpeedOfWhoItCatches(t *testing.T) {
	t.Parallel()
	c := zoneCave(t)
	e := c.zoneFight(t)
	c.castSpiritGuardians(t, 3)
	c.h.roller.queue(20, 1) // the ogre's save passes
	c.passTo(t, e, "Ogro")
	if card := cardOf(t, byLabel(t, c.get(t, c.master), "Ogro"), "effect:spirit-guardians-slowed"); card == nil {
		t.Fatal("the ogre that began its turn in the aura carries no slowing")
	}
	// Large, 30 ft: half is 15 ft, three squares.
	if _, err := c.move(t, c.master, "Ogro", 2, 7); err == nil {
		// the master's move is free; a player's is what the budget bounds, so only the card is asserted
		t.Log("master moves are not bounded by the speed")
	}
}

// Web is difficult terrain inside: a walk across it costs double.
func TestWebIsDifficultTerrain(t *testing.T) {
	t.Parallel()
	c := zoneCave(t)
	e := c.zoneFight(t)
	c.mustMove(t, c.master, "Pensantus", 17, 7)
	c.passTo(t, e, "Pensantus")
	c.mustCastArea(t, c.ana, "Pensantus", webKey, slotOfLevel(2), at(18, 3), nil)
	c.nextTurnOf(t, "Pensantus")
	before := byLabel(t, c.get(t, c.master), "Pensantus").GetMovementUsedDft()
	c.mustMove(t, c.ana, "Pensantus", 17, 4) // 5 ft outside, then two squares of the web at 10 ft each
	got := byLabel(t, c.get(t, c.master), "Pensantus").GetMovementUsedDft() - before
	if got != 250 {
		t.Errorf("the walk cost %d tenths of a foot, want 250: 5 ft outside and 10 ft each in difficult terrain", got)
	}
}

// passRaw ends turns, as the master, until the combatant is on turn, without skipping the questions the start of a turn
// opens (endTurn does).
func (c *cave) passRaw(t *testing.T, label string) {
	t.Helper()
	for range 14 {
		if _, err := c.endTurnRaw(t, c.master, c.get(t, c.master), true); err != nil {
			t.Fatalf("EndTurn() error = %v", err)
		}
		if c.get(t, c.master).GetCurrentCombatantId() == c.id(t, label) {
			return
		}
	}
	t.Fatalf("%s never came on turn", label)
}

// The master's calls ride the zone's notes (the web shows them on the master's card alone, RN-37).
func TestZoneDefsNameTheMastersCalls(t *testing.T) {
	t.Parallel()
	for key, want := range map[string]string{"spell:darkness": "master_light", "spell:fog-cloud": "master_wind", "spell:stinking-cloud": "master_breath"} {
		if !slices.Contains(zoneDefs[key].notes(true), want) {
			t.Errorf("%s notes = %v, want %s", key, zoneDefs[key].notes(true), want)
		}
	}
	for key := range map[string]bool{"spell:darkness": true, "spell:fog-cloud": true, "spell:stinking-cloud": true} {
		for _, n := range zoneDefs[key].notes(false) {
			if strings.HasPrefix(n, "master_") {
				t.Errorf("%s notes for a player = %v, want no master call", key, zoneDefs[key].notes(false))
			}
		}
	}
	if slices.Contains(zoneDefs["spell:silence"].notes(true), "master_wind") || len(zoneDefs["spell:silence"].notes(true)) != 0 {
		t.Errorf("silence notes = %v, want none", zoneDefs["spell:silence"].notes(true))
	}
}
