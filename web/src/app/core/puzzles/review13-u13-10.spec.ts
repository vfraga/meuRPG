// Finding U13-10 (review/unit-13-web-live-rest.md): a late NotFound of puzzle A's refresh marks the newly opened puzzle B as gone.
import { Code, ConnectError } from '@connectrpc/connect';

import type { PuzzleRun } from '../../../gen/meurpg/play/v1/puzzles_pb';
import { PuzzlePlay, type PlayApi } from './puzzle-play';
import { lightsPuzzle, playerRun } from './puzzles-testing';

const runOf = (id: string): PuzzleRun => ({
  ...playerRun(lightsPuzzle(id, `Puzzle ${id}`)),
  revision: 1,
});

interface Call {
  id: string;
  resolve: (r: PuzzleRun) => void;
  reject: (e: unknown) => void;
}

function setup(): { play: PuzzlePlay; calls: Call[] } {
  const calls: Call[] = [];
  const api: PlayApi = {
    run: (_c, id) =>
      new Promise<PuzzleRun>((resolve, reject) => calls.push({ id, resolve, reject })),
    move: () => Promise.reject(new Error('unused')),
    tryHint: () => Promise.reject(new Error('unused')),
  };
  const play = new PuzzlePlay(
    api,
    () => 'camp-1',
    async () => undefined,
    () => 'key',
    () => '',
    () => () => undefined,
  );
  return { play, calls };
}

describe('Review13 U13-10: a late NotFound of the previous puzzle must not close the new one', () => {
  it('control: B opens normally when A answers late with a run', async () => {
    const { play, calls } = setup();
    const a = play.open('A');
    const b = play.open('B');
    calls[0].resolve(runOf('A'));
    calls[1].resolve(runOf('B'));
    await Promise.all([a, b]);
    expect(play.gone()).toBe(false);
    expect(play.run()?.puzzleId).toBe('B');
  });

  it('keeps B playable when the refresh of A is rejected NotFound after open(B)', async () => {
    const { play, calls } = setup();
    const a = play.open('A');
    const b = play.open('B');
    expect(calls.map((c) => c.id)).toEqual(['A', 'B']);
    calls[1].resolve(runOf('B'));
    await b;
    expect(play.run()?.puzzleId).toBe('B');
    // The master had closed A: its read lands now, after B was opened.
    calls[0].reject(new ConnectError('closed', Code.NotFound));
    await a;
    expect(play.gone()).toBe(false);
    expect(play.run()?.puzzleId).toBe('B');
  });
});
