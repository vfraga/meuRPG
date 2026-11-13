package leaktest

import (
	"bytes"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1/playv1connect"
)

// An adult red dragon in Ana's sight: the master reads its legendary state (3 actions, 3 uses
// of Legendary Resistance, 3 options), and no player reads the field in the combat, the log or
// anywhere the matrix looks (RN-20, RN-29). The matrix flags the field as a master-only field
// in every player's answer; this test is the positive control that the field is populated.
func TestTheDragonsLegendaryStateIsTheMastersAlone(t *testing.T) {
	w := newWorld(t)
	enc := w.master.currentEncounter(w)
	var dragon *playv1.Combatant
	for _, c := range enc.GetCombatants() {
		if c.GetBestiaryCreatureKey() == dragonKey {
			dragon = c
		}
	}
	l := dragon.GetLegendary()
	if l == nil || l.GetActionsPerRound() != 3 || l.GetActionsLeft() != 3 || l.GetResistancePerDay() != 3 || l.GetResistanceLeft() != 3 || len(l.GetOptions()) != 3 {
		t.Fatalf("the master reads the dragon's legendary state as %v, want 3/3 actions, 3/3 resistance and 3 options", l)
	}
	// The Portuguese name and the attack key are the master's too: the master reads them (the positive control).
	tail := l.GetOptions()[1]
	if tail.GetNamePt() != "Ataque de cauda" || tail.GetAttackKey() == "" || l.GetOptions()[0].GetAttackKey() != "" || l.GetOptions()[2].GetNamePt() != "Ataque de asas" {
		t.Fatalf("the master reads the options as %v, want name_pt \"Ataque de cauda\" and an attack_key on the tail attack alone", l.GetOptions())
	}
	if tail.GetAttackDetailPt() != "Cauda: +14 · 2d8 + 8 de concussão · alcance 4,5 m" || l.GetOptions()[0].GetAttackDetailPt() != "" {
		t.Fatalf("the master reads the tail attack's detail as %q, want its numbers line, and none on the text option", tail.GetAttackDetailPt())
	}
	for _, p := range []*person{w.ana, w.caio} {
		seen := false
		mustSee := p == w.ana // the dragon stands in Ana's sight, not in Caio's
		for _, c := range p.currentEncounter(w).GetCombatants() {
			if c.GetId() != dragon.GetId() {
				continue
			}
			seen = true
			if c.GetLegendary() != nil {
				t.Errorf("%s reads the dragon's legendary state %v", p.name, c.GetLegendary())
			}
		}
		if mustSee && !seen {
			t.Errorf("%s does not read the dragon, which is in plain sight: the control is missing", p.name)
		}
		for _, r := range w.reads(p) {
			for _, f := range w.inspect(p, r, nil) {
				t.Errorf("%s: %s", p.name, f)
			}
		}
	}
}

// legendaryWords are what no player reads of a legendary spend: the option names (an attack, a text option),
// their keys and the word itself. The master's answers hold them (the positive control).
var legendaryWords = []string{"Wing Attack", "wing-attack", "Tail Attack", "tail-attack", "Ataque de asas", "Ataque de cauda", "Detectar", "legendary", "Cauda: +14", "2d8 + 8 de concussão"}

