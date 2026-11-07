import { ChangeDetectionStrategy, Component, computed, input, signal } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import type { TvCombatantVm } from '../../../core/play/table-screen-view';
import { CombatantToken } from '../../../shared/combatant-token/combatant-token';
import { TvFit } from '../tv-fit/tv-fit';
import { orderRows } from './order-rows';

/** The words that mean a combatant is out of the fight, for the greyed token. */
const OUT = new Set(['Derrotado', 'Caído']);

/** "Ordem" and "Rodada N": the combatants the table sees, the one on turn marked, a word of state each (RN-20). */
@Component({
  selector: 'app-tv-order',
  imports: [CombatantToken, MatIconModule, TvFit],
  templateUrl: './tv-order.html',
  styleUrl: './tv-order.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class TvOrder {
  readonly order = input<readonly TvCombatantVm[]>([]);
  readonly round = input(1);
  /** The turn is one the table does not see: "Vez do mestre". */
  readonly mastersTurn = input(false);

  protected readonly list = computed(() => orderRows(this.order(), this.round()));
  /** The rows that did not fit the panel's height (a TV; none on a phone). */
  protected readonly cut = signal(0);
  /** The combatants left out: by the cap of the list, and by the room. */
  protected readonly more = computed(() => this.list().more + this.cut());

  protected out(who: TvCombatantVm): boolean {
    return who.stateWord !== null && OUT.has(who.stateWord);
  }
}
