package play

import (
	"slices"
	"testing"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// Grease and Entangle (SRD 5.1) keep their area on the map. These tests need the database
// (MEURPG_TEST_DATABASE_URL). The cave is the one of zones_test.go: the ogre (saving throws +0) stands at
// (7,7), Pensantus the wizard at (12,7), Toren at (5,7), the goblin far to the east.

const (
	greaseKey    = "spell:grease"
	entangleKey  = "spell:entangle"
	entangleFx   = "effect:entangle-restrained"
	restrainedC  = "condition:restrained"
	proneC       = condProne
	greaseOrigin = 10 // the column of an empty patch of the strip: nobody stands within a square of it
)

// greaseCave is the zone cave with Pensantus knowing Grease.
func greaseCave(t *testing.T) *cave {
	t.Helper()
	return newCaveOf(t, 9, []string{greaseKey, thunderwave, webKey}, func(a *armed) *charactersv1.Character {
		return a.caio.caster(t, a.campaignID, "Toren", "class:cleric", "race:human", 7, abilities(14, 12, 14, 16, 10),
			[]string{maceKey}, []string{sacredFlame}, nil, []string{blessKey})
	})
}

// entangleCave is the cave with Toren a druid of level 3 who prepared Entangle (Wisdom 16: save DC 13).
func entangleCave(t *testing.T) *cave {
	t.Helper()
	return newCaveOf(t, 5, []string{thunderwave}, func(a *armed) *charactersv1.Character {
		return a.caio.caster(t, a.campaignID, "Toren", "class:druid", "race:human", 3, abilities(12, 12, 14, 16, 10),
			nil, nil, nil, []string{entangleKey})
	})
}

func (c *cave) isProne(t *testing.T, label string) bool {
	t.Helper()
	return slices.Contains(byLabel(t, c.get(t, c.master), label).GetConditions(), proneC)
}

// zoneReasons are the reasons of the lines the master reads for the zones of a spell, in order.
func (c *cave) zoneReasons(t *testing.T, spell string) []string {
	t.Helper()
	var out []string
	for _, en := range c.zoneLines(t, c.master, spell) {
		out = append(out, en.GetEffect().GetReason())
	}
	slices.Reverse(out) // the log reads latest first
	return out
}

// entangleFight starts the fight with Toren at (17,7), in reach of the cube around (18,3), and the ogre inside it.
func (c *cave) entangleFight(t *testing.T) {
	t.Helper()
	c.zoneFight(t)
	c.mustMove(t, c.master, "Toren", 17, 7)
	c.mustMove(t, c.master, "Ogro", 18, 3)
}

// castGrease has Pensantus cast Grease at the column on row 7, on the turn of Pensantus.
func (c *cave) castGrease(t *testing.T, col int32) {
	t.Helper()
	c.passTo(t, c.zoneFight(t), "Pensantus")
	c.mustCastArea(t, c.ana, "Pensantus", greaseKey, slotOfLevel(1), at(col, 7), nil)
}

// Grease covers a 10-foot square at a point for a minute, with no concentration, and it is difficult terrain.
func TestGreaseIsASquareOfTenFeetForAMinuteWithoutConcentration(t *testing.T) {
	t.Parallel()
	c := greaseCave(t)
	c.castGrease(t, greaseOrigin)
	z := c.oneZone(t, c.master, greaseKey)
	if z.GetShape() != rulesv1.SpellAreaShape_SPELL_AREA_SHAPE_CUBE || z.GetSizeFt() != 10 || z.GetConcentration() || z.GetRoundsLeft() != 10 {
		t.Fatalf("zone = %v, want a 10-foot cube for 10 rounds with no concentration", z)
	}
	if len(z.GetSquares()) != 4 || !zoneHas(z, greaseOrigin, 7) {
		t.Errorf("squares = %v, want 4 squares including the point", z.GetSquares())
	}
	if !hasNote(z, "difficult_terrain") || z.GetSaveAbility() != "dex" {
		t.Errorf("zone = %v, want difficult terrain and a Dexterity save", z)
	}
	if c.get(t, c.master).GetCurrentCombatantId() != c.id(t, "Pensantus") || byLabel(t, c.get(t, c.master), "Pensantus").GetConcentrationSpell() != "" {
		t.Errorf("the caster concentrates on Grease: it asks none")
	}
}

// A creature standing in the area when Grease appears makes a Dexterity save or falls prone; a pass stays up.
func TestGreaseKnocksDownWhoStandsInItAtTheCast(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		roll   int
		prone  bool
		change playv1.CombatLogEffectChange
	}{
		{"failed", 5, true, playv1.CombatLogEffectChange_COMBAT_LOG_EFFECT_CHANGE_SAVE_FAILED},
		{"passed", 20, false, playv1.CombatLogEffectChange_COMBAT_LOG_EFFECT_CHANGE_SAVED},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := greaseCave(t)
			c.passTo(t, c.zoneFight(t), "Pensantus")
			c.h.roller.queue(tt.roll) // the ogre's Dexterity save: +0
			c.mustCastArea(t, c.ana, "Pensantus", greaseKey, slotOfLevel(1), at(7, 7), nil)
			if got := c.isProne(t, "Ogro"); got != tt.prone {
				t.Errorf("the ogre prone = %v, want %v", got, tt.prone)
			}
			lines := c.zoneLines(t, c.master, greaseKey)
			if len(lines) != 1 || lines[0].GetEffect().GetReason() != "zone_cast" || lines[0].GetEffect().GetChange() != tt.change {
				t.Errorf("lines = %v, want one zone_cast line with the change %v", lines, tt.change)
			}
		})
	}
}

