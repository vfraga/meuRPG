import { ChangeDetectionStrategy, Component, input } from '@angular/core';

import { CombatantToken } from './combatant-token';
import { RiderMark } from './rider-mark';

/**
 * The legend's "Montado": a small token with the rider's disc on the saddle, as the map draws the pair, then the words.
 * It stands in the legend's list as an item of its own (`role="listitem"`).
 */
@Component({
  selector: 'app-mounted-legend',
  host: { role: 'listitem' },
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [CombatantToken, RiderMark],
  template: `<span class="pair" aria-hidden="true"
      ><app-combatant-token initial="" [npc]="true" [ridden]="true" [size]="22" [onMap]="true" /><app-rider-mark
        initial=""
        [onMap]="true"
        size="22px" /></span
    >{{ text() }}`,
  styles: `
    :host {
      display: flex;
      align-items: center;
      gap: var(--mr-space-2);
    }

    .pair {
      position: relative;
      display: inline-flex;
      flex: none;
    }
  `,
})
export class MountedLegend {
  /** What the mark says: "Montado" for the master, "Montado: o cavaleiro pequeno, na sela" for a player. */
  readonly text = input.required<string>();
}
