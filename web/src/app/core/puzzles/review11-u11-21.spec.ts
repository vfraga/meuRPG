// Finding ids U11-21, U11-22, U11-23: PuzzlePlay late answers/queued taps of puzzle A leak into puzzle B after open(B).
import { Code, ConnectError } from '@connectrpc/connect';

import type { PuzzleRun, TryPuzzleHintResponse } from '../../../gen/meurpg/play/v1/puzzles_pb';
import { PuzzlePlay, type PlayApi } from './puzzle-play';
import type { MoveAnswer } from './puzzles-client';
import { hintAnswer, lightsPuzzle, playerRun, sequencePuzzle } from './puzzles-testing';

interface Deferred<T> {
  promise: Promise<T>;
  resolve: (v: T) => void;
  reject: (e: unknown) => void;
}
function deferred<T>(): Deferred<T> {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

class Api implements PlayApi {
  runs: { id: string; d: Deferred<PuzzleRun> }[] = [];
  moves: { id: string; d: Deferred<MoveAnswer> }[] = [];
  hints: { id: string; d: Deferred<TryPuzzleHintResponse> }[] = [];
  run(_c: string, id: string): Promise<PuzzleRun> {
    const d = deferred<PuzzleRun>();
    this.runs.push({ id, d });
    return d.promise;
  }
  move(_c: string, id: string): Promise<MoveAnswer> {
    const d = deferred<MoveAnswer>();
    this.moves.push({ id, d });
    return d.promise;
  }
  tryHint(_c: string, id: string): Promise<TryPuzzleHintResponse> {
    const d = deferred<TryPuzzleHintResponse>();
    this.hints.push({ id, d });
    return d.promise;
  }
}

const flush = async () => {
  for (let i = 0; i < 10; i++) {
    await Promise.resolve();
  }
};

function setup(): { api: Api; play: PuzzlePlay } {
  const api = new Api();
  let key = 0;
  const play = new PuzzlePlay(
    api,
    () => 'camp-1',
    async () => undefined,
    () => `key-${++key}`,
    () => '',
    () => () => undefined,
  );
  return { api, play };
}

const A = lightsPuzzle('A', 'Puzzle A');
const B = lightsPuzzle('B', 'Puzzle B');
const runOf = (p = A, partial: Record<string, unknown> = {}): PuzzleRun => playerRun(p, partial);

/** Opens `id` and answers its read with `run`. */
async function openWith(api: Api, play: PuzzlePlay, id: string, run: PuzzleRun): Promise<void> {
  const opening = play.open(id);
  api.runs[api.runs.length - 1].d.resolve(run);
  await opening;
}

describe('Review11 U11-21: a late NotFound refresh of puzzle A marks puzzle B as gone', () => {
  it('does not set gone on B when A\'s in-flight refresh fails with NotFound after open(B)', async () => {
    const { api, play } = setup();
    await openWith(api, play, 'A', runOf(A));
    const lateA = play.refresh();
    const readA = api.runs[api.runs.length - 1];
    expect(readA.id).toBe('A');

    await openWith(api, play, 'B', runOf(B));
    expect(play.run()?.puzzleId).toBe('B');
    expect(play.gone()).toBe(false);

    readA.d.reject(new ConnectError('closed', Code.NotFound));
    await lateA;
    expect(play.gone()).toBe(false);
  });
});

describe('Review11 U11-22: puzzle A\'s hint result lands on puzzle B', () => {
  it('does not show A\'s hint result (or its error message) on B after open(B)', async () => {
    const { api, play } = setup();
    await openWith(api, play, 'A', runOf(A, { canTryHint: true }));
    const trying = play.tryHint({ case: 'app' } as never);
    await flush();
    expect(api.hints).toHaveLength(1);

    await openWith(api, play, 'B', runOf(B, { canTryHint: true }));
    api.hints[0].d.resolve(hintAnswer(runOf(A, { canTryHint: true, revision: 2 }), true));
    await trying;

    expect(play.run()?.puzzleId).toBe('B');
    expect(play.hintTry()).toBeNull();
  });

  it('does not write A\'s hint error message on B', async () => {
    const { api, play } = setup();
    await openWith(api, play, 'A', runOf(A, { canTryHint: true }));
    const trying = play.tryHint({ case: 'app' } as never);
    await flush();
    await openWith(api, play, 'B', runOf(B, { canTryHint: true }));
    api.hints[0].d.reject(new ConnectError('boom', Code.PermissionDenied));
    const refreshing = trying;
    await flush();
    // let the refresh in the error branch read (for A's id or B's) if it started
    for (const r of api.runs.slice(2)) {
      r.d.resolve(runOf(B));
    }
    await refreshing;
    expect(play.message()).toBe('');
  });
});

describe('Review11 U11-23: queued bell taps of puzzle A run against puzzle B', () => {
  it('sends no move to B for taps made on A', async () => {
    const { api, play } = setup();
    const seq = sequencePuzzle('A', 'Sinos');
    const seqB = sequencePuzzle('B', 'Outros sinos');
    const sequence = { totalSteps: 6, plays: 1, playing: false, shown: [], stepMs: 1200, nextInMs: 0 };
    await openWith(api, play, 'A', runOf(seq, { sequence }));
    const bell = (n: number) => ({ kind: { case: 'sequence' as const, value: { bell: n } } });

    const t1 = play.move(bell(0));
    const t2 = play.move(bell(1));
    const t3 = play.move(bell(2));
    await flush();
    expect(api.moves.map((m) => m.id)).toEqual(['A']);

    await openWith(api, play, 'B', runOf(seqB, { sequence }));
    api.moves[0].d.resolve({
      run: runOf(seq, { sequence, revision: 2 }),
      replayed: false,
      solvedByThisMove: false,
    });
    await flush();
    // Taps go one at a time, so each wrongly sent one releases the next: drain until nothing is left to answer (bounded).
    const answered = new Set<unknown>();
    for (let round = 0; round < 10; round++) {
      await flush();
      const fresh = api.moves.filter((m) => !answered.has(m));
      if (fresh.length === 0) {
        break;
      }
      for (const m of fresh) {
        answered.add(m);
        m.d.resolve({ run: runOf(seqB, { sequence, revision: 2 + round }), replayed: false, solvedByThisMove: false });
      }
    }
    await Promise.allSettled([t1, t2, t3]);

    expect(api.moves.filter((m) => m.id === 'B')).toHaveLength(0);
  });
});
