package play

import (
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// Dismiss and Release of a steed: a dismissal keeps the bond, a release ends it.
// In a combat the owner's player spends the action of its turn.
// Out of a combat it is free.

func (a *armed) dismissSteedAs(t *testing.T, u *user, id string, how charactersv1.CreatureDismissal) error {
	t.Helper()
	_, err := u.characters.DismissCreature(t.Context(), connect.NewRequest(&charactersv1.DismissCreatureRequest{CampaignId: a.campaignID, CreatureId: id, Dismissal: how}))
	return err
}

func (a *armed) steedCreatureID(t *testing.T) string {
	t.Helper()
	var id string
	if err := a.h.pool.QueryRow(t.Context(), `SELECT id::text FROM character_creatures WHERE source = 'find_steed' ORDER BY created_at DESC LIMIT 1`).Scan(&id); err != nil {
		t.Fatalf("read the steed's creature: %v", err)
	}
	return id
}

func (a *armed) steedBondOf(t *testing.T, id string) (bonded bool, reason string) {
	t.Helper()
	var r *string
	if err := a.h.pool.QueryRow(t.Context(), `SELECT bonded, dismissed_reason FROM character_creatures WHERE id = $1`, id).Scan(&bonded, &r); err != nil {
		t.Fatalf("read the steed's bond: %v", err)
	}
	return bonded, ptrOr(r, "")
}

// TestFindSteedDismissKeepsTheBondAndReleaseEndsIt: out of a combat both are free; a dismissed steed comes back with the next cast, and a
// released one is gone with its bond, so the next cast may make another form.
func TestFindSteedDismissKeepsTheBondAndReleaseEndsIt(t *testing.T) {
	t.Parallel()
	a := newRiders(t)
	first := a.mustCastSteed(t, warhorse, "celestial", "Trovão")
	if err := a.dismissSteedAs(t, a.ana, first.GetId(), charactersv1.CreatureDismissal_CREATURE_DISMISSAL_DISMISS); err != nil {
		t.Fatalf("dismiss error = %v", err)
	}
	if bonded, reason := a.steedBondOf(t, first.GetId()); !bonded || reason != "dismissed" {
		t.Errorf("a dismissed steed: bonded = %v, reason = %q, want the bond kept and the reason dismissed", bonded, reason)
	}
	if again := a.mustCastSteed(t, warhorse, "celestial", "Trovão"); again.GetId() != first.GetId() {
		t.Errorf("the cast after a dismissal made %s, want the same steed %s", again.GetId(), first.GetId())
	}
	if err := a.dismissSteedAs(t, a.ana, first.GetId(), charactersv1.CreatureDismissal_CREATURE_DISMISSAL_RELEASE); err != nil {
		t.Fatalf("release error = %v", err)
	}
	if bonded, reason := a.steedBondOf(t, first.GetId()); bonded || reason != "owner" {
		t.Errorf("a released steed: bonded = %v, reason = %q, want the bond ended and the reason owner", bonded, reason)
	}
	if other := a.mustCastSteed(t, elk, "fey", "Galho"); other.GetId() == first.GetId() || other.GetMonsterKey() != elk || !other.GetBonded() {
		t.Errorf("the cast after a release = %v, want a new bonded elk", other)
	}
}

// TestFindSteedReleaseIsTheOwnersAndTheMastersOnly: another player does not find the steed, and releasing a released steed again changes nothing.
func TestFindSteedReleaseIsTheOwnersAndTheMastersOnly(t *testing.T) {
	t.Parallel()
	a := newRiders(t)
	steed := a.mustCastSteed(t, warhorse, "celestial", "Trovão")
	release := charactersv1.CreatureDismissal_CREATURE_DISMISSAL_RELEASE
	if err := a.dismissSteedAs(t, a.caio, steed.GetId(), release); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("a release by another player = %v, want not_found", err)
	}
	if err := a.dismissSteedAs(t, a.master, steed.GetId(), release); err != nil {
		t.Fatalf("the master's release error = %v", err)
	}
	if err := a.dismissSteedAs(t, a.ana, steed.GetId(), release); err != nil {
		t.Errorf("a second release of the same steed = %v, want nothing changed", err)
	}
	if bonded, reason := a.steedBondOf(t, steed.GetId()); bonded || reason != "master" {
		t.Errorf("the released steed: bonded = %v, reason = %q, want the bond ended by the master", bonded, reason)
	}
}

