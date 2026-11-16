import { computed, signal } from '@angular/core';

import type { DiceMode, DicePreference } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import type { SummonSpellOptions } from '../../../gen/meurpg/characters/v1/characters_pb';
import type { SlotRow } from '../../core/combat/cast-flow';
import type { Combatant } from '../../../gen/meurpg/play/v1/combat_pb';
import type { CombatClient } from '../../core/combat/combat-client';
import type { CombatState } from '../../core/combat/combat-state';
import { parseSum } from '../../core/combat/combat-dice';
import { summonResult } from '../../core/combat/summon-result';
import { diceFlagsOf, resultTitleOf } from './summon-views';

/** What the sheet chose for a casting made in a combat. */
export interface SummonCombatCast {
  readonly campaignId: string;
  readonly encounterId: string;
  readonly casterId: string;
  /** The page's copy of the combat: the answer's combat goes there. */
  readonly state: CombatState;
  readonly spell: SummonSpellOptions;
  /** The slot; `null` for a feature's summon, which spends a use of its resource. */
  readonly slot: SlotRow | null;
  /** The d20 of the group's initiative: the face of a physical die, or `null` for the app's. */
  readonly face: number | null;
  readonly key: string;
  readonly option: number;
  readonly creatureKeys: readonly string[];
  readonly names: readonly string[];
}

/** The result stage of a casting in a combat: the sentence, the creatures that joined, and the sheet's new title. */
export interface SummonCombatResult {
  readonly text: string;
  readonly creatures: readonly Combatant[];
  readonly title: string;
}

/**
 * In a combat the cast goes through `CastSpell`: the d20 of the group's initiative (the app's, or the one typed), the slot
 * and the choice. The answer's combat goes to the page's copy, and the sheet gets what to show.
 */
export async function castSummonInCombat(
  api: CombatClient,
  c: SummonCombatCast,
): Promise<SummonCombatResult> {
  const res = await api.casts.castSpell(
    c.campaignId,
    c.encounterId,
    c.casterId,
    c.spell.spellKey,
    c.slot ? { level: c.slot.level, pact: c.slot.pact } : null,
    [],
    c.face !== null ? { face: c.face } : { inApp: true as const },
    c.key,
    { option: c.option, creatureKeys: c.creatureKeys, names: c.names },
    '',
    undefined,
    [],
    c.spell.grantedBy ? { grantedBy: c.spell.grantedBy } : {},
  );
  c.state.apply(res.encounter);
  const made = summonResult(res.encounter, res.summoned);
  return { text: made.text, creatures: made.creatures, title: resultTitleOf(made.creatures) };
}

/** The faces of the d20 of an initiative roll. */
const D20 = 20;

/**
 * The one d20 of the group's initiative in a combat: the app's, or the face of a physical die typed here. The campaign may
 * allow only one of them (RN-18); where both are allowed the player's preference starts it, and a link switches.
 */
export class InitiativeDie {
  readonly canApp: boolean;
  readonly canType: boolean;
  /** The d20 comes from a physical die. */
  readonly typing;
  readonly typed = signal('');
  /** The face field shows, and the cast needs and sends the face. */
  readonly needsFace = computed(() => this.inCombat && (this.typing() || !this.canApp));
  /** The d20 typed: a number from 1 to 20, or `null`. */
  readonly face = computed(() => parseSum(this.typed(), 1, D20));
  private readonly inCombat: boolean;

  constructor(combat: { diceMode: DiceMode; preference: DicePreference } | undefined) {
    const flags = diceFlagsOf(combat);
    this.inCombat = !!combat;
    this.canApp = flags.canApp;
    this.canType = flags.canType;
    this.typing = signal(flags.typing);
  }
}
