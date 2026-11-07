package leaktest

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1/playv1connect"
)

// A Knight's Parry (SRD 5.1, RN-20, RN-31) is decided by the server, with no window: a hit that the bonus does not turn
// goes on exactly as against a creature with no Parry (no wait, no window, no reaction status), and a hit it turns is a
// miss that the players read with one line ("aparou o ataque.") and no number. When the master asks to be asked
// (SetMonsterReactionMode), a melee hit on the creature waits for him: a player reads "Esperando o mestre" and,
// after the answer, an ordinary miss. The mode is the master's alone. The master reads everything (the positive controls).

// parryWords are what no player reads of a Parry, before or after it is used: "aparou" is the players' line of an
// automatic Parry that turned a hit, and has its own test.
var parryWords = []string{"Parry", "parry", "PARRY", "Aparar", "aparou", "monster:knight", "parry_ask", "parryAsk"}

// parryFight puts a visible Knight next to Toren, on Toren's turn, and arms the needles.
func (w *world) parryFight() *playv1.Combatant {
	w.t.Helper()
	knight := w.reactionFightOf("monster:knight", w.secrets.public("knight-name"))
	for _, word := range parryWords {
		if word != "aparou" { // the players' line of an automatic Parry that turned a hit: the tests that expect none arm it
			w.secrets.add(&canary{needle: word, kind: "parry"})
		}
	}
	return knight
}

// parryAskFight is parryFight with the master having asked to answer the Knight's Parry.
func (w *world) parryAskFight() *playv1.Combatant {
	w.t.Helper()
	knight := w.parryFight()
	w.secrets.add(&canary{needle: "aparou", kind: "parry"}) // in "Perguntar" the players read no line of the Parry
	w.setParryAsk(knight, true)
	return knight
}

// setParryAsk is the master's SetMonsterReactionMode on the creature.
func (w *world) setParryAsk(c *playv1.Combatant, ask bool) {
	w.t.Helper()
	must(w.master.combat.SetMonsterReactionMode(w.t.Context(), rq(&playv1.SetMonsterReactionModeRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: c.GetId(), IdempotencyKey: newKey(), Ask: ask,
	})))
}

// daggerHit is Toren's dagger attack on the target as Caio: +4 to hit, so a d20 of 14 makes 18, the Knight's armor class.
func (w *world) daggerHit(target *playv1.Combatant, face int32) reply {
	w.t.Helper()
	return w.caio.call(playv1connect.CombatServiceRollAttackProcedure, &playv1.RollAttackRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), AttackerId: w.combatant(w.toren).GetId(), AttackKey: "equipment:dagger", TargetId: target.GetId(),
		IdempotencyKey: newKey(), Roll: &playv1.RollAttackRequest_D20Face{D20Face: face},
	})
}

