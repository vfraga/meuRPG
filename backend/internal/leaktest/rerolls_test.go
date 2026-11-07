package leaktest

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1/playv1connect"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The reroll question of a d20 and the dice a reroll rule rolls again (RN-10, RN-20, #573), with the d20 rerolls
// switched on (the stack of the other tests runs with them off, as cmd/api does). The hold of a roll that waits for the
// reroll, with the rule it is held for (its limit, its resource, its source) and the d20 that showed the low face, is
// for the roller's player and for the master: another player reads nothing of it, not the id, not the rule, not the
// face, in the combat, in the list of the questions, in the outside offer, in the answer to a try at answering it or in
// the live stream. Once the roll is settled the rerolled faces are in the log for everyone, and a Divine Smite's extra
// die is rolled again against an undead and against any other target alike, so the lines a player reads do not say what
// the target is.
//
// The world is the attack rolls': Toren, the paladin of Caio, holds the table feat that gives the rule.

const (
	// rerollPool is the resource the feat's rule spends; rerollAtMost the limit of its d20 rule; rerollHeldFace the d20 the
	// attack shows when it is held.
	rerollPool     = "leak_reroll_pool"
	rerollAtMost   = 17
	rerollHeldFace = 5
	kindRerollSrc  = "reroll-source"
	kindRerollRes  = "reroll-resource"
	kindRerollMax  = "reroll-at-most"
	kindRerollDie  = "reroll-held-d20"
	kindRerollName = "reroll-source-name"
	// rerollFeatName is the feat's name, which the rolls name to the roller and the master (source_name).
	rerollFeatName = "Dado Sortudo da Mesa"
)

// rerollFeat gives Toren the table feat of the rule and returns the feat's key.
func (rt *rollsTable) rerollFeat() string {
	ctx := rt.t.Context()
	entry := must(rt.master.table.CreateTableEntry(ctx, rq(&rulesv1.CreateTableEntryRequest{
		CampaignId: rt.campaign, Body: &rulesv1.CreateTableEntryRequest_TableFeat{TableFeat: &rulesv1.TableFeat{
			NamePt: rerollFeatName, DescPt: []string{"Um talento da mesa."},
			Effects: []*rulesv1.TableEffect{
				{Type: "resource", Resource: rerollPool, Max: "6", Recharge: "short_rest"},
				{Type: "reroll", Rolls: []string{"attack", "check", "save"}, AtMost: rerollAtMost, Resource: rerollPool, Cost: "1"},
				{Type: "reroll", Rolls: []string{"damage"}, AtMost: 2, Resource: rerollPool, Cost: "1"},
			},
		}},
	}))).GetEntry()
	sheet := rt.toren.GetSheet()
	sheet.GetFull().FeatKeys = append(sheet.GetFull().GetFeatKeys(), entry.GetKey())
	rt.toren = must(rt.master.characters.UpdateCharacter(ctx, rq(&charactersv1.UpdateCharacterRequest{
		CampaignId: rt.campaign, CharacterId: rt.toren.GetId(), Revision: rt.toren.GetRevision(), Name: rt.toren.GetName(), Sheet: sheet,
	}))).GetCharacter()
	return entry.GetKey()
}

// attackLines are the attack lines of the log as p reads them, with the target.
func (rt *rollsTable) attackLines(p *person, target string) []*playv1.CombatLogEntry {
	var out []*playv1.CombatLogEntry
	for _, r := range rt.logOf(p).GetRounds() {
		for _, en := range r.GetEntries() {
			if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_ATTACK && en.GetTargetId() == rt.ids[target] {
				out = append(out, en)
			}
		}
	}
	return out
}

// partLines are the parts of a damage line as a player reads them: key, faces, the faces rolled again with their source
// and whether they count, never the target.
func partLines(en *playv1.CombatLogEntry) []string {
	var out []string
	for _, pr := range en.GetDamage().GetParts() {
		s := fmt.Sprint(pr.GetPartKey(), " ", pr.GetFaces())
		for _, rr := range pr.GetRerolled() {
			s += fmt.Sprintf(" %d>%d@%s", rr.GetFrom(), rr.GetTo(), rr.GetSourceKey())
		}
		if !pr.GetCounted() {
			s += " NOT-COUNTED"
		}
		out = append(out, s)
	}
	return out
}

