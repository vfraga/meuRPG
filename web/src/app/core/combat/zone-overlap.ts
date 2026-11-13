import type { Square } from './combat-grid';

/** The squares two zones share, as the two triangles each square is split in: the older zone's upper left, the newer one's lower right. */
export interface OverlapPaths {
  /** Index of the older of the two newest zones that cover the squares. */
  readonly older: number;
  readonly newer: number;
  /** The upper left triangles of the squares, as one SVG path. */
  readonly upper: string;
  /** The lower right triangles. */
  readonly lower: string;
}

/**
 * Where zones lie over each other: each square covered by two or more zones is cut on its diagonal, the older zone's pattern in the
 * upper left triangle and the newer one's in the lower right (a square three zones cover takes its two newest). One entry per pair.
 * `footprints` are in the order the zones were cast, oldest first.
 */
export function overlapPaths(footprints: readonly (readonly Square[])[]): OverlapPaths[] {
  const cover = new Map<string, number[]>();
  footprints.forEach((squares, i) => {
    for (const s of squares) {
      const key = `${s.col},${s.row}`;
      cover.set(key, [...(cover.get(key) ?? []), i]);
    }
  });
  const pairs = new Map<string, { older: number; newer: number; upper: string; lower: string }>();
  for (const [key, who] of cover) {
    if (who.length < 2) {
      continue;
    }
    const older = who[who.length - 2];
    const newer = who[who.length - 1];
    if (older === newer) {
      continue;
    }
    const [c, r] = key.split(',').map(Number);
    const pair = pairs.get(`${older}-${newer}`) ?? { older, newer, upper: '', lower: '' };
    pair.upper += `M${c} ${r}H${c + 1}L${c} ${r + 1}Z`;
    pair.lower += `M${c + 1} ${r}V${r + 1}H${c}Z`;
    pairs.set(`${older}-${newer}`, pair);
  }
  return [...pairs.values()];
}

/** Each footprint without the squares another zone covers too: those squares are painted by the overlap triangles alone. */
export function withoutShared(footprints: readonly (readonly Square[])[]): Square[][] {
  const count = new Map<string, number>();
  const key = (s: Square) => `${s.col},${s.row}`;
  for (const f of footprints) {
    for (const s of f) {
      count.set(key(s), (count.get(key(s)) ?? 0) + 1);
    }
  }
  return footprints.map((f) => f.filter((s) => count.get(key(s)) === 1));
}
