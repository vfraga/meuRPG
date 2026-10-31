import { unreadableCastingPaths } from '../../../core/content/whole-number';
import type {
  GetClassTableDefaultsResponse,
  TableEntry,
} from '../../../../gen/meurpg/rules/v1/table_content_pb';
import {
  type CastingDraft,
  type ClassDraft,
  type GridColumns,
  type LevelFeature,
  classFeatureBase,
  LEVEL_COUNT,
  type RowDraft,
  classFeaturePaths,
  defaultTableOf,
  rowsOfTable,
  isGridPath,
} from '../../../core/content/class-draft';
import type { ViolationContext } from '../../../core/content/content-violations';
import type { EffectMenuVm } from '../../../core/content/effect-draft';
import {
  ABILITY_FIELDS,
  type AbilityField,
  unreadableFeaturePaths,
} from '../../../core/content/feature-draft';
import type { SelectOption } from '../../../shared/form-fields/select-field';
import type { GridChip, GridEdit, GridRowVm } from '../../../shared/level-grid/level-grid';

/** The armor, shield and weapon groups every class lists (the keys; the names are the server's). */
export const ARMOR_KEYS = [
  'proficiency:light-armor',
  'proficiency:medium-armor',
  'proficiency:heavy-armor',
  'proficiency:shields',
] as const;
export const WEAPON_KEYS = ['proficiency:simple-weapons', 'proficiency:martial-weapons'] as const;
export const GROUP_KEYS: readonly string[] = [...ARMOR_KEYS, ...WEAPON_KEYS];

/** The question that comes up when a change would replace a table the master edited. */
export type TableAsk =
  { readonly kind: 'casting'; readonly next: CastingDraft } | { readonly kind: 'restore' };

/** The two multiclass groups: "todas" (`minimums`) and "pelo menos uma" (`any_of`). */
export type RequirementField = 'minimums' | 'anyOf';

/** What a requirement stands at when it is added, and when the field is emptied while typed. */
const REQUIREMENT_DEFAULT = 13;
const REQUIREMENT_FLOOR = 1;

/** The sections of the page, in the order of the side list. */
export const SECTIONS = [
  { id: 'basics', label: 'Básico' },
  { id: 'profs', label: 'Proficiências' },
  { id: 'casting', label: 'Conjuração' },
  { id: 'table', label: 'Tabela dos 20 níveis' },
  { id: 'features', label: 'Características' },
  { id: 'subclasses', label: 'Subclasse' },
] as const;

/** The paths (after `table_class.`) each section owns, as the server names a refusal. */
const SECTION_PATHS: Readonly<Record<string, (p: string) => boolean>> = {
  basics: (p) => /^(name_pt|hit_die|saving_throws|skill_)/.test(p),
  profs: (p) => /^(proficiencies|multiclass_|minimums|any_of)/.test(p),
  casting: (p) => p.startsWith('casting'),
  table: (p) =>
    /^levels(\[\d+\](\.(prof_bonus|cantrips_known|spells_known|slots.*))?)?$/.test(p) ||
    p.startsWith('asi_levels'),
  features: (p) => /^levels\[\d+\]\.features/.test(p) || p === 'features',
};

/** Whether a section has a refusal in it. */
export function sectionHasIssue(id: string, fields: readonly string[]): boolean {
  const owns = SECTION_PATHS[id] ?? ((p: string) => p.startsWith('subclass_level'));
  return fields.some((f) => owns(f.replace('table_class.', '')));
}

/** The server's numbers before a class has any: the editor starts here until the page hands it the real defaults. */
export function emptyDefaults(): GetClassTableDefaultsResponse {
  return {
    profBonus: [],
    asiLevels: [],
    subclassLevel: 3,
    tables: [],
  } as unknown as GetClassTableDefaultsResponse;
}

/** The 20 rows of the grid, each with the locked chips (ASI, subclass) and the features of its level. */
export function gridRowsOf(d: ClassDraft): GridRowVm[] {
  return d.rows.map((row, i) => {
    const level = i + 1;
    const chips: GridChip[] = [];
    if (d.asiLevels.includes(level))
      chips.push({
        id: `asi-${level}`,
        label: 'Incremento no Valor de Habilidade',
        locked: true,
      });
    if (d.subclassLevel === level)
      chips.push({ id: `sub-${level}`, label: 'Escolha de subclasse', locked: true });
    for (const f of d.features.filter((x) => x.level === level)) {
      chips.push({ id: f.id, label: f.feature.name.trim() || 'Sem nome', locked: false });
    }
    return { level, base: `table_class.levels[${i}]`, row, chips };
  });
}

