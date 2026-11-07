package leaktest

import (
	"bytes"
	"slices"
	"strings"
	"testing"
	"time"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1/playv1connect"
)

// The limits of a creature that a player controls (a spider of Caio's paladin, whose Web is Recharge 5-6) belong to
// the master and to that player (RN-29): the view of the creature, the line of the Recharge roll at the start of its
// turn and the reason a spent Web is refused. Ana, the other player, reads none of them, whether the roll came out
// as a face that recharges the Web, one that does not, or the Web was never spent; and the combat looks the same to
// her in the three. The same on a map with the fog of war and on one without.

const (
	teiaLabel = "Teia"
	webKey    = "monster:giant-spider#web"
)

// controllerNeedles are the words of a creature's limit: Caio and the master may read them, Ana may not.
func (w *world) controllerNeedles() {
	for _, word := range append(slices.Clone(limitedWords), webKey, "Recharge 5-6") {
		w.secrets.add(&canary{needle: word, kind: "controlled-limit", readers: names([]*person{w.caio})})
	}
}

// aTurnOfTeia is what Teia's turn starting left on the screens of the people at the table.
type aTurnOfTeia struct {
	rt       *rollsTable
	rise     map[string]int32
	entries  map[string]int
	rolls    map[string][]*playv1.CombatLogRecharge
	web      map[string]*playv1.LimitedAction
	findings []string
	cases    []string // the kinds of event Ana's stream heard
}

// newTeiaTable builds the table with Caio's spider Teia next to the skeleton. The Web is spent when face is not 0,
// and the d6 of the server comes out as face.
func newTeiaTable(t *testing.T, fog bool, face int) *rollsTable {
	t.Helper()
	rt := newRollsTableFog(t, fog, func(rt *rollsTable) {
		w := rt.world
		must(w.master.characters.GiveCreature(t.Context(), rq(&charactersv1.GiveCreatureRequest{
			CampaignId: w.campaign, CharacterId: w.toren.GetId(), MonsterKey: "monster:giant-spider", Name: teiaLabel,
		})))
	})
	w, ctx := rt.world, t.Context()
	w.controllerNeedles()
	must(w.master.combat.MoveCombatant(ctx, rq(&playv1.MoveCombatantRequest{
		CampaignId: w.campaign, EncounterId: rt.encounter.GetId(), CombatantId: rt.ids[teiaLabel], IdempotencyKey: newKey(), Col: 5, Row: 5, Forced: true,
	})))
	if face != 0 {
		if _, err := w.pool.Exec(ctx, `UPDATE combatants SET creature_spent = jsonb_build_object($2::text, 1) WHERE id = $1`, rt.ids[teiaLabel], "recharge:"+webKey); err != nil {
			t.Fatalf("spend the Web: %v", err)
		}
		w.scriptRecharge(face)
	}
	return rt
}

// teiaTurn plays the table (with the fog on or off) until Teia's turn has started, and reads what it left.
func teiaTurn(t *testing.T, fog bool, face int) aTurnOfTeia {
	t.Helper()
	rt := newTeiaTable(t, fog, face)
	w, ctx := rt.world, t.Context()
	people := []*person{w.caio, w.ana, w.master}
	before := map[string]int32{}
	logged := map[string]int{}
	for _, p := range people {
		before[p.name], logged[p.name] = p.currentEncounter(w).GetRevision(), len(w.logOfPerson(p))
	}
	ana := w.watchStream(w.ana)
	rt.toTurn(teiaLabel)
	out := aTurnOfTeia{rt: rt, rise: map[string]int32{}, entries: map[string]int{}, rolls: map[string][]*playv1.CombatLogRecharge{}, web: map[string]*playv1.LimitedAction{}}
	for _, p := range people {
		out.rise[p.name] = p.currentEncounter(w).GetRevision() - before[p.name]
		out.entries[p.name] = len(w.logOfPerson(p)) - logged[p.name]
		for _, e := range w.logOfPerson(p) {
			if e.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_RECHARGE_ROLLED {
				out.rolls[p.name] = append(out.rolls[p.name], e.GetRecharge())
			}
		}
		for _, l := range rt.combatantOf(p, teiaLabel).GetLimitedActions() {
			if l.GetActionKey() == webKey {
				out.web[p.name] = l
			}
		}
	}
	for _, p := range []*person{w.ana, w.caio} {
		for _, r := range w.reads(p) {
			for _, f := range w.inspect(p, r, nil) {
				out.findings = append(out.findings, p.name+": "+f)
			}
		}
	}
	must(w.master.combat.EndEncounter(ctx, rq(&playv1.EndEncounterRequest{CampaignId: w.campaign, EncounterId: rt.encounter.GetId(), IdempotencyKey: newKey()})))
	must(w.master.play.EndGameSession(ctx, rq(&playv1.EndGameSessionRequest{CampaignId: w.campaign, GameSessionId: w.session})))
	select {
	case <-ana.done:
	case <-time.After(30 * time.Second):
		t.Fatal("Ana's stream did not end with the session")
	}
	for _, ev := range ana.since(1) {
		if c := eventCase(ev); c != "heartbeat" {
			out.cases = append(out.cases, c)
		}
	}
	return out
}

