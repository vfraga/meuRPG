import { signal } from '@angular/core';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  DiceRollSchema,
  PendingDamageSchema,
  PendingDamageStatus,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { encounter } from '../../../../core/combat/combat-testing';
import { CastDamage, type CastDamageContext } from './cast-damage';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';

const owed = (id: string, over: Record<string, unknown> = {}) =>
  create(PendingDamageSchema, {
    id,
    targetId: 'g',
    status: PendingDamageStatus.AWAITING_ROLL,
    diceCount: 1,
    diceSides: 8,
    castId: id,
    ...over,
  } as never);

describe('CastDamage', () => {
  let rolled: unknown[][];
  let answer: (id: string) => Promise<unknown>;
  let applied: unknown[];
  let played: unknown[];
  const busy = () => signal(false);

  function damage(resume = [owed('p1'), owed('p2')], over: Partial<CastDamageContext> = {}) {
    return new CastDamage({
      api: fakeCombatClient({
        rollDamage: (...args: unknown[]) => {
          rolled.push(args);
          return answer(args[2] as string);
        },
      }),
      campaignId: 'c',
      encounterId: 'enc',
      resume,
      state: { apply: (e) => applied.push(e) },
      animator: { play: (s) => played.push(s) },
      busy: busy(),
      error: signal(''),
      ...over,
    });
  }

  beforeEach(() => {
    rolled = [];
    applied = [];
    played = [];
    answer = (id) =>
      Promise.resolve({
        encounter: encounter({}),
        pending: owed(id, {
          status: PendingDamageStatus.ROLLED,
          damageTypePt: 'fogo',
          roll: create(DiceRollSchema, { diceCount: 1, diceSides: 8, faces: [5], total: 5 }),
        }),
        cast: [owed('extra', { status: PendingDamageStatus.ROLLED })],
      });
  });

  it('starts with the damages a closed sheet left, grouped by cast', () => {
    const d = damage();
    expect([...d.pendings().keys()]).toEqual(['p1', 'p2']);
    expect(d.groups().length).toBe(2);
    expect(d.owed()).toBe(true);
    expect(damage([]).owed()).toBe(false);
  });

  it('takes the damages the cast made', () => {
    const d = damage([]);
    d.set([owed('a'), owed('b')]);
    expect([...d.pendings().keys()]).toEqual(['a', 'b']);
  });

  it('rolls every group in the app, one call each, applying and animating each answer', async () => {
    const d = damage();
    await d.rollAll();
    expect(rolled.map((r) => [r[2], r[3]])).toEqual([
      ['p1', { inApp: true }],
      ['p2', { inApp: true }],
    ]);
    expect(applied.length).toBe(2);
    expect(played.length).toBe(2);
    expect(d.pendings().get('p1')?.status).toBe(PendingDamageStatus.ROLLED);
    expect(d.pendings().has('extra')).toBe(true);
  });

  it('stops at the first roll that fails and says why', async () => {
    answer = () => Promise.reject(new ConnectError('x', Code.Unavailable));
    const error = signal('');
    const d = damage(undefined, { error });
    await d.rollAll();
    expect(rolled.length).toBe(1);
    expect(error()).not.toBe('');
  });

  it('rolls the typed sum for the first group, and tells whether it worked', async () => {
    const d = damage();
    expect(await d.rollTyped(6)).toBe(true);
    expect(rolled[0][3]).toEqual({ sum: 6 });
    expect(await damage([]).rollTyped(6)).toBe(false);
    answer = () => Promise.reject(new Error('no'));
    expect(await damage().rollTyped(6)).toBe(false);
  });

  it('keeps one key for the same die until it works, and makes another for another die', async () => {
    answer = () => Promise.reject(new Error('lost answer'));
    const d = damage();
    await d.rollTyped(6);
    await d.rollTyped(6);
    await d.rollTyped(7);
    expect(rolled[0][4]).toBe(rolled[1][4]);
    expect(rolled[2][4]).not.toBe(rolled[0][4]);
  });

  it('rolls nothing while a request is in the air, and frees the sheet after one', async () => {
    const flag = signal(true);
    const d = damage(undefined, { busy: flag });
    expect(await d.rollGroup([owed('p1')], { inApp: true })).toBe(false);
    expect(rolled).toEqual([]);
    flag.set(false);
    await d.rollGroup([owed('p1')], { inApp: true });
    expect(flag()).toBe(false);
  });

  it('animates nothing when the damage has no roll to draw', async () => {
    answer = (id) => Promise.resolve({ encounter: encounter({}), pending: owed(id), cast: [] });
    await damage().rollAll();
    expect(played).toEqual([]);
  });
});
