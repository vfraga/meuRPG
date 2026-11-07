import type { TableContentViolation } from '../../../gen/meurpg/rules/v1/table_content_pb';
import { FIELD_TEXTS, FORMULA_TEXT, LONG_TEXT } from './content-violation-texts';

/**
 * What the server refused in a write, put back on the fields (MR-025, E10-01 state 5). A violation has the path of
 * the field in the request's body ("table_race.traits[1].effects[0].value") and a reason code; the app writes the
 * Portuguese from the code and the path, and never reads `message` (it is English, for logs). Every editor input
 * carries its own path as `data-field`, so a path finds its input with no table per editor; a path the screen draws
 * no input for goes to its nearest ancestor that has one (an array item to the array), and a violation with no field,
 * or about another entry, goes to the top.
 */

export interface PlacedViolation {
  readonly field: string;
  readonly reason: string;
  /** The Portuguese text. */
  readonly text: string;
}

export interface Placement {
  /** By the path of the input that shows it. */
  readonly byField: ReadonlyMap<string, readonly PlacedViolation[]>;
  /** For the summary at the top: no field, or another entry. */
  readonly top: readonly PlacedViolation[];
  /** The inputs that have a message, in the order the server listed them. */
  readonly fields: readonly string[];
  /** How many things need an adjustment (the summary counts these). */
  readonly count: number;
}

export const NO_PLACEMENT: Placement = { byField: new Map(), top: [], fields: [], count: 0 };

/** The placement without the messages on `paths`: the fields the client now says something about itself. The rest stays. */
export function withoutFields(placement: Placement, paths: readonly string[]): Placement {
  const gone = new Set(paths);
  const byField = new Map([...placement.byField].filter(([path]) => !gone.has(path)));
  const fields = placement.fields.filter((path) => !gone.has(path));
  return { byField, top: placement.top, fields, count: fields.length + placement.top.length };
}

/** "table_race.traits[1].effects[0].value" without its indexes: "table_race.traits[].effects[].value". */
export function shapeOf(path: string): string {
  return path.replace(/\[\d+\]/g, '[]');
}

/** The path one step up: "a.b[2].c" → "a.b[2]" → "a.b" → "a" → "". */
export function parentOf(path: string): string {
  if (path.endsWith(']')) {
    return path.slice(0, path.lastIndexOf('['));
  }
  const dot = path.lastIndexOf('.');
  return dot < 0 ? '' : path.slice(0, dot);
}

/** The path of the nearest input that exists at or above `field`, or `''` when none does. */
export function inputFor(field: string, isKnown: (path: string) => boolean): string {
  let path = field;
  while (path !== '') {
    if (isKnown(path)) {
      return path;
    }
    path = parentOf(path);
  }
  return '';
}

/** What the editor of a kind says in the sentences ("uma magia da mesa"). */
export interface ViolationContext {
  /** "uma magia", "uma raça", "um antecedente". */
  readonly aOne: string;
  /** The name of the entry a `key` points to, for "Sobre Corujeiro: …". */
  readonly nameOf?: (key: string) => string;
  /** The most features a class or a subclass has (the menu's), for "Uma classe da mesa não pode ter mais de 60 características." */
  readonly maxFeatures?: number;
  /** How many features the entry being saved has: "Esta classe tem 61." */
  readonly featureCount?: number;
  /** Where a violation lands when it is not the input of its own path: "too many features" is the panel head's, not a row's. */
  readonly redirect?: (field: string, reason: string) => string;
}

/** The most features a class or a subclass has when the menu did not say. */
const DEFAULT_MAX_FEATURES = 60;

const SIZE_LIMIT_TEXT = 'Esta entrada passou de 64 KiB de dados. Tire um pouco de texto.';

/** A violation about the entry as a whole, with no field. */
function entryText(r: string): string {
  switch (r) {
    case 'limit':
      return 'A mesa já tem 1000 entradas. Arquive ou use o que já existe.';
    case 'size_limit':
      return SIZE_LIMIT_TEXT;
    case 'overlay':
      return 'O conteúdo da mesa não fecha com esta entrada. Confira o que ela usa.';
    default:
      return 'O servidor recusou esta entrada. Confira os campos.';
  }
}

/** The reasons about a name, or `undefined` when the reason says nothing special about one. */
function nameText(r: string, ctx: ViolationContext, shape: string): string | undefined {
  if (r === 'duplicate_name') {
    return `Já existe ${ctx.aOne} da mesa com este nome. Escolha outro.`;
  }
  if (r === 'bad_name' || r === 'bad_text') {
    // a table creature's name allows 80 letters, like its traits and attacks
    return shape.startsWith('table_creature.')
      ? 'O nome tem de 1 a 80 letras.'
      : 'O nome tem de 1 a 60 letras.';
  }
  if (r === 'bad_key' || r === 'reserved_key' || r === 'duplicate_key') {
    return 'Este nome não pode ser usado. Escolha outro.';
  }
  return undefined;
}

