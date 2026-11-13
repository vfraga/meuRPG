import type { AreaPick } from '../../core/combat/area-flow';
import { type Square, arrowStep, squareAt, squareCenter } from '../../core/combat/combat-grid';
import { type AreaShape, areaSquares, rangeFt, rotate } from '../../core/combat/spell-area';
import { metersFixed } from '../../core/units';

/** Half a square: from a square's corner to its center, or to its side. */
const HALF = 0.5;

/** Sides of a square when a big map is zoomed in so that a finger hits one; the first step fits the map to the screen. */
const CELL_PX_NEAR = 22;
const CELL_PX_CLOSE = 30;
const CELL_PX_CLOSEST = 40;
export const ZOOM_CELLS = [null, CELL_PX_NEAR, CELL_PX_CLOSE, CELL_PX_CLOSEST] as const;
/** A map wider than this many squares gets the zoom: its squares are too small to tap at the screen's width. */
export const BIG_MAP_COLUMNS = 30;

/** The square kept on the map. */
export function clampSquare(s: Square, columns: number, rows: number): Square {
  return {
    col: Math.min(columns - 1, Math.max(0, s.col)),
    row: Math.min(rows - 1, Math.max(0, s.row)),
  };
}

/** The point the arrows move to: from the hover or the placed point, else from the caster (the corner if it is not on the map). */
export function movedPoint(
  from: AreaPick | null,
  caster: Square | null,
  step: { dc: number; dr: number },
  n: number,
  columns: number,
  rows: number,
): AreaPick {
  const base = from?.kind === 'point' ? from.square : (caster ?? { col: 0, row: 0 });
  return {
    kind: 'point',
    square: clampSquare(
      { col: base.col + step.dc * n, row: base.row + step.dr * n },
      columns,
      rows,
    ),
  };
}

/** The direction turned 45° (right and down clockwise, left and up the other way); east when none is chosen yet. */
export function turnedDirection(from: AreaPick | null, turn: number): AreaPick {
  const base = from?.kind === 'direction' ? from.direction : null;
  return { kind: 'direction', direction: base ? rotate(base, turn) : { dx: 1, dy: 0 } };
}

/** The dashed line from the caster to the point, with its length ("7,5 m"), in squares of the map. */
export function areaLine(
  caster: Square | null,
  origin: Square | null,
  columns: number,
  rows: number,
) {
  if (!caster || !origin) {
    return null;
  }
  const a = squareCenter(caster, columns, rows);
  const b = squareCenter(origin, columns, rows);
  return {
    x1: a.x,
    y1: a.y,
    x2: b.x,
    y2: b.y,
    label: metersFixed(rangeFt(caster, origin)),
    mx: (a.x + b.x) / 2,
    my: (a.y + b.y) / 2,
  };
}

/** The diamond of a shape out of the caster: on the side of the caster the area leaves from, in percent of the map. */
export function edgeMarkOf(
  pick: AreaPick | null,
  caster: Square | null,
  columns: number,
  rows: number,
) {
  if (pick?.kind !== 'direction' || !caster) {
    return null;
  }
  return {
    x: ((caster.col + HALF + pick.direction.dx * HALF) / columns) * 100,
    y: ((caster.row + HALF + pick.direction.dy * HALF) / rows) * 100,
  };
}

/** The square under a pointer, `lift` pixels above it (a finger covers what it chooses). */
export function squareUnder(
  event: PointerEvent,
  lift: number,
  columns: number,
  rows: number,
): Square {
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect();
  return squareAt(
    (event.clientX - rect.left) / Math.max(rect.width, 1),
    (event.clientY - lift - rect.top) / Math.max(rect.height, 1),
    columns,
    rows,
  );
}

/** Where the finger is, in percent of the map. */
export function fingerAt(event: PointerEvent): { x: number; y: number } {
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect();
  return {
    x: ((event.clientX - rect.left) / Math.max(rect.width, 1)) * 100,
    y: ((event.clientY - rect.top) / Math.max(rect.height, 1)) * 100,
  };
}

/** What a key does in the "Centrar em…" list: the active option's new place, `'choose'` or `'close'`; `null` for a key it leaves alone. */
export function listMove(
  key: string,
  active: number,
  n: number,
): number | 'choose' | 'close' | null {
  switch (key) {
    case 'ArrowDown':
      return (active + 1) % n;
    case 'ArrowUp':
      return (active - 1 + n) % n;
    case 'Home':
      return 0;
    case 'End':
      return n - 1;
    case 'Enter':
    case ' ':
      return 'choose';
    case 'Escape':
      return 'close';
    default:
      return null;
  }
}

/** The browser's outline of an area at a pick (before the server answers): a point's sphere or cylinder, or what a direction
 * cuts out from the caster. Nothing without a pick, or a direction without the caster on the map. */
export function outlineOf(
  pick: AreaPick | null,
  caster: Square | null,
  area: AreaShape,
  columns: number,
  rows: number,
): Square[] {
  if (!pick) {
    return [];
  }
  if (pick.kind === 'point') {
    return areaSquares(area, { origin: pick.square }, columns, rows);
  }
  return caster ? areaSquares(area, { caster, direction: pick.direction }, columns, rows) : [];
}

/** Shift and an arrow move five squares. */
const SHIFT_STEP = 5;

/** The pick an arrow key makes: a point moves a square (Shift: five), a direction turns 45°; `null` for any other key. */
export function arrowPick(
  event: Pick<KeyboardEvent, 'key' | 'shiftKey'>,
  from: AreaPick | null,
  caster: Square | null,
  isPoint: boolean,
  columns: number,
  rows: number,
): AreaPick | null {
  const step = arrowStep(event.key);
  if (!step) {
    return null;
  }
  return isPoint
    ? movedPoint(from, caster, step, event.shiftKey ? SHIFT_STEP : 1, columns, rows)
    : turnedDirection(from, step.dc + step.dr);
}

/** A point farther from the caster than the spell's range (0: no range). */
export function beyondReach(pick: AreaPick, caster: Square, range: number): boolean {
  return pick.kind === 'point' && range > 0 && rangeFt(caster, pick.square) > range;
}
