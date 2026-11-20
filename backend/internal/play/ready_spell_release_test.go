package play

import (
	"slices"
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The release of a readied spell with its own targets, darts and point (#857): the answer carries what CastSpell would.

// readyForRelease readies the caster's spell on its own turn and gives the turn on, so its window can be fired.
func (a *armed) readyForRelease(t *testing.T, spell string, slot int32, target string) *playv1.Encounter {
	t.Helper()
	e := a.castersFight(t, 1)
	a.mustReady(t, a.ana, a.readySpell(t, e, "Pensantus", spell, slot, target))
	return a.mustEndTurn(t, a.master, e)
}

// castLines are the cast lines of the spell in the log.
func (a *armed) castLines(t *testing.T, e *playv1.Encounter, spell string) []*playv1.CombatLogEntry {
	t.Helper()
	var found []*playv1.CombatLogEntry
	for _, r := range a.log(t, a.master, e).GetRounds() {
		for _, en := range r.GetEntries() {
			if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_SPELL_CAST && en.GetKey() == spell {
				found = append(found, en)
			}
		}
	}
	return found
}

// castLine is the last cast line of the spell in the log, nil when there is none.
func (a *armed) castLine(t *testing.T, e *playv1.Encounter, spell string) *playv1.CombatLogEntry {
	t.Helper()
	if lines := a.castLines(t, e, spell); len(lines) > 0 {
		return lines[len(lines)-1]
	}
	return nil
}

func withTargets(targets ...*playv1.SpellTarget) func(*playv1.AnswerReactionRequest) {
	return func(r *playv1.AnswerReactionRequest) {
		useSpellAnswer(r)
		r.Targets = targets
	}
}

// The three darts of a readied Magic Missile go where the answer splits them.
func TestAReadiedMagicMissileIsReleasedWithItsDartsSplit(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.readyForRelease(t, magicMissileSpell, 1, "Capitão Goblin")
	w := a.mustFire(t, e, "Pensantus")
	res := a.mustAnswer(t, a.ana, e, w.GetId(), withTargets(darts(a, t, "Capitão Goblin", 2), darts(a, t, "Goblin", 1)))
	if !res.GetResult().GetUsed() {
		t.Fatalf("the use's result = %v", res.GetResult())
	}
	cast := a.castLine(t, e, magicMissileSpell)
	if cast == nil || len(cast.GetSpell().GetTargets()) != 2 {
		t.Fatalf("the release's line = %v, want two targets", cast)
	}
	got := map[string]int32{}
	for _, tg := range cast.GetSpell().GetTargets() {
		got[tg.GetTargetId()] += tg.GetDarts()
	}
	if got[a.id(t, "Capitão Goblin")] != 2 || got[a.id(t, "Goblin")] != 1 {
		t.Errorf("darts by target = %v, want 2 on the Capitão and 1 on the Goblin", got)
	}
}

// Darts that do not add up to the slot's are refused, and the window stays open to be answered again.
func TestAReadiedMagicMissileRefusesWrongDarts(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.readyForRelease(t, magicMissileSpell, 1, "Capitão Goblin")
	w := a.mustFire(t, e, "Pensantus")
	if _, err := a.answerReaction(t, a.ana, e, w.GetId(), withTargets(darts(a, t, "Capitão Goblin", 1), darts(a, t, "Goblin", 1))); err == nil {
		t.Fatal("two darts of three were taken")
	}
	if !a.readiedOpen(t, "Pensantus") || a.readiedStatus(t, "Pensantus") != readiedWaiting {
		t.Errorf("the refused answer left rows %q", a.readiedStatus(t, "Pensantus"))
	}
	a.mustAnswer(t, a.ana, e, w.GetId(), withTargets(darts(a, t, "Goblin", 3)))
}

// targets and creature_ids together are refused: the answer names the targets one way.
func TestAReadiedSpellRefusesTargetsAndCreatureIDsTogether(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.readyForRelease(t, magicMissileSpell, 1, "Capitão Goblin")
	w := a.mustFire(t, e, "Pensantus")
	_, err := a.answerReaction(t, a.ana, e, w.GetId(), func(r *playv1.AnswerReactionRequest) {
		useSpellAnswer(r)
		r.Targets = []*playv1.SpellTarget{darts(a, t, "Goblin", 3)}
		r.CreatureIds = []string{a.id(t, "Goblin")}
	})
	if err == nil {
		t.Error("both lists were taken")
	}
}

// While the row waits, the options of its readier list the targets of the held spell, with the darts of its slot.
func TestTheOptionsListTheTargetsOfAHeldSpellWhileItWaits(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.readyForRelease(t, magicMissileSpell, 1, "Capitão Goblin")
	got := a.mustOptions(t, a.ana, e, "Pensantus").GetSpellTargets()
	i := slices.IndexFunc(got, func(s *playv1.SpellTargets) bool {
		return s.GetSpellKey() == magicMissileSpell && slices.ContainsFunc(s.GetDarts(), func(d *playv1.DartsAtSlot) bool { return d.GetSlotLevel() == 1 && d.GetDarts() == 3 })
	})
	if i < 0 || len(got[i].GetTargets()) == 0 {
		t.Errorf("the options list no held Magic Missile at 1st level: %v", got)
	}
}

// On its readier's own turn the options list no held spell: the window opens off that turn, and a second action (Action
// Surge) could list an ordinary entry of the same spell beside it, which nothing would tell apart.
func TestTheOptionsOfTheReadierOnItsTurnListNoHeldSpell(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	a.mustReady(t, a.ana, a.readySpell(t, e, "Pensantus", magicMissileSpell, 1, "Capitão Goblin"))
	if got := a.mustOptions(t, a.ana, e, "Pensantus").GetSpellTargets(); len(got) != 0 {
		t.Errorf("the options on the readier's own turn list %v, want none", got)
	}
}

// A readied attack, a weapon's or a held spell attack cantrip's (Fire Bolt), is released as an attack: targets are for a
// spell that is cast, so an answer that carries them is refused, and the window stays open with its row waiting.
func TestTargetsOnAReadiedAttackAreRefusedAndTheWindowStaysOpen(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		label string
		ready func(t *testing.T) (*armed, *playv1.Encounter, *user)
	}{
		{"a held Fire Bolt", "Pensantus", func(t *testing.T) (*armed, *playv1.Encounter, *user) {
			t.Helper()
			a := newCasters(t)
			e := a.readyForRelease(t, fireBolt, 0, "Goblin")
			return a, e, a.ana
		}},
		{"a readied weapon attack", "Toren", func(t *testing.T) (*armed, *playv1.Encounter, *user) {
			t.Helper()
			a := newArmed(t)
			e := a.readyThenPass(t, nil)
			return a, e, a.caio
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a, e, u := tc.ready(t)
			w := a.mustFire(t, e, tc.label)
			_, err := a.answerReaction(t, u, e, w.GetId(), withTargets(darts(a, t, "Goblin", 0)))
			wantCode(t, "targets on "+tc.name, err, connect.CodeInvalidArgument)
			if !a.readiedOpen(t, tc.label) || a.readiedStatus(t, tc.label) != readiedWaiting {
				t.Errorf("the refused answer left the window open %v and rows %q", a.readiedOpen(t, tc.label), a.readiedStatus(t, tc.label))
			}
		})
	}
}

// A readied area names nobody on purpose: the creature said at Ready is not put in its place. In a combat with no map
// Sleep released with no targets is cast on nobody.
func TestAReadiedAreaReleasedWithNoTargetsIsNotAimedAtTheCreatureNamedAtReady(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.start(t, plan{
		npcs: []*playv1.Participant{{CharacterId: a.capitao.GetId()}}, npcRolls: []int{1}, reveal: []string{"Capitão Goblin"}, theatre: true,
		players: map[string]int32{"Pensantus": 20, "Toren": 15, "Brisa": 10},
	})
	a.mustReady(t, a.ana, a.readySpell(t, e, "Pensantus", sleepSpell, 1, "Capitão Goblin"))
	e = a.mustEndTurn(t, a.master, e)
	w := a.mustFire(t, e, "Pensantus")
	a.h.roller.queue(8, 8, 8, 8, 8)
	a.mustAnswer(t, a.ana, e, w.GetId(), useSpellAnswer)
	if cast := a.castLine(t, e, sleepSpell); cast == nil || len(cast.GetSpell().GetTargets()) != 0 {
		t.Errorf("the release's line = %v, want Sleep on nobody", cast)
	}
}

// A readied spell that lays a zone on a map (Web, Grease, a table spell with a zone) is refused by the answer: it
// carries no point, so the release would spend the reaction and lay nothing. CastSpell releases it instead.
func TestAReadiedZoneOnAMapIsNotReleasedByTheAnswer(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, spell string
		slot        int32
	}{
		{"Web", webSpell, 2},
		{"Grease", greaseKey, 1},
		{"a table spell with a zone", tableSphereZone, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := newTableReadier(t)
			e := a.castersFight(t, 1)
			a.mustReady(t, a.ana, a.readySpell(t, e, readyZoneCaster, tc.spell, tc.slot, ""))
			e = a.mustEndTurn(t, a.master, e)
			w := a.mustFire(t, e, readyZoneCaster)
			events := a.events(t)
			_, err := a.answerReaction(t, a.ana, e, w.GetId(), useSpellAnswer)
			wantCode(t, tc.name+" answered", err, connect.CodeInvalidArgument)
			if got, want := err.Error(), "invalid_argument: this spell lays a zone on the map, which the answer cannot place"; got != want {
				t.Fatalf("%s answered: error = %q, want %q", tc.name, got, want)
			}
			if !a.readiedOpen(t, readyZoneCaster) || a.readiedStatus(t, readyZoneCaster) != readiedWaiting || a.events(t) != events {
				t.Errorf("the refused answer left the window open %v, rows %q, events %d -> %d", a.readiedOpen(t, readyZoneCaster), a.readiedStatus(t, readyZoneCaster), events, a.events(t))
			}
		})
	}
}

