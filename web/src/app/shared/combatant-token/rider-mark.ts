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
      [size]="'calc(' + size() + ' * 0.42)'"
    />
  `,
  styles: `
    :host {
      position: absolute;
      inset: 0;
      pointer-events: none;
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
  /** The side of the mount's token, as a CSS length. */
  readonly size = input.required<string>();
}
