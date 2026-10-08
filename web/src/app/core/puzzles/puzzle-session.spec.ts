import { PuzzleRunStatus } from '../../../gen/meurpg/play/v1/puzzles_pb';
import type { MasterPuzzleRun } from '../../../gen/meurpg/play/v1/puzzles_pb';
import { PuzzleSessionState, type SessionApi } from './puzzle-session';
import {
  FakePuzzlesClient,
  asClient,
  lightsPuzzle,
  lockPuzzle,
  masterRun,
  summary,
} from './puzzles-testing';

const a = lightsPuzzle('a', 'O selo da Capela');
const b = lockPuzzle('b', 'O cofre do Refeitório');

function setup(master: boolean) {
  const fake = new FakePuzzlesClient();
  const state = new PuzzleSessionState(
    asClient(fake),
    () => 'camp-1',
    () => master,
  );
  return { fake, state };
}

describe('PuzzleSessionState (MR-038)', () => {
  it("reads the master's menu with where each puzzle stands", async () => {
    const { fake, state } = setup(true);
    fake.sessionResult = [
      masterRun(a, PuzzleRunStatus.SHOWN),
      masterRun(b, PuzzleRunStatus.NOT_SHOWN),
    ];
    await state.refresh();
    expect(state.runs().map((r) => r.puzzle?.name)).toEqual([
      'O selo da Capela',
      'O cofre do Refeitório',
    ]);
    expect(state.status()).toBe('ready');
  });

  it('reads only what the players are shown for a player', async () => {
    const { fake, state } = setup(false);
    fake.shownResult = [summary(a)];
    await state.refresh();
    expect(state.shown().map((s) => s.name)).toEqual(['O selo da Capela']);
    expect(fake.calls.map((c) => c[0])).toEqual(['listShown']);
  });

  it('reads one puzzle again on puzzle_changed (the hint names only its ID), not the whole menu', async () => {
    const { fake, state } = setup(true);
    fake.sessionResult = [
      masterRun(a, PuzzleRunStatus.SHOWN),
      masterRun(b, PuzzleRunStatus.NOT_SHOWN),
    ];
    await state.refresh();
    fake.calls = [];
    fake.runResults.set('a', masterRun(a, PuzzleRunStatus.SOLVED));
    await state.changed('a');
    expect(fake.calls.map((c) => c[0])).toEqual(['masterRun']);
    expect(state.runs()[0].status).toBe(PuzzleRunStatus.SOLVED);
    expect(state.runs()[1].status).toBe(PuzzleRunStatus.NOT_SHOWN);
  });

  it('reads the menu when the changed puzzle is not on it, and tells the open puzzle which one changed', async () => {
    const { fake, state } = setup(true);
    fake.sessionResult = [masterRun(a, PuzzleRunStatus.SHOWN)];
    await state.changed('zzz');
    expect(fake.calls.map((c) => c[0])).toEqual(['listSession']);
    expect(state.versionOf('zzz')).toBe(1);
    expect(state.versionOf('other')).toBe(0);
  });

  it('counts two hints of one turn, so the open puzzle misses none', async () => {
    const { fake, state } = setup(false);
    fake.shownResult = [summary(a)];
    await Promise.all([state.changed('a'), state.changed('a')]);
    expect(state.versionOf('a')).toBe(2);
    await state.refresh();
    expect(state.versionOf('a')).toBe(3); // a full read counts for every puzzle
    expect(state.versionOf('b')).toBe(1);
  });

  it('never puts back a puzzle with an older run revision than the one on screen', async () => {
    const { fake, state } = setup(true);
    const newer = masterRun(a, PuzzleRunStatus.SHOWN);
    newer.run = { ...newer.run!, revision: 7 };
    fake.sessionResult = [newer];
    await state.refresh();
    const older = masterRun(a, PuzzleRunStatus.SHOWN);
    older.run = { ...older.run!, revision: 3 };
    fake.runResults.set('a', older);
    await state.changed('a');
    expect(state.runs()[0].run?.revision).toBe(7);
  });

  it('keeps the list when a read fails, and says so', async () => {
    const { fake, state } = setup(true);
    fake.sessionResult = [masterRun(a, PuzzleRunStatus.SHOWN)];
    await state.refresh();
    fake.failWith = new Error('down');
    await state.refresh();
    expect(state.runs().length).toBe(1);
    expect(state.status()).toBe('error');
  });

  it("puts an action's answer in place at once, and a read that started before it never wins", async () => {
    const { fake, state } = setup(true);
    fake.sessionResult = [masterRun(a, PuzzleRunStatus.SHOWN)];
    await state.refresh();
    let release!: (r: ReturnType<typeof masterRun>) => void;
    fake.masterRun = () => new Promise((resolve) => (release = resolve));
    const slow = state.changed('a');
    state.replace(masterRun(a, PuzzleRunStatus.CLOSED));
    release(masterRun(a, PuzzleRunStatus.SHOWN));
    await slow;
    expect(state.runs()[0].status).toBe(PuzzleRunStatus.CLOSED);
  });

  it('forgets everything when the session page opens another campaign', async () => {
    const { fake, state } = setup(true);
    fake.sessionResult = [masterRun(a, PuzzleRunStatus.SHOWN)];
    await state.refresh();
    state.clear();
    expect(state.runs()).toEqual([]);
    expect(state.status()).toBe('idle');
  });
});

function deferredValue<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((r) => (resolve = r));
  return { promise, resolve };
}

function slowListSetup() {
  const list = deferredValue<MasterPuzzleRun[]>();
  let nextRun: MasterPuzzleRun = masterRun(a, PuzzleRunStatus.SHOWN);
  const api = {
    listSession: () => list.promise,
    masterRun: async () => nextRun,
    listShown: async () => [],
  } as unknown as SessionApi;
  const state = new PuzzleSessionState(
    api,
    () => 'camp-1',
    () => true,
  );
  return { list, state, setNext: (r: MasterPuzzleRun) => (nextRun = r) };
}

describe('PuzzleSessionState, a list read that began before a newer write', () => {
  it('the answer of a master action is not undone by an older list answer', async () => {
    const { list, state } = slowListSetup();
    const pending = state.refresh();
    state.replace(masterRun(a, PuzzleRunStatus.SOLVED)); // Reset/Close answered meanwhile
    list.resolve([masterRun(a, PuzzleRunStatus.SHOWN)]); // pre-action snapshot
    await pending;
    expect(state.runs().find((r) => r.puzzle?.id === 'a')?.status).toBe(PuzzleRunStatus.SOLVED);
  });

  it('a puzzle_changed read applied meanwhile is not undone by an older list answer', async () => {
    const { list, state, setNext } = slowListSetup();
    // the puzzle is already known on screen, so changed() reads just that puzzle
    state.replace(masterRun(a, PuzzleRunStatus.SHOWN));
    const pending = state.refresh();
    setNext(masterRun(a, PuzzleRunStatus.SOLVED));
    await state.changed('a');
    list.resolve([masterRun(a, PuzzleRunStatus.SHOWN)]);
    await pending;
    expect(state.runs()[0].status).toBe(PuzzleRunStatus.SOLVED);
  });
});
