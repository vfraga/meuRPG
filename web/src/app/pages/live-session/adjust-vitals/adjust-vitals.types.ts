import { VitalsChange, VitalsVm } from '../live-session.types';

/** What the page hands the adjust sheet (or dialog). */
export interface AdjustVitalsData {
  readonly campaignId: string;
  readonly vitals: VitalsVm;
  /** "Ladina 3, de Ana". */
  readonly sub: string;
  /** Whose screen shows the change; `null` when they have no name. */
  readonly playerName: string | null;
}

/** How the sheet closed: saved (with the new vitals), or the session
 * turned out to be over. Closing without saving gives `undefined`. */
export type AdjustVitalsResult =
  { readonly kind: 'saved'; readonly vitals: VitalsVm } | { readonly kind: 'ended' };

/** The numbers the sheet edits. */
export interface VitalsDraft {
  readonly hitPointsCurrent: number;
  readonly hitPointsTemporary: number;
  /** Used slots per spell level. */
  readonly slotsUsed: Readonly<Record<number, number>>;
  readonly pactSlotsUsed: number | null;
  readonly hitDiceUsed: number;
  /** Used uses per resource key. */
  readonly resourcesUsed: Readonly<Record<string, number>>;
  /** The beast's hit points, while the druid is in Wild Shape; `null` otherwise. */
  readonly wildShapeHitPoints: number | null;
}

/** The most temporary hit points the server takes (play.proto). */
export const MAX_TEMPORARY_HP = 999;

export function draftFrom(v: VitalsVm): VitalsDraft {
  return {
    hitPointsCurrent: v.hitPointsCurrent,
    hitPointsTemporary: v.hitPointsTemporary,
    slotsUsed: Object.fromEntries(v.spellSlots.map((s) => [s.level, s.used])),
    pactSlotsUsed: v.pactSlots ? v.pactSlots.used : null,
    hitDiceUsed: v.hitDiceUsed,
    resourcesUsed: Object.fromEntries((v.resources ?? []).map((r) => [r.key, r.used])),
    wildShapeHitPoints: v.wildShape ? v.wildShape.hitPointsCurrent : null,
  };
}

/**
 * The correction to send: only what changed from what the sheet opened
 * with. Every value replaces the stored one (absolute, not "−5"), so a
 * retry can't apply twice, and a number nobody touched here can't undo a
 * change made meanwhile. `null` when nothing changed.
 */
export function changeBetween(before: VitalsVm, draft: VitalsDraft): VitalsChange | null {
  const change: {
    hitPointsCurrent?: number;
    hitPointsTemporary?: number;
    spellSlotsUsed?: { level: number; used: number }[];
    pactSlotsUsed?: number;
    hitDiceUsed?: number;
    resourcesUsed?: { key: string; used: number }[];
    wildShapeHitPointsCurrent?: number;
  } = {};
  if (draft.hitPointsCurrent !== before.hitPointsCurrent) {
    change.hitPointsCurrent = draft.hitPointsCurrent;
  }
  if (draft.hitPointsTemporary !== before.hitPointsTemporary) {
    change.hitPointsTemporary = draft.hitPointsTemporary;
  }
  const slots = before.spellSlots
    .filter((s) => draft.slotsUsed[s.level] !== s.used)
    .map((s) => ({ level: s.level, used: draft.slotsUsed[s.level] }));
  if (slots.length > 0) {
    change.spellSlotsUsed = slots;
  }
  if (
    before.pactSlots &&
    draft.pactSlotsUsed !== null &&
    draft.pactSlotsUsed !== before.pactSlots.used
  ) {
    change.pactSlotsUsed = draft.pactSlotsUsed;
  }
  if (draft.hitDiceUsed !== before.hitDiceUsed) {
    change.hitDiceUsed = draft.hitDiceUsed;
  }
  const resources = (before.resources ?? [])
    .filter(
      (r) => draft.resourcesUsed[r.key] !== undefined && draft.resourcesUsed[r.key] !== r.used,
    )
    .map((r) => ({ key: r.key, used: draft.resourcesUsed[r.key] }));
  if (resources.length > 0) {
    change.resourcesUsed = resources;
  }
  if (
    before.wildShape &&
    draft.wildShapeHitPoints !== null &&
    draft.wildShapeHitPoints !== before.wildShape.hitPointsCurrent
  ) {
    change.wildShapeHitPointsCurrent = draft.wildShapeHitPoints;
  }
  return Object.keys(change).length > 0 ? change : null;
}

/** Same numbers, so a retry reuses the idempotency key. */
export function sameChange(a: VitalsChange, b: VitalsChange): boolean {
  return JSON.stringify(a) === JSON.stringify(b);
}