func TestAParrysWindowIsTheMastersAloneAndItsUseReadsAsAMiss(t *testing.T) {
	w := newWorld(t)
	knight := w.parryAskFight()
	hit := w.daggerHit(knight, 14)
	if !hit.ok() {
		t.Fatalf("Toren's attack: status %d %s", hit.status, hit.body)
	}
	window := windowOfKind(w.master, w, playv1.ReactionKind_REACTION_KIND_PARRY)
	if window == nil || window.GetParry().GetAcBonus() != 2 || window.GetTrigger().GetTargetArmorClass() != 18 {
		t.Fatalf("the master reads %v, want the Knight's Parry (+2) against armor class 18", window)
	}
	w.secrets.add(&canary{needle: window.GetId(), kind: "reaction-window-id"})

	check := func(when string) {
		t.Helper()
		for _, p := range []*person{w.ana, w.caio} {
			rs := w.reads(p)
			if p == w.caio {
				rs = append(rs, hit)
			}
			for _, r := range rs {
				for _, f := range w.inspect(p, r, nil) {
					t.Errorf("%s %s: %s", when, p.name, f)
				}
			}
		}
	}
	check("while the Knight may parry")
	for _, p := range []*person{w.ana, w.caio} {
		if got := windowOfKind(p, w, playv1.ReactionKind_REACTION_KIND_PARRY); got != nil {
			t.Errorf("%s reads the Parry window %v", p.name, got)
		}
		if wait := p.currentEncounter(w).GetReactionWait().GetTitlePt(); wait != "Esperando o mestre" {
			t.Errorf("%s reads the wait %q, want \"Esperando o mestre\"", p.name, wait)
		}
	}
	// The positive control: the master's reads hold the window and the word.
	if !anyBody(w.reads(w.master), window.GetId()) || !anyBody(w.reads(w.master), "REACTION_KIND_PARRY") {
		t.Error("the master's reads do not hold the window: the needles are unreachable")
	}
	// A player cannot answer it, and asking cannot tell a real window from an invented one.
	asked := w.caio.call(playv1connect.CombatServiceAnswerReactionProcedure, &playv1.AnswerReactionRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), WindowId: window.GetId(), Answer: playv1.ReactionChoice_REACTION_CHOICE_USE, IdempotencyKey: newKey(),
	})
	fake := w.caio.call(playv1connect.CombatServiceAnswerReactionProcedure, &playv1.AnswerReactionRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), WindowId: newKey(), Answer: playv1.ReactionChoice_REACTION_CHOICE_USE, IdempotencyKey: newKey(),
	})
	if asked.ok() || fake.ok() || asked.status != fake.status || !bytes.Equal(asked.body, fake.body) {
		t.Errorf("a real window is refused with %d %s, an invented one with %d %s: they must read the same", asked.status, asked.body, fake.status, fake.body)
	}

	used := must(w.master.combat.AnswerReaction(t.Context(), rq(&playv1.AnswerReactionRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), WindowId: window.GetId(), Answer: playv1.ReactionChoice_REACTION_CHOICE_USE, IdempotencyKey: newKey(),
	})))
	if !used.GetResult().GetParry().GetStopped() {
		t.Fatalf("the answer = %v, want the hit stopped", used.GetResult())
	}
	check("after the Knight parried")

	// The positive controls of the log: the master reads the line and the reason; Caio, who attacked, reads a plain
	// miss (Ana stands out of the Knight's sight on the fog map and reads no line of it at all).
	masterLine, masterAttack := "", false
	for _, e := range w.logOfPerson(w.master) {
		if e.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_REACTION_WINDOW {
			masterLine = e.GetReactionTextPt()
		}
		masterAttack = masterAttack || (e.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_ATTACK && e.GetStoppedByReaction())
	}
	if !strings.Contains(masterLine, "aparou (+2 CA)") || !masterAttack {
		t.Errorf("the master's log: line %q, stopped attack %v; want \"aparou (+2 CA)\" and the attack stopped by a reaction", masterLine, masterAttack)
	}
	miss := false
	for _, e := range w.logOfPerson(w.caio) {
		miss = miss || (e.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_ATTACK && e.GetActorId() == w.combatant(w.toren).GetId() && e.GetOutcome() == playv1.AttackOutcome_ATTACK_OUTCOME_MISS && !e.GetStoppedByReaction())
	}
	if !miss {
		t.Error("Caio's log has no plain miss of Toren's attack: the control is missing")
	}
}

