import type { EffectMenuField } from '../../../gen/meurpg/rules/v1/table_content_pb';
import type { EffectDraft } from './effect-draft.types';
import { visibleFields } from './effect-visible';

/**
 * The named choices of a feat (RN-23, MR-025): a `choice` effect of kind ability or damage type with a `name`. Later effects
 * of the same feat write `@name` for the pick. The rules are the server's (`rules/picks.go`, `rules/feat_choices.go`).
 */

export type ChoiceKind = 'ability' | 'damage_type';

export interface FeatChoice {
  kind: ChoiceKind;
  /** The master's name, the part after the "@". */
  name: string;
  /** The place of this choice among the feat's choices of its kind, from 1. */
  number: number;
  /** How many choices of its kind the feat has. */
  ofKind: number;
  /** "Habilidade" or "Tipo de dano", numbered "(1ª)" or "(1º)" only when the feat has two or more of the kind. */
  label: string;
  /** Where the choice is in the feat's effects. */
  index: number;
}

export interface FormulaInsertion {
  /** `mod("@name")` or `score("@name")`. */
  text: string;
  /** "Modificador · Habilidade (1ª)". */
  label: string;
}

export interface MissingChoice {
  /** The effect that holds the reference. */
  index: number;
  /** The field's path, "table_feat.effects[2].proficiency". */
  path: string;
  /** The name the field points at, with no "@". */
  name: string;
}

/** What each kind of choice is called, and its default name; the one table the editor reads for its words. */
export const CHOICE_KINDS: Readonly<
  Record<
    ChoiceKind,
    {
      title: string;
      ordinal: string;
      feminine: boolean;
      /** "uma habilidade": what a field that takes the kind asks for. */
      field: string;
      /** The player's pick after its count: "habilidade", "habilidades". */
      words: readonly [string, string];
      defaultName: string;
    }
  >
> = {
  ability: {
    title: 'Habilidade',
    ordinal: 'ª',
    feminine: true,
    field: 'uma habilidade',
    words: ['habilidade', 'habilidades'],
    defaultName: 'habilidade',
  },
  damage_type: {
    title: 'Tipo de dano',
    ordinal: 'º',
    feminine: false,
    field: 'um tipo de dano',
    words: ['tipo de dano', 'tipos de dano'],
    defaultName: 'tipo_de_dano',
  },
};

/** The kind a text names, or null ("ability", "damage_type"; anything else, `feature` included, is none). */
export function choiceKindOf(text: string): ChoiceKind | null {
  return Object.hasOwn(CHOICE_KINDS, text) ? (text as ChoiceKind) : null;
}

function kindOf(e: EffectDraft): ChoiceKind | null {
  return e.type === 'choice' && e.name !== '' ? choiceKindOf(e.choice) : null;
}

/** The feat's named choices in the order of its effects, numbered by kind as the server numbers the level-up questions. */
export function featChoices(effects: readonly EffectDraft[]): FeatChoice[] {
  const found: { kind: ChoiceKind; name: string; index: number }[] = [];
  effects.forEach((e, index) => {
    const kind = kindOf(e);
    if (kind) {
      found.push({ kind, name: e.name, index });
    }
  });
  return found.map((c, at) => {
    const ofKind = found.filter((o) => o.kind === c.kind).length;
    const number = found.slice(0, at + 1).filter((o) => o.kind === c.kind).length;
    const { title, ordinal } = CHOICE_KINDS[c.kind];
    return { ...c, number, ofKind, label: ofKind > 1 ? `${title} (${number}${ordinal})` : title };
  });
}

/** The choices of `kind` that come before the effect at `index`: the only ones its field may point at. */
export function choicesBefore(
  effects: readonly EffectDraft[],
  index: number,
  kind: ChoiceKind,
): FeatChoice[] {
  return featChoices(effects).filter((c) => c.kind === kind && c.index < index);
}

/** The name the editor gives a new choice of `kind` ("habilidade", then "habilidade_2"), so the master never types one; '' for a kind that has no name. */
export function defaultChoiceName(kind: string, taken: readonly string[]): string {
  const known = choiceKindOf(kind);
  if (!known) {
    return '';
  }
  const base = CHOICE_KINDS[known].defaultName;
  let name = base;
  for (let n = 2; taken.includes(name); n++) {
    name = `${base}_${n}`;
  }
  return name;
}

/** What a field writes to point at a choice. */
export function choiceRef(choice: Pick<FeatChoice, 'name'>): string {
  return `@${choice.name}`;
}

/** What a formula may read of a choice: only an ability's modifier or score (the server's `mod` and `score`). */
export function formulaInsertions(choices: readonly FeatChoice[]): FormulaInsertion[] {
  return choices
    .filter((c) => c.kind === 'ability')
    .flatMap((c) => [
      { text: `mod("${choiceRef(c)}")`, label: `Modificador · ${c.label}` },
      { text: `score("${choiceRef(c)}")`, label: `Valor · ${c.label}` },
    ]);
}

/** The ways a pick writes a choice: the text before `@name` ("save.@habilidade", "damage-type:@tipo", plain "@habilidade"). */
export type RefFormId = 'proficiency' | 'save' | 'score' | 'check' | 'ability' | 'damage';

/** A field that offers the feat's choices: the kind it takes and the forms it writes them in, in the order they are offered. */
export interface PickSpec {
  kind: ChoiceKind;
  forms: readonly RefFormId[];
}

const ABILITY_PICK = (...forms: RefFormId[]): PickSpec => ({ kind: 'ability', forms });
const DAMAGE_PICK: PickSpec = { kind: 'damage_type', forms: ['damage'] };

