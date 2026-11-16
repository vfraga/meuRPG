import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';

import { CombatantToken } from './combatant-token';

/** The side of the mount's token in the turn's banner on a phone, in pixels. */
const SIDE = 30;
/** The rider's disc beside the mount's token: half its side. */
const RIDER = 0.5;

/**
 * The token of a rider's turn with the mount it directs (#588, boards 588-04 and 588-05): the mount's token, with the rider's
 * small disc on its top right corner. The map draws the pair its own way, inside one square (`app-rider-mark`). Decorative: the
 * title beside it names the two.
 */
@Component({
  selector: 'app-pair-token',
  imports: [CombatantToken],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <app-combatant-token [initial]="mount()" [npc]="npc()" [creature]="!npc()" [current]="current()" [size]="size()" />
    <app-combatant-token class="pt__rider" [initial]="rider()" [size]="small()" />
  `,
  styles: `
    :host {
      position: relative;
      display: inline-flex;
      flex: none;
    }

    .pt__rider {
      position: absolute;
      top: -6px;
      right: -6px;
    }
  `,
  host: { 'aria-hidden': 'true' },
})
export class PairToken {
  /** The mount's initial. */
  readonly mount = input.required<string>();
  /** The rider's initial: a player's character, a filled disc. */
  readonly rider = input.required<string>();
  /** The mount is an NPC (a rounded square); otherwise a player's creature (round, dashed). */
  readonly npc = input(false);
  readonly current = input(false);
  /** The side of the mount's token, in pixels. */
  readonly size = input(SIDE);

  protected readonly small = computed(() => Math.round(this.size() * RIDER));
}
