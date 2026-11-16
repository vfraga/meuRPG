import { signal } from '@angular/core';

import type { Combatant } from '../../../gen/meurpg/play/v1/combat_pb';
import type { Square } from '../../core/combat/combat-grid';

/** A token dropped on a square: the combatant's id (a pair's is its mount's) and where. */
export interface TokenDrop extends Square {
  readonly id: string;
}

/** What the drag needs from the map. */
export interface DragHost {
  readonly surface: () => HTMLElement;
  readonly shown: () => readonly Combatant[];
  readonly movable: (c: Combatant) => boolean;
  /** The token the keyboard moves next is the one last grabbed. */
  readonly grab: (id: string) => void;
  readonly squareUnder: (event: PointerEvent) => Square;
  readonly drop: (drop: TokenDrop) => void;
}

/**
 * Dragging a token with a pointer (E6-04): a press on a token the caller may move grabs it, each move says the square under the
 * pointer (`drag`, which the map draws the token on), and a release after a move drops it there. A cancel drops nothing.
 */
export class TokenDrag {
  readonly drag = signal<TokenDrop | null>(null);
  private dragging: { id: string; moved: boolean; pointer: number } | null = null;

  constructor(private readonly host: DragHost) {}

  down(event: PointerEvent): void {
    const id = (event.target as HTMLElement).closest<HTMLElement>('[data-id]')?.dataset['id'];
    const token = id ? this.host.shown().find((c) => c.id === id) : undefined;
    if (!token || !this.host.movable(token) || event.button > 0) {
      return;
    }
    this.host.grab(token.id);
    this.dragging = { id: token.id, moved: false, pointer: event.pointerId };
    this.host.surface().setPointerCapture?.(event.pointerId);
  }

  move(event: PointerEvent): void {
    const d = this.dragging;
    if (!d || d.pointer !== event.pointerId) {
      return;
    }
    d.moved = true;
    this.drag.set({ id: d.id, ...this.host.squareUnder(event) });
  }

  up(event: PointerEvent): void {
    const d = this.dragging;
    this.dragging = null;
    if (!d || d.pointer !== event.pointerId) {
      return;
    }
    const drop = this.drag();
    this.drag.set(null);
    if (event.type === 'pointerup' && d.moved && drop) {
      this.host.drop(drop);
    }
  }
}
