import { create } from '@bufbuild/protobuf';

import { GetTrapNoticersResponseSchema } from '../../../gen/meurpg/maps/v1/maps_pb';
import { TrapBoard } from './trap-board';

const res = (dc: number) => create(GetTrapNoticersResponseSchema, { noticeDc: dc });

function setup(read: (n: number) => Promise<unknown>) {
  let n = 0;
  const maps = { getTrapNoticers: () => read(++n) };
  const traps = {
    activity: async () => ({ activity: [] }),
    damages: async () => ({ damages: [] }),
  };
  return new TrapBoard(
    traps as never,
    maps as never,
    () => 'c',
    () => 'm',
    () => true,
  );
}

describe('TrapBoard "Quem notaria"', () => {
  it('says the read failed, and a retry clears it', async () => {
    let fail = true;
    const board = setup(async () => {
      if (fail) {
        throw new Error('down');
      }
      return res(15);
    });
    await board.watchNoticers('p1');
    expect(board.noticersFailed().has('p1')).toBe(true);
    expect(board.noticers().has('p1')).toBe(false);
    fail = false;
    await board.retryNoticers('p1');
    expect(board.noticersFailed().has('p1')).toBe(false);
    expect(board.noticers().get('p1')?.noticeDc).toBe(15);
  });

  it('never lets a stale answer overwrite a newer one', async () => {
    const slow: ((v: unknown) => void)[] = [];
    const board = setup((n) =>
      n === 1 ? new Promise((r) => slow.push(r)) : Promise.resolve(res(20)),
    );
    const first = board.watchNoticers('p1');
    await board.retryNoticers('p1'); // the newer read answers 20
    slow[0](res(5)); // the older one arrives late
    await first;
    expect(board.noticers().get('p1')?.noticeDc).toBe(20);
  });

  it('reads who would notice again when a token moves, once more for a burst of moves', async () => {
    const reads: ((v: unknown) => void)[] = [];
    const board = setup((n) =>
      n === 1 ? Promise.resolve(res(10)) : new Promise((r) => reads.push(r)),
    );
    await board.watchNoticers('p1');
    expect(board.noticers().get('p1')?.noticeDc).toBe(10);
    const first = board.tokensMoved();
    // Three more moves while the read is out: one read follows, not three.
    void board.tokensMoved();
    void board.tokensMoved();
    void board.tokensMoved();
    reads[0](res(11));
    await vi.waitFor(() => expect(reads).toHaveLength(2));
    reads[1](res(12));
    await first;
    expect(board.noticers().get('p1')?.noticeDc).toBe(12);
    expect(reads).toHaveLength(2);
  });

  it('reads nothing for a move when no trap card is open', async () => {
    const reads = vi.fn(async () => res(1));
    const board = setup(reads);
    await board.tokensMoved();
    expect(reads).not.toHaveBeenCalled();
  });
});
