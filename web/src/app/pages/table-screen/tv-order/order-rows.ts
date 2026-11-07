import type { TvCombatantVm } from '../../../core/play/table-screen-view';

/** How many rows the order draws before "+N". */
export const TV_ORDER_MAX = 10;

export type TvOrderRow =
  | { readonly kind: 'who'; readonly who: TvCombatantVm }
  | { readonly kind: 'round'; readonly label: string };

export interface TvOrderRows {
  readonly rows: readonly TvOrderRow[];
  /** The combatants left out of the list. */
  readonly more: number;
}

/**
 * The order as the TV reads it: the one on turn first, then the rest of the round, then "Rodada N começa" and the
 * ones who already went. When the turn is one the table does not see, the order stays as the server sent it.
 */
export function orderRows(
  order: readonly TvCombatantVm[],
  round: number,
  max = TV_ORDER_MAX,
): TvOrderRows {
  const turn = order.findIndex((c) => c.onTurn);
  const rotated = turn > 0 ? [...order.slice(turn), ...order.slice(0, turn)] : [...order];
  const rows: TvOrderRow[] = rotated.map((who) => ({ kind: 'who', who }));
  if (turn > 0) {
    rows.splice(order.length - turn, 0, { kind: 'round', label: `Rodada ${round + 1} começa` });
  }
  const kept = rows.slice(0, max + (turn > 0 ? 1 : 0));
  const shown = kept.filter((r) => r.kind === 'who').length;
  return { rows: kept, more: order.length - shown };
}
