import { create } from '@bufbuild/protobuf';

import {
  CombatantSchema,
  ReadiedActionSchema,
  ReadiedKind,
  ReadiedStatus,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { holdsReadied, readiedCardView, targetPhrase } from './ready-card';

const plain = (s: string) => s.replace(/ /g, ' ');

const people = [
  { id: 'z1', label: 'Zumbi 1' },
  { id: 'b', label: 'Brisa' },
];

const held = (over: Record<string, unknown> = {}) =>
  create(ReadiedActionSchema, {
    id: 'r1',
    triggerText: 'Quando o zumbi passar pela porta',
    kind: ReadiedKind.ATTACK,
    actionKey: 'equipment:longsword',
    actionNamePt: 'Espada longa',
    targetCombatantId: 'z1',
    status: ReadiedStatus.WAITING,
    ...over,
  } as never);

describe('the readied card (#582)', () => {
  it('names the held action and the creature, and quotes the trigger', () => {
    const v = readiedCardView(held(), people, { ownTurn: true, reactionUsed: false });
    expect(v.title).toBe('Espada longa no Zumbi 1');
    expect(v.quote).toBe('“Quando o zumbi passar pela porta”');
    expect(v.concentration).toBe(false);
  });

  it('agrees the article with the name of the creature', () => {
    expect(targetPhrase('Brisa')).toBe('na Brisa');
    expect(targetPhrase('Zumbi 1')).toBe('no Zumbi 1');
    expect(targetPhrase('')).toBe('');
  });

  it("says what the reaction costs on the readier's turn, and what the card waits for off it", () => {
    expect(readiedCardView(held(), people, { ownTurn: true, reactionUsed: false }).state).toBe(
      'Gasta a reação quando o mestre disparar · se perde no começo da sua vez',
    );
    expect(readiedCardView(held(), people, { ownTurn: false, reactionUsed: false }).state).toBe(
      'Esperando o gatilho · reação livre',
    );
    expect(readiedCardView(held(), people, { ownTurn: false, reactionUsed: true }).state).toBe(
      'Esperando o gatilho · reação usada',
    );
  });

  it('writes a move by its limit and a spell with its concentration', () => {
    const move = readiedCardView(
      held({
        kind: ReadiedKind.MOVE,
        actionNamePt: '',
        actionKey: '',
        moveFt: 25,
        targetCombatantId: '',
      }),
      people,
      {
        ownTurn: false,
        reactionUsed: false,
      },
    );
    expect(plain(move.title)).toBe('Mover, até 7,5 m');
    const spell = readiedCardView(
      held({ kind: ReadiedKind.SPELL, actionNamePt: 'Mísseis Mágicos', targetCombatantId: 'b' }),
      people,
      {
        ownTurn: true,
        reactionUsed: false,
      },
    );
    expect(spell.title).toBe('Mísseis Mágicos na Brisa');
    expect(spell.concentration).toBe(true);
  });

  it('names no creature for another action, which has no target to name', () => {
    const dodge = readiedCardView(
      held({ kind: ReadiedKind.OTHER, actionNamePt: 'Esquivar' }),
      people,
      { ownTurn: true, reactionUsed: false },
    );
    expect(dodge.title).toBe('Esquivar');
  });

  it('holds only an action that still waits', () => {
    expect(holdsReadied(create(CombatantSchema, { readied: held() }))).toBe(true);
    expect(
      holdsReadied(create(CombatantSchema, { readied: held({ status: ReadiedStatus.LOST }) })),
    ).toBe(false);
    expect(holdsReadied(create(CombatantSchema, {}))).toBe(false);
    expect(holdsReadied(null)).toBe(false);
  });
});
