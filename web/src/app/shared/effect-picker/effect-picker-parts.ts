import {
  type EffectDraft,
  type EffectMenuVm,
  UNREADABLE_RANGE,
} from '../../core/content/effect-draft';
import { UNREADABLE_BONUS, UNREADABLE_COUNT } from '../../core/content/whole-number';
import {
  CRITICAL_COPY,
  EFFECT_FIELD_LABELS,
  EFFECT_REFUSAL_TEXTS,
  EFFECT_TYPE_FIELD_LABELS,
  RESOURCE_PICK_COPY,
} from '../../core/content/effect-field-copy';
import { GENERIC_EFFECT_TEXTS } from '../../core/content/content-violation-texts';
import { CRITICAL_RANGE_TARGET, isScoreTarget } from '../../core/content/effect-visible';
import type { EffectMenuField } from '../../../gen/meurpg/rules/v1/table_content_pb';
import { capitalized } from '../../core/creatures/bestiary-format';
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

/** The first limit of a score modifier: the highest a normal score goes. */
export const SCORE_CAP_DEFAULT = 20;

/** The mode a modifier target fixes (the critical range is "set", a score "add"), or '' when it takes every mode. */
export function fixedMode(menu: EffectMenuVm, target: string): string {
  const only: readonly string[] = menu.optionOf('modifier_targets', target)?.onlyModes ?? [];
  if (only.length === 0 && isScoreTarget(target)) {
    // The pick of the feat's ability ("score.@habilidade") is no entry of the menu, and is a score all the same.
    return 'add';
  }
  return only.length === 1 ? only[0] : '';
}

