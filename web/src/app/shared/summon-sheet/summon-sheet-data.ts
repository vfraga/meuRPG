import type { DiceMode, DicePreference } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import type { CombatState } from '../../core/combat/combat-state';

/** The sheet opened in a combat (Conjurar Animais, MR-037, E9-12): the cast goes through the combat. */
export interface SummonCombat {
  readonly encounterId: string;
  /** The caster's combatant (a UUID). */
  readonly casterId: string;
  readonly diceMode: DiceMode;
  readonly preference: DicePreference;
  /** The page's copy of the combat: the answer's combat goes there. */
  readonly state: CombatState;
  /** The spell the caster concentrates on now ("Teia"); empty when none: casting this ends it. */
  readonly concentrating: string;
}

/** What the panel (or the combat) hands the sheet. */
export interface SummonSheetData {
  readonly campaignId: string;
  readonly characterId: string;
  readonly spellKey: string;
  /** Set in a combat: the group's initiative roll and the result stage. */
  readonly combat?: SummonCombat;
}
