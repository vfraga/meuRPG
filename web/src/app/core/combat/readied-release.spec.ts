import { create } from '@bufbuild/protobuf';

import {
  CombatantState,
  GetTurnOptionsResponseSchema,
  ReadiedActionSchema,
  ReadiedKind,
  SpellTargetsSchema,
  TargetInReachSchema,
} from '../../../gen/meurpg/play/v1/combat_pb';
import {
  NO_PICK,
  aimedUse,
  releaseComplete,
  releasePlan,
  releaseStart,
  releaseStatus,
  releaseTargets,
  releaseToggled,
} from './readied-release';

const reach = (id: string, label: string, tooFar = false) =>
  create(TargetInReachSchema, {
    combatantId: id,
    label,
    state: CombatantState.UNHURT,
    distanceFt: 5,
    tooFar,
  });

const held = (key: string, named = '') =>
  create(ReadiedActionSchema, {
    id: 'r',
    kind: ReadiedKind.SPELL,
    actionKey: key,
    targetCombatantId: named,
  });

const options = (st: Record<string, unknown>) =>
  create(GetTurnOptionsResponseSchema, { spellTargets: [create(SpellTargetsSchema, st as never)] });

const targets = [reach('tor', 'Tor'), reach('z1', 'Zumbi 1'), reach('z2', 'Zumbi 2', true)];

describe('readied-release (#857: a held spell released from its window)', () => {
  it('has no plan for a spell the server lists nothing for', () => {
    expect(
      releasePlan(options({ spellKey: 'spell:other' }), held('spell:sleep'), 'tor'),
    ).toBeNull();
  });

  it('deals the darts of the held slot and sends the creatures that got one', () => {
    const plan = releasePlan(
      options({
        spellKey: 'spell:magic-missile',
        maxTargets: 3,
        darts: [{ slotLevel: 2, darts: 4 }],
        targets,
      }),
      held('spell:magic-missile'),
      'tor',
    )!;
    expect(plan.rule.kind).toBe('darts');
    expect(plan.total).toBe(4);
    expect(releaseTargets(plan, NO_PICK)).toBeNull();
    expect(releaseStatus(plan, NO_PICK)).toBe('Distribua todos os dardos: 0 de 4.');
    const pick = {
      chosen: [],
      dealt: new Map([
        ['z1', 3],
        ['tor', 1],
      ]),
    };
    expect(releaseComplete(plan, pick)).toBe(true);
    expect(releaseTargets(plan, pick)).toEqual([
      { combatantId: 'z1', darts: 3 },
      { combatantId: 'tor', darts: 1 },
    ]);
  });

  it('deals every dart of a high slot: Magic Missile held at the 9th level has 11, more than the 10 targets a cast lists', () => {
    const plan = releasePlan(
      options({
        spellKey: 'spell:magic-missile',
        maxTargets: 11,
        darts: [{ slotLevel: 9, darts: 11 }],
        targets,
      }),
      held('spell:magic-missile'),
      'tor',
    )!;
    expect(plan.rule.kind).toBe('darts');
    expect(plan.total).toBe(11);
    expect(releaseStatus(plan, NO_PICK)).toBe('Distribua todos os dardos: 0 de 11.');
    const ten = { chosen: [], dealt: new Map([['z1', 10]]) };
    expect(releaseComplete(plan, ten)).toBe(false);
    const all = {
      chosen: [],
      dealt: new Map([
        ['z1', 10],
        ['tor', 1],
      ]),
    };
    expect(releaseComplete(plan, all)).toBe(true);
    expect(releaseTargets(plan, all)).toEqual([
      { combatantId: 'z1', darts: 10 },
      { combatantId: 'tor', darts: 1 },
    ]);
  });

  it('picks several creatures (at least one) and starts with the named one when it can be aimed at', () => {
    const plan = releasePlan(
      options({ spellKey: 'spell:bless', maxTargets: 3, targets }),
      held('spell:bless'),
      'tor',
    )!;
    expect(plan.rule).toMatchObject({ kind: 'multi', min: 1, max: 3 });
    expect(releaseComplete(plan, NO_PICK)).toBe(false);
    expect(releaseStart(plan, 'z1').chosen).toEqual(['z1']);
    expect(releaseStart(plan, 'z2')).toBe(NO_PICK); // out of reach: not chosen
    expect(releaseToggled(plan, { chosen: ['z1'], dealt: new Map() }, 'tor').chosen).toEqual([
      'z1',
      'tor',
    ]);
  });

  it('starts empty for an area that may meet nobody, and sends no targets', () => {
    const plan = releasePlan(
      options({ spellKey: 'spell:sleep', maxTargets: 0, targets }),
      held('spell:sleep'),
      'tor',
    )!;
    expect(plan.rule).toMatchObject({ kind: 'multi', min: 0 });
    expect(releaseStart(plan, 'z1')).toBe(NO_PICK);
    expect(aimedUse(plan, NO_PICK, null)).toEqual({ use: true, die: { inApp: true } });
  });

  it('asks nothing of a spell that stays on its caster', () => {
    const plan = releasePlan(
      options({ spellKey: 'spell:shield-of-faith', maxTargets: 0, targets: [reach('tor', 'Tor')] }),
      held('spell:shield-of-faith'),
      'tor',
    )!;
    expect(plan.rule.kind).toBe('none');
    expect(releaseComplete(plan, NO_PICK)).toBe(true);
  });

  it('answers with the one creature picked when there is no plan', () => {
    expect(aimedUse(null, NO_PICK, 'z1')).toEqual({
      use: true,
      creatureIds: ['z1'],
      die: { inApp: true },
    });
    expect(aimedUse(null, NO_PICK, null)).toEqual({ use: true, die: { inApp: true } });
  });
});
