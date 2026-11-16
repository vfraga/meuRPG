package leaktest

import (
	"errors"
	"slices"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1/playv1connect"
)

// The pass and the refund of the Inspiration in a combat (RN-43, RN-10): who reads each line, who reads GetFavor, and who hears each hint.
// One table of rollsTable (Toren is Caio's, Pensantus is Ana's, both combatants) and a third player, Dani, who joined and is not in the
// combat: Dani reads nothing of the Inspiration, in no read and in no hint. Each step writes, then reads again as everyone: the sentences
// are canaries with the people who may read them (the master reads the ones with no reader named), and the positive controls are the
// readers' own answers. The events of each step are the ones the streams got after the step's own marker.

// stepSpec says what a step proves: want holds who must read each canary now (nobody named: the master alone), gone the canaries nobody
// reads any more, allow the kinds of event each player may hear after the step's marker (the master hears whatever) and need the ones
// they must.
type stepSpec struct {
	want  map[*canary][]*person
	gone  []*canary
	allow map[*person][]string
	need  map[*person][]string
	// quiet makes the master's hints count too: the step is one that tells nobody anything.
	quiet bool
	// receiverNothing checks Ana, the receiver of the pass, as it checks Dani: no ask, no prompt, no holder but her own character and no
	// line of the Inspiração that is not one of her own canaries (passedR, undoneR).
	receiverNothing bool
}

// eventCasesAfter are the kinds of the events a stream got after the first n, apart from the marker and the heartbeat.
func eventCasesAfter(sw *streamWatcher, n int) []string {
	var out []string
	for _, ev := range sw.since(n) {
		if ev.GetCurrentMapChanged() != nil || ev.GetHeartbeat() != nil {
			continue
		}
		out = append(out, eventCase(ev))
	}
	return out
}

// passRun is the table, the watchers and the canaries of one run.
type passRun struct {
	t        *testing.T
	rt       *rollsTable
	w        *world
	third    *person
	people   []*person
	watchers map[*person]*streamWatcher
	all      []*canary
	pass     *playv1.FavorPass
	passID   *canary
	prompt   string
	// receiverChecked makes every step check the receiver's reads (receiverNothing of every step).
	receiverChecked bool
}

func (r *passRun) add(kind, text string, readers ...*person) *canary {
	c := r.w.secrets.add(&canary{needle: text, kind: kind, readers: names(readers)})
	r.all = append(r.all, c)
	return c
}

func (r *passRun) settle() {
	r.w.settle(r.watchers[r.w.master], r.watchers[r.w.caio], r.watchers[r.w.ana], r.watchers[r.third])
}

var (
	favorReads = []func(w *world) read{
		func(*world) read {
			return read{
				procedure: playv1connect.FavorServiceGetFavorProcedure, allow: members,
				req: func(w *world) proto.Message { return &playv1.GetFavorRequest{CampaignId: w.campaign} },
			}
		},
		func(*world) read {
			return read{
				procedure: playv1connect.CombatServiceListCombatLogProcedure, allow: members,
				req: func(w *world) proto.Message {
					return &playv1.ListCombatLogRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId()}
				},
			}
		},
	}
	tableHints = []string{"encounter_changed", "combat_log_changed", "turn_changed", "combatant_moved"}
	rollHints  = append(slices.Clone(tableHints), "vitals_changed") // the roller's own vitals change with the spend
	vitalsOnly = []string{"vitals_changed"}
	vitalsLog  = []string{"vitals_changed", "combat_log_changed"}
)

// readAs reads GetFavor and the combat's log as everyone, Dani included, and keeps the answers.
func (r *passRun) readAs() *answers {
	ans := newAnswers()
	for _, mk := range favorReads {
		rd := mk(r.w)
		r.w.runRead(r.t, rd, ans)
		reply := r.third.call(rd.procedure, rd.req(r.w))
		ans.keep(r.third, reply)
		if !reply.ok() {
			r.t.Errorf("Dani was refused %s: HTTP %d %s", rd.procedure, reply.status, reply.body)
		}
		for _, f := range r.w.inspect(r.third, reply, nil) {
			r.t.Errorf("Dani's %s: %s\n\tanswer: %s", rd.procedure, f, shorten(reply.body))
		}
	}
	return ans
}

