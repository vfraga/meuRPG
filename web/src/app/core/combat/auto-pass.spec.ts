import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { Code, ConnectError } from '@connectrpc/connect';
import { vi } from 'vitest';

import { CombatantKind, type Encounter, ReactionKind } from '../../../gen/meurpg/play/v1/combat_pb';
import { MasterAutoPass, autoPassOffers, masterWaitNote } from './auto-pass';
import { CombatState } from './combat-state';
import { combatant, encounter, reactionWindow } from './combat-testing';
import { AUTO_PASS_SECONDS, autoPassKey } from './reaction-autopass';

const toren = combatant({ id: 't', label: 'Toren', kind: CombatantKind.PLAYER, mine: true });
const goblin = combatant({ id: 'g', label: 'Goblin' });
const shield = (id = 'w1') =>
  reactionWindow({
    id,
    kind: ReactionKind.SHIELD,
    reactorId: 't',
    reactorIsPlayer: true,
    answerNow: true,
  });
const offer = (id: string, reactorId: string) =>
  ({ id, reactorId, moverId: 'g', forYou: true }) as never;
const withWindows = (over: Partial<Encounter> = {}) =>
  encounter({ revision: 2, combatants: [toren, goblin], reactionWindows: [shield()], ...over });

function setup(opts: { master?: boolean; enc?: Encounter } = {}) {
  vi.useFakeTimers({ now: new Date('2026-10-16T12:00:00Z') });
  const state = new CombatState();
  state.apply(opts.enc ?? withWindows());
  const master = signal(opts.master ?? true);
  const api = {
    answerReaction: vi.fn(async () => ({ encounter: encounter({ revision: 9, round: 7 }) })),
    skipOpportunity: vi.fn(async () => encounter({ revision: 9, round: 8 })),
  };
  const pass = TestBed.runInInjectionContext(
    () =>
      new MasterAutoPass({
        api: api as never,
        campaignId: () => 'c',
        encounter: () => state.shown(),
        isMaster: () => master(),
        state: () => state,
      }),
  );
  TestBed.tick();
  return { pass, state, api, master };
}

describe('MasterAutoPass', () => {
  afterEach(() => vi.useRealTimers());

  it('counts down the optional reaction of a player and passes it with its own key when the time is up', async () => {
    const { pass, state, api } = setup();
    expect(pass.windowsLeft()).toEqual({ w1: AUTO_PASS_SECONDS });
    await vi.advanceTimersByTimeAsync(10_000);
    expect(pass.windowsLeft()).toEqual({ w1: AUTO_PASS_SECONDS - 10 });
    expect(api.answerReaction).not.toHaveBeenCalled();

    await vi.advanceTimersByTimeAsync(20_000);
    expect(api.answerReaction).toHaveBeenCalledTimes(1);
    expect(api.answerReaction).toHaveBeenCalledWith(
      'c',
      'enc',
      'w1',
      { use: false },
      autoPassKey('w1'),
    );
    expect(state.shown()?.round).toBe(7);
  });

  it('runs no clock on a player’s screen', async () => {
    const { pass, api } = setup({ master: false });
    expect(pass.windowsLeft()).toEqual({});
    await vi.advanceTimersByTimeAsync(40_000);
    expect(api.answerReaction).not.toHaveBeenCalled();
    expect(vi.getTimerCount()).toBe(0);
  });

  it('stops the timer when nothing waits any more', async () => {
    const { state } = setup();
    expect(vi.getTimerCount()).toBe(1);
    state.apply(withWindows({ revision: 3, reactionWindows: [] }));
    TestBed.tick();
    expect(vi.getTimerCount()).toBe(0);
  });

  it('counts down the opportunity attacks of a player’s character, never the master’s own', async () => {
    const enc = withWindows({
      reactionWindows: [],
      opportunityOffers: [offer('o1', 't'), offer('o2', 'g')],
    });
    expect(autoPassOffers(enc).map((o) => o.id)).toEqual(['o1']);
    const { pass, api } = setup({ enc });
    expect(pass.offersLeft()).toEqual({ o1: AUTO_PASS_SECONDS });
    await vi.advanceTimersByTimeAsync(30_000);
    expect(api.skipOpportunity).toHaveBeenCalledWith('c', 'enc', 'o1', autoPassKey('o1'));
    expect(api.skipOpportunity).toHaveBeenCalledTimes(1);
  });

  it('sends the pass again after a failure that is the network’s, never after a refusal', async () => {
    const { api } = setup();
    api.answerReaction.mockRejectedValueOnce(new ConnectError('lost', Code.Unavailable));
    await vi.advanceTimersByTimeAsync(31_000);
    expect(api.answerReaction).toHaveBeenCalledTimes(2);

    const refused = setup();
    refused.api.answerReaction.mockRejectedValue(
      new ConnectError('answered', Code.FailedPrecondition),
    );
    await vi.advanceTimersByTimeAsync(40_000);
    expect(refused.api.answerReaction).toHaveBeenCalledTimes(1);
  });

  it('sends the pass again after a failure that is not a Connect error', async () => {
    const { api } = setup();
    api.answerReaction.mockRejectedValueOnce(new TypeError('network'));
    await vi.advanceTimersByTimeAsync(31_000);
    expect(api.answerReaction).toHaveBeenCalledTimes(2);
  });

  it('stops the timer with the screen', () => {
    setup();
    expect(vi.getTimerCount()).toBe(1);
    TestBed.resetTestingModule();
    expect(vi.getTimerCount()).toBe(0);
  });
});

describe('masterWaitNote', () => {
  const name = (id: string) => (id === 'char-t' ? 'Vinicius' : '');

  it('says what the master waits for on a reaction of a player, with the time left of its automatic pass', () => {
    const e = withWindows();
    const plain = masterWaitNote(e, {}, name);
    expect(plain).not.toBe('');
    expect(plain).not.toContain('Passa sozinho');
    expect(masterWaitNote(e, { w1: 12 }, name)).toBe(`${plain} \u00b7 passa sozinho em 12 s`);
  });

  it('says nothing when nothing waits', () => {
    expect(masterWaitNote(encounter({ combatants: [toren, goblin] }), {}, name)).toBe('');
  });

  it('names the player an opportunity attack waits for, before any reaction', () => {
    const e = withWindows({ opportunityOffers: [offer('o1', 't')] });
    expect(masterWaitNote(e, { w1: 5 }, name)).toContain('Vinicius');
  });
});
