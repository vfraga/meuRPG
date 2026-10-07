// Finding U13-08 (review/unit-13-web-live-rest.md): PuzzlePlay.send() has no in-flight guard for riddle/cipher answers, so a double submit inside one frame spends two attempts.
import type { PuzzleRun } from '../../../gen/meurpg/play/v1/puzzles_pb';
import { PuzzlePlay } from './puzzle-play';
import { FakePuzzlesClient, playerRun, riddlePuzzle } from './puzzles-testing';

const riddle = riddlePuzzle('p1', 'A porta da Cripta pergunta');
const answer = { kind: { case: 'riddle' as const, value: { answer: 'escuridão' } } };
const open = (revision: number): PuzzleRun => ({ ...playerRun(riddle), revision });

type Reply = { run: PuzzleRun; replayed: boolean; solvedByThisMove: boolean };

async function setup(): Promise<{ fake: FakePuzzlesClient; play: PuzzlePlay }> {
  const fake = new FakePuzzlesClient();
  let key = 0;
  const play = new PuzzlePlay(
    fake,
    () => 'camp-1',
    async () => undefined,
    () => `key-${++key}`,
    () => '',
    () => () => undefined,
  );
  fake.playerRunResult = open(1);
  await play.open('p1');
  return { fake, play };
}

describe('Review13 U13-08: a riddle answer sent twice in one frame', () => {
  it('control: one answer makes one call and counts one in flight', async () => {
    const { fake, play } = await setup();
    const resolvers: ((r: Reply) => void)[] = [];
    fake.moveResult = () => new Promise((resolve) => resolvers.push(resolve));
    const a = play.move(answer);
    expect(fake.moveKeys.length).toBe(1);
    expect(play.pending()).toBe(1);
    resolvers[0]({ run: open(2), replayed: false, solvedByThisMove: false });
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
      resolve({ run: open(2), replayed: false, solvedByThisMove: false }),
    );
    await Promise.all([a, b]);
  });
});
