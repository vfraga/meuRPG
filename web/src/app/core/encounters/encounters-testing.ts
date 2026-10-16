import { create, type MessageInitShape } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { vi } from 'vitest';

import {
  EncounterBand,
  EncounterBuildBlockedReason,
  EncounterBuildBlockedSchema,
  type EncounterEvaluation,
  EncounterPartyMemberSchema,
  EncounterEvaluationSchema,
  type EncounterLine,
  EncounterLineSchema,
  EncounterWarning,
} from '../../../gen/meurpg/play/v1/encounters_pb';
import type { CreatureSummary } from '../../../gen/meurpg/rules/v1/rules_pb';
import { summary } from '../creatures/creatures-testing';
import type { MonsterGroupSpec } from '../combat/combat-client-types';
import type { BattleRead, GenerateBand, PartyNpcSpec, SavedEncounter } from './encounters-client';

/** The SRD creatures of the artboards (E10-09), with their XP by challenge rating. */
export const OGRE = summary('monster:ogre', 'Ogro', {
  name: 'Ogre',
  type: 'giant',
  challengeRating: '2',
  xp: 450,
  armorClass: 11,
  hitPoints: 59,
});
export const BUGBEAR = summary('monster:bugbear', 'Bugbear', {
  name: 'Bugbear',
  type: 'humanoid',
  challengeRating: '1',
  xp: 200,
});
export const HOBGOBLIN = summary('monster:hobgoblin', 'Hobgoblin', {
  name: 'Hobgoblin',
  type: 'humanoid',
  challengeRating: '1/2',
  xp: 100,
});
export const GOBLIN = summary('monster:goblin', 'Goblin', {
  name: 'Goblin',
  type: 'humanoid',
  challengeRating: '1/4',
  xp: 50,
});
export const BANDIT = summary('monster:bandit', 'Bandido', {
  name: 'Bandit',
  type: 'humanoid',
  typePt: 'humanoide',
  size: 'Medium',
  sizePt: 'Médio',
  challengeRating: '1/8',
  xp: 25,
  armorClass: 12,
  hitPoints: 11,
});

export function line(
  creature: CreatureSummary,
  count: number,
  over: MessageInitShape<typeof EncounterLineSchema> = {},
): EncounterLine {
  return create(EncounterLineSchema, { creature, count, subtotalXp: creature.xp * count, ...over });
}

/** Mirathel's party (artboard 1): Pensantus, Toren and Brisa at level 4, Sálvia at 5. */
export const MIRATHEL = [
  { characterId: 'c1', name: 'Pensantus', level: 4, npc: false },
  { characterId: 'c2', name: 'Toren', level: 4, npc: false },
  { characterId: 'c3', name: 'Brisa', level: 4, npc: false },
  { characterId: 'c4', name: 'Sálvia', level: 5, npc: false },
].map((m) => create(EncounterPartyMemberSchema, m));

/** What the server answers for a few creatures against Mirathel (the numbers of the artboards): budgets 1.250 / 1.875 / 2.600. */
export function evaluation(
  lines: EncounterLine[],
  over: MessageInitShape<typeof EncounterEvaluationSchema> = {},
): EncounterEvaluation {
  const total = lines.reduce((n, l) => n + l.subtotalXp, 0);
  const band =
    total <= 1250
      ? EncounterBand.LOW
      : total <= 1875
        ? EncounterBand.MODERATE
        : total <= 2600
          ? EncounterBand.HIGH
          : EncounterBand.ABOVE_HIGH;
  const above = total > 2600;
  return create(EncounterEvaluationSchema, {
    party: MIRATHEL,
    budget: { low: 1250, moderate: 1875, high: 2600 },
    lines,
    totalXp: total,
    creatureCount: lines.reduce((n, l) => n + l.count, 0),
    band,
    overXp: above ? total - 2600 : 0,
    maxCr: '7',
    lowestLevel: 4,
    warnings: above ? [EncounterWarning.ABOVE_HIGH] : [],
    ...over,
  } as MessageInitShape<typeof EncounterEvaluationSchema>);
}

/** The server's refusal of the builder: `failed_precondition` with the typed reason. */
export function buildBlocked(reason: EncounterBuildBlockedReason): ConnectError {
  return new ConnectError('blocked', Code.FailedPrecondition, undefined, [
    { desc: EncounterBuildBlockedSchema, value: create(EncounterBuildBlockedSchema, { reason }) },
  ]);
}

