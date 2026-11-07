import { ChangeDetectionStrategy, Component, computed, input, signal } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import { CombatantToken } from '../../../shared/combatant-token/combatant-token';
import { TvFit } from '../tv-fit/tv-fit';

/** How many NPCs "Em cena" draws before "+N". */
export const TV_STAGE_MAX = 8;

export interface TvSceneNpc {
  readonly name: string;
  readonly speaking: boolean;
}

/** The scene open to the whole table: its description, large, and the NPCs the table sees, with who speaks. */
@Component({
  selector: 'app-tv-scene',
  imports: [CombatantToken, MatIconModule, TvFit],
  templateUrl: './tv-scene.html',
  styleUrl: './tv-scene.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class TvScene {
  readonly description = input('');
  readonly stage = input<readonly TvSceneNpc[]>([]);

  protected readonly shown = computed(() => this.stage().slice(0, TV_STAGE_MAX));
  /** The NPCs that did not fit the card's height (a TV; none on a phone). */
  protected readonly cut = signal(0);
  protected readonly more = computed(
    () => Math.max(0, this.stage().length - TV_STAGE_MAX) + this.cut(),
  );

  protected initial(name: string): string {
    return [...name.trim()][0]?.toUpperCase() ?? '?';
  }
}