// thirdReadsNothing is the structural control: Dani's GetFavor is empty and the log holds no line of the Inspiration.
func (r *passRun) thirdReadsNothing(step string) {
	ctx := r.t.Context()
	res := must(r.third.favor.GetFavor(ctx, rq(&playv1.GetFavorRequest{CampaignId: r.w.campaign})))
	if len(res.GetHolders()) != 0 || len(res.GetPasses()) != 0 || len(res.GetRefundPrompts()) != 0 {
		r.t.Errorf("%s: Dani reads GetFavor = %v, want nothing", step, res)
	}
	log := must(r.third.combat.ListCombatLog(ctx, rq(&playv1.ListCombatLogRequest{CampaignId: r.w.campaign, EncounterId: r.w.encounter.GetId()})))
	for _, rd := range log.GetRounds() {
		for _, e := range rd.GetEntries() {
			if e.GetFavor() != nil {
				r.t.Errorf("%s: Dani reads the line %q", step, e.GetFavor().GetTextPt())
			}
		}
	}
}

func (r *passRun) checkReads(name string, spec stepSpec, ans *answers) {
	r.checkWanted(name, spec, ans)
	for _, c := range spec.gone {
		for _, p := range r.people {
			if ans.holds(p, c) {
				r.t.Errorf("%s: %s still reads %s after its undo", name, p.name, c)
			}
		}
	}
	for _, c := range r.all {
		if ans.holds(r.third, c) {
			r.t.Errorf("%s: Dani reads %s", name, c)
		}
	}
}

func (r *passRun) checkWanted(name string, spec stepSpec, ans *answers) {
	for c, readers := range spec.want {
		for _, p := range readers {
			if !ans.holds(p, c) {
				r.t.Errorf("%s: %s reads %s in no answer: the control that the line is there is missing", name, p.name, c)
			}
		}
		if len(readers) == 0 && !ans.holds(r.w.master, c) {
			r.t.Errorf("%s: the master reads %s in no answer", name, c)
		}
	}
}

func (r *passRun) checkHints(name string, spec stepSpec, marks map[*person]int) {
	for _, p := range r.people {
		got := eventCasesAfter(r.watchers[p], marks[p])
		if p != r.w.master || spec.quiet {
			for _, k := range got {
				if !slices.Contains(spec.allow[p], k) {
					r.t.Errorf("%s: %s heard %s, want only %v (heard %v)", name, p.name, k, spec.allow[p], got)
				}
			}
		}
		for _, k := range spec.need[p] {
			if !slices.Contains(got, k) {
				r.t.Errorf("%s: %s heard no %s (heard %v): the control that the hint is sent is missing", name, p.name, k, got)
			}
		}
	}
}

// step does one write, settles the streams, reads as everyone and checks the reads and the hints.
func (r *passRun) step(name string, do func(), spec stepSpec) {
	r.t.Helper()
	marks := map[*person]int{}
	for p, sw := range r.watchers {
		marks[p] = sw.count()
	}
	do()
	r.settle()
	r.checkReads(name, spec, r.readAs())
	r.thirdReadsNothing(name)
	if spec.receiverNothing || r.receiverChecked {
		r.receiverReadsNothing(name)
	}
	r.checkHints(name, spec, marks)
}

// receiverReadsNothing is the structural control for Ana, who owns the receiver: GetFavor holds no ask and no prompt and only her own
// character as a holder, and the log holds no line of the Inspiração but the ones a canary of hers names.
func (r *passRun) receiverReadsNothing(step string) {
	ctx := r.t.Context()
	w := r.w
	res := must(w.ana.favor.GetFavor(ctx, rq(&playv1.GetFavorRequest{CampaignId: w.campaign})))
	if len(res.GetPasses()) != 0 || len(res.GetRefundPrompts()) != 0 {
		r.t.Errorf("%s: Ana reads GetFavor = %v, want no ask and no prompt", step, res)
	}
	for _, h := range res.GetHolders() {
		if h.GetCharacterId() != w.pens.GetId() {
			r.t.Errorf("%s: Ana reads the holder %v, want only her own character", step, h)
		}
	}
	own := map[string]bool{}
	for _, c := range r.all {
		if c.readers[w.ana.name] {
			own[c.needle] = true
		}
	}
	log := must(w.ana.combat.ListCombatLog(ctx, rq(&playv1.ListCombatLogRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId()})))
	for _, rd := range log.GetRounds() {
		for _, e := range rd.GetEntries() {
			if e.GetFavor() != nil && !own[e.GetFavor().GetTextPt()] {
				r.t.Errorf("%s: Ana reads the line %q, which is not hers", step, e.GetFavor().GetTextPt())
			}
		}
	}
}

func (r *passRun) grant(character string) func() {
	return func() {
		w := r.w
		must(w.master.favor.GrantFavor(r.t.Context(), rq(&playv1.GrantFavorRequest{CampaignId: w.campaign, CharacterId: character, IdempotencyKey: newKey()})))
	}
}