// offTurn passes the turns until the combatant is not the one on turn: it may spend its legendary actions.
func (w *world) offTurn(c *playv1.Combatant) {
	w.t.Helper()
	ctx := w.t.Context()
	m := w.master
	for range 40 {
		cur := must(m.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
		if cur.GetCurrentCombatantId() != c.GetId() {
			w.encounter = cur
			return
		}
		must(m.combat.EndTurn(ctx, rq(&playv1.EndTurnRequest{CampaignId: w.campaign, EncounterId: cur.GetId(), IdempotencyKey: newKey(), ExpectedCombatantId: cur.GetCurrentCombatantId(), ExpectedRound: cur.GetRound(), DiscardPendingDamage: true})))
	}
	w.t.Fatalf("the turn of %s never passed", c.GetLabel())
}

// The master spends the dragon's legendary actions at the end of another's turn: on a text option (Wing
// Attack, 2 actions) and on an attack (Tail Attack, through RollAttack). No player reads the option, the
// cost, the count or the word in the combat, the log or the answers they hold; the attack is the ordinary
// line of the dragon's tail on Toren. The master reads them (the positive control), and a player's own try
// to spend is refused.
func TestALegendarySpendIsTheMastersAlone(t *testing.T) {
	w := newWorld(t)
	for _, word := range legendaryWords {
		w.secrets.add(&canary{needle: word, kind: "legendary-spend"})
	}
	var dragon *playv1.Combatant
	for _, c := range w.master.currentEncounter(w).GetCombatants() {
		if c.GetBestiaryCreatureKey() == dragonKey {
			dragon = c
		}
	}
	w.offTurn(dragon)

	text := w.master.call(playv1connect.CombatServiceSpendLegendaryActionProcedure, &playv1.SpendLegendaryActionRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: dragon.GetId(), OptionKey: "wing-attack", IdempotencyKey: newKey(),
	})
	if !text.ok() {
		t.Fatalf("the text spend: status %d %s", text.status, text.body)
	}
	strike := w.master.call(playv1connect.CombatServiceRollAttackProcedure, &playv1.RollAttackRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), AttackerId: dragon.GetId(), AttackKey: "basic:2", TargetId: w.combatant(w.toren).GetId(),
		IdempotencyKey: newKey(), D20Faces: []int32{20, 20}, LegendaryOption: "tail-attack",
	})
	if !strike.ok() {
		t.Fatalf("the legendary attack: status %d %s", strike.status, strike.body)
	}
	for _, p := range []*person{w.ana, w.caio} {
		for _, r := range w.readsOf(p) {
			for _, f := range w.inspect(p, r, nil) {
				t.Errorf("%s: %s", p.name, f)
			}
		}
		refused := p.call(playv1connect.CombatServiceSpendLegendaryActionProcedure, &playv1.SpendLegendaryActionRequest{
			CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: dragon.GetId(), OptionKey: "detect", IdempotencyKey: newKey(),
		})
		if refused.ok() {
			t.Errorf("%s spent the dragon's legendary action", p.name)
		}
	}
	// The positive controls: the master's answers and log hold each needle, and Ana reads the attack as a line.
	master := append(w.reads(w.master), text, strike)
	for _, word := range []string{"Wing Attack", "wing-attack", "legendary"} {
		found := false
		for _, r := range master {
			found = found || bytes.Contains(r.body, []byte(word))
		}
		if !found {
			t.Errorf("the master's answers do not hold %q: the needle is unreachable", word)
		}
	}
	var state *playv1.LegendaryState
	for _, c := range w.master.currentEncounter(w).GetCombatants() {
		if c.GetId() == dragon.GetId() {
			state = c.GetLegendary()
		}
	}
	if state.GetActionsLeft() != 0 {
		t.Errorf("the master reads the dragon with %v, want 0 actions left (2 and 1 spent)", state)
	}
	sawAttack := false
	for _, e := range w.logOfPerson(w.ana) {
		sawAttack = sawAttack || (e.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_ATTACK && e.GetActorId() == dragon.GetId())
	}
	if !sawAttack {
		t.Error("Ana's log has no attack line of the dragon: the control is missing")
	}
}

// resistanceWords are what no player reads of a Legendary Resistance: its name in both languages and the key.
// The master's answers hold the English ones (the positive control).
var resistanceWords = []string{"Legendary Resistance", "legendary-resistance", "Resistência Lendária"}