// TestFindSteedDismissInACombatSpendsTheOwnersAction: on the owner's turn the dismissal costs the action and puts the rider on foot.
// Off turn, or with the action spent, it is refused with nothing changed.
// The master does it free.
func TestFindSteedDismissInACombatSpendsTheOwnersAction(t *testing.T) {
	t.Parallel()
	a, e := riderInFight(t)
	id := a.steedCreatureID(t)
	dismiss := charactersv1.CreatureDismissal_CREATURE_DISMISSAL_DISMISS
	if byLabel(t, e, "Aurélio").GetActionUsed() {
		t.Fatal("Aurélio starts with his action spent")
	}
	if err := a.dismissSteedAs(t, a.ana, id, dismiss); err != nil {
		t.Fatalf("dismiss on the turn error = %v", err)
	}
	cur := a.get(t, a.master)
	if !byLabel(t, cur, "Aurélio").GetActionUsed() {
		t.Error("the dismissal did not spend Aurélio's action")
	}
	if got := a.steedRowOf(t, "Aurélio"); got.MountedOn != "" {
		t.Errorf("Aurélio rides %q after the dismissal, want on foot", got.MountedOn)
	}
	if bonded, _ := a.steedBondOf(t, id); !bonded {
		t.Error("the dismissal ended the bond")
	}
}

