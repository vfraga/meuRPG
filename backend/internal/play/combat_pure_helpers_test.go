package play

import (
	"strings"
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
	"github.com/PuraFome/meuRPG/backend/internal/rules/reaction"
)

// Characterization of the pure pieces of the combat: what an attack costs,
// the Portuguese sentences of the attack-roll sources, the free square next to an owner, the
// line a viewer reads of the windows it waits for, and the Cutting Words line. None needs a
// database: each is frozen as the code answered before it was split.

func strp(s string) *string { return &s }

func TestAttackEconomyFreezesWhatEachAttackCosts(t *testing.T) {
	sword := link.Attack{Key: "sword", Melee: true}
	dagger := link.Attack{Key: "dagger", Melee: true, Light: true}
	second := link.Attack{Key: "second", Melee: true, Light: true}
	fist := link.Attack{Key: "fist", Melee: true, Unarmed: true, MartialArts: true}
	bolt := link.Attack{Key: "bolt", Spell: true, Beams: 3}
	cantrip := link.Attack{Key: "ray", Spell: true}
	sheet := func(per int, atks ...link.Attack) link.Sheet { return link.Sheet{AttacksPerAction: per, Attacks: atks} }
	cases := []struct {
		name     string
		who      playdb.Combatant
		sheet    link.Sheet
		attack   link.Attack
		frenzy   bool
		wantKind combat.BonusKind
		wantErr  string // the message of the refusal, "" for none
	}{
		{"a cantrip takes the free action", playdb.Combatant{}, sheet(1, cantrip), cantrip, false, combat.BonusNone, ""},
		{"a cantrip after the action was used is refused", playdb.Combatant{ActionUsed: true, AttacksMade: 1, ActionAttackKey: strp("sword")}, sheet(1, sword, cantrip), cantrip, false, combat.BonusNone, "the action of this turn is used"},
		{"the next beam of the cast cantrip is free", playdb.Combatant{ActionUsed: true, AttacksMade: 1, ActionAttackKey: strp("bolt")}, sheet(1, bolt), bolt, false, combat.BonusNone, ""},
		{"a beam past the last is refused", playdb.Combatant{ActionUsed: true, AttacksMade: 3, ActionAttackKey: strp("bolt")}, sheet(1, bolt), bolt, false, combat.BonusNone, "the action of this turn is used"},
		{"a weapon takes the free action", playdb.Combatant{}, sheet(1, sword), sword, false, combat.BonusNone, ""},
		{"the next attack of the Attack action", playdb.Combatant{ActionUsed: true, AttacksMade: 1, ActionAttackKey: strp("sword")}, sheet(2, sword), sword, false, combat.BonusNone, ""},
		{"a spent action with one attack is refused", playdb.Combatant{ActionUsed: true, AttacksMade: 1, ActionAttackKey: strp("sword")}, sheet(1, sword), sword, false, combat.BonusNone, "the action of this turn is used"},
		{"all the attacks of the Attack action made", playdb.Combatant{ActionUsed: true, AttacksMade: 2, ActionAttackKey: strp("sword")}, sheet(2, sword), sword, false, combat.BonusNone, "the Attack action made all its attacks"},
		{"a weapon after a cantrip took the action", playdb.Combatant{ActionUsed: true, AttacksMade: 1, ActionAttackKey: strp("ray")}, sheet(2, cantrip, sword), sword, false, combat.BonusNone, "the action of this turn is used"},
		{"two-weapon fighting with the bonus action free", playdb.Combatant{ActionUsed: true, AttacksMade: 1, ActionAttackKey: strp("dagger")}, sheet(1, dagger, second), second, false, combat.BonusTwoWeapon, ""},
		{"two-weapon fighting with the bonus action used", playdb.Combatant{ActionUsed: true, AttacksMade: 1, BonusActionUsed: true, ActionAttackKey: strp("dagger")}, sheet(1, dagger, second), second, false, combat.BonusNone, "the bonus action of this turn is used"},
		{"a martial arts strike after the Attack action", playdb.Combatant{ActionUsed: true, AttacksMade: 1, ActionAttackKey: strp("fist")}, sheet(1, fist), fist, false, combat.BonusMartialArts, ""},
		{"a flurry strike does not need the bonus action", playdb.Combatant{ActionUsed: true, AttacksMade: 1, BonusActionUsed: true, BonusAttacksLeft: 1, ActionAttackKey: strp("fist")}, sheet(1, fist), fist, false, combat.BonusFlurry, ""},
		{"a frenzy attack follows no Attack action", playdb.Combatant{ActionUsed: true, AttacksMade: 1, ActionAttackKey: strp("sword")}, sheet(1, sword), sword, true, combat.BonusFrenzy, ""},
		{"a frenzy attack with the bonus action used", playdb.Combatant{ActionUsed: true, AttacksMade: 1, BonusActionUsed: true, ActionAttackKey: strp("sword")}, sheet(1, sword), sword, true, combat.BonusNone, "the bonus action of this turn is used"},
	}
	// reasons are the blocked reasons a refusal carries, for the rows that name one.
	reasons := map[string]playv1.EncounterBlockedReason{
		"a spent action with one attack is refused":      playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ACTION_USED,
		"all the attacks of the Attack action made":      playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ATTACKS_USED,
		"two-weapon fighting with the bonus action used": playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_BONUS_ACTION_USED,
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			kind, err := attackEconomy(c.who, c.sheet, c.attack, c.frenzy)
			if want, ok := reasons[c.name]; ok {
				if got := encounterBlocked(t, err).GetReason(); got != want {
					t.Errorf("reason = %v, want %v", got, want)
				}
			}
			if kind != c.wantKind {
				t.Errorf("kind = %v, want %v", kind, c.wantKind)
			}
			switch {
			case c.wantErr == "" && err != nil:
				t.Errorf("err = %v, want none", err)
			case c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)):
				t.Errorf("err = %v, want %q", err, c.wantErr)
			}
		})
	}
}