func (r *passRun) ask() {
	w := r.w
	r.pass = must(w.caio.favor.PassFavor(r.t.Context(), rq(&playv1.PassFavorRequest{
		CampaignId: w.campaign, FromCharacterId: w.toren.GetId(), ToCharacterId: w.pens.GetId(), IdempotencyKey: newKey(),
	}))).GetPass()
	r.passID = w.secrets.add(&canary{needle: r.pass.GetId(), kind: "favor-pass-id", readers: names([]*person{w.caio})})
	r.all = append(r.all, r.passID)
}

func (r *passRun) answerPass(accept bool) func() {
	return func() {
		w := r.w
		must(w.master.favor.AnswerFavorPass(r.t.Context(), rq(&playv1.AnswerFavorPassRequest{
			CampaignId: w.campaign, PassId: r.pass.GetId(), Accept: accept, IdempotencyKey: newKey(),
		})))
	}
}

func (r *passRun) cancel() {
	w := r.w
	must(w.caio.favor.CancelFavorPass(r.t.Context(), rq(&playv1.CancelFavorPassRequest{CampaignId: w.campaign, PassId: r.pass.GetId(), IdempotencyKey: newKey()})))
}

func (r *passRun) undo() {
	w := r.w
	ctx := r.t.Context()
	log := must(w.master.combat.ListCombatLog(ctx, rq(&playv1.ListCombatLogRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId()})))
	must(w.master.combat.UndoLastAction(ctx, rq(&playv1.UndoLastActionRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), ExpectedEventId: log.GetUndoableEventId(), IdempotencyKey: newKey(),
	})))
}

// hide puts an advantage on the combatant that its player does not read: a spend on it adds nothing over every source.
func (r *passRun) hide(label string) {
	_, err := r.w.pool.Exec(r.t.Context(), `INSERT INTO combatant_states (encounter_id, combatant_id, kind, started_round, created_at, group_id, source_key, source_kind, duration_kind, modifiers, player_visible)
		SELECT encounter_id, id, 'effect', 1, now(), gen_random_uuid(), 'spell:bless', 'master', 'until_dismissed', $3::jsonb, false FROM combatants WHERE encounter_id = $1 AND label = $2`,
		r.w.encounter.GetId(), label, `[{"kind":"attack_advantage"}]`)
	if err != nil {
		panic(err)
	}
}

// spendAndPrompt is Toren's attack that spends the Inspiration for nothing; the master's prompt is then open.
func (r *passRun) spendAndPrompt() {
	r.rt.mustAttack(r.w.caio, labelToren, dagger, "Esqueleto", []int32{4, 17}, func(req *playv1.RollAttackRequest) { req.SpendFavor = true })
	prompts := must(r.w.master.favor.GetFavor(r.t.Context(), rq(&playv1.GetFavorRequest{CampaignId: r.w.campaign}))).GetRefundPrompts()
	if len(prompts) != 1 {
		panic("the master reads no single refund prompt")
	}
	r.prompt = prompts[0].GetId()
	r.all = append(r.all, r.w.secrets.add(&canary{needle: r.prompt, kind: "favor-refund-prompt-id"}))
}

func (r *passRun) answerRefund(giveBack bool) func() {
	return func() {
		w := r.w
		must(w.master.favor.AnswerFavorRefund(r.t.Context(), rq(&playv1.AnswerFavorRefundRequest{
			CampaignId: w.campaign, PromptId: r.prompt, GiveBack: giveBack, IdempotencyKey: newKey(),
		})))
	}
}

func newPassRun(t *testing.T) *passRun {
	t.Helper()
	rt := newRollsTable(t)
	w := rt.world
	third := w.newPerson("Dani")
	w.join(false, third)
	r := &passRun{t: t, rt: rt, w: w, third: third, people: []*person{w.master, w.caio, w.ana, third}}
	r.watchers = map[*person]*streamWatcher{}
	for _, p := range r.people {
		r.watchers[p] = w.watchStream(p)
	}
	r.settle()
	return r
}

