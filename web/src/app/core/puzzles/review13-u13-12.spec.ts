// Finding U13-12 (review/unit-13-web-live-rest.md): the wrong-answer verdict is read from the run's lastMove, so a replayed answer or an unknown own name misjudges it.
import { Code, ConnectError } from '@connectrpc/connect';

import type { PuzzleRun } from '../../../gen/meurpg/play/v1/puzzles_pb';
import { PuzzlePlay } from './puzzle-play';
import { FakePuzzlesClient, at, playerRun, riddlePuzzle } from './puzzles-testing';

const riddle = riddlePuzzle('p1', 'A porta da Cripta pergunta');
const answer = { kind: { case: 'riddle' as const, value: { answer: 'escuridão' } } };
const riddleRun = (revision: number, partial: Partial<PuzzleRun> = {}): PuzzleRun => ({
  ...playerRun(riddle),
  revision,
  ...partial,
});
const wrongBy = (characterName: string, secondsAgo: number) =>
  ({ characterName, wrong: true, changed: [], at: at(secondsAgo) }) as never;

function setup(own: string): { fake: FakePuzzlesClient; play: PuzzlePlay } {
  const fake = new FakePuzzlesClient();
  const play = new PuzzlePlay(
    fake,
    () => 'camp-1',
    async () => undefined,
    () => 'key-1',
    () => own,
    () => () => undefined,
  );
  return { fake, play };
}

describe('Review13 U13-12: wrong-answer verdict inferred from lastMove', () => {
  it('control: the own wrong answer, answered at once, is told wrong', async () => {
    const { fake, play } = setup('Toren');
    fake.playerRunResult = riddleRun(1);
    await play.open('p1');
    fake.moveResult = () => ({
      run: riddleRun(2, { lastMove: wrongBy('Toren', 5) }),
      replayed: false,
      solvedByThisMove: false,
    });
    expect((await play.move(answer)).wrong).toBe(true);
  });

  it('tells the player their answer was wrong when a retry is replayed after another player moved', async () => {
    const { fake, play } = setup('Toren');
    fake.playerRunResult = riddleRun(1);
    await play.open('p1');
    fake.moveResult = (n) => {
      if (n < 2) {
        // The first answer is lost; the server had already judged Toren's answer wrong.
        throw new ConnectError('down', Code.Unavailable);
      }
      // Replay: the run as it is now, Lia's wrong move made since is the last one.
      return {
        run: riddleRun(3, { lastMove: wrongBy('Lia', 1) }),
        replayed: true,
        solvedByThisMove: false,
      };
    };
    const verdict = await play.move(answer);
    expect(fake.moveKeys).toEqual(['key-1', 'key-1']);
    expect(verdict.wrong).toBe(true);
  });

  it("does not take another player's wrong answer for the player's own when the own name is not loaded", async () => {
    const { fake, play } = setup('');
    fake.playerRunResult = riddleRun(1);
    await play.open('p1');
    fake.moveResult = () => ({
      // Toren's answer was right but did not solve it; Lia's wrong move landed after.
      run: riddleRun(2, { lastMove: wrongBy('Lia', 1) }),
      replayed: false,
      solvedByThisMove: false,
    });
    expect((await play.move(answer)).wrong).toBe(false);
  });
});
