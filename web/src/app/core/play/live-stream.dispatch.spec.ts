// What `LiveStream` hands each handler for each kind of message, plus the corners of its lifecycle that
// `live-stream.spec.ts` leaves out: a tab opened hidden, a closed stream under a timer, a message of an unknown kind and
// a replaced connection that keeps sending. It uses the same fake call and fake clock as `live-stream.spec.ts`.
import { LiveStream, LiveStreamHandlers } from './live-stream';
import { LiveEventVm } from './live-session.types';

class FakeCall {
  private readonly queue: (IteratorResult<LiveEventVm> | { error: unknown })[] = [];
  private wake: (() => void) | null = null;
  aborted = false;

  constructor(readonly signal: AbortSignal) {
    signal.addEventListener('abort', () => {
      this.aborted = true;
      this.queue.push({ error: new Error('aborted') });
      this.wake?.();
    });
  }

  push(event: LiveEventVm): void {
    this.queue.push({ value: event, done: false });
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

function setVisibility(state: 'visible' | 'hidden'): void {
  Object.defineProperty(document, 'visibilityState', { value: state, configurable: true });
  document.dispatchEvent(new Event('visibilitychange'));
}

type Mocks = { [K in keyof LiveStreamHandlers]: ReturnType<typeof vi.fn> };

describe('LiveStream: what each message hands the page', () => {
  let calls: FakeCall[];
  let handlers: Mocks;
  let stream: LiveStream;
  const last = () => calls[calls.length - 1];
  const flush = () => vi.advanceTimersByTimeAsync(0);

  function build(over: Partial<Mocks> = {}): void {
    handlers = {
      onReady: vi.fn(),
      onVitals: vi.fn(),
      onCurrentMap: vi.fn(),
      onMapChanged: vi.fn(),
      onTokenMoved: vi.fn(),
      onVisionChanged: vi.fn(),
      onShownImage: vi.fn(),
      onLeftImages: vi.fn(),
      onEncounterChanged: vi.fn(),
      onReactionWindowOpened: vi.fn(),
      onReactionWindowClosed: vi.fn(),
      onTurnChanged: vi.fn(),
      onCombatantMoved: vi.fn(),
      onCombatLogChanged: vi.fn(),
      onXpChanged: vi.fn(),
      onSceneChanged: vi.fn(),
      onNotesChanged: vi.fn(),
      onStageChanged: vi.fn(),
      onTrapNoticed: vi.fn(),
      onCreaturesChanged: vi.fn(),
      onSpellCastsChanged: vi.fn(),
      onGroupCheckChanged: vi.fn(),
      onContentChanged: vi.fn(),
      onCharacterChanged: vi.fn(),
      onCharacterRevived: vi.fn(),
      onRevivifyChanged: vi.fn(),
      onPuzzleChanged: vi.fn(),
      onCallingYou: vi.fn(),
      onEnded: vi.fn(),
      onFatal: vi.fn(),
      ...over,
    };
    stream = new LiveStream({
      open: (signal) => {
        const call = new FakeCall(signal);
        calls.push(call);
        return call.events();
      },
      classify: () => 'transient',
      handlers: handlers as unknown as LiveStreamHandlers,
      document,
      random: () => 0.5,
    });
  }

  beforeEach(() => {
    vi.useFakeTimers();
    setVisibility('visible');
    calls = [];
    build();
  });

  afterEach(() => {
    stream.stop();
    vi.useRealTimers();
    setVisibility('visible');
  });

  const image = { id: 'img', url: 'x.png', caption: '' } as never;
  const turn = { encounterId: 'e', round: 1, currentCombatantId: 'c', masterTurn: false };
  const move = { encounterId: 'e', combatantId: 'c', col: 1, row: 2 };

  const table: [string, LiveEventVm, keyof Mocks, unknown[]][] = [
    ['currentMap', { kind: 'currentMap', mapId: 'm1' }, 'onCurrentMap', ['m1']],
    ['currentMap cleared', { kind: 'currentMap', mapId: null }, 'onCurrentMap', [null]],
    ['mapChanged', { kind: 'mapChanged', mapId: 'm1' }, 'onMapChanged', ['m1']],
    [
      'tokenMoved',
      { kind: 'tokenMoved', mapId: 'm1', characterId: 'c1', xBp: 10, yBp: 20 },
      'onTokenMoved',
      [{ kind: 'tokenMoved', mapId: 'm1', characterId: 'c1', xBp: 10, yBp: 20 }],
    ],
    ['visionChanged', { kind: 'visionChanged', mapId: 'm1' }, 'onVisionChanged', ['m1']],
    ['leftImages', { kind: 'leftImages' }, 'onLeftImages', []],
    ['shownImage', { kind: 'shownImage', image }, 'onShownImage', [image]],
    ['shownImage stopped', { kind: 'shownImage', image: null }, 'onShownImage', [null]],
    [
      'encounterChanged',
      { kind: 'encounterChanged', encounterId: 'e', revision: 3, mode: 1 },
      'onEncounterChanged',
      [{ kind: 'encounterChanged', encounterId: 'e', revision: 3, mode: 1 }],
    ],
    [
      'reactionWindowOpened',
      { kind: 'reactionWindowOpened', encounterId: 'e', windowId: 'w' },
      'onReactionWindowOpened',
      [{ kind: 'reactionWindowOpened', encounterId: 'e', windowId: 'w' }],
    ],
    [
      'reactionWindowClosed',
      {
        kind: 'reactionWindowClosed',
        encounterId: 'e',
        windowId: 'w',
        closedByItself: true,
        text: 'acabou',
      },
      'onReactionWindowClosed',
      [
        {
          kind: 'reactionWindowClosed',
          encounterId: 'e',
          windowId: 'w',
          closedByItself: true,
          text: 'acabou',
        },
      ],
    ],
    [
      'turnChanged',
      { kind: 'turnChanged', ...turn },
      'onTurnChanged',
      [{ kind: 'turnChanged', ...turn }],
    ],
    [
      'combatantMoved',
      { kind: 'combatantMoved', ...move },
      'onCombatantMoved',
      [{ kind: 'combatantMoved', ...move }],
    ],
    ['combatLogChanged', { kind: 'combatLogChanged' }, 'onCombatLogChanged', []],
    ['xpChanged', { kind: 'xpChanged' }, 'onXpChanged', []],
    ['sceneChanged', { kind: 'sceneChanged' }, 'onSceneChanged', []],
    ['sceneCheckRolled', { kind: 'sceneCheckRolled' }, 'onSceneChanged', []],
    ['notesChanged', { kind: 'notesChanged' }, 'onNotesChanged', []],
    ['stageChanged', { kind: 'stageChanged' }, 'onStageChanged', []],
    [
      'trapNoticed',
      { kind: 'trapNoticed', mapId: 'm1', pointId: 'p1' },
      'onTrapNoticed',
      [{ kind: 'trapNoticed', mapId: 'm1', pointId: 'p1' }],
    ],
    ['creaturesChanged', { kind: 'creaturesChanged' }, 'onCreaturesChanged', []],
    ['spellCastsChanged', { kind: 'spellCastsChanged' }, 'onSpellCastsChanged', []],
    ['groupCheckChanged', { kind: 'groupCheckChanged' }, 'onGroupCheckChanged', []],
    ['contentChanged', { kind: 'contentChanged' }, 'onContentChanged', []],
    [
      'characterChanged',
      { kind: 'characterChanged', characterId: 'c1' },
      'onCharacterChanged',
      ['c1'],
    ],
    [
      'characterRevived',
      { kind: 'characterRevived', characterId: 'c1' },
      'onCharacterRevived',
      ['c1'],
    ],
    ['revivifyChanged', { kind: 'revivifyChanged' }, 'onRevivifyChanged', []],
    ['callingYou', { kind: 'callingYou' }, 'onCallingYou', []],
    ['puzzleChanged', { kind: 'puzzleChanged', puzzleId: 'pz' }, 'onPuzzleChanged', ['pz']],
  ];

  it.each(table)('hands %s to its handler, and only to it', async (_name, event, handler, args) => {
    stream.start();
    last().push({ kind: 'ready' });
    last().push(event);
    await flush();
    expect(handlers[handler]).toHaveBeenCalledTimes(1);
    expect(handlers[handler]).toHaveBeenCalledWith(...args);
    const others = (Object.keys(handlers) as (keyof Mocks)[]).filter(
      (k) => k !== handler && k !== 'onReady' && k !== 'onCharacterChanged',
    );
    for (const other of others) {
      expect(handlers[other], other).not.toHaveBeenCalled();
    }
    expect(stream.status()).toBe('live');
  });

  it('tells the revival as a change of the character first, then as a revival', async () => {
    const order: string[] = [];
    build({
      onCharacterChanged: vi.fn(() => order.push('changed')),
      onCharacterRevived: vi.fn(() => order.push('revived')),
    });
    stream.start();
    last().push({ kind: 'characterRevived', characterId: 'c1' });
    await flush();
    expect(order).toEqual(['changed', 'revived']);
  });

  it('skips a handler the page did not give, for every kind that has an optional one', async () => {
    build();
    stream = new LiveStream({
      open: (signal) => {
        const call = new FakeCall(signal);
        calls.push(call);
        return call.events();
      },
      classify: () => 'transient',
      handlers: { onReady: vi.fn(), onVitals: vi.fn(), onEnded: vi.fn(), onFatal: vi.fn() },
      document,
      random: () => 0.5,
    });
    stream.start();
    for (const [, event] of table) {
      last().push(event);
    }
    last().push({ kind: 'heartbeat' });
    await flush();
    expect(stream.status()).toBe('connecting');
  });

  it('hands the vitals, and stamps the time of every message, heartbeats too', async () => {
    stream.start();
    expect(stream.lastMessageAt()).toBeNull();
    last().push({ kind: 'heartbeat' });
    await flush();
    const first = stream.lastMessageAt();
    expect(first).toBeInstanceOf(Date);
    await vi.advanceTimersByTimeAsync(5_000);
    last().push({ kind: 'ready' });
    await flush();
    expect(stream.lastMessageAt()!.getTime()).toBeGreaterThan(first!.getTime());
    expect(stream.status()).toBe('live');
  });

  it('ignores a message of a kind it does not know, and keeps the stream', async () => {
    stream.start();
    last().push({ kind: 'ready' });
    last().push({ kind: 'somethingNew' } as unknown as LiveEventVm);
    last().push({ kind: 'xpChanged' });
    await flush();
    expect(handlers.onXpChanged).toHaveBeenCalledTimes(1);
    expect(stream.status()).toBe('live');
  });

  it('does not read what a replaced connection still sends', async () => {
    stream.start();
    last().push({ kind: 'ready' });
    await flush();
    const first = last();
    stream.restart();
    first.push({ kind: 'xpChanged' });
    await flush();
    expect(handlers.onXpChanged).not.toHaveBeenCalled();
  });
});

describe('LiveStream: the corners of its lifecycle', () => {
  let calls: FakeCall[];
  let stream: LiveStream;
  const flush = () => vi.advanceTimersByTimeAsync(0);
  const priv = () =>
    stream as unknown as {
      scheduleReconnect(): void;
      visibilityChanged(): void;
      connect(): Promise<void>;
    };

  beforeEach(() => {
    vi.useFakeTimers();
    setVisibility('visible');
    calls = [];
    stream = new LiveStream({
      open: (signal) => {
        const call = new FakeCall(signal);
        calls.push(call);
        return call.events();
      },
      classify: () => 'transient',
      handlers: {
        onReady: vi.fn(),
        onVitals: vi.fn(),
        onEnded: vi.fn(),
        onFatal: vi.fn(),
      },
      document,
      random: () => 0.5,
    });
  });

  afterEach(() => {
    stream.stop();
    vi.useRealTimers();
    setVisibility('visible');
  });

  it('waits to be seen when it is started in a hidden tab, then connects at once', async () => {
    setVisibility('hidden');
    stream.start();
    expect(stream.status()).toBe('paused');
    expect(calls.length).toBe(0);
    setVisibility('visible');
    await flush();
    expect(calls.length).toBe(1);
    expect(stream.status()).toBe('reconnecting');
  });

  it('does not restart a stream that is closed', async () => {
    stream.start();
    stream.stop();
    stream.restart();
    await vi.advanceTimersByTimeAsync(60_000);
    expect(calls.length).toBe(1);
    expect(stream.status()).toBe('closed');
  });

  it('connects to nothing once closed', async () => {
    stream.stop();
    await priv().connect();
    expect(calls.length).toBe(0);
    expect(stream.status()).toBe('closed');
  });

  it('pauses instead of connecting when the tab was hidden while the retry waited', async () => {
    stream.start();
    calls[0].fail(new Error('network'));
    await flush();
    expect(stream.status()).toBe('reconnecting');
    setVisibility('hidden');
    await vi.advanceTimersByTimeAsync(2_000);
    expect(calls.length).toBe(1);
    expect(stream.status()).toBe('paused');
  });

  it('schedules and watches nothing once closed', async () => {
    stream.start();
    stream.stop();
    priv().scheduleReconnect();
    priv().visibilityChanged();
    await vi.advanceTimersByTimeAsync(60_000);
    expect(stream.status()).toBe('closed');
    expect(calls.length).toBe(1);
  });
});

/** A call that does not notice it was aborted: it goes on yielding, or ends, as a slow network would. */
class StubbornCall {
  private readonly queue: (LiveEventVm | 'end' | Error)[] = [];
  private wake: (() => void) | null = null;

  push(event: LiveEventVm | 'end' | Error): void {
    this.queue.push(event);
    this.wake?.();
  }

  async *events(): AsyncIterable<LiveEventVm> {
    for (;;) {
      while (this.queue.length === 0) {
        await new Promise<void>((resolve) => (this.wake = resolve));
      }
      const next = this.queue.shift()!;
      if (next === 'end') {
        return;
      }
      if (next instanceof Error) {
        throw next;
      }
      yield next;
    }
  }
}

describe('LiveStream: a connection that was replaced', () => {
  let calls: StubbornCall[];
  let stream: LiveStream;
  let xp: number;
  const flush = () => vi.advanceTimersByTimeAsync(0);

  beforeEach(() => {
    vi.useFakeTimers();
    setVisibility('visible');
    calls = [];
    xp = 0;
    stream = new LiveStream({
      open: () => {
        const call = new StubbornCall();
        calls.push(call);
        return call.events();
      },
      classify: () => 'transient',
      handlers: {
        onReady: vi.fn(),
        onVitals: vi.fn(),
        onXpChanged: () => xp++,
        onEnded: vi.fn(),
        onFatal: vi.fn(),
      },
      document,
      random: () => 0.5,
    });
  });

  afterEach(() => {
    stream.stop();
    vi.useRealTimers();
    setVisibility('visible');
  });

  it('is dropped when it still sends a message', async () => {
    stream.start();
    calls[0].push({ kind: 'ready' });
    await flush();
    stream.restart();
    calls[0].push({ kind: 'xpChanged' });
    await flush();
    expect(xp).toBe(0);
    await vi.advanceTimersByTimeAsync(2_000);
    expect(calls.length).toBe(2);
    expect(stream.status()).toBe('reconnecting');
  });

  it('is dropped when it ends, without a second reconnection', async () => {
    stream.start();
    calls[0].push({ kind: 'ready' });
    await flush();
    stream.restart();
    calls[0].push('end');
    await flush();
    await vi.advanceTimersByTimeAsync(2_000);
    expect(calls.length).toBe(2);
  });

  it('is dropped when it fails late, too', async () => {
    stream.start();
    calls[0].push({ kind: 'ready' });
    await flush();
    stream.restart();
    calls[0].push(new Error('late'));
    await flush();
    await vi.advanceTimersByTimeAsync(2_000);
    expect(calls.length).toBe(2);
  });
});