/** "Preenchida com a tabela do SRD de Paladino": says where the numbers come from, in the server's words. */
export function tableNoteOf(
  casting: CastingDraft,
  defaults: GetClassTableDefaultsResponse,
  nameOf: (key: string) => string,
): string {
  const t = defaultTableOf(casting, defaults);
  const ref = t?.referenceClassKey ? nameOf(t.referenceClassKey) : '';
  if (casting.kind === '') {
    return 'Uma classe sem conjuração só tem o bônus de proficiência e as características em cada nível. “Restaurar o padrão” volta ao bônus do SRD.';
  }
  const from = ref
    ? `Preenchida com a tabela do ${ref} do SRD.`
    : 'Preenchida com o padrão de um terço.';
  const prep =
    casting.preparation === 'prepared'
      ? ' As magias são preparadas: o número vem da habilidade e do nível, não da tabela.'
      : ' As magias são conhecidas: a coluna “Magias” diz quantas.';
  return `${from} Edite o que quiser; “Restaurar o padrão” volta a ela.${prep}`;
}

/** The words of the question about replacing an edited table. */
export function questionTextOf(
  q: TableAsk | null,
  casting: CastingDraft,
  defaults: GetClassTableDefaultsResponse,
  nameOf: (key: string) => string,
): string {
  if (!q) return '';
  const t = defaultTableOf(q.kind === 'restore' ? casting : q.next, defaults);
  const ref = t?.referenceClassKey ? ` do ${nameOf(t.referenceClassKey)}` : '';
  return q.kind === 'restore'
    ? `Voltar a tabela ao padrão${ref}? O que você editou nela se perde.`
    : `Você editou a tabela dos níveis. Refazê-la com o padrão${ref} para esta conjuração? O que você editou nela se perde.`;
}

/** The rows with one cell typed. */
export function editedRows(rows: readonly RowDraft[], e: GridEdit): RowDraft[] {
  return rows.map((r, i) => {
    if (i !== e.level - 1) return r;
    switch (e.field) {
      case 'profBonus':
        return { ...r, profBonus: e.value };
      case 'cantrips':
        return { ...r, cantrips: e.value };
      case 'spells':
        return { ...r, spells: e.value };
      default:
        return { ...r, slots: r.slots.map((n, k) => (k === e.slot ? e.value : n)) };
    }
  });
}

/** The list with `key` taken out when it is there, put in when it is not. */
export function toggled(list: readonly string[], key: string): string[] {
  return list.includes(key) ? list.filter((k) => k !== key) : [...list, key];
}

/** The proficiencies with a group on (once, last) or off. */
export function withProficiency(list: readonly string[], key: string, on: boolean): string[] {
  const rest = list.filter((k) => k !== key);
  return on ? [...rest, key] : rest;
}

/** The ability a requirement row asks, at its default minimum. */
export function addedRequirement(d: ClassDraft, field: RequirementField, ability: string) {
  return { ...d[field], [ability]: REQUIREMENT_DEFAULT };
}

/** The ability taken off a requirement. */
export function removedRequirement(d: ClassDraft, field: RequirementField, ability: string) {
  return { ...d[field], [ability]: 0 };
}

/** Another ability for a row: the minimum moves with it (a row never takes an ability another row has); null when it cannot. */
export function movedRequirement(
  d: ClassDraft,
  field: RequirementField,
  from: AbilityField,
  to: string,
) {
  if (to === from || d[field][to as AbilityField] > 0) return null;
  return { ...d[field], [from]: 0, [to]: d[field][from] };
}

/** A minimum typed: a field emptied or at 0 keeps its place at 1 until it is taken off (0 would remove the line while it is typed). */
export function typedRequirement(
  d: ClassDraft,
  field: RequirementField,
  ability: AbilityField,
  text: string,
) {
  const n = Number(text.replace(/\D/g, ''));
  return { ...d[field], [ability]: Number.isFinite(n) && n > 0 ? n : REQUIREMENT_FLOOR };
}

/** Whether a path the server may refuse is one the page has an input for. */
export function isKnownPath(
  path: string,
  d: ClassDraft,
  columns: GridColumns,
  menu: EffectMenuVm,
): boolean {
  const p = 'table_class';
  if (KNOWN_FIELDS.some((f) => path === `${p}.${f}`)) return true;
  if (
    ABILITY_FIELDS.some((a) =>
      path === `${p}.minimums.${a}`
        ? d.minimums[a] > 0
        : path === `${p}.any_of.${a}`
          ? d.anyOf[a] > 0
          : false,
    )
  )
    return true;
  if (isGridPath(path, `${p}.levels`, columns, true)) return true;
  return classFeaturePaths(d.features, menu).includes(path);
}

