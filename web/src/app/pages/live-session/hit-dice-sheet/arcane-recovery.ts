import type { SlotUsageVm } from '../../../core/play/resource-usage.types';
import type { VitalsVm } from '../../../core/play/live-session.types';

/** The resource of the wizard's Recuperação Arcana (`rules.ArcaneRecoveryKey`). */
export const ARCANE_RECOVERY_KEY = 'arcane_recovery';

/** The highest slot level Recuperação Arcana gives back (SRD 5.1, Wizard: "none ... 6th level or higher"). */
export const ARCANE_RECOVERY_MAX_LEVEL = 5;

/** Where a wizard's Recuperação Arcana stands: `null` for a character without it. */
export interface ArcaneRecoveryState {
  /** The combined spell level the slots may add up to. */
  readonly allowance: number;
  /** The use is spent: it comes back on a long rest. */
  readonly spent: boolean;
}

export function arcaneRecoveryState(v: VitalsVm): ArcaneRecoveryState | null {
  const use = v.resources?.find((r) => r.key === ARCANE_RECOVERY_KEY);
  const allowance = v.arcaneRecoveryAllowance ?? 0;
  if (!use || allowance < 1) {
    return null;
  }
  return { allowance, spent: use.total - use.used < 1 };
}

/** The slots of levels 1 to 5 with some expended, lowest first: the only ones the feature can give back. */
export function expendedSlots(
  v: VitalsVm,
): readonly (SlotUsageVm & { readonly expended: number })[] {
  return v.spellSlots
    .filter((s) => s.level <= ARCANE_RECOVERY_MAX_LEVEL && s.used > 0)
    .map((s) => ({ ...s, expended: Math.min(s.used, s.total) }))
    .filter((s) => s.expended > 0);
}

/** "2 níveis", "1 nível". */
export function levelsWords(n: number): string {
  return `${n} ${n === 1 ? 'nível' : 'níveis'}`;
}
