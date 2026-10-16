import type { ResourceUsageVm } from '../play/resource-usage.types';

/** What is left of a counted resource (the paladin's pool, the sorcerer's points, the bard's uses) and how much it holds. */
export interface Pool {
  readonly left: number;
  readonly total: number;
}

/** The resource keys of the class resources that have a dialog (`ResourceUsage.key`). */
export const LAY_ON_HANDS_RESOURCE = 'lay_on_hands';
export const SORCERY_POINTS_RESOURCE = 'sorcery_points';
export const BARDIC_INSPIRATION_RESOURCE = 'bardic_inspiration';

/** The pool of one resource from the character's vitals, or `null` when the character does not have it. */
export function poolOf(
  resources: readonly ResourceUsageVm[] | undefined,
  key: string,
): Pool | null {
  const found = resources?.find((r) => r.key === key);
  return found ? { left: Math.max(0, found.total - found.used), total: found.total } : null;
}

/** "1 ponto", "8 pontos". */
export function pointsText(n: number): string {
  return `${n} ${n === 1 ? 'ponto' : 'pontos'}`;
}
