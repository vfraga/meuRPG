import { Directive, ElementRef, inject } from '@angular/core';
import { MatSelect } from '@angular/material/select';

/** The helper line under the field, in px (20 and a 4 px gap), and the gap the list keeps under it. */
const HINT_ROOM = 24;
const LIST_GAP = 6;
/** The list at its tallest (`.mr-weapons-panel` in styles.scss) and a little air under it. */
const LIST_HEIGHT = 256;
const AIR = 8;
/** What a list that opens under its field needs below the field's bottom edge, in px. */
const ROOM_BELOW = HINT_ROOM + LIST_GAP + LIST_HEIGHT + AIR;

/**
 * Makes the list of a `mat-select` open under its field and never over it (`<mat-select appOpenDown>`). The select flips
 * its list above the field when the room below is short, and the list then covers the helper line it is meant to stay
 * under. With the positions restricted to below, a short room only makes the list shorter (it scrolls). When the room
 * below is less than a list needs, the field is brought to the middle of the screen first, at once and before the list
 * opens: the list is placed against the field where it stands, and a scroll that came after (the select tells it has
 * opened when its animation ends) would move the field while the list waited for the next scroll event to follow it.
 */
@Directive({ selector: 'mat-select[appOpenDown]' })
export class OpenDown {
  private readonly select = inject(MatSelect);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef).nativeElement;

  constructor() {
    // `_positions` is private to Material (no public input sets where the list opens). The spec test "keeps only the
    // positions under the field" (open-down.spec.ts) fails when Material renames or removes it, so an upgrade says so.
    this.select._positions = this.select._positions.filter((p) => p.originY === 'bottom');
    const open = this.select.open.bind(this.select);
    this.select.open = () => {
      const canOpen = !this.select.disabled && !this.select.panelOpen;
      if (canOpen && window.innerHeight - this.host.getBoundingClientRect().bottom < ROOM_BELOW) {
        (this.host.closest('mat-form-field') ?? this.host).scrollIntoView({
          block: 'center',
          behavior: 'instant',
        });
      }
      open();
    };
  }
}
