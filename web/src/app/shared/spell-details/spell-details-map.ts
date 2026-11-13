import { create } from '@bufbuild/protobuf';

import {
  SpellSchema,
  CastingTimeUnit as GenCastingTimeUnit,
  SpellAttackType as GenSpellAttackType,
  SpellDetails as GenSpellDetails,
  SpellDurationKind as GenSpellDurationKind,
  SpellDurationUnit as GenSpellDurationUnit,
  SpellRangeKind as GenSpellRangeKind,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import {
  CastingTimeUnitKey,
  DurationKindKey,
  DurationUnitKey,
  RangeKindKey,
  SpellDetailsVm,
} from './spell-details.types';

/**
 * The generated `SpellDetails` as the gen-free view-model the "?" sheet reads.
 * It lives next to the sheet, so the editor and the session both use it.
 */

const CASTING_UNIT_FROM_GEN: Record<GenCastingTimeUnit, CastingTimeUnitKey> = {
  [GenCastingTimeUnit.UNSPECIFIED]: '',
  [GenCastingTimeUnit.ACTION]: 'action',
  [GenCastingTimeUnit.BONUS_ACTION]: 'bonus_action',
  [GenCastingTimeUnit.REACTION]: 'reaction',
  [GenCastingTimeUnit.MINUTE]: 'minute',
  [GenCastingTimeUnit.HOUR]: 'hour',
};

const RANGE_KIND_FROM_GEN: Record<GenSpellRangeKind, RangeKindKey> = {
  [GenSpellRangeKind.UNSPECIFIED]: '',
  [GenSpellRangeKind.SELF]: 'self',
  [GenSpellRangeKind.TOUCH]: 'touch',
  [GenSpellRangeKind.RANGED]: 'ranged',
  [GenSpellRangeKind.SIGHT]: 'sight',
  [GenSpellRangeKind.UNLIMITED]: 'unlimited',
  [GenSpellRangeKind.SPECIAL]: 'special',
};

const DURATION_KIND_FROM_GEN: Record<GenSpellDurationKind, DurationKindKey> = {
  [GenSpellDurationKind.UNSPECIFIED]: '',
  [GenSpellDurationKind.INSTANTANEOUS]: 'instantaneous',
  [GenSpellDurationKind.TIMED]: 'timed',
  [GenSpellDurationKind.UNTIL_DISPELLED]: 'until_dispelled',
  [GenSpellDurationKind.SPECIAL]: 'special',
};

const DURATION_UNIT_FROM_GEN: Record<GenSpellDurationUnit, DurationUnitKey> = {
  [GenSpellDurationUnit.UNSPECIFIED]: '',
  [GenSpellDurationUnit.ROUND]: 'round',
  [GenSpellDurationUnit.MINUTE]: 'minute',
  [GenSpellDurationUnit.HOUR]: 'hour',
  [GenSpellDurationUnit.DAY]: 'day',
};

/** The dice at the lowest level a map names: the spell's own circle (or the character's first level). */
function firstDice(byLevel: Record<number, string>): string {
  const levels = Object.keys(byLevel).map(Number);
  return levels.length > 0 ? byLevel[Math.min(...levels)] : '';
}

/** A spell of the table's own: its key ends in "@mesa" (the one place that says so). */
export function isTableSpellKey(key: string): boolean {
  return key.endsWith('@mesa');
}

function castingTimeOf(c: GenSpellDetails['castingTime']): SpellDetailsVm['castingTime'] {
  return {
    amount: c?.amount ?? 0,
    unit: CASTING_UNIT_FROM_GEN[c?.unit ?? GenCastingTimeUnit.UNSPECIFIED],
    trigger: c?.trigger ?? '',
    raw: c?.raw ?? '',
  };
}

function rangeOf(r: GenSpellDetails['range']): SpellDetailsVm['range'] {
  return {
    kind: RANGE_KIND_FROM_GEN[r?.kind ?? GenSpellRangeKind.UNSPECIFIED],
    distanceFt: r?.distanceFt ?? 0,
    raw: r?.raw ?? '',
  };
}

function componentsOf(c: GenSpellDetails['components']): SpellDetailsVm['components'] {
  return {
    verbal: c?.verbal ?? false,
    somatic: c?.somatic ?? false,
    material: c?.material ?? false,
    materialText: c?.materialText ?? '',
    materialTextPt: c?.materialTextPt ?? '',
  };
}

function durationOf(d: GenSpellDetails['duration']): SpellDetailsVm['duration'] {
  return {
    kind: DURATION_KIND_FROM_GEN[d?.kind ?? GenSpellDurationKind.UNSPECIFIED],
    amount: d?.amount ?? 0,
    unit: DURATION_UNIT_FROM_GEN[d?.unit ?? GenSpellDurationUnit.UNSPECIFIED],
    upTo: d?.upTo ?? false,
    concentration: d?.concentration ?? false,
    raw: d?.raw ?? '',
  };
}

const ATTACK_FROM_GEN: Partial<Record<GenSpellAttackType, SpellDetailsVm['attack']>> = {
  [GenSpellAttackType.MELEE]: 'melee',
  [GenSpellAttackType.RANGED]: 'ranged',
};

function damageOf(damage: GenSpellDetails['damage']): SpellDetailsVm['damage'] {
  return damage
    .map((x) => ({
      dice: firstDice(x.bySlotLevel) || firstDice(x.byCharacterLevel),
      typePt: x.damageTypePt,
    }))
    .filter((x) => x.dice !== '');
}

/** What the catalog row says of the spell itself (the whole row may be missing: its defaults are the empty spell's). */
function identityOf(row: GenSpellDetails['spell']) {
  const spell = row ?? create(SpellSchema);
  return {
    key: spell.key,
    namePt: spell.namePt,
    nameEn: spell.name,
    level: spell.level,
    schoolNamePt: spell.schoolNamePt,
    ritual: spell.ritual,
    concentration: spell.concentration,
    classKeys: spell.classKeys,
    archived: spell.archived,
    table: isTableSpellKey(spell.key),
  };
}

/**
 * The details of a spell cast through a grant that waives the material: the component line drops the
 * "M" and its text, and names the grant that waives it.
 */
export function withoutMaterial(vm: SpellDetailsVm, grantName: string): SpellDetailsVm {
  return {
    ...vm,
    components: { ...vm.components, material: false, materialWaivedBy: grantName },
  };
}

export function spellDetailsFromGen(d: GenSpellDetails): SpellDetailsVm {
  return {
    ...identityOf(d.spell),
    castingTime: castingTimeOf(d.castingTime),
    range: rangeOf(d.range),
    components: componentsOf(d.components),
    duration: durationOf(d.duration),
    description: d.description,
    higherLevel: d.higherLevel,
    descriptionPt: d.descriptionPt,
    higherLevelPt: d.higherLevelPt,
    textPtMissing: d.textPtMissing,
    textPtOnly: d.textPtOnly,
    targetLabel: d.target?.labelPt ?? '',
    attack: ATTACK_FROM_GEN[d.attackType] ?? 'none',
    damage: damageOf(d.damage),
  };
}
