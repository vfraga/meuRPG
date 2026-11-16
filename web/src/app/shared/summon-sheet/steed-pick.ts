import { type Signal, computed, signal } from '@angular/core';

import type { SummonSpellOptions } from '../../../gen/meurpg/characters/v1/characters_pb';
import {
  FIND_STEED,
  steedCastError,
  steedCastVerb,
  steedCostLine,
} from '../../core/creatures/steed';

/**
 * What the summon sheet keeps for Convocar Montaria alone (#588): the steed's type, picked on the first cast, and the bonded
 * steed that comes back as it was, with nothing to pick. `on` is false for every other spell, and the sheet asks nothing here.
 */
export class SteedPick {
  readonly on: boolean;
  /** The bonded steed (`SummonSpellOptions.bonded_steed`): the cast brings the same one back. */
  readonly bonded = computed(() => (this.on && this.spell()?.bondedSteed) || null);
  /** The type picked (`SummonChoice.creature_type`); `''` until then. */
  readonly type = signal('');
  /** The type is still to pick: the cast button stays off. */
  readonly needsType = computed(() => this.on && this.type() === '');
  readonly costLine = computed(() => steedCostLine(this.spell()?.level ?? 0));
  readonly castError = steedCastError;

  constructor(
    spellKey: string,
    private readonly spell: Signal<SummonSpellOptions | null>,
  ) {
    this.on = spellKey === FIND_STEED;
  }

  /** Once the options are read: the bonded steed's form and type are the cast's. Returns the form's key, or `''`. */
  preset(): string {
    const bonded = this.bonded();
    if (!bonded) {
      return '';
    }
    this.type.set(bonded.creatureType);
    return bonded.monsterKey;
  }

  /** "Convocar Estrela", "Convocar Cavalo de guerra", or the plain verb before a form is chosen. */
  verb(formTitle: string): string {
    return steedCastVerb(this.bonded()?.name ?? '', formTitle);
  }

  /** What the cast adds to `SummonChoice`. */
  choice(): { readonly creatureType?: string } {
    return this.on ? { creatureType: this.type() } : {};
  }
}
