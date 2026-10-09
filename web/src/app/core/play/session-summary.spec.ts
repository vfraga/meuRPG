import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';

import {
  CharacterHighlightsSchema,
  HighlightCategorySchema,
  HighlightKind,
  HighlightWinnerSchema,
} from '../../../gen/meurpg/play/v1/combat_pb';
import {
  SessionCharacterSummarySchema,
  type SessionSummary,
  SessionSummarySchema,
} from '../../../gen/meurpg/play/v1/summary_pb';
import {
  COMBAT_COLUMNS,
  checksRatio,
  combatRows,
  formatDuration,
  sessionSpan,
  summaryOwn,
  summaryRows,
  summaryTiles,
} from './session-summary';

const plain = (s: string | undefined) => s?.replace(/\u00a0/g, ' ');

function category(kind: HighlightKind, value: number, ...winners: [string, string][]) {
  return create(HighlightCategorySchema, {
    kind,
    value,
    winners: winners.map(([characterId, name]) =>
      create(HighlightWinnerSchema, { characterId, name }),
    ),
  });
}

function player(characterId: string, name: string, passed: number, tried: number, combat = {}) {
  return create(SessionCharacterSummarySchema, {
    highlights: create(CharacterHighlightsSchema, { characterId, name, ...combat }),
    checksPassed: passed,
    checksTried: tried,
  });
}

/** The artboards' session (E8-11): 3 h 5 min, one combat, three scenes, 9 of 12. */
const master: SessionSummary = create(SessionSummarySchema, {
  startedAt: timestampFromDate(new Date(2026, 9, 3, 20, 5)),
  endedAt: timestampFromDate(new Date(2026, 9, 3, 23, 10)),
  duration: { seconds: BigInt(3 * 3600 + 5 * 60), nanos: 0 },
  combats: 1,
  scenesOpened: 3,
  checksPassed: 9,
  checksTried: 12,
  categories: [
    category(HighlightKind.MOST_DAMAGE, 23, ['toren', 'Toren']),
    category(HighlightKind.TANK, 24, ['brisa', 'Brisa']),
    category(HighlightKind.FINAL_BLOW, 2, ['pens', 'Pensantus'], ['toren', 'Toren']),
    category(HighlightKind.CHECKS_PASSED, 4, ['brisa', 'Brisa']),
  ],
  players: [
    player('pens', 'Pensantus', 3, 4),
    player('toren', 'Toren', 2, 3),
    player('brisa', 'Brisa', 4, 5),
  ],
});

