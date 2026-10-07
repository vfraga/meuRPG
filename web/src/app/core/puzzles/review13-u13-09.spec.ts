// Finding U13-09 (review/unit-13-web-live-rest.md): PuzzlePlay.refresh() swallows every error but NotFound, so a failed first read leaves the page on "Abrindo o quebra-cabeça..." forever.
import { Code, ConnectError } from '@connectrpc/connect';
import { PuzzlePlay } from './puzzle-play';
import { FakePuzzlesClient, playerRun, riddlePuzzle } from './puzzles-testing';

async function setup(): Promise<{ fake: FakePuzzlesClient; play: PuzzlePlay }> {
  const fake = new FakePuzzlesClient();
  const play = new PuzzlePlay(
    fake,
    () => 'camp-1',
    async () => undefined,
    () => 'key',
    () => '',
    () => () => undefined,
  );
  fake.playerRunResult = playerRun(riddlePuzzle('p1', 'A porta pergunta'));
  return { fake, play };
}

describe('Review13 U13-09: the first read of the run fails', () => {
  it('control: a good read shows the run', async () => {
    const { play } = await setup();
    await play.open('p1');
    expect(play.run()).not.toBeNull();
  });

  it('control: NotFound is told to the player (gone)', async () => {
    const { fake, play } = await setup();
    fake.failWith = new ConnectError('x', Code.NotFound);
    await play.open('p1');
    expect(play.gone()).toBe(true);
  });

  it.each([Code.Unavailable, Code.PermissionDenied, Code.FailedPrecondition])(
    'tells the player something (not an endless spinner) on code %s',
    async (code) => {
      const { fake, play } = await setup();
      fake.failWith = new ConnectError('x', code);
      await play.open('p1');
      // The template shows the spinner when run is null and gone is false; message is not shown there either.
      const spinner = play.run() === null && !play.gone();
      expect(spinner && play.message() === '').toBe(false);
    },
  );
});
