import {
  BACKOFF_MAX_MS,
  DEAD_STREAM_MS,
  HIDDEN_CLOSE_MS,
  LiveStream,
  LiveStreamHandlers,
  backoffDelay,
} from './live-stream';
import { LiveErrorKind, LiveEventVm, VitalsVm } from './live-session.types';

/** One `WatchGameSession` call the test drives by hand. */
class FakeCall {
  private readonly queue: (IteratorResult<LiveEventVm> | { error: unknown })[] = [];
  private wake: (() => void) | null = null;
  aborted = false;

  constructor(readonly signal: AbortSignal) {
    signal.addEventListener('abort', () => {
      this.aborted = true;
      this.fail(new Error('aborted'));
    });
  }

  push(event: LiveEventVm): void {
    this.queue.push({ value: event, done: false });
    this.wake?.();
  }

  end(): void {
    this.queue.push({ value: undefined, done: true });
    this.wake?.();
  }

  fail(error: unknown): void {
    this.queue.push({ error });
    this.wake?.();
  }

  async *events(): AsyncIterable<LiveEventVm> {
    for (;;) {
      while (this.queue.length === 0) {
        await new Promise<void>((resolve) => (this.wake = resolve));
      }
      const next = this.queue.shift()!;
      if ('error' in next) {
        throw next.error;
      }
      if (next.done) {
        return;
      }
      yield next.value;
    }
  }
}

class FailWith extends Error {
  constructor(readonly kind: LiveErrorKind) {
    super(kind);
  }
}

function vitals(revision: number): VitalsVm {
  return {
    characterId: 'pensantus',
    name: 'Pensantus',
    playerUserId: 'u1',
    hitPointsCurrent: 17,
    hitPointsMax: 23,
    hitPointsTemporary: 0,
    spellSlots: [],
    pactSlots: null,
    hitDice: '3d6',
    hitDiceSizes: [{ faces: 6, total: 3, used: 1 }],
    hitDiceTotal: 3,
    hitDiceUsed: 1,
    revision,
  };
}

function setVisibility(state: 'visible' | 'hidden'): void {
  Object.defineProperty(document, 'visibilityState', { value: state, configurable: true });
  document.dispatchEvent(new Event('visibilitychange'));
}

