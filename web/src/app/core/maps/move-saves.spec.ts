import { MapPosition, MoveSaves } from './move-saves';

/** A save the test finishes by hand, to put moves in flight at once. */
function deferred(): {
  promise: Promise<void>;
  resolve: () => void;
  reject: (err: unknown) => void;
} {
  let resolve!: () => void;
  let reject!: (err: unknown) => void;
  const promise = new Promise<void>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function setup() {
  const saves = new MoveSaves();
  const sent: string[] = [];
  const pending: ReturnType<typeof deferred>[] = [];
  const failures: string[] = [];
  const handlers = {
    save: (to: MapPosition) => {
      sent.push(`${to.xBp},${to.yBp}`);
      const d = deferred();
      pending.push(d);
      return d.promise;
    },
    failed: (saved: MapPosition, err: unknown) => failures.push(`${saved.xBp},${saved.yBp} ${err}`),
  };
  return { saves, sent, pending, failures, handlers };
}

const at = (xBp: number): MapPosition => ({ xBp, yBp: 5400 });

describe('MoveSaves', () => {
  it('sends a second move of the same item only after the first is saved', async () => {
    const { saves, sent, pending, handlers } = setup();
    const first = saves.move('map-1/t1', at(5200), at(5700), handlers);
    await saves.move('map-1/t1', at(5700), at(6200), handlers);
    expect(sent).toEqual(['5700,5400']);

    pending[0].resolve();
    await vi.waitFor(() => expect(sent).toEqual(['5700,5400', '6200,5400']));
    pending[1].resolve();
    await first;
  });

  it('skips the moves in between and saves only the latest', async () => {
    const { saves, sent, pending, handlers } = setup();
    const first = saves.move('map-1/t1', at(5200), at(5300), handlers);
    await saves.move('map-1/t1', at(5300), at(5400), handlers);
    await saves.move('map-1/t1', at(5400), at(5500), handlers);

    pending[0].resolve();
    await vi.waitFor(() => expect(pending).toHaveLength(2));
    pending[1].resolve();
    await first;
    expect(sent).toEqual(['5300,5400', '5500,5400']);
  });

  it('saves different items at the same time', () => {
    const { saves, sent, handlers } = setup();
    void saves.move('map-1/t1', at(5200), at(5700), handlers);
    void saves.move('map-1/t2', at(1000), at(1500), handlers);
    expect(sent).toEqual(['5700,5400', '1500,5400']);
  });

  it('on a failure, drops the waiting move and goes back to the last saved position', async () => {
    const { saves, sent, pending, failures, handlers } = setup();
    const first = saves.move('map-1/t1', at(5200), at(5700), handlers);
    await saves.move('map-1/t1', at(5700), at(6200), handlers);
    pending[0].reject('no');
    await first;
    expect(sent).toEqual(['5700,5400']);
    expect(failures).toEqual(['5200,5400 no']);
  });

  it('goes back to the move the server saved when a later one fails', async () => {
    const { saves, pending, failures, handlers } = setup();
    const first = saves.move('map-1/t1', at(5200), at(5700), handlers);
    await saves.move('map-1/t1', at(5700), at(6200), handlers);
    pending[0].resolve();
    await vi.waitFor(() => expect(pending).toHaveLength(2));
    pending[1].reject('no');
    await first;
    expect(failures).toEqual(['5700,5400 no']);
  });

  it('starts fresh after the saves of an item end', async () => {
    const { saves, sent, pending, handlers } = setup();
    const first = saves.move('map-1/t1', at(5200), at(5700), handlers);
    pending[0].resolve();
    await first;
    void saves.move('map-1/t1', at(5700), at(6200), handlers);
    expect(sent).toEqual(['5700,5400', '6200,5400']);
  });

  it('says an item has a save in flight until its saves end', async () => {
    const { saves, pending, handlers } = setup();
    expect(saves.isPending('map-1/t1')).toBe(false);
    const first = saves.move('map-1/t1', at(5200), at(5700), handlers);
    expect(saves.isPending('map-1/t1')).toBe(true);
    expect(saves.isPending('map-1/t2')).toBe(false);
    pending[0].resolve();
    await first;
    expect(saves.isPending('map-1/t1')).toBe(false);
  });
});
