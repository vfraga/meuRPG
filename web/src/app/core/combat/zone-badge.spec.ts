import { badgeSpots, clampSpot } from './zone-badge';

const sq = (col: number, row: number) => ({ col, row });

function block(col: number, row: number, w: number, h: number) {
  const out = [];
  for (let c = col; c < col + w; c++) {
    for (let r = row; r < row + h; r++) {
      out.push(sq(c, r));
    }
  }
  return out;
}

/** The badge's box must lie inside the map: its centre is half its size away from every edge. */
function inside(p: { col: number; row: number }, s: number, cols: number, rows: number) {
  const eps = 1e-9;
  expect(p.col - s / 2).toBeGreaterThanOrEqual(-eps);
  expect(p.row - s / 2).toBeGreaterThanOrEqual(-eps);
  expect(p.col + s / 2).toBeLessThanOrEqual(cols + eps);
  expect(p.row + s / 2).toBeLessThanOrEqual(rows + eps);
}

describe('the clamp of a badge', () => {
  it('keeps the whole box inside the map', () => {
    expect(clampSpot({ col: 0, row: 0 }, 1.5, 24, 16)).toEqual({ col: 0.75, row: 0.75 });
    expect(clampSpot({ col: 24, row: 16 }, 1.5, 24, 16)).toEqual({ col: 23.25, row: 15.25 });
    expect(clampSpot({ col: 5, row: 5 }, 1.5, 24, 16)).toEqual({ col: 5, row: 5 });
  });
});

describe('the corner badge of a zone', () => {
  it('sits inside the top left corner square of the footprint', () => {
    expect(badgeSpots([block(4, 3, 4, 4)], 24, 16, 0.5)).toEqual([{ col: 4.25, row: 3.25 }]);
  });

  it.each([20, 24])('stays inside a map of %i columns for a zone at every corner', (cols) => {
    const rows = 16;
    const zones = [
      block(0, 0, 3, 3),
      block(cols - 3, 0, 3, 3),
      block(0, rows - 3, 3, 3),
      block(cols - 3, rows - 3, 3, 3),
      [sq(0, 0)],
      [sq(cols - 1, rows - 1)],
    ];
    for (const size of [0.5, 1.5]) {
      for (const z of zones) {
        inside(badgeSpots([z], cols, rows, size)[0], size, cols, rows);
      }
    }
  });

  it('moves a later zone to another corner when two share the first one', () => {
    const spots = badgeSpots([block(4, 3, 4, 4), block(4, 3, 4, 4)], 24, 16, 1.5);
    expect(spots[0]).toEqual({ col: 4.75, row: 3.75 });
    expect(spots[1]).toEqual({ col: 7.25, row: 6.25 });
  });

  it.each([0.5, 1.5])(
    'never puts two badges closer than their size (%f squares), walking the edge when the corners are taken',
    (size) => {
      const zone = block(4, 3, 8, 8);
      const spots = badgeSpots(
        Array.from({ length: 7 }, () => zone),
        24,
        16,
        size,
      );
      for (let i = 0; i < spots.length; i++) {
        inside(spots[i], size, 24, 16);
        for (let j = i + 1; j < spots.length; j++) {
          const d = Math.max(
            Math.abs(spots[i].col - spots[j].col),
            Math.abs(spots[i].row - spots[j].row),
          );
          expect(d).toBeGreaterThanOrEqual(size - 1e-9);
        }
      }
    },
  );
});

describe('badges of zones that touch', () => {
  it('keeps the badges of two adjacent one-square zones apart as far as the room allows, inside the map', () => {
    const [a, b] = badgeSpots([[sq(0, 0)], [sq(1, 0)]], 24, 16, 1.5);
    inside(a, 1.5, 24, 16);
    inside(b, 1.5, 24, 16);
    const d = Math.max(Math.abs(a.col - b.col), Math.abs(a.row - b.row));
    expect(d).toBeGreaterThanOrEqual(1);
  });
});