describe('LiveStream (ADR-0005 client rules)', () => {
  let calls: FakeCall[];
  let handlers: { [K in keyof LiveStreamHandlers]: ReturnType<typeof vi.fn> };
  let stream: LiveStream;

  const last = () => calls[calls.length - 1];
  const flush = () => vi.advanceTimersByTimeAsync(0);
  /** Lets `ms` go by with the server's heartbeat every 20 s (it sends one
   * every 25 s), so the dead-stream check stays quiet. */
  async function withHeartbeats(ms: number): Promise<void> {
    for (let left = ms; left > 0; left -= 20_000) {
      await vi.advanceTimersByTimeAsync(Math.min(20_000, left));
      if (!last().aborted) {
        last().push({ kind: 'heartbeat' });
        await flush();
      }
    }
  }

  beforeEach(() => {
    vi.useFakeTimers();
    setVisibility('visible');
    calls = [];
    handlers = {
      onReady: vi.fn(),
      onVitals: vi.fn(),
      onEncounterChanged: vi.fn(),
      onTurnChanged: vi.fn(),
      onCombatantMoved: vi.fn(),
      onCombatLogChanged: vi.fn(),
      onXpChanged: vi.fn(),
      onPuzzleChanged: vi.fn(),
      onReactionWindowOpened: vi.fn(),
      onReactionWindowClosed: vi.fn(),
      onContentChanged: vi.fn(),
      onCharacterChanged: vi.fn(),
      onCharacterRevived: vi.fn(),
      onRevivifyChanged: vi.fn(),
      onCallingYou: vi.fn(),
      onSceneChanged: vi.fn(),
      onNotesChanged: vi.fn(),
      onVisionChanged: vi.fn(),
      onStageChanged: vi.fn(),
      onSpellCastsChanged: vi.fn(),
      onGroupCheckChanged: vi.fn(),
      onEnded: vi.fn(),
      onFatal: vi.fn(),
    };
    stream = new LiveStream({
      open: (signal) => {
        const call = new FakeCall(signal);
        calls.push(call);
        return call.events();
      },
      classify: (err) => (err instanceof FailWith ? err.kind : 'transient'),
      handlers: handlers as unknown as LiveStreamHandlers,
      document,
      random: () => 0.5, // no jitter: the delays are exact
    });
  });

  afterEach(() => {
    stream.stop();
    vi.useRealTimers();
    setVisibility('visible');
  });

  it('reads the snapshot on `ready` and goes live', async () => {
    stream.start();
    expect(stream.status()).toBe('connecting');
    last().push({ kind: 'ready' });
    await flush();
    expect(handlers.onReady).toHaveBeenCalledTimes(1);
    expect(stream.status()).toBe('live');
    expect(stream.lastMessageAt()).not.toBeNull();
  });

  it('hands each vitals change to the page', async () => {
    stream.start();
    last().push({ kind: 'ready' });
    last().push({ kind: 'vitals', vitals: vitals(3) });
    await flush();
    expect(handlers.onVitals).toHaveBeenCalledWith(vitals(3));
  });

  it("hands a puzzle's ID to the page on `puzzle_changed` (MR-038)", async () => {
    stream.start();
    last().push({ kind: 'ready' });
    last().push({ kind: 'puzzleChanged', puzzleId: 'p-1' });
    await flush();
    expect(handlers.onPuzzleChanged).toHaveBeenCalledWith('p-1');
  });

  it('hands the page the id of a reaction window that opened, and the reason of one that closed by itself (PM-04)', async () => {
    stream.start();
    last().push({ kind: 'ready' });
    last().push({ kind: 'reactionWindowOpened', encounterId: 'e1', windowId: 'w1' });
    last().push({
      kind: 'reactionWindowClosed',
      encounterId: 'e1',
      windowId: 'w1',
      closedByItself: true,
      text: 'Queda Suave fechou. Você já usou a sua reação.',
    });
    await flush();
    expect(handlers.onReactionWindowOpened).toHaveBeenCalledWith(
      expect.objectContaining({ encounterId: 'e1', windowId: 'w1' }),
    );
    expect(handlers.onReactionWindowClosed).toHaveBeenCalledWith(
      expect.objectContaining({
        windowId: 'w1',
        closedByItself: true,
        text: 'Queda Suave fechou. Você já usou a sua reação.',
      }),
    );
  });

  it("tells the page the table's content changed (RN-23, content_changed), with nothing in it", async () => {
    stream.start();
    last().push({ kind: 'ready' });
    last().push({ kind: 'contentChanged' });
    await flush();
    expect(handlers.onContentChanged).toHaveBeenCalledTimes(1);
    expect(handlers.onContentChanged).toHaveBeenCalledWith();
    expect(stream.status()).toBe('live');
  });

  it('tells the page a casting of Revivify outside a combat changed, with nothing in it', async () => {
    stream.start();
    last().push({ kind: 'ready' });
    last().push({ kind: 'revivifyChanged' });
    await flush();
    expect(handlers.onRevivifyChanged).toHaveBeenCalledTimes(1);
    expect(handlers.onRevivifyChanged).toHaveBeenCalledWith();
    expect(stream.status()).toBe('live');
  });

  it('tells the page the master is calling, with nothing in it', async () => {
    stream.start();
    last().push({ kind: 'ready' });
    last().push({ kind: 'callingYou' });
    await flush();
    expect(handlers.onCallingYou).toHaveBeenCalledTimes(1);
    expect(handlers.onCallingYou).toHaveBeenCalledWith();
    expect(stream.status()).toBe('live');
  });

  it('tells the page a character lives again as a change of that character and as a revival', async () => {
    stream.start();
    last().push({ kind: 'ready' });
    last().push({ kind: 'characterChanged', characterId: 'c-1' });
    last().push({ kind: 'characterRevived', characterId: 'c-2' });
    await flush();
    expect(handlers.onCharacterChanged).toHaveBeenCalledTimes(2);
    expect(handlers.onCharacterChanged).toHaveBeenLastCalledWith('c-2');
    expect(handlers.onCharacterRevived).toHaveBeenCalledTimes(1);
    expect(handlers.onCharacterRevived).toHaveBeenCalledWith('c-2');
  });

  it('tells the page the XP changed (MR-016), and keeps the stream alive', async () => {
    stream.start();
    last().push({ kind: 'ready' });
    last().push({ kind: 'xpChanged' });
    last().push({ kind: 'xpChanged' });
    await flush();
    expect(handlers.onXpChanged).toHaveBeenCalledTimes(2);
    expect(stream.status()).toBe('live');
  });

  it('hands both scene hints to the page as one: read the open scene again (MR-015)', async () => {
    stream.start();
    last().push({ kind: 'ready' });
    last().push({ kind: 'sceneChanged' });
    last().push({ kind: 'sceneCheckRolled' });
    await flush();
    expect(handlers.onSceneChanged).toHaveBeenCalledTimes(2);
  });

  it("tells the page a clue arrived in the player's notes (MR-030), and keeps the stream alive", async () => {
    stream.start();
    last().push({ kind: 'ready' });
    last().push({ kind: 'notesChanged' });
    await flush();
    expect(handlers.onNotesChanged).toHaveBeenCalledTimes(1);
    expect(stream.status()).toBe('live');
  });

  it('tells the page what a player sees of a fog map changed (MR-036), naming the map and nothing else', async () => {
    stream.start();
    last().push({ kind: 'ready' });
    last().push({ kind: 'visionChanged', mapId: 'map-1' });
    await flush();
    expect(handlers.onVisionChanged).toHaveBeenCalledWith('map-1');
    expect(stream.status()).toBe('live');
  });

  it('hands `spell_casts_changed` to the page on its own, so the casts are read again (MR-048)', async () => {
    stream.start();
    last().push({ kind: 'ready' });
    last().push({ kind: 'spellCastsChanged' });
    await flush();
    expect(handlers.onSpellCastsChanged).toHaveBeenCalledTimes(1);
    expect(handlers.onStageChanged).not.toHaveBeenCalled();
  });

  it('hands `group_check_changed` to the page on its own, so the group check is read again (W7-X)', async () => {
    stream.start();
    last().push({ kind: 'ready' });
    last().push({ kind: 'groupCheckChanged' });
    await flush();
    expect(handlers.onGroupCheckChanged).toHaveBeenCalledTimes(1);
    expect(handlers.onSpellCastsChanged).not.toHaveBeenCalled();
    expect(stream.status()).toBe('live');
  });

  it('hands `stage_changed` to the page on its own, so the stage is read again (MR-031)', async () => {
    stream.start();
    last().push({ kind: 'ready' });
    last().push({ kind: 'stageChanged' });
    await flush();
    expect(handlers.onStageChanged).toHaveBeenCalledTimes(1);
    expect(handlers.onSceneChanged).not.toHaveBeenCalled();
  });

  it("hands the combat's events to the page (MR-013)", async () => {
    stream.start();
    last().push({ kind: 'ready' });
    last().push({ kind: 'encounterChanged', encounterId: 'e1', revision: 4 });
    last().push({
      kind: 'turnChanged',
      encounterId: 'e1',
      round: 2,
      currentCombatantId: '',
      masterTurn: true,
    });
    last().push({ kind: 'combatantMoved', encounterId: 'e1', combatantId: 'c1', col: 3, row: 5 });
    last().push({ kind: 'combatLogChanged' });
    await flush();
    expect(handlers.onCombatLogChanged).toHaveBeenCalledTimes(1);
    expect(handlers.onEncounterChanged).toHaveBeenCalledWith(
      expect.objectContaining({ encounterId: 'e1', revision: 4 }),
    );
    expect(handlers.onTurnChanged).toHaveBeenCalledWith(
      expect.objectContaining({ round: 2, masterTurn: true }),
    );
    expect(handlers.onCombatantMoved).toHaveBeenCalledWith(
      expect.objectContaining({ combatantId: 'c1', col: 3, row: 5 }),
    );
  });

  it('`session_ended` ends it for good, without reconnecting', async () => {
    stream.start();
    last().push({ kind: 'ready' });
    last().push({ kind: 'ended' });
    await flush();
    expect(handlers.onEnded).toHaveBeenCalledTimes(1);
    expect(stream.status()).toBe('closed');
    await vi.advanceTimersByTimeAsync(BACKOFF_MAX_MS * 2);
    expect(calls.length).toBe(1);
  });

  it('reconnects after a clean end with backoff, and reads a new snapshot', async () => {
    stream.start();
    last().push({ kind: 'ready' });
    await flush();
    last().end(); // the 30-minute cap
    await flush();
    expect(stream.status()).toBe('reconnecting');

    await vi.advanceTimersByTimeAsync(999);
    expect(calls.length).toBe(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(calls.length).toBe(2);

    last().push({ kind: 'ready' });
    await flush();
    expect(handlers.onReady).toHaveBeenCalledTimes(2);
    expect(stream.status()).toBe('live');
  });

  it('doubles the wait on each failure, up to 30 seconds', async () => {
    stream.start();
    const waits: number[] = [];
    for (let i = 0; i < 7; i++) {
      last().fail(new Error('network'));
      await flush();
      const before = calls.length;
      let waited = 0;
      while (calls.length === before) {
        await vi.advanceTimersByTimeAsync(1000);
        waited += 1000;
      }
      waits.push(waited);
    }
    expect(waits).toEqual([1000, 2000, 4000, 8000, 16000, 30000, 30000]);
  });

  it('starts the backoff over once a connection reaches `ready`', async () => {
    stream.start();
    last().fail(new Error('network'));
    await vi.advanceTimersByTimeAsync(1000);
    last().fail(new Error('network'));
    await vi.advanceTimersByTimeAsync(2000);
    last().push({ kind: 'ready' });
    await flush();
    last().fail(new Error('network'));
    await vi.advanceTimersByTimeAsync(1000);
    expect(calls.length).toBe(4);
  });

  it('waits longer on each restart in a row (a failing snapshot), until one works', async () => {
    stream.start();
    const readyAndRestart = async () => {
      last().push({ kind: 'ready' });
      await flush();
      stream.restart();
      await flush();
    };
    await readyAndRestart();
    await vi.advanceTimersByTimeAsync(1999);
    expect(calls.length).toBe(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(calls.length).toBe(2);

    await readyAndRestart();
    await vi.advanceTimersByTimeAsync(3999);
    expect(calls.length).toBe(2);
    await vi.advanceTimersByTimeAsync(1);
    expect(calls.length).toBe(3);

    // The snapshot worked: the next restart is quick again.
    last().push({ kind: 'ready' });
    await flush();
    stream.confirmHealthy();
    stream.restart();
    await vi.advanceTimersByTimeAsync(2000);
    expect(calls.length).toBe(4);
  });

  it('`not_found` and `unauthenticated` end it without reconnecting', async () => {
    stream.start();
    last().fail(new FailWith('no-access'));
    await flush();
    expect(handlers.onFatal).toHaveBeenCalledWith('no-access');
    expect(stream.status()).toBe('closed');

    const other = new LiveStream({
      open: (signal) => {
        const call = new FakeCall(signal);
        calls.push(call);
        return call.events();
      },
      classify: (err) => (err instanceof FailWith ? err.kind : 'transient'),
      handlers: handlers as unknown as LiveStreamHandlers,
      document,
    });
    other.start();
    last().fail(new FailWith('signed-out'));
    await flush();
    expect(handlers.onFatal).toHaveBeenCalledWith('signed-out');
    await vi.advanceTimersByTimeAsync(BACKOFF_MAX_MS * 2);
    expect(calls.length).toBe(2);
  });

  it('`NO_OPEN_SESSION` means the session ended', async () => {
    stream.start();
    last().fail(new FailWith('no-session'));
    await flush();
    expect(handlers.onEnded).toHaveBeenCalledTimes(1);
    expect(stream.status()).toBe('closed');
  });

  it('treats 60 seconds without any message as a dead stream', async () => {
    stream.start();
    last().push({ kind: 'ready' });
    await flush();
    // Heartbeats keep it alive…
    await vi.advanceTimersByTimeAsync(25_000);
    last().push({ kind: 'heartbeat' });
    await vi.advanceTimersByTimeAsync(DEAD_STREAM_MS - 1);
    expect(last().aborted).toBe(false);
    // …silence doesn't.
    await vi.advanceTimersByTimeAsync(1);
    expect(calls[0].aborted).toBe(true);
    expect(stream.status()).toBe('reconnecting');
    await vi.advanceTimersByTimeAsync(1000);
    expect(calls.length).toBe(2);
  });

  it('closes after 2 minutes hidden, and reopens at once when visible again', async () => {
    stream.start();
    last().push({ kind: 'ready' });
    await flush();

    setVisibility('hidden');
    await withHeartbeats(HIDDEN_CLOSE_MS - 1);
    expect(calls[0].aborted).toBe(false);
    await vi.advanceTimersByTimeAsync(1);
    expect(calls[0].aborted).toBe(true);
    expect(stream.status()).toBe('paused');

    // Nothing reconnects while hidden, however long.
    await vi.advanceTimersByTimeAsync(10 * 60_000);
    expect(calls.length).toBe(1);

    setVisibility('visible');
    await flush();
    expect(calls.length).toBe(2);
    last().push({ kind: 'ready' });
    await flush();
    expect(handlers.onReady).toHaveBeenCalledTimes(2);
  });

  it('keeps the stream through a short look away', async () => {
    stream.start();
    last().push({ kind: 'ready' });
    await flush();
    setVisibility('hidden');
    await withHeartbeats(30_000);
    setVisibility('visible');
    await withHeartbeats(HIDDEN_CLOSE_MS);
    expect(calls.length).toBe(1);
    expect(calls[0].aborted).toBe(false);
  });

  it("doesn't reconnect while hidden after a drop; waits to be seen", async () => {
    stream.start();
    last().push({ kind: 'ready' });
    await flush();
    setVisibility('hidden');
    last().fail(new Error('network'));
    await vi.advanceTimersByTimeAsync(BACKOFF_MAX_MS * 3);
    expect(calls.length).toBe(1);
    expect(stream.status()).toBe('paused');
    setVisibility('visible');
    await flush();
    expect(calls.length).toBe(2);
  });

  it('stop() closes the stream and every timer', async () => {
    stream.start();
    last().push({ kind: 'ready' });
    await flush();
    stream.stop();
    expect(calls[0].aborted).toBe(true);
    await vi.advanceTimersByTimeAsync(DEAD_STREAM_MS * 3);
    expect(calls.length).toBe(1);
  });

  describe('a lost hint (the heartbeat is ahead of the stream)', () => {
    let behind: ReturnType<typeof vi.fn>;

    beforeEach(() => {
      behind = vi.fn(() => Promise.resolve(true));
      handlers.onBehind = behind;
    });

    async function connect(seq: number): Promise<void> {
      stream.start();
      last().push({ kind: 'ready', sessionSeq: seq });
      await flush();
    }

    it('reads again once, within that heartbeat, after a dropped event', async () => {
      await connect(7);
      // The hint with number 8 never arrived; the next one (a vitals) did, with a number the stream ignores.
      last().push({ kind: 'vitals', vitals: vitals(2) });
      last().push({ kind: 'heartbeat', sessionSeq: 9 });
      await flush();
      expect(behind).toHaveBeenCalledTimes(1);
      last().push({ kind: 'heartbeat', sessionSeq: 9 });
      await flush();
      expect(behind).toHaveBeenCalledTimes(1);
    });

    it('reads nothing on a heartbeat equal to or behind the number from `ready`', async () => {
      await connect(7);
      last().push({ kind: 'heartbeat', sessionSeq: 7 });
      last().push({ kind: 'heartbeat', sessionSeq: 3 });
      await flush();
      expect(behind).not.toHaveBeenCalled();
    });

    it('keeps the number when the read fails, and asks again at the next heartbeat', async () => {
      await connect(7);
      behind.mockResolvedValueOnce(false);
      last().push({ kind: 'heartbeat', sessionSeq: 9 });
      await flush();
      expect(behind).toHaveBeenCalledTimes(1);
      last().push({ kind: 'heartbeat', sessionSeq: 9 });
      await flush();
      expect(behind).toHaveBeenCalledTimes(2);
      last().push({ kind: 'heartbeat', sessionSeq: 9 });
      await flush();
      expect(behind).toHaveBeenCalledTimes(2);
    });

    it('treats a read that throws like a failed one', async () => {
      await connect(7);
      behind.mockRejectedValueOnce(new Error('offline'));
      last().push({ kind: 'heartbeat', sessionSeq: 8 });
      await flush();
      last().push({ kind: 'heartbeat', sessionSeq: 8 });
      await flush();
      expect(behind).toHaveBeenCalledTimes(2);
    });

    it('reads nothing on a heartbeat with no number (an event this app does not know)', async () => {
      await connect(7);
      last().push({ kind: 'heartbeat' });
      await flush();
      expect(behind).not.toHaveBeenCalled();
    });

    it('does not start a second read while one is out', async () => {
      await connect(7);
      let finish = (_ok: boolean): void => undefined;
      behind.mockReturnValueOnce(new Promise<boolean>((resolve) => (finish = resolve)));
      last().push({ kind: 'heartbeat', sessionSeq: 8 });
      last().push({ kind: 'heartbeat', sessionSeq: 9 });
      await flush();
      expect(behind).toHaveBeenCalledTimes(1);
      finish(true);
      await flush();
      last().push({ kind: 'heartbeat', sessionSeq: 9 });
      await flush();
      // 9 was not covered by the read that began at 8's heartbeat: it is read once more.
      expect(behind).toHaveBeenCalledTimes(2);
    });

    it('ignores a read of an old connection that ends after the new one is ready', async () => {
      await connect(7);
      let finish = (_ok: boolean): void => undefined;
      behind.mockReturnValueOnce(new Promise<boolean>((resolve) => (finish = resolve)));
      last().push({ kind: 'heartbeat', sessionSeq: 15 });
      await flush();
      last().end();
      await vi.advanceTimersByTimeAsync(backoffDelay(0, () => 0.5));
      last().push({ kind: 'ready', sessionSeq: 12 });
      await flush();
      finish(true);
      await flush();
      last().push({ kind: 'heartbeat', sessionSeq: 12 });
      await flush();
      expect(behind).toHaveBeenCalledTimes(1);
      // The old read did not move the new connection's number to 15.
      last().push({ kind: 'heartbeat', sessionSeq: 14 });
      await flush();
      expect(behind).toHaveBeenCalledTimes(2);
    });

    it('can re-read on the new connection while a read of the old one is still out', async () => {
      await connect(7);
      behind.mockReturnValueOnce(new Promise<boolean>(() => undefined));
      last().push({ kind: 'heartbeat', sessionSeq: 9 });
      await flush();
      last().end();
      await vi.advanceTimersByTimeAsync(backoffDelay(0, () => 0.5));
      last().push({ kind: 'ready', sessionSeq: 12 });
      await flush();
      last().push({ kind: 'heartbeat', sessionSeq: 14 });
      await flush();
      expect(behind).toHaveBeenCalledTimes(2);
    });

    it('does not let the end of an old read free a read of the new connection that is still out', async () => {
      await connect(7);
      let finishOld = (_ok: boolean): void => undefined;
      behind.mockReturnValueOnce(new Promise<boolean>((resolve) => (finishOld = resolve)));
      last().push({ kind: 'heartbeat', sessionSeq: 9 });
      await flush();
      last().end();
      await vi.advanceTimersByTimeAsync(backoffDelay(0, () => 0.5));
      last().push({ kind: 'ready', sessionSeq: 12 });
      await flush();
      behind.mockReturnValueOnce(new Promise<boolean>(() => undefined));
      last().push({ kind: 'heartbeat', sessionSeq: 14 });
      await flush();
      expect(behind).toHaveBeenCalledTimes(2);
      finishOld(true);
      await flush();
      last().push({ kind: 'heartbeat', sessionSeq: 15 });
      await flush();
      // The new connection's own read is still out: the old one finishing does not start another.
      expect(behind).toHaveBeenCalledTimes(2);
    });

    it('takes the number again from each `ready`', async () => {
      await connect(7);
      last().end();
      await vi.advanceTimersByTimeAsync(backoffDelay(0, () => 0.5));
      last().push({ kind: 'ready', sessionSeq: 12 });
      await flush();
      last().push({ kind: 'heartbeat', sessionSeq: 12 });
      await flush();
      expect(behind).not.toHaveBeenCalled();
      last().push({ kind: 'heartbeat', sessionSeq: 13 });
      await flush();
      expect(behind).toHaveBeenCalledTimes(1);
    });
  });
});

describe('backoffDelay', () => {
  it('doubles from 1 s to a 30 s cap', () => {
    const mid = () => 0.5;
    expect([0, 1, 2, 3, 4, 5, 6, 10].map((a) => backoffDelay(a, mid))).toEqual([
      1000, 2000, 4000, 8000, 16000, 30000, 30000, 30000,
    ]);
  });

  it('adds up to 20 % of jitter either way, never above 30 s', () => {
    expect(backoffDelay(2, () => 0)).toBe(3200);
    expect(backoffDelay(2, () => 1)).toBe(4800);
    expect(backoffDelay(10, () => 1)).toBe(BACKOFF_MAX_MS);
  });
});
