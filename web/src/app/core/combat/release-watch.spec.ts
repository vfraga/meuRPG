import { TestBed } from '@angular/core/testing';
import { timestampFromMs } from '@bufbuild/protobuf/wkt';
import { create } from '@bufbuild/protobuf';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import {
  CombatantState,
  ReactionKind,
  ReadiedActionSchema,
  ReadiedKind,
  ReadiedStatus,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { CombatState } from './combat-state';
import { combatant, encounter, reactionWindow } from './combat-testing';
import { ReleaseWatch } from './release-watch';

const T0 = Date.parse('2026-10-10T12:00:00Z');
const MS = 1000;

const row = (status = ReadiedStatus.WAITING) =>
  create(ReadiedActionSchema, {
    id: 'r1',
    kind: ReadiedKind.SPELL,
    actionKey: 'spell:fireball',
    actionNamePt: 'Bola de Fogo',
    status,
  });

const win = (over: object = {}) =>
  reactionWindow({
    id: 'w1',
    kind: ReactionKind.READIED,
    reactorId: 'pen',
    reactorLabel: 'Pensantus',
    reactorIsPlayer: true,
    ...over,
  } as never);

function setup(opts: { move?: boolean } = {}) {
  const state = new CombatState();
  const open = (windows: ReturnType<typeof win>[], pen: object = {}, revision = 1) => {
    state.apply(
      encounter({
        revision,
        combatants: [combatant({ id: 'pen', label: 'Pensantus', readied: row(), ...pen })],
        reactionWindows: windows,
      }),
    );
    TestBed.tick();
  };
  open([win({ rolling: true, waitBeganAt: timestampFromMs(T0), waitedMs: 12_000n })]);
  const watch = TestBed.runInInjectionContext(
    () =>
      new ReleaseWatch(() => state.encounter(), {
        windowId: 'w1',
        readiedId: 'r1',
        reactorId: 'pen',
        spellName: 'Bola de Fogo',
        move: opts.move ?? false,
      }),
  );
  return { state, watch, open };
}

describe('a release watching its window (#857, #856; 857-09 cards 1 and 7)', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(T0 + 12 * MS);
  });
  afterEach(() => vi.useRealTimers());

  it('is not gone while the window is in the combat, and counts the 120 s from the server start', () => {
    const { watch } = setup();
    expect(watch.gone()).toBe(false);
    expect(watch.view()).toBeNull();
    expect(watch.secondsLeft()).toBe(108);
    vi.advanceTimersByTime(8 * MS);
    expect(watch.secondsLeft()).toBe(100);
  });

  it('says the master undid the trigger when the window went away and the readied row still waits', () => {
    const { watch, open } = setup();
    open([], {}, 2);
    expect(watch.gone()).toBe(true);
    expect(watch.view()).toEqual({
      lead: 'O mestre desfez o gatilho:',
      rest: 'a Bola de Fogo continua preparada.',
    });
  });

  it('says the spell and the slot are lost when the row does not wait any more', () => {
    const { watch, open } = setup();
    open([], { readied: undefined }, 2);
    expect(watch.view()?.lead).toBe('A janela fechou.');
    expect(watch.view()?.rest).toContain('a magia preparada e o espaço dela se perderam');
  });

  it('says the reaction was used, and that the character fell', () => {
    const used = setup();
    used.open([], { readied: undefined, reactionUsed: true }, 2);
    expect(used.watch.view()?.lead).toBe('A janela fechou e a sua reação foi usada.');
    const down = setup();
    down.open([], { readied: undefined, state: CombatantState.DOWN }, 2);
    expect(down.watch.view()?.lead).toBe('Condição Caído.');
  });

  it('says it passed by itself in 2 minutes when the clock ran out before the window went', () => {
    const { watch, open } = setup();
    vi.advanceTimersByTime(109 * MS);
    open([], { readied: undefined }, 2);
    expect(watch.view()).toEqual({ lead: 'Passou sozinho:', rest: 'você não respondeu em 2 min.' });
  });

  it('says the move was not made, for a move', () => {
    const { watch, open } = setup({ move: true });
    open([], { readied: undefined }, 2);
    expect(watch.view()?.rest).toBe('Você não se moveu, e a reação continua livre.');
  });

  it('keeps what it said once the window is gone, whatever the combat says next', () => {
    const { watch, open } = setup();
    open([], {}, 2);
    vi.advanceTimersByTime(500 * MS);
    open([], { readied: undefined }, 3);
    expect(watch.view()?.lead).toBe('O mestre desfez o gatilho:');
    expect(watch.view()?.rest).toBe('a Bola de Fogo continua preparada.');
  });

  it('does not restamp a window it already read: the same read seen again at +48 s still has 60 s left', () => {
    const { state, watch, open } = setup();
    const again = win({ rolling: true, waitBeganAt: timestampFromMs(T0), waitedMs: 60_000n });
    open([again], {}, 2);
    expect(watch.secondsLeft()).toBe(60);
    vi.advanceTimersByTime(48 * MS);
    // The combat is read again with the same window object: its stamp stays, the 48 s count.
    state.apply(
      encounter({
        revision: 3,
        combatants: [combatant({ id: 'pen', label: 'Pensantus', readied: row() })],
        reactionWindows: [again],
      }),
    );
    TestBed.tick();
    expect(watch.secondsLeft()).toBe(12);
  });

  it('counts a window read again with the same waited_ms from its own read, not from the first', () => {
    const { watch, open } = setup();
    open([win({ rolling: true, waitBeganAt: timestampFromMs(T0), waitedMs: 60_000n })], {}, 2);
    vi.advanceTimersByTime(48 * MS);
    open([win({ rolling: true, waitBeganAt: timestampFromMs(T0), waitedMs: 60_000n })], {}, 3);
    expect(watch.secondsLeft()).toBe(60);
  });

  it("counts from waited_ms when the window's wait_began_at is an hour off the device clock", () => {
    const { watch, open } = setup();
    open(
      [win({ rolling: true, waitBeganAt: timestampFromMs(T0 - 3_600_000), waitedMs: 30_000n })],
      {},
      2,
    );
    expect(watch.secondsLeft()).toBe(90);
    vi.advanceTimersByTime(10 * MS);
    expect(watch.secondsLeft()).toBe(80);
  });

  it('keeps the 120 s on a monotonic clock: a wall-clock jump of +1 h mid-wait does not pass the window', () => {
    const { watch } = setup();
    vi.advanceTimersByTime(8 * MS);
    vi.setSystemTime(T0 + 12 * MS + 8 * MS + 3_600_000);
    vi.advanceTimersByTime(MS);
    expect(watch.secondsLeft()).toBe(99);
  });

  it('stops its one-second clock and its watch on destroy(), before the injector goes', () => {
    vi.useFakeTimers();
    try {
      const state = new CombatState();
      state.apply(
        encounter({
          combatants: [combatant({ id: 'pen', label: 'Pensantus', readied: row() })],
          reactionWindows: [win()],
        }),
      );
      const watch = TestBed.runInInjectionContext(
        () =>
          new ReleaseWatch(() => state.encounter(), {
            windowId: 'w1',
            readiedId: 'r1',
            reactorId: 'pen',
            spellName: 'Bola de Fogo',
            move: false,
          }),
      );
      const stop = vi.spyOn(globalThis, 'clearInterval');
      watch.destroy();
      expect(stop).toHaveBeenCalledTimes(1);
    } finally {
      vi.useRealTimers();
    }
  });
});
