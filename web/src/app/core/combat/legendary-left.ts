import {
  CombatLogKind,
  type CombatLogEntry,
  type Encounter,
  EncounterStatus,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { type LegendaryNames, legendaryOptionName } from './legendary-card';

/**
 * What the master's turn column says about legendary actions outside the card: the line under "Próximo turno" with
 * what is left to spend, and the line of the last spend that the card's undo stands under. The server decides what a
 * creature may spend (`LegendaryState.usable_now`, `actions_left`); this only phrases it. A pronoun is never guessed
 * from a name: the creature is named as its label. Pure functions, tested without a DOM.
 */

/** One creature that has legendary actions to spend now. */
interface Spendable {
  readonly label: string;
  readonly left: number;
}

function spendable(e: Encounter): readonly Spendable[] {
  if (e.status !== EncounterStatus.ACTIVE) {
    return [];
  }
  return e.combatants.flatMap((c) => {
    const l = c.legendary;
    const left = l ? Math.min(l.actionsLeft, l.actionsPerRound) : 0;
    return l && l.options.length > 0 && l.usableNow && left > 0 && c.id !== e.currentCombatantId
      ? [{ label: c.label, left }]
      : [];
  });
}

function actionsWord(n: number): string {
  return n === 1 ? 'ação lendária' : 'ações lendárias';
}

/**
 * "Restam 2 ações lendárias: Dragão vermelho adulto." under the "Próximo turno" button while a creature has legendary
 * actions to spend now (it is not on turn, it can act, it has points left); `''` otherwise. Several creatures share one
 * sentence, "Restam ações lendárias: Dragão vermelho adulto (2), Vampiro (3).", so the line never grows past two rows.
 */
export function legendaryLeftLine(e: Encounter): string {
  const all = spendable(e);
  if (all.length === 0) {
    return '';
  }
  if (all.length === 1) {
    return `Restam ${all[0].left} ${actionsWord(all[0].left)}: ${all[0].label}.`;
  }
  return `Restam ações lendárias: ${all.map((s) => `${s.label} (${s.left})`).join(', ')}.`;
}

/**
 * "Gastou 2: Ataque de asas.": the master's last spend, from the log's undoable
 * entry, or `''` when that entry is not a spend of legendary actions. The option reads in Portuguese when it is known.
 */
export function legendarySpentLine(
  entry: CombatLogEntry | null,
  names: LegendaryNames | undefined,
): string {
  const spent = entry?.kind === CombatLogKind.LEGENDARY_SPENT ? entry.legendarySpent : undefined;
  if (!entry || !spent) {
    return '';
  }
  const option = legendaryOptionName(names, entry.actorId, spent.optionName) ?? spent.optionName;
  return `Gastou ${spent.cost}: ${option}.`;
}
