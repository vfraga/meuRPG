import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { Code, ConnectError } from '@connectrpc/connect';

import { AuthService, AuthState } from '../../core/auth/auth.service';
import {
  OPEN_SESSIONS_FETCHER,
  OpenSessionVm,
  OpenSessions,
  POLL_INTERVAL_MS,
  sessionForLiveLink,
  sessionsToAnnounce,
} from './open-sessions';

function open(id: string, campaignId: string, isMaster = false, number = 4): OpenSessionVm {
  return {
    sessionId: id,
    campaignId,
    campaignName: `Campanha ${campaignId}`,
    sessionNumber: number,
    startedAt: new Date('2026-09-30T23:05:00Z'),
    isMaster,
  };
}

function setVisibility(state: 'visible' | 'hidden'): void {
  Object.defineProperty(document, 'visibilityState', { value: state, configurable: true });
  document.dispatchEvent(new Event('visibilitychange'));
}

describe('OpenSessions (the RN-06 poll)', () => {
  const auth = signal<AuthState>({ status: 'unknown' });
  let fetch: ReturnType<typeof vi.fn<() => Promise<OpenSessionVm[]>>>;
  let service: OpenSessions;
  let refresh: ReturnType<typeof vi.fn<() => Promise<void>>>;

  beforeEach(() => {
    vi.useFakeTimers();
    setVisibility('visible');
    auth.set({ status: 'unknown' });
    fetch = vi.fn(() => Promise.resolve([open('s1', 'c1')]));
    // GetMe, read again, finds the session gone.
    refresh = vi.fn(() => {
      auth.set({ status: 'signed-out' });
      return Promise.resolve();
    });
    TestBed.configureTestingModule({
      providers: [
        { provide: AuthService, useValue: { state: auth.asReadonly(), refresh } },
        { provide: OPEN_SESSIONS_FETCHER, useValue: () => Promise.resolve(fetch) },
      ],
    });
    service = TestBed.inject(OpenSessions);
  });

  afterEach(() => {
    vi.useRealTimers();
    setVisibility('visible');
  });

  async function signIn(): Promise<void> {
    auth.set({
      status: 'signed-in',
      user: { id: 'u1', displayName: null },
      sessionExpiresAt: null,
    });
    TestBed.tick();
    await vi.advanceTimersByTimeAsync(0);
  }

  it('asks nothing while signed out', async () => {
    TestBed.tick();
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS * 3);
    expect(fetch).not.toHaveBeenCalled();
    expect(service.sessions()).toEqual([]);
  });

  it('asks once on sign-in, then every 30 seconds while the tab is visible', async () => {
    await signIn();
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(service.sessions().map((s) => s.sessionId)).toEqual(['s1']);
    expect(service.liveCampaignIds().has('c1')).toBe(true);

    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS - 1);
    expect(fetch).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(fetch).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS);
    expect(fetch).toHaveBeenCalledTimes(3);
  });

  it('stops while the tab is hidden, and asks at once when it is visible again', async () => {
    await signIn();
    expect(fetch).toHaveBeenCalledTimes(1);

    setVisibility('hidden');
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS * 5);
    expect(fetch).toHaveBeenCalledTimes(1);

    setVisibility('visible');
    await vi.advanceTimersByTimeAsync(0);
    expect(fetch).toHaveBeenCalledTimes(2);
    // …and the 30-second rhythm starts again from there.
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS);
    expect(fetch).toHaveBeenCalledTimes(3);
  });

  it('keeps what it had when a call fails, and tries again 30 seconds later', async () => {
    await signIn();
    fetch.mockRejectedValueOnce(new Error('network'));
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS);
    expect(fetch).toHaveBeenCalledTimes(2);
    expect(service.sessions().map((s) => s.sessionId)).toEqual(['s1']);
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS);
    expect(fetch).toHaveBeenCalledTimes(3);
  });

  it('reads the login session again when the poll is unauthenticated, then stops', async () => {
    await signIn();
    fetch.mockRejectedValue(new ConnectError('no session', Code.Unauthenticated));
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS);
    TestBed.tick();
    expect(refresh).toHaveBeenCalledTimes(1);
    expect(service.sessions()).toEqual([]);
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS * 3);
    expect(fetch).toHaveBeenCalledTimes(2);
  });

  it('does not read the login session again for a network failure', async () => {
    await signIn();
    fetch.mockRejectedValueOnce(new Error('network'));
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS);
    expect(refresh).not.toHaveBeenCalled();
  });

  it('forgets everything and stops on sign-out', async () => {
    await signIn();
    auth.set({ status: 'signed-out' });
    TestBed.tick();
    expect(service.sessions()).toEqual([]);
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS * 3);
    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it('refresh() asks now and restarts the wait', async () => {
    await signIn();
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS / 2);
    await service.refresh();
    expect(fetch).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS / 2);
    expect(fetch).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS / 2);
    expect(fetch).toHaveBeenCalledTimes(3);
  });

  it('remembers closed notices in memory only', async () => {
    service.dismiss('s1');
    expect(service.dismissed().has('s1')).toBe(true);
    expect(sessionStorage.length + localStorage.length).toBe(0);
  });
});