func TestAdvantageSentencesFreezeEachSource(t *testing.T) {
	names := func(key string) string { return "<" + key + ">" }
	third := textContext{names: names, target: "Orc", allyLabel: "Lobo", helper: "Mira"}
	self := textContext{self: true, names: names, target: "Orc", allyLabel: "Lobo", helper: "Mira"}
	bare := textContext{names: names}
	prone := "condition:prone"
	cases := []struct {
		name string
		ctx  textContext
		src  combat.Source
		want string
	}{
		{"prone target, near", third, combat.Source{Kind: combat.SourceProneTarget, Effect: combat.ModeAdvantage, Condition: prone}, "Alvo <condition:prone> a 1,5 m: vantagem"},
		{"prone target, far", third, combat.Source{Kind: combat.SourceProneTarget, Effect: combat.ModeDisadvantage, Condition: prone}, "Alvo <condition:prone> a mais de 1,5 m: desvantagem"},
		{"prone attacker, third person", third, combat.Source{Kind: combat.SourceProneAttacker, Condition: prone}, "O atacante está <condition:prone>: desvantagem"},
		{"restrained attacker, reader rolls", self, combat.Source{Kind: combat.SourceRestrainedAttacker, Condition: "condition:restrained"}, "Você está <condition:restrained>: desvantagem"},
		{"blinded attacker", third, combat.Source{Kind: combat.SourceBlindedAttacker, Condition: "condition:blinded"}, "O atacante está <condition:blinded>: desvantagem"},
		{"poisoned attacker", third, combat.Source{Kind: combat.SourcePoisonedAttacker, Condition: "condition:poisoned"}, "O atacante está <condition:poisoned>: desvantagem"},
		{"frightened attacker", third, combat.Source{Kind: combat.SourceFrightenedAttacker, Condition: "condition:frightened"}, "O atacante está <condition:frightened>: desvantagem enquanto a fonte do medo está à vista"},
		{"invisible attacker", self, combat.Source{Kind: combat.SourceInvisibleAttacker, Condition: "condition:invisible"}, "Você está <condition:invisible>: vantagem"},
		{"restrained target", third, combat.Source{Kind: combat.SourceRestrainedTarget, Condition: "condition:restrained"}, "Alvo <condition:restrained>: vantagem"},
		{"blinded target", third, combat.Source{Kind: combat.SourceBlindedTarget, Condition: "condition:blinded"}, "Alvo <condition:blinded>: vantagem"},
		{"stunned target", third, combat.Source{Kind: combat.SourceStunnedTarget, Condition: "condition:stunned"}, "Alvo <condition:stunned>: vantagem"},
		{"paralyzed target", third, combat.Source{Kind: combat.SourceParalyzedTarget, Condition: "condition:paralyzed"}, "Alvo <condition:paralyzed>: vantagem"},
		{"unconscious target", third, combat.Source{Kind: combat.SourceUnconsciousTarget, Condition: "condition:unconscious"}, "Alvo <condition:unconscious>: vantagem"},
		{"petrified target", third, combat.Source{Kind: combat.SourcePetrifiedTarget, Condition: "condition:petrified"}, "Alvo <condition:petrified>: vantagem"},
		{"invisible target", third, combat.Source{Kind: combat.SourceInvisibleTarget, Condition: "condition:invisible"}, "Alvo <condition:invisible>: desvantagem"},
		{"reckless attacker", third, combat.Source{Kind: combat.SourceRecklessAttacker}, "<feature:reckless-attack>: vantagem nos ataques corpo a corpo com Força neste turno"},
		{"reckless target", third, combat.Source{Kind: combat.SourceRecklessTarget}, "Alvo em <feature:reckless-attack>: vantagem nos ataques contra ele"},
		{"dodging target", third, combat.Source{Kind: combat.SourceDodgingTarget}, "Alvo Esquivando e vendo o atacante: desvantagem"},
		{"magic resistance", self, combat.Source{Kind: combat.SourceMagicResistance}, "Resistência à Magia: vantagem em testes de resistência contra magias e outros efeitos mágicos"},
		{"sunlight attack of a Sensitivity", third, combat.Source{Kind: combat.SourceSunlightAttack, Condition: combat.TraitSunlightSensitivity}, "Luz do sol (Sensibilidade à Luz do Sol)"},
		{"sunlight attack of a Weakness", third, combat.Source{Kind: combat.SourceSunlightAttack, Condition: combat.TraitSunlightWeakness}, "Luz do sol (Fraqueza à Luz do Sol)"},
		{"the Sensitivity's Perception names the sight", third, combat.Source{Kind: combat.SourceSunlightPerception, Condition: combat.TraitSunlightSensitivity}, "Luz do sol, nos testes que dependem da visão (Sensibilidade à Luz do Sol)"},
		{"the Weakness's checks", third, combat.Source{Kind: combat.SourceSunlightPerception, Condition: combat.TraitSunlightWeakness}, "Luz do sol (Fraqueza à Luz do Sol)"},
		{"sunlight save", third, combat.Source{Kind: combat.SourceSunlightSave, Condition: combat.TraitSunlightWeakness}, "Luz do sol (Fraqueza à Luz do Sol)"},
		{"pack tactics names the ally to the master", third, combat.Source{Kind: combat.SourcePackTactics}, "Táticas de Matilha: um aliado (Lobo) está a 1,5 m de Orc e não está incapacitado"},
		{"pack tactics hides the ally from the roller", self, combat.Source{Kind: combat.SourcePackTactics}, "Táticas de Matilha: um aliado está a 1,5 m do alvo e não está incapacitado"},
		{"pack tactics without an ally label", bare, combat.Source{Kind: combat.SourcePackTactics}, "Táticas de Matilha: um aliado está a 1,5 m do alvo e não está incapacitado"},
		{"help names the helper", third, combat.Source{Kind: combat.SourceHelp}, "Ajuda de Mira: vantagem"},
		{"help of an unnamed ally", bare, combat.Source{Kind: combat.SourceHelp}, "Ajuda de um aliado: vantagem"},
		{"unseen attacker", third, combat.Source{Kind: combat.SourceUnseenAttacker}, "Atacante que o alvo não vê: vantagem"},
		{"unseen target", third, combat.Source{Kind: combat.SourceUnseenTarget}, "Alvo que o atacante não vê: desvantagem"},
		{"long range", third, combat.Source{Kind: combat.SourceLongRange}, "Além do alcance normal: desvantagem"},
		{"hostile nearby", third, combat.Source{Kind: combat.SourceHostileNearby}, "Um inimigo a 1,5 m de quem ataca à distância: desvantagem"},
		{"effect attack names the effect", third, combat.Source{Kind: combat.SourceEffectAttack, Condition: "spell:vicious-mockery"}, "<spell:vicious-mockery>: desvantagem neste ataque"},
		{"a save source falls to the other sentences", third, combat.Source{Kind: combat.SourceDodgingSave}, "Esquivando: vantagem em testes de resistência de Destreza"},
		{"a check source falls to the other sentences", third, combat.Source{Kind: combat.SourcePerceptionDim}, "Você vê o chão em penumbra: desvantagem na Percepção"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.ctx.sentence(c.src); got != c.want {
				t.Errorf("sentence = %q, want %q", got, c.want)
			}
		})
	}
}