func TestFavorPassAndRefundReachOnlyWhoTheyAreFor(t *testing.T) {
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("the test: %v", rec)
		}
	}()
	r := newPassRun(t)
	r.receiverChecked = true
	w, caio, ana := r.w, r.w.caio, r.w.ana
	toren, pens := labelToren, labelPens
	grantT := r.add("favor-line-grant", "O mestre deu a Inspiração a "+toren+".")
	refusedM := r.add("favor-line-refused-master", "O mestre recusou o pedido de "+toren+": a Inspiração continua com "+toren+".")
	refusedG := r.add("favor-line-refused-giver", "O mestre recusou: a Inspiração continua com você.", caio)
	canceledM := r.add("favor-line-canceled-master", toren+" desistiu de passar a Inspiração para "+pens+".")
	canceledG := r.add("favor-line-canceled-giver", "Você desistiu de passar a Inspiração.", caio)
	passedM := r.add("favor-line-passed-master", toren+" passou a Inspiração para "+pens+".")
	passedG := r.add("favor-line-passed-giver", "O mestre aceitou: a sua Inspiração foi para "+pens+".", caio)
	passedR := r.add("favor-line-passed-receiver", toren+" passou a Inspiração para você.", ana)
	undoneR := r.add("favor-line-undone-receiver", "O mestre desfez a passagem: a Inspiração não está mais com você.", ana)
	refundedM := r.add("favor-line-refunded-master", "O mestre devolveu a Inspiração de "+toren+".")
	refundedR := r.add("favor-line-refunded-roller", "O mestre devolveu a sua Inspiração.", caio)
	// The sources the roller could not read are the master's text alone, in the prompt he reads and in no other field or reader.
	hiddenText := r.add("favor-refund-hidden-source", "Bênção: vantagem neste ataque")
	both := func(cs ...*canary) map[*canary][]*person {
		out := map[*canary][]*person{}
		for _, c := range cs {
			out[c] = nil
		}
		return out
	}
	toCaio := func(allow, need []string) (map[*person][]string, map[*person][]string) {
		return map[*person][]string{caio: allow}, map[*person][]string{caio: need}
	}

	// The grant is the master's line alone: the holder's player hears the vitals hint, not the log's.
	r.step("the grant", r.grant(w.toren.GetId()), stepSpec{want: both(grantT), allow: map[*person][]string{caio: vitalsOnly}, need: map[*person][]string{caio: vitalsOnly, w.master: vitalsLog}})
	// The ask is no line: the giver hears the vitals hint, and Ana (the receiver) and Dani nothing.
	r.step("the ask", r.ask, stepSpec{allow: map[*person][]string{caio: vitalsOnly}, need: map[*person][]string{caio: vitalsOnly}})
	// A refusal moved nothing: the giver and the master read it and hear it; the receiver hears and reads nothing.
	allow, need := toCaio(vitalsLog, vitalsLog)
	want := both(refusedM)
	want[refusedG] = []*person{caio}
	r.step("the refusal", r.answerPass(false), stepSpec{want: want, allow: allow, need: need})
	allow, need = toCaio(vitalsLog, []string{"combat_log_changed"})
	r.step("the undo of the refusal", r.undo, stepSpec{gone: []*canary{refusedM, refusedG}, allow: allow, need: need})
	r.step("the second ask", r.ask, stepSpec{allow: map[*person][]string{caio: vitalsOnly}})
	want = both(canceledM)
	want[canceledG] = []*person{caio}
	allow, need = toCaio(vitalsLog, vitalsLog)
	r.step("the cancel", r.cancel, stepSpec{want: want, allow: allow, need: need})
	allow, _ = toCaio(vitalsLog, nil)
	r.step("the undo of the cancel", r.undo, stepSpec{gone: []*canary{canceledM, canceledG}, allow: allow})
	r.step("the third ask", r.ask, stepSpec{allow: map[*person][]string{caio: vitalsOnly}})
	r.step("the third player's writes", r.thirdWrites, stepSpec{quiet: true})
	// Only the accepted pass reaches the receiver.
	want = both(passedM)
	want[passedG], want[passedR] = []*person{caio}, []*person{ana}
	r.step("the accept", r.answerPass(true), stepSpec{
		want:  want,
		allow: map[*person][]string{caio: vitalsLog, ana: vitalsLog}, need: map[*person][]string{caio: vitalsLog, ana: vitalsLog},
	})
	r.step("the undo of the pass", r.undo, stepSpec{
		want: map[*canary][]*person{undoneR: {ana}}, gone: []*canary{passedM, passedG, passedR},
		allow: map[*person][]string{caio: vitalsLog, ana: vitalsLog}, need: map[*person][]string{ana: vitalsOnly},
	})
	r.step("the undo of the grant", r.undo, stepSpec{gone: []*canary{grantT}, allow: map[*person][]string{caio: vitalsOnly}, need: map[*person][]string{caio: vitalsOnly}})

	// The refund: a roll that spent the Inspiration for nothing (a hidden advantage the roller does not read) asks the master alone.
	r.hide(toren)
	r.step("the grant for the refund", r.grant(w.toren.GetId()), stepSpec{want: both(grantT), allow: map[*person][]string{caio: vitalsOnly}})
	rolled := stepSpec{want: both(hiddenText), allow: map[*person][]string{caio: rollHints, ana: tableHints, r.third: tableHints}}
	r.step("the roll that opens the prompt", r.spendAndPrompt, rolled)
	allow, need = toCaio(vitalsLog, vitalsLog)
	r.step("the refund", r.answerRefund(true), stepSpec{want: map[*canary][]*person{refundedM: nil, refundedR: {caio}}, allow: allow, need: need})

	// The returned Inspiration is Toren's again: the next round he spends it the same way and the master keeps it spent.
	r.rt.toTurn(pens)
	r.rt.toTurn(toren)
	r.settle()
	r.step("the roll that opens the second prompt", r.spendAndPrompt, rolled)
	r.step("the keep", r.answerRefund(false), stepSpec{need: map[*person][]string{w.master: vitalsOnly}})
}

