// Finding U11-20: a slow listSession (ready/reconnect) landing after a newer replace()/changed() overwrites it with a stale snapshot.
import { PuzzleRunStatus } from '../../../gen/meurpg/play/v1/puzzles_pb';
import type { MasterPuzzleRun } from '../../../gen/meurpg/play/v1/puzzles_pb';
import { PuzzleSessionState, type SessionApi } from './puzzle-session';
import { lightsPuzzle, masterRun } from './puzzles-testing';

const a = lightsPuzzle('a', 'O selo da Capela');

function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((r) => (resolve = r));
  return { promise, resolve };
}

function setup() {
  const list = deferred<MasterPuzzleRun[]>();
  let nextRun: MasterPuzzleRun = masterRun(a, PuzzleRunStatus.SHOWN);
  const api = {
    listSession: () => list.promise,
    masterRun: async () => nextRun,
    listShown: async () => [],
  } as unknown as SessionApi;
  const state = new PuzzleSessionState(api, () => 'camp-1', () => true);
  return { list, state, setNext: (r: MasterPuzzleRun) => (nextRun = r) };
}

describe('Review11 U11-20: stale list read overwrites a newer write', () => {
  it('replace() (master action) is not undone by an older listSession answer', async () => {
    const { list, state } = setup();
    const pending = state.refresh();
    state.replace(masterRun(a, PuzzleRunStatus.SOLVED)); // Reset/Close answered meanwhile
    list.resolve([masterRun(a, PuzzleRunStatus.SHOWN)]); // pre-action snapshot
    await pending;
    expect(state.runs().find((r) => r.puzzle?.id === 'a')?.status).toBe(PuzzleRunStatus.SOLVED);
  });

  it('a puzzle_changed read applied meanwhile is not undone by an older listSession answer', async () => {
    const { list, state, setNext } = setup();
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
