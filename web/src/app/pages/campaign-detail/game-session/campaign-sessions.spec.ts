import { TestBed } from '@angular/core/testing';

import { CampaignSessions } from './campaign-sessions';
import { GameSessionSource, GameSessionVm } from './game-session-card.types';

function session(sessionNumber: number, ended = true): GameSessionVm {
  return {
    id: `sess-${sessionNumber}`,
    sessionNumber,
    startedAt: new Date(2026, 8, sessionNumber, 19, 0),
    endedAt: ended ? new Date(2026, 8, sessionNumber, 22, 0) : null,
  };
}

describe('CampaignSessions', () => {
  const listSessions = vi.fn();
  let store: CampaignSessions;

  beforeEach(() => {
    listSessions.mockReset().mockResolvedValue([session(3, false), session(2), session(1)]);
    TestBed.configureTestingModule({
      providers: [CampaignSessions, { provide: GameSessionSource, useValue: { listSessions } }],
    });
    store = TestBed.inject(CampaignSessions);
  });

  it('splits the one list into the open session and the ended ones, newest first', async () => {
    await store.ensureLoaded('c1');
    expect(store.open()?.sessionNumber).toBe(3);
    expect(store.ended().map((s) => s.sessionNumber)).toEqual([2, 1]);
  });

  it('reads the list once for however many panels ask', async () => {
    await Promise.all([store.ensureLoaded('c1'), store.ensureLoaded('c1')]);
    await store.ensureLoaded('c1');
    expect(listSessions).toHaveBeenCalledTimes(1);
  });

  it('reads again for another campaign', async () => {
    await store.ensureLoaded('c1');
    await store.ensureLoaded('c2');
    expect(listSessions).toHaveBeenCalledTimes(2);
    expect(listSessions).toHaveBeenLastCalledWith('c2');
  });

  it('puts a session that just ended at the top of the ended ones, in place of the open one', async () => {
    await store.ensureLoaded('c1');
    store.put({ ...session(3, false), endedAt: new Date(2026, 8, 3, 22, 0) });
    expect(store.open()).toBeNull();
    expect(store.ended().map((s) => s.sessionNumber)).toEqual([3, 2, 1]);
  });

  it('keeps the list on screen during a quiet reload and drops the answer of an older read', async () => {
    await store.ensureLoaded('c1');
    let resolveSlow!: (list: GameSessionVm[]) => void;
    listSessions.mockReturnValueOnce(new Promise((r) => (resolveSlow = r)));
    const slow = store.reload('c1', true);
    expect(store.state().status).toBe('ready');
    listSessions.mockResolvedValueOnce([session(5, false)]);
    await store.reload('c1', true);
    resolveSlow([session(9)]);
    await slow;
    expect(store.open()?.sessionNumber).toBe(5);
  });

  it('keeps the error and reads again on request', async () => {
    const failure = new Error('down');
    listSessions.mockRejectedValueOnce(failure);
    await store.ensureLoaded('c1');
    expect(store.state()).toEqual({ status: 'error', error: failure });
    await store.reload();
    expect(store.ended()).toHaveLength(2);
  });
});
