import { type ElementRef, afterNextRender, effect, untracked, type Signal } from '@angular/core';

import type { Square } from '../../../../core/combat/combat-grid';
import { type ScrollBox, centreScroll, keepInViewScroll } from './move-page-view';

/** What the map's scroll needs from the page. */
export interface MoveScrollInputs {
  /** The frame the map scrolls in (unset until it is drawn). */
  readonly scroller: Signal<ElementRef<HTMLElement> | undefined>;
  readonly wide: Signal<boolean>;
  readonly cell: Signal<number>;
  readonly origin: Signal<Square>;
  readonly chosen: Signal<Square | null>;
}

/** The scroll of the map on the narrow layout: open on the token, and a chosen square never leaves the view. Build it in an injection context. */
export class MoveScroll {
  constructor(private readonly i: MoveScrollInputs) {
    // The map is bigger than the frame it scrolls in: open with the token in the middle of it.
    afterNextRender(() => this.to((box) => centreScroll(this.i.origin(), this.i.cell(), box)));
    // A chosen square never scrolls out of sight (or under the app bar).
    effect(() => {
      const sq = this.i.chosen();
      if (sq) {
        untracked(() => this.to((box) => keepInViewScroll(sq, this.i.cell(), box)));
      }
    });
  }

  /** Scrolls the map to where `target` says, given the part of it the scroller shows (nothing in the wide layout). */
  private to(target: (box: ScrollBox) => { left: number; top: number }): void {
    const el = this.i.scroller()?.nativeElement;
    if (!el || this.i.wide()) {
      return;
    }
    const at = target({
      left: el.scrollLeft,
      top: el.scrollTop,
      width: el.clientWidth,
      height: el.clientHeight,
    });
    el.scrollLeft = at.left;
    el.scrollTop = at.top;
  }
}
