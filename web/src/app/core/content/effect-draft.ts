import type {
  EffectMenuField,
  EffectMenuList,
  EffectMenuType,
  EffectOptionSet,
  GetEffectMenuResponse,
  TableEffect,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import { feetToMeters, metersToFeet } from '../units';

/**
 * A feature's effect as the editor holds it (MR-025, ADR-0018, section 4), and the menu it is chosen from. The menu is the
 * server's (`GetEffectMenu`): the types, the fields each one reads, the closed lists with their Portuguese names. Nothing
 * here lists a target, a sense or a recharge; whatever the menu offers, the server accepts. What goes to the server is
 * only the fields the chosen type reads (a field of another type is refused as `bad_value`), so changing the type drops
 * what the old one had.
 */

/** The fields of `TableEffect`, as plain values: strings for text, arrays for lists, numbers for counts and ranges. */
export interface EffectDraft {
  /** A stable id the editor tracks the row by (never sent): focus and "Mais opções" follow a moved item. */
  id: string;
  type: string;
  target: string;
  mode: string;
  value: string;
  when: string;
  /** Written as "against:magic, about:items": split into tags when sent. */
  tags: string;
  proficiency: string;
  level: string;
  roll: string;
  targets: string[];
  sense: string;
  /** The range as typed, in metres (the server stores feet): kept as text so the field never rewrites what is being typed. */
  rangeM: string;
  resource: string;
  max: string;
  recharge: string;
  choice: string;
  count: number;
  from: string[];
  economy: string;
  spells: string[];
  textPt: string;
}

let nextId = 0;

/** A new row id, for effects and features. */
export function newRowId(): string {
  return `row-${nextId++}`;
}

export function emptyEffect(type = ''): EffectDraft {
  return {
    id: newRowId(),
    type,
    target: '',
    mode: '',
    value: '',
    when: '',
    tags: '',
    proficiency: '',
    level: '',
    roll: '',
    targets: [],
    sense: '',
    rangeM: '',
    resource: '',
    max: '',
    recharge: '',
    choice: '',
    count: 0,
    from: [],
    economy: '',
    spells: [],
    textPt: '',
  };
}

/** A stored effect, as a draft. */
export function effectToDraft(e: TableEffect): EffectDraft {
  return {
    id: newRowId(),
    type: e.type,
    target: e.target,
    mode: e.mode,
    value: e.value,
    when: e.when,
    tags: e.tags.join(', '),
    proficiency: e.proficiency,
    level: e.level,
    roll: e.roll,
    targets: [...e.targets],
    sense: e.sense,
    rangeM: rangeMeters(e.rangeFt),
    resource: e.resource,
    max: e.max,
    recharge: e.recharge,
    choice: e.choice,
    count: e.count,
    from: [...e.from],
    economy: e.economy,
    spells: [...e.spells],
    textPt: e.textPt,
  };
}

/** The fields a request carries for one effect: the type, and only what that type reads. */
export type EffectInit = Partial<Omit<TableEffect, '$typeName' | '$unknown'>>;

/** The tags of "against:magic, about:items": trimmed, empty ones dropped. */
export function splitTags(text: string): string[] {
  return text
    .split(',')
    .map((t) => t.trim())
    .filter((t) => t !== '');
}

/** The effect a request sends: `type` and the fields the menu says that type reads, and nothing else. An empty text or an empty
 * list is not sent: the server reads "not set" the same way. */
export function draftToEffect(d: EffectDraft, menu: EffectMenuVm): EffectInit {
  const out: EffectInit = { type: d.type };
  const fields = menu.fieldsOf(d.type);
  for (const f of fields) {
    switch (f.name) {
      case 'target':
        if (d.target) out.target = d.target;
        break;
      case 'mode':
        if (d.mode) out.mode = d.mode;
        break;
      case 'value':
        if (d.value.trim()) out.value = d.value.trim();
        break;
      case 'when':
        if (d.when.trim()) out.when = d.when.trim();
        break;
      case 'tags': {
        const tags = splitTags(d.tags);
        if (tags.length > 0) out.tags = tags;
        break;
      }
      case 'proficiency':
        if (d.proficiency) out.proficiency = d.proficiency;
        break;
      case 'level':
        if (d.level) out.level = d.level;
        break;
      case 'roll':
        if (d.roll) out.roll = d.roll;
        break;
      case 'targets':
        if (d.targets.length > 0) out.targets = [...d.targets];
        break;
      case 'sense':
        if (d.sense) out.sense = d.sense;
        break;
      case 'range_ft':
        if (rangeFeet(d.rangeM) > 0) out.rangeFt = rangeFeet(d.rangeM);
        break;
      case 'resource':
        if (d.resource.trim()) out.resource = d.resource.trim();
        break;
      case 'max':
        if (d.max.trim()) out.max = d.max.trim();
        break;
      case 'recharge':
        if (d.recharge) out.recharge = d.recharge;
        break;
      case 'choice':
        if (d.choice) out.choice = d.choice;
        break;
      case 'count':
        if (d.count > 0) out.count = d.count;
        break;
      case 'from':
        if (d.from.length > 0) out.from = [...d.from];
        break;
      case 'economy':
        if (d.economy) out.economy = d.economy;
        break;
      case 'spells':
        if (d.spells.length > 0) out.spells = [...d.spells];
        break;
      case 'text_pt':
        if (d.textPt.trim()) out.textPt = d.textPt.trim();
        break;
      default:
        break;
    }
  }
  return out;
}

/** The labels of the effect fields, in our words (the menu names the fields by their proto name). */
export const EFFECT_FIELD_LABELS: Readonly<Record<string, string>> = {
  target: 'O que muda',
  mode: 'Como muda',
  value: 'Valor',
  when: 'Só quando',
  tags: 'Situação',
  proficiency: 'Proficiência em',
  level: 'Quanto',
  roll: 'Rolagem',
  targets: 'Em quê',
  sense: 'Sentido',
  range_ft: 'Alcance',
  resource: 'Nome do recurso',
  max: 'Usos (máximo)',
  recharge: 'Volta em',
  choice: 'O jogador escolhe',
  count: 'Quantos',
  from: 'Entre',
  economy: 'Tipo de ação',
  spells: 'Magias',
  text_pt: 'Texto na ficha',
};

/** One line under a field, where the name alone does not say. This is screen copy; the formula functions and the tag
 * prefixes it mentions come from the server's menu (`EffectMenuVm.formulaHint`, `tagHint`). */
export const EFFECT_FIELD_HINTS: Readonly<Record<string, string>> = {
  resource: 'Só letras minúsculas sem acento, números e _ (de 1 a 40).',
  text_pt: 'O que a ficha mostra. {value} vira o valor com sinal.',
};

/** A value of a closed list: the key the effect stores and the Portuguese name. */
export interface MenuOption {
  readonly key: string;
  readonly namePt: string;
  readonly hintPt: string;
}

/** The menu the pickers read, from `GetEffectMenu`. */
export class EffectMenuVm {
  readonly types: readonly EffectMenuType[];
  readonly lists: ReadonlyMap<string, readonly MenuOption[]>;
  readonly optionSets: readonly EffectOptionSet[];
  readonly classIndexes: readonly string[];
  readonly helpers: GetEffectMenuResponse['helpers'];
  readonly maxFeatures: number;
  readonly maxEffects: number;
  readonly maxTags: number;
  /** The Portuguese name of a class key, set by the page once it has the catalog. */
  classNamePt: (key: string) => string = (key) => key.replace(/^class:/, '');

  constructor(res: GetEffectMenuResponse) {
    this.types = res.types;
    this.lists = new Map(res.lists.map((l: EffectMenuList) => [l.name, l.values]));
    this.optionSets = res.optionSets;
    this.classIndexes = res.classIndexes;
    this.helpers = res.helpers;
    this.maxFeatures = res.maxFeaturesPerClass;
    this.maxEffects = res.maxEffectsPerFeature;
    this.maxTags = res.maxTagsPerEffect;
  }

  /** "Um número ou uma fórmula com: mod("<habilidade>"), prof()…": the functions are the server's (`helpers`). */
  formulaHint(): string {
    const calls = this.helpers.map((h) => h.call);
    return calls.length > 0
      ? `Um número ou uma fórmula. Funções: ${calls.join(', ')}.`
      : 'Um número ou uma fórmula.';
  }

  /** The prefixes a tag starts with, from the menu's `tag_prefixes`: "against:… (Contra…)". */
  tagHint(): string {
    const prefixes = this.list('tag_prefixes').map((p) => `${p.key}…`);
    return prefixes.length > 0
      ? `Separe por vírgula, começando por ${prefixes.join(' ou ')}. O app só lembra; o mestre decide.`
      : 'Separe por vírgula. O app só lembra; o mestre decide.';
  }

  typeOf(type: string): EffectMenuType | undefined {
    return this.types.find((t) => t.type === type);
  }

  fieldsOf(type: string): readonly EffectMenuField[] {
    return this.typeOf(type)?.fields ?? [];
  }

  /** The closed list a field points at. */
  list(name: string): readonly MenuOption[] {
    return this.lists.get(name) ?? [];
  }

  /** What a `choice` effect's `from` may hold, by the kind of choice: skills, languages, tools, an option set's options or a
   * spell list's class. Empty for a kind with no list ("any of the kind"). */
  fromOptions(choice: string): readonly MenuOption[] {
    switch (choice) {
      case 'skill':
      case 'expertise':
        return this.list('skills');
      case 'language':
        return this.list('languages');
      case 'tool':
        return this.list('tools');
      case 'cantrip':
      case 'spell':
        return this.classIndexes.map((i) => ({
          key: `class:${i}`,
          namePt: this.classNamePt(`class:${i}`),
          hintPt: '',
        }));
      case 'feature':
        return this.optionSets.map((s) => ({ key: s.key, namePt: s.namePt, hintPt: '' }));
      default:
        return [];
    }
  }

  /** The Portuguese name of a value of a list (the key when the menu does not know it). */
  nameOf(list: string, key: string): string {
    return this.list(list).find((v) => v.key === key)?.namePt ?? key;
  }
}

/** The metres a range field shows, from the feet it stores. */
export function rangeMeters(ft: number): string {
  return ft > 0 ? String(feetToMeters(ft)).replace('.', ',') : '';
}

/** The feet of what a range field holds ("18" or "4,5" metres); 0 for an empty or unreadable field. */
export function rangeFeet(text: string): number {
  const n = Number(text.trim().replace(',', '.'));
  return Number.isFinite(n) && n > 0 ? metersToFeet(n) : 0;
}

/** Whether a range field holds something that is not a number of metres (empty is fine: it means none). */
export function unreadableRange(text: string): boolean {
  const t = text.trim();
  if (t === '') {
    return false;
  }
  const n = Number(t.replace(',', '.'));
  return !Number.isFinite(n) || n < 0;
}

/** What the editor says of a range field it cannot read. */
export const UNREADABLE_RANGE = 'Escreva só o número de metros, como 18 ou 4,5.';

/** Like rangeFeet, but unreadable text is NaN instead of 0, so a field where 0 is valid cannot save it by accident. */
export function strictRangeFeet(text: string): number {
  return unreadableRange(text) ? Number.NaN : rangeFeet(text);
}

/** A required field of the type that still has nothing: the editor says so before the server does. */
export function missingRequired(d: EffectDraft, menu: EffectMenuVm): string[] {
  const out: string[] = [];
  for (const f of menu.fieldsOf(d.type)) {
    if (!f.required) {
      continue;
    }
    const empty = ((): boolean => {
      switch (f.name) {
        case 'target':
          return d.target === '';
        case 'mode':
          return d.mode === '';
        case 'value':
          return d.value.trim() === '';
        case 'proficiency':
          return d.proficiency === '';
        case 'roll':
          return d.roll === '';
        case 'targets':
          return d.targets.length === 0;
        case 'sense':
          return d.sense === '';
        case 'range_ft':
          return rangeFeet(d.rangeM) <= 0;
        case 'resource':
          return d.resource.trim() === '';
        case 'max':
          return d.max.trim() === '';
        case 'recharge':
          return d.recharge === '';
        case 'choice':
          return d.choice === '';
        case 'count':
          return d.count <= 0;
        case 'economy':
          return d.economy === '';
        default:
          return false;
      }
    })();
    if (empty) {
      out.push(f.name);
    }
  }
  return out;
}