/**
 * The fields that may hold `@name` (`pickFields` of the server), by the proto name, with the draft's key. A field with a `pick`
 * offers the choices of its kind; a field without one is a formula, which reads `mod("@name")` and `score("@name")` of abilities.
 */
export const REFERENCE_FIELDS: readonly {
  name: string;
  key: keyof EffectDraft;
  pick: PickSpec | null;
}[] = [
  { name: 'proficiency', key: 'proficiency', pick: ABILITY_PICK('proficiency') },
  { name: 'target', key: 'target', pick: ABILITY_PICK('score', 'save') },
  { name: 'targets', key: 'targets', pick: ABILITY_PICK('check', 'save') },
  { name: 'ability', key: 'ability', pick: ABILITY_PICK('ability') },
  { name: 'attack_ability', key: 'attackAbility', pick: ABILITY_PICK('ability') },
  { name: 'damage_ability', key: 'damageAbility', pick: ABILITY_PICK('ability') },
  { name: 'damage_type', key: 'damageType', pick: DAMAGE_PICK },
  { name: 'damage_types', key: 'damageTypes', pick: DAMAGE_PICK },
  { name: 'except', key: 'except', pick: DAMAGE_PICK },
  { name: 'value', key: 'value', pick: null },
  { name: 'when', key: 'when', pick: null },
  { name: 'max', key: 'max', pick: null },
  { name: 'dice', key: 'dice', pick: null },
  { name: 'duration_rounds', key: 'durationRounds', pick: null },
  { name: 'from_level', key: 'fromLevel', pick: null },
  { name: 'cost', key: 'cost', pick: null },
  { name: 'cost_max', key: 'costMax', pick: null },
  { name: 'temp_hp', key: 'tempHp', pick: null },
  { name: 'value_per_point', key: 'valuePerPoint', pick: null },
];

/** The pick spec of a field, or null when the field takes no choice as a pick. */
export function pickSpec(field: string): PickSpec | null {
  return REFERENCE_FIELDS.find((f) => f.name === field)?.pick ?? null;
}

/** The names a text points at with `@name`, once each; the table suffix of a key ("feature:x@mesa") is no reference. */
function referencesIn(text: string): string[] {
  const names = [...text.matchAll(/@([a-z0-9_]+)/g)].map((m) => m[1]).filter((n) => n !== 'mesa');
  return [...new Set(names)];
}

function textsOf(e: EffectDraft, key: keyof EffectDraft): string[] {
  const v = e[key];
  if (typeof v === 'string') {
    return [v];
  }
  return Array.isArray(v) ? v.filter((s): s is string => typeof s === 'string') : [];
}

type Visible = { fieldsOf(type: string): readonly EffectMenuField[] };

/** The fields of an effect that hold `@name`, with the names each points at. With `visible`, only the fields the effect's type reads (a field the type does not read is not sent). */
function referencedFields(
  e: EffectDraft,
  visible?: Visible,
): { name: string; references: string[] }[] {
  const shown = visible
    ? new Set(visibleFields(visible.fieldsOf(e.type), e).map((f) => f.name))
    : null;
  return REFERENCE_FIELDS.filter((f) => !shown || shown.has(f.name)).map((f) => ({
    name: f.name,
    references: [...new Set(textsOf(e, f.key).flatMap(referencesIn))],
  }));
}

/**
 * Every field that points at a choice no earlier effect declares, with the name it lacks. A choice that was removed, renamed or
 * moved below its field leaves such a field; nothing is re-pointed, and the save waits until the master fixes each one.
 */
export function missingChoices(
  effects: readonly EffectDraft[],
  visible?: Visible,
  base = 'table_feat.effects',
): MissingChoice[] {
  const out: MissingChoice[] = [];
  const declared = new Set<string>();
  effects.forEach((e, index) => {
    for (const f of referencedFields(e, visible)) {
      for (const name of f.references.filter((n) => !declared.has(n))) {
        out.push({ index, path: `${base}[${index}].${f.name}`, name });
      }
    }
    if (kindOf(e)) {
      declared.add(e.name);
    }
  });
  return out;
}

/** The names a field of an effect points at, once each. */
function referencesOfField(e: EffectDraft, field: string): string[] {
  const own = REFERENCE_FIELDS.find((f) => f.name === field);
  return own ? [...new Set(textsOf(e, own.key).flatMap(referencesIn))] : [];
}

export interface ReferenceProblem {
  /** `removed` when no effect of the feat declares the name any more, `later` when the choice is declared at this effect or below it. */
  why: 'removed' | 'later';
  /** The name the field points at, with no "@". */
  name: string;
}

/** Why a field's pick cannot be read, with the name it points at; null when every name it holds is a choice of an earlier effect (or it holds none). */
export function referenceProblem(
  e: EffectDraft,
  field: string,
  choices: readonly FeatChoice[],
  index: number,
): ReferenceProblem | null {
  let later: ReferenceProblem | null = null;
  for (const name of referencesOfField(e, field)) {
    const known = choices.filter((c) => c.name === name);
    if (known.some((c) => c.index < index)) {
      continue;
    }
    if (known.length === 0) {
      return { why: 'removed', name };
    }
    later ??= { why: 'later', name };
  }
  return later;
}

export interface ChoiceUse {
  choice: FeatChoice;
  /** How many later effects have a field that points at the choice. */
  effects: number;
}

/** Each named choice of the feat with the number of effects that use it, in the order of the feat's effects; `visible` is the same as `missingChoices` takes. */
export function choiceUses(effects: readonly EffectDraft[], visible?: Visible): ChoiceUse[] {
  return featChoices(effects).map((choice) => {
    const used = effects.filter(
      (e, index) =>
        index > choice.index &&
        referencedFields(e, visible).some((f) => f.references.includes(choice.name)),
    );
    return { choice, effects: used.length };
  });
}