// The cast catches only the creatures in the area: the ones outside are left alone.
func TestGreaseLeavesWhoIsOutsideAlone(t *testing.T) {
	t.Parallel()
	c := greaseCave(t)
	c.castGrease(t, greaseOrigin)
	for _, label := range []string{"Ogro", "Toren", "Pensantus", "Goblin"} {
		if c.isProne(t, label) {
			t.Errorf("%s is prone: it was not in the area", label)
		}
	}
	if lines := c.zoneLines(t, c.master, greaseKey); len(lines) != 0 {
		t.Errorf("lines = %v, want none", lines)
	}
}

// A creature that walks into the area makes the save, and a creature that ends its turn inside makes it again.
func TestGreaseCatchesWhoEntersAndWhoEndsItsTurnInside(t *testing.T) {
	t.Parallel()
	c := greaseCave(t)
	c.castGrease(t, greaseOrigin)
	e := c.get(t, c.master)
	c.passTo(t, e, "Ogro")
	c.h.roller.queue(20) // the save on entering: passed
	c.mustMove(t, c.master, "Ogro", greaseOrigin, 7)
	if c.isProne(t, "Ogro") {
		t.Fatal("the ogre that passed the save on entering is prone")
	}
	c.h.roller.queue(5) // the save at the end of its turn: failed
	if _, err := c.endTurn(t, c.master, c.get(t, c.master), true); err != nil {
		t.Fatalf("EndTurn() error = %v", err)
	}
	if !c.isProne(t, "Ogro") {
		t.Error("the ogre that ended its turn in the grease and failed the save is not prone")
	}
	if got, want := c.zoneReasons(t, greaseKey), []string{"zone_enter", "zone_end"}; !slices.Equal(got, want) {
		t.Errorf("reasons = %v, want %v", got, want)
	}
}

// A creature that does not end its turn in the area is not asked.
func TestGreaseDoesNotAskWhoEndsItsTurnOutside(t *testing.T) {
	t.Parallel()
	c := greaseCave(t)
	c.castGrease(t, greaseOrigin)
	c.passTo(t, c.get(t, c.master), "Ogro")
	if _, err := c.endTurn(t, c.master, c.get(t, c.master), true); err != nil {
		t.Fatalf("EndTurn() error = %v", err)
	}
	if lines := c.zoneLines(t, c.master, greaseKey); len(lines) != 0 {
		t.Errorf("lines = %v, want none", lines)
	}
}

