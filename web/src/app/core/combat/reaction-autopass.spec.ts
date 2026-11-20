import { timestampFromMs } from '@bufbuild/protobuf/wkt';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { ReactionKind, ReactionWindowStatus } from '../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter, reactionWindow } from './combat-testing';
import {
  AUTO_PASS_SECONDS,
  AutoPassClock,
  autoPassKey,
  autoPassText,
  autoPasses,
  playerAutoPassText,
  playerChoosingText,
  playerRollingSecondsLeft,
  playerRollingText,
  playerSecondsLeft,
  ROLLING_WAIT_SECONDS,
  rollingText,
} from './reaction-autopass';
import { windowsBarText } from './reaction-master';

const MS = 1000;

function win(id: string, kind: ReactionKind, over: object = {}) {
  return reactionWindow({
    id,
    kind,
    reactorId: 'pen',
    reactorLabel: 'Pensantus',
    reactorIsPlayer: true,
    answerNow: true,
    ...over,
  } as never);
}

describe("the automatic pass of a player's optional reaction", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-10-10T12:00:00Z'));
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it('never passes a window that came to the master: his answer is the one that counts', () => {
    expect(autoPasses(win('w', ReactionKind.SHIELD))).toBe(true);
    expect(autoPasses(win('w', ReactionKind.SHIELD, { unanswered: true }))).toBe(false);
  });

  it('still passes a readied action flagged as unanswered: the server flags every window 10 s old once the combat is sent again (#857)', () => {
    expect(autoPasses(win('w', ReactionKind.READIED, { unanswered: true }))).toBe(true);
  });

  it("passes a monk's Queda Lenta like any other optional reaction", () => {
    expect(autoPasses(win('w', ReactionKind.SLOW_FALL))).toBe(true);
  });

  it("passes a player's fired readied action at 30 s, never an NPC's", () => {
    const clock = new AutoPassClock();
    const mine = win('w1', ReactionKind.READIED);
    const npc = win('w2', ReactionKind.READIED, { reactorIsPlayer: false });
    expect(autoPasses(mine)).toBe(true);
    expect(autoPasses(npc)).toBe(false);
    clock.sync([mine, npc], Date.now());
    vi.advanceTimersByTime(AUTO_PASS_SECONDS * MS);
    clock.sync([mine, npc], Date.now());
    expect(clock.due(Date.now())).toEqual(['w1']);
  });

  it("passes a player's reaction feature at 30 s, never a hidden creature's window (the master's from the start)", () => {
    const clock = new AutoPassClock();
    const mine = win('w1', ReactionKind.FEATURE);
    const hidden = win('w2', ReactionKind.FEATURE, { unanswered: true });
    expect(autoPasses(mine)).toBe(true);
    expect(autoPasses(hidden)).toBe(false);
    clock.sync([mine, hidden], Date.now());
    vi.advanceTimersByTime(AUTO_PASS_SECONDS * MS);
    clock.sync([mine, hidden], Date.now());
    expect(clock.due(Date.now())).toEqual(['w1']);
  });

  it('never passes a readied attack whose roll is held for other reactions (answerNow false)', () => {
    const clock = new AutoPassClock();
    const held = win('w1', ReactionKind.READIED, { answerNow: false });
    expect(autoPasses(held)).toBe(false);
    clock.sync([held], Date.now());
    vi.advanceTimersByTime(ROLLING_WAIT_SECONDS * MS);
    clock.sync([held], Date.now());
    expect(clock.due(Date.now())).toEqual([]);
  });

  it('passes an optional player window once, at 30 s, with the auto-pass key', () => {
    const clock = new AutoPassClock();
    const w = win('w1', ReactionKind.CUTTING_WORDS);
    clock.sync([w], Date.now());
    expect(clock.due(Date.now())).toEqual([]);
    vi.advanceTimersByTime((AUTO_PASS_SECONDS - 1) * MS);
    clock.sync([w], Date.now());
    expect(clock.due(Date.now())).toEqual([]);
    vi.advanceTimersByTime(MS);
    clock.sync([w], Date.now());
    expect(clock.due(Date.now())).toEqual(['w1']);
    // Once: a second look does not send it again.
    expect(clock.due(Date.now())).toEqual([]);
    // The server takes only a UUID as a key: the window's id with its digits flipped, the same on every screen.
    const id = '0f1e2d3c-4b5a-4987-a6b5-c4d3e2f1a0b9';
    expect(autoPassKey(id)).toBe('f0e1d2c3-b4a5-4678-a94a-3b2c1d0e5f46');
    expect(autoPassKey(id)).toMatch(
      /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/,
    );
    expect(autoPassKey(id.toUpperCase())).toBe(autoPassKey(id));
  });

  it('passes a Proteção window too: using it is a choice', () => {
    const clock = new AutoPassClock();
    const w = win('w1', ReactionKind.PROTECTION);
    clock.sync([w], Date.now());
    vi.advanceTimersByTime(AUTO_PASS_SECONDS * MS);
    clock.sync([w], Date.now());
    expect(clock.due(Date.now())).toEqual(['w1']);
  });

  it('may send a pass again after a failure that was not a refusal', () => {
    const clock = new AutoPassClock();
    const w = win('w1', ReactionKind.SHIELD);
    clock.sync([w], Date.now());
    vi.advanceTimersByTime(AUTO_PASS_SECONDS * MS);
    expect(clock.due(Date.now())).toEqual(['w1']);
    clock.release('w1');
    expect(clock.due(Date.now())).toEqual(['w1']);
  });

  it('never passes a saving throw, a contest, the master check or an NPC window', () => {
    const never = [
      win('c', ReactionKind.CONCENTRATION_SAVE),
      win('e', ReactionKind.EFFECT_SAVE),
      win('k', ReactionKind.CONTEST),
      win('m', ReactionKind.MASTER_CHECK),
      win('r', ReactionKind.HELLISH_REBUKE, { secondStep: true }),
      win('n', ReactionKind.SHIELD, { reactorIsPlayer: false }),
      win('q', ReactionKind.SHIELD, { answerNow: false }),
    ];
    for (const w of never) {
      expect(autoPasses(w)).toBe(false);
    }
    const clock = new AutoPassClock();
    clock.sync(never, Date.now());
    vi.advanceTimersByTime(AUTO_PASS_SECONDS * 2 * MS);
    expect(clock.due(Date.now())).toEqual([]);
  });

  it('does not pass a window answered before 30 s', () => {
    const clock = new AutoPassClock();
    const w = win('w1', ReactionKind.UNCANNY_DODGE);
    clock.sync([w], Date.now());
    vi.advanceTimersByTime(10 * MS);
    const answered = win('w1', ReactionKind.UNCANNY_DODGE, {
      status: ReactionWindowStatus.ANSWERED,
    });
    clock.sync([answered], Date.now());
    vi.advanceTimersByTime(AUTO_PASS_SECONDS * MS);
    expect(clock.due(Date.now())).toEqual([]);
    expect(clock.soonest(Date.now())).toBeNull();
  });

  it('starts the 30 s when the window becomes the one to answer, not before', () => {
    const clock = new AutoPassClock();
    clock.sync([win('w1', ReactionKind.COUNTERSPELL, { answerNow: false })], Date.now());
    vi.advanceTimersByTime(20 * MS);
    clock.sync([win('w1', ReactionKind.COUNTERSPELL)], Date.now());
    expect(clock.secondsLeft('w1', Date.now())).toBe(AUTO_PASS_SECONDS);
  });

  it('passes an opportunity attack offer once at 30 s, with a UUID key, and forgets a gone one', () => {
    const clock = new AutoPassClock();
    const offer = '3f2a9c1e-5b7d-4e68-9a10-c2d4e6f80b13';
    clock.syncIds([offer], Date.now());
    vi.advanceTimersByTime(29 * MS);
    expect(clock.due(Date.now())).toEqual([]);
    vi.advanceTimersByTime(MS);
    expect(clock.due(Date.now())).toEqual([offer]);
    expect(clock.due(Date.now())).toEqual([]);
    expect(autoPassKey(offer)).toMatch(
      /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/,
    );
    clock.syncIds([], Date.now());
    expect(clock.soonest(Date.now())).toBeNull();
  });

  it('counts the seconds down, and writes them for the master and for the player', () => {
    const clock = new AutoPassClock();
    clock.sync([win('w1', ReactionKind.CUTTING_WORDS)], Date.now());
    vi.advanceTimersByTime(6 * MS);
    expect(clock.secondsLeft('w1', Date.now())).toBe(24);
    expect(autoPassText(24)).toBe('Passa sozinho em 24 s');
    expect(playerAutoPassText(24)).toBe('Se você não responder, passa sozinho em 24 s.');
    expect(playerSecondsLeft(Date.now() - 6 * MS, Date.now())).toBe(24);
    expect(playerSecondsLeft(Date.now() - 99 * MS, Date.now())).toBe(0);
  });

  it("keeps the bar's words for a readied action flagged as unanswered: the table waits for the player, not for the master (#857)", () => {
    const e = encounter({
      combatants: [combatant({ id: 'pen', label: 'Pensantus' })],
      reactionWindows: [win('w1', ReactionKind.READIED, { unanswered: true })],
    } as never);
    expect(windowsBarText(e)).toBe('Esperando a reação de Pensantus');
  });

  it("goes with the master's bar text", () => {
    const e = encounter({
      combatants: [combatant({ id: 'pen', label: 'Pensantus' })],
      reactionWindows: [win('w1', ReactionKind.CUTTING_WORDS)],
    } as never);
    expect(windowsBarText(e)).toBe('Esperando a reação de Pensantus');
  });
});

