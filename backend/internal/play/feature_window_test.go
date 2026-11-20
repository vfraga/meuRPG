package play

import (
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The feature window (#743 part 2, docs/architecture/play/reactions.md "The feature window"): a reaction feature whose use
// declares a trigger opens a REACTION window of kind FEATURE for its owner when an enemy starts its turn or moves within range.
// The names are invented. These tests need the database.

const kindFeature = playv1.ReactionKind_REACTION_KIND_FEATURE

// haloTable is a table where Pensantus has the reaction feature "Halo": 2 uses a combat, one enemy within 10 ft, triggered by an
// enemy that starts its turn or moves to within 10 ft of her.
func haloTable(t *testing.T, kinds ...rulesv1.FeatureTriggerKind) *featureTable {
	t.Helper()
	return haloTableOf(t, "2", kinds, nil)
}

const (
	whenItStarts = rulesv1.FeatureTriggerKind_FEATURE_TRIGGER_KIND_ENEMY_STARTS_TURN_WITHIN
	whenItMoves  = rulesv1.FeatureTriggerKind_FEATURE_TRIGGER_KIND_ENEMY_MOVES_WITHIN
)

// haloTableOf is haloTable with the uses of the combat, the moments (both when none) and the other features Pensantus has.
func haloTableOf(t *testing.T, uses string, kinds []rulesv1.FeatureTriggerKind, others []*rulesv1.TableFeature) *featureTable {
	t.Helper()
	if len(kinds) == 0 {
		kinds = []rulesv1.FeatureTriggerKind{whenItStarts, whenItMoves}
	}
	enemy := &rulesv1.FeatureTargets{Min: 1, Max: 1, Who: rulesv1.FeatureTargetWho_FEATURE_TARGET_WHO_ENEMY, RangeFt: 10}
	u := useOf(rulesv1.UseScope_USE_SCOPE_COMBAT, uses, enemy, nil)
	u.Trigger = &rulesv1.FeatureTrigger{Kinds: kinds, RangeFt: 10}
	return newFeatureTableOf(t, append([]*rulesv1.TableFeature{grantFeature("Halo", "reaction", "", u)}, others...))
}

// A goblin that starts its turn 10 ft from Pensantus opens a FEATURE window for her naming the goblin, and the turn waits for it.
func TestAnEnemyStartingItsTurnWithinRangeOpensTheFeatureWindow(t *testing.T) {
	t.Parallel()
	m := haloTable(t)
	e := m.fightHalo(t)
	e = m.passTo(t, e, "Goblin")
	w := m.windowOf(t, m.ana, kindFeature)
	if w == nil {
		t.Fatalf("Ana reads no FEATURE window when the goblin, 10 ft away, starts its turn: %v", m.windows(t, m.ana))
	}
	if w.GetReactorId() != m.id(t, "Pensantus") || !w.GetForYou() {
		t.Errorf("the window = reactor %q for_you %v, want Pensantus's and for Ana", w.GetReactorLabel(), w.GetForYou())
	}
	if got := w.GetTrigger().GetActorLabel(); got != "Goblin" {
		t.Errorf("the window names %q, want Goblin", got)
	}
	if got := w.GetTrigger().GetFeatureTrigger(); got != rulesv1.FeatureTriggerKind_FEATURE_TRIGGER_KIND_ENEMY_STARTS_TURN_WITHIN {
		t.Errorf("the window's moment = %v, want the start of the turn", got)
	}
	_, err := m.endTurn(t, m.master, e, true)
	wantBlockedBy(t, "EndTurn while the window waits", err, blockedReactionPending)
}

// fightHalo is the fight with the Capitão out of the range of Halo: he starts his turn in the same order as the goblin, whichever comes first.
func (m *featureTable) fightHalo(t *testing.T) *playv1.Encounter {
	t.Helper()
	e := m.fight(t)
	m.put(t, "Capitão Goblin", 9, 9)
	return e
}

// haloWindow is the FEATURE window the master reads, after the goblin's turn started 10 ft from Pensantus.
func (m *featureTable) haloWindow(t *testing.T) (*playv1.Encounter, *playv1.ReactionWindow) {
	t.Helper()
	e := m.passTo(t, m.fightHalo(t), "Goblin")
	w := m.windowOf(t, m.master, kindFeature)
	if w == nil {
		t.Fatalf("the master reads no FEATURE window: %v", m.windows(t, m.master))
	}
	return e, w
}

func (m *featureTable) pensantus(t *testing.T) *playv1.Combatant {
	t.Helper()
	return byLabel(t, m.get(t, m.master), "Pensantus")
}

// A goblin that moves from 35 ft to 10 ft of Pensantus on its turn opens the window at the end of its move, naming the move.
func TestAnEnemyMovingWithinRangeOpensTheFeatureWindow(t *testing.T) {
	t.Parallel()
	m := haloTable(t)
	e := m.fightHalo(t)
	m.put(t, "Goblin", 12, 5)
	e = m.passTo(t, e, "Goblin")
	if w := m.windowOf(t, m.ana, kindFeature); w != nil {
		t.Fatalf("a goblin 35 ft away starts its turn and opens %v", w)
	}
	m.moveTo(t, e, "Goblin", 7, 5)
	w := m.windowOf(t, m.ana, kindFeature)
	if w == nil {
		t.Fatalf("Ana reads no FEATURE window when the goblin moves to 10 ft: %v", m.windows(t, m.ana))
	}
	if got := w.GetTrigger().GetFeatureTrigger(); got != whenItMoves {
		t.Errorf("the window's moment = %v, want the move", got)
	}
	if w.GetTrigger().GetActorLabel() != "Goblin" || w.GetTrigger().GetActionNamePt() != "Halo" {
		t.Errorf("the window names %q and %q, want the Goblin and Halo", w.GetTrigger().GetActorLabel(), w.GetTrigger().GetActionNamePt())
	}
}

// What does not open the window: a goblin out of the range, a feature that waits for the other moment, a goblin that was near
// already and moves near, a reaction that is spent, the same side.
func TestTheFeatureWindowIsNotOpenedWhenTheTriggerDoesNotHold(t *testing.T) {
	t.Parallel()
	reflexo := grantFeature("Reflexo", "reaction", "", useOf(rulesv1.UseScope_USE_SCOPE_ROUND, "1", nil, &rulesv1.FeatureApplies{TempHp: "4"}))
	cases := []struct {
		name  string
		table func(t *testing.T) *featureTable
		run   func(t *testing.T, m *featureTable)
	}{
		{"the goblin starts its turn 15 ft away", func(t *testing.T) *featureTable { t.Helper(); return haloTable(t) }, func(t *testing.T, m *featureTable) {
			t.Helper()
			e := m.fightHalo(t)
			m.put(t, "Goblin", 8, 5)
			m.passTo(t, e, "Goblin")
		}},
		{"the feature waits for a move and the goblin starts its turn near", func(t *testing.T) *featureTable { t.Helper(); return haloTable(t, whenItMoves) }, func(t *testing.T, m *featureTable) {
			t.Helper()
			m.passTo(t, m.fightHalo(t), "Goblin")
		}},
		{"the feature waits for a turn and the goblin moves near", func(t *testing.T) *featureTable { t.Helper(); return haloTable(t, whenItStarts) }, func(t *testing.T, m *featureTable) {
			t.Helper()
			e := m.fightHalo(t)
			m.put(t, "Goblin", 12, 5)
			e = m.passTo(t, e, "Goblin")
			m.moveTo(t, e, "Goblin", 7, 5)
		}},
		{"the goblin was near and moves near", func(t *testing.T) *featureTable { t.Helper(); return haloTable(t, whenItMoves) }, func(t *testing.T, m *featureTable) {
			t.Helper()
			e := m.passTo(t, m.fightHalo(t), "Goblin")
			m.moveTo(t, e, "Goblin", 7, 4)
		}},
		{"the reaction is spent", func(t *testing.T) *featureTable {
			t.Helper()
			return haloTableOf(t, "2", nil, []*rulesv1.TableFeature{reflexo})
		}, func(t *testing.T, m *featureTable) {
			t.Helper()
			e := m.passTo(t, m.fightHalo(t), "Toren")
			m.mustUse(t, m.ana, e, "Pensantus", "Reflexo", nil)
			m.passTo(t, e, "Goblin")
		}},
		{"an ally of Pensantus starts its turn near", func(t *testing.T) *featureTable { t.Helper(); return haloTable(t) }, func(t *testing.T, m *featureTable) {
			t.Helper()
			m.passTo(t, m.fightHalo(t), "Toren")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := tc.table(t)
			tc.run(t, m)
			if w := m.windowOf(t, m.master, kindFeature); w != nil {
				t.Errorf("the master reads a FEATURE window: %v", w)
			}
		})
	}
}

// "Usar" runs the feature as a reaction on the goblin: the reaction and one use are spent, the log says the feature was used,
// the window is closed and the goblin's turn goes on.
func TestAnsweringTheFeatureWindowUseSpendsTheReaction(t *testing.T) {
	t.Parallel()
	m := haloTable(t)
	e, w := m.haloWindow(t)
	res, err := m.answerReaction(t, m.ana, e, w.GetId(), useAnswer(0))
	if err != nil {
		t.Fatalf("AnswerReaction(USE) error = %v", err)
	}
	if res.GetResult().GetKind() != kindFeature || !res.GetResult().GetUsed() {
		t.Errorf("the result = %v, want a used FEATURE", res.GetResult())
	}
	if !m.pensantus(t).GetReactionUsed() {
		t.Errorf("Pensantus's reaction is free after she used Halo")
	}
	if left := m.row(t, m.ana, m.get(t, m.ana), "Pensantus", "Halo").GetUseLeft(); left != 1 {
		t.Errorf("Halo's uses left = %d, want 1", left)
	}
	if m.windowOf(t, m.master, kindFeature) != nil {
		t.Errorf("the window stands after its answer")
	}
	if lines := logLinesOf(t, m.armed, m.master, e, playv1.CombatLogKind_COMBAT_LOG_KIND_FEATURE_USED); len(lines) != 1 || lines[0].GetActorLabel() != "Pensantus" {
		t.Errorf("the log has %v, want one FEATURE_USED of Pensantus", lines)
	}
	if _, err := m.endTurn(t, m.master, e, true); err != nil {
		t.Errorf("the goblin's turn waits after the answer: %v", err)
	}
}

// A feature takes no darts: a "Usar" that carries targets is refused as invalid, and the window, the reaction and the uses stay.
func TestAnsweringTheFeatureWindowWithTargetsIsRefused(t *testing.T) {
	t.Parallel()
	m := haloTable(t)
	e, w := m.haloWindow(t)
	_, err := m.answerReaction(t, m.ana, e, w.GetId(), func(r *playv1.AnswerReactionRequest) {
		useAnswer(0)(r)
		r.Targets = []*playv1.SpellTarget{darts(m.armed, t, "Goblin", 1)}
	})
	wantCode(t, "a feature window used with targets", err, connect.CodeInvalidArgument)
	if m.windowOf(t, m.master, kindFeature) == nil || m.pensantus(t).GetReactionUsed() || m.row(t, m.ana, m.get(t, m.ana), "Pensantus", "Halo").GetUseLeft() != 2 {
		t.Errorf("after the refusal: window open %v, reaction used %v, Halo uses left %d (want it open, free and 2)",
			m.windowOf(t, m.master, kindFeature) != nil, m.pensantus(t).GetReactionUsed(), m.row(t, m.ana, m.get(t, m.ana), "Pensantus", "Halo").GetUseLeft())
	}
}

// "Deixar passar" spends nothing: the reaction stays free, the uses stay, no line is written, the turn goes on.
func TestAnsweringTheFeatureWindowPassSpendsNothing(t *testing.T) {
	t.Parallel()
	m := haloTable(t)
	e, w := m.haloWindow(t)
	res, err := m.answerReaction(t, m.ana, e, w.GetId(), passAnswer)
	if err != nil {
		t.Fatalf("AnswerReaction(PASS) error = %v", err)
	}
	if res.GetResult().GetKind() != kindFeature || res.GetResult().GetUsed() {
		t.Errorf("the result = %v, want an unused FEATURE", res.GetResult())
	}
	if m.pensantus(t).GetReactionUsed() {
		t.Errorf("Pensantus's reaction is spent by a pass")
	}
	if left := m.row(t, m.ana, m.get(t, m.ana), "Pensantus", "Halo").GetUseLeft(); left != 2 {
		t.Errorf("Halo's uses left = %d, want 2", left)
	}
	if lines := logLinesOf(t, m.armed, m.master, e, playv1.CombatLogKind_COMBAT_LOG_KIND_FEATURE_USED); len(lines) != 0 {
		t.Errorf("the log has %v, want no FEATURE_USED", lines)
	}
	if m.windowOf(t, m.master, kindFeature) != nil {
		t.Errorf("the window stands after its pass")
	}
	if _, err := m.endTurn(t, m.master, e, true); err != nil {
		t.Errorf("the goblin's turn waits after the pass: %v", err)
	}
}

// The master answers a player's window as the player would; another player may not.
func TestTheMasterAnswersAPlayersFeatureWindowAndAnotherPlayerCannot(t *testing.T) {
	t.Parallel()
	m := haloTable(t)
	e, w := m.haloWindow(t)
	_, err := m.answerReaction(t, m.caio, e, w.GetId(), useAnswer(0))
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("Caio's answer to Ana's window: error = %v, want permission denied", err)
	}
	res, err := m.answerReaction(t, m.master, e, w.GetId(), useAnswer(0))
	if err != nil {
		t.Fatalf("the master's USE error = %v", err)
	}
	if !res.GetResult().GetByMaster() || !m.pensantus(t).GetReactionUsed() {
		t.Errorf("the master's answer: by master %v, reaction used %v, want both", res.GetResult().GetByMaster(), m.pensantus(t).GetReactionUsed())
	}
}

// Without a 30 s answer the window waits: the turn of the goblin cannot end, and Ana's feature cannot be used twice.
func TestTheFeatureWindowHoldsTheTurn(t *testing.T) {
	t.Parallel()
	m := haloTable(t)
	e, _ := m.haloWindow(t)
	_, err := m.endTurn(t, m.master, e, true)
	wantBlockedBy(t, "EndTurn while the feature window waits", err, blockedReactionPending)
}

// Undo works until the window is answered, and takes the answer back: the move that opened the window takes it away, a use
// opens it again with the reaction and the use given back, a pass opens it again.
func TestUndoAndTheFeatureWindow(t *testing.T) {
	t.Parallel()
	t.Run("the move that opened it", func(t *testing.T) {
		t.Parallel()
		m := haloTable(t)
		e := m.fightHalo(t)
		m.put(t, "Goblin", 12, 5)
		e = m.passTo(t, e, "Goblin")
		m.moveTo(t, e, "Goblin", 7, 5)
		m.undoLast(t, e)
		if m.windowOf(t, m.master, kindFeature) != nil {
			t.Errorf("the window stands after the move was undone")
		}
	})
	t.Run("a use", func(t *testing.T) {
		t.Parallel()
		m := haloTable(t)
		e, w := m.haloWindow(t)
		m.mustAnswer(t, m.ana, e, w.GetId(), useAnswer(0))
		m.undoLast(t, e)
		if m.windowOf(t, m.ana, kindFeature) == nil {
			t.Errorf("the window is not open again after the use was undone")
		}
		if m.pensantus(t).GetReactionUsed() {
			t.Errorf("the reaction is spent after the use was undone")
		}
		if left := m.row(t, m.ana, m.get(t, m.ana), "Pensantus", "Halo").GetUseLeft(); left != 2 {
			t.Errorf("Halo's uses left = %d after the undo, want 2", left)
		}
	})
	t.Run("a pass", func(t *testing.T) {
		t.Parallel()
		m := haloTable(t)
		e, w := m.haloWindow(t)
		m.mustAnswer(t, m.ana, e, w.GetId(), passAnswer)
		m.undoLast(t, e)
		if m.windowOf(t, m.ana, kindFeature) == nil {
			t.Errorf("the window is not open again after the pass was undone")
		}
	})
}

// A feature whose use has spent its uses does not open the window again.
func TestTheFeatureWindowIsNotOpenedWhenNoUseIsLeft(t *testing.T) {
	t.Parallel()
	m := haloTableOf(t, "1", nil, nil)
	e, w := m.haloWindow(t)
	m.mustAnswer(t, m.ana, e, w.GetId(), useAnswer(0))
	m.passTo(t, e, "Pensantus")
	m.passTo(t, e, "Goblin")
	if w := m.windowOf(t, m.master, kindFeature); w != nil {
		t.Errorf("the window opens with no use left: %v", w)
	}
}

// A trigger of a kind nobody knows is refused when the table entry is saved.
func TestAnUnknownFeatureTriggerIsRefusedAtSave(t *testing.T) {
	t.Parallel()
	m := newFeatureTable(t)
	u := useOf(rulesv1.UseScope_USE_SCOPE_COMBAT, "1", nil, nil)
	u.Trigger = &rulesv1.FeatureTrigger{Kinds: []rulesv1.FeatureTriggerKind{rulesv1.FeatureTriggerKind(99)}, RangeFt: 10}
	sub := &rulesv1.TableSubclass{
		NamePt: "Mestre dos Gatilhos", ClassKey: "class:fighter", DescPt: []string{"Gatilhos."},
		Levels: []*rulesv1.TableSubclassLevel{{Level: 3, Features: []*rulesv1.TableFeature{grantFeature("Gatilho", "reaction", "", u)}}},
	}
	_, err := m.master.table.CreateTableEntry(t.Context(), connect.NewRequest(&rulesv1.CreateTableEntryRequest{
		CampaignId: m.campaignID, Body: &rulesv1.CreateTableEntryRequest_TableSubclass{TableSubclass: sub},
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("CreateTableEntry with an unknown trigger: error = %v, want invalid argument", err)
	}
}

// A goblin the master hid opens the window for the master alone: Ana reads no window and no word of it, Caio reads that the
// master is waited for, nobody but the master may answer, and the master's answer works (RN-10).
func TestAHiddenCreaturesFeatureWindowIsTheMastersAlone(t *testing.T) {
	t.Parallel()
	m := haloTable(t)
	e := m.fightHalo(t)
	hideAgain(t, m.armed, e, "Goblin")
	e = m.passTo(t, e, "Goblin")
	w := m.windowOf(t, m.master, kindFeature)
	if w == nil || w.GetTrigger().GetActorLabel() != "Goblin" {
		t.Fatalf("the master reads %v, want Pensantus's FEATURE window naming the goblin", w)
	}
	for _, u := range []*user{m.ana, m.caio} {
		if got := m.windowOf(t, u, kindFeature); got != nil {
			t.Errorf("a player reads %v of the hidden creature's window", got)
		}
		if wait := m.get(t, u).GetReactionWait(); wait.GetTitlePt() != "Esperando o mestre" {
			t.Errorf("a player reads the wait %q, want %q", wait.GetTitlePt(), "Esperando o mestre")
		}
	}
	_, err := m.answerReaction(t, m.ana, e, w.GetId(), passAnswer)
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("Ana's answer to the hidden creature's window: error = %v, want permission denied", err)
	}
	if lines := logLinesOf(t, m.armed, m.ana, e, playv1.CombatLogKind_COMBAT_LOG_KIND_FEATURE_USED); len(lines) != 0 {
		t.Errorf("Ana reads %v before the answer", lines)
	}
	if _, err := m.answerReaction(t, m.master, e, w.GetId(), useAnswer(0)); err != nil {
		t.Fatalf("the master's USE of the hidden creature's window: %v", err)
	}
	if lines := logLinesOf(t, m.armed, m.ana, e, playv1.CombatLogKind_COMBAT_LOG_KIND_FEATURE_USED); len(lines) != 0 {
		t.Errorf("Ana reads the use against a hidden creature: %v", lines)
	}
	if lines := logLinesOf(t, m.armed, m.master, e, playv1.CombatLogKind_COMBAT_LOG_KIND_FEATURE_USED); len(lines) != 1 {
		t.Errorf("the master reads %d lines of the use, want 1", len(lines))
	}
}

// glowTable is a table where Pensantus has "Brilho", a reaction that is aimed at no creature (it gives her temporary hit points):
// 2 uses a combat, triggered by an enemy that starts its turn or moves to within 10 ft of her.
func glowTable(t *testing.T) *featureTable {
	t.Helper()
	u := useOf(rulesv1.UseScope_USE_SCOPE_COMBAT, "2", nil, &rulesv1.FeatureApplies{TempHp: "4"})
	u.Trigger = &rulesv1.FeatureTrigger{Kinds: []rulesv1.FeatureTriggerKind{whenItStarts, whenItMoves}, RangeFt: 10}
	return newFeatureTableOf(t, []*rulesv1.TableFeature{grantFeature("Brilho", "reaction", "", u)})
}

// A feature aimed at no creature has no hidden target to make its event secret: answering the window of a hidden goblin with
// USE is secret all the same, so no player hears that a creature came near. The owner reads that the use is spent in its own
// character (the reaction, the uses left), not in the log.
func TestAnUnaimedFeatureUsedOnAHiddenCreaturesWindowIsSecret(t *testing.T) {
	t.Parallel()
	m := glowTable(t)
	e := m.fightHalo(t)
	hideAgain(t, m.armed, e, "Goblin")
	e = m.passTo(t, e, "Goblin")
	w := m.windowOf(t, m.master, kindFeature)
	if w == nil {
		t.Fatalf("the master reads no FEATURE window of the hidden goblin: %v", m.windows(t, m.master))
	}
	if _, err := m.answerReaction(t, m.master, e, w.GetId(), useAnswer(0)); err != nil {
		t.Fatalf("the master's USE: %v", err)
	}
	for name, u := range map[string]*user{"Ana (the owner)": m.ana, "Caio": m.caio} {
		if lines := logLinesOf(t, m.armed, u, e, playv1.CombatLogKind_COMBAT_LOG_KIND_FEATURE_USED); len(lines) != 0 {
			t.Errorf("%s reads %v: the use came out in the log of a creature the master hid", name, lines)
		}
	}
	if lines := logLinesOf(t, m.armed, m.master, e, playv1.CombatLogKind_COMBAT_LOG_KIND_FEATURE_USED); len(lines) != 1 {
		t.Errorf("the master reads %d lines of the use, want 1", len(lines))
	}
	if !m.pensantus(t).GetReactionUsed() {
		t.Errorf("Pensantus's reaction is free after the use")
	}
	if left := m.row(t, m.ana, m.get(t, m.ana), "Pensantus", "Brilho").GetUseLeft(); left != 1 {
		t.Errorf("Ana reads %d uses of Brilho left, want 1: the owner learns of the spent use there", left)
	}
}
