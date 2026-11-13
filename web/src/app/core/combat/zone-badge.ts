import type { Square } from './combat-grid';

/** The badge's drawn size and the inset of its diamond inside that box, in px: the one place the layer's and the legend's CSS read them from. */
export const ZONE_BADGE_PX = 24;
export const ZONE_BADGE_INSET_PX = 4;
export const ZONE_BADGE_BORDER_PX = 1.5;
/** The size of a square, in px, assumed until the map has been measured (the smallest the map draws). */
export const FALLBACK_CELL_PX = 16;

/** A point of the grid, in squares from the map's top left corner. */
export interface BadgeSpot {
  readonly col: number;
  readonly row: number;
}

const EPS = 1e-9;

/** The centre of a badge `size` squares wide, moved the least that puts its whole box inside a `columns` x `rows` map. */
export function clampSpot(p: BadgeSpot, size: number, columns: number, rows: number): BadgeSpot {
  const h = size / 2;
  return {
    col: Math.min(Math.max(p.col, h), Math.max(columns - h, h)),
    row: Math.min(Math.max(p.row, h), Math.max(rows - h, h)),
  };
}

/** Places along one edge of the footprint, one badge apart, the corner first. */
function along(from: number, to: number, size: number): number[] {
  const out: number[] = [];
  for (let x = from; x <= to + EPS; x += size) {
    out.push(x);
  }
  return out;
}

/**
 * The centre of each zone's letter badge, in squares. A badge `size` squares wide sits in a corner of its footprint's box (inside
 * the corner square only when a square is at least as wide as the badge; a smaller square leaves the badge over its neighbours):
 * the top left first, then the bottom right, top right and bottom left, then along the edges, the first place no other badge is
 * within `size` of. Every centre keeps the whole badge inside the map. With no such place (zones too small and too close) the
 * place farthest from the badges already laid is taken, so badges are apart where there is room.
 */
export function badgeSpots(
  footprints: readonly (readonly Square[])[],
  columns: number,
  rows: number,
  size: number,
): BadgeSpot[] {
  const h = size / 2;
  const taken: BadgeSpot[] = [];
  return footprints.map((squares) => {
    const cols = squares.map((s) => s.col);
    const rws = squares.map((s) => s.row);
    const [c0, c1] = [Math.min(...cols) + h, Math.max(...cols) + 1 - h];
    const [r0, r1] = [Math.min(...rws) + h, Math.max(...rws) + 1 - h];
    const candidates = [
      { col: c0, row: r0 },
      { col: c1, row: r1 },
      { col: c1, row: r0 },
      { col: c0, row: r1 },
      ...along(c0 + size, c1, size).flatMap((col) => [
        { col, row: r0 },
        { col, row: r1 },
      ]),
      ...along(r0 + size, r1, size).flatMap((row) => [
        { col: c0, row },
        { col: c1, row },
      ]),
    ].map((p) => clampSpot(p, size, columns, rows));
    const apart = (p: BadgeSpot) =>
      taken.every((t) => Math.max(Math.abs(t.col - p.col), Math.abs(t.row - p.row)) >= size - EPS);
    const gap = (p: BadgeSpot) =>
      Math.min(...taken.map((t) => Math.max(Math.abs(t.col - p.col), Math.abs(t.row - p.row))));
    const farthest = candidates.reduce((a, p) => (gap(p) > gap(a) ? p : a));
    const spot = candidates.find(apart) ?? farthest;
    taken.push(spot);
    return spot;
  });
}
