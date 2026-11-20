import { create } from '@bufbuild/protobuf';

import { CombatantKind } from '../../../gen/meurpg/play/v1/combat_pb';
import {
  GroupCheckMemberViewSchema,
  HideObserverSchema,
  HiderTotalSchema,
  ShoveBlockedReason,
  SurpriseSuggestionReason,
  SurpriseSuggestionSchema,
  ContestPurpose,
  ContestWinner,
} from '../../../gen/meurpg/play/v1/contest_types_pb';
import { combatant, encounter } from './combat-testing';
import {
  answerWait,
  applyLabel,
  byWho,
  escapeDcOf,
  groupProgress,
  askedTest,
  groupTitle,
  groupVerdict,
  masterContestTitle,
  masterPushLine,
  memberResult,
  notNoticing,
  observerRows,
  optionLabel,
  suggestedSkill,
  surpriseRows,
  totalLine,
  winnerLine,
} from './contest-master';
import { checkRoll, contestView, groupCheck, hideAttempt, skillOption } from './contest-testing';
import { ContestSkill } from '../../../gen/meurpg/play/v1/contest_types_pb';

const e = encounter({
  combatants: [
    combatant({ id: 't', label: 'Toren', kind: CombatantKind.PLAYER }),
    combatant({ id: 'b', label: 'Brisa', kind: CombatantKind.PLAYER }),
    combatant({ id: 'h', label: 'Hobgoblin' }),
    combatant({ id: 'cap', label: 'Capitão bandido' }),
  ],
});

describe('what the master reads of a contest', () => {
  it('titles the card as the board does', () => {
    expect(masterContestTitle(e, contestView())).toBe('Toren tenta agarrar o Hobgoblin');
    expect(masterContestTitle(e, contestView({ initiatorId: 'h', defenderId: 'b' }))).toBe(
      'Hobgoblin tenta agarrar Brisa',
    );
    expect(
      masterContestTitle(e, contestView({ purpose: ContestPurpose.SHOVE, defenderId: 'h' })),
    ).toBe('Toren tenta empurrar o Hobgoblin');
    expect(
      masterContestTitle(
        e,
        contestView({ purpose: ContestPurpose.ESCAPE, initiatorId: 'b', defenderId: 'h' }),
      ),
    ).toBe('Brisa tenta escapar');
  });

  it('writes the total as die + modifier = total, with a true minus', () => {
    expect(totalLine(checkRoll())).toBe('15 + 5 = 20');
    expect(totalLine(checkRoll({ faces: [9], modifier: -1, total: 8 }))).toBe('9 − 1 = 8');
  });

  it('suggests the skill with the higher modifier and says what each rolls', () => {
    const options = [
      skillOption({ skill: ContestSkill.ATHLETICS, modifier: 1 }),
      skillOption({ skill: ContestSkill.ACROBATICS, modifier: 3, suggested: true }),
    ];
    expect(suggestedSkill(options)).toBe(ContestSkill.ACROBATICS);
    expect(optionLabel(options[0])).toBe('Atletismo (+1)');
    expect(optionLabel(skillOption({ known: false }))).toBe('Atletismo (modificador desconhecido)');
  });

  it('says who wins, and that a tie changes nothing', () => {
    expect(winnerLine(e, contestView({ winner: ContestWinner.INITIATOR }))).toBe('Toren vence');
    expect(winnerLine(e, contestView({ winner: ContestWinner.DEFENDER }))).toBe(
      'O Hobgoblin vence',
    );
    expect(winnerLine(e, contestView({ winner: ContestWinner.TIE }))).toBe('Empate: nada muda');
  });

  it('rolls "pelo Hobgoblin" and "por Brisa"', () => {
    expect(byWho({ label: 'Hobgoblin', player: false })).toBe('pelo Hobgoblin');
    expect(byWho({ label: 'Brisa', player: true })).toBe('por Brisa');
  });

  it('says what the contest waits for', () => {
    expect(answerWait(e, contestView())).toBe('Esperando a sua resposta.');
    expect(answerWait(e, contestView({ defenderId: 'b', waitingCombatantId: 'b' }))).toBe(
      'Esperando Brisa. O jogador está respondendo no celular.',
    );
    expect(answerWait(e, contestView({ defenderId: 'b', deferred: true }))).toBe(
      'Brisa deixou o mestre rolar o teste.',
    );
  });

  it('says the push, and why a blocked one stays', () => {
    expect(masterPushLine('de Toren', ShoveBlockedReason.UNSPECIFIED)).toBe(
      'Para longe de Toren, em linha reta.',
    );
    expect(masterPushLine('de Toren', ShoveBlockedReason.WALL)).toBe(
      'Para longe de Toren. Há uma parede na casa de trás: o alvo não sai do lugar.',
    );
    expect(masterPushLine('de Toren', ShoveBlockedReason.CREATURE)).toContain('uma criatura');
  });

  it('takes an escape DC from 1 to 40 and nothing else', () => {
    expect(escapeDcOf('16')).toBe(16);
    expect(escapeDcOf(' 40 ')).toBe(40);
    for (const bad of ['', '0', '41', '1,5', 'dezesseis', '-3']) {
      expect(escapeDcOf(bad)).toBeNull();
    }
  });
});