// readsFavor is what a player's GetFavor says: the characters it names as holding the Inspiração and the receiver_has of each pass.
func (r *passRun) readsFavor(p *person) (holders []string, receiverHas []bool) {
	res := must(p.favor.GetFavor(r.t.Context(), rq(&playv1.GetFavorRequest{CampaignId: r.w.campaign})))
	for _, h := range res.GetHolders() {
		holders = append(holders, h.GetCharacterId())
	}
	slices.Sort(holders)
	for _, ps := range res.GetPasses() {
		receiverHas = append(receiverHas, ps.GetReceiverHas())
	}
	return holders, receiverHas
}

// wantFavorReads checks the structural control of the one-owner table: who reads which holder and which pass, and that the receiver_has of
// a pass is the master's alone, whoever owns the receiver.
func (r *passRun) wantFavorReads(step string, holders []string, masterHas, ownerHas []bool) {
	r.t.Helper()
	w := r.w
	want := slices.Clone(holders)
	slices.Sort(want)
	gotHolders, gotHas := r.readsFavor(w.caio)
	if !slices.Equal(gotHolders, want) || !slices.Equal(gotHas, ownerHas) {
		r.t.Errorf("%s: Caio reads holders %v and receiver_has %v, want %v and %v", step, gotHolders, gotHas, want, ownerHas)
	}
	if _, has := r.readsFavor(w.master); !slices.Equal(has, masterHas) {
		r.t.Errorf("%s: the master reads receiver_has %v, want %v", step, has, masterHas)
	}
	if gotHolders, gotHas = r.readsFavor(w.ana); len(gotHolders) != 0 || len(gotHas) != 0 {
		r.t.Errorf("%s: Ana, who owns neither character now, reads holders %v and passes %v, want nothing", step, gotHolders, gotHas)
	}
}

