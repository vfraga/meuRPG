import { timestampFromMs } from '@bufbuild/protobuf/wkt';
import { describe, expect, it } from 'vitest';

import { ReactionKind } from '../../../gen/meurpg/play/v1/combat_pb';
import { reactionWindow } from './combat-testing';
import { AutoPassClock } from './reaction-autopass';

const T0 = Date.parse('2026-10-10T12:00:00Z');
const MS = 1000;

const readied = (over: object = {}) =>
  reactionWindow({
    id: 'w1',
    kind: ReactionKind.READIED,
    reactorId: 'pen',
    reactorLabel: 'Pensantus',
    reactorIsPlayer: true,
    answerNow: true,
    ...over,
  } as never);

describe("the master's clock of a readied window (#857)", () => {
  it('counts 30 s from when it first saw the window, as before', () => {
    const clock = new AutoPassClock();
    clock.sync([readied()], T0);
    expect(clock.secondsLeft('w1', T0 + 6 * MS)).toBe(24);
  });

  it('switches to the one wait of 120 s counted from the server mark, not from when this screen saw it', () => {
    const clock = new AutoPassClock();
    clock.sync([readied()], T0);
    clock.sync(
      [readied({ rolling: true, waitBeganAt: timestampFromMs(T0 + 10 * MS), waitedMs: 2000n })],
      T0 + 12 * MS,
    );
    expect(clock.secondsLeft('w1', T0 + 12 * MS)).toBe(118);
    expect(clock.secondsLeft('w1', T0 + 22 * MS)).toBe(108);
  });

  it('goes on from the same start when the player comes back, and never gives the 30 s back', () => {
    const clock = new AutoPassClock();
    const start = timestampFromMs(T0 + 10 * MS);
    clock.sync([readied({ rolling: true, waitBeganAt: start, waitedMs: 2000n })], T0 + 12 * MS);
    clock.sync([readied({ rolling: false, waitBeganAt: start, waitedMs: 48_000n })], T0 + 58 * MS);
    expect(clock.secondsLeft('w1', T0 + 58 * MS)).toBe(72);
  });

  it('counts a window the screen sees for the first time with the mark already set (a reload)', () => {
    const clock = new AutoPassClock();
    clock.sync(
      [readied({ rolling: true, waitBeganAt: timestampFromMs(T0), waitedMs: 40_000n })],
      T0 + 40 * MS,
    );
    expect(clock.secondsLeft('w1', T0 + 40 * MS)).toBe(80);
  });

  it('passes the window once, when the 120 s end', () => {
    const clock = new AutoPassClock();
    clock.sync(
      [readied({ rolling: false, waitBeganAt: timestampFromMs(T0), waitedMs: 5000n })],
      T0 + 5 * MS,
    );
    expect(clock.due(T0 + 119 * MS)).toEqual([]);
    expect(clock.due(T0 + 120 * MS)).toEqual(['w1']);
    expect(clock.due(T0 + 121 * MS)).toEqual([]);
  });

  it('does not use the mark of the other windows this way: a Shield window rolling keeps its own wait', () => {
    const clock = new AutoPassClock();
    const shield = reactionWindow({
      id: 's1',
      kind: ReactionKind.SHIELD,
      reactorIsPlayer: true,
      answerNow: true,
      rolling: true,
    } as never);
    clock.sync([shield], T0);
    expect(clock.secondsLeft('s1', T0 + 10 * MS)).toBe(110);
  });
});
