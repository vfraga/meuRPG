import { ChangeDetectionStrategy, Component, computed, input, signal } from '@angular/core';

import { CombatantToken } from '../../../shared/combatant-token/combatant-token';
import { TvFit } from '../tv-fit/tv-fit';

/** How many names "Na mesa" draws before "+N". */
export const TV_PARTY_MAX = 12;

/** The waiting screen: "Nada à mostra ainda" and who is at the table. */
@Component({
  selector: 'app-tv-idle',
  imports: [CombatantToken, TvFit],
  templateUrl: './tv-idle.html',
  styleUrl: './tv-idle.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class TvIdle {
  readonly party = input<readonly string[]>([]);

  protected readonly shown = computed(() => this.party().slice(0, TV_PARTY_MAX));
  /** The names that did not fit the room (a TV; none on a phone). */
  protected readonly cut = signal(0);
  protected readonly more = computed(
    () => Math.max(0, this.party().length - TV_PARTY_MAX) + this.cut(),
  );

  protected initial(name: string): string {
    return [...name.trim()][0]?.toUpperCase() ?? '?';
  }
}
