import { isUnlimited } from '../../../core/combat/unlimited-uses';
import type { ResourceUsageVm } from '../../../core/play/resource-usage.types';
import type { VitalsVm } from '../../../core/play/live-session.types';
import { slotLevelLabel } from '../../../core/play/vitals';

/** Up to this many uses the box draws a pip for each; above it the box shows only the number (PM-07b 7). */
export const MAX_PIPS = 6;

/** One resource box of the sheet. */
export interface ResourceBoxVm {
  readonly key: string;
  readonly name: string;
  /** "2 de 3", or "ilimitado". */
  readonly count: string;
  /** The uses left, for the pips; `null` when the box shows only the number. */
  readonly pips: { readonly left: number; readonly total: number } | null;
  /** "Volta num descanso curto ou longo"; empty when the resource has no rule the app words. */
  readonly again: string;
}

/** The line saying when the uses come back (`ResourceUsage.recharge`); nothing for the other kinds. */
export function rechargeLine(recharge: ResourceUsageVm['recharge']): string {
  switch (recharge) {
    case 'short_rest':
      return 'Volta num descanso curto ou longo';
    case 'long_rest':
      return 'Volta num descanso longo';
    default:
      return '';
  }
}

/** The uses a resource has left. */
function left(r: { total: number; used: number }): number {
  return Math.max(0, r.total - r.used);
}

/** The boxes of the character's resources, in the sheet's order; a resource with no uses at this level has none. */
export function resourceBoxes(resources: readonly ResourceUsageVm[] | undefined): ResourceBoxVm[] {
  return (resources ?? [])
    .filter((r) => r.total > 0)
    .map((r) => {
      const name = r.namePt || 'Recurso';
      const unlimited = isUnlimited(r.total);
      const count = unlimited ? 'ilimitado' : `${left(r)} de ${r.total}`;
      const again = rechargeLine(r.recharge);
      return {
        key: r.key,
        name,
        count,
        pips: !unlimited && r.total <= MAX_PIPS ? { left: left(r), total: r.total } : null,
        again,
      };
    });
}

/** One row of "Espaços de magia": "1º nível ●●●○ 3 de 4". */
export interface SlotCounterRow {
  readonly key: string;
  readonly label: string;
  readonly left: number;
  readonly total: number;
  readonly count: string;
  /** "criado" next to the count when Flexible Casting made some of the slots. */
  readonly created: string;
}

function createdWords(created: number): string {
  if (created <= 0) {
    return '';
  }
  return created === 1 ? 'criado' : `${created} criados`;
}

/** The rows of the spell slots, one per level, then the pact slots (which the sheet already kept apart). */
export function slotCounterRows(v: Pick<VitalsVm, 'spellSlots' | 'pactSlots'>): SlotCounterRow[] {
  const row = (
    key: string,
    label: string,
    s: { total: number; used: number },
    created: number,
  ): SlotCounterRow => {
    const count = `${left(s)} de ${s.total}`;
    const made = createdWords(created);
    return {
      key,
      label,
      left: left(s),
      total: s.total,
      count,
      created: made,
    };
  };
  const rows = v.spellSlots.map((s) =>
    row(`level-${s.level}`, slotLevelLabel(s.level), s, s.created ?? 0),
  );
  if (v.pactSlots) {
    rows.push(row('pact', `Pacto · ${slotLevelLabel(v.pactSlots.slotLevel)}`, v.pactSlots, 0));
  }
  return rows;
}

/** The note under the slots: when they come back, and the warlock's exception only for a character that has pact slots. */
export function slotsFootnote(v: Pick<VitalsVm, 'spellSlots' | 'pactSlots'>): string {
  const regular = v.spellSlots.length > 0;
  if (regular && v.pactSlots) {
    return 'Voltam num descanso longo. (O Bruxo recupera os espaços de pacto num descanso curto ou longo.)';
  }
  if (v.pactSlots) {
    return 'Os espaços de pacto voltam num descanso curto ou longo.';
  }
  return regular ? 'Voltam num descanso longo.' : '';
}