func TestSquareNearFreezesTheSearchOrder(t *testing.T) {
	enc := playdb.Encounter{GridColumns: 6, GridRows: 6}
	at := func(col, row int32) playdb.Combatant { return playdb.Combatant{GridCol: &col, GridRow: &row} }
	defeated := at(2, 2)
	defeated.Defeated = true
	unplaced := playdb.Combatant{}
	cases := []struct {
		name         string
		enc          playdb.Encounter
		all          []playdb.Combatant
		col, row     int32
		wantC, wantR int32
		wantOK       bool
	}{
		{"the first ring starts at the top left", enc, []playdb.Combatant{at(3, 3)}, 3, 3, 2, 2, true},
		{"a taken square is skipped, row by row", enc, []playdb.Combatant{at(3, 3), at(2, 2)}, 3, 3, 3, 2, true},
		{"a defeated combatant does not hold its square", enc, []playdb.Combatant{at(3, 3), defeated}, 3, 3, 2, 2, true},
		{"an unplaced combatant holds none", enc, []playdb.Combatant{at(3, 3), unplaced}, 3, 3, 2, 2, true},
		{"the grid edge pushes the search on", enc, []playdb.Combatant{at(0, 0)}, 0, 0, 1, 0, true},
		{"a full first ring goes to the second", enc, ringAround(3, 3, 1), 3, 3, 1, 1, true},
		{"no square within four", playdb.Encounter{GridColumns: 1, GridRows: 1}, []playdb.Combatant{at(0, 0)}, 0, 0, 0, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			col, row, ok := squareNear(c.enc, c.all, c.col, c.row)
			if col != c.wantC || row != c.wantR || ok != c.wantOK {
				t.Errorf("squareNear = (%d, %d, %v), want (%d, %d, %v)", col, row, ok, c.wantC, c.wantR, c.wantOK)
			}
		})
	}
}

