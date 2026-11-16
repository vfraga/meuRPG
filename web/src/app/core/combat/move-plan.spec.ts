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
  afterLeftText,
  costTitle,
  extraBreakdown,
  extrasOf,
  indexOptions,
  leftLine,
  ofThe,
  jumpCardText,
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
    // A refusal the server did not name (the zero of the enum) is still a refusal, not "beyond".
    const unnamed = indexOptions(
      create(GetMoveOptionsResponseSchema, {
        refused: [create(RefusedSquareSchema, { col: 4, row: 4, reason: MoveRefusal.UNSPECIFIED })],
      }),
    );
    expect(verdictFor(unnamed, origin, { col: 4, row: 4 }).kind).toBe('refused');
    expect(verdictFor(index, origin, origin).kind).toBe('here');
    expect(verdictFor(indexOptions(null), origin, { col: 7, row: 8 }).kind).toBe('unknown');
  });

  it("says the cost in metres and what is left, from the server's numbers", () => {
    expect(plain(costTitle(71))).toBe('Mover 2,1 m');
    expect(plain(afterLeftText(229))).toBe('Depois restam 6,9 m.');
    expect(plain(afterLeftText(0))).toBe('Depois restam 0,0 m.');
    expect(plain(costTitle(258))).toBe('Mover 7,7 m');
  });

  describe("what swimming, climbing and crawling added, from the server's numbers", () => {
    const square = (parts: Partial<{ swim: number; climb: number; crawl: number; path: number }>) =>
      create(ReachableSquareSchema, {
        col: 3,
        row: 2,
        costDft: 300,
        swimExtraDft: parts.swim ?? 0,
        climbExtraDft: parts.climb ?? 0,
        crawlExtraDft: parts.crawl ?? 0,
        pathDft: parts.path ?? 0,
        extraDft: (parts.swim ?? 0) + (parts.climb ?? 0),
      });
    const words = (s: ReturnType<typeof extrasOf>) => ({
      parts: s.parts.map((p) => `${p.kind}:${plain(p.text)}`),
      note: s.note,
    });

    it('a swim alone says that each metre in the water counts double', () => {
      expect(words(extrasOf(square({ swim: 100 })))).toEqual({
        parts: ['swim:+3,0 m nadando'],
        note: ': cada metro na água conta dobrado.',
      });
    });

    it('a climb alone says the same for climbing', () => {
      expect(words(extrasOf(square({ climb: 50 })))).toEqual({
        parts: ['climb:+1,5 m escalando'],
        note: ': cada metro escalado conta dobrado.',
      });
    });

    it('two extras are each said with its word, with no sentence after them', () => {
      expect(words(extrasOf(square({ swim: 50, crawl: 50 })))).toEqual({
        parts: ['swim:+1,5 m nadando', 'crawl:+1,5 m rastejando'],
        note: '',
      });
      expect(words(extrasOf(square({ swim: 50, climb: 50 })))).toEqual({
        parts: ['swim:+1,5 m nadando', 'climb:+1,5 m escalando'],
        note: '',
      });
    });

    it('says nothing when the square pays no extra', () => {
      expect(extrasOf(square({}))).toEqual({ parts: [], note: '' });
    });

    it('breaks a crawl down into the path and each extra, as the server counted them', () => {
      expect(plain(extraBreakdown(square({ swim: 50, crawl: 50, path: 50 })))).toBe(
        '1,5 m do caminho + 1,5 m nadando + 1,5 m rastejando.',
      );
      expect(plain(extraBreakdown(square({ crawl: 50, path: 50 })))).toBe(
        '1,5 m do caminho + 1,5 m rastejando.',
      );
      // Without a crawl the cost line already says it all.
      expect(extraBreakdown(square({ swim: 50, path: 50 }))).toBe('');
    });
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
    // Dragging a grappled creature (W7-X): no free square behind the path to leave it on.
    expect(say(MoveRefusal.NO_ROOM_TO_DRAG)).toEqual({
      title: 'Sem casa para arrastar',
      detail:
        'Não há casa livre atrás de você para quem você segura. Escolha outro quadrado, ou solte o agarrão.',
    });
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

  it('words the warning of a long jump as the board does', () => {
    expect(plain(provokeWarning(['Goblin 2'], 'salto'))).toBe(
      'Esse salto sai do alcance do Goblin 2. Ele pode atacar você de graça (ataque de oportunidade).',
    );
    expect(plain(provokeWarning(['Goblin 1', 'Brisa'], 'salto'))).toBe(
      'Esse salto sai do alcance do Goblin 1 e da Brisa. Eles podem atacar você de graça (ataque de oportunidade).',
    );
    expect(plain(provokeWarning(['Brisa'], 'salto'))).toContain('Ela pode atacar você de graça');
  });

  it('writes the jump card: where it lands, and the trap only when the character knows it', () => {
    expect(plain(jumpCardText(150, 50))).toBe('Cai 4,5 m adiante. Depois restam 1,5 m.');
    expect(plain(jumpCardText(150, 50, 'Fosso'))).toBe(
      'Cai do outro lado do Fosso. Depois restam 1,5 m. O Fosso só dispara onde você cai.',
    );
    // Without the server's number the card does not make one up.
    expect(plain(jumpCardText(150, null))).toBe('Cai 4,5 m adiante.');
    // A trap the character does not know has no name on the card: the server sends none.
    expect(jumpCardText(150, 50, '')).not.toMatch(/Fosso|armadilha/i);
  });

  it('writes what is left of a part, and what was walked', () => {
    expect(plain(leftLine(300, 300, 0, 6))).toBe('Restam 9,0 m de 9,0 m (6 quadrados de 1,5 m).');
    expect(plain(leftLine(600, 600, 0, 12, 'Cavalo de guerra'))).toBe(
      'Restam 18,0 m de 18,0 m ao Cavalo de guerra (12 quadrados de 1,5 m).',
    );
    expect(plain(leftLine(600, 600, 0, null, 'Aranha'))).toBe('Restam 18,0 m de 18,0 m à Aranha.');
    expect(plain(leftLine(200, 300, 100, null))).toBe(
      'Restam 6,0 m de 9,0 m. Você já andou 3,0 m.',
    );
  });
});
