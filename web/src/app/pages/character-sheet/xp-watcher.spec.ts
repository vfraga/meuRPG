import { TestBed } from '@angular/core/testing';

import { LiveSessionSourceLive } from '../live-session/live-session-source.live';
import type { LiveEventVm } from '../live-session/live-session.types';
import { XpWatcher } from './xp-watcher';

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

describe('XpWatcher (E7-10)', () => {
  let calls: Call[];
  const flush = () => new Promise((resolve) => setTimeout(resolve, 0));

  beforeEach(() => {
    calls = [];
    const source = {
      watch: (campaignId: string, signal: AbortSignal) => {
        const call = new Call(campaignId, signal);
        calls.push(call);
        return call.events();
      },
      classifyError: () => 'transient',
    };
    TestBed.configureTestingModule({
      providers: [XpWatcher, { provide: LiveSessionSourceLive, useValue: source }],
    });
  });

  it("opens the session's stream and says so when the XP changes", async () => {
    const onChange = vi.fn();
    const watcher = TestBed.inject(XpWatcher);
    watcher.follow('camp-1', onChange);
    await flush();
    expect(calls.map((c) => c.campaignId)).toEqual(['camp-1']);

    calls[0].push({ kind: 'ready' });
    calls[0].push({ kind: 'xpChanged' });
    await flush();
    // The first `ready` is the page's own load; only the news counts.
    expect(onChange).toHaveBeenCalledTimes(1);
    watcher.follow(null, onChange);
  });

  it('reads again on a later `ready` (a reconnection may have missed an event)', async () => {
    const onChange = vi.fn();
    const watcher = TestBed.inject(XpWatcher);
    watcher.follow('camp-1', onChange);
    await flush();
    calls[0].push({ kind: 'ready' });
    calls[0].push({ kind: 'ready' });
    await flush();
    expect(onChange).toHaveBeenCalledTimes(1);
    watcher.follow(null, onChange);
  });

  it('says so when the creatures change (MR-037), through the real stream, and not for the XP', async () => {
    const onChange = vi.fn();
    const onCreatures = vi.fn();
    const watcher = TestBed.inject(XpWatcher);
    watcher.follow('camp-1', onChange, onCreatures);
    await flush();
    calls[0].push({ kind: 'ready' });
    calls[0].push({ kind: 'creaturesChanged' });
    await flush();
    expect(onCreatures).toHaveBeenCalledTimes(1);
    expect(onChange).not.toHaveBeenCalled();
    calls[0].push({ kind: 'xpChanged' });
    await flush();
    expect(onChange).toHaveBeenCalledTimes(1);
    expect(onCreatures).toHaveBeenCalledTimes(1);
    watcher.follow(null, onChange);
  });

  it('reads the creatures again on a later `ready` too: a gift made while the tab was hidden shows up', async () => {
    const onCreatures = vi.fn();
    const watcher = TestBed.inject(XpWatcher);
    watcher.follow('camp-1', vi.fn(), onCreatures);
    await flush();
    calls[0].push({ kind: 'ready' });
    await flush();
    // The page reads on load itself: the first `ready` asks for nothing.
    expect(onCreatures).not.toHaveBeenCalled();
    calls[0].push({ kind: 'ready' });
    await flush();
    expect(onCreatures).toHaveBeenCalledTimes(1);
    watcher.follow(null, vi.fn());
  });

  it("says so when a character's vitals or the combat change (the form of a Wild Shape ends that way), with the character, and on a later `ready`", async () => {
    const onForm = vi.fn();
    const watcher = TestBed.inject(XpWatcher);
    watcher.follow('camp-1', vi.fn(), undefined, undefined, onForm);
    await flush();
    calls[0].push({ kind: 'ready' });
    calls[0].push({ kind: 'xpChanged' });
    await flush();
    expect(onForm).not.toHaveBeenCalled();
    calls[0].push({
      kind: 'vitals',
      vitals: { characterId: 'char-1' } as never,
    });
    calls[0].push({ kind: 'encounterChanged', encounterId: 'enc-1', revision: 3 });
    await flush();
    expect(onForm.mock.calls).toEqual([['char-1'], [null]]);
    calls[0].push({ kind: 'ready' });
    await flush();
    expect(onForm).toHaveBeenLastCalledWith(null);
    watcher.follow(null, vi.fn());
  });

  it('reads the sheet again when the table\'s content changes (RN-23, "A classe mudou"), on the same stream, and on a reconnection', async () => {
    const onContent = vi.fn();
    const onChange = vi.fn();
    const watcher = TestBed.inject(XpWatcher);
    watcher.follow('camp-1', onChange, undefined, onContent);
    await flush();
    calls[0].push({ kind: 'ready' });
    calls[0].push({ kind: 'contentChanged' });
    await flush();
    expect(onContent).toHaveBeenCalledTimes(1);
    expect(onChange).not.toHaveBeenCalled();
    calls[0].push({ kind: 'ready' });
    await flush();
    expect(onContent).toHaveBeenCalledTimes(2);
    expect(calls).toHaveLength(1);
    watcher.follow(null, onChange);
  });

  it('does not open a second stream for the same campaign, and closes the first for another', async () => {
    const watcher = TestBed.inject(XpWatcher);
    watcher.follow('camp-1', vi.fn());
    watcher.follow('camp-1', vi.fn());
    await flush();
    expect(calls).toHaveLength(1);

    watcher.follow('camp-2', vi.fn());
    await flush();
    expect(calls[0].signal.aborted).toBe(true);
    expect(calls.map((c) => c.campaignId)).toEqual(['camp-1', 'camp-2']);
    watcher.follow(null, vi.fn());
  });

  it('closes the stream when asked to follow nothing (the page went away)', async () => {
    const watcher = TestBed.inject(XpWatcher);
    watcher.follow('camp-1', vi.fn());
    await flush();
    watcher.follow(null, vi.fn());
    expect(calls[0].signal.aborted).toBe(true);
  });

  it('stops when the session ends', async () => {
    const watcher = TestBed.inject(XpWatcher);
    watcher.follow('camp-1', vi.fn());
    await flush();
    calls[0].push({ kind: 'ended' });
    await flush();
    expect(calls[0].signal.aborted).toBe(true);
    // A new session of the same campaign is followed again.
    watcher.follow('camp-1', vi.fn());
    await flush();
    expect(calls).toHaveLength(2);
    watcher.follow(null, vi.fn());
  });
});
