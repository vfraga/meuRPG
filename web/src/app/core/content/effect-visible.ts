import type { EffectMenuField } from '../../../gen/meurpg/rules/v1/table_content_pb';
import { onlyProficientApplies } from './effect-writers';

/** What `visibleFields` reads of a draft. */
interface Shape {
  readonly type: string;
  readonly level: string;
  readonly proficiency: string;
  readonly applies: string;
  readonly spellChange: string;
  readonly target?: string;
  readonly rolls?: readonly string[];
  readonly resource?: string;
  readonly cost?: string;
  readonly damageType?: string;
  readonly cap?: number;
  readonly uses?: string;
  readonly usesCost?: number;
}

/** The modifier target whose scope ("Vale para") is asked: the critical range, the only target that reads `applies`. */
export const CRITICAL_RANGE_TARGET = 'critical_range';

/** Whether a modifier target is an ability score ("score.str", or the pick "score.@habilidade"): the only one that has a limit. */
export function isScoreTarget(target: string): boolean {
  return target.startsWith('score.');
}

/** The fields of a reroll, a roll minimum or a modifier that apply now. A field that holds a value stays, so a refusal of it has
 * somewhere to land: the damage type of a minimum on rolls other than damage alone, the cost of a reroll with no resource. */
function rollFieldsHidden(d: Shape): readonly string[] {
  if (d.type === 'roll_minimum') {
    const only = d.rolls?.length === 1 && d.rolls[0] === 'damage';
    return only || (d.damageType ?? '') !== '' ? [] : ['damage_type'];
  }
  if (d.type === 'reroll') {
    return (d.resource ?? '') !== '' || (d.cost ?? '') !== '' ? [] : ['cost'];
  }
  if (d.type === 'granted_spell') {
    return grantFieldsHidden(d);
  }
  return d.type === 'modifier' ? modifierFieldsHidden(d) : [];
}

/** A granted spell asks the uses each cast spends only when it spends a resource (or already holds a cost, so a refusal has somewhere to land). */
function grantFieldsHidden(d: Shape): readonly string[] {
  const resource = (d.uses ?? '').trim();
  return (resource !== '' && resource !== 'at_will') || (d.usesCost ?? 0) !== 0
    ? []
    : ['uses_cost'];
}

/** A modifier asks the scope only for the critical range and the limit only for a score (or where one is already typed). */
function modifierFieldsHidden(d: Shape): readonly string[] {
  return [
    ...(d.target === CRITICAL_RANGE_TARGET ? [] : ['applies']),
    ...(isScoreTarget(d.target ?? '') || (d.cap ?? 0) !== 0 ? [] : ['cap']),
  ];
}

/** What a spell_modifier's change reads besides the fields every change has: the swap lists the types it swaps among, the bonus
 * damage its amount and type, the extra dice their count. The other fields stay out of the editor (and of the request). */
const SPELL_CHANGE_FIELDS: Readonly<Record<string, readonly string[]>> = {
  damage_type_swap: ['from'],
  bonus_damage: ['value', 'damage_type'],
  extra_dice_per_slot_over: ['count'],
};
const SPELL_CHANGE_ONLY: ReadonlySet<string> = new Set(Object.values(SPELL_CHANGE_FIELDS).flat());

/** A superiority_die on a roll of the character's own (checks, saves, initiative) reads fewer fields: only the checks take a
 * `targets` list, only a save an `ability`, and none of the three costs an action. */
const ROLL_DIE_HIDES: Readonly<Record<string, readonly string[]>> = {
  check: ['ability', 'economy'],
  save: ['targets', 'economy'],
  initiative: ['ability', 'targets', 'economy'],
};

/** The fields of the draft's type that apply to it now: a spell_modifier shows only what its chosen change reads (none of the
 * change's own fields until one is chosen), a superiority_die on a roll only what that roll reads; every other type shows all
 * of its menu fields. */
export function visibleFields(
  fields: readonly EffectMenuField[],
  d: Shape,
): readonly EffectMenuField[] {
  if (d.type === 'superiority_die') {
    const hidden = ROLL_DIE_HIDES[d.applies] ?? ['targets'];
    return fields.filter((f) => !hidden.includes(f.name));
  }
  if (d.type !== 'spell_modifier') {
    const hidden = rollFieldsHidden(d);
    return fields.filter(
      (f) => !hidden.includes(f.name) && (f.name !== 'only_proficient' || onlyProficientApplies(d)),
    );
  }
  const own = SPELL_CHANGE_FIELDS[d.spellChange] ?? [];
  return fields.filter((f) => !SPELL_CHANGE_ONLY.has(f.name) || own.includes(f.name));
}

/** The optional fields the editor never folds under "Mais opções": what a reroll spends and costs, the damage type of a minimum, the
 * scope of the critical range. Each is in `visibleFields` only when it applies, so showing it is showing what the effect reads. */
const NEVER_FOLDED: Readonly<Record<string, readonly string[]>> = {
  reroll: ['resource', 'cost'],
  roll_minimum: ['damage_type'],
  modifier: ['applies'],
  granted_spell: ['uses_cost'],
};

export function neverFolded(type: string, name: string): boolean {
  return NEVER_FOLDED[type]?.includes(name) ?? false;
}