/** The effect after its target changed: a target that fixes its mode sets it, only the critical range keeps a scope and only a score a limit. */
export function afterTarget(menu: EffectMenuVm, d: EffectDraft): EffectDraft {
  const score = isScoreTarget(d.target);
  return {
    ...d,
    mode: fixedMode(menu, d.target) || d.mode,
    applies: d.target === CRITICAL_RANGE_TARGET ? d.applies : '',
    cap: score ? d.cap || SCORE_CAP_DEFAULT : 0,
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

/** The closed list's value that the menu no longer lists (an engine that dropped it), as the option that keeps it on the select, or
 * `null`. Only the menu's own lists say it: the spells come from the table's catalog, which may not be loaded yet, and an empty
 * list says nothing. */
export function leftTheMenuChoice(
  field: { name: string; kind: string },
  value: string,
  list: readonly { value: string }[],
): SelectOption | null {
  const own = field.kind === 'choice' && field.name !== 'spell' && field.name !== 'spells';
  if (!own || value === '' || list.length === 0 || list.some((o) => o.value === value)) {
    return null;
  }
  return {
    value,
    label: field.name === 'target' ? 'Um alvo que saiu do menu' : 'Um valor que saiu do menu',
  };
}

/** A choice with a placeholder when it is required, and a real "Padrão" (the field left empty) when it is not. A value the menu no
 * longer has stays on the select, said in words, so the field does not show another one in its place. */
export function choiceList(
  list: SelectOption[],
  kept: SelectOption | null,
  required: boolean,
  type: string,
): SelectOption[] {
  const all = kept ? [kept, ...list] : list;
  return required
    ? all
    : [{ value: '', label: type === 'roll_minimum' ? 'Qualquer tipo' : 'Padrão' }, ...all];
}

/** The hint of a closed list: the picked option's, the entry's own, or what to do about the stored code the menu no longer has. */
export function choiceHint(
  list: readonly (SelectOption & { hint: string })[],
  field: { name: string; kind: string },
  value: string,
  featHint: string,
  own: string,
): string {
  const picked = list.find((o) => o.value === value)?.hint;
  const kept = leftTheMenuChoice(field, value, list) ? value : '';
  return (
    picked || featHint || (kept ? `O valor guardado (${kept}) não existe nas regras novas.` : own)
  );
}

/** Whether a draft value holds something: a non-empty list, a number but zero, text but blanks, or `true`. */
export function holdsSomething(v: unknown): boolean {
  if (Array.isArray(v)) {
    return v.length > 0;
  }
  if (typeof v === 'number') {
    return v !== 0;
  }
  return typeof v === 'string' ? v.trim() !== '' : v === true;
}

const OWN_HINT_REFS: ReadonlySet<string> = new Set(['restore', 'uses']);

const STATE_RESOURCE_HINT =
  'O nome de um recurso que esta entrada define (o campo "Nome do recurso" de um efeito Recurso). Cada vez que o estado liga, gasta 1 uso; sem usos, o cartão fica desligado.';

const RESOURCE_REF_HINT =
  'O nome de um recurso que esta entrada define (o campo "Nome do recurso" de um efeito Recurso). Gasta o custo (1 se vazio); sem usos o bastante, a ação fica desligada.';

/** The hint of a field that names a resource or a state of the entry. */
export function refHint(
  e: EffectDraft,
  menu: EffectMenuVm,
  f: EffectMenuField,
  own: string,
): string {
  if (f.kind === 'state_ref') {
    // The name of a state effect of the entry, and the ready values the menu lists ("rage").
    const ready = menu.list(f.list).map((o) => `${o.key} (${o.namePt})`);
    const named =
      'O nome de um estado que esta entrada define (o campo "Nome do estado" de um efeito Estado).';
    return ready.length > 0 ? `${named} Também vale: ${ready.join(', ')}.` : named;
  }
  // `uses` has its own sentence (an empty resource is at will, not a disabled action), as `restore` does
  if (e.type === 'state') {
    return STATE_RESOURCE_HINT;
  }
  return OWN_HINT_REFS.has(f.name) ? own : RESOURCE_REF_HINT;
}

/** The labels of a hook's two resource fields: the one it spends and the one it gives back. */
const RESOURCE_REF_LABEL_HOOK: Readonly<Record<string, string>> = {
  resource: 'Gasta usos de',
  restore: 'Devolve usos de',
};

/** The label of a field: the critical range's own, the type's own wording, the resource fields', else the field's. */
export function fieldLabel(e: EffectDraft, f: EffectMenuField): string {
  if (f.name === 'value' && isCriticalRange(e)) {
    return CRITICAL_COPY.value;
  }
  const typed = EFFECT_TYPE_FIELD_LABELS[e.type]?.[f.name];
  if (typed) {
    return typed;
  }
  // The resource an action spends is the same proto field as a resource's own name; a hook's `restore` gives back.
  if (f.kind === 'resource_ref') {
    return e.type === 'on_event'
      ? (RESOURCE_REF_LABEL_HOOK[f.name] ?? 'Gasta usos de')
      : 'Gasta usos de';
  }
  return EFFECT_FIELD_LABELS[f.name] ?? f.name;
}

/** The hint a field has whatever the type: the bounds of a count or a limit, the tag prefixes, the critical range's own line; `undefined` for the rest. */
export function fixedHint(
  e: EffectDraft,
  menu: EffectMenuVm,
  f: EffectMenuField,
): string | undefined {
  if (f.name === 'count' && f.max > 0) {
    return `De ${f.min} a ${f.max}.`;
  }
  if (f.name === 'cap') {
    return `De ${f.min} a ${f.max}. A habilidade sobe até ele e nunca passa.`;
  }
  if (f.name === 'tags') {
    return menu.tagHint();
  }
  if (f.name === 'value' && isCriticalRange(e)) {
    return CRITICAL_COPY.valueHint;
  }
  return undefined;
}

/** What a number field says when its text reads as no whole number the server stores. */
export function unreadableText(f: EffectMenuField, inMeters: boolean): string {
  if (inMeters) {
    return UNREADABLE_RANGE;
  }
  return f.min < 0 ? UNREADABLE_BONUS : UNREADABLE_COUNT;
}

/** Why "Como muda" cannot change: the critical range sets, a score only adds, any other target fixes it. */
export function modeLockNote(e: EffectDraft): string {
  if (isCriticalRange(e)) {
    return CRITICAL_COPY.modeLock;
  }
  return isScoreTarget(e.target) ? 'Uma habilidade só soma.' : 'Este alvo só aceita este modo.';
}

/** How a field is drawn when the menu's kind is not the whole story: the rolls as check rows, a reroll's resource from the entry's own, the critical range's scope as cards. */
export function fieldView(type: string, f: EffectMenuField, ownResources: boolean): string {
  if (f.name === 'rolls' && f.kind === 'choices') {
    return 'rolls';
  }
  if (f.kind === 'resource_ref' && type === 'reroll' && ownResources) {
    return 'uses';
  }
  return f.name === 'applies' && type === 'modifier' ? 'cards' : f.kind;
}

/** The options of a closed list the menu names, each with the menu's one-line explanation; the damage types in sentence case. */
export function listOptions(
  menu: EffectMenuVm,
  effect: EffectDraft,
  f: EffectMenuField,
  spells: readonly SelectOption[],
): (SelectOption & { hint: string })[] {
  if (f.name === 'spell') {
    return spells.map((o) => ({ ...o, hint: '' }));
  }
  const swap = f.name === 'from' && f.list === '';
  const list = swap
    ? effect.type === 'spell_modifier'
      ? menu.list('damage_types')
      : menu.fromOptions(effect.choice)
    : f.name === 'spells'
      ? []
      : menu.list(f.list);
  // The server writes a damage type in lower case ("fogo"); a list of them reads in sentence case, as every label does.
  const damage =
    f.list === 'damage_types' ||
    (swap && (effect.type === 'spell_modifier' || effect.choice === 'damage_type'));
  return list.map((o) => ({
    value: o.key,
    label: damage ? capitalized(o.namePt) : o.namePt,
    hint: o.hintPt,
  }));
}