// ringAround is the combatants standing on every square of the ring around (col, row), and on it.
func ringAround(col, row, ring int32) []playdb.Combatant {
	var out []playdb.Combatant
	for r := row - ring; r <= row+ring; r++ {
		for c := col - ring; c <= col+ring; c++ {
			out = append(out, playdb.Combatant{GridCol: &c, GridRow: &r})
		}
	}
	return out
}

func TestWaitOfFreezesTheLineEachViewerReads(t *testing.T) {
	player := func(label string, hidden bool) *playdb.Combatant {
		return &playdb.Combatant{ID: label, Label: label, Kind: kindPlayer, Hidden: hidden}
	}
	npc := &playdb.Combatant{ID: "npc", Label: "Orc", Kind: kindNPC}
	master := combatViewer{master: true}
	reader := combatViewer{userID: "u"}
	cases := []struct {
		name    string
		v       combatViewer
		kind    reaction.Kind
		reactor *playdb.Combatant
		answers bool
		want    reaction.Wait
	}{
		{"a contest the reader answers waits for nobody", reader, reaction.Contest, player("Sálvia", false), true, reaction.Wait{}},
		{"a contest with no reactor the reader answers", reader, reaction.Contest, nil, true, reaction.Wait{}},
		{"a contest names the seen player", reader, reaction.Contest, player("Sálvia", false), false, reaction.Wait{Contesters: []string{"Sálvia"}}},
		{"a contest on an unseen player is the master's", reader, reaction.Contest, player("Sálvia", true), false, reaction.Wait{Master: true}},
		{"a contest on an NPC is the master's", reader, reaction.Contest, npc, false, reaction.Wait{Master: true}},
		{"a contest with no reactor is the master's", reader, reaction.Contest, nil, false, reaction.Wait{Master: true}},
		{"the master reads a player's reaction", master, reaction.UncannyDodgeKind, player("Sálvia", false), false, reaction.Wait{Reactors: []string{"Sálvia"}}},
		{"the master reads a player's concentration save", master, reaction.Concentration, player("Sálvia", false), false, reaction.Wait{Savers: []string{"Sálvia"}}},
		{"the master reads an NPC's reaction as his own", master, reaction.Shield, npc, true, reaction.Wait{Self: true}},
		{"the master reads a window with no reactor as his own", master, reaction.MasterCheck, nil, true, reaction.Wait{Self: true}},
		{"a player answering reads nothing", reader, reaction.Shield, player("Sálvia", false), true, reaction.Wait{}},
		{"a player names the seen reactor", reader, reaction.Shield, player("Sálvia", false), false, reaction.Wait{Reactors: []string{"Sálvia"}}},
		{"a player names the seen saver", reader, reaction.Concentration, player("Sálvia", false), false, reaction.Wait{Savers: []string{"Sálvia"}}},
		{"a player never names an unseen reactor", reader, reaction.Shield, player("Sálvia", true), false, reaction.Wait{Master: true}},
		{"a player reads an NPC's reaction as the master's", reader, reaction.Shield, npc, false, reaction.Wait{Master: true}},
		{"a player reads a window with no reactor as the master's", reader, reaction.MasterCheck, nil, false, reaction.Wait{Master: true}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			wv := &windowView{v: c.v}
			var got reaction.Wait
			wv.waitOf(playdb.ReactionWindow{Kind: string(c.kind)}, c.reactor, c.answers, &got)
			if got.Self != c.want.Self || got.Master != c.want.Master ||
				strings.Join(got.Reactors, ",") != strings.Join(c.want.Reactors, ",") ||
				strings.Join(got.Savers, ",") != strings.Join(c.want.Savers, ",") ||
				strings.Join(got.Contesters, ",") != strings.Join(c.want.Contesters, ",") {
				t.Errorf("wait = %+v, want %+v", got, c.want)
			}
		})
	}
}