const KNOWN_FIELDS = [
  'name_pt',
  'hit_die',
  'saving_throws',
  'saving_throws[0]',
  'saving_throws[1]',
  'skill_choose',
  'skill_from',
  'proficiencies',
  'multiclass_proficiencies',
  'multiclass_skill_choose',
  'minimums',
  'any_of',
  'subclass_level',
  'asi_levels',
  'casting.kind',
  'casting.ability',
  'casting.list_from',
  'casting.srd_spell_keys',
  'casting.preparation',
  'casting.start_level',
  'casting.prepared_max',
  'casting',
  'features',
];

/** The two multiclass groups of prerequisites, as the page lists them. */
export const REQUIREMENT_GROUPS = [
  {
    field: 'minimums' as const,
    label: 'Quem entra na classe precisa de todas estas habilidades',
    add: 'Adicionar habilidade',
  },
  { field: 'anyOf' as const, label: 'Ou de pelo menos uma destas', add: 'Adicionar habilidade' },
];

/** What a refusal of the server needs to know of the editor, read when the refusal comes (so it sees the draft as it is then). */
export function classViolationContext(editor: {
  entries(): readonly TableEntry[];
  menu(): EffectMenuVm;
  draft(): ClassDraft;
}): ViolationContext {
  return {
    aOne: 'uma classe',
    nameOf: (key: string) => editor.entries().find((e) => e.key === key)?.namePt ?? '',
    get maxFeatures() {
      return editor.menu().maxFeatures;
    },
    get featureCount() {
      return editor.draft().features.length;
    },
    // "Too many features" is the panel head's, with the counter, and not one feature row's.
    redirect: (field: string, reason: string) =>
      reason === 'limit' && /^table_class\.levels\[\d+\]\.features\[\d+\]$/.test(field)
        ? 'table_class.features'
        : field,
  };
}

/** What an answer to the table question does: the draft's changes and the sentence for a screen reader (none: nothing changes). */
export function answeredQuestion(
  q: TableAsk,
  replace: boolean,
  d: ClassDraft,
  defaults: GetClassTableDefaultsResponse,
): { patch: Partial<ClassDraft>; status: string } | null {
  const fresh = (c: CastingDraft) => rowsOfTable(defaultTableOf(c, defaults), defaults);
  if (q.kind === 'restore') {
    return replace
      ? { patch: { rows: fresh(d.casting) }, status: 'A tabela voltou ao padrão.' }
      : null;
  }
  return {
    patch: { casting: q.next, rows: replace ? fresh(q.next) : d.rows },
    status: replace
      ? 'A tabela foi refeita com o padrão.'
      : 'A conjuração mudou; a tabela ficou como estava.',
  };
}

/** The hit dice a class may have. */
const D6 = 6;
const D8 = 8;
const D10 = 10;
const D12 = 12;
export const HIT_DICE: SelectOption<number>[] = [D6, D8, D10, D12].map((n) => ({
  value: n,
  label: `d${n}`,
}));

/** The levels, for the select of the subclass level and the ASI picker. */
export const LEVEL_OPTIONS: SelectOption<number>[] = Array.from(
  { length: LEVEL_COUNT },
  (_, i) => ({
    value: i + 1,
    label: String(i + 1),
  }),
);
export const LEVEL_NUMBERS = Array.from({ length: LEVEL_COUNT }, (_, i) => i + 1);
export const ASI_OPTIONS: SelectOption[] = LEVEL_OPTIONS.map((o) => ({
  value: String(o.value),
  label: o.label,
}));

/** The subclasses the table wrote for a class. */
export function subclassesOf(entries: readonly TableEntry[], classKey: string): TableEntry[] {
  return classKey
    ? entries.filter((e) => e.body.case === 'tableSubclass' && e.body.value.classKey === classKey)
    : [];
}

/** The paths of the casting's start level and of the number fields, in every feature of a class, whose text is no whole number. */
export function unreadableClassPaths(
  casting: CastingDraft,
  features: readonly LevelFeature[],
  menu: EffectMenuVm,
): string[] {
  return [
    ...(casting.kind !== '' ? unreadableCastingPaths('table_class.casting', casting) : []),
    ...features.flatMap((f, i) =>
      unreadableFeaturePaths(classFeatureBase(features, i), f.feature, menu),
    ),
  ];
}
