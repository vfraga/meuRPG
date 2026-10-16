import { trapArea } from './trap-area';

describe('trapArea', () => {
  it('is the point square for size 1', () => {
    // a 24 x 16 grid: (11, 7) is the middle of the square
    const a = trapArea(4792, 4688, 1, 24, 16)!;
    expect(a.left).toBeCloseTo((11 / 24) * 100);
    expect(a.top).toBeCloseTo((7 / 16) * 100);
    expect(a.width).toBeCloseTo(100 / 24);
  });

  it('puts the point square in the middle of the block, the first of two for an even side', () => {
    const a = trapArea(4792, 4688, 2, 24, 16)!;
    expect(a.left).toBeCloseTo((11 / 24) * 100); // the point square is the first of the two middle ones
    expect(a.width).toBeCloseTo((2 / 24) * 100);
    const b = trapArea(4792, 4688, 3, 24, 16)!;
    expect(b.left).toBeCloseTo((10 / 24) * 100);
    expect(b.width).toBeCloseTo((3 / 24) * 100);
  });

  it('is clipped to the grid and absent without one', () => {
    expect(trapArea(0, 0, 4, 24, 16)!.left).toBe(0);
    expect(trapArea(0, 0, 4, 0, 0)).toBeNull();
  });
});