/** The reasons that say the same wherever they come from, or `undefined`. */
function reasonText(shape: string, leaf: string, r: string): string | undefined {
  switch (r) {
    case 'immutable':
      return leaf === 'level'
        ? 'Um truque não vira magia de 1º nível ou mais, nem o contrário. Crie outra.'
        : 'Isto não muda depois de criado.';
    case 'size_limit':
      return SIZE_LIMIT_TEXT;
    case 'bad_formula':
      return FORMULA_TEXT;
    case 'forbidden_effect':
      return 'Este efeito não está no menu.';
    case 'bad_value':
      return shape.endsWith('.effects[].resource')
        ? 'Use só letras minúsculas sem acento, números e _ (de 1 a 40), como vigilia.'
        : undefined;
    default:
      return undefined;
  }
}

/** The row of the table that matches the field (and the reason), with the feature limits of the class worded. */
function rowText(shape: string, r: string, ctx: ViolationContext): string | undefined {
  const row = FIELD_TEXTS.find(
    (f) => f.shape.test(shape) && (f.reason === undefined || f.reason === r),
  );
  if (!row) {
    return undefined;
  }
  if (row.text !== 'LIMIT_CLASS' && row.text !== 'LIMIT_SUBCLASS') {
    return row.text;
  }
  const max = ctx.maxFeatures ?? DEFAULT_MAX_FEATURES;
  const one = row.text === 'LIMIT_CLASS' ? 'classe' : 'subclasse';
  const has = ctx.featureCount !== undefined ? `Esta ${one} tem ${ctx.featureCount}. ` : '';
  return `${has}Uma ${one} da mesa não pode ter mais de ${max} características.`;
}

/** What is said when nothing above matched the field: by the reason alone. */
function fallbackText(shape: string, r: string): string {
  if (r === 'dangling_reference') {
    return 'Isto aponta para algo que não existe mais. Escolha de novo.';
  }
  if (shape.includes('.effects[].')) {
    return r === 'bad_value'
      ? 'Este valor não serve para este efeito.'
      : 'Confira este campo do efeito.';
  }
  switch (r) {
    case 'bad_text':
      return LONG_TEXT;
    case 'limit':
      return 'Isto passou do limite.';
    case 'duplicate_name':
    case 'duplicate_key':
      return 'Já existe uma entrada com isto. Escolha outro.';
    case 'bad_table':
      return 'A tabela tem um erro. Confira os níveis, as linhas e os espaços de magia.';
    case 'bad_casting':
      return 'A conjuração tem um erro. Confira como e com o quê ela funciona.';
    case 'unknown_field':
      return 'Este campo não existe. Tire-o do arquivo e importe de novo.';
    default:
      return 'Este valor não é aceito. Confira o campo.';
  }
}

/** The Portuguese of one violation, by the shape of its path and its reason. */
export function violationText(
  v: Pick<TableContentViolation, 'field' | 'reason'>,
  ctx: ViolationContext,
): string {
  const shape = shapeOf(v.field);
  if (shape === '') {
    return entryText(v.reason);
  }
  const leaf = shape.slice(shape.lastIndexOf('.') + 1);
  // A trait's or an attack's name has one reason for every way it is wrong: its own row says them all.
  const named =
    leaf === 'name_pt' && !/^table_creature\.(traits|attacks)\[\]\./.test(shape)
      ? nameText(v.reason, ctx, shape)
      : undefined;
  return (
    named ??
    reasonText(shape, leaf, v.reason) ??
    rowText(shape, v.reason, ctx) ??
    fallbackText(shape, v.reason)
  );
}

/** The server checks a feature's summon as the hidden spell it is cast as, so the feature's own text is echoed at `.summon.desc_pt[0]`: it is the feature's text field that holds it. */
export function summonEcho(field: string): string {
  return field.replace(/\.summon\.desc_pt(\[\d+\])?$/, '.desc_pt');
}

/** Puts every violation on its input: `isKnown` says which paths the editor draws an input for right now. */
export function placeViolations(
  violations: readonly TableContentViolation[],
  isKnown: (path: string) => boolean,
  ctx: ViolationContext,
): Placement {
  const byField = new Map<string, PlacedViolation[]>();
  const top: PlacedViolation[] = [];
  const fields: string[] = [];
  for (const v of violations) {
    const text = violationText(v, ctx);
    const field = v.field;
    if (v.key !== '') {
      // About another entry: it never marks a field of this one.
      const who = ctx.nameOf?.(v.key) ?? '';
      top.push({ field: '', reason: v.reason, text: who ? `Sobre ${who}: ${text}` : text });
      continue;
    }
    const at =
      field === '' ? '' : inputFor(ctx.redirect?.(field, v.reason) ?? summonEcho(field), isKnown);
    const placed: PlacedViolation = { field: at, reason: v.reason, text };
    if (at === '') {
      top.push(placed);
      continue;
    }
    const list = byField.get(at) ?? [];
    // The same sentence twice on one input says nothing new.
    if (!list.some((p) => p.text === text)) {
      list.push(placed);
    }
    byField.set(at, list);
    if (!fields.includes(at)) {
      fields.push(at);
    }
  }
  return { byField, top, fields, count: fields.length + top.length };
}

/** "Não foi possível salvar a magia. 3 campos precisam de ajuste. Nada foi salvo; o que você digitou continua aqui." */
export function refusalSummary(what: string, count: number): string {
  const n = count === 1 ? '1 campo precisa' : `${count} campos precisam`;
  return `${n} de ajuste. Nada foi salvo; o que você digitou continua aqui.`.replace(
    /^/,
    `Não foi possível salvar ${what}. `,
  );
}
