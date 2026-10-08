import type { PuzzleRun } from '../../../gen/meurpg/play/v1/puzzles_pb';
import { Code, ConnectError } from '@connectrpc/connect';

import { PuzzlePlay } from './puzzle-play';
import { FakePuzzlesClient, playerRun, riddlePuzzle } from './puzzles-testing';

const riddle = riddlePuzzle('p1', 'A porta da Cripta pergunta');
const answer = { kind: { case: 'riddle' as const, value: { answer: 'escuridão' } } };
const open = (revision: number): PuzzleRun => ({ ...playerRun(riddle), revision });

type Reply = { run: PuzzleRun; replayed: boolean; solvedByThisMove: boolean; wrong: boolean };

async function setup(): Promise<{ fake: FakePuzzlesClient; play: PuzzlePlay }> {
  const fake = new FakePuzzlesClient();
  let key = 0;
  const play = new PuzzlePlay(
    fake,
    () => 'camp-1',
    async () => undefined,
    () => `key-${++key}`,
    () => () => undefined,
  );
  fake.playerRunResult = open(1);
  await play.open('p1');
  return { fake, play };
}

describe('PuzzlePlay: a riddle answer sent twice in one frame', () => {
  it('control: one answer makes one call and counts one in flight', async () => {
    const { fake, play } = await setup();
    const resolvers: ((r: Reply) => void)[] = [];
    fake.moveResult = () => new Promise((resolve) => resolvers.push(resolve));
    const a = play.move(answer);
    expect(fake.moveKeys.length).toBe(1);
    expect(play.pending()).toBe(1);
    resolvers[0]({ run: open(2), replayed: false, solvedByThisMove: false, wrong: false });
    await a;
    expect(play.pending()).toBe(0);
  });

  it('sends ONE request when the same answer is submitted twice before the first returns', async () => {
    const { fake, play } = await setup();
    const resolvers: ((r: Reply) => void)[] = [];
    fake.moveResult = () => new Promise((resolve) => resolvers.push(resolve));
    const a = play.move(answer);
    const b = play.move(answer); // Enter held, or a double tap before `busy` reaches the board
    expect(fake.moveKeys.length).toBe(1);
    resolvers.forEach((resolve) =>
      resolve({ run: open(2), replayed: false, solvedByThisMove: false, wrong: false }),
    );
    await Promise.all([a, b]);
  });
});

async function setupFirstRead(): Promise<{ fake: FakePuzzlesClient; play: PuzzlePlay }> {
  const fake = new FakePuzzlesClient();
  const play = new PuzzlePlay(
    fake,
    () => 'camp-1',
    async () => undefined,
    () => 'key',
    () => () => undefined,
  );
  fake.playerRunResult = playerRun(riddlePuzzle('p1', 'A porta pergunta'));
  return { fake, play };
}

describe('PuzzlePlay: the first read of the run fails', () => {
  it('control: a good read shows the run', async () => {
    const { play } = await setupFirstRead();
    await play.open('p1');
    expect(play.run()).not.toBeNull();
  });

  it('control: NotFound is told to the player (gone)', async () => {
    const { fake, play } = await setupFirstRead();
    fake.failWith = new ConnectError('x', Code.NotFound);
    await play.open('p1');
    expect(play.gone()).toBe(true);
  });

  it.each([Code.Unavailable, Code.PermissionDenied, Code.FailedPrecondition])(
    'says why nothing opened (not an endless spinner) on code %s',
    async (code) => {
      const { fake, play } = await setupFirstRead();
      fake.failWith = new ConnectError('x', code);
      await play.open('p1');
      expect(play.run()).toBeNull();
      expect(play.gone()).toBe(false);
      expect(play.loadError()).not.toBe('');
    },
  );
});