describe('a reaction its player is rolling', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-10-10T12:00:00Z'));
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it('is not passed at 30 s but waits the longer, bounded time counted from the mark', () => {
    const clock = new AutoPassClock();
    clock.sync([win('w1', ReactionKind.CUTTING_WORDS)], Date.now());
    // The mark arrives at 20 s: the longer wait starts then, not at 0.
    vi.advanceTimersByTime(20 * MS);
    const rolling = win('w1', ReactionKind.CUTTING_WORDS, { rolling: true });
    clock.sync([rolling], Date.now());
    expect(clock.isRolling('w1')).toBe(true);
    expect(clock.secondsLeft('w1', Date.now())).toBe(ROLLING_WAIT_SECONDS);
    vi.advanceTimersByTime(AUTO_PASS_SECONDS * MS);
    clock.sync([rolling], Date.now());
    expect(clock.due(Date.now())).toEqual([]);
    vi.advanceTimersByTime((ROLLING_WAIT_SECONDS - AUTO_PASS_SECONDS) * MS);
    clock.sync([rolling], Date.now());
    // Bounded: a player who walked away does not hold the table forever.
    expect(clock.secondsLeft('w1', Date.now())).toBe(0);
    expect(clock.due(Date.now())).toEqual(['w1']);
  });

  it('waits the longer time for a readied action too: the master reads the same mark of a READIED window (#857)', () => {
    const clock = new AutoPassClock();
    // The server's mark dates the one wait (`wait_began_at`) along with `rolling`: the clock reads that date, not its own sight of the mark.
    const held = win('w1', ReactionKind.READIED, {
      rolling: true,
      waitBeganAt: timestampFromMs(Date.now()),
      waitedMs: 0n,
    });
    clock.sync([held], Date.now());
    expect(clock.secondsLeft('w1', Date.now())).toBe(ROLLING_WAIT_SECONDS);
    vi.advanceTimersByTime(AUTO_PASS_SECONDS * MS);
    clock.sync([held], Date.now());
    expect(clock.due(Date.now())).toEqual([]);
  });

  it("counts a marked readied window from the server's waited_ms and the time since it was read, not from this device's clock (#857)", () => {
    const clock = new AutoPassClock();
    // This device's clock is an hour behind the server's: wait_began_at is an hour ahead of it.
    const held = win('w1', ReactionKind.READIED, {
      rolling: true,
      waitBeganAt: timestampFromMs(Date.now() + 3_600_000),
      waitedMs: 30_000n,
    });
    clock.sync([held], Date.now());
    expect(clock.secondsLeft('w1', Date.now())).toBe(ROLLING_WAIT_SECONDS - 30);
    vi.advanceTimersByTime(10 * MS);
    clock.sync([held], Date.now());
    expect(clock.secondsLeft('w1', Date.now())).toBe(ROLLING_WAIT_SECONDS - 40);
    vi.advanceTimersByTime(5 * MS);
    expect(clock.secondsLeft('w1', Date.now())).toBe(ROLLING_WAIT_SECONDS - 45);
  });

  it('tells the player who is choosing the targets how long the table waits', () => {
    expect(playerChoosingText(120)).toBe(
      'Você está escolhendo: a mesa espera 2 min. Depois disso, passa sozinho.',
    );
  });

  it('forgets the mark when the window is gone', () => {
    const clock = new AutoPassClock();
    clock.sync([win('w1', ReactionKind.SHIELD, { rolling: true })], Date.now());
    clock.sync([], Date.now());
    expect(clock.isRolling('w1')).toBe(false);
  });

  it('says it to the master and to the player, with minutes', () => {
    expect(rollingText(112)).toBe('Rolando… passa sozinho em 112 s');
    expect(playerRollingText(112)).toContain('a mesa espera 1 min 52 s');
    expect(playerRollingText(120)).toContain('a mesa espera 2 min.');
    expect(playerRollingText(45)).toContain('a mesa espera 45 s.');
  });

  it("counts the player's own wait down from 2 minutes and never above it", () => {
    const t = Date.now();
    expect(playerRollingSecondsLeft(t, t)).toBe(ROLLING_WAIT_SECONDS);
    expect(playerRollingSecondsLeft(t, t - MS)).toBe(ROLLING_WAIT_SECONDS);
    expect(playerRollingSecondsLeft(t, t + 100 * MS)).toBe(20);
    expect(playerRollingSecondsLeft(t, t + 500 * MS)).toBe(0);
  });
});
