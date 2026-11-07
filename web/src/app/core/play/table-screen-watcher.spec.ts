import { TestBed } from '@angular/core/testing';
import { Code, ConnectError } from '@connectrpc/connect';

import { nextTurn } from '../../../test-settle';
import { TableScreenClient } from './table-screen-client';
import { TV_DEBOUNCE_MS, TV_CLOCK, TableScreenWatcher, tvEvents } from './table-screen-watcher';
import { TV_LATE_GRACE_MS, TV_RETRY_MS, TV_STALE_MS } from './table-screen-state';
import {
  FakeClock,
  FakeWatchCall,
  noOpenSession,
  screenResponse,
  tvWire,
} from './table-screen-testing';

function setVisibility(state: 'visible' | 'hidden'): void {
  Object.defineProperty(document, 'visibilityState', { value: state, configurable: true });
  document.dispatchEvent(new Event('visibilitychange'));
}

describe('TableScreenWatcher', () => {
  let clock: FakeClock;
  let calls: FakeWatchCall[];
  let gets: ReturnType<typeof vi.fn>;
  let watcher: TableScreenWatcher;

  const last = () => calls[calls.length - 1];
  /** Lets the stream's events, the reads and the effects run. */
  async function settleAll(): Promise<void> {
    await nextTurn();
    await nextTurn();
    TestBed.tick();
  }

  beforeEach(() => {
    vi.useFakeTimers();
    setVisibility('visible');
    clock = new FakeClock(new Date(2026, 0, 5, 21, 10));
    calls = [];
    gets = vi.fn(() => Promise.resolve(screenResponse()));
    TestBed.configureTestingModule({
      providers: [
        TableScreenWatcher,
        { provide: TV_CLOCK, useValue: clock },
        {
          provide: TableScreenClient,
          useValue: {
            get: gets,
            watch: (_: string, signal: AbortSignal) => {
              const call = new FakeWatchCall(signal);
              calls.push(call);
              return call.events();
            },
          },
        },
      ],
    });
    watcher = TestBed.inject(TableScreenWatcher);
  });

  afterEach(() => {
    watcher.stop();
    vi.useRealTimers();
  });

  it('reads the screen on `ready`', async () => {
    watcher.start('c1');
    await settleAll();
    expect(watcher.state.phase().kind).toBe('loading');
    last().push('ready');
    await settleAll();
    expect(gets).toHaveBeenCalledTimes(1);
    expect(gets).toHaveBeenCalledWith('c1');
    expect(watcher.state.phase().kind).toBe('ready');
  });

  it('reads again after `changed`, once per burst', async () => {
    watcher.start('c1');
    await settleAll();
    last().push('ready');
    await settleAll();
    for (let i = 0; i < 5; i++) {
      last().push('changed');
    }
    await settleAll();
    expect(gets).toHaveBeenCalledTimes(1);
    clock.advance(TV_DEBOUNCE_MS);
    await settleAll();
    expect(gets).toHaveBeenCalledTimes(2);
  });

  it('makes one more read, not many, for a hint that comes during a read', async () => {
    watcher.start('c1');
    await settleAll();
    last().push('ready');
    await settleAll();
    let finish = (): void => undefined;
    gets.mockReturnValueOnce(
      new Promise((resolve) => {
        finish = () => resolve(screenResponse());
      }),
    );
    last().push('changed');
    await settleAll();
    clock.advance(TV_DEBOUNCE_MS);
    await settleAll();
    expect(gets).toHaveBeenCalledTimes(2);
    for (let i = 0; i < 4; i++) {
      last().push('changed');
      await settleAll();
      clock.advance(TV_DEBOUNCE_MS);
      await settleAll();
    }
    // The first read is still on its way: nothing more started.
    expect(gets).toHaveBeenCalledTimes(2);
    finish();
    await settleAll();
    expect(gets).toHaveBeenCalledTimes(3);
  });

  it('keeps the last screen while reconnecting, hides it after a minute and returns on `ready`', async () => {
    watcher.start('c1');
    await settleAll();
    last().push('ready');
    await settleAll();
    last().fail(new ConnectError('down', Code.Unavailable));
    await settleAll();
    expect(watcher.state.phase()).toMatchObject({ kind: 'ready', late: null });
    clock.advance(TV_LATE_GRACE_MS);
    const phase = watcher.state.phase();
    expect(phase.kind === 'ready' && phase.late).toContain('atrasada desde 21:10');
    clock.advance(TV_STALE_MS);
    expect(watcher.state.phase().kind).toBe('stale');
    await vi.advanceTimersByTimeAsync(2000);
    await settleAll();
    expect(calls).toHaveLength(2);
    last().push('ready');
    await settleAll();
    expect(gets).toHaveBeenCalledTimes(2);
    expect(watcher.state.phase()).toMatchObject({ kind: 'ready', late: null });
  });

  it('shows the error and retries the read every 10 s', async () => {
    gets.mockRejectedValueOnce(new ConnectError('down', Code.Unavailable));
    watcher.start('c1');
    await settleAll();
    last().push('ready');
    await settleAll();
    expect(watcher.state.phase().kind).toBe('error');
    clock.advance(TV_RETRY_MS);
    await settleAll();
    expect(gets).toHaveBeenCalledTimes(2);
    expect(watcher.state.phase().kind).toBe('ready');
  });

  it('marks the screen late when a read fails after one worked, and reads again in 10 s', async () => {
    watcher.start('c1');
    await settleAll();
    last().push('ready');
    await settleAll();
    gets.mockRejectedValueOnce(new ConnectError('down', Code.Unavailable));
    last().push('changed');
    await settleAll();
    clock.advance(TV_DEBOUNCE_MS);
    await settleAll();
    expect(watcher.state.phase().kind).toBe('ready');
    clock.advance(TV_LATE_GRACE_MS);
    const phase = watcher.state.phase();
    expect(phase.kind === 'ready' && phase.late).toContain('atrasada');
    clock.advance(TV_RETRY_MS);
    await settleAll();
    expect(gets).toHaveBeenCalledTimes(3);
    expect(watcher.state.phase()).toMatchObject({ kind: 'ready', late: null });
  });

  it('shows a lost sign-in, not the refusal, and stops', async () => {
    watcher.start('c1');
    await settleAll();
    last().fail(new ConnectError('who', Code.Unauthenticated));
    await settleAll();
    expect(watcher.state.phase().kind).toBe('signed-out');
    await vi.advanceTimersByTimeAsync(60_000);
    expect(calls).toHaveLength(1);
  });

  it('stops reading when a read says the sign-in is gone', async () => {
    gets.mockRejectedValueOnce(new ConnectError('who', Code.Unauthenticated));
    watcher.start('c1');
    await settleAll();
    last().push('ready');
    await settleAll();
    expect(watcher.state.phase().kind).toBe('signed-out');
    expect(last().aborted).toBe(true);
    clock.advance(TV_RETRY_MS * 3);
    await settleAll();
    expect(gets).toHaveBeenCalledTimes(1);
  });

  it('ends on `session_ended` and stops the stream', async () => {
    watcher.start('c1');
    await settleAll();
    last().push('ready');
    await settleAll();
    last().push('sessionEnded');
    await settleAll();
    expect(watcher.state.phase()).toMatchObject({
      kind: 'ended',
      title: 'A sessão 3 terminou às 21:10.',
    });
    expect(last().aborted).toBe(true);
  });

  it('refuses a player: no reconnecting', async () => {
    watcher.start('c1');
    await settleAll();
    last().fail(new ConnectError('no', Code.PermissionDenied));
    await settleAll();
    expect(watcher.state.phase().kind).toBe('no-access');
    await vi.advanceTimersByTimeAsync(60_000);
    expect(calls).toHaveLength(1);
    expect(gets).not.toHaveBeenCalled();
  });

  it('refuses on not found with the same card', async () => {
    watcher.start('c1');
    await settleAll();
    last().fail(new ConnectError('no', Code.NotFound));
    await settleAll();
    expect(watcher.state.phase().kind).toBe('no-access');
  });

  it('ends when there is no open session', async () => {
    watcher.start('c1');
    await settleAll();
    last().fail(noOpenSession());
    await settleAll();
    expect(watcher.state.phase().kind).toBe('ended');
    expect(calls).toHaveLength(1);
  });

  it('stops reading when the read says no access', async () => {
    gets.mockRejectedValueOnce(new ConnectError('no', Code.PermissionDenied));
    watcher.start('c1');
    await settleAll();
    last().push('ready');
    await settleAll();
    expect(watcher.state.phase().kind).toBe('no-access');
    expect(last().aborted).toBe(true);
  });

  it('opens nothing while the tab is hidden', async () => {
    setVisibility('hidden');
    watcher.start('c1');
    await settleAll();
    expect(calls).toHaveLength(0);
    expect(watcher.state.phase().kind).toBe('loading');
    setVisibility('visible');
    await settleAll();
    expect(calls).toHaveLength(1);
  });
});

describe('tvEvents', () => {
  it('maps the stream to the events of the live stream', async () => {
    async function* wire() {
      yield tvWire('ready');
      yield tvWire('heartbeat');
      yield tvWire('changed');
      yield tvWire('sessionEnded');
    }
    const kinds: string[] = [];
    for await (const e of tvEvents(wire())) {
      kinds.push(e.kind);
    }
    expect(kinds).toEqual(['ready', 'heartbeat', 'contentChanged', 'ended']);
  });
});