// onDragonsTurn passes the turns until the dragon is on turn.
func (w *world) onDragonsTurn(c *playv1.Combatant) {
	w.t.Helper()
	ctx := w.t.Context()
	for range 40 {
		cur := must(w.master.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
		if cur.GetCurrentCombatantId() == c.GetId() {
			w.encounter = cur
			return
		}
		must(w.master.combat.EndTurn(ctx, rq(&playv1.EndTurnRequest{CampaignId: w.campaign, EncounterId: cur.GetId(), IdempotencyKey: newKey(), ExpectedCombatantId: cur.GetCurrentCombatantId(), ExpectedRound: cur.GetRound(), DiscardPendingDamage: true})))
	}
	w.t.Fatalf("the turn of %s never came", c.GetLabel())
}

// Hold Person holds the dragon and its turn ends, which asks the master its Wisdom save. The master rolls a
// 1 and turns the failure into a success with Legendary Resistance. No player reads the trait, the failed
// roll, the count or the word in the combat, the log or the answers they hold: the line they read is a save
// that passed. The master reads them (the positive controls), and a player's own try to use the trait is
// refused.
func TestALegendaryResistanceIsTheMastersAlone(t *testing.T) {
	w := newWorld(t)
	for _, word := range resistanceWords {
		w.secrets.add(&canary{needle: word, kind: "legendary-resistance"})
	}
	var dragon *playv1.Combatant
	for _, c := range w.master.currentEncounter(w).GetCombatants() {
		if c.GetBestiaryCreatureKey() == dragonKey {
			dragon = c
		}
	}
	ctx := t.Context()
	visible := true
	must(w.master.lasting.AddLastingEffect(ctx, rq(&playv1.AddLastingEffectRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), IdempotencyKey: newKey(), CatalogKey: "spell:hold-person",
		TargetIds: []string{dragon.GetId()}, CasterId: w.combatant(w.toren).GetId(), PlayerVisible: &visible,
		Duration: &playv1.EffectDurationChoice{Kind: playv1.EffectDurationKind_EFFECT_DURATION_KIND_ROUNDS, Rounds: 10, AnchorCombatantId: dragon.GetId()},
	})))
	w.onDragonsTurn(dragon)
	must(w.master.combat.EndTurn(ctx, rq(&playv1.EndTurnRequest{
		CampaignId: w.campaign, EncounterId: w.encounter.GetId(), IdempotencyKey: newKey(), ExpectedCombatantId: dragon.GetId(), DiscardPendingDamage: true,
	})))
	enc := must(w.master.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
	var window string
	for _, rw := range enc.GetReactionWindows() {
		if rw.GetKind() == playv1.ReactionKind_REACTION_KIND_EFFECT_SAVE {
			window = rw.GetId()
		}
	}
	if window == "" {
		t.Fatal("the dragon's save is not asked")
	}
	save := func(p *person) reply {
		return p.call(playv1connect.LastingEffectServiceRollEffectSaveProcedure, &playv1.RollEffectSaveRequest{
			CampaignId: w.campaign, EncounterId: enc.GetId(), WindowId: window, IdempotencyKey: newKey(),
			Roll: &playv1.RollEffectSaveRequest_D20Face{D20Face: 1}, UseLegendaryResistance: true,
		})
	}
	for _, p := range []*person{w.ana, w.caio} {
		if refused := save(p); refused.ok() {
			t.Errorf("%s used the dragon's Legendary Resistance", p.name)
		}
	}
	roll := save(w.master)
	if !roll.ok() {
		t.Fatalf("the master's save: status %d %s", roll.status, roll.body)
	}
	for _, p := range []*person{w.ana, w.caio} {
		for _, r := range w.reads(p) {
			for _, f := range w.inspect(p, r, nil) {
				t.Errorf("%s: %s", p.name, f)
			}
		}
	}
	// The positive controls: the master's answer and log hold the choice and the word, and Ana, who sees the
	// dragon, reads the ordinary line of a save that passed.
	master := append(w.reads(w.master), roll)
	for _, word := range []string{"resisted", "resistedLeft"} {
		found := false
		for _, r := range master {
			found = found || bytes.Contains(r.body, []byte(word))
		}
		if !found {
			t.Errorf("the master's answers do not hold %q: the control is missing", word)
		}
	}
	passed := false
	for _, e := range w.logOfPerson(w.ana) {
		fx := e.GetEffect()
		passed = passed || (fx.GetChange() == playv1.CombatLogEffectChange_COMBAT_LOG_EFFECT_CHANGE_SAVED && fx.GetD20() == 0 && !fx.GetResisted())
	}
	if !passed {
		t.Error("Ana's log has no line of a save that passed: the control is missing")
	}
}

