import { ChangeDetectionStrategy, Component, input } from '@angular/core';

import { CombatantToken } from './combatant-token';

/**
 * The rider on its mount's token: the rider's small disc inside the token, top right, sitting on an ink
 * saddle crescent. It fills the box of the mount's token, which its parent positions (`position: relative`); the mount's own
 * initial moves to the lower left (`CombatantToken.ridden`). Decorative: the lists and the map's text name the pair.
 */
@Component({
  selector: 'app-rider-mark',
  imports: [CombatantToken],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <span class="rm__corner" [class.rm__corner--in]="footprint() > 1" [style.--rm-side]="size()" [style.--rm-n]="footprint()">
      <svg class="rm__saddle" viewBox="0 0 100 100" focusable="false">
        <path d="M33 37 Q60 108 97 41 Q63 80 33 37 Z" />
      </svg>
      <app-combatant-token
        class="rm__disc"
        [initial]="initial()"
        [npc]="npc()"
        [creature]="creature()"
        [defeated]="defeated()"
        [onMap]="onMap()"
        [size]="'max(calc(' + size() + ' * 0.42), 13px)'"
      />
    </span>
  `,
  styles: `
    :host {
      position: absolute;
      inset: 0;
      pointer-events: none;
    }

    // The saddle and the disc sit in a corner box of one square's token, top right, so a mount drawn on 2 × 2 keeps the
    // rider's disc the size of any other and in the corner.
    .rm__corner {
      position: absolute;
      top: 0;
      right: 0;
      width: var(--rm-side);
      height: var(--rm-side);
    }

    // A mount drawn on 2 × 2 or more: the corner box moves in, so the rider's disc sits inside the mount's circle, at 45°, not
    // on a corner of the bounding box that the circle does not reach.
    .rm__corner--in {
      --rm-d: calc(var(--rm-n) * var(--rm-side));
      --rm-k: calc((var(--rm-d) / 2 - 0.25 * var(--rm-side)) * 0.7071);
      top: calc(var(--rm-d) / 2 - var(--rm-k) - 0.33 * var(--rm-side));
      right: calc(var(--rm-d) / 2 - var(--rm-k) - 0.3 * var(--rm-side));
    }

    .rm__saddle {
      position: absolute;
      inset: 0;
      width: 100%;
      height: 100%;
      overflow: visible;
      fill: var(--mr-ink);
      stroke: var(--mr-surface);
      stroke-width: 3;
      stroke-linejoin: round;
      paint-order: stroke;
    }

    :host(.rm--map) .rm__saddle {
      fill: var(--mr-map-token-ink);
      stroke: var(--mr-map-token-surface);
    }

    .rm__disc {
      position: absolute;
      top: 12%;
      right: 9%;
      // The small disc keeps a thin edge, whatever the size of the token under it.
      border-width: 1px;
      // The rider's initial is readable on a 22 px token too: never under 10 px.
      font-size: max(calc(var(--rm-side) * 0.2), 10px);
    }
  `,
  host: { 'aria-hidden': 'true', '[class.rm--map]': 'onMap()' },
})
export class RiderMark {
  /** The rider's initial. */
  readonly initial = input.required<string>();
  readonly npc = input(false);
  readonly creature = input(false);
  readonly defeated = input(false);
  /** Drawn on a battle map: the map's fixed colours. */
  readonly onMap = input(false);
  /** The side of one square's token, as a CSS length: the corner the saddle and the disc fill (a mount on 2 × 2 has the same corner). */
  readonly size = input.required<string>();
  /** How many squares a side of the mount's token covers (`2` for a Large mount): the disc moves inside its circle. */
  readonly footprint = input(1);
}