describe('sessionsToAnnounce', () => {
  // Newest first, as ListOpenGameSessions answers.
  const sessions = [open('s2', 'c2'), open('s1', 'c1')];
  const ids = (list: readonly OpenSessionVm[]) => list.map((s) => s.sessionId);

  it('announces every session the person plays in, oldest first', () => {
    expect(ids(sessionsToAnnounce(sessions, new Set(), '/campaigns'))).toEqual(['s1', 's2']);
  });

  it('a session that starts later goes below, so the first notice never moves', () => {
    const later = [open('s3', 'c3'), ...sessions];
    expect(ids(sessionsToAnnounce(later, new Set(), '/'))).toEqual(['s1', 's2', 's3']);
  });

  it('leaves out closed notices', () => {
    expect(ids(sessionsToAnnounce(sessions, new Set(['s2']), '/'))).toEqual(['s1']);
    expect(sessionsToAnnounce(sessions, new Set(['s1', 's2']), '/')).toEqual([]);
  });

  it("doesn't announce a session on its campaign's page (its Sessão panel says it)", () => {
    expect(ids(sessionsToAnnounce(sessions, new Set(), '/campaigns/c2'))).toEqual(['s1']);
    expect(ids(sessionsToAnnounce(sessions, new Set(), '/campaigns/c2?x=1#y'))).toEqual(['s1']);
  });

  it('announces on the pages under a campaign, such as a sheet', () => {
    expect(ids(sessionsToAnnounce(sessions, new Set(), '/campaigns/c2/characters/p1'))).toEqual([
      's1',
      's2',
    ]);
  });

  it('never announces on a session page', () => {
    expect(sessionsToAnnounce(sessions, new Set(), '/campaigns/c2/session')).toEqual([]);
    expect(sessionsToAnnounce(sessions, new Set(), '/campaigns/c9/session')).toEqual([]);
  });

  it("doesn't announce to the master the session they started", () => {
    expect(sessionsToAnnounce([open('s3', 'c3', true)], new Set(), '/')).toEqual([]);
  });
});

describe('sessionForLiveLink', () => {
  it('links to the newest open session, as master or player', () => {
    const sessions = [open('s3', 'c3', true), open('s1', 'c1')];
    expect(sessionForLiveLink(sessions, '/profile')?.sessionId).toBe('s3');
  });

  it('shows nothing on a session page, whose status line has its own pill', () => {
    expect(sessionForLiveLink([open('s1', 'c1')], '/campaigns/c1/session')).toBeNull();
  });

  it('shows nothing without an open session', () => {
    expect(sessionForLiveLink([], '/')).toBeNull();
  });
});