func TestCuttingWordsLineFreezesWhatEachViewerReads(t *testing.T) {
	base := func(master bool, re reactionEvent, other playdb.Combatant, seesOther bool) *reactionFacts {
		return &reactionFacts{
			re: &re, v: combatViewer{master: master}, other: other, seesOther: seesOther, seesReactor: true,
			who: "Bardo", by: " (respondido pelo mestre)",
		}
	}
	orc := playdb.Combatant{Label: "Orc"}
	cases := []struct {
		name   string
		f      *reactionFacts
		want   string
		wantOK bool
	}{
		{"unused says nothing", base(true, reactionEvent{}, orc, true), "", false},
		{"a reactor the viewer does not see says nothing", func() *reactionFacts {
			f := base(true, reactionEvent{Used: true}, orc, true)
			f.seesReactor = false
			return f
		}(), "", false},
		{"ineffective, for the master", base(true, reactionEvent{Used: true, Ineffect: true, DieSides: 6, Die: 4}, orc, true), "Bardo usou Palavras de Interrupção: d6 (4), sem efeito: Orc não ouve o bardo ou é imune a enfeitiçar.", true},
		{"ineffective, for the master, nobody named", base(true, reactionEvent{Used: true, Ineffect: true, DieSides: 6, Die: 4}, playdb.Combatant{}, true), "Bardo usou Palavras de Interrupção: d6 (4), sem efeito: o alvo não ouve o bardo ou é imune a enfeitiçar.", true},
		{"ineffective, for a player", base(false, reactionEvent{Used: true, Ineffect: true}, orc, true), "Bardo usou Palavras de Interrupção: sem efeito. (respondido pelo mestre)", true},
		{"an attack, for the master", base(true, reactionEvent{Used: true, Roll: "attack", DieSides: 6, Die: 3, Before: 17, After: 14, AC: 15, Outcome: outcomeMiss}, orc, true), "Bardo usou Palavras de Interrupção: d6 (3) subtraído do ataque de Orc: 17 → 14 contra CA 15: errou. (respondido pelo mestre)", true},
		{"a roll that says no kind is an attack", base(true, reactionEvent{Used: true, DieSides: 8, Die: 8, Before: 20, After: 12, AC: 13, Outcome: outcomeHit}, orc, false), "Bardo usou Palavras de Interrupção: d8 (8) subtraído do ataque: 20 → 12 contra CA 13: acertou. (respondido pelo mestre)", true},
		{"a damage roll, for the master", base(true, reactionEvent{Used: true, Roll: "damage", DieSides: 6, Die: 2, Before: 9, After: 7}, orc, true), "Bardo usou Palavras de Interrupção: d6 (2) subtraído da jogada de dano de Orc: 9 → 7. (respondido pelo mestre)", true},
		{"a test, for the master", base(true, reactionEvent{Used: true, Roll: "test", DieSides: 6, Die: 5, Before: 15, After: 10}, orc, true), "Bardo usou Palavras de Interrupção: d6 (5) subtraído do teste de Orc: 15 → 10. (respondido pelo mestre)", true},
		{"an attack that still hit, for a player", base(false, reactionEvent{Used: true, Roll: "attack", Outcome: outcomeCrit}, orc, true), "Bardo usou Palavras de Interrupção: o ataque de Orc ainda acertou. (respondido pelo mestre)", true},
		{"an attack that missed, for a player", base(false, reactionEvent{Used: true, Outcome: outcomeMiss}, orc, false), "Bardo usou Palavras de Interrupção: o ataque errou. (respondido pelo mestre)", true},
		{"a damage roll, for a player", base(false, reactionEvent{Used: true, Roll: "damage"}, orc, true), "Bardo usou Palavras de Interrupção: o dano de Orc diminuiu. (respondido pelo mestre)", true},
		{"a test, for a player", base(false, reactionEvent{Used: true, Roll: "test"}, orc, true), "Bardo usou Palavras de Interrupção: o teste de Orc diminuiu. (respondido pelo mestre)", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := c.f.cuttingWords()
			if got != c.want || ok != c.wantOK {
				t.Errorf("cuttingWords = (%q, %v), want (%q, %v)", got, ok, c.want, c.wantOK)
			}
		})
	}
}