func TestTheControllerOfACreatureReadsItsLimitsAndNoOtherPlayerDoes(t *testing.T) {
	for _, fog := range []bool{true, false} {
		t.Run(map[bool]string{true: "fog", false: "no fog"}[fog], func(t *testing.T) {
			control := teiaTurn(t, fog, 0)
			failed, recharged := teiaTurn(t, fog, 1), teiaTurn(t, fog, 5)
			for _, tc := range []struct {
				name string
				got  aTurnOfTeia
				face int32
				open bool
			}{{"a 1", failed, 1, false}, {"a 5", recharged, 5, true}} {
				// The positive controls: Caio and the master read the roll and the Web.
				for _, who := range []string{"Caio", "Mestre"} {
					r := tc.got.rolls[who]
					if len(r) != 1 || r[0].GetFace() != tc.face || r[0].GetRecharged() != tc.open {
						t.Errorf("%s: %s reads the rolls %v, want one of %d", tc.name, who, r, tc.face)
					}
					if l := tc.got.web[who]; l == nil || l.GetAvailable() != tc.open {
						t.Errorf("%s: %s reads the Web as %v, want available %v", tc.name, who, l, tc.open)
					} else if l.GetKept() {
						t.Errorf("%s: %s reads kept on a Recharge action, which ends with the combat", tc.name, who)
					}
				}
				if len(tc.got.rolls["Ana"]) != 0 || tc.got.web["Ana"] != nil {
					t.Errorf("%s: Ana reads the rolls %v and the Web %v", tc.name, tc.got.rolls["Ana"], tc.got.web["Ana"])
				}
				// Ana's combat is the one of a turn with nothing to roll.
				for _, who := range []string{"Ana", "Caio"} {
					if tc.got.rise[who] != control.rise[who] {
						t.Errorf("%s: %s's revision went up by %d, and by %d with nothing to roll", tc.name, who, tc.got.rise[who], control.rise[who])
					}
				}
				if tc.got.entries["Ana"] != control.entries["Ana"] || !slices.Equal(tc.got.cases, control.cases) {
					t.Errorf("%s: Ana's log has %d entries and her stream %v, with nothing to roll %d and %v", tc.name, tc.got.entries["Ana"], tc.got.cases, control.entries["Ana"], control.cases)
				}
			}
			for _, f := range slices.Concat(control.findings, failed.findings, recharged.findings) {
				t.Error(f)
			}
			if len(control.rolls["Caio"]) != 0 || len(control.rolls["Mestre"]) != 0 {
				t.Errorf("the control rolled: %v", control.rolls)
			}
		})
	}
}

// The controller reads why a spent Web is refused; Ana, who cannot use it, reads nothing of the limit.
func TestTheControllerOfACreatureIsToldWhyItsSpentActionIsRefused(t *testing.T) {
	rt := newTeiaTable(t, true, 1)
	w := rt.world
	rt.toTurn(teiaLabel)
	try := func(p *person) reply {
		return p.call(playv1connect.CombatServiceRollAttackProcedure, &playv1.RollAttackRequest{
			CampaignId: w.campaign, EncounterId: rt.encounter.GetId(), AttackerId: rt.ids[teiaLabel], AttackKey: webKey, TargetId: rt.ids["Esqueleto"],
			IdempotencyKey: newKey(), D20Faces: []int32{15, 15},
		})
	}
	for _, p := range []*person{w.caio, w.master} {
		if r := try(p); r.ok() || !bytes.Contains(r.body, []byte("CREATURE_ACTION_RECHARGING")) {
			t.Errorf("%s: status %d %s, want the recharging reason", p.name, r.status, r.body)
		}
	}
	if r := try(w.ana); r.ok() || strings.Contains(string(r.body), "RECHARGING") || strings.Contains(string(r.body), "NO_USES") {
		t.Errorf("Ana: status %d %s, want a refusal that says nothing of the limit", r.status, r.body)
	}
}