// One player owns the giver and the receiver of a pass (Caio plays Pensantus now, and Ana owns neither character): the accept is the giver's
// line for him and the receiver's sentence is nobody's, the undo of the pass writes no line for him, the receiver_has of the pass is the
// master's alone even when the receiver holds one already, and Ana, who owned the receiver, hears and reads nothing of it.
func TestFavorPassBetweenOnePlayersCharactersReachesHimAlone(t *testing.T) {
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("the test: %v", rec)
		}
	}()
	r := newPassRun(t)
	r.receiverChecked = true
	w, caio := r.w, r.w.caio
	for _, q := range []string{`UPDATE characters SET status = 'dead', died_at = now(), player_user_id = $2 WHERE id = $1`, `UPDATE combatants SET user_id = $2 WHERE character_id = $1`} {
		if _, err := w.pool.Exec(t.Context(), q, w.pens.GetId(), caio.id); err != nil {
			t.Fatalf("hand Pensantus to Caio: %v", err)
		}
	}
	r.settle()
	toren, pens := labelToren, labelPens
	grantT := r.add("favor-line-grant", "O mestre deu a Inspiração a "+toren+".")
	grantP := r.add("favor-line-grant-receiver", "O mestre deu a Inspiração a "+pens+".")
	passedM := r.add("favor-line-passed-master", toren+" passou a Inspiração para "+pens+".")
	passedG := r.add("favor-line-passed-giver", "O mestre aceitou: a sua Inspiração foi para "+pens+".", caio)
	// The receiver's sentences are written for a receiver who is another player: with one owner nobody reads them but the master's log.
	r.add("favor-line-passed-receiver", toren+" passou a Inspiração para você.")
	r.add("favor-line-undone-receiver", "O mestre desfez a passagem: a Inspiração não está mais com você.")
	canceledM := r.add("favor-line-canceled-master", toren+" desistiu de passar a Inspiração para "+pens+".")
	canceledG := r.add("favor-line-canceled-giver", "Você desistiu de passar a Inspiração.", caio)
	toCaio := func(allow, need []string) stepSpec {
		return stepSpec{allow: map[*person][]string{caio: allow}, need: map[*person][]string{caio: need}}
	}
	tID, pID := w.toren.GetId(), w.pens.GetId()

	r.step("the grant", r.grant(tID), stepSpec{want: map[*canary][]*person{grantT: nil}, allow: map[*person][]string{caio: vitalsOnly}, need: map[*person][]string{caio: vitalsOnly}})
	r.step("the ask", r.ask, toCaio(vitalsOnly, vitalsOnly))
	r.wantFavorReads("the ask", []string{tID}, []bool{false}, []bool{false})
	spec := toCaio(vitalsLog, vitalsLog)
	spec.want = map[*canary][]*person{passedM: nil, passedG: {caio}}
	r.step("the accept", r.answerPass(true), spec)
	r.wantFavorReads("the accept", []string{pID}, nil, nil)
	spec = toCaio(vitalsLog, vitalsOnly)
	spec.gone = []*canary{passedM, passedG}
	r.step("the undo of the pass", r.undo, spec)
	r.wantFavorReads("the undo of the pass", []string{tID}, nil, nil)

	// The receiver holds one too: the master reads receiver_has, the owner of both does not.
	// Pensantus is dead (a player owns one living character), and the vitals of a dead character tell nobody anything.
	r.step("the grant to the receiver", r.grant(pID), stepSpec{want: map[*canary][]*person{grantP: nil}, allow: map[*person][]string{caio: vitalsOnly}})
	r.step("the second ask", r.ask, toCaio(vitalsOnly, vitalsOnly))
	r.wantFavorReads("the second ask", []string{tID, pID}, []bool{true}, []bool{false})
	spec = toCaio(vitalsLog, vitalsLog)
	spec.want = map[*canary][]*person{canceledM: nil, canceledG: {caio}}
	r.step("the cancel", r.cancel, spec)
	r.wantFavorReads("the cancel", []string{tID, pID}, nil, nil)
}

// blockedReason is the FavorBlocked reason a refusal carries, unspecified when it carries none.
func blockedReason(err error) playv1.FavorBlockedReason {
	if ce, ok := errors.AsType[*connect.Error](err); ok {
		for _, d := range ce.Details() {
			if v, derr := d.Value(); derr == nil {
				if b, isBlocked := v.(*playv1.FavorBlocked); isBlocked {
					return b.GetReason()
				}
			}
		}
	}
	return playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_UNSPECIFIED
}

// wantRefused checks the exact code and reason of a write a third player made: a stranger's character or pass is not told apart from one that
// is not there, and a call of the master's is the master's.
func (r *passRun) wantRefused(what string, err error, code connect.Code, reason playv1.FavorBlockedReason) {
	r.t.Helper()
	if connect.CodeOf(err) != code || blockedReason(err) != reason {
		r.t.Errorf("%s: error = %v (code %v, reason %v), want code %v and reason %v", what, err, connect.CodeOf(err), blockedReason(err), code, reason)
	}
}

