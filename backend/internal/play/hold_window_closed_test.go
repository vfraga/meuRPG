package play

import (
	"testing"

	connect "connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The question kept for a combat save (the Bardic die, Lucky) and the Inspiration it may have spent go with the window of the save when the
// window closes by itself: the roll never happens (hold_forget.go). These tests run the real triggers that close the window of a
// concentration save and of the save of an effect (a player's combatant cannot be removed from a running combat, so that is no trigger; the
// windows of a combat that ends are deleted with it, not closed; a player at 0 hit points keeps its window), each with a hold of a die, a hold of Lucky, and a hold that spent the Inspiration.

// savedHold is a table with an unanswered held question of a save and the way to read it.
type savedHold struct {
	f      *favorFight
	kind   string // inspiration_holds.kind
	who    string // the label of the roller
	id     string
	spent  bool
	hasDie bool
}

// heldSaveKinds are the three questions a save keeps.
var heldSaveKinds = []struct {
	name       string
	die, spent bool
}{
	{"a Bardic die", true, false},
	{"a Lucky 1", false, false},
	{"a Lucky 1 with the Inspiration spent", false, true},
	{"a Bardic die with the Inspiration spent", true, true},
}

// holdOf reads the one unanswered hold of the kind and, when the case spent the Inspiration, writes the spend the way the paths that take
// it write it: the hold's favor_spent and the character's row gone.
func (h *savedHold) keep(t *testing.T, _ string, _ string) {
	t.Helper()
	if err := h.f.h.pool.QueryRow(t.Context(), `SELECT id FROM inspiration_holds WHERE kind = $1`, h.kind).Scan(&h.id); err != nil {
		t.Fatalf("the question the save kept: %v", err)
	}
	if h.spent {
		// The row was spent: it is not there. (A grant here would be the combat's last action, and the undo this test makes would take it.)
		h.f.execSQL(t, `DELETE FROM character_favor WHERE campaign_id = $1`, h.f.campaignID)
		h.f.execSQL(t, `UPDATE inspiration_holds SET favor_spent = true WHERE id = $1`, h.id)
		if h.f.favorHolds(t) || !h.f.holdFavorOf(t, h.id) {
			t.Fatal("the setup did not leave a hold that spent the Inspiration")
		}
	}
}

// concentrationHold is a halfling Pensantus who concentrates on Teia and is hit: the save asks the die or Lucky and keeps the question.
func concentrationHold(t *testing.T, die, spent bool) *savedHold {
	t.Helper()
	a, e := concentratingHitOn(t, newCastersOf(t, 3, "race:halfling"), 6)
	f := favorFightOf(t, a, e)
	w := a.windowOf(t, a.ana, playv1.ReactionKind_REACTION_KIND_CONCENTRATION_SAVE)
	if w == nil {
		t.Fatal("Pensantus has no concentration window")
	}
	face := int32(1)
	if die {
		a.giveCombatantDie(t, "Pensantus", e)
		face = 3
	}
	if _, err := a.resolveConcentration(t, a.ana, e, w.GetId(), concentrationFace(face)); err != nil {
		t.Fatalf("ResolveConcentrationSave(%d) error = %v", face, err)
	}
	h := &savedHold{f: f, kind: string(heldConcentrationSave), who: "Pensantus", spent: spent, hasDie: die}
	h.keep(t, "Pensantus", a.pens.GetId())
	return h
}

// effectHold is a halfling Toren under Hold Person whose turn ended: the save of the effect asks the die or Lucky and keeps the question.
// It returns the effect row's id too.
func effectHold(t *testing.T, die, spent bool) (*savedHold, string) {
	t.Helper()
	a := fighterTable(t, "race:halfling")
	a.setRules(t, showSaveDC)
	e, w := a.holdOnToren(t)
	f := favorFightOf(t, a, e)
	face := int32(1)
	if die {
		a.giveCombatantDie(t, "Toren", e)
		face = 3
	}
	if _, err := a.rollEffectSave(t, a.caio, e, w.GetId(), effectSaveFace(face)); err != nil {
		t.Fatalf("RollEffectSave(%d) error = %v", face, err)
	}
	h := &savedHold{f: f, kind: string(heldEffectSave), who: "Toren", spent: spent, hasDie: die}
	h.keep(t, "Toren", a.toren.GetId())
	return h, a.rpcEffectID(t, holdPerson, "Toren")
}

// windowClosedByItself says the window of the kind is closed with a reason, as a close by itself leaves it.
func (h *savedHold) windowClosedByItself(t *testing.T, kind string) {
	t.Helper()
	var status string
	var reason *string
	if err := h.f.h.pool.QueryRow(t.Context(), `SELECT status, closed_reason FROM reaction_windows WHERE kind = $1 ORDER BY seq DESC LIMIT 1`, kind).Scan(&status, &reason); err != nil {
		t.Fatalf("read the window: %v", err)
	}
	if status != windowClosed || reason == nil {
		t.Fatalf("the window is %q with reason %v, want it closed by itself with a reason: the test does not reach the path", status, reason)
	}
}

// want says the hold is gone, the Inspiration it spent is back and the Bardic die is where it was (unless its holder left).
func (h *savedHold) want(t *testing.T, what string, dieStays bool) {
	t.Helper()
	if h.f.holdExists(t, h.id) {
		t.Errorf("%s: the hold %s is still there", what, h.id)
	}
	if h.spent && !h.f.favorHolds(t) {
		t.Errorf("%s: the Inspiration did not come back (rows = %d)", what, h.f.rows(t))
	}
	if h.hasDie && dieStays {
		var sides *int32
		if err := h.f.h.pool.QueryRow(t.Context(), `SELECT inspiration_sides FROM combatants WHERE label = $1 AND encounter_id = $2`, h.who, h.f.e.GetId()).Scan(&sides); err != nil {
			t.Fatalf("read the die of %s: %v", h.who, err)
		}
		if sides == nil || *sides != 8 {
			t.Errorf("%s: the die of %s = %v, want the d8 still there", what, h.who, sides)
		}
	}
}

func TestHeldConcentrationSaveGoesWithItsWindow(t *testing.T) {
	t.Parallel()
	closers := []struct {
		name     string
		dieStays bool
		close    func(t *testing.T, h *savedHold)
	}{
		{"the concentration ended first", true, func(t *testing.T, h *savedHold) {
			t.Helper()
			h.f.dropConcentration(t, h.f.e)
		}},
		{"the damage is undone", true, func(t *testing.T, h *savedHold) {
			t.Helper()
			h.f.undoLast(t, h.f.e)
		}},
		{"the encounter ends", false, func(t *testing.T, h *savedHold) {
			t.Helper()
			h.f.endEncounter(t, h.f.e)
		}},
	}
	for _, c := range closers {
		for _, k := range heldSaveKinds {
			t.Run(c.name+" with "+k.name, func(t *testing.T) {
				t.Parallel()
				h := concentrationHold(t, k.die, k.spent)
				c.close(t, h)
				if c.name != "the encounter ends" {
					h.windowClosedByItself(t, "concentration_save")
				}
				h.want(t, c.name, c.dieStays)
			})
		}
	}
}

func TestHeldEffectSaveGoesWithItsWindow(t *testing.T) {
	t.Parallel()
	closers := []struct {
		name     string
		dieStays bool
		close    func(t *testing.T, h *savedHold, effect string)
	}{
		{"the effect ended first", true, func(t *testing.T, h *savedHold, effect string) {
			t.Helper()
			a := h.f.armed
			if _, err := a.rpcEnd(t, a.master, h.f.e, effect, playv1.EffectEndScope_EFFECT_END_SCOPE_THIS); err != nil {
				t.Fatalf("EndLastingEffect() error = %v", err)
			}
		}},
		{"the target leaves the effect", true, func(t *testing.T, h *savedHold, effect string) {
			t.Helper()
			a := h.f.armed
			if _, err := a.rpcRemove(t, a.master, h.f.e, effect, a.id(t, "Toren")); err != nil {
				t.Fatalf("RemoveEffectTarget() error = %v", err)
			}
		}},
		{"the encounter ends", false, func(t *testing.T, h *savedHold, _ string) {
			t.Helper()
			h.f.endEncounter(t, h.f.e)
		}},
	}
	for _, c := range closers {
		for _, k := range heldSaveKinds {
			t.Run(c.name+" with "+k.name, func(t *testing.T) {
				t.Parallel()
				h, effect := effectHold(t, k.die, k.spent)
				c.close(t, h, effect)
				if c.name != "the encounter ends" {
					h.windowClosedByItself(t, "effect_save")
				}
				h.want(t, c.name, c.dieStays)
			})
		}
	}
}

// The save of a zone (a beam a creature enters) keeps its question the same way, and the zone ending closes the window by itself.
func TestHeldZoneSaveGoesWithItsWindow(t *testing.T) {
	t.Parallel()
	c := newCaveOf(t, 5, []string{mistyStep}, func(a *armed) *charactersv1.Character {
		return a.caio.caster(t, a.campaignID, "Toren", "class:druid", "race:human", 3, abilities(12, 12, 14, 16, 10), nil, nil, nil, []string{moonbeamKey})
	})
	e := c.zoneFight(t)
	c.mustCastArea(t, c.caio, "Toren", moonbeamKey, slotOfLevel(2), at(8, 7), nil)
	c.passTo(t, e, "Pensantus")
	if _, err := c.misty(t, c.ana, at(8, 7)); err != nil {
		t.Fatalf("CastSpell(Misty Step) error = %v", err)
	}
	w := c.windowOf(t, c.ana, playv1.ReactionKind_REACTION_KIND_EFFECT_SAVE)
	if w == nil {
		t.Fatal("the teleported wizard's player is not asked the save of the beam")
	}
	c.giveCombatantDie(t, "Pensantus", e)
	if _, err := c.rollEffectSave(t, c.ana, c.get(t, c.master), w.GetId(), effectSaveFace(3)); err != nil {
		t.Fatalf("RollEffectSave() error = %v", err)
	}
	var id string
	if err := c.h.pool.QueryRow(t.Context(), `SELECT id FROM inspiration_holds WHERE kind = $1`, string(heldEffectSave)).Scan(&id); err != nil {
		t.Fatalf("the question the zone's save kept: %v", err)
	}
	zone := c.oneZone(t, c.master, moonbeamKey)
	if _, err := c.master.combat.EndSpellZone(t.Context(), connect.NewRequest(&playv1.EndSpellZoneRequest{CampaignId: c.campaignID, EncounterId: e.GetId(), ZoneId: zone.GetId(), IdempotencyKey: newKey()})); err != nil {
		t.Fatalf("EndSpellZone() error = %v", err)
	}
	var n int
	if err := c.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM inspiration_holds WHERE id = $1`, id).Scan(&n); err != nil || n != 0 {
		t.Errorf("the hold %s is still there after the zone ended (%d, %v)", id, n, err)
	}
}

// The question of a window that closed is gone: answering it is refused, it does not settle a save nobody asks any more.
func TestAnsweringTheHeldQuestionOfAClosedWindowIsRefused(t *testing.T) {
	t.Parallel()
	h := concentrationHold(t, false, false)
	h.f.dropConcentration(t, h.f.e)
	if _, err := h.f.answerHeldRoll(t, h.f.ana, h.id, luckyAnswer(17)); err == nil {
		t.Error("the answer to the question of a closed window was accepted")
	}
}
