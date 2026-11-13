import type {
  TableContentViolation,
  TableEntry,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import { EFFECT_FIELD_LABELS } from './effect-field-copy';
import { KIND_NOUNS, KIND_WORDS, navOfKind } from './content-kinds';
import { violationText } from './content-violations';

/**
 * The entries the rules refuse (issue "left out entries"): the server builds the campaign's content without them
 * and tells the master which and why (`ListTableEntriesResponse.left_out`, first violation of each). This turns that
 * into what the list, the entry page and the campaign page say: a readable place ("Nível 1 · Vigília · efeito 1"),
 * the field in the editor's words ("O que muda"), the sentence of the violation, and what is out only because of it.
 * Pure functions, no screen: the rows are tested without one.
 */

/** Where in the entry a violation is, in the editor's words. */
export interface PathPlace {
  /** The parts of the place: "Nível 1", "Vigília", "efeito 1". Empty for the entry as a whole. */
  readonly parts: readonly string[];
  /** The label of the field the editor draws ("O que muda"), or `''` when the path names no field with a label. */
  readonly field: string;
}

/** The labels of the fields an entry has that are not an effect's (an effect's are `EFFECT_FIELD_LABELS`). */
const FIELD_LABELS: Readonly<Record<string, string>> = {
  name_pt: 'Nome',
  desc_pt: 'Descrição',
  hit_die: 'Dado de vida',
  saving_throws: 'Testes de resistência',
  skill_choose: 'Quantas perícias',
  skill_from: 'Perícias da lista',
  proficiencies: 'Proficiências',
  multiclass_proficiencies: 'Proficiências de multiclasse',
  multiclass_skill_choose: 'Perícias de multiclasse',
  minimums: 'Pré-requisito de multiclasse',
  any_of: 'Pré-requisito de multiclasse',
  subclass_level: 'Nível da subclasse',
  casting: 'Conjuração',
  list_from: 'Lista de magias',
  slots: 'Espaços de magia',
  cantrips_known: 'Truques conhecidos',
  spells_known: 'Magias conhecidas',
  prof_bonus: 'Bônus de proficiência',
  class_key: 'Classe',
  race_key: 'Raça',
  level: 'Nível',
  size: 'Tamanho',
  speed_ft: 'Deslocamento',
  darkvision_ft: 'Visão no escuro',
  ability_bonuses: 'Bônus de habilidade',
  choice_bonuses: 'Bônus à escolha',
  languages: 'Idiomas',
  language_choices: 'Idiomas à escolha',
  replaces_trait_keys: 'Traços que substitui',
  skills: 'Perícias',
  tools: 'Ferramentas',
  always_prepared: 'Magias sempre preparadas',
  spell_key: 'Magia',
  school_key: 'Escola',
  class_keys: 'Classes',
  prerequisite: 'Pré-requisito',
  set: 'Lista',
  offered_by: 'Oferecida por',
};

const TOKEN = /^([a-z_]+)(?:\[(\d+)\])?$/;

function camel(name: string): string {
  return name.replace(/_([a-z])/g, (_, c: string) => c.toUpperCase());
}

function asRecord(v: unknown): Record<string, unknown> | undefined {
  return typeof v === 'object' && v !== null ? (v as Record<string, unknown>) : undefined;
}

function nameOfNode(node: unknown, fallback: string): string {
  const name = asRecord(node)?.['namePt'];
  return typeof name === 'string' && name !== '' ? name : fallback;
}

/**
 * The place and the field of a violation's path in an entry: "table_class.levels[0].features[0].effects[0].target" in the
 * class Guardião do Vale is level 1, the feature Vigília, effect 1, the field "O que muda". The entry's own body names the
 * level and the feature, so the path needs no table per editor; a path it cannot read gives what it could.
 */
export function placeOfPath(entry: TableEntry, field: string): PathPlace {
  let cursor = asRecord(entry.body.value);
  const parts: string[] = [];
  let inEffect = false;
  let label = '';
  for (const raw of field.split('.').slice(1)) {
    const m = TOKEN.exec(raw);
    if (!m) {
      continue;
    }
    const index = m[2] === undefined ? undefined : Number(m[2]);
    const list = cursor?.[camel(m[1])];
    const node =
      index === undefined ? list : Array.isArray(list) ? (list as unknown[])[index] : undefined;
    const step = stepOf(m[1], index, node, inEffect);
    if (step.part) {
      parts.push(step.part);
    }
    inEffect = inEffect || step.effect;
    label = step.label ?? label;
    cursor = asRecord(node);
  }
  return { parts, field: label };
}

/** What one token of a path says: a place to name, whether it enters an effect, and the label of a field. */
function stepOf(
  name: string,
  index: number | undefined,
  node: unknown,
  inEffect: boolean,
): { part?: string; effect: boolean; label?: string } {
  const part = index === undefined ? singlePart(name, node) : listedPart(name, index, node);
  if (part !== undefined) {
    return { part, effect: name === 'effects' };
  }
  return {
    effect: false,
    label: (inEffect ? EFFECT_FIELD_LABELS[name] : undefined) ?? FIELD_LABELS[name],
  };
}

/** The place a token without an index names: the feature of a background or a subrace. */
function singlePart(name: string, node: unknown): string | undefined {
  return name === 'feature' ? nameOfNode(node, 'característica') : undefined;
}

/** The place a token with an index names: "Nível 3", the feature, "efeito 2", "opção Dragão". */
function listedPart(name: string, index: number, node: unknown): string | undefined {
  const level = asRecord(node)?.['level'];
  const places: Record<string, () => string> = {
    levels: () => `Nível ${typeof level === 'number' && level > 0 ? level : index + 1}`,
    features: () => nameOfNode(node, `característica ${index + 1}`),
    traits: () => nameOfNode(node, `traço ${index + 1}`),
    options: () => `opção ${nameOfNode(node, String(index + 1))}`,
    effects: () => `efeito ${index + 1}`,
  };
  return places[name]?.();
}

/** The sentence of a violation with its first letter small, to follow "O que muda:". */
function lowered(text: string): string {
  return /^[A-ZÀ-Ý](?=[a-zà-ÿ]| [a-zà-ÿ])/.test(text)
    ? text.charAt(0).toLowerCase() + text.slice(1)
    : text;
}

/** An entry that is out only because the class or race it belongs to is out. */
export interface LeftOutDependent {
  readonly entry: TableEntry;
  /** "Saiu porque a classe dela, Guardião do Vale, está fora. Volta sozinha quando a classe voltar." */
  readonly text: string;
}

/** One entry the master has to fix, with what is out together with it. */
export interface LeftOutRow {
  readonly entry: TableEntry;
  /** The violation, with its key left as the entry's own. */
  readonly violation: TableContentViolation;
  readonly place: PathPlace;
  /** The Portuguese sentence of the violation, first letter small when it follows a place. */
  readonly text: string;
  /** The same sentence with its first letter as the server's words give it, for a line that starts with it. */
  readonly sentence: string;
  /** "Nível 1 · Vigília · efeito 1", or `''`. */
  readonly placeLine: string;
  /** The place for a phone, where there is no room for the level: "Vigília, efeito 1". */
  readonly shortPlace: string;
  /** "Classe", "Talento": the word of the kind for the tag. */
  readonly kindWord: string;
  readonly dependents: readonly LeftOutDependent[];
  /** "1 ficha usa esta classe." or `''`. */
  readonly usingLine: string;
  /** "1 ficha usa", for a phone. */
  readonly usingShort: string;
}

export interface LeftOutModel {
  /** The entries to fix, in the list's order (kind, then name). */
  readonly rows: readonly LeftOutRow[];
  /** Every entry left out, dependents included: "3 entradas fora da campanha". */
  readonly count: number;
  /** The keys of every entry left out, for the tag on the list's rows. */
  readonly keys: ReadonlySet<string>;
}

export const NO_LEFT_OUT: LeftOutModel = { rows: [], count: 0, keys: new Set() };

/** The key of the class or race an entry belongs to, or `''` for one that stands alone. */
export function parentKeyOf(entry: TableEntry): string {
  const b = entry.body;
  switch (b.case) {
    case 'tableSubclass':
    case 'tableClassFeature':
      return b.value.classKey;
    case 'tableSubrace':
      return b.value.raceKey;
    default:
      return '';
  }
}

function parentWord(entry: TableEntry): string {
  return entry.body.case === 'tableSubrace' ? 'raça' : 'classe';
}

/** "A ficha que usa" lines: "1 ficha usa esta classe.", "3 fichas usam este talento.". */
export function usingLineOf(entry: TableEntry): string {
  const n = entry.charactersUsing;
  if (n <= 0) {
    return '';
  }
  const noun = KIND_NOUNS[entry.kind];
  const this_ = noun?.article === 'O' ? 'este' : 'esta';
  return `${n} ${n === 1 ? 'ficha usa' : 'fichas usam'} ${this_} ${noun?.noun ?? 'entrada'}.`;
}

/** "1 ficha usa", "3 fichas usam", or `''`. */
export function usingShortOf(entry: TableEntry): string {
  const n = entry.charactersUsing;
  return n > 0 ? `${n} ${n === 1 ? 'ficha usa' : 'fichas usam'}` : '';
}

/** What the page says of the entries the rules refuse: rows to fix, with what each took out with it. */
export function buildLeftOut(
  entries: readonly TableEntry[],
  leftOut: readonly TableContentViolation[],
): LeftOutModel {
  if (leftOut.length === 0) {
    return NO_LEFT_OUT;
  }
  const byKey = new Map(entries.map((e) => [e.key, e]));
  // Only what is on the page counts: a key with no entry is not shown, and an entry the master archived is no longer reported
  // (the server drops it too; the page does it at once, before it reads again).
  const out = new Map(
    leftOut.filter((v) => byKey.get(v.key)?.archived === false).map((v) => [v.key, v]),
  );
  if (out.size === 0) {
    return NO_LEFT_OUT;
  }
  const dependents = new Map<string, LeftOutDependent[]>();
  const rows: LeftOutRow[] = [];
  for (const entry of entries) {
    const violation = out.get(entry.key);
    if (!violation) {
      continue;
    }
    const parent = byKey.get(parentKeyOf(entry));
    if (parent && out.has(parent.key) && violation.reason === 'dangling_reference') {
      const list = dependents.get(parent.key) ?? [];
      const word = parentWord(entry);
      list.push({
        entry,
        text: `Saiu porque a ${word} dela, ${parent.namePt}, está fora. Volta sozinha quando a ${word} voltar.`,
      });
      dependents.set(parent.key, list);
      continue;
    }
    const place = placeOfPath(entry, violation.field);
    const ctx = { aOne: navOfKind(entry.kind)?.aOne ?? 'uma entrada' };
    const sentence = violationText(violation, ctx);
    rows.push({
      entry,
      violation,
      place,
      sentence,
      text: lowered(sentence),
      placeLine: place.parts.join(' · '),
      shortPlace: place.parts.filter((p) => !p.startsWith('Nível')).join(', '),
      kindWord: KIND_WORDS[entry.kind] ?? 'Entrada',
      dependents: [],
      usingLine: usingLineOf(entry),
      usingShort: usingShortOf(entry),
    });
  }
  const withDependents = rows.map((r) => ({ ...r, dependents: dependents.get(r.entry.key) ?? [] }));
  const shown = withDependents.flatMap((r) => [
    r.entry.key,
    ...r.dependents.map((d) => d.entry.key),
  ]);
  return { rows: withDependents, count: shown.length, keys: new Set(shown) };
}

/** "3 entradas fora da campanha". */
export function leftOutTitle(count: number): string {
  return `${count} ${count === 1 ? 'entrada fora' : 'entradas fora'} da campanha`;
}

/** "Junto com ela: Juramento da Névoa (subclasse), que depende dela." The names are bold on the screen. */
export function togetherWith(row: LeftOutRow): {
  readonly names: readonly { name: string; word: string }[];
  readonly tail: string;
} {
  return {
    names: row.dependents.map((d) => ({
      name: d.entry.namePt,
      word: (KIND_WORDS[d.entry.kind] ?? '').toLowerCase(),
    })),
    tail: row.dependents.length === 1 ? 'que depende dela' : 'que dependem dela',
  };
}

/** Whether the entry the page shows is one of the left-out ones, and the violation to put on its editor. */
export function violationOfEntry(
  leftOut: readonly TableContentViolation[],
  key: string,
): TableContentViolation | undefined {
  return leftOut.find((v) => v.key === key);
}

/** What the page of a left-out entry says above its editor (K2). */
export interface LeftOutNote {
  /** "Esta classe está fora da campanha." (bold). */
  readonly lead: string;
  /** The rest: what is wrong and what to do. */
  readonly text: string;
  /** "1 ficha usa esta classe e está com aviso lá.", or `''`. */
  readonly using: string;
  /** The class or race this entry is out for, when it is out only because of it. */
  readonly parent: { readonly key: string; readonly name: string } | null;
}

function agree(entry: TableEntry): { this_: string; she: string; hers: string } {
  const male = KIND_NOUNS[entry.kind]?.article === 'O';
  return { this_: male ? 'este' : 'esta', she: male ? 'ele' : 'ela', hers: male ? 'dele' : 'dela' };
}

function usingNote(entry: TableEntry): string {
  const n = entry.charactersUsing;
  if (n <= 0) {
    return '';
  }
  const noun = KIND_NOUNS[entry.kind]?.noun ?? 'entrada';
  const { this_ } = agree(entry);
  return n === 1
    ? `1 ficha usa ${this_} ${noun} e está com aviso lá.`
    : `${n} fichas usam ${this_} ${noun} e estão com aviso lá.`;
}

function names(list: readonly string[]): string {
  return new Intl.ListFormat('pt-BR', { type: 'conjunction' }).format(list);
}

/** The note above the editor of an entry that is left out, or `null` for one that is not. `editing` is whether the person can fix it here. */
export function leftOutNote(
  entry: TableEntry,
  model: LeftOutModel,
  editing: boolean,
): LeftOutNote | null {
  if (!model.keys.has(entry.key)) {
    return null;
  }
  const noun = KIND_NOUNS[entry.kind]?.noun ?? 'entrada';
  const { this_, she, hers } = agree(entry);
  const lead = `${this_.charAt(0).toUpperCase()}${this_.slice(1)} ${noun} está fora da campanha.`;
  const using = usingNote(entry);
  const row = model.rows.find((r) => r.entry.key === entry.key);
  if (!row) {
    const parent = model.rows.find((r) =>
      r.dependents.some((d) => d.entry.key === entry.key),
    )?.entry;
    const word = parentWord(entry);
    return {
      lead,
      text: parent
        ? `Saiu porque a ${word} ${hers}, ${parent.namePt}, está fora. ${editing ? `Corrija a ${word}: ${she} volta sozinha.` : `Volta sozinha quando a ${word} voltar.`}`
        : `Saiu porque a ${word} ${hers} está fora.`,
      using,
      parent: parent ? { key: parent.key, name: parent.namePt } : null,
    };
  }
  const what = row.violation.field
    ? `As regras não aceitam mais um campo ${hers}, ${editing ? 'marcado abaixo' : row.place.field ? `o campo “${row.place.field}”` : 'marcado na entrada'}.`
    : `As regras não aceitam mais ${this_} ${noun}: ${row.text}`;
  const together =
    row.dependents.length > 0
      ? `, e ${names(row.dependents.map((d) => d.entry.namePt))} volta${row.dependents.length > 1 ? 'm' : ''} junto`
      : '';
  return {
    lead,
    text: editing
      ? `${what} Corrija e salve: ${she} volta${together}.`
      : `${what} Para corrigir, abra a entrada no computador.`,
    using,
    parent: null,
  };
}

/** "Guardião do Vale voltou para a campanha. Juramento da Névoa voltou junto, e a ficha que usa a classe não tem mais o aviso."
 * After a save: `''` when the entry was not out before or is still out. */
export function backInSentence(
  before: LeftOutModel,
  after: LeftOutModel,
  entry: TableEntry,
  entries: readonly TableEntry[],
): string {
  if (!before.keys.has(entry.key) || after.keys.has(entry.key)) {
    return '';
  }
  const noun = KIND_NOUNS[entry.kind]?.noun ?? 'entrada';
  const { this_ } = agree(entry);
  const back = entries
    .filter((e) => e.key !== entry.key && before.keys.has(e.key) && !after.keys.has(e.key))
    .map((e) => e.namePt);
  const clauses: string[] = [];
  if (back.length > 0) {
    clauses.push(`${names(back)} ${back.length > 1 ? 'voltaram' : 'voltou'} junto`);
  }
  if (entry.charactersUsing > 0) {
    clauses.push(
      `${entry.charactersUsing === 1 ? 'a ficha que usa' : 'as fichas que usam'} ${this_} ${noun} ${entry.charactersUsing === 1 ? 'não tem' : 'não têm'} mais o aviso`,
    );
  }
  const first = `${entry.namePt} voltou para a campanha.`;
  if (clauses.length === 0) {
    return first;
  }
  const rest = clauses.join(', e ');
  return `${first} ${rest.charAt(0).toUpperCase()}${rest.slice(1)}.`;
}