// thirdWrites are Dani's writes on an Inspiration that is not hers while Caio's ask is pending: each is refused with its exact code and
// reason, changes nothing and tells nobody anything.
func (r *passRun) thirdWrites() {
	w := r.w
	ctx := r.t.Context()
	unspecified := playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_UNSPECIFIED
	_, err := r.third.favor.PassFavor(ctx, rq(&playv1.PassFavorRequest{CampaignId: w.campaign, FromCharacterId: w.toren.GetId(), ToCharacterId: w.pens.GetId(), IdempotencyKey: newKey()}))
	r.wantRefused("Dani's PassFavor of Caio's character", err, connect.CodeFailedPrecondition, playv1.FavorBlockedReason_FAVOR_BLOCKED_REASON_NOT_THE_HOLDER)
	_, err = r.third.favor.CancelFavorPass(ctx, rq(&playv1.CancelFavorPassRequest{CampaignId: w.campaign, PassId: r.pass.GetId(), IdempotencyKey: newKey()}))
	r.wantRefused("Dani's CancelFavorPass of Caio's ask", err, connect.CodeNotFound, unspecified)
	_, err = r.third.favor.AnswerFavorPass(ctx, rq(&playv1.AnswerFavorPassRequest{CampaignId: w.campaign, PassId: r.pass.GetId(), Accept: true, IdempotencyKey: newKey()}))
	r.wantRefused("Dani's AnswerFavorPass", err, connect.CodePermissionDenied, unspecified)
	_, err = r.w.ana.favor.CancelFavorPass(ctx, rq(&playv1.CancelFavorPassRequest{CampaignId: w.campaign, PassId: r.pass.GetId(), IdempotencyKey: newKey()}))
	r.wantRefused("the receiver's CancelFavorPass of the ask", err, connect.CodeNotFound, unspecified)
	if r.prompt != "" {
		// The prompt is the master's alone to answer: a player, the roller included, is refused whatever the answer is.
		for _, p := range []*person{r.third, w.caio} {
			for _, giveBack := range []bool{true, false} {
				_, err = p.favor.AnswerFavorRefund(ctx, rq(&playv1.AnswerFavorRefundRequest{CampaignId: w.campaign, PromptId: r.prompt, GiveBack: giveBack, IdempotencyKey: newKey()}))
				r.wantRefused(p.name+"'s AnswerFavorRefund", err, connect.CodePermissionDenied, unspecified)
			}
		}
	}
}

// A session that ends cancels the pass still pending and keeps the refund prompt still open: each is the master's and the giver's owner's to
// hear, nobody else's (the receiver and a third player hear nothing of either), and no pass or prompt is read after it.
func TestFavorSessionEndReachesOnlyWhoItIsFor(t *testing.T) {
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("the test: %v", rec)
		}
	}()
	r := newPassRun(t)
	r.receiverChecked = true
	w, caio := r.w, r.w.caio
	hiddenText := r.add("favor-refund-hidden-source", "Bênção: vantagem neste ataque")
	r.hide(labelToren)
	r.step("the grant", r.grant(w.toren.GetId()), stepSpec{allow: map[*person][]string{caio: vitalsOnly}})
	r.step("the roll that opens the prompt", r.spendAndPrompt, stepSpec{
		want: map[*canary][]*person{hiddenText: nil}, allow: map[*person][]string{caio: rollHints, w.ana: tableHints, r.third: tableHints},
	})
	r.step("the grant again", r.grant(w.toren.GetId()), stepSpec{allow: map[*person][]string{caio: vitalsOnly}})
	r.step("the ask", r.ask, stepSpec{allow: map[*person][]string{caio: vitalsOnly}})
	r.step("the third player's writes", r.thirdWrites, stepSpec{quiet: true, want: map[*canary][]*person{r.passID: {caio}}})
	r.endTheSession(map[*person]bool{w.master: true, caio: true})
	var pass, prompt string
	if err := w.pool.QueryRow(t.Context(), `SELECT (SELECT status FROM favor_passes WHERE id = $1), (SELECT status FROM favor_refund_prompts WHERE id = $2)`, r.pass.GetId(), r.prompt).Scan(&pass, &prompt); err != nil {
		t.Fatalf("read the pass and the prompt: %v", err)
	}
	if pass != "canceled" || prompt != "kept" {
		t.Errorf("after the session's end the pass is %q and the prompt %q, want the pass canceled and the prompt kept", pass, prompt)
	}
}

// endTheSession ends the combat and the session and reads what each stream heard: the vitals hint of Toren (the cancel of his ask and the
// keeping of his prompt) reaches the master and the giver's owner and nobody else, and no event of the end carries a canary of the pass or
// the prompt to a player who may not read it. The streams end with the session, so the events are read after they close.
func (r *passRun) endTheSession(hearsToren map[*person]bool) {
	w := r.w
	ctx := r.t.Context()
	marks := map[*person]int{}
	for p, sw := range r.watchers {
		marks[p] = sw.count()
	}
	must(w.master.combat.EndEncounter(ctx, rq(&playv1.EndEncounterRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), IdempotencyKey: newKey()})))
	must(w.master.play.EndGameSession(ctx, rq(&playv1.EndGameSessionRequest{CampaignId: w.campaign, GameSessionId: w.session})))
	for p, sw := range r.watchers {
		select {
		case <-sw.done:
		case <-time.After(30 * time.Second):
			r.t.Fatalf("%s's stream did not end with the session", p.name)
		}
		heard := false
		for _, ev := range sw.since(marks[p]) {
			if ev.GetVitalsChanged().GetVitals().GetCharacterId() == w.toren.GetId() {
				heard = true
			}
			if p == w.master {
				continue
			}
			body := mustJSON(ev)
			for _, f := range w.inspect(p, reply{status: 200, body: body, msg: ev}, nil) {
				r.t.Errorf("%s: event %s of the session's end: %s\n\tevent: %s", p.name, eventCase(ev), f, shorten(body))
			}
		}
		if heard != hearsToren[p] {
			r.t.Errorf("%s heard Toren's vitals hint at the session's end = %v, want %v", p.name, heard, hearsToren[p])
		}
	}
}

