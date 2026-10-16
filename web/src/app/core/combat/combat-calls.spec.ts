import { Code, ConnectError } from '@connectrpc/connect';

import { CombatCalls } from './combat-calls';
import { CombatState } from './combat-state';
import { encounter } from './combat-testing';

function setup(over: { get?: () => Promise<unknown>; shown?: boolean } = {}) {
  const state = new CombatState();
  const enc = encounter({ revision: 3 });
  if (over.shown !== false) {
    state.apply(enc);
  }
  const get = vi.fn(over.get ?? (async () => enc));
  const calls = new CombatCalls({
    api: { get } as never,
    campaignId: () => 'c',
    encounter: () => state.shown(),
    state: () => state,
  });
  return { calls, state, enc, get };
}

describe('CombatCalls', () => {
  it('applies the combat a call answers, and is busy only while it is in the air', async () => {
    const { calls, state } = setup();
    const next = encounter({ revision: 4, round: 9 });
    let busyDuring = false;
    const ok = await calls.run(async () => {
      busyDuring = calls.busy();
      return next;
    });
    expect(ok).toBe(true);
    expect(busyDuring).toBe(true);
    expect(calls.busy()).toBe(false);
    expect(state.shown()?.round).toBe(9);
  });

  it('makes no call without a combat, nor while another is in the air', async () => {
    const none = setup({ shown: false });
    const never = vi.fn();
    expect(await none.calls.run(never)).toBe(false);
    expect(never).not.toHaveBeenCalled();

    const { calls, enc } = setup();
    let release: (e: typeof enc) => void = () => undefined;
    const first = calls.run(() => new Promise((r) => (release = r)));
    expect(await calls.run(never)).toBe(false);
    release(enc);
    expect(await first).toBe(true);
    expect(never).not.toHaveBeenCalled();
  });

  it('says why a call was refused, clears the message on the next call and does not read again for a refusal that is final', async () => {
    const { calls, get } = setup();
    const refused = await calls.run(async () => {
      throw new ConnectError('no', Code.PermissionDenied);
    });
    expect(refused).toBe(false);
    expect(calls.error()).not.toBe('');
    expect(get).not.toHaveBeenCalled();
    expect(await calls.run(async () => encounter({ revision: 5 }))).toBe(true);
    expect(calls.error()).toBe('');
  });

  it.each([
    Code.Aborted,
    Code.FailedPrecondition,
    Code.NotFound,
    Code.Unavailable,
    Code.DeadlineExceeded,
  ])('reads the combat again after code %s, because the screen may be stale', async (code) => {
    const { calls, state, get } = setup({ get: async () => encounter({ revision: 8, round: 7 }) });
    await calls.run(async () => {
      throw new ConnectError('stale', code);
    });
    expect(get).toHaveBeenCalledWith('c');
    expect(state.shown()?.round).toBe(7);
  });

  it('leaves a failed read to the stream', async () => {
    const { calls, get } = setup({
      get: async () => {
        throw new ConnectError('down', Code.Unavailable);
      },
    });
    await expect(
      calls.run(async () => {
        throw new ConnectError('stale', Code.Aborted);
      }),
    ).resolves.toBe(false);
    expect(get).toHaveBeenCalledTimes(1);
  });
});