// Ana's Sacred Flame makes the adult red dragon fail its Dexterity save with a 1. The cast waits for the master
// in a window of its own, and the master turns the failure into a success with Legendary Resistance. No player
// reads the window, the offer, the failed roll, the count or the trait in the combat, the log, the answers they
// hold or the stream: Ana reads a save that passed, and the hold sends the players no hint of its own. A player's
// try to answer the window is refused as a window that does not exist. The master reads all of it (the positive
// controls).
func TestACastsLegendaryResistanceIsTheMastersAlone(t *testing.T) {
	w := newWorld(t)
	ctx := t.Context()
	m := w.master
	for _, word := range append([]string{"REACTION_KIND_LEGENDARY_RESISTANCE", "legendaryResistance", "Usar Resistência Lendária"}, resistanceWords...) {
		w.secrets.add(&canary{needle: word, kind: "legendary-resistance-window"})
	}
	pens, dragon := w.sacredFlameFight()
	// The dragon stands beside Toren too, so both players see the creatures around the cast.
	inSight := []string{"dragon-name", "seen-npc-name", "combatant-id"}
	w.scriptD20(1)

	var cast, answer, masterLog reply
	var window *playv1.ReactionWindow
	var asked, masterReads []reply
	after := map[*person][]reply{}
	var passed bool
	got := w.hiddenOnlyEvents(func() {
		cast = w.ana.call(playv1connect.CombatServiceCastSpellProcedure, &playv1.CastSpellRequest{
			CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CasterId: pens.GetId(), SpellKey: "spell:sacred-flame", IdempotencyKey: newKey(),
			Targets: []*playv1.SpellTarget{{CombatantId: dragon.GetId()}},
		})
		if !cast.ok() {
			t.Fatalf("Ana's cast: status %d %s", cast.status, cast.body)
		}
		enc := must(m.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
		for _, rw := range enc.GetReactionWindows() {
			if rw.GetKind() == playv1.ReactionKind_REACTION_KIND_LEGENDARY_RESISTANCE {
				window = rw
			}
		}
		if window == nil {
			t.Fatalf("the master reads %v, want the Legendary Resistance window", enc.GetReactionWindows())
		}
		// While it waits, no player reads it, and a player's answer is refused like an invented window's.
		asked = []reply{w.reads(w.ana)[0], w.reads(w.caio)[0]}
		for _, p := range []*person{w.ana, w.caio} {
			asks := p.call(playv1connect.CombatServiceAnswerReactionProcedure, &playv1.AnswerReactionRequest{
				CampaignId: w.campaign, EncounterId: w.encounter.GetId(), WindowId: window.GetId(), Answer: playv1.ReactionChoice_REACTION_CHOICE_USE,
				CreatureIds: []string{dragon.GetId()}, IdempotencyKey: newKey(),
			})
			fake := p.call(playv1connect.CombatServiceAnswerReactionProcedure, &playv1.AnswerReactionRequest{
				CampaignId: w.campaign, EncounterId: w.encounter.GetId(), WindowId: newKey(), Answer: playv1.ReactionChoice_REACTION_CHOICE_USE,
				CreatureIds: []string{dragon.GetId()}, IdempotencyKey: newKey(),
			})
			if asks.ok() || fake.ok() || asks.status != fake.status || !bytes.Equal(asks.body, fake.body) {
				t.Errorf("%s: a real window is refused with %d %s, an invented one with %d %s: they must read the same", p.name, asks.status, asks.body, fake.status, fake.body)
			}
		}
		answer = m.call(playv1connect.CombatServiceAnswerReactionProcedure, &playv1.AnswerReactionRequest{
			CampaignId: w.campaign, EncounterId: w.encounter.GetId(), WindowId: window.GetId(), Answer: playv1.ReactionChoice_REACTION_CHOICE_USE,
			CreatureIds: []string{dragon.GetId()}, IdempotencyKey: newKey(),
		})
		if !answer.ok() {
			t.Fatalf("the master's answer: status %d %s", answer.status, answer.body)
		}
		for _, p := range []*person{w.ana, w.caio} {
			after[p] = w.reads(p)
		}
		masterReads = w.reads(m)
		masterLog = m.call(playv1connect.CombatServiceListCombatLogProcedure, &playv1.ListCombatLogRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId()})
		for _, e := range w.logOfPerson(w.ana) {
			for _, tg := range e.GetSpell().GetTargets() {
				s := tg.GetSave()
				passed = passed || (s.GetOutcome() == playv1.SaveOutcome_SAVE_OUTCOME_SAVED && s.GetRoll() == nil && !s.GetResisted())
			}
		}
	})

	for _, p := range []*person{w.ana, w.caio} {
		reads := slices.Concat(after[p], asked)
		reads = append(reads, cast)
		for _, r := range reads {
			for _, f := range w.inspect(p, r, inSight) {
				t.Errorf("%s: %s", p.name, f)
			}
		}
	}
	// The stream: the master hears the window open and close; the players hear neither, and nothing of the hold.
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
			t.Errorf("%s heard %d openings and %d closings of the master's window", p.name, o, c)
		}
		for _, ev := range got[p.name] {
			if strings.Contains(ev, "reaction") {
				t.Errorf("%s heard %s: the hold sends the players no hint of its own", p.name, ev)
			}
		}
	}
	// The positive controls: the master's reads hold the offer, the choice and the count; Ana reads a save that passed.
	master := slices.Concat(masterReads, []reply{answer})
	for _, word := range []string{"resisted", "resistedLeft"} {
		found := false
		for _, r := range master {
			found = found || contains(r.body, word)
		}
		if !found {
			t.Errorf("the master's answers do not hold %q: the control is missing", word)
		}
	}
	if offer := protojson.Format(window); !strings.Contains(offer, "legendaryResistance") || !strings.Contains(offer, "usesLeft") {
		t.Errorf("the master's window holds %s: the offer is missing", offer)
	}
	if !contains(masterLog.body, "resisted") {
		t.Error("the master's log does not hold the choice: the control is missing")
	}
	if !passed {
		t.Error("Ana's log has no line of a save that passed: the control is missing")
	}
}

