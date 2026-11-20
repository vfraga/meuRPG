import {
  type CombatLogEntry,
  CombatLogKind,
  ConsumableUseOutcome,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { kindOf } from '../characters/consumable-vm';
import { itemWords, rollSum } from '../characters/consumables';
import type { LogLine } from './combat-log-types';

/** What a builder of these lines is given: the entry and the fields every line shares. */
interface ConsumableArgs {
  readonly e: CombatLogEntry;
  readonly base: { id: string; actor: string; hidden: boolean; undoable: boolean };
}

/** The dice as the sheet writes them: "2d4+2" is "2d4 + 2", "1d4-1" is "1d4 − 1". */
function spacedDice(text: string): string {
  return text.replace(/\s*([+−-])\s*/, (_, sign: string) => ` ${sign === '-' ? '−' : sign} `);
}

/**
 * "CONSUMABLE_USED": who drank or gave which item. The server sends the numbers of a heal (the dice, the faces and the hit
 * points) only to the master and the owners of the user and of the receiver, so a line with none says only who and what
 * (RN-10). A resistance names the damage type and says the length the server sent, which every reader knows. The verb follows the
 * kind the server sent, never the item's name.
 */
function consumableUsed({ e, base }: ConsumableArgs): LogLine | null {
  const use = e.consumableUse;
  if (!use) {
    return null;
  }
  const kind = kindOf(use.kind);
  const { indefinite: item, verb } = itemWords(use.itemNamePt, kind);
  const lead = use.targetLabel ? ` deu ${item} a ${use.targetLabel}` : ` ${verb} ${item}`;
  let text = lead;
  if (use.outcome === ConsumableUseOutcome.HEALED && use.diceText !== '') {
    text = `${lead}: ${use.healed} PV (${spacedDice(use.diceText)}: ${rollSum(use.faces, use.rollTotal)})`;
  } else if (use.outcome === ConsumableUseOutcome.RESISTANCE && use.damageTypeNamePt !== '') {
    const length = use.effectDurationTextPt ? ` ${use.effectDurationTextPt}` : '';
    text = `${lead}: resistência a ${use.damageTypeNamePt.toLowerCase()}${length}`;
  }
  return { ...base, icon: kind === 'scroll' ? 'description' : 'science', text };
}

/** The line of a potion or another consumable used in a combat: the master's, the user's owner's and the receiver's owner's carry the numbers. */
export const CONSUMABLE_LINES: readonly (readonly [
  CombatLogKind,
  (args: ConsumableArgs) => LogLine | null,
])[] = [[CombatLogKind.CONSUMABLE_USED, consumableUsed]];
