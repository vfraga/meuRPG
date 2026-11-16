import type { Combatant } from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { ExtraActionOption } from '../../../../../gen/meurpg/play/v1/lasting_effects_pb';
import { AttackKind, type AttackOption } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { groupState } from '../../../../core/combat/combat-options';
import { cannotAct } from '../../../../core/combat/conditions';
import type { ActionTabId } from '../../../../core/combat/action-tabs';
import { GRAPPLE_DETAIL, SHOVE_DETAIL } from '../../../../core/combat/contest-view';
import {
  ContestAttackOptionKind,
  type ContestAttackOption,
} from '../../../../../gen/meurpg/play/v1/contest_types_pb';
import { circleLabel } from '../../../../core/combat/combat-grid';
import { freeText } from '../../../../core/combat/cast-flow';
import type { PactSlotsVm, SlotUsageVm } from '../../../../core/play/resource-usage.types';

/** The standard actions an extra action may be, by the key the server sends. */
export const EXTRA_NAMES: Readonly<Record<string, string>> = {
  dash: 'Disparada',
  disengage: 'Desengajar',
  hide: 'Esconder',
  'use-an-object': 'Usar um objeto',
};

/** The icon beside each tab's words (C4-03, C4-04). */
export const TAB_ICONS: Readonly<Record<ActionTabId, string>> = {
  attacks: 'swords',
  spells: 'auto_fix_high',
  class: 'shield',
  other: 'more_horiz',
};

/** "1º": the circle as the slots line writes it. */
export const shortCircle = (level: number): string => `${level}º`;

/** The question Fúria asks a Berserker (SRD 5.1, Path of the Berserker, Frenzy). */
export const FRENZY_QUESTION =
  'Entrar em frenesi? Um ataque corpo a corpo como ação bônus em cada turno seguinte; quando a fúria acabar, você ganha 1 nível de exaustão.';

/** The word of the attack list for Agarrar and Empurrar. */
export function specialAttack(o: ContestAttackOption): { name: string; detail: string } {
  return o.kind === ContestAttackOptionKind.SHOVE
    ? { name: 'Empurrar', detail: SHOVE_DETAIL }
    : { name: 'Agarrar', detail: GRAPPLE_DETAIL };
}

/** One row of the slots above the spells: the circle, how many there are and how many are spent. */
export interface SlotRow {
  readonly title: string;
  readonly short: string;
  readonly total: number;
  readonly used: number;
  readonly text: string;
}

/** One row for each circle the character has slots in, then the pact slots. */
export function slotRowsOf(slots: {
  readonly usage: readonly SlotUsageVm[];
  readonly pact: PactSlotsVm | null;
}): SlotRow[] {
  const { usage, pact } = slots;
  const rows = usage
    .filter((u) => u.total > 0)
    .map((u) => ({
      title: circleLabel(u.level),
      short: shortCircle(u.level),
      total: u.total,
      used: u.used,
      text: freeText(u.total - u.used, u.total),
    }));
  if (pact && pact.total > 0) {
    rows.push({
      title: `${circleLabel(pact.slotLevel)} (pacto)`,
      short: `${shortCircle(pact.slotLevel)} pacto`,
      total: pact.total,
      used: pact.used,
      text: freeText(pact.total - pact.used, pact.total),
    });
  }
  return rows;
}

/** The word on the Ação header: with Extra Attack, once the first attack spent the action, "1 ataque restante" (an open circle: it is not over). */
export function actionWordOf(
  own: Combatant,
  left: number,
  perAction: number,
): { readonly word: string; readonly used: boolean } {
  if (cannotAct(own)) {
    return { word: 'Indisponível', used: true };
  }
  const used = own.actionUsed;
  if (used && left > 0 && perAction > 1) {
    return { word: `${left} ${left === 1 ? 'ataque restante' : 'ataques restantes'}`, used: false };
  }
  return { word: groupState(used), used };
}

/** The weapon attacks the extra action may make (one weapon attack, SRD 5.1, Velocidade): no cantrip and no save. */
export function extraWeaponsOf(
  extra: ExtraActionOption | null,
  attacks: readonly AttackOption[],
): readonly AttackOption[] {
  return extra?.allowedActions.includes('attack')
    ? attacks.filter((a) => a.attack?.kind === AttackKind.WEAPON && a.attack.saveDc === 0)
    : [];
}

/** The standard actions the extra action may be, but the attack: "Disparada", "Desengajar", "Esconder", "Usar um objeto". */
export function extraStandardsOf(
  extra: ExtraActionOption | null,
): readonly { readonly key: string; readonly name: string }[] {
  return (extra?.allowedActions ?? [])
    .filter((k) => k !== 'attack' && k in EXTRA_NAMES)
    .map((k) => ({ key: `standard:${k}`, name: EXTRA_NAMES[k] }));
}
