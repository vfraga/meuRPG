import { overlapPaths, withoutShared } from './zone-overlap';

const sq = (col: number, row: number) => ({ col, row });

describe('where two zones lie over each other', () => {
  it('has nothing without a shared square', () => {
    expect(overlapPaths([[sq(0, 0)], [sq(1, 0)]])).toEqual([]);
  });

  it('splits a shared square on its diagonal: the older zone upper left, the newer lower right', () => {
    const [o] = overlapPaths([
      [sq(0, 0), sq(1, 1)],
      [sq(1, 1), sq(2, 2)],
    ]);
    expect(o.older).toBe(0);
    expect(o.newer).toBe(1);
    expect(o.upper).toBe('M1 1H2L1 2Z');
    expect(o.lower).toBe('M2 1V2H1Z');
  });

  it('joins every shared square of the same two zones in one entry', () => {
    const list = overlapPaths([
      [sq(0, 0), sq(1, 0)],
      [sq(0, 0), sq(1, 0)],
    ]);
    expect(list.length).toBe(1);
    expect(list[0].upper).toBe('M0 0H1L0 1ZM1 0H2L1 1Z');
  });

  it('takes the two newest zones of a square that three cover', () => {
    const list = overlapPaths([[sq(0, 0)], [sq(0, 0)], [sq(0, 0)]]);
    expect(list.map((o) => [o.older, o.newer])).toEqual([[1, 2]]);
  });
});

describe('the squares a zone keeps for itself', () => {
  it('leaves out every square another zone covers too', () => {
    const left = withoutShared([[sq(0, 0), sq(1, 0)], [sq(1, 0), sq(2, 0)], [sq(5, 5)]]);
    expect(left).toEqual([[sq(0, 0)], [sq(2, 0)], [sq(5, 5)]]);
  });
});
