import { create } from '@bufbuild/protobuf';

import { DiceMode } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  AttackTargetsSchema,
  CombatantState,
  GetTurnOptionsResponseSchema,
  ReadiedActionSchema,
  ReadiedKind,
  TargetInReachSchema,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { AdvantageSourceSchema, RollMode } from '../../../gen/meurpg/play/v1/combat_rolls_pb';
import {
  AttackKind,
  AttackOptionSchema,
  SpellDetailsSchema,
  SpellHitPointEffectKind,
  TurnOptionsSchema,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import { releaseAnswerOf, releaseChosenOf, releaseRollAsks, releaseRollOf } from './release-roll';
import { tight } from '../format/text';
import type { ReleasePlan } from './readied-release';

const held = (over: Record<string, unknown> = {}) =>
  create(ReadiedActionSchema, {
    id: 'r1',
    kind: ReadiedKind.ATTACK,
    actionKey: 'equipment:longsword',
    actionNamePt: 'Espada longa',
    targetCombatantId: 'ogro',
    ...over,
  } as never);

const reach = (id: string, mode: RollMode, sources: unknown[] = [], feet = 5) =>
  create(TargetInReachSchema, {
    combatantId: id,
    label: id === 'ogro' ? 'Ogro' : 'Zumbi 1',
    state: CombatantState.UNHURT,
    distanceFt: feet,
    rollMode: mode,
    sources: sources.map((s) => create(AdvantageSourceSchema, s as never)),
  });

const options = (key: string, targets = [reach('ogro', RollMode.NORMAL)], bonus = 6) =>
  create(GetTurnOptionsResponseSchema, {
    options: create(TurnOptionsSchema, {
      attacks: [
        create(AttackOptionSchema, {
          enabled: true,
          attack: { key, namePt: 'Espada longa', kind: AttackKind.WEAPON, attackBonus: bonus },
        } as never),
      ],
    }),
    attackTargets: [create(AttackTargetsSchema, { attackKey: key, targets } as never)],
  });

const sleep = create(SpellDetailsSchema, {
  spell: { key: 'spell:sleep', level: 1 },
  hitPointEffect: {
    kind: SpellHitPointEffectKind.POOL,
    poolDiceCount: 5,
    poolDiceSides: 8,
    poolDicePerLevel: 2,
  },
} as never);

const plan = (over: Partial<ReleasePlan> = {}): ReleasePlan => ({
  rule: { kind: 'multi', min: 1, max: 5 },
  rows: [],
  total: 0,
  placement: 0,
  onMap: false,
  wall: false,
  ...over,
});

describe('releaseRollOf (#938: what the roll step of a released readied action asks)', () => {
  it('asks the d20 of a held weapon attack, with the attack bonus and the words of the attack sheet', () => {
    const roll = releaseRollOf({
      readied: held(),
      options: options('equipment:longsword'),
      plan: null,
      target: 'ogro',
      details: null,
    });
    expect(roll).toMatchObject({
      kind: 'attack',
      faces: 1,
      bonus: 6,
      label: 'Role 1d20 para Espada longa (+6)',
      hint: 'Role o seu dado e digite o número que saiu (1 a 20).',
      totalNote: 'Total do ataque',
    });
  });

  it('asks the pair of d20 with disadvantage, the lower counting, and keeps the circumstances the server lists', () => {
    const src = { effect: RollMode.DISADVANTAGE, textPt: 'Ogro a mais de 1,5 m' };
    const roll = releaseRollOf({
      readied: held(),
      options: options('equipment:longsword', [reach('ogro', RollMode.DISADVANTAGE, [src])]),
      plan: null,
      target: 'ogro',
      details: null,
    });
    expect(roll).toMatchObject({ kind: 'attack', faces: 2, combine: 'lower' });
    expect(roll?.kind === 'attack' && roll.sources.map((s) => s.textPt)).toEqual([
      'Ogro a mais de 1,5 m',
    ]);
    expect(roll?.kind === 'attack' && roll.hint).toContain('Conta o menor.');
    const adv = releaseRollOf({
      readied: held(),
      options: options('equipment:longsword', [reach('ogro', RollMode.ADVANTAGE)]),
      plan: null,
      target: 'ogro',
      details: null,
    });
    expect(adv).toMatchObject({ faces: 2, combine: 'higher' });
  });

  it('asks nothing while no creature is chosen or the options are not read', () => {
    const base = { readied: held(), plan: null, details: null };
    expect(
      releaseRollOf({ ...base, options: options('equipment:longsword'), target: null }),
    ).toBeNull();
    expect(releaseRollOf({ ...base, options: null, target: 'ogro' })).toBeNull();
  });

  it('asks the d20 of a held spell attack the server lists targets for', () => {
    const roll = releaseRollOf({
      readied: held({
        kind: ReadiedKind.SPELL,
        actionKey: 'spell:fire-bolt',
        actionNamePt: 'Raio de Fogo',
      }),
      options: options('spell:fire-bolt', [reach('ogro', RollMode.NORMAL)], 7),
      plan: null,
      target: 'ogro',
      details: null,
    });
    expect(roll).toMatchObject({
      kind: 'attack',
      bonus: 7,
      label: 'Role 1d20 para Raio de Fogo (+7)',
    });
  });

  it('leaves a held spell with several rays to the piece of the rays (the answer is as it is today)', () => {
    const roll = releaseRollOf({
      readied: held({
        kind: ReadiedKind.SPELL,
        actionKey: 'spell:scorching-ray',
        actionNamePt: 'Raio Ardente',
      }),
      options: options('spell:scorching-ray'),
      plan: plan({ rule: { kind: 'darts', min: 1, max: 3, rays: true }, total: 3 }),
      target: null,
      details: null,
    });
    expect(roll).toBeNull();
  });

  it('asks the sum of the pool of Sono at the slot it was held with, in a combat without a map', () => {
    const roll = releaseRollOf({
      readied: held({
        kind: ReadiedKind.SPELL,
        actionKey: 'spell:sleep',
        actionNamePt: 'Sono',
        slotLevel: 2,
      }),
      options: create(GetTurnOptionsResponseSchema, {}),
      plan: plan(),
      target: null,
      details: sleep,
    });
    expect(roll).toMatchObject({
      kind: 'pool',
      min: 7,
      max: 56,
      label: 'Role 7d8 para Sono: some os dados',
      hint: 'Digite a soma dos dados, de 7 a 56.',
      totalNote: 'Total dos dados',
    });
  });

  it('releases a pool spell placed on the map from the map, with no step of its own', () => {
    const roll = releaseRollOf({
      readied: held({
        kind: ReadiedKind.SPELL,
        actionKey: 'spell:sleep',
        actionNamePt: 'Sono',
        slotLevel: 1,
      }),
      options: create(GetTurnOptionsResponseSchema, {}),
      plan: plan({ onMap: true }),
      target: null,
      details: sleep,
    });
    expect(roll).toBeNull();
  });

  it('asks nothing of another action, or of a spell with no roll at the cast', () => {
    expect(
      releaseRollOf({
        readied: held({ kind: ReadiedKind.OTHER, actionKey: 'dodge' }),
        options: create(GetTurnOptionsResponseSchema, {}),
        plan: null,
        target: null,
        details: null,
      }),
    ).toBeNull();
    expect(
      releaseRollOf({
        readied: held({ kind: ReadiedKind.SPELL, actionKey: 'spell:bless' }),
        options: create(GetTurnOptionsResponseSchema, {}),
        plan: plan(),
        target: null,
        details: create(SpellDetailsSchema, { spell: { key: 'spell:bless', level: 1 } } as never),
      }),
    ).toBeNull();
  });
});

describe('releaseRollAsks (#938: the table that types the dice)', () => {
  it('has the step at a table with physical dice or where the players choose, never where the app rolls', () => {
    expect(releaseRollAsks(DiceMode.PHYSICAL)).toBe(true);
    expect(releaseRollAsks(DiceMode.PLAYERS_CHOOSE)).toBe(true);
    expect(releaseRollAsks(DiceMode.APP)).toBe(false);
  });
});

describe('releaseChosenOf (#938: the one line of what is released)', () => {
  const atk = releaseRollOf({
    readied: held(),
    options: options('equipment:longsword'),
    plan: null,
    target: 'ogro',
    details: null,
  })!;

  it('names the attack, the creature and the distance', () => {
    expect(releaseChosenOf(atk, held(), [{ id: 'ogro', label: 'Ogro' }], 5)).toEqual({
      icon: 'swords',
      caption: 'Ataque preparado',
      lead: 'Espada longa',
      rest: tight(' no Ogro · a 1,5 m'),
    });
  });

  it('says no distance when the combat has no map, and "na" for a feminine name', () => {
    expect(releaseChosenOf(atk, held(), [{ id: 'b', label: 'Aranha' }], undefined).rest).toBe(
      ' na Aranha',
    );
  });

  it('names the held spell with its slot and every creature of the pool', () => {
    const pool = releaseRollOf({
      readied: held({
        kind: ReadiedKind.SPELL,
        actionKey: 'spell:sleep',
        actionNamePt: 'Sono',
        slotLevel: 1,
      }),
      options: create(GetTurnOptionsResponseSchema, {}),
      plan: plan(),
      target: null,
      details: sleep,
    })!;
    expect(
      releaseChosenOf(
        pool,
        held({ kind: ReadiedKind.SPELL, actionNamePt: 'Sono', slotLevel: 1 }),
        [
          { id: 'ogro', label: 'Ogro' },
          { id: 'z1', label: 'Zumbi 1' },
        ],
        undefined,
      ),
    ).toEqual({
      icon: 'auto_fix_high',
      caption: 'Magia preparada · espaço de 1º nível',
      lead: 'Sono',
      rest: ' no Ogro e no Zumbi 1',
    });
  });
});

describe('releaseAnswerOf (#938: what the window sends)', () => {
  const base = { use: true, creatureIds: ['ogro'] } as const;
  const one = releaseRollOf({
    readied: held(),
    options: options('equipment:longsword'),
    plan: null,
    target: 'ogro',
    details: null,
  })!;
  const pair = releaseRollOf({
    readied: held(),
    options: options('equipment:longsword', [reach('ogro', RollMode.DISADVANTAGE)]),
    plan: null,
    target: 'ogro',
    details: null,
  })!;
  const pool = releaseRollOf({
    readied: held({ kind: ReadiedKind.SPELL, actionKey: 'spell:sleep', slotLevel: 1 }),
    options: create(GetTurnOptionsResponseSchema, {}),
    plan: plan(),
    target: null,
    details: sleep,
  })!;

  it('sends the one typed face, the pair, the pool sum, and the faces an effect adds', () => {
    expect(releaseAnswerOf(base, one, { typed: [14], extra: [] })).toEqual({
      ...base,
      die: { typed: 14 },
    });
    expect(releaseAnswerOf(base, pair, { typed: [4, 17], extra: [3] })).toEqual({
      ...base,
      die: { d20Faces: [4, 17] },
      extraDieFaces: [3],
    });
    expect(releaseAnswerOf({ use: true }, pool, { typed: [23], extra: [] })).toEqual({
      use: true,
      die: { poolSum: 23 },
    });
  });

  it('sends the dice of the app, and the table dice with it, when the player chose to roll in the app', () => {
    expect(releaseAnswerOf(base, one, 'app')).toEqual({ ...base, die: { inApp: true } });
  });
});