// sacredFlameFight makes Ana's Pensantus a cleric with Sacred Flame, puts her beside the adult red dragon and
// passes the turns until it is hers. It returns her combatant and the dragon's.
func (w *world) sacredFlameFight() (pens, dragon *playv1.Combatant) {
	w.t.Helper()
	ctx := w.t.Context()
	m := w.master
	// Ana's Pensantus is a cleric with Sacred Flame, next to the dragon, on her turn.
	sheet := w.fullSheet("class:cleric", "race:gnome", 1)
	sheet.GetFull().BaseScores.Wisdom = 16
	sheet.GetFull().CantripKeys = []string{"spell:sacred-flame"}
	must(m.characters.UpdateCharacter(ctx, rq(&charactersv1.UpdateCharacterRequest{
		CampaignId: w.campaign, CharacterId: w.pens.GetId(), Revision: w.pens.GetRevision(), Name: w.pens.GetName(), Sheet: sheet,
	})))
	for _, c := range m.currentEncounter(w).GetCombatants() {
		if c.GetBestiaryCreatureKey() == dragonKey {
			dragon = c
		}
	}
	pens = w.combatant(w.pens)
	w.placeForced(m, w.encounter.GetId(), placement{pens.GetId(), 12, 13}, placement{dragon.GetId(), 13, 13})
	for range 40 {
		cur := must(m.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
		if cur.GetCurrentCombatantId() == pens.GetId() {
			w.encounter = cur
			break
		}
		must(m.combat.EndTurn(ctx, rq(&playv1.EndTurnRequest{CampaignId: w.campaign, EncounterId: cur.GetId(), IdempotencyKey: newKey(), ExpectedCombatantId: cur.GetCurrentCombatantId(), ExpectedRound: cur.GetRound()})))
	}
	if w.encounter.GetCurrentCombatantId() != pens.GetId() {
		w.t.Fatal("Pensantus's turn never came")
	}
	return pens, dragon
}

// heldAnswer is what the master does with the window of a held cast.
type heldAnswer int

const (
	passWindow heldAnswer = iota
	useTrait
	removeDragon
)

// heldCastRead is everything a player can read of a Sacred Flame at the dragon that waits for the master and
// then lands: the combat and the log while it waits and after the answer, the answer to the cast itself and the
// stream hints, as the player's own text.
type heldCastRead struct {
	player map[string]string
	master []reply
	window *playv1.ReactionWindow
}

// heldCastRun plays Ana's Sacred Flame at a dragon with three uses left in a world of its own: the dragon rolls
// face, and the master answers the window: USE on the dragon, PASS, or takes the dragon out of the combat.
func heldCastRun(t *testing.T, face int, answer heldAnswer) heldCastRead {
	t.Helper()
	w := newWorld(t)
	ctx := t.Context()
	m := w.master
	pens, dragon := w.sacredFlameFight()
	w.scriptD20(face)
	players := []*person{w.ana, w.caio}
	parts := map[string][]reply{}
	out := heldCastRead{player: map[string]string{}}
	got := w.hiddenOnlyEvents(func() {
		cast := w.ana.call(playv1connect.CombatServiceCastSpellProcedure, &playv1.CastSpellRequest{
			CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CasterId: pens.GetId(), SpellKey: "spell:sacred-flame", IdempotencyKey: newKey(),
			Targets: []*playv1.SpellTarget{{CombatantId: dragon.GetId()}},
		})
		if !cast.ok() {
			t.Fatalf("Ana's cast: status %d %s", cast.status, cast.body)
		}
		parts[w.ana.name] = append(parts[w.ana.name], cast)
		enc := must(m.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
		for _, rw := range enc.GetReactionWindows() {
			if rw.GetKind() == playv1.ReactionKind_REACTION_KIND_LEGENDARY_RESISTANCE {
				out.window = rw
			}
		}
		for _, p := range players {
			parts[p.name] = append(parts[p.name], w.reads(p)...) // while it waits
		}
		if out.window == nil {
			t.Errorf("the master reads %v, want the Legendary Resistance window", enc.GetReactionWindows())
			return
		}
		if answer == removeDragon {
			must(m.combat.RemoveCombatant(ctx, rq(&playv1.RemoveCombatantRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: dragon.GetId(), IdempotencyKey: newKey()})))
			for _, p := range players {
				parts[p.name] = append(parts[p.name], w.reads(p)...) // after the dragon left
			}
			out.master = w.reads(m)
			return
		}
		req := &playv1.AnswerReactionRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), WindowId: out.window.GetId(), Answer: playv1.ReactionChoice_REACTION_CHOICE_PASS, IdempotencyKey: newKey()}
		if answer == useTrait {
			req.Answer, req.CreatureIds = playv1.ReactionChoice_REACTION_CHOICE_USE, []string{dragon.GetId()}
		}
		resp := m.call(playv1connect.CombatServiceAnswerReactionProcedure, req)
		if !resp.ok() {
			t.Fatalf("the master's answer: status %d %s", resp.status, resp.body)
		}
		for _, p := range players {
			parts[p.name] = append(parts[p.name], w.reads(p)...) // after the answer
		}
		out.master = slices.Concat(w.reads(m), []reply{resp})
	})
	for _, p := range players {
		var text strings.Builder
		for _, r := range parts[p.name] {
			text.Write(r.body)
			text.WriteString("\n")
		}
		text.WriteString(strings.Join(got[p.name], "\n"))
		out.player[p.name] = normalizeRead(text.String())
	}
	return out
}