// Grease does not ask a creature at the start of its turn: only the cast, the entry and the end of the turn.
func TestGreaseDoesNotAskAtTheStartOfATurn(t *testing.T) {
	t.Parallel()
	c := greaseCave(t)
	c.passTo(t, c.zoneFight(t), "Pensantus")
	c.h.roller.queue(20)
	c.mustCastArea(t, c.ana, "Pensantus", greaseKey, slotOfLevel(1), at(7, 7), nil) // the ogre passes
	c.passTo(t, c.get(t, c.master), "Ogro")
	if lines := c.zoneReasons(t, greaseKey); !slices.Equal(lines, []string{"zone_cast"}) {
		t.Errorf("reasons = %v, want only the cast", lines)
	}
}

// Grease ends by its round, not by concentration: another concentration spell leaves it, and ten rounds end it.
func TestGreaseEndsByItsRoundNotByConcentration(t *testing.T) {
	t.Parallel()
	c := greaseCave(t)
	c.castGrease(t, greaseOrigin)
	c.nextTurnOf(t, "Pensantus")
	c.mustCastArea(t, c.ana, "Pensantus", webKey, slotOfLevel(2), at(18, 3), nil)
	if len(zonesOf(c.get(t, c.master), greaseKey)) != 1 {
		t.Fatal("Grease ended with the concentration of another spell")
	}
	for range 8 {
		c.nextTurnOf(t, "Pensantus")
	}
	if zs := zonesOf(c.get(t, c.master), greaseKey); len(zs) != 1 || zs[0].GetRoundsLeft() != 1 {
		t.Fatalf("zones in round 10 = %v, want Grease with one round left", zs)
	}
	c.nextTurnOf(t, "Pensantus")
	if zs := zonesOf(c.get(t, c.master), greaseKey); len(zs) != 0 {
		t.Errorf("zones after ten rounds = %v, want none", zs)
	}
}

// The undo of the cast takes the zone away and stands the creature it knocked down up again.
func TestGreaseUndoOfTheCastRestoresTheZoneAndTheCondition(t *testing.T) {
	t.Parallel()
	c := greaseCave(t)
	c.passTo(t, c.zoneFight(t), "Pensantus")
	c.h.roller.queue(5)
	c.mustCastArea(t, c.ana, "Pensantus", greaseKey, slotOfLevel(1), at(7, 7), nil)
	if !c.isProne(t, "Ogro") {
		t.Fatal("the ogre is not prone after the failed save")
	}
	c.undoLast(t)
	if zs := c.get(t, c.master).GetZones(); len(zs) != 0 {
		t.Errorf("zones after the undo = %v, want none", zs)
	}
	if c.isProne(t, "Ogro") {
		t.Error("the ogre is still prone after the undo of the cast")
	}
}

// Entangle fills a 20-foot cube for a minute of concentration; it is difficult terrain.
func TestEntangleIsACubeOfTwentyFeetHeldByConcentration(t *testing.T) {
	t.Parallel()
	c := entangleCave(t)
	c.entangleFight(t)
	c.h.roller.queue(20)
	c.mustCastArea(t, c.caio, "Toren", entangleKey, slotOfLevel(1), at(18, 3), nil)
	z := c.oneZone(t, c.master, entangleKey)
	if z.GetShape() != rulesv1.SpellAreaShape_SPELL_AREA_SHAPE_CUBE || z.GetSizeFt() != 20 || !z.GetConcentration() || len(z.GetSquares()) != 16 {
		t.Fatalf("zone = %v, want a 20-foot cube of 16 squares held by concentration", z)
	}
	if !hasNote(z, "difficult_terrain") || z.GetSaveAbility() != "str" {
		t.Errorf("zone = %v, want difficult terrain and a Strength save", z)
	}
}

