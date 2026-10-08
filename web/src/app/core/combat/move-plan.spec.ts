import { create } from '@bufbuild/protobuf';
import { describe, expect, it } from 'vitest';

import {
  GetMoveOptionsResponseSchema,
  MoveRefusal,
  ReachableSquareSchema,
  RefusedSquareSchema,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { combatant } from './combat-testing';
import {
  afterText,
  costTitle,
  indexOptions,
  leftLine,
  ofThe,
  provokeWarning,
  provokedBy,
  refusalText,
  trapQuestion,
  verdictFor,
} from './move-plan';

const plain = (t: string) => t.replace(/\u00a0/g, ' ');
const origin = { col: 8, row: 7 };

// Toren (README-B, "Numbers"): 7,07 ft = 2,1 m to (7,8); a wall square, an enemy square and one too costly.
const options = create(GetMoveOptionsResponseSchema, {
  movementLeftDft: 300,
  reachable: [
    create(ReachableSquareSchema, { col: 7, row: 8, costDft: 71 }),
    create(ReachableSquareSchema, {
      col: 5,
      row: 10,
      costDft: 258,
      provokesReactorIds: ['g2'],
      knownTrapName: 'Fosso escondido',
    }),
  ],
  refused: [
    create(RefusedSquareSchema, { col: 8, row: 5, reason: MoveRefusal.WALL }),
    create(RefusedSquareSchema, { col: 9, row: 7, reason: MoveRefusal.ENEMY }),
    create(RefusedSquareSchema, { col: 6, row: 9, reason: MoveRefusal.OCCUPIED }),
    create(RefusedSquareSchema, { col: 3, row: 7, reason: MoveRefusal.TOO_COSTLY }),
  ],
});

describe("the move page reads the server's options", () => {
  const index = indexOptions(options);

  it('finds where a square stands in the options', () => {
    expect(verdictFor(index, origin, { col: 7, row: 8 }).kind).toBe('ok');
    expect(verdictFor(index, origin, { col: 8, row: 5 })).toEqual({
      kind: 'refused',
      reason: MoveRefusal.WALL,
    });
    expect(verdictFor(index, origin, { col: 15, row: 7 }).kind).toBe('beyond');
    expect(verdictFor(index, origin, origin).kind).toBe('here');
    expect(verdictFor(indexOptions(null), origin, { col: 7, row: 8 }).kind).toBe('unknown');
  });

  it("says the cost in metres and what is left, from the server's numbers", () => {
    expect(plain(costTitle(71))).toBe('Mover 2,1 m');
    expect(plain(afterText(300, 71))).toBe('Depois restam 6,9 m.');
    expect(plain(afterText(300, 300))).toBe('Depois restam 0,0 m.');
    expect(plain(costTitle(258))).toBe('Mover 7,7 m');
  });

  it('explains each refusal by its reason, never naming what is in the way', () => {
    const say = (reason: MoveRefusal) => refusalText({ kind: 'refused', reason }, 300);
    expect(say(MoveRefusal.WALL)).toEqual({
      title: 'Sem caminho reto',
      detail:
        'Uma parede bloqueia esse caminho, no meio da linha ou no próprio quadrado. Escolha outro quadrado; para contornar uma parede no caminho, mova em partes.',
    });
    expect(say(MoveRefusal.ENEMY)?.title).toBe('Inimigo no caminho');
    expect(say(MoveRefusal.OCCUPIED)?.title).toBe('Ocupado');
    expect(say(MoveRefusal.TOO_COSTLY)?.title).toBe('Longe demais');
    expect(plain(refusalText({ kind: 'beyond' }, 300)!.detail)).toContain('9,0 m que você tem');
    expect(refusalText({ kind: 'ok', square: options.reachable[0] }, 300)).toBeNull();
  });

  it('warns by name about a reactor it sees, and asks about a trap', () => {
    const goblin = combatant({ id: 'g2', label: 'Goblin 2' });
    const names = provokedBy(options.reachable[1], [
      goblin,
      combatant({ id: 'g3', label: 'Goblin 3' }),
    ]);
    expect(names).toEqual(['Goblin 2']);
    expect(plain(provokeWarning(names))).toBe(
      'Sair do alcance do Goblin 2 pode provocar um ataque de oportunidade.',
    );
    expect(plain(provokeWarning(['Goblin 1', 'Brisa']))).toBe(
      'Sair do alcance do Goblin 1 e da Brisa pode provocar um ataque de oportunidade.',
    );
    expect(provokedBy(options.reachable[0], [goblin])).toEqual([]);
    expect(trapQuestion('Fosso escondido')).toBe(
      'Isso entra no Fosso escondido. Mover assim mesmo?',
    );
    expect(ofThe(['Toren'])).toBe('do Toren');
  });

  it('writes what is left of a part, and what was walked', () => {
    expect(plain(leftLine(300, 300, 0, 6))).toBe('Restam 9,0 m de 9,0 m (6 quadrados de 1,5 m).');
    expect(plain(leftLine(200, 300, 100, null))).toBe(
      'Restam 6,0 m de 9,0 m. Você já andou 3,0 m.',
    );
  });
});