describe('what the master reads of a Hide', () => {
  const attempt = hideAttempt({
    hiderId: 'b',
    observers: [
      create(HideObserverSchema, { combatantId: 'h', passivePerception: 10, known: true }),
      create(HideObserverSchema, {
        combatantId: 'cap',
        passivePerception: 10,
        known: true,
        noticed: true,
      }),
    ],
  });

  it('lists each observer with the passive Perception and whether it notices', () => {
    const rows = observerRows(e, attempt);
    expect(rows.map((r) => [r.label, r.passive, r.noticed])).toEqual([
      ['Hobgoblin', 'Percepção passiva 10', false],
      ['Capitão bandido', 'Percepção passiva 10', true],
    ]);
  });

  it('counts the ones that do not notice, less the ones marked "Vê claramente"', () => {
    const rows = observerRows(e, attempt);
    expect(notNoticing(rows, new Set())).toBe(1);
    expect(notNoticing(rows, new Set(['h']))).toBe(0);
  });

  it('words the button by the count', () => {
    expect(applyLabel('escondida', 3)).toBe('Aplicar: escondida (3 não notam)');
    expect(applyLabel('escondido', 1)).toBe('Aplicar: escondido (1 não nota)');
    expect(applyLabel('escondida', 0)).toBe('Aplicar: ninguém é enganado');
  });
});

describe('what the master reads of a group check', () => {
  const member = (over: object) =>
    create(GroupCheckMemberViewSchema, { characterId: 'x', name: 'X', ...over });
  const g = groupCheck({
    dc: 13,
    needed: 3,
    passedCount: 2,
    verdictKnown: true,
    groupPassed: false,
    members: [
      member({
        name: 'Brisa',
        answered: true,
        roll: checkRoll({ total: 19 }),
        passedKnown: true,
        passed: true,
      }),
      member({ name: 'Toren', answered: true, roll: checkRoll({ total: 8 }), passedKnown: true }),
      member({ name: 'Ragna' }),
    ],
  });

  it('titles with the DC when there is one, and counts who answered', () => {
    expect(groupTitle(g)).toBe('Teste em grupo: Furtividade, CD 13');
    expect(groupTitle(groupCheck())).toBe('Teste em grupo: Furtividade');
    expect(groupTitle(groupCheck({ group: false }))).toBe('Teste pedido: Furtividade');
    expect(askedTest('Percepção')).toBe('um teste de Percepção');
    expect(askedTest('Teste de Força')).toBe('um teste de Força');
    expect(askedTest('Teste de resistência de Constituição')).toBe(
      'um teste de resistência de Constituição',
    );
    expect(groupProgress(g)).toBe(
      '2 de 3 responderam. O grupo passa se ao menos metade dos convocados passar.',
    );
  });

  it('reads each row and the verdict that only the master reads', () => {
    expect(g.members.map(memberResult)).toEqual(['19 Passou', '8 Falhou', 'Não respondeu']);
    expect(groupVerdict(g)).toBe('2 de 3 passaram; precisa de 3. O grupo falhou.');
    expect(groupVerdict(groupCheck())).toBe('');
  });
});

describe('what the master reads of the surprise', () => {
  it('shows the passive Perception and the Stealth that beats it, or "Não está escondido: nota"', () => {
    const rows = surpriseRows(e, [
      create(SurpriseSuggestionSchema, {
        combatantId: 'h',
        passivePerception: 10,
        suggested: true,
        reason: SurpriseSuggestionReason.HIDERS_BEAT,
        hiders: [create(HiderTotalSchema, { combatantId: 'b', stealthTotal: 19, beats: true })],
      }),
      create(SurpriseSuggestionSchema, {
        combatantId: 't',
        passivePerception: 11,
        reason: SurpriseSuggestionReason.NO_HIDERS,
      }),
    ]);
    expect(rows.map((r) => [r.label, r.detail, r.suggested])).toEqual([
      ['Hobgoblin', 'Percepção passiva 10 · Furtividade de Brisa 19 vence', true],
      ['Toren', 'Percepção passiva 11 · Não está escondido: nota', false],
    ]);
  });
});