// Whoever stands in the area at the cast makes a Strength save or is restrained until the spell ends, with the
// escape the web gives: a Strength check against the spell's DC.
func TestEntangleRestrainsWhoFailsTheSaveAtTheCast(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name       string
		roll       int
		restrained bool
	}{{"failed", 5, true}, {"passed", 20, false}} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := entangleCave(t)
			c.zoneFight(t)
			c.h.roller.queue(tt.roll)
			c.mustCastArea(t, c.caio, "Toren", entangleKey, slotOfLevel(1), at(18, 3), nil)
			ogre := byLabel(t, c.get(t, c.master), "Ogro")
			card := cardOf(t, ogre, entangleFx)
			if (card != nil) != tt.restrained || slices.Contains(ogre.GetConditions(), restrainedC) != tt.restrained {
				t.Fatalf("the ogre = %v, want restrained = %v", ogre, tt.restrained)
			}
			if card != nil && card.GetBreakFreeAbility() != "str" {
				t.Errorf("break free = %q, want a Strength check", card.GetBreakFreeAbility())
			}
			if got := c.zoneReasons(t, entangleKey); !slices.Equal(got, []string{"zone_cast"}) {
				t.Errorf("reasons = %v, want the cast", got)
			}
		})
	}
}

// Entering later does nothing: no save, no condition, and neither does the start or the end of a turn there.
func TestEntangleDoesNothingToWhoComesInLater(t *testing.T) {
	t.Parallel()
	c := entangleCave(t)
	c.entangleFight(t)
	c.h.roller.queue(20)
	c.mustCastArea(t, c.caio, "Toren", entangleKey, slotOfLevel(1), at(18, 3), nil)
	e := c.get(t, c.master)
	c.passTo(t, e, "Goblin")
	c.mustMove(t, c.master, "Goblin", 20, 5)
	if _, err := c.endTurn(t, c.master, c.get(t, c.master), true); err != nil {
		t.Fatalf("EndTurn() error = %v", err)
	}
	gob := byLabel(t, c.get(t, c.master), "Goblin")
	if cardOf(t, gob, entangleFx) != nil || len(gob.GetConditions()) != 0 {
		t.Errorf("the goblin that came in later = %v, want it free", gob)
	}
	if got := c.zoneReasons(t, entangleKey); !slices.Equal(got, []string{"zone_cast"}) {
		t.Errorf("reasons = %v, want only the one of the cast", got)
	}
}

// Entangle ends with the caster's concentration, and the restraint with it.
func TestEntangleEndsWithTheConcentrationAndFreesTheCreature(t *testing.T) {
	t.Parallel()
	c := entangleCave(t)
	c.entangleFight(t)
	c.h.roller.queue(5)
	c.mustCastArea(t, c.caio, "Toren", entangleKey, slotOfLevel(1), at(18, 3), nil)
	if cardOf(t, byLabel(t, c.get(t, c.master), "Ogro"), entangleFx) == nil {
		t.Fatal("the ogre is not restrained")
	}
	c.nextTurnOf(t, "Toren")
	c.mustCastArea(t, c.caio, "Toren", entangleKey, slotOfLevel(1), at(16, 7), nil) // a new concentration
	zs := zonesOf(c.get(t, c.master), entangleKey)
	if len(zs) != 1 || zs[0].GetOrigin().GetCol() != 16 {
		t.Fatalf("zones = %v, want only the new one", zs)
	}
	ogre := byLabel(t, c.get(t, c.master), "Ogro")
	if cardOf(t, ogre, entangleFx) != nil || slices.Contains(ogre.GetConditions(), restrainedC) {
		t.Errorf("the ogre = %v, want it free when the concentration ended", ogre)
	}
}

// The undo of the cast takes the zone and the restraint away.
func TestEntangleUndoOfTheCastRestoresTheZoneAndTheCreature(t *testing.T) {
	t.Parallel()
	c := entangleCave(t)
	c.entangleFight(t)
	c.h.roller.queue(5)
	c.mustCastArea(t, c.caio, "Toren", entangleKey, slotOfLevel(1), at(18, 3), nil)
	c.undoLast(t)
	if zs := c.get(t, c.master).GetZones(); len(zs) != 0 {
		t.Errorf("zones after the undo = %v, want none", zs)
	}
	ogre := byLabel(t, c.get(t, c.master), "Ogro")
	if cardOf(t, ogre, entangleFx) != nil || slices.Contains(ogre.GetConditions(), restrainedC) {
		t.Errorf("the ogre = %v, want it free after the undo", ogre)
	}
}

