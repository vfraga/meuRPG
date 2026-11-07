import { Ability } from '../../../gen/meurpg/rules/v1/rules_pb';
import type { MessageInitShape } from '@bufbuild/protobuf';

import type {
  TableSpellEffect,
  TableSpellSchema,
  TableSpellZone,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import type { SummonDraft } from './summon-draft';

/**
 * The spell editor's form (MR-025, RN-23, E10-01 states 4, 4b and 5) and what it sends. A table spell is a
 * `TableSpell`: the same details an SRD spell has, so the combat that resolves an SRD spell resolves this one
 * (ADR-0018). The mechanic is optional: "Só texto" is always valid. Distances are typed in metres, in steps of 1,5 m,
 * and stored in feet (5 ft = 1,5 m); the server checks every limit, the form only keeps what the person typed.
 */

export type SpellInit = MessageInitShape<typeof TableSpellSchema>;

export type RangeChoice = 'ranged' | 'self' | 'touch' | 'sight' | 'unlimited';
export type DurationChoice = 'instant' | 'timed' | 'until_dispelled';
export type TargetChoice = 'creature' | 'creatures' | 'area' | 'self';
export type MechanicChoice = 'text' | 'attack' | 'save' | 'heal' | 'summon';
export type TimeChoice = 'action' | 'bonus_action' | 'reaction' | 'minute' | 'hour';

/** The number fields typed as whole numbers: a NaN in the draft marks text that is no whole number. */
export type SpellCountField = 'count' | 'perSlot' | 'timeAmount' | 'durationAmount';

export interface SpellDraft {
  name: string;
  /** 0 is a truque. */
  level: number;
  schoolKey: string;
  time: TimeChoice;
  timeAmount: number;
  trigger: string;
  range: RangeChoice;
  rangeM: string;
  duration: DurationChoice;
  durationAmount: number;
  durationUnit: 'round' | 'minute' | 'hour' | 'day';
  upTo: boolean;
  verbal: boolean;
  somatic: boolean;
  material: boolean;
  materialPt: string;
  concentration: boolean;
  ritual: boolean;
  classKeys: string[];
  text: string;
  higher: string;
  target: TargetChoice;
  count: number;
  perSlot: number;
  shape: 'cone' | 'cube' | 'cylinder' | 'line' | 'sphere';
  sizeM: string;
  mechanic: MechanicChoice;
  attack: 'melee' | 'ranged';
  saveAbility: Ability;
  saveOnSuccess: 'half' | 'none';
  dice: string;
  damageType: string;
  /** "+1d8 por nível acima do 1º" (a leveled spell) or at the cantrip tiers (a truque). */
  more: string;
  healDice: string;
  healMore: string;
  healModifier: boolean;
  /** Damage entries after the first, kept as they are (the form edits one). */
  otherDamage: readonly OtherDamage[];
  /** What was typed in a count field when it is not how the number is written back ("+2", "007", text no number); never sent. */
  typed?: Partial<Record<SpellCountField, string>>;
  /** What "Invocar" edits (#578); only sent while that mechanic is chosen, kept here so choosing it again gives the creatures back. */
  summon: SummonDraft | null;
  /** The target and the range "Invocar" replaced with the caster's: given back when the mechanic is left and they are still the caster's. */
  beforeSummon?: { target: TargetChoice; range: RangeChoice };
  /** What the form has no control for: the stored effect and zone go back as they were loaded. */
  kept?: KeptSpellFields;
}

/** The parts of a spell the editor cannot show yet; saving sends them untouched. */
export interface KeptSpellFields {
  readonly effect?: TableSpellEffect;
  readonly zone?: TableSpellZone;
}

export interface OtherDamage {
  readonly damageTypeKey: string;
  readonly dice: string;
  readonly perSlotLevel: string;
  readonly perTier: string;
}

export function emptySpell(): SpellDraft {
  return {
    name: '',
    level: 1,
    schoolKey: 'school:evocation',
    time: 'action',
    timeAmount: 1,
    trigger: '',
    range: 'ranged',
    rangeM: '',
    duration: 'instant',
    durationAmount: 1,
    durationUnit: 'minute',
    upTo: false,
    verbal: true,
    somatic: true,
    material: false,
    materialPt: '',
    concentration: false,
    ritual: false,
    classKeys: [],
    text: '',
    higher: '',
    target: 'creature',
    count: 2,
    perSlot: 0,
    shape: 'cone',
    sizeM: '',
    mechanic: 'text',
    attack: 'ranged',
    saveAbility: Ability.DEXTERITY,
    saveOnSuccess: 'half',
    dice: '',
    damageType: 'damage-type:necrotic',
    more: '',
    healDice: '',
    healMore: '',
    healModifier: true,
    otherDamage: [],
    summon: null,
  };
}