// The Knight's window (in "Perguntar") is heard by the master alone: the players' streams get the combat's ordinary change events
// and neither of the window's two, with Parry used or passed (the master's stream gets both: the control).
func TestAParrysWindowIsHeardByTheMasterAlone(t *testing.T) {
	w := newWorld(t)
	knight := w.parryAskFight()
	got := w.hiddenOnlyEvents(func() {
		must(w.caio.combat.RollAttack(t.Context(), rq(&playv1.RollAttackRequest{
			CampaignId: w.campaign, EncounterId: w.encounter.GetId(), AttackerId: w.combatant(w.toren).GetId(), AttackKey: "equipment:dagger", TargetId: knight.GetId(),
			IdempotencyKey: newKey(), Roll: &playv1.RollAttackRequest_D20Face{D20Face: 14},
		})))
		enc := must(w.master.combat.GetEncounter(t.Context(), rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
		must(w.master.combat.AnswerReaction(t.Context(), rq(&playv1.AnswerReactionRequest{
			CampaignId: w.campaign, EncounterId: w.encounter.GetId(), WindowId: enc.GetReactionWindows()[0].GetId(),
			Answer: playv1.ReactionChoice_REACTION_CHOICE_USE, IdempotencyKey: newKey(),
		})))
	})
	count := func(name string) (opened, closed int) {
		for _, ev := range got[name] {
			switch {
			case strings.HasPrefix(ev, "reaction_window_opened"):
				opened++
			case strings.HasPrefix(ev, "reaction_window_closed"):
				closed++
			}
		}
		return opened, closed
	}
	if o, c := count(w.master.name); o != 1 || c != 1 {
		t.Errorf("the master heard %d openings and %d closings, want one of each (%v)", o, c, got[w.master.name])
	}
	for _, p := range []*person{w.ana, w.caio} {
		if o, c := count(p.name); o != 0 || c != 0 {
			t.Errorf("%s heard %d openings and %d closings of the Knight's window", p.name, o, c)
		}
	}
}

// anyBody says one of the replies holds the word.
func anyBody(rs []reply, word string) bool {
	for _, r := range rs {
		if contains(r.body, word) {
			return true
		}
	}
	return false
}

// ids are the UUIDs of an answer, which differ between two combats.
var ids = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// attackShape is what a player's answer to an attack says of the roll and the hit: the roll and the damage it opened
// without the ids that differ between combats.
func attackShape(t *testing.T, r reply) (string, *playv1.RollAttackResponse) {
	t.Helper()
	res := &playv1.RollAttackResponse{}
	if err := protojson.Unmarshal(r.body, res); err != nil {
		t.Fatalf("read the attack answer: %v", err)
	}
	shape := &playv1.RollAttackResponse{Roll: res.GetRoll(), PendingDamage: res.GetPendingDamage()}
	return ids.ReplaceAllString(protojson.Format(shape), "ID"), res
}

// A hit that the Knight's Parry does not turn reads, to the player, exactly as a hit on a creature with no Parry: the
// same answer, no wait, no window, no reaction status, no line and no event of a window on the stream. The master
// reads the mode (the positive control) and the player never does.
func TestAnAutomaticParryThatDoesNotTriggerLooksLikeNoParry(t *testing.T) {
	// 19 + 4 = 23 against the Knight's 18 (and 20 with Parry) and against the Bandit's 12.
	const face = 19
	shapes := map[string]string{}
	for name, creature := range map[string]string{"knight": "monster:knight", "bandit": "monster:bandit"} {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t)
			target := w.reactionFightOf(creature, w.secrets.public("duel-"+name+"-name"))
			for _, word := range parryWords {
				w.secrets.add(&canary{needle: word, kind: "parry"})
			}
			// What the players read is read inside the act: the stream's check ends the session.
			heard := w.hiddenOnlyEvents(func() {
				hit := w.daggerHit(target, face)
				if !hit.ok() {
					t.Fatalf("Toren's attack: status %d %s", hit.status, hit.body)
				}
				shape, res := attackShape(t, hit)
				shapes[name] = shape
				if res.GetPendingDamage().GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_AWAITING_ROLL || res.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_HIT {
					t.Errorf("Caio's answer = %v / %v, want a hit whose damage waits for its roll", res.GetRoll().GetOutcome(), res.GetPendingDamage().GetStatus())
				}
				if res.GetEncounter().GetReactionWait() != nil || len(res.GetEncounter().GetReactionWindows()) != 0 {
					t.Errorf("Caio's answer holds the wait %v and windows %v", res.GetEncounter().GetReactionWait(), res.GetEncounter().GetReactionWindows())
				}
				for _, p := range []*person{w.ana, w.caio} {
					enc := p.currentEncounter(w)
					if enc.GetReactionWait() != nil || len(enc.GetReactionWindows()) != 0 || len(enc.GetReactionPrompts()) != 0 {
						t.Errorf("%s reads the wait %v windows %v prompts %v after the hit", p.name, enc.GetReactionWait(), enc.GetReactionWindows(), enc.GetReactionPrompts())
					}
					rs := w.reads(p)
					if p == w.caio {
						rs = append(rs, hit)
					}
					for _, r := range rs {
						for _, f := range w.inspect(p, r, nil) {
							t.Errorf("%s: %s", p.name, f)
						}
					}
					for _, e := range w.logOfPerson(p) {
						if e.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_REACTION_WINDOW || e.GetReactionTextPt() != "" {
							t.Errorf("%s reads the reaction line %q", p.name, e.GetReactionTextPt())
						}
					}
				}
				if windowOfKind(w.master, w, playv1.ReactionKind_REACTION_KIND_PARRY) != nil {
					t.Error("the master reads a Parry window: none should open")
				}
				// The positive control: the master reads the mode of the Knight and of nobody else.
				var mode *bool
				for _, c := range w.master.currentEncounter(w).GetCombatants() {
					if c.GetId() == target.GetId() {
						mode = c.ParryAsk
					}
				}
				if (creature == "monster:knight") != (mode != nil) || (mode != nil && *mode) {
					t.Errorf("the master reads parry_ask %v for %s, want false for the Knight and none for the Bandit", mode, creature)
				}
			})
			for _, p := range []*person{w.master, w.ana, w.caio} {
				for _, ev := range heard[p.name] {
					if strings.HasPrefix(ev, "reaction_window") {
						t.Errorf("%s heard %s", p.name, ev)
					}
				}
			}
		})
	}
	// The two answers are the same but for the names and the numbers that are public.
	k, b := ids.ReplaceAllString(shapes["knight"], "ID"), ids.ReplaceAllString(shapes["bandit"], "ID")
	if k != b {
		t.Errorf("the answer on the Knight is %s, on the Bandit %s; want the same", k, b)
	}
}