// A player's character standing in the grease is asked its save, like a creature entering a damage zone, and
// a failed one falls prone.
func TestGreaseAsksThePlayerOfWhoStandsInItAtTheCast(t *testing.T) {
	t.Parallel()
	c := greaseCave(t)
	c.passTo(t, c.zoneFight(t), "Pensantus")
	c.mustCastArea(t, c.ana, "Pensantus", greaseKey, slotOfLevel(1), at(12, 7), nil) // on herself
	w := c.windowOf(t, c.ana, playv1.ReactionKind_REACTION_KIND_EFFECT_SAVE)
	if w == nil {
		t.Fatal("the wizard standing in the grease is not asked the save")
	}
	if c.isProne(t, "Pensantus") {
		t.Fatal("the wizard is prone before the save was rolled")
	}
	c.h.roller.queue(1)
	if _, err := c.rollEffectSave(t, c.ana, c.get(t, c.master), w.GetId(), effectSaveFace(1)); err != nil {
		t.Fatalf("RollEffectSave() error = %v", err)
	}
	if !c.isProne(t, "Pensantus") {
		t.Error("the wizard that failed the save is not prone")
	}
}

// The undo of the cast closes the question it opened: the zone it belongs to is gone.
func TestGreaseUndoOfTheCastClosesThePlayersQuestion(t *testing.T) {
	t.Parallel()
	c := greaseCave(t)
	c.passTo(t, c.zoneFight(t), "Pensantus")
	c.mustCastArea(t, c.ana, "Pensantus", greaseKey, slotOfLevel(1), at(12, 7), nil)
	if c.windowOf(t, c.ana, playv1.ReactionKind_REACTION_KIND_EFFECT_SAVE) == nil {
		t.Fatal("the wizard is not asked the save")
	}
	c.undoLast(t)
	if w := c.windowOf(t, c.ana, playv1.ReactionKind_REACTION_KIND_EFFECT_SAVE); w != nil {
		t.Errorf("the question of the undone cast is still open: %v", w)
	}
	if zs := c.get(t, c.master).GetZones(); len(zs) != 0 {
		t.Errorf("zones after the undo = %v, want none", zs)
	}
}

// castHitsOf are the targets the last spell_cast event of the session recorded, in order, with their saving throws.
func (c *cave) castHitsOf(t *testing.T) []castHit {
	t.Helper()
	var payload []byte
	if err := c.h.pool.QueryRow(t.Context(), `SELECT payload FROM session_events WHERE kind = $1 ORDER BY seq DESC LIMIT 1`, eventSpellCast).Scan(&payload); err != nil {
		t.Fatalf("read the last cast: %v", err)
	}
	ev, err := readEvent(payload)
	if err != nil {
		t.Fatalf("decode the cast: %v", err)
	}
	return ev.Hits
}

