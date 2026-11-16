import { create } from '@bufbuild/protobuf';
import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';

import {
  CombatantKind,
  GetMoveOptionsResponseSchema,
  GetTurnOptionsResponseSchema,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import { MountOptionsSchema, TurnOptionsSchema } from '../../../../gen/meurpg/rules/v1/rules_pb';
import { combatant, encounter } from '../../../core/combat/combat-testing';
import { MoveFlow, type MoveFlowContext } from './move-flow';

function setup(mounted: boolean, kind = 'controlled') {
  const tor = combatant({
    id: 'tor',
    label: 'Tor',
    kind: CombatantKind.PLAYER,
    mine: true,
    mountedOnId: mounted ? 'cav' : '',
  });
  const horse = combatant({ id: 'cav', label: 'Cavalo de guerra', mountKind: kind });
  const e = encounter({
    combatants: [tor, horse],
    currentCombatantId: 'tor',
    turnGroupIds: ['tor'],
  });
  const asked: string[] = [];
  const sent: unknown[][] = [];
  const answer = { ...e };
  const moves = {
    moveOptions: (_c: string, _e: string, who: string) => {
      asked.push(who);
      return Promise.resolve(create(GetMoveOptionsResponseSchema, {}));
    },
    move: (...args: unknown[]) => {
      sent.push(['move', ...args]);
      return Promise.resolve({
        encounter: answer,
        stoppedEarly: false,
        lockedDoor: false,
        provoked: false,
      });
    },
    mount: (...args: unknown[]) => {
      sent.push(['mount', ...args]);
      return failing.mount ? Promise.reject(new Error('boom')) : Promise.resolve(answer);
    },
    dismount: (...args: unknown[]) => {
      sent.push(['dismount', ...args]);
      return Promise.resolve({ encounter: answer, stoppedEarly: blocked.value });
    },
  };
  const blocked = { value: false };
  const failing = { mount: false };
  const mountOptions = create(MountOptionsSchema, { mountedOnId: mounted ? 'cav' : '' });
  const ctx = {
    api: { moves },
    calls: { busy: signal(false), error: signal(''), refreshAfter: () => Promise.resolve() },
    campaignId: () => 'camp',
    encounter: () => e,
    state: () => ({
      apply: () => undefined,
      moving: signal(true),
      mapOpen: signal(false),
      applyMove: () => undefined,
    }),
    own: () => tor,
    isMaster: () => false,
    active: () => true,
    theatre: () => false,
    options: () =>
      create(GetTurnOptionsResponseSchema, {
        options: create(TurnOptionsSchema, { mount: mountOptions }),
      }),
    creatureOptions: () => new Map(),
    takeAction: () => Promise.resolve(true),
    openSpend: () => undefined,
  } as unknown as MoveFlowContext;
  const flow = TestBed.runInInjectionContext(() => new MoveFlow(ctx));
  return { flow, asked, sent, blocked, failing, tick: () => TestBed.tick() };
}

describe('MoveFlow, mounts', () => {
  it('reads the mount options of the mover', () => {
    const { flow } = setup(true);
    expect(flow.mount.options()?.mountedOnId).toBe('cav');
  });

  it('asks where the controlled mount can go, and moves the mount for its rider', async () => {
    const { flow, asked, sent, tick } = setup(true);
    tick();
    expect(flow.mount.rideId()).toBe('cav');
    expect(asked).toEqual(['cav']);
    await flow.confirmMove({ col: 8, row: 5 });
    expect(sent[0]).toEqual(['move', 'camp', expect.anything(), 'cav', 8, 5]);
  });

  it('moves the rider itself when the mount keeps its own turn, or when it rides nothing', async () => {
    const independent = setup(true, 'independent');
    independent.tick();
    expect(independent.flow.mount.rideId()).toBe('');
    expect(independent.asked).toEqual(['tor']);
    const walking = setup(false);
    walking.tick();
    await walking.flow.confirmMove({ col: 8, row: 5 });
    expect(walking.sent[0][3]).toBe('tor');
  });

  it('mounts and dismounts with the rider and the square, and keeps the page open', async () => {
    const { flow, sent } = setup(false);
    await flow.mount.mountOn('cav');
    expect(sent[0]).toEqual(['mount', 'camp', expect.anything(), 'tor', 'cav']);
    await flow.mount.dismountTo({ col: 6, row: 6 });
    expect(sent[1]).toEqual(['dismount', 'camp', expect.anything(), 'tor', 6, 6]);
    expect(flow.mount.blocked()).toBe(false);
  });

  it('says so when something unseen held the square, and clears it with the next try', async () => {
    const { flow, blocked } = setup(true);
    blocked.value = true;
    await flow.mount.dismountTo({ col: 6, row: 6 });
    expect(flow.mount.blocked()).toBe(true);
    blocked.value = false;
    await flow.mount.dismountTo({ col: 7, row: 6 });
    expect(flow.mount.blocked()).toBe(false);
  });

  it('drops the warning when the page closes, and when the page opens for a creature', async () => {
    const { flow, blocked } = setup(true);
    blocked.value = true;
    await flow.mount.dismountTo({ col: 6, row: 6 });
    flow.closeFullPage();
    expect(flow.mount.blocked()).toBe(false);
    await flow.mount.dismountTo({ col: 6, row: 6 });
    expect(flow.mount.blocked()).toBe(true);
    flow.openCreature('cav');
    expect(flow.mount.blocked()).toBe(false);
  });

  it('keeps the page open and says why when mounting is refused, and forgets it on the next try', async () => {
    const { flow, failing } = setup(false);
    failing.mount = true;
    await flow.mount.mountOn('cav');
    expect(flow.moveError()).not.toBe('');
    failing.mount = false;
    await flow.mount.mountOn('cav');
    expect(flow.moveError()).toBe('');
  });

  it('drops the warning of a failed dismount once a move works', async () => {
    const { flow, blocked } = setup(true);
    blocked.value = true;
    await flow.mount.dismountTo({ col: 6, row: 6 });
    expect(flow.mount.blocked()).toBe(true);
    await flow.confirmMove({ col: 8, row: 5 });
    expect(flow.mount.blocked()).toBe(false);
  });
});
