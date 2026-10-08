import { describe, expect, it } from 'vitest';

import { MapLayer } from '../../../gen/meurpg/maps/v1/maps_pb';
import {
  NO_GAP_TEXT,
  doorBlock,
  hidesWhoStands,
  planDoor,
  planDoorBlock,
  wallUnderDoor,
} from './door-paint';

/** A 5 x 3 map: a wall row at the top and the bottom, floor in the middle; plus what each test paints. */
function reader(walls: readonly string[], doors: Readonly<Record<string, number>> = {}) {
  const wall = new Set(walls);
  return (layer: MapLayer, col: number, row: number) =>
    layer === MapLayer.WALL
      ? wall.has(`${col},${row}`)
        ? 1
        : 0
      : layer === MapLayer.DOORS
        ? (doors[`${col},${row}`] ?? 0)
        : 0;
}

// A wall line down the middle (column 2), floor on both sides.
const split = reader(['2,0', '2,1', '2,2']);

describe('planDoor (the "Porta" tool)', () => {
  it('opens the gap in a wall that has floor on both sides, and puts the door there', () => {
    expect(planDoor(split, 5, 3, 2, 1, 2)).toEqual({
      ok: true,
      kind: 2,
      // The safe order: the door first, then the wall goes, so no player sees a gap in between (RN-10).
      writes: [
        { layer: MapLayer.DOORS, value: 2 },
        { layer: MapLayer.WALL, value: 0 },
      ],
    });
  });

  it('refuses a wall without floor on both sides, with the words the panel shows', () => {
    // A wall two squares thick: the floor is two squares away on each side.
    const thick = reader(['2,0', '2,1', '2,2', '3,0', '3,1', '3,2']);
    expect(planDoor(thick, 6, 3, 2, 1, 2)).toEqual({ ok: false });
    // The solid rock of a corner.
    expect(planDoor(reader(['0,0', '1,0', '0,1', '1,1']), 5, 3, 1, 1, 2)).toEqual({ ok: false });
    expect(NO_GAP_TEXT).toBe('Aqui não dá: uma porta precisa de chão dos dois lados.');
  });

  it('refuses a plain floor square that is no gap', () => {
    expect(planDoor(split, 5, 3, 3, 1, 2)).toEqual({ ok: false });
  });

  it('puts a door in a gap that is already floor between two walls', () => {
    const gap = reader(['1,0', '1,2']);
    expect(planDoor(gap, 3, 3, 1, 1, 3)).toEqual({
      ok: true,
      kind: 3,
      writes: [{ layer: MapLayer.DOORS, value: 3 }],
    });
  });

  it('changes the kind of a door that is there, and does nothing for the same kind', () => {
    const closed = reader(['2,0', '2,2'], { '2,1': 2 });
    expect(planDoor(closed, 5, 3, 2, 1, 3)).toEqual({
      ok: true,
      kind: 3,
      writes: [{ layer: MapLayer.DOORS, value: 3 }],
    });
    expect(planDoor(closed, 5, 3, 2, 1, 2)).toEqual({ ok: true, kind: 2, writes: [] });
  });

  it('"Tirar a porta" takes the door away and closes the gap again as wall', () => {
    const closed = reader(['2,0', '2,2'], { '2,1': 2 });
    expect(planDoor(closed, 5, 3, 2, 1, 0)).toEqual({
      ok: true,
      kind: 0,
      // The wall comes back first, then the door goes.
      writes: [
        { layer: MapLayer.WALL, value: 1 },
        { layer: MapLayer.DOORS, value: 0 },
      ],
    });
    // Nothing to take away where there is no door.
    expect(planDoor(closed, 5, 3, 3, 1, 0)).toEqual({ ok: true, kind: 0, writes: [] });
  });

  it('refuses a square outside the grid', () => {
    expect(planDoor(split, 5, 3, 9, 1, 2)).toEqual({ ok: false });
  });
});

describe('hidesWhoStands', () => {
  it('is true for the doors that block sight: closed, locked and secret', () => {
    expect([0, 1, 2, 3, 4, 5].map((k) => hidesWhoStands(k as 0 | 1 | 2 | 3 | 4 | 5))).toEqual([
      false,
      false,
      true,
      true,
      false,
      true,
    ]);
  });
});

describe('planDoorBlock (the "Porta" tool on a calibrated map)', () => {
  // 4 x 2 rules' squares, factor 2: two blocks side by side; the wall of the left one is a single column of the block.
  const walls = new Set(['0,0']);
  const read = (layer: MapLayer, col: number, row: number) =>
    layer === MapLayer.WALL && walls.has(`${col},${row}`) ? 1 : 0;

  it('judges the tap on the block and lists every square of it', () => {
    const { plan, squares } = planDoorBlock(read, 4, 2, 2, 1, 1, 2);
    expect(plan.ok).toBe(false); // a wall block with a wall beside it and no floor on both sides
    expect(squares).toEqual([
      { col: 0, row: 0 },
      { col: 1, row: 0 },
      { col: 0, row: 1 },
      { col: 1, row: 1 },
    ]);
  });

  it('is planDoor when the factor is 1', () => {
    expect(planDoorBlock(read, 4, 2, 1, 1, 1, 2).plan).toEqual(planDoor(read, 4, 2, 1, 1, 2));
  });
});

describe('doorBlock and wallUnderDoor (revealing a secret door)', () => {
  it('is the one square on a map that was never calibrated', () => {
    expect(doorBlock(10, 10, 1, 4, 2)).toEqual([{ col: 4, row: 2 }]);
  });

  it('is the whole drawing square on a calibrated map, cut at the edge of the grid', () => {
    expect(doorBlock(10, 10, 2, 3, 2)).toEqual([
      { col: 2, row: 2 },
      { col: 3, row: 2 },
      { col: 2, row: 3 },
      { col: 3, row: 3 },
    ]);
    expect(doorBlock(5, 5, 3, 4, 4)).toEqual([
      { col: 3, row: 3 },
      { col: 4, row: 3 },
      { col: 3, row: 4 },
      { col: 4, row: 4 },
    ]);
  });

  it('lists only the squares of the block that have a wall', () => {
    const walls = [
      { col: 2, row: 2 },
      { col: 3, row: 3 },
      { col: 9, row: 9 },
    ];
    expect(wallUnderDoor(walls, 10, 10, 2, { col: 3, row: 2 })).toEqual([
      { col: 2, row: 2 },
      { col: 3, row: 3 },
    ]);
    expect(wallUnderDoor([], 10, 10, 2, { col: 3, row: 2 })).toEqual([]);
  });
});