func TestRerollQuestionsReachOnlyWhoMayReadThem(t *testing.T) {
	var feat string
	rt := newRollsTableOf(t, true, true, func(rt *rollsTable) { feat = rt.rerollFeat() })
	w, m, caio, ana := rt.world, rt.master, rt.caio, rt.ana
	rt.buildRows()
	rt.rows = append(rt.rows,
		read{
			procedure: playv1connect.ResourceServiceListHeldRollsProcedure, allow: members,
			req: func(w *world) proto.Message { return &playv1.ListHeldRollsRequest{CampaignId: w.campaign} },
		},
		read{
			procedure: playv1connect.ResourceServiceGetOutsideInspirationProcedure, allow: members,
			why: "a player reads the die of their own character; the master reads every die held",
			req: func(w *world) proto.Message { return &playv1.GetOutsideInspirationRequest{CampaignId: w.campaign} },
		},
	)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("the table: %v", r)
		}
	}()
	// What the rule holds: the feature, the pool it spends, its limit and the face the attack shows are Caio's and the
	// master's while the roll waits.
	w.secrets.add(&canary{needle: feat, kind: kindRerollSrc, readers: names([]*person{caio})})
	w.secrets.add(&canary{needle: rerollFeatName, kind: kindRerollName, readers: names([]*person{caio})})
	w.secrets.add(&canary{needle: rerollPool, kind: kindRerollRes, readers: names([]*person{caio})})
	w.secrets.add(&canary{needle: "17", kind: kindRerollMax, numeric: true, value: rerollAtMost, fields: []string{"at_most"}, readers: names([]*person{caio})})
	w.secrets.add(&canary{needle: "5", kind: kindRerollDie, numeric: true, value: rerollHeldFace, fields: []string{"faces"}, readers: names([]*person{caio})})
	for _, label := range []string{labelToren, labelPens, "Esqueleto", "Arbusto", "Bandido", "Lobo"} {
		w.secrets.id("held-combatant", rt.ids[label], ana, caio)
	}
	watchers := map[*person]*streamWatcher{m: w.watchStream(m), ana: w.watchStream(ana), caio: w.watchStream(caio)}
	rt.toTurn(labelToren)
	rt.ask(t, "the combat begins")

	// 1. Toren's attack shows a 5, which the rule covers: the question is Caio's and the master's.
	held := rt.mustAttack(caio, labelToren, dagger, "Esqueleto", []int32{rerollHeldFace}, nil)
	offer := held.GetHeldRollOffer()
	if offer == nil || offer.GetReroll() == nil || offer.GetKind() != playv1.HeldRollKind_HELD_ROLL_KIND_WEAPON_ATTACK || offer.GetCombatantId() != rt.ids[labelToren] {
		t.Fatalf("Toren's attack = %v, want the reroll question of his weapon attack", held)
	}
	hold := w.secrets.id("reroll-hold", offer.GetHoldId(), caio)
	rt.ask(t, "the attack waits for the reroll")
	for who, p := range map[string]*person{"the master": m, "Caio": caio} {
		if got := rt.combatantOf(p, labelToren).GetHeldRollOffer(); got.GetHoldId() != hold || got.GetReroll().GetSourceKey() != feat || got.GetReroll().GetResource() != rerollPool || got.GetReroll().GetAtMost() != rerollAtMost || got.GetReroll().GetSourceName() != rerollFeatName || got.GetReroll().GetSides() != 20 {
			t.Errorf("%s reads %v on Toren, want the reroll question with its rule (the positive control)", who, got)
		}
		if got := rt.listHeld(p); len(got) != 1 || got[0].GetHoldId() != hold {
			t.Errorf("%s lists %v, want Toren's question", who, got)
		}
	}
	if got := rt.combatantOf(ana, labelToren).GetHeldRollOffer(); got != nil {
		t.Errorf("Ana reads %v on Toren, want nothing", got)
	}
	if got := rt.listHeld(ana); len(got) != 0 {
		t.Errorf("Ana lists %v, want nothing", got)
	}
	_, err := ana.resources.AnswerHeldRoll(t.Context(), rq(&playv1.AnswerHeldRollRequest{
		CampaignId: w.campaign, HoldId: hold, IdempotencyKey: newKey(),
		Answer: &playv1.AnswerHeldRollRequest_Reroll{Reroll: &playv1.RerollAnswer{Reroll: true}},
	}))
	if connect.CodeOf(err) != connect.CodeNotFound || strings.Contains(err.Error(), hold) {
		t.Errorf("Ana's answer = %v, want not_found without the id", err)
	}
	rt.ask(t, "Ana tried to answer")

	// 2. Caio rolls it again: the new d20 (16) hits, and the log tells everyone the rerolled face and its source.
	w.dice.ScriptFor(t, 20, 16)
	res := must(caio.resources.AnswerHeldRoll(t.Context(), rq(&playv1.AnswerHeldRollRequest{
		CampaignId: w.campaign, HoldId: hold, IdempotencyKey: newKey(),
		Answer: &playv1.AnswerHeldRollRequest_Reroll{Reroll: &playv1.RerollAnswer{Reroll: true}},
	})))
	if rr := res.GetAttack().GetRoll().GetD20().GetRerolled(); len(rr) != 1 || rr[0].GetFrom() != rerollHeldFace || rr[0].GetTo() != 16 || res.GetAttack().GetPendingDamage() == nil {
		t.Fatalf("Caio's answer = %v, want the 5 rolled again to 16 and a hit", res)
	}
	rt.ask(t, "the attack was rolled again")
	for who, p := range map[string]*person{"the master": m, "Caio": caio} {
		lines := rt.attackLines(p, "Esqueleto")
		if len(lines) != 1 || len(lines[0].GetAttackRoll().GetRerolled()) != 1 || lines[0].GetAttackRoll().GetRerolled()[0].GetSourceKey() != feat || lines[0].GetAttackRoll().GetRerolled()[0].GetSourceName() != rerollFeatName {
			t.Errorf("%s reads %v, want the d20 rolled again with its source in the log (the positive control)", who, lines)
		}
	}
	for _, en := range rt.attackLines(ana, "Esqueleto") {
		if len(en.GetAttackRoll().GetRerolled()) != 0 {
			t.Errorf("Ana reads the d20 rolled again %v, want nothing of another player's roll", en.GetAttackRoll().GetRerolled())
		}
	}

	// 3. The smite against the skeleton and, a turn later, against the bandit: the dice show 1, the rule rolls every one
	// of them again, the extra die too, whether it counts or not.
	damage := func(target string, pending string) {
		w.dice.ScriptFor(t, 4, 1, 3)
		w.dice.ScriptFor(t, 8, 1, 1, 1, 6, 4, 2)
		rt.damage(caio, pending, func(r *playv1.RollDamageRequest) {
			r.ExtrasChosen = true
			r.SelectedExtras = []*playv1.SelectedExtra{{Key: "divine-smite", SlotLevel: 1}}
			r.Roll = &playv1.RollDamageRequest_RollInApp{RollInApp: true}
		})
		rt.ask(t, "the damage rolled again on "+target)
	}
	damage("the skeleton", res.GetAttack().GetPendingDamage().GetId())
	rt.endTurn(caio)
	rt.toTurn(labelToren)
	strike := rt.mustAttack(caio, labelToren, dagger, "Bandido", []int32{18}, nil)
	if strike.GetPendingDamage() == nil || strike.GetHeldRollOffer() != nil {
		t.Fatalf("the attack on the bandit = %v, want a hit with no question: the 18 is above the limit", strike)
	}
	damage("the bandit", strike.GetPendingDamage().GetId())
	rt.endTurn(caio)
	// What the roller's player reads of the two damages is the same lines, the extra die among them, rolled again in both;
	// only the master reads which one counted. Another player reads no part of either.
	for _, p := range []*person{caio, m} {
		skeleton, bandit := rt.attackLines(p, "Esqueleto"), rt.attackLines(p, "Bandido")
		if len(skeleton) != 1 || len(bandit) != 1 {
			t.Fatalf("%s's log has %d lines for the skeleton and %d for the bandit, want one each", p.name, len(skeleton), len(bandit))
		}
		a, b := partLines(skeleton[0]), partLines(bandit[0])
		if p == caio && (strings.ReplaceAll(strings.Join(a, "|"), " NOT-COUNTED", "") != strings.ReplaceAll(strings.Join(b, "|"), " NOT-COUNTED", "") || strings.Contains(strings.Join(a, "|")+strings.Join(b, "|"), "NOT-COUNTED") ||
			!strings.Contains(strings.Join(a, "|"), "divine-smite-extra") || !strings.Contains(strings.Join(a, "|"), "@"+feat)) {
			t.Errorf("Caio reads the parts %v for the skeleton and %v for the bandit, want the same lines and rerolled faces, every die counted, the extra die among them", a, b)
		}
		if p == m && (strings.Contains(strings.Join(a, "|"), "NOT-COUNTED") || !strings.Contains(strings.Join(b, "|"), "divine-smite-extra [") || !strings.Contains(strings.Join(b, "|"), "NOT-COUNTED")) {
			t.Errorf("the master reads the parts %v for the skeleton and %v for the bandit, want the extra die counted against the skeleton alone", a, b)
		}
	}
	for _, target := range []string{"Esqueleto", "Bandido"} {
		for _, en := range rt.attackLines(ana, target) {
			if len(en.GetDamage().GetParts()) != 0 {
				t.Errorf("Ana reads the parts %v of an attack on %s, want nothing of another player's damage", partLines(en), target)
			}
		}
	}

	// 4. The master rolls a group check for Toren on a 4: the question is Caio's, the master answers for him once his own
	// chance has passed, and nothing of it reaches Ana.
	view := must(m.contests.RequestGroupCheck(t.Context(), rq(&playv1.RequestGroupCheckRequest{
		CampaignId: w.campaign, IdempotencyKey: newKey(), SkillKey: "skill:stealth", Dc: groupCheckDC,
	}))).GetGroupCheck()
	w.dice.ScriptFor(t, 20, 4)
	asked := must(m.contests.RollForPlayer(t.Context(), rq(&playv1.RollForPlayerRequest{
		CampaignId: w.campaign, IdempotencyKey: newKey(), GroupCheckId: view.GetId(), CharacterId: w.toren.GetId(), Roll: appRoll(),
	})))
	check := asked.GetHeldRollOffer()
	if check == nil || check.GetReroll() == nil || check.GetKind() != playv1.HeldRollKind_HELD_ROLL_KIND_GROUP_CHECK {
		t.Fatalf("the master's roll for Toren = %v, want the reroll question of the group check", asked)
	}
	checkHold := w.secrets.id("reroll-group-hold", check.GetHoldId(), caio)
	rt.ask(t, "the group check waits for the reroll")
	if got := rt.listHeld(ana); len(got) != 0 {
		t.Errorf("Ana lists %v, want nothing", got)
	}
	if got := rt.listHeld(caio); len(got) != 1 || got[0].GetHoldId() != checkHold {
		t.Errorf("Caio lists %v, want the group check's question (the positive control)", got)
	}
	rt.clock.advance(11 * time.Second)
	w.dice.ScriptFor(t, 20, 17)
	out := must(m.resources.AnswerHeldRoll(t.Context(), rq(&playv1.AnswerHeldRollRequest{
		CampaignId: w.campaign, HoldId: checkHold, IdempotencyKey: newKey(),
		Answer: &playv1.AnswerHeldRollRequest_Reroll{Reroll: &playv1.RerollAnswer{Reroll: true}},
	})))
	if out.GetGroupCheck() == nil {
		t.Errorf("the master's answer = %v, want the group check written", out)
	}
	rt.ask(t, "the master answered for Toren")
	if got := rt.listHeld(caio); len(got) != 0 {
		t.Errorf("Caio lists %v after the answer, want nothing", got)
	}

	// 5. The master retires the feat: the roller's sheet keeps it, the roll still lists its key, and nobody is told its name
	// any more (RN-23), not the roller and not the master.
	must(m.table.ArchiveTableEntry(t.Context(), rq(&rulesv1.ArchiveTableEntryRequest{CampaignId: w.campaign, Key: feat})))
	rt.ask(t, "the feat was retired")
	for who, p := range map[string]*person{"the master": m, "Caio": caio} {
		lines := rt.attackLines(p, "Esqueleto")
		if len(lines) != 1 || len(lines[0].GetAttackRoll().GetRerolled()) != 1 || lines[0].GetAttackRoll().GetRerolled()[0].GetSourceKey() != feat || lines[0].GetAttackRoll().GetRerolled()[0].GetSourceName() != "" {
			t.Errorf("%s reads %v after the feat was retired, want the key kept and no name", who, lines)
		}
	}

	// 6. The retired feat still serves the sheet that has it: a new low attack asks the question with the key and no name, to
	// the roller and to the master, and not at all to Ana.
	rt.toTurn(labelToren)
	again := rt.mustAttack(caio, labelToren, dagger, "Lobo", []int32{rerollHeldFace}, nil).GetHeldRollOffer()
	rt.ask(t, "the retired feat asks its question")
	for who, p := range map[string]*person{"the master": m, "Caio": caio} {
		got := rt.combatantOf(p, labelToren).GetHeldRollOffer().GetReroll()
		if again == nil || got.GetSourceKey() != feat || got.GetSourceName() != "" || got.GetSides() != 20 {
			t.Errorf("%s reads the question %v of the retired feat, want its key, no name and a d20", who, got)
		}
		for _, o := range rt.listHeld(p) {
			if o.GetReroll().GetSourceName() != "" {
				t.Errorf("%s lists the retired feat as %q, want no name", who, o.GetReroll().GetSourceName())
			}
		}
	}
	if got := rt.combatantOf(ana, labelToren).GetHeldRollOffer(); got != nil {
		t.Errorf("Ana reads %v on Toren, want nothing", got)
	}
	must(caio.resources.AnswerHeldRoll(t.Context(), rq(&playv1.AnswerHeldRollRequest{
		CampaignId: w.campaign, HoldId: again.GetHoldId(), IdempotencyKey: newKey(),
		Answer: &playv1.AnswerHeldRollRequest_Reroll{Reroll: &playv1.RerollAnswer{Reroll: false}},
	})))

	// The end: the stream told the master and the roller of the steps, and carried no question to Ana.
	must(m.combat.EndEncounter(t.Context(), rq(&playv1.EndEncounterRequest{CampaignId: w.campaign, EncounterId: rt.encounter.GetId(), IdempotencyKey: newKey()})))
	must(m.play.EndGameSession(t.Context(), rq(&playv1.EndGameSessionRequest{CampaignId: w.campaign, GameSessionId: w.session})))
	for p, sw := range watchers {
		select {
		case <-sw.done:
		case <-time.After(30 * time.Second):
			t.Fatalf("%s's stream did not end with the session", p.name)
		}
		var hints int
		for _, ev := range sw.events {
			body, err := protojson.Marshal(ev)
			if err != nil {
				t.Fatalf("marshal an event: %v", err)
			}
			if eventCase(ev) == "held_rolls_changed" {
				hints++
			}
			r := reply{status: 200, body: body, msg: ev}
			rt.got.keep(p, r)
			if p == m {
				continue
			}
			for _, f := range w.inspect(p, r, nil) {
				t.Errorf("%s: event %s: %s\n\tevent: %s", p.name, eventCase(ev), f, shorten(body))
			}
		}
		switch p {
		case ana:
			if hints != 0 {
				t.Errorf("Ana's stream heard %d hints of the held rolls, want none: none was hers", hints)
			}
		case caio, m:
			if hints == 0 {
				t.Errorf("%s's stream heard no hint of the held rolls, want one (the positive control)", p.name)
			}
		}
	}
	t.Run("canaries are reachable", func(t *testing.T) { checkCanariesAreReachable(t, w, rt.got) })
}