// A hit that the Knight's Parry turns is a miss: the answer to the player is any miss; the players read the one line,
// the master reads the numbers, and a player never reads the mode.
func TestAnAutomaticParryThatTurnsAHitReadsAsAMissWithOneLine(t *testing.T) {
	w := newWorld(t)
	knight := w.parryFight()
	hit := w.daggerHit(knight, 14) // 18 against 18, and 20 with Parry
	if !hit.ok() {
		t.Fatalf("Toren's attack: status %d %s", hit.status, hit.body)
	}
	_, res := attackShape(t, hit)
	if res.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_MISS || res.GetPendingDamage() != nil || res.GetRoll().TargetArmorClass != nil {
		t.Errorf("Caio's answer = %v / %v, want a plain miss", res.GetRoll(), res.GetPendingDamage())
	}
	for _, p := range []*person{w.ana, w.caio} {
		if got := windowOfKind(p, w, playv1.ReactionKind_REACTION_KIND_PARRY); got != nil || p.currentEncounter(w).GetReactionWait() != nil {
			t.Errorf("%s reads the Parry window %v or a wait", p.name, got)
		}
	}
	// The needles of the Parry stay armed but for the players' line itself, and the numbers of the master's.
	for _, word := range []string{"+2 CA", "contra CA"} {
		w.secrets.add(&canary{needle: word, kind: "parry-turned"})
	}
	for _, p := range []*person{w.ana, w.caio} {
		rs := w.reads(p)
		if p == w.caio {
			rs = append(rs, hit)
		}
		for _, r := range rs {
			for _, f := range w.inspect(p, r, nil) {
				t.Errorf("%s: %s", p.name, f)
			}
		}
	}
	masterLine := ""
	for _, e := range w.logOfPerson(w.master) {
		if e.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_REACTION_WINDOW {
			masterLine = e.GetReactionTextPt()
		}
	}
	if !strings.Contains(masterLine, "aparou (+2 CA): 18 contra CA 20, o ataque errou.") {
		t.Errorf("the master's line = %q, want the Parry with the numbers", masterLine)
	}
	lines := func(p *person) (out []string) {
		for _, e := range w.logOfPerson(p) {
			if e.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_REACTION_WINDOW || e.GetReactionTextPt() != "" {
				out = append(out, e.GetReactionTextPt())
			}
		}
		return out
	}
	// Caio's character sees the Knight (the control); Ana's does not, on the fog map, and reads no line of it at all.
	if got := lines(w.caio); len(got) != 1 || got[0] != knight.GetLabel()+" aparou o ataque." {
		t.Errorf("Caio's reaction lines = %q, want the one line %q with no number", got, knight.GetLabel()+" aparou o ataque.")
	}
	if got := lines(w.ana); len(got) != 0 {
		t.Errorf("Ana reads the reaction lines %q of a Knight she does not see", got)
	}
	if !anyBody(w.reads(w.master), "parryAsk") {
		t.Error("the master's reads do not hold parry_ask: the needle is unreachable")
	}
}

// Only the master sets the mode, and asking cannot tell a Knight from a creature with no Parry to a player.
func TestSetMonsterReactionModeIsTheMastersAlone(t *testing.T) {
	w := newWorld(t)
	knight := w.parryFight()
	asked := func(p *person, id string) reply {
		return p.call(playv1connect.CombatServiceSetMonsterReactionModeProcedure, &playv1.SetMonsterReactionModeRequest{
			CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: id, IdempotencyKey: newKey(), Ask: true,
		})
	}
	known, fake := asked(w.caio, knight.GetId()), asked(w.caio, newKey())
	if known.ok() || fake.ok() || known.status != fake.status || !bytes.Equal(known.body, fake.body) {
		t.Errorf("a player asking for the Knight is refused with %d %s, for a stranger with %d %s: they must read the same", known.status, known.body, fake.status, fake.body)
	}
	if got := asked(w.master, knight.GetId()); !got.ok() {
		t.Fatalf("the master's SetMonsterReactionMode: status %d %s", got.status, got.body)
	}
	for _, p := range []*person{w.ana, w.caio} {
		for _, r := range w.reads(p) {
			for _, f := range w.inspect(p, r, nil) {
				t.Errorf("%s: %s", p.name, f)
			}
		}
	}
	if !anyBody(w.reads(w.master), "parryAsk") {
		t.Error("the master's reads do not hold parry_ask after asking: the needle is unreachable")
	}
}