var (
	readID   = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	readTime = regexp.MustCompile(`\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(\.\d+)?Z`)
)

// normalizeRead blanks the two things that differ between two worlds by nature: the ids (random UUIDs, named
// by the order they first appear in) and the timestamps. Nothing else is touched.
func normalizeRead(s string) string {
	s = readTime.ReplaceAllString(s, "<time>")
	seen := map[string]int{}
	return readID.ReplaceAllStringFunc(s, func(id string) string {
		if _, ok := seen[id]; !ok {
			seen[id] = len(seen) + 1
		}
		return "<id" + strconv.Itoa(seen[id]) + ">"
	})
}

// A dragon with a use left holds Ana's cast whatever it rolled, and the players read the same thing either way:
// the same wait, the same log, the same answer to the cast and the same stream hints, whether the save passed on
// its own or failed and the master turned it into a success. Only the ids and the timestamps are normalised.
// The master reads the two runs differently (the positive controls: a failed roll to offer and a use spent, or
// nothing to offer and nothing spent).
func TestAPassedAndAResistedSaveReadTheSameToThePlayers(t *testing.T) {
	var passed, resisted heldCastRead
	t.Run("the save passes", func(t *testing.T) { passed = heldCastRun(t, 20, passWindow) })
	t.Run("the save fails and the master uses the trait", func(t *testing.T) { resisted = heldCastRun(t, 1, useTrait) })
	for _, name := range []string{"Ana", "Caio"} {
		if passed.player[name] != resisted.player[name] {
			t.Errorf("%s reads a passed save and a resisted one differently:\n--- passed\n%s\n--- resisted\n%s", name, passed.player[name], resisted.player[name])
		}
		if !strings.Contains(passed.player[name], "Esperando o mestre") {
			t.Errorf("%s reads no wait in the run where the save passed: the control is missing", name)
		}
	}
	if !strings.Contains(passed.player["Ana"], "SAVE_OUTCOME_SAVED") {
		t.Error("Ana reads no save that passed: the control is missing")
	}
	if n := len(passed.window.GetLegendaryResistance().GetOffers()); n != 0 {
		t.Errorf("the master's window of the passed save offers %d saves, want none", n)
	}
	if n := len(resisted.window.GetLegendaryResistance().GetOffers()); n != 1 {
		t.Errorf("the master's window of the failed save offers %d saves, want one", n)
	}
	for name, run := range map[string]heldCastRead{"passed": passed, "resisted": resisted} {
		spent := false
		for _, r := range run.master {
			spent = spent || contains(r.body, `"resisted":true`)
		}
		if spent != (name == "resisted") {
			t.Errorf("the master's reads of the %s run show a use spent = %v", name, spent)
		}
	}
}

// The master takes the dragon out of the combat while the cast waits. The window and the wait end whatever the
// dragon rolled, so the players read the same thing after the removal for a passed save and for a failed one.
// Only the ids and the timestamps are normalised.
func TestARemovedDragonEndsThePassedAndTheFailedWaitAlike(t *testing.T) {
	var passed, failed heldCastRead
	t.Run("the save passes", func(t *testing.T) { passed = heldCastRun(t, 20, removeDragon) })
	t.Run("the save fails", func(t *testing.T) { failed = heldCastRun(t, 1, removeDragon) })
	for _, name := range []string{"Ana", "Caio"} {
		if passed.player[name] != failed.player[name] {
			t.Errorf("%s reads the removal after a passed save and after a failed one differently:\n--- passed\n%s\n--- failed\n%s", name, passed.player[name], failed.player[name])
		}
		if !strings.Contains(failed.player[name], "Esperando o mestre") {
			t.Errorf("%s reads no wait before the removal: the control is missing", name)
		}
	}
}
