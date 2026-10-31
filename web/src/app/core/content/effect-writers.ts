import { metersToFeet } from '../units';
import { type EffectDraft, type EffectInit } from './effect-draft.types';

/** The tags of "against:magic, about:items": trimmed, empty ones dropped. */
export function splitTags(text: string): string[] {
  return text
    .split(',')
    .map((t) => t.trim())
    .filter((t) => t !== '');
}

/** The feet of what a range field holds ("18" or "4,5" metres); 0 for an empty or unreadable field. */
export function rangeFeet(text: string): number {
  const n = Number(text.trim().replace(',', '.'));
  return Number.isFinite(n) && n > 0 ? metersToFeet(n) : 0;
}

type EffectWriter = (d: EffectDraft) => EffectInit;

type NumberKey =
  'count' | 'healPercentMax' | 'diceSides' | 'rangeMultiplier' | 'attackBonus' | 'damageBonus';

/** A whole number that is sent when above 0. */
const positiveNumber =
  (key: NumberKey): EffectWriter =>
  (d) =>
    d[key] > 0 ? ({ [key]: d[key] } as EffectInit) : {};

/** Like positiveNumber, for a field that may be negative (a bonus): any value but 0 is sent, and an unreadable one (NaN) is not. */
const signedNumber =
  (key: NumberKey): EffectWriter =>
  (d) =>
    d[key] !== 0 && !Number.isNaN(d[key]) ? ({ [key]: d[key] } as EffectInit) : {};

/** A text the server reads trimmed; an empty one is not sent. */
const trimmedText =
  (
    key:
      | 'value'
      | 'when'
      | 'resource'
      | 'max'
      | 'textPt'
      | 'cost'
      | 'costMax'
      | 'valuePerPoint'
      | 'heal'
      | 'tempHp'
      | 'restore'
      | 'fromLevel'
      | 'uses'
      | 'stateName'
      | 'durationRounds'
      | 'weaponName'
      | 'attackName'
      | 'damageDice'
      | 'dice',
  ): EffectWriter =>
  (d) => {
    const text = d[key].trim();
    return text ? ({ [key]: text } as EffectInit) : {};
  };

/** A choice that is sent when something is picked. */
const pickedText =
  (
    key:
      | 'target'
      | 'mode'
      | 'proficiency'
      | 'level'
      | 'roll'
      | 'sense'
      | 'recharge'
      | 'choice'
      | 'economy'
      | 'applies'
      | 'ability'
      | 'except'
      | 'while'
      | 'event'
      | 'spell'
      | 'damageType'
      | 'weapon'
      | 'frequency'
      | 'appliesTo'
      | 'spellChange'
      | 'kind'
      | 'againstCondition'
      | 'attackAbility'
      | 'damageAbility',
  ): EffectWriter =>
  (d) =>
    d[key] ? ({ [key]: d[key] } as EffectInit) : {};

/** A list that is sent (as a copy) when it has something in it. */
const pickedList =
  (key: 'targets' | 'from' | 'spells' | 'damageTypes' | 'conditions' | 'endsOn'): EffectWriter =>
  (d) =>
    d[key].length > 0 ? ({ [key]: [...d[key]] } as EffectInit) : {};

/** How each field the menu names goes into the request, by the field's proto name. ranged_attack's switches and its multiplier
 * are only sent when on (or set). A name the table does not know writes nothing. */
