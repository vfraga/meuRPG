import { create } from '@bufbuild/protobuf';

import {
  CombatantState,
  ReactionKind,
  ReadiedActionSchema,
  ReadiedKind,
  ReadiedStatus,
  TargetInReachSchema,
} from '../../../gen/meurpg/play/v1/combat_pb';
import {
  readiedGoneText,
  readiedLegend,
  readiedPrompt,
  readiedTargetRows,
  readiedMasterRow,
} from './reaction-readied';
import { combatant, encounter, reactionWindow } from './combat-testing';

const held = (over: Record<string, unknown> = {}) =>
  create(ReadiedActionSchema, {
    id: 'r1',
    triggerText: 'Quando o zumbi passar pela porta',
    kind: ReadiedKind.ATTACK,
    actionKey: 'equipment:longsword',
    actionNamePt: 'Espada longa',
    targetCombatantId: 'z1',
    status: ReadiedStatus.WAITING,
    moveFt: 0,
    ...over,
  } as never);

const plain = (s: string) => s.replace(/ /g, ' ');

describe('the prompt of a readied window (582-06, 582-07, 582-08)', () => {
  it('quotes the trigger and asks the attack by its weapon', () => {
    const p = readiedPrompt(held(), 2, 'Tor');
    expect(p.title).toBe('Ação preparada');
    expect(p.subtitle).toBe('Reação · Rodada 2');
    expect(p.lead).toBe('O mestre disse que o gatilho aconteceu:');
    expect(p.quote).toBe('“Quando o zumbi passar pela porta”');
    expect(p.question).toBe('Atacar com a Espada longa agora?');
    expect(p.declineLabel).toBe('Deixar passar');
    expect(p.releasable).toBe(true);
    expect(p.note).toBe('Gasta a sua reação; ela volta no começo da sua vez.');
  });

  it('names a spell and says that passing loses it, and offers to release it', () => {
    const p = readiedPrompt(
      held({ kind: ReadiedKind.SPELL, actionNamePt: 'Mísseis Mágicos' }),
      2,
      'Pensantus',
    );
    expect(p.title).toBe('Soltar Mísseis Mágicos');
    expect(p.subtitle).toBe('Reação · a ação preparada de Pensantus');
    expect(p.releasable).toBe(true);
    expect(p.note).toContain('Deixar passar perde a magia');
  });

  it('says a move with its limit and a standard action by its name, with one button', () => {
    const move = readiedPrompt(
      held({ kind: ReadiedKind.MOVE, moveFt: 25, actionNamePt: '' }),
      2,
      'Tor',
    );
    expect(plain(move.question)).toBe('Mover agora, até 7,5 m?');
    expect(move.releasable).toBe(false);
    const other = readiedPrompt(
      held({ kind: ReadiedKind.OTHER, actionNamePt: 'Esquivar', triggerText: 'Se a porta abrir' }),
      2,
      'Tor',
    );
    expect(other.question).toBe('Esquivar agora?');
    expect(other.useLabel).toBe('Esquivar agora');
    expect(other.releasable).toBe(true);
  });

  it('still prompts when the row is not on the wire (the readier is not the caller)', () => {
    const p = readiedPrompt(undefined, 2, 'Tor');
    expect(p.quote).toBe('');
    expect(p.question).toBe('Usar a ação preparada agora?');
  });
});

describe('who an attack of the window can be aimed at', () => {
  const t = (id: string, label: string, over: Record<string, unknown> = {}) =>
    create(TargetInReachSchema, {
      combatantId: id,
      label,
      state: CombatantState.UNHURT,
      distanceFt: 5,
      ...over,
    } as never);

  it('names the creature the player said, and the reason of the one out of reach', () => {
    const rows = readiedTargetRows(
      [
        t('z1', 'Zumbi 1', { state: CombatantState.HURT }),
        t('z2', 'Zumbi 2', { distanceFt: 15, tooFar: true }),
      ],
      'z1',
      5,
    );
    expect(rows.map((r) => plain(r.sub))).toEqual([
      'O alvo que você disse · Ferido · a 1,5 m',
      'Ileso · a 4,5 m',
    ]);
    expect(plain(rows[1].blocked)).toBe('Fora de alcance: até 1,5 m');
    expect(rows[0].blocked).toBe('');
  });

  it('says a creature behind total cover cannot be a target, and counts the ones that can', () => {
    const rows = readiedTargetRows(
      [t('z1', 'Zumbi 1'), t('z2', 'Zumbi 2', { untargetable: true })],
      '',
      5,
    );
    expect(rows[1].blocked).toBe('Cobertura total: não dá para mirar');
    expect(readiedLegend(rows)).toBe('Em quem · 1 criatura ao alcance');
    expect(readiedLegend([rows[0], { ...rows[0], id: 'x' }])).toBe(
      'Em quem · 2 criaturas ao alcance',
    );
    expect(readiedLegend([])).toBe('Em quem · ninguém ao alcance');
  });
});

describe('a window that closed without the player', () => {
  it('says it passed by itself only when the clock ran out, and otherwise only that it closed', () => {
    const base = { reactionUsed: false, down: false };
    expect(readiedGoneText({ ...base, timedOut: true })).toBe(
      'Passou sozinho: você não respondeu em 30 s.',
    );
    // The master may have passed or answered it by hand: nothing on the wire says which.
    expect(readiedGoneText({ ...base, timedOut: false })).toBe('A janela fechou.');
    expect(readiedGoneText({ ...base, reactionUsed: true, timedOut: false })).toBe(
      'A janela fechou e a sua reação foi usada.',
    );
    expect(readiedGoneText({ ...base, down: true, timedOut: true })).toBe('Você ficou Caído.');
  });
});

describe("the master's row of a readied window (582-05)", () => {
  const e = encounter({
    combatants: [
      combatant({ id: 'tor', label: 'Tor', readied: held() }),
      combatant({ id: 'z1', label: 'Zumbi 1' }),
    ],
  });
  const w = reactionWindow({
    id: 'w1',
    kind: ReactionKind.READIED,
    reactorId: 'tor',
    reactorLabel: 'Tor',
    reactorIsPlayer: true,
    answerNow: true,
  });

  it('reads the held action and the trigger, and answers for the player in the board words', () => {
    const row = readiedMasterRow(e, w);
    expect(plain(row.description)).toBe(
      'Ação preparada: Espada longa no Zumbi 1 · “Quando o zumbi passar pela porta”',
    );
    expect(row.waits).toBe('Esperando Tor');
    expect(row.useLabel).toBe('Atacar por Tor');
    expect(row.passLabel).toBe('Deixar passar');
    expect(row.useOff).toBe('');
  });

  it('cannot use a move or a spell for the player from here, and says so', () => {
    const move = encounter({
      combatants: [
        combatant({
          id: 'tor',
          label: 'Tor',
          readied: held({ kind: ReadiedKind.MOVE, moveFt: 25 }),
        }),
      ],
    });
    expect(readiedMasterRow(move, w).useOff).toBe(
      'Mover é pelo mapa: mova Tor com a janela aberta.',
    );
  });
});
