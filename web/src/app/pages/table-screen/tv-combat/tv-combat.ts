import { NgTemplateOutlet } from '@angular/common';
import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import type { TvCombatantVm, TvMapVm, TvRollVm } from '../../../core/play/table-screen-view';
import { TvFit } from '../tv-fit/tv-fit';
import { TvMap } from '../tv-map/tv-map';
import { TvOrder } from '../tv-order/tv-order';
import { TvRolls } from '../tv-rolls/tv-rolls';

/** The lines of "Último no combate" the server sends; the panel draws the ones that fit its height. */
export const TV_LOG_LINES = 6;

/**
 * The combat as the table sees it: the map (or its card, or nothing in a combat without one), the order with the
 * round, the last lines of the log and the free rolls. The server sends what the table may read; this lays it out.
 */
@Component({
  selector: 'app-tv-combat',
  imports: [MatIconModule, NgTemplateOutlet, TvFit, TvMap, TvOrder, TvRolls],
  templateUrl: './tv-combat.html',
  styleUrl: './tv-combat.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class TvCombat {
  readonly round = input(1);
  readonly order = input<readonly TvCombatantVm[]>([]);
  readonly mastersTurn = input(false);
  readonly log = input<readonly string[]>([]);
  readonly map = input<TvMapVm | null>(null);
  readonly rolls = input<readonly TvRollVm[]>([]);

  protected readonly lines = computed(() => this.log().slice(0, TV_LOG_LINES));
}
