/**
 * What the "?" next to a spell shows (`rules.v1.SpellDetails`, trimmed to the
 * four fields of the sheet and the SRD text). Shared by the character editor
 * and the combat's spell list: neither pulls the other's code.
 */

export type CastingTimeUnitKey = '' | 'action' | 'bonus_action' | 'reaction' | 'minute' | 'hour';
export type RangeKindKey = '' | 'self' | 'touch' | 'ranged' | 'sight' | 'unlimited' | 'special';
export type DurationKindKey = '' | 'instantaneous' | 'timed' | 'until_dispelled' | 'special';
export type DurationUnitKey = '' | 'round' | 'minute' | 'hour' | 'day';

/**
 * What the "?" next to a spell shows (`rules.v1.SpellDetails`, trimmed to
 * the four fields of the sheet and the SRD text). Values are in the SRD's
 * units and language; `spell-details-format.ts` writes them in Portuguese.
 * Whatever the structure can't carry stays in each `raw`.
 */
export interface SpellDetailsVm {
  readonly key: string;
  readonly namePt: string;
  /** The SRD's own (English) name. */
  readonly nameEn: string;
  /** 0 is a cantrip. */
  readonly level: number;
  readonly schoolNamePt: string;
  readonly ritual: boolean;
  readonly concentration: boolean;
  readonly castingTime: {
    readonly amount: number;
    readonly unit: CastingTimeUnitKey;
    /** For a reaction, when it is cast, in English. */
    readonly trigger: string;
    readonly raw: string;
  };
  readonly range: {
    readonly kind: RangeKindKey;
    readonly distanceFt: number;
    readonly raw: string;
  };
  readonly components: {
    readonly verbal: boolean;
    readonly somatic: boolean;
    readonly material: boolean;
    /** In English (a table spell's own is in Portuguese). */
    readonly materialText: string;
    /** Our Portuguese translation of `materialText`; empty or unset while the spell has none. */
    readonly materialTextPt?: string;
    /** The name of the grant that waives the material, when one does: the line says why the "M" is gone. */
    readonly materialWaivedBy?: string;
  };
  readonly duration: {
    readonly kind: DurationKindKey;
    readonly amount: number;
    readonly unit: DurationUnitKey;
    readonly upTo: boolean;
    readonly concentration: boolean;
    readonly raw: string;
  };
  /** The SRD description, in English, one paragraph per entry. */
  readonly description: readonly string[];
  /** "At Higher Levels", in English. Empty when the spell has none. */
  readonly higherLevel: readonly string[];
  /** Our Portuguese translation of `description`, one paragraph each; empty while `textPtMissing`. */
  readonly descriptionPt?: readonly string[];
  /** Our Portuguese translation of `higherLevel`; empty while `textPtMissing`. */
  readonly higherLevelPt?: readonly string[];
  /** An SRD spell with no Portuguese text yet: only the English exists. */
  readonly textPtMissing?: boolean;
  /** The text exists only in Portuguese (a table spell): no English to offer. */
  readonly textPtOnly?: boolean;
  /** A spell of the table's own ("Da mesa"): the master wrote its texts, in Portuguese. */
  readonly table?: boolean;
  /** "Uma criatura", "Cone de 4,5 m"... (`target.label_pt`); empty or unset for none. */
  readonly targetLabel?: string;
  /** The attack roll, for a table spell's "Ataque" row. */
  readonly attack?: 'none' | 'melee' | 'ranged';
  /** The damage at the spell's own level, one entry per type ("2d8", "necrótico"). */
  readonly damage?: readonly { readonly dice: string; readonly typePt: string }[];
  /** Content keys of the classes whose list has the spell. */
  readonly classKeys?: readonly string[];
  /** The table retired it: only the master still reads it. */
  readonly archived?: boolean;
}