/** The encounter client of the builder, with what each test needs and a record of what was asked. */
export class FakeEncountersClient {
  /** What `evaluate` answers: built from the entries asked, by `table`. */
  table = new Map<string, CreatureSummary>([
    [OGRE.key, OGRE],
    [BUGBEAR.key, BUGBEAR],
    [HOBGOBLIN.key, HOBGOBLIN],
    [GOBLIN.key, GOBLIN],
    [BANDIT.key, BANDIT],
  ]);
  evaluateCalls: { entries: MonsterGroupSpec[]; party: PartyNpcSpec[] }[] = [];
  /** A promise per call to hold the answer back, in order; empty: answers at once. */
  gates: Promise<void>[] = [];
  evaluateFail: unknown = null;
  /** Extra party members the server adds to the party of the answer (what the extra NPCs would be). */
  budgetFor = (party: readonly PartyNpcSpec[]) =>
    party.length === 0 ? undefined : { low: 1400, moderate: 2100, high: 3000 };

  evaluate = vi.fn(
    async (_c: string, entries: readonly MonsterGroupSpec[], party: readonly PartyNpcSpec[]) => {
      this.evaluateCalls.push({ entries: [...entries], party: [...party] });
      const gate = this.gates.shift();
      if (gate) {
        await gate;
      }
      if (this.evaluateFail) {
        throw this.evaluateFail;
      }
      const lines = entries.map((e) => line(this.table.get(e.creatureKey)!, e.count));
      const budget = this.budgetFor(party);
      return evaluation(
        lines,
        party.length > 0
          ? {
              party: [
                ...MIRATHEL,
                ...party.map((p) => ({
                  characterId: p.characterId,
                  name: p.name || 'Orin, o guia',
                  level: p.level,
                  npc: true,
                })),
              ],
              budget,
              lowestLevel: Math.min(4, ...party.map((p) => p.level)),
              maxCr: String(Math.min(4, ...party.map((p) => p.level)) + 3),
            }
          : {},
      );
    },
  );

  generated: { lines: [CreatureSummary, number][]; seed: number } = {
    lines: [
      [OGRE, 1],
      [GOBLIN, 2],
    ],
    seed: 7731,
  };
  generateCalls: { band: GenerateBand; type: string; seed: number; party: PartyNpcSpec[] }[] = [];
  generateFail: unknown = null;
  /** The seeds `generate` answers with, one per call (then `generated.seed`). */
  seeds: number[] = [];
  generate = vi.fn(
    async (
      _c: string,
      band: GenerateBand,
      type: string,
      seed: number,
      party: readonly PartyNpcSpec[],
    ) => {
      this.generateCalls.push({ band, type, seed, party: [...party] });
      if (this.generateFail) {
        throw this.generateFail;
      }
      return {
        evaluation: evaluation(this.generated.lines.map(([c, n]) => line(c, n))),
        seed: seed || this.seeds.shift() || this.generated.seed,
      };
    },
  );

  swapList: CreatureSummary[] = [];
  swapCalls: { key: string; type: string }[] = [];
  swaps = vi.fn(async (_c: string, key: string, type: string) => {
    this.swapCalls.push({ key, type });
    return this.swapList;
  });

  saves: { pointId: string; encounter: SavedEncounter; party: PartyNpcSpec[] }[] = [];
  saveFail: unknown = null;
  save = vi.fn(
    async (
      _c: string,
      pointId: string,
      encounter: SavedEncounter,
      party: readonly PartyNpcSpec[],
    ) => {
      this.saves.push({ pointId, encounter, party: [...party] });
      if (this.saveFail) {
        throw this.saveFail;
      }
      return {
        evaluation: evaluation(
          encounter.monsters.map((m) => line(this.table.get(m.creatureKey)!, m.count)),
        ),
        updatedAt: new Date(),
      };
    },
  );

  /** What `get` answers by point, and what `list` says each map keeps. */
  battle = new Map<string, BattleRead>();
  kept: { mapPointId: string; creatureCount: number }[] = [];
  getCalls: string[] = [];
  get = vi.fn(async (_c: string, pointId: string) => {
    this.getCalls.push(pointId);
    return this.battle.get(pointId) ?? { encounter: null, evaluation: null, unknownKeys: [] };
  });
  list = vi.fn(async (_c: string, _mapId: string) => this.kept);
  clear = vi.fn(async () => undefined);
}

/** A saved encounter of the artboard (E10-09 state 7): 1 Ogre, 2 Bugbears, 4 Hobgoblins, 6 Goblins. */
export function artboardEncounter(): {
  encounter: SavedEncounter;
  evaluation: EncounterEvaluation;
} {
  const groups: [CreatureSummary, number][] = [
    [OGRE, 1],
    [BUGBEAR, 2],
    [HOBGOBLIN, 4],
    [GOBLIN, 6],
  ];
  return {
    encounter: {
      monsters: groups.map(([c, n]) => ({ creatureKey: c.key, count: n })),
      hp: 'average',
      hidden: true,
    },
    evaluation: evaluation(groups.map(([c, n]) => line(c, n))),
  };
}
