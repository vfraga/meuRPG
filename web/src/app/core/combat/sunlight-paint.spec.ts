import {
  PaintQueue,
  type SunChange,
  cursorNote,
  lineSquares,
  paintStatus,
  squaresCount,
  strokeChange,
  withStroke,
} from './sunlight-paint';

const sq = (col: number, row: number) => ({ col, row });

describe('sunlight paint words', () => {
  it('pluralises the squares', () => {
    expect(squaresCount(0)).toBe('0 quadrados');
    expect(squaresCount(1)).toBe('1 quadrado');
    expect(paintStatus(12)).toBe('Pintando luz do sol · 12 quadrados');
    expect(paintStatus(1)).toBe('Pintando luz do sol · 1 quadrado');
  });

  it('announces the cursor square counted from 1', () => {
    expect(cursorNote(sq(2, 4), true)).toBe('Coluna 3, linha 5: luz do sol');
    expect(cursorNote(sq(0, 0), false)).toBe('Coluna 1, linha 1: sem luz do sol');
  });
});

describe('lineSquares', () => {
  it('is the one square when the pointer did not leave it', () => {
    expect(lineSquares(sq(1, 1), sq(1, 1))).toEqual([sq(1, 1)]);
  });

  it('fills the gap of a fast drag, both ends included', () => {
    expect(lineSquares(sq(0, 0), sq(3, 0))).toEqual([sq(0, 0), sq(1, 0), sq(2, 0), sq(3, 0)]);
    expect(lineSquares(sq(2, 2), sq(0, 0))).toEqual([sq(2, 2), sq(1, 1), sq(0, 0)]);
    expect(lineSquares(sq(0, 3), sq(0, 1))).toEqual([sq(0, 3), sq(0, 2), sq(0, 1)]);
  });
});

describe('strokeChange', () => {
  it('adds only the squares that are not lit, once each', () => {
    const c = strokeChange([sq(1, 1)], 'paint', [sq(1, 1), sq(2, 1), sq(2, 1)]);
    expect(c).toEqual({ addSquares: [sq(2, 1)], removeSquares: [] });
  });

  it('removes only the squares that are lit', () => {
    const c = strokeChange([sq(1, 1)], 'erase', [sq(1, 1), sq(2, 1)]);
    expect(c).toEqual({ addSquares: [], removeSquares: [sq(1, 1)] });
  });

  it('previews the stroke over the lit squares', () => {
    expect(withStroke([sq(0, 0)], 'paint', [sq(1, 0)])).toEqual([sq(0, 0), sq(1, 0)]);
    expect(withStroke([sq(0, 0), sq(1, 0)], 'erase', [sq(0, 0)])).toEqual([sq(1, 0)]);
  });
});

describe('PaintQueue', () => {
  it('sends one call at a time and merges what comes meanwhile', async () => {
    const sent: SunChange[] = [];
    let release!: () => void;
    const first = new Promise<boolean>((r) => (release = () => r(true)));
    const send = vi.fn((c: SunChange) => {
      sent.push(c);
      return sent.length === 1 ? first : Promise.resolve(true);
    });
    const queue = new PaintQueue(send);
    const p = queue.push({ addSquares: [sq(0, 0)], removeSquares: [] });
    void queue.push({ addSquares: [sq(1, 0), sq(2, 0)], removeSquares: [] });
    void queue.push({ addSquares: [], removeSquares: [sq(2, 0), sq(5, 5)] });
    expect(sent).toHaveLength(1);
    release();
    await p;
    expect(sent).toHaveLength(2);
    expect(sent[1]).toEqual({ addSquares: [sq(1, 0)], removeSquares: [sq(2, 0), sq(5, 5)] });
  });
});