// hideOutside is the master's advantage on a character that its player does not read, out of any combat: a spend on a check adds nothing
// over every source.
func (r *passRun) hideOutside(characterID string) {
	_, err := r.w.pool.Exec(r.t.Context(), `INSERT INTO character_effects (campaign_id, character_id, group_id, source_key, source_kind, duration_kind, modifiers, player_visible)
		VALUES ($1, $2, gen_random_uuid(), 'spell:enhance-ability', 'spell', 'until_dismissed', $3::jsonb, false)`,
		r.w.campaign, characterID, `[{"kind":"check_advantage","abilities":["str","dex","con","int","wis","cha"]}]`)
	if err != nil {
		panic(err)
	}
}

// Outside any combat there is no log line: the ask, the pass and the refund are in GetFavor and in the hints alone. The ask is the giver's
// owner's and the master's, the prompt and its hidden source the master's, and the receiver and a third player read and hear nothing of
// either, before and after the master answers.
func TestFavorOutsideACombatReachesOnlyWhoItIsFor(t *testing.T) {
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("the test: %v", rec)
		}
	}()
	r := newPassRun(t)
	r.receiverChecked = true
	w, caio := r.w, r.w.caio
	hiddenText := r.add("favor-refund-hidden-source-outside", "Efeito ativo: vantagem em testes de habilidade")
	must(w.master.combat.EndEncounter(t.Context(), rq(&playv1.EndEncounterRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), IdempotencyKey: newKey()})))
	r.settle()
	toGiver := map[*person][]string{caio: vitalsOnly}
	r.step("the grant", r.grant(w.toren.GetId()), stepSpec{allow: toGiver})
	r.step("the ask", r.ask, stepSpec{allow: toGiver, need: toGiver})
	r.step("the third player's writes", r.thirdWrites, stepSpec{quiet: true, want: map[*canary][]*person{r.passID: {caio}}})
	r.step("the accept", r.answerPass(true), stepSpec{
		gone:  []*canary{r.passID},
		allow: map[*person][]string{caio: vitalsOnly, w.ana: vitalsOnly}, need: map[*person][]string{caio: vitalsOnly, w.ana: vitalsOnly},
	})

	// The refund: Pensantus holds it now; she spends it on a group check with a hidden advantage.
	r.hideOutside(w.pens.GetId())
	r.step("the roll that opens the prompt", func() {
		asked := must(w.master.contests.RequestGroupCheck(t.Context(), rq(&playv1.RequestGroupCheckRequest{
			CampaignId: w.campaign, IdempotencyKey: newKey(), SkillKey: "skill:stealth", Dc: groupCheckDC, CharacterIds: []string{w.pens.GetId()},
		}))).GetGroupCheck()
		if reply := w.ana.call(playv1connect.ContestServiceRollGroupCheckProcedure, &playv1.RollGroupCheckRequest{
			CampaignId: w.campaign, IdempotencyKey: newKey(), GroupCheckId: asked.GetId(), Roll: appRoll(), SpendFavor: true,
		}); !reply.ok() {
			panic("Ana's group check was refused: " + string(reply.body))
		}
		prompts := must(w.master.favor.GetFavor(t.Context(), rq(&playv1.GetFavorRequest{CampaignId: w.campaign}))).GetRefundPrompts()
		if len(prompts) != 1 {
			panic("the master reads no single refund prompt")
		}
		r.prompt = prompts[0].GetId()
		r.all = append(r.all, w.secrets.add(&canary{needle: r.prompt, kind: "favor-refund-prompt-id"}))
	}, stepSpec{want: map[*canary][]*person{hiddenText: nil}, allow: map[*person][]string{w.ana: {"vitals_changed", "group_check_changed"}, caio: {"group_check_changed"}, r.third: {"group_check_changed"}}})
	r.step("the third player's and the roller's answers to the prompt", r.thirdWrites, stepSpec{quiet: true})
	r.step("the refund", r.answerRefund(true), stepSpec{allow: map[*person][]string{w.ana: vitalsOnly}, need: map[*person][]string{w.ana: vitalsOnly}})
}
