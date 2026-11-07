import { type EffectDraft, type EffectMenuVm } from '../../core/content/effect-draft';
import { EFFECT_REFUSAL_TEXTS, RESOURCE_PICK_COPY } from '../../core/content/effect-field-copy';
import { GENERIC_EFFECT_TEXTS } from '../../core/content/content-violation-texts';
import { CRITICAL_RANGE_TARGET } from '../../core/content/effect-visible';
import type { SelectOption } from '../form-fields/select-field';

/**
 * The parts of the effect picker that are plain functions of the draft and the menu: the resources of the entry a reroll may
 * spend, the mode a target fixes, and what a refusal says of a field several types share.
 */

/** "sorte_teimosa" as a person says it: "Sorte Teimosa". */
function titled(key: string): string {
  return key
    .split('_')
    .filter((w) => w !== '')
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
    .join(' ');
}

/** "Sorte Teimosa (3 por descanso longo)": a resource the entry defines, with its uses and when they come back. */
function resourceLabel(r: EffectDraft, menu: EffectMenuVm): string {
  const max = r.max.trim();
  const when = r.recharge === '' ? '' : menu.nameOf('recharges', r.recharge).toLowerCase();
  if (max === '') {
    return titled(r.resource);
  }
  const rest = r.recharge === 'short_rest' || r.recharge === 'long_rest';
  const uses = when === '' ? max : `${max}${rest ? ' por ' : ', '}${when}`;
  return `${titled(r.resource)} (${uses})`;
}

/** What "Gasta usos de" offers a reroll. */
export interface ResourcePick {
  readonly options: SelectOption[];
  /** The field is a list: the entry defines a resource, and what is picked is "Nenhum" or one of them. Otherwise it is a text, because the
   * server also takes the resources of the class and of the SRD, which the picker does not list. */
  readonly list: boolean;
}

/** The entry's own resource effects (the siblings of the effect), with "Nenhum" first. */
export function resourcePick(
  siblings: readonly EffectDraft[],
  menu: EffectMenuVm,
  current: string,
): ResourcePick {
  const options: SelectOption[] = [{ value: '', label: RESOURCE_PICK_COPY.none }];
  const seen = new Set<string>();
  for (const r of siblings.filter((e) => e.type === 'resource')) {
    const key = r.resource.trim();
    if (key !== '' && !seen.has(key)) {
      seen.add(key);
      options.push({ value: key, label: resourceLabel(r, menu) });
    }
  }
  const name = current.trim();
  return { options, list: seen.size > 0 && (name === '' || seen.has(name)) };
}

/** The mode a modifier target fixes (the critical range is "set"), or '' when it takes every mode. */
export function fixedMode(menu: EffectMenuVm, target: string): string {
  const only: readonly string[] = menu.optionOf('modifier_targets', target)?.onlyModes ?? [];
  return only.length === 1 ? only[0] : '';
}

/** The effect after its target changed: a target that fixes its mode sets it, and only the critical range keeps a scope. */
export function afterTarget(menu: EffectMenuVm, d: EffectDraft): EffectDraft {
  return {
    ...d,
    mode: fixedMode(menu, d.target) || d.mode,
    applies: d.target === CRITICAL_RANGE_TARGET ? d.applies : '',
  };
}

/** Whether the critical range is the modifier's target. */
export function isCriticalRange(e: Pick<EffectDraft, 'type' | 'target'>): boolean {
  return e.type === 'modifier' && e.target === CRITICAL_RANGE_TARGET;
}

/** The refusals of a field with the words of the effect's own type where the server's reason said it generally. */
export function refusalTexts(
  e: EffectDraft,
  field: string,
  texts: readonly string[],
): readonly string[] {
  const type = isCriticalRange(e) ? `modifier.${CRITICAL_RANGE_TARGET}` : e.type;
  const own = EFFECT_REFUSAL_TEXTS[`${type}.${field}`];
  return own ? texts.map((t) => (GENERIC_EFFECT_TEXTS.has(t) ? own : t)) : texts;
}
