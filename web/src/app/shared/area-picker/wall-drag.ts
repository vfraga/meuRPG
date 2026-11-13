import type { AreaPick } from '../../core/combat/area-flow';
import type { Square } from '../../core/combat/combat-grid';
import { type Direction, sameSquare, toward } from '../../core/combat/spell-area';

/** What the drag of a wall needs from the map. */
export interface WallDragHost {
  readOnly(): boolean;
  /** The square under the pointer. */
  squareAt(event: PointerEvent): Square;
  /** The mouse passes over a square (no button down): the start follows it, as a point does. */
  hover(pick: AreaPick | null): void;
  /** A press and release on one square: a tap on it. */
  tap(pick: AreaPick): void;
  /** A press on a square and a release on another: the start and the way (one of eight) toward the release. */
  aimed(square: Square, direction: Direction): void;
}

/**
 * A straight wall's gesture on the map: press on the first square, drag toward where it runs, release. The press is the start
 * and the nearest of the eight directions toward the release is the way; a release on the same square is a plain tap. A finger
 * is not lifted above the square, because the first square is where it lands. The direction buttons do the same for a keyboard.
 */
export class WallDrag {
  private held: { id: number; square: Square } | null = null;

  constructor(private readonly host: WallDragHost) {}

  down(event: PointerEvent): void {
    if (this.host.readOnly() || (event.pointerType === 'mouse' && event.button > 0)) {
      return;
    }
    this.held = { id: event.pointerId, square: this.host.squareAt(event) };
    (event.currentTarget as HTMLElement).setPointerCapture?.(event.pointerId);
  }

  move(event: PointerEvent): void {
    if (this.host.readOnly() || this.held || event.pointerType !== 'mouse') {
      return;
    }
    this.host.hover({ kind: 'point', square: this.host.squareAt(event) });
  }

  up(event: PointerEvent): void {
    const held = this.held;
    if (this.host.readOnly() || !held || held.id !== event.pointerId) {
      return;
    }
    this.held = null;
    const end = this.host.squareAt(event);
    const direction = sameSquare(held.square, end) ? null : toward(held.square, end);
    if (direction) {
      this.host.aimed(held.square, direction);
    } else {
      this.host.tap({ kind: 'point', square: held.square });
    }
  }

  cancel(): void {
    this.held = null;
    this.host.hover(null);
  }
}