// A player answers the save of a zone at the cast and fails it; the master's undo still finds the cast, and takes
// the fall back with the zone, the NPC's too.
func TestGreaseUndoOfTheCastAfterThePlayerAnswersAndFails(t *testing.T) {
	t.Parallel()
	c := greaseCave(t)
	c.passTo(t, c.zoneFight(t), "Pensantus")
	c.mustMove(t, c.master, "Ogro", 13, 7)
	c.h.roller.queue(5) // the ogre's save
	c.mustCastArea(t, c.ana, "Pensantus", greaseKey, slotOfLevel(1), at(12, 7), nil)
	w := c.windowOf(t, c.ana, playv1.ReactionKind_REACTION_KIND_EFFECT_SAVE)
	if w == nil {
		t.Fatal("the wizard is not asked the save")
	}
	c.h.roller.queue(1)
	if _, err := c.rollEffectSave(t, c.ana, c.get(t, c.master), w.GetId(), effectSaveFace(1)); err != nil {
		t.Fatalf("RollEffectSave() error = %v", err)
	}
	if !c.isProne(t, "Pensantus") || !c.isProne(t, "Ogro") {
		t.Fatalf("prone after the answer: wizard %v, ogre %v, want both", c.isProne(t, "Pensantus"), c.isProne(t, "Ogro"))
	}
	c.undoLast(t)
	if c.isProne(t, "Pensantus") || c.isProne(t, "Ogro") {
		t.Errorf("prone after the undo: wizard %v, ogre %v, want neither", c.isProne(t, "Pensantus"), c.isProne(t, "Ogro"))
	}
	if zs := c.get(t, c.master).GetZones(); len(zs) != 0 {
		t.Errorf("zones after the undo = %v, want none", zs)
	}
}

// The same for Entangle: the restraint the answer added goes with the cast.
func TestEntangleUndoOfTheCastAfterThePlayerAnswersAndFails(t *testing.T) {
	t.Parallel()
	c := entangleCave(t)
	c.entangleFight(t)
	c.mustMove(t, c.master, "Ogro", 18, 7)
	c.h.roller.queue(5) // the ogre's save
	c.mustCastArea(t, c.caio, "Toren", entangleKey, slotOfLevel(1), at(17, 7), nil)
	w := c.windowOf(t, c.caio, playv1.ReactionKind_REACTION_KIND_EFFECT_SAVE)
	if w == nil {
		t.Fatal("the druid is not asked the save")
	}
	c.h.roller.queue(1)
	if _, err := c.rollEffectSave(t, c.caio, c.get(t, c.master), w.GetId(), effectSaveFace(1)); err != nil {
		t.Fatalf("RollEffectSave() error = %v", err)
	}
	if cardOf(t, byLabel(t, c.get(t, c.master), "Toren"), entangleFx) == nil {
		t.Fatal("the druid is not restrained after the failed answer")
	}
	c.undoLast(t)
	for _, label := range []string{"Toren", "Ogro"} {
		cb := byLabel(t, c.get(t, c.master), label)
		if cardOf(t, cb, entangleFx) != nil || slices.Contains(cb.GetConditions(), restrainedC) {
			t.Errorf("%s after the undo = %v, want free", label, cb)
		}
	}
	if zs := c.get(t, c.master).GetZones(); len(zs) != 0 {
		t.Errorf("zones after the undo = %v, want none", zs)
	}
}

// The cast records every creature it caught, whatever its save: the one that passed is there as the one that
// failed, so what the cast reveals or lists does not tell the saves apart.
func TestTheCastRecordsEveryCreatureItCaughtWhateverTheSave(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		build func(*testing.T) *cave
		cast  func(*testing.T, *cave, int)
	}{
		{"grease", greaseCave, func(t *testing.T, c *cave, face int) {
			c.passTo(t, c.zoneFight(t), "Pensantus")
			c.h.roller.queue(face)
			c.mustCastArea(t, c.ana, "Pensantus", greaseKey, slotOfLevel(1), at(7, 7), nil)
		}},
		{"entangle", entangleCave, func(t *testing.T, c *cave, face int) {
			c.entangleFight(t)
			c.h.roller.queue(face)
			c.mustCastArea(t, c.caio, "Toren", entangleKey, slotOfLevel(1), at(18, 3), nil)
		}},
	} {
		for _, face := range []int{1, 20} { // fail, pass
			t.Run(tc.name+"/"+map[int]string{1: "fail", 20: "pass"}[face], func(t *testing.T) {
				t.Parallel()
				c := tc.build(t)
				tc.cast(t, c, face)
				hits := c.castHitsOf(t)
				if len(hits) != 1 || hits[0].Target != c.id(t, "Ogro") || hits[0].Save == nil {
					t.Errorf("hits = %+v, want the ogre alone, with its save", hits)
				}
			})
		}
	}
}
