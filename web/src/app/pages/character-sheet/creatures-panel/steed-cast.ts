import { type Injector, signal } from '@angular/core';

import type {
  CharacterCreature,
  SummonSpellOptions,
} from '../../../../gen/meurpg/characters/v1/characters_pb';
import { CombatClient } from '../../../core/combat/combat-client';
import { FIND_STEED } from '../../../core/creatures/steed';
import { steedCastHint } from '../../../core/creatures/steed-card';
import { castCostText } from '../../../core/creatures/summon-labels';

/**
 * What the "Criaturas" panel says at the cast button of Convocar Montaria (#588, boards 588-23 and 588-26). The cast takes
 * ten minutes: in a combat that holds the character the button is dashed, with why. Under it, what the cast does for a
 * bonded steed: brings it back when it is away, or fills its hit points when it is here.
 */
export class SteedCast {
  /** A running combat holds the character. */
  readonly fighting = signal(false);
  private seq = 0;

  constructor(private readonly injector: Injector) {}

  /** Reads the combat when the character can cast the steed during a session; no combat is read otherwise. */
  async read(
    spells: readonly SummonSpellOptions[],
    live: boolean,
    campaignId: string,
    characterId: string,
  ): Promise<void> {
    const seq = ++this.seq;
    if (!live || !spells.some((s) => s.spellKey === FIND_STEED)) {
      this.fighting.set(false);
      return;
    }
    let fighting = false;
    try {
      const enc = await this.injector.get(CombatClient).turn.get(campaignId);
      fighting = !!enc?.combatants.some((c) => c.characterId === characterId);
    } catch {
      // No answer: the button stays on, and the server refuses a cast in a combat in the sheet's own words.
    }
    if (seq === this.seq) {
      this.fighting.set(fighting);
    }
  }

  /** Why the cast button of `spell` is off; `''` when it is on. */
  why(spell: SummonSpellOptions, live: boolean): string {
    if (!live) {
      return 'Agora não há sessão aberta.';
    }
    return spell.spellKey === FIND_STEED && this.fighting()
      ? 'Leva 10 minutos: não dá durante um combate.'
      : '';
  }

  /** The line under the cast button. */
  cost(spell: SummonSpellOptions, creatures: readonly CharacterCreature[]): string {
    const steed = spell.bondedSteed;
    if (!steed) {
      return castCostText(spell);
    }
    return creatures.some((c) => c.id === steed.id)
      ? `${castCostText(spell)} ${steedCastHint(steed.name, false)}`
      : steedCastHint(steed.name, true);
  }
}
