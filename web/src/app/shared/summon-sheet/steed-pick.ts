import { type Signal, computed, signal } from '@angular/core';

import type { Creature } from '../../../gen/meurpg/rules/v1/rules_pb';
import type { SummonSpellOptions } from '../../../gen/meurpg/characters/v1/characters_pb';
import { creatureErrorMessage } from '../../core/creatures/creature-errors';
import {
  FIND_STEED,
  steedCastError,
  steedCastVerb,
  steedCostLine,
  steedSetupLine,
  steedSubtitleTail,
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
  /** The end of the sheet's subtitle. */
  readonly tail = computed(() => (this.on ? steedSubtitleTail(this.bonded() !== null) : ''));

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

  /** The bold part of the footer's line: all of it, or for Convocar Montaria only what it spends (its first sentence, 588-21). */
  costLead(line: string): string {
    const end = this.on ? line.indexOf('. ') : -1;
    return end < 0 ? line : line.slice(0, end + 1);
  }

  /** What follows the bold part, with its space; `''` when the whole line is bold. */
  costRest(line: string): string {
    return line.slice(this.costLead(line).length);
  }

  /** "Espaço de 2º nível · 2 livres"; `''` with no slot chosen. */
  setupLine(slot: { readonly level: number; readonly free: number } | null): string {
    return slot ? steedSetupLine(slot.level, slot.free) : '';
  }

  /** A refused cast: the steed's own words for Convocar Montaria, any summon's for the rest. */
  castError(err: unknown): string {
    return this.on
      ? steedCastError(err, this.bonded()?.name ?? '')
      : creatureErrorMessage(err, 'cast');
  }

  /** The bonded steed's stat block among the forms read, for its fixed row; `null` until then. */
  block(forms: readonly Creature[] | null): Creature | null {
    return forms?.find((c) => c.summary?.key === this.bonded()?.monsterKey) ?? null;
  }

  /** What the cast adds to `SummonChoice`. */
  choice(): { readonly creatureType?: string } {
    return this.on ? { creatureType: this.type() } : {};
  }
}