// steedUntouched asserts a refused dismissal changed nothing: the rider still rides, the steed combatant is there, the action is as it was.
func (a *armed) steedUntouched(t *testing.T, what, id string, events int, actionUsed bool) {
	t.Helper()
	if bonded, reason := a.steedBondOf(t, id); !bonded || reason != "" {
		t.Errorf("%s: the steed's creature is bonded = %v, reason %q, want live", what, bonded, reason)
	}
	if got := a.steedRowOf(t, "Aurélio"); got.MountedOn != a.id(t, "Trovão") {
		t.Errorf("%s: Aurélio rides %q, want the steed", what, got.MountedOn)
	}
	var present int
	if err := a.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM combatants WHERE label = 'Trovão' AND NOT dismissed`).Scan(&present); err != nil || present != 1 {
		t.Errorf("%s: the steed's combatants = %d (%v), want 1", what, present, err)
	}
	if got := byLabel(t, a.get(t, a.master), "Aurélio").GetActionUsed(); got != actionUsed || a.events(t) != events {
		t.Errorf("%s: action used %v (was %v), events %d (was %d)", what, got, actionUsed, a.events(t), events)
	}
}

func TestFindSteedDismissOffTurnIsRefusedAndTheMasterDismissesFree(t *testing.T) {
	t.Parallel()
	a, e := riderInFight(t)
	id := a.steedCreatureID(t)
	dismiss := charactersv1.CreatureDismissal_CREATURE_DISMISSAL_DISMISS
	a.passTo(t, e, "Toren")
	events := a.events(t)
	wantBlockedBy(t, "the dismissal off turn", a.dismissSteedAs(t, a.ana, id, dismiss), blockedNotYourTurn)
	a.steedUntouched(t, "off turn", id, events, false)
	if err := a.dismissSteedAs(t, a.master, id, dismiss); err != nil {
		t.Errorf("the master's dismissal off turn = %v, want free", err)
	}
	if byLabel(t, a.get(t, a.master), "Aurélio").GetActionUsed() {
		t.Error("the master's dismissal spent Aurélio's action")
	}
}

// TestFindSteedDismissWithTheActionSpentIsRefused: on the owner's turn with the action already spent, the dismissal is refused with ACTION_USED.
func TestFindSteedDismissWithTheActionSpentIsRefused(t *testing.T) {
	t.Parallel()
	a, e := riderInFight(t)
	id := a.steedCreatureID(t)
	if _, err := a.action(t, a.ana, e, "Aurélio", "standard:dodge"); err != nil {
		t.Fatalf("Dodge error = %v", err)
	}
	events := a.events(t)
	wantBlockedBy(t, "the dismissal with the action spent", a.dismissSteedAs(t, a.ana, id, charactersv1.CreatureDismissal_CREATURE_DISMISSAL_DISMISS), blockedActionUsed)
	a.steedUntouched(t, "action spent", id, events, true)
}

// TestFindSteedNothingInACombatCanUndoItsCastingAway pins why the creatures' delete (DeleteCreatures, the undo of a casting) never reaches a bonded
// steed: the spell takes 10 minutes, so a cast in a combat is refused, and the cast out of one is a creature_summoned event that
// chainCreatures skips, so a combat has no undo that points at it. The steed is still there, bonded, after an undo is tried.
func TestFindSteedNothingInACombatCanUndoItsCastingAway(t *testing.T) {
	t.Parallel()
	a, e := steedFight(t, false)
	id := a.steedCreatureID(t)
	if got := a.log(t, a.master, e).GetUndoableEventId(); got != "" {
		t.Errorf("the log offers to undo %q at the start of a combat, want nothing: the steed's casting is not undoable there", got)
	}
	if err := a.undo(t, a.master, e, ""); err == nil {
		t.Error("UndoLastAction() = nil, want it refused with nothing to undo")
	}
	if bonded, reason := a.steedBondOf(t, id); !bonded || reason != "" {
		t.Errorf("the steed after the undo: bonded %v, reason %q, want it bonded and live", bonded, reason)
	}
}

// steedLine is the log line of the steed's dismissal or release the user reads, or nil.
func (a *armed) steedLine(t *testing.T, u *user, e *playv1.Encounter, kind playv1.CombatLogKind) *playv1.CombatLogEntry {
	t.Helper()
	var found *playv1.CombatLogEntry
	for _, round := range a.log(t, u, e).GetRounds() {
		for _, entry := range round.GetEntries() {
			if entry.GetKind() == kind {
				found = entry
			}
		}
	}
	return found
}

// TestFindSteedDismissAndReleaseInACombatAreLines: sending the steed away in a running combat writes one line.
// The owner's character is the actor and the steed is the one sent away.
// The master and every player who sees the steed read it.
func TestFindSteedDismissAndReleaseInACombatAreLines(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		how   charactersv1.CreatureDismissal
		kind  playv1.CombatLogKind
		other playv1.CombatLogKind
	}{
		{"dismissed", charactersv1.CreatureDismissal_CREATURE_DISMISSAL_DISMISS, playv1.CombatLogKind_COMBAT_LOG_KIND_CREATURE_DISMISSED, playv1.CombatLogKind_COMBAT_LOG_KIND_CREATURE_RELEASED},
		{"released", charactersv1.CreatureDismissal_CREATURE_DISMISSAL_RELEASE, playv1.CombatLogKind_COMBAT_LOG_KIND_CREATURE_RELEASED, playv1.CombatLogKind_COMBAT_LOG_KIND_CREATURE_DISMISSED},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a, e := riderInFight(t)
			if line := a.steedLine(t, a.master, e, tc.kind); line != nil {
				t.Fatalf("the log has a %v line before the steed is sent away", tc.kind)
			}
			if err := a.dismissSteedAs(t, a.ana, a.steedCreatureID(t), tc.how); err != nil {
				t.Fatalf("send the steed away error = %v", err)
			}
			for who, u := range map[string]*user{"the master": a.master, "the owner": a.ana, "another player who sees the steed": a.caio} {
				line := a.steedLine(t, u, e, tc.kind)
				if line == nil {
					t.Errorf("%s reads no %v line", who, tc.kind)
					continue
				}
				if line.GetActorLabel() != "Aurélio" || line.GetTargetLabel() != "Trovão" {
					t.Errorf("%s reads %q and %q, want the owner's character and the steed", who, line.GetActorLabel(), line.GetTargetLabel())
				}
				if line.GetByMaster() {
					t.Errorf("%s reads the owner's own dismissal as the master's act", who)
				}
				if wrong := a.steedLine(t, u, e, tc.other); wrong != nil {
					t.Errorf("%s reads a %v line too", who, tc.other)
				}
			}
		})
	}
}

// anySteedLine says whether anybody of the table reads a steed line, of either kind.
func (a *armed) anySteedLine(t *testing.T, e *playv1.Encounter) map[string]bool {
	t.Helper()
	read := map[string]bool{}
	for who, u := range map[string]*user{"the master": a.master, "the owner": a.ana, "another player": a.caio} {
		for _, kind := range []playv1.CombatLogKind{playv1.CombatLogKind_COMBAT_LOG_KIND_CREATURE_DISMISSED, playv1.CombatLogKind_COMBAT_LOG_KIND_CREATURE_RELEASED} {
			if a.steedLine(t, u, e, kind) != nil {
				read[who] = true
			}
		}
	}
	return read
}

// TestFindSteedLineFollowsTheVisibilityOfWhoIsInIt: a hidden steed or owner makes the line the master's alone.
// This holds when the steed is sent away while one of them is hidden.
// The API never hides a player's creature, so the test hides it in the table.
func TestFindSteedLineFollowsTheVisibilityOfWhoIsInIt(t *testing.T) {
	t.Parallel()
	for _, hidden := range []string{"Trovão", "Aurélio"} {
		t.Run(hidden, func(t *testing.T) {
			t.Parallel()
			a, e := riderInFight(t)
			if _, err := a.h.pool.Exec(t.Context(), `UPDATE combatants SET hidden = true WHERE label = $1`, hidden); err != nil {
				t.Fatalf("hide %s: %v", hidden, err)
			}
			if err := a.dismissSteedAs(t, a.master, a.steedCreatureID(t), charactersv1.CreatureDismissal_CREATURE_DISMISSAL_DISMISS); err != nil {
				t.Fatalf("dismiss error = %v", err)
			}
			if got := a.anySteedLine(t, e); !got["the master"] || got["the owner"] || got["another player"] {
				t.Errorf("who reads the line of a dismissal with %s hidden = %v, want the master alone", hidden, got)
			}
		})
	}
}

// TestFindSteedSendingAwayWritesNoLineWhenItIsNotASteedOrNotARunningCombat: some cases leave no line.
// A creature that is not a bonded steed leaves none.
// A combat in setup leaves none, and so does a combat that is over.
// A steed removed by the master or fallen at 0 hit points leaves none either.
func TestFindSteedSendingAwayWritesNoLineWhenItIsNotASteedOrNotARunningCombat(t *testing.T) {
	t.Parallel()
	dismiss := charactersv1.CreatureDismissal_CREATURE_DISMISSAL_DISMISS
	cases := []struct {
		name string
		run  func(t *testing.T, a *armed, e *playv1.Encounter)
	}{
		{"a creature that is not a steed", func(t *testing.T, a *armed, _ *playv1.Encounter) {
			t.Helper()
			id := a.steedCreatureID(t)
			if _, err := a.h.pool.Exec(t.Context(), `UPDATE character_creatures SET source = 'master', bonded = false WHERE id = $1`, id); err != nil {
				t.Fatalf("make it a creature of the master: %v", err)
			}
			if err := a.dismissSteedAs(t, a.master, id, dismiss); err != nil {
				t.Fatalf("dismiss error = %v", err)
			}
		}},
		{"a combat in setup", func(t *testing.T, a *armed, _ *playv1.Encounter) {
			t.Helper()
			if _, err := a.h.pool.Exec(t.Context(), `UPDATE encounters SET status = 'setup'`); err != nil {
				t.Fatalf("put the combat in setup: %v", err)
			}
			if err := a.dismissSteedAs(t, a.master, a.steedCreatureID(t), dismiss); err != nil {
				t.Fatalf("dismiss error = %v", err)
			}
		}},
		{"a combat that is over", func(t *testing.T, a *armed, e *playv1.Encounter) {
			t.Helper()
			if _, err := a.master.combat.EndEncounter(t.Context(), connect.NewRequest(&playv1.EndEncounterRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey()})); err != nil {
				t.Fatalf("end the combat: %v", err)
			}
			if err := a.dismissSteedAs(t, a.master, a.steedCreatureID(t), dismiss); err != nil {
				t.Fatalf("dismiss error = %v", err)
			}
		}},
		{"a steed removed by the master", func(t *testing.T, a *armed, e *playv1.Encounter) {
			t.Helper()
			if _, err := a.master.combat.RemoveCombatant(t.Context(), connect.NewRequest(&playv1.RemoveCombatantRequest{
				CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Trovão"), IdempotencyKey: newKey(),
			})); err != nil {
				t.Fatalf("RemoveCombatant() error = %v", err)
			}
		}},
		{"a steed at 0 hit points", func(t *testing.T, a *armed, e *playv1.Encounter) {
			t.Helper()
			if _, err := a.adjustHP(t, e, "Trovão", func(r *playv1.AdjustCombatantHitPointsRequest) {
				r.Change = &playv1.AdjustCombatantHitPointsRequest_HitPoints{HitPoints: 0}
			}); err != nil {
				t.Fatalf("take the steed to 0 hit points: %v", err)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a, e := steedFight(t, false)
			tc.run(t, a, e)
			if got := a.anySteedLine(t, e); len(got) != 0 {
				t.Errorf("%s: who reads a steed line = %v, want nobody", tc.name, got)
			}
		})
	}
}

// TestFindSteedDismissByTheMasterIsMarkedAsTheMastersAct: the master sends the owner's steed away.
// Every reader of the line is told it was the master's act.
func TestFindSteedDismissByTheMasterIsMarkedAsTheMastersAct(t *testing.T) {
	t.Parallel()
	a, e := riderInFight(t)
	if err := a.dismissSteedAs(t, a.master, a.steedCreatureID(t), charactersv1.CreatureDismissal_CREATURE_DISMISSAL_DISMISS); err != nil {
		t.Fatalf("dismiss error = %v", err)
	}
	for who, u := range map[string]*user{"the master": a.master, "the owner": a.ana, "another player": a.caio} {
		line := a.steedLine(t, u, e, playv1.CombatLogKind_COMBAT_LOG_KIND_CREATURE_DISMISSED)
		if line == nil || !line.GetByMaster() {
			t.Errorf("%s reads %v, want the line marked as the master's act", who, line)
		}
	}
}