export const EFFECT_WRITERS: ReadonlyMap<string, EffectWriter> = new Map<string, EffectWriter>([
  ['target', pickedText('target')],
  ['mode', pickedText('mode')],
  ['value', trimmedText('value')],
  ['when', trimmedText('when')],
  [
    'tags',
    (d) => {
      const tags = splitTags(d.tags);
      return tags.length > 0 ? { tags } : {};
    },
  ],
  ['proficiency', pickedText('proficiency')],
  ['level', pickedText('level')],
  ['roll', pickedText('roll')],
  ['targets', pickedList('targets')],
  ['sense', pickedText('sense')],
  ['range_ft', (d) => (rangeFeet(d.rangeM) > 0 ? { rangeFt: rangeFeet(d.rangeM) } : {})],
  ['resource', trimmedText('resource')],
  ['max', trimmedText('max')],
  ['recharge', pickedText('recharge')],
  ['choice', pickedText('choice')],
  ['count', positiveNumber('count')],
  ['from', pickedList('from')],
  ['economy', pickedText('economy')],
  ['applies', pickedText('applies')],
  ['ability', pickedText('ability')],
  ['spells', pickedList('spells')],
  ['text_pt', trimmedText('textPt')],
  ['except', pickedText('except')],
  ['while', pickedText('while')],
  ['cost', trimmedText('cost')],
  ['cost_max', trimmedText('costMax')],
  ['value_per_point', trimmedText('valuePerPoint')],
  ['heal', trimmedText('heal')],
  ['temp_hp', trimmedText('tempHp')],
  ['event', pickedText('event')],
  ['restore', trimmedText('restore')],
  ['heal_percent_max', positiveNumber('healPercentMax')],
  ['spell', pickedText('spell')],
  ['from_level', trimmedText('fromLevel')],
  ['uses', trimmedText('uses')],
  ['also_with_slot', (d) => (d.alsoWithSlot ? { alsoWithSlot: true } : {})],
  ['ritual_only', (d) => (d.ritualOnly ? { ritualOnly: true } : {})],
  ['counts_as_known', (d) => (d.countsAsKnown ? { countsAsKnown: true } : {})],
  ['kind', pickedText('kind')],
  ['damage_types', pickedList('damageTypes')],
  ['conditions', pickedList('conditions')],
  ['against_condition', pickedText('againstCondition')],
  ['dice', trimmedText('dice')],
  ['dice_sides', positiveNumber('diceSides')],
  ['damage_type', pickedText('damageType')],
  ['weapon', pickedText('weapon')],
  ['frequency', pickedText('frequency')],
  ['no_crit_doubling', (d) => (d.noCritDoubling ? { noCritDoubling: true } : {})],
  ['applies_to', pickedText('appliesTo')],
  ['spell_change', pickedText('spellChange')],
  ['state_name', trimmedText('stateName')],
  ['duration_rounds', trimmedText('durationRounds')],
  ['ends_on', pickedList('endsOn')],
  ['weapon_name', trimmedText('weaponName')],
  ['attack_ability', pickedText('attackAbility')],
  ['damage_ability', pickedText('damageAbility')],
  ['attack_bonus', signedNumber('attackBonus')],
  ['damage_bonus', signedNumber('damageBonus')],
  ['reach_ft', (d) => (rangeFeet(d.reachM) > 0 ? { reachFt: rangeFeet(d.reachM) } : {})],
  ['attack_name', trimmedText('attackName')],
  ['damage_dice', trimmedText('damageDice')],
  ['ignore_cover', (d) => (d.ignoreCover ? { ignoreCover: true } : {})],
  [
    'only_proficient',
    (d) => (onlyProficientApplies(d) && d.onlyProficient ? { onlyProficient: true } : {}),
  ],
  ['range_multiplier', positiveNumber('rangeMultiplier')],
  [
    'no_long_range_disadvantage',
    (d) => (d.noLongRangeDisadvantage ? { noLongRangeDisadvantage: true } : {}),
  ],
]);

/** Whether "Só onde já tem proficiência" means something for the effect: it doubles a skill, so it goes with the level
 * "Especialização" on a skill ("skill:*" is every skill). The editor shows it only then, and the request sends it only then. */
export function onlyProficientApplies(d: Pick<EffectDraft, 'level' | 'proficiency'>): boolean {
  return d.level === 'expertise' && d.proficiency.startsWith('skill:');
}