describe('session summary (MR-032)', () => {
  it('writes the duration in hours and minutes, tied to their numbers', () => {
    expect(plain(formatDuration(3 * 3600 + 5 * 60))).toBe('3 h 5 min');
    expect(plain(formatDuration(2 * 3600))).toBe('2 h');
    expect(plain(formatDuration(45 * 60))).toBe('45 min');
    expect(plain(formatDuration(20))).toBe('menos de 1 min');
    expect(formatDuration(3 * 3600 + 5 * 60)).toContain('3 h 5 min');
  });

  it('writes "9 de 12" with the number tied to its "de"', () => {
    expect(checksRatio(9, 12)).toBe('9 de 12');
  });

  it('says when it ran, "das 20:05 às 23:10"', () => {
    expect(plain(sessionSpan(master))).toBe('das 20:05 às 23:10');
    expect(sessionSpan(master)).toContain('das\u00a020:05');
  });

  it('makes a tile per category, the checks one included, with how many the winner tried (master)', () => {
    const tiles = summaryTiles(master);
    expect(tiles.map((t) => t.label)).toEqual([
      'Mais dano causado',
      'Tanque',
      'Golpe final',
      'Mais testes passados fora do combate',
    ]);
    const checks = tiles[3];
    expect(plain(checks.value)).toBe('4 testes');
    expect(checks.names).toBe('Brisa');
    expect(plain(checks.sub)).toBe('de 5 tentados');
    expect(checks.sub).toContain('de\u00a05\u00a0tentados');
    // The other tiles are the combat's, untouched.
    expect(tiles[1].sub).toBe('mais dano recebido');
  });

  it('says nothing of how many were tried when the winners tied with different numbers, or for a player (no table)', () => {
    const tie = create(SessionSummarySchema, {
      categories: [
        category(HighlightKind.CHECKS_PASSED, 3, ['pens', 'Pensantus'], ['brisa', 'Brisa']),
      ],
      players: [player('pens', 'Pensantus', 3, 4), player('brisa', 'Brisa', 3, 5)],
    });
    expect(summaryTiles(tie)[0].sub).toBe('');
    const forPlayer = create(SessionSummarySchema, { categories: master.categories, players: [] });
    expect(summaryTiles(forPlayer).every((t) => !t.sub.startsWith('de'))).toBe(true);
  });

  it("makes the master's table: one row per player, passed of tried, zeros included", () => {
    expect(summaryRows(master).map((r) => [r.name, plain(r.cells[0])])).toEqual([
      ['Pensantus', '3 de 4'],
      ['Toren', '2 de 3'],
      ['Brisa', '4 de 5'],
    ]);
    expect(
      plain(
        summaryRows(create(SessionSummarySchema, { players: [player('x', 'Mira', 0, 2)] }))[0]
          .cells[0],
      ),
    ).toBe('0 de 2');
    // Fought but tried no test: "nenhum teste", muted, never "0 de 0".
    const fought = summaryRows(
      create(SessionSummarySchema, { players: [player('x', 'Mira', 0, 0, { damageDealt: 5 })] }),
    )[0];
    expect([fought.cells[0], fought.muted]).toEqual(['nenhum teste', true]);
  });

  it('makes "Seu resultado": the combat numbers when the character had any, and the checks when it tried any', () => {
    const own = summaryOwn(player('pens', 'Pensantus', 3, 4, { damageDealt: 17, finalBlows: 2 }));
    expect(own.map((n) => [n.label, plain(n.value)])).toEqual([
      ['Dano causado', '17'],
      ['Dano recebido', '0'],
      ['Golpes finais', '2'],
      ['Cura', '0'],
      ['Acertos críticos', '0'],
      ['Testes passados fora do combate', '3 de 4'],
    ]);
    // Only a scene: just the checks. Nothing at all: no block.
    expect(summaryOwn(player('pens', 'Pensantus', 1, 2)).map((n) => n.label)).toEqual([
      'Testes passados fora do combate',
    ]);
    expect(summaryOwn(player('pens', 'Pensantus', 0, 0))).toEqual([]);
    expect(summaryOwn(undefined)).toEqual([]);
  });

  it('draws "Mais tesouro encontrado" as its own tile, never as damage, and skips a kind it does not know', () => {
    const withTreasure = create(SessionSummarySchema, {
      categories: [
        category(HighlightKind.MOST_DAMAGE, 8, ['brisa', 'Brisa']),
        category(HighlightKind.TREASURE_FOUND, 245, ['toren', 'Toren']),
        category(99 as HighlightKind, 3, ['toren', 'Toren']),
      ],
    });
    const tiles = summaryTiles(withTreasure);
    expect(tiles.map((t) => t.label)).toEqual(['Mais dano causado', 'Mais tesouro encontrado']);
    expect(plain(tiles[1].value)).toBe('245 PO');
    expect(tiles[1].icon).toBe('paid');
  });

  it('gives a character that only found treasure no row of zeros, and a "Seu resultado" with the treasure', () => {
    const onlyTreasure = create(SessionCharacterSummarySchema, {
      highlights: create(CharacterHighlightsSchema, { characterId: 'toren', name: 'Toren' }),
      treasureFoundPo: 245,
    });
    expect(
      summaryRows(
        create(SessionSummarySchema, { players: [onlyTreasure, player('brisa', 'Brisa', 1, 1)] }),
      ).map((r) => r.name),
    ).toEqual(['Brisa']);
    expect(summaryOwn(onlyTreasure).map((n) => [n.label, plain(n.value)])).toEqual([
      ['Tesouro encontrado', '245 PO'],
    ]);
    const both = create(SessionCharacterSummarySchema, {
      highlights: create(CharacterHighlightsSchema, {
        characterId: 'toren',
        name: 'Toren',
        damageDealt: 7,
      }),
      treasureFoundPo: 120,
    });
    expect(summaryOwn(both).map((n) => n.label)).toEqual([
      'Dano causado',
      'Dano recebido',
      'Golpes finais',
      'Cura',
      'Acertos críticos',
      'Tesouro encontrado',
    ]);
  });

  it('gives "Números de cada jogador" a row for each character that fought, in the combat\'s column order, and none for one that only rolled', () => {
    const summary = create(SessionSummarySchema, {
      players: [
        player('toren', 'Toren', 0, 0, {
          damageDealt: 41,
          healingDone: 2,
          damageTaken: 19,
          finalBlows: 3,
          criticalHits: 1,
        }),
        player('pens', 'Pensantus', 3, 4),
      ],
    });
    expect(COMBAT_COLUMNS).toEqual([
      'Dano causado',
      'Cura',
      'Dano recebido',
      'Golpes finais',
      'Acertos críticos',
    ]);
    expect(combatRows(summary)).toEqual([
      { id: 'toren', name: 'Toren', cells: ['41', '2', '19', '3', '1'] },
    ]);
  });

  it('gives no row to players who only rolled checks', () => {
    expect(combatRows(master)).toEqual([]);
  });
});
