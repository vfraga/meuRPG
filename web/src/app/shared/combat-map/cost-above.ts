import type { Square } from '../../core/combat/combat-grid';

/** Whether the cost pill of the chosen square goes above it: below by default, but never over a door (it would hide the mark), and above at the bottom edge. */
export function costAbove(
  at: Square,
  rows: number,
  doors: readonly { col: number; row: number }[],
): boolean {
  const door = (row: number) => doors.some((d) => d.col === at.col && d.row === row);
  const belowFree = at.row + 2 < rows && !door(at.row + 1);
  const aboveFree = at.row > 0 && !door(at.row - 1);
  return !belowFree && (aboveFree || at.row + 2 >= rows);
}
