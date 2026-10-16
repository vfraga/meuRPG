/**
 * The resource counters of a character's live numbers (spell slots, pact slots, Wild Shape, class and race
 * resources). A leaf of its own: `core/resources` and `core/combat` read these shapes while `live-session.types.ts`
 * reads `core/combat` for its events, so keeping them in that file made the import knot.
 */

/** One spell level's slots (`SpellSlotUsage`). */
export interface SlotUsageVm {
  readonly level: number;
  readonly total: number;
  readonly used: number;
  /** How many of `total` Flexible Casting created (they vanish on a long rest); absent or 0 for none. */
  readonly created?: number;
}

/** A warlock's pact magic slots (`PactSlotUsage`): all of one level. */
export interface PactSlotsVm {
  readonly slotLevel: number;
  readonly total: number;
  readonly used: number;
}

/** The beast a druid is in (`WildShapeState`, MR-037): its own pool of hit points. */
export interface WildShapeVm {
  /** "monster:wolf". */
  readonly beastKey: string;
  /** "Lobo". */
  readonly beastNamePt: string;
  readonly hitPointsCurrent: number;
  readonly hitPointsMax: number;
}

/** How many uses of a class or race resource the sheet gives and how many are spent (`ResourceUsage`). */
export interface ResourceUsageVm {
  /** "wild_shape", "second_wind". */
  readonly key: string;
  /** "Forma Selvagem"; absent or empty when the server did not say. */
  readonly namePt?: string;
  readonly total: number;
  readonly used: number;
  /** When the uses come back: `short_rest` is "num descanso curto ou longo" (`RECHARGE_SHORT_REST`), `long_rest` only "num descanso longo". */
  readonly recharge: 'short_rest' | 'long_rest' | 'dawn' | 'none';
}
