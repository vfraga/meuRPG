import { MapLayer } from '../../../gen/meurpg/maps/v1/maps_pb';
import type { Square } from '../combat/combat-grid';
import type { DoorKind } from './layers';

/** One write of a door stroke: the layer, the value and the square. A door that is put where there was a wall clears the wall first. */
export interface DoorWrite {
  readonly layer: MapLayer;
  readonly value: number;
}

/** What a tap of the door tool does on one square: the writes, in order (empty when nothing changes), or why it cannot. */
export type DoorPlan =
  | { readonly ok: true; readonly writes: readonly DoorWrite[]; readonly kind: DoorKind | 0 }
  | { readonly ok: false };

/** How the plan reads the layers as they are now (the master's copy). */
export type LayerReader = (layer: MapLayer, col: number, row: number) => number;

/** The words for a tap that cannot put a door (the editor shows them in the "Porta" panel). */
export const NO_GAP_TEXT = 'Aqui não dá: uma porta precisa de chão dos dois lados.';

/**
 * What one tap of the "Porta" tool does on a square (E10-05 7, RN-26). The server stores a door in its own layer and treats a door
 * painted on a wall as a wall, so the tool does the whole job:
 *
 * - **Tirar a porta** (`kind` 0) closes the gap again: the wall comes back first, then the door goes.
 * - **Tocar numa porta** troca o tipo.
 * - **Tocar num quadrado de parede com chão dos dois lados** (to the left and right, or above and below) abre o vão e põe a porta (a porta primeiro, depois a parede some).
 * - **Tocar num vão** (floor with a wall on each side) puts the door in it.
 * - Anything else cannot take a door.
 *
 * Pure: it reads the layers and says what to write; `EditorPainting` writes it.
 */
export function planDoor(
  read: LayerReader,
  columns: number,
  rows: number,
  col: number,
  row: number,
  kind: DoorKind | 0,
): DoorPlan {
  if (col < 0 || row < 0 || col >= columns || row >= rows) {
    return { ok: false };
  }
  const door = read(MapLayer.DOORS, col, row);
  const wall = read(MapLayer.WALL, col, row) === 1;
  if (kind === 0) {
    // The wall first, then the door goes: the square is never a gap in between.
    return {
      ok: true,
      kind,
      writes:
        door === 0
          ? []
          : [
              ...(wall ? [] : [{ layer: MapLayer.WALL, value: 1 }]),
              { layer: MapLayer.DOORS, value: 0 },
            ],
    };
  }
  if (door !== 0) {
    return {
      ok: true,
      kind,
      writes: door === kind ? [] : [{ layer: MapLayer.DOORS, value: kind }],
    };
  }
  const open = (c: number, r: number) =>
    c >= 0 && r >= 0 && c < columns && r < rows && read(MapLayer.WALL, c, r) === 0;
  const closed = (c: number, r: number) =>
    c >= 0 && r >= 0 && c < columns && r < rows && read(MapLayer.WALL, c, r) === 1;
  if (wall) {
    if ((open(col - 1, row) && open(col + 1, row)) || (open(col, row - 1) && open(col, row + 1))) {
      return {
        ok: true,
        kind,
        // The door first, then the wall goes (a door on a wall still reads as a wall): a player never sees a gap, and the fog never
        // remembers what lies behind a closed or secret door. If the second batch is refused the editor reads the layers again
        // (`PaintSession.syncAfterRefusal`), and what is left is a door over a wall, never an open gap.
        writes: [
          { layer: MapLayer.DOORS, value: kind },
          { layer: MapLayer.WALL, value: 0 },
        ],
      };
    }
    return { ok: false };
  }
  if (
    (closed(col - 1, row) && closed(col + 1, row)) ||
    (closed(col, row - 1) && closed(col, row + 1))
  ) {
    return { ok: true, kind, writes: [{ layer: MapLayer.DOORS, value: kind }] };
  }
  return { ok: false };
}

/**
 * `planDoor` for a map whose drawing squares are worth `factor` x `factor` rules' squares: the server paints and erases a door over the
 * whole block of the tapped square (a door is a whole square of the drawing), so the tap is judged on blocks (a block is a wall when any
 * of its squares is, a door when its first square is) and every write goes to all the squares of the block. `squares` lists them.
 */
export function planDoorBlock(
  read: LayerReader,
  columns: number,
  rows: number,
  factor: number,
  col: number,
  row: number,
  kind: DoorKind | 0,
): { readonly plan: DoorPlan; readonly squares: readonly Square[] } {
  const f = Math.max(1, Math.floor(factor));
  const bc = Math.floor(col / f) * f;
  const br = Math.floor(row / f) * f;
  const squares: Square[] = [];
  for (let r = br; r < Math.min(br + f, rows); r++) {
    for (let c = bc; c < Math.min(bc + f, columns); c++) {
      squares.push({ col: c, row: r });
    }
  }
  if (f === 1) {
    return { plan: planDoor(read, columns, rows, col, row, kind), squares };
  }
  const blockRead: LayerReader = (layer, c, r) => {
    const cells = [];
    for (let rr = r * f; rr < Math.min(r * f + f, rows); rr++) {
      for (let cc = c * f; cc < Math.min(c * f + f, columns); cc++) {
        cells.push(read(layer, cc, rr));
      }
    }
    return layer === MapLayer.DOORS ? (cells.find((v) => v !== 0) ?? 0) : Math.max(0, ...cells);
  };
  return {
    plan: planDoor(blockRead, Math.ceil(columns / f), Math.ceil(rows / f), bc / f, br / f, kind),
    squares,
  };
}

/**
 * The squares of the rules' grid that make the drawing's square holding `(col, row)`: a door is a whole drawing square, so
 * revealing one clears the wall under all of them (one square on a map that was never calibrated).
 */
export function doorBlock(
  columns: number,
  rows: number,
  factor: number,
  col: number,
  row: number,
): readonly Square[] {
  const f = Math.max(1, Math.floor(factor));
  const bc = Math.floor(col / f) * f;
  const br = Math.floor(row / f) * f;
  const squares: Square[] = [];
  for (let r = br; r < Math.min(br + f, rows); r++) {
    for (let c = bc; c < Math.min(bc + f, columns); c++) {
      squares.push({ col: c, row: r });
    }
  }
  return squares;
}

/** The squares of the door's block that have a wall under them. */
export function wallUnderDoor(
  walls: readonly Square[],
  columns: number,
  rows: number,
  factor: number,
  door: Square,
): readonly Square[] {
  return doorBlock(columns, rows, factor, door.col, door.row).filter((s) =>
    walls.some((w) => w.col === s.col && w.row === s.row),
  );
}

/** Whether a door of this kind blocks the sight of a creature standing in its square (RN-26: closed, locked and secret). */
export function hidesWhoStands(kind: DoorKind | 0): boolean {
  return kind === 2 || kind === 3 || kind === 5;
}
