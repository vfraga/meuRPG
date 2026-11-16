import type { Combatant } from '../../../gen/meurpg/play/v1/combat_pb';

/** Hit points as a pair. */
export interface Pool {
  readonly current: number;
  readonly max: number;
}

/**
 * The hit points the combat plays with (boards 584-02, 584-04, 584-05): a druid in a Wild Shape form is the form's pool, in the
 * order, on the cards and on the turn; its own come back when the form ends. `null` when the viewer has no numbers (RN-20).
 */
export function combatPool(c: Combatant): Pool | null {
  if (c.wildShapeHitPointsMax !== undefined && c.wildShapeHitPointsCurrent !== undefined) {
    return { current: c.wildShapeHitPointsCurrent, max: c.wildShapeHitPointsMax };
  }
  return c.hitPointsMax === undefined
    ? null
    : { current: c.hitPointsCurrent ?? c.hitPointsMax, max: c.hitPointsMax };
}

/** The druid's own hit points while it is in a form ("a Sálvia tem 27 de 27"); `null` out of one, or without numbers. */
export function ownPool(c: Combatant): Pool | null {
  return c.wildShapeHitPointsMax !== undefined && c.hitPointsMax !== undefined
    ? { current: c.hitPointsCurrent ?? c.hitPointsMax, max: c.hitPointsMax }
    : null;
}

/** The pool of a player's own vitals: the form's while in one. */
export function vitalsPool(v: {
  readonly hitPointsCurrent: number;
  readonly hitPointsMax: number;
  readonly wildShape?: {
    readonly hitPointsCurrent: number;
    readonly hitPointsMax: number;
  } | null;
}): Pool {
  return v.wildShape
    ? { current: v.wildShape.hitPointsCurrent, max: v.wildShape.hitPointsMax }
    : { current: v.hitPointsCurrent, max: v.hitPointsMax };
}

/** How full the row's pool is, 0 to 100, for its bar; 0 with no pool to show. */
export function poolPercent(c: Combatant): number {
  const pool = combatPool(c);
  return pool && pool.max > 0 ? Math.min(100, Math.round((pool.current / pool.max) * 100)) : 0;
}
