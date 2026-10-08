import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';

import { LiveSessionSourceLive } from '../../pages/live-session/live-session-source.live';
import type { LiveEventVm } from '../../pages/live-session/live-session.types';
import { OpenSessions } from '../../shell/live-notice/open-sessions';
import { CONTENT_DEBOUNCE_MS, ContentWatcher } from './content-watcher';

/** One stream the test drives by hand. */
class Call {
  private queue: LiveEventVm[] = [];
  private wake: (() => void) | null = null;
  constructor(
    readonly campaignId: string,
    readonly signal: AbortSignal,
  ) {}
  push(event: LiveEventVm) {
    this.queue.push(event);
    this.wake?.();
  }
  async *events(): AsyncIterable<LiveEventVm> {
    for (;;) {
      while (this.queue.length === 0) {
        await new Promise<void>((resolve) => (this.wake = resolve));
      }
      yield this.queue.shift()!;
    }
  }
}

describe('ContentWatcher (RN-23, RN-10: the live content hint)', () => {
  let calls: Call[];
  const sessions = signal<readonly { campaignId: string }[]>([]);
  // A clock the spec drives by hand: the debounce is crossed by advancing it, never by sleeping.
  const tick = () => vi.advanceTimersByTimeAsync(0);
  const pause = (ms: number) => vi.advanceTimersByTimeAsync(ms);

  beforeEach(() => {
    vi.useFakeTimers();
    calls = [];
    sessions.set([]);
    const source = {
      watch: (campaignId: string, signal: AbortSignal) => {
        const call = new Call(campaignId, signal);
        calls.push(call);
        return call.events();
      },
      classifyError: () => 'transient',
    };
    TestBed.configureTestingModule({
      providers: [
        ContentWatcher,
        { provide: LiveSessionSourceLive, useValue: source },
        { provide: OpenSessions, useValue: { sessions } },
      ],
    });
  });

  it('reads once for a burst of hints, and carries no content', async () => {
    const onChange = vi.fn();
    const watcher = TestBed.inject(ContentWatcher);
    watcher.follow('camp-1', onChange);
    await tick();
    calls[0].push({ kind: 'ready' });
    calls[0].push({ kind: 'contentChanged' });
    calls[0].push({ kind: 'contentChanged' });
    calls[0].push({ kind: 'contentChanged' });
    await tick();
    expect(onChange).not.toHaveBeenCalled();
    await pause(CONTENT_DEBOUNCE_MS + 50);
    expect(onChange).toHaveBeenCalledTimes(1);
    expect(onChange).toHaveBeenCalledWith();
    watcher.follow(null, onChange);
  });

  it('ignores the other hints of the session', async () => {
    const onChange = vi.fn();
    const watcher = TestBed.inject(ContentWatcher);
    watcher.follow('camp-1', onChange);
    await tick();
    calls[0].push({ kind: 'ready' });
    calls[0].push({ kind: 'xpChanged' });
    calls[0].push({ kind: 'combatLogChanged' });
    await pause(CONTENT_DEBOUNCE_MS + 50);
    expect(onChange).not.toHaveBeenCalled();
    watcher.follow(null, onChange);
  });

  it('reads again on a later `ready`: a hint may have been missed while the stream was down', async () => {
    const onChange = vi.fn();
    const watcher = TestBed.inject(ContentWatcher);
    watcher.follow('camp-1', onChange);
    await tick();
    calls[0].push({ kind: 'ready' });
    calls[0].push({ kind: 'ready' });
    await pause(CONTENT_DEBOUNCE_MS + 50);
    expect(onChange).toHaveBeenCalledTimes(1);
    watcher.follow(null, onChange);
  });

  it('reads on the first `ready` of a session that opened while the page was up (it may have changed in between)', async () => {
    const onChange = vi.fn();
    const watcher = TestBed.inject(ContentWatcher);
    TestBed.runInInjectionContext(() => watcher.whileLive(() => 'camp-1', onChange));
    TestBed.tick();
    await tick();
    sessions.set([{ campaignId: 'camp-1' }]);
    TestBed.tick();
    await tick();
    calls[0].push({ kind: 'ready' });
    await pause(CONTENT_DEBOUNCE_MS + 50);
    expect(onChange).toHaveBeenCalledTimes(1);
  });

  it('does not read on the first `ready` when the session was already open as the page came (its own load did)', async () => {
    sessions.set([{ campaignId: 'camp-1' }]);
    const onChange = vi.fn();
    const watcher = TestBed.inject(ContentWatcher);
    TestBed.runInInjectionContext(() => watcher.whileLive(() => 'camp-1', onChange));
    TestBed.tick();
    await tick();
    calls[0].push({ kind: 'ready' });
    await pause(CONTENT_DEBOUNCE_MS + 50);
    expect(onChange).not.toHaveBeenCalled();
  });

  it('clears the waiting read when the page is destroyed', async () => {
    const onChange = vi.fn();
    const watcher = TestBed.inject(ContentWatcher);
    watcher.follow('camp-1', onChange);
    await tick();
    calls[0].push({ kind: 'ready' });
    calls[0].push({ kind: 'contentChanged' });
    await tick();
    TestBed.resetTestingModule();
    await pause(CONTENT_DEBOUNCE_MS + 50);
    expect(onChange).not.toHaveBeenCalled();
  });

  it('closes the stream, and drops a read that was waiting, when the page goes away', async () => {
    const onChange = vi.fn();
    const watcher = TestBed.inject(ContentWatcher);
    watcher.follow('camp-1', onChange);
    await tick();
    calls[0].push({ kind: 'ready' });
    calls[0].push({ kind: 'contentChanged' });
    await tick();
    watcher.follow(null, onChange);
    expect(calls[0].signal.aborted).toBe(true);
    await pause(CONTENT_DEBOUNCE_MS + 50);
    expect(onChange).not.toHaveBeenCalled();
  });

  it('follows the campaign only while it has an open session', async () => {
    const onChange = vi.fn();
    const watcher = TestBed.inject(ContentWatcher);
    TestBed.runInInjectionContext(() => watcher.whileLive(() => 'camp-1', onChange));
    TestBed.tick();
    await tick();
    // No open session: no stream, the page reads on load only.
    expect(calls).toHaveLength(0);
    sessions.set([{ campaignId: 'camp-1' }]);
    TestBed.tick();
    await tick();
    expect(calls.map((c) => c.campaignId)).toEqual(['camp-1']);
    // The session ends (the list of open sessions empties): the stream closes.
    sessions.set([]);
    TestBed.tick();
    await tick();
    expect(calls[0].signal.aborted).toBe(true);
  });
});