// A readied Sleep on a map is placed at a point, which the answer never carries (CastSpell does): the answer is
// refused, with targets or without, and the window stays open. The targets are never dropped for the creatures in the area.
// The answer's own guard refuses it, and that guard's reason is what this test holds.
func TestAReadiedAreaOnAMapIsNotReleasedByTheAnswer(t *testing.T) {
	t.Parallel()
	for name, set := range map[string]func(*testing.T, *armed) func(*playv1.AnswerReactionRequest){
		"with targets": func(t *testing.T, a *armed) func(*playv1.AnswerReactionRequest) {
			t.Helper()
			return withTargets(darts(a, t, "Goblin", 0))
		},
		"without targets": func(*testing.T, *armed) func(*playv1.AnswerReactionRequest) { return useSpellAnswer },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			a := newCasters(t)
			e := a.readyForRelease(t, sleepSpell, 1, "")
			w := a.mustFire(t, e, "Pensantus")
			_, err := a.answerReaction(t, a.ana, e, w.GetId(), set(t, a))
			wantCode(t, "Sleep placed on a map, answered "+name, err, connect.CodeInvalidArgument)
			if got, want := err.Error(), "invalid_argument: this spell is placed at a point: set origin"; got != want {
				t.Fatalf("Sleep placed on a map, answered %s: error = %q, want the area guard's %q", name, got, want)
			}
			if !a.readiedOpen(t, "Pensantus") || a.readiedStatus(t, "Pensantus") != readiedWaiting {
				t.Errorf("the refused answer left the window open %v and rows %q", a.readiedOpen(t, "Pensantus"), a.readiedStatus(t, "Pensantus"))
			}
		})
	}
}
