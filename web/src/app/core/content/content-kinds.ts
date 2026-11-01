import { TableContentKind, type TableEntry } from '../../../gen/meurpg/rules/v1/table_content_pb';
import { joinDots, tight } from '../format/text';
import { prerequisiteSupport } from './feat-text';
import { classFeatureSupport } from './class-feature-text';
import { optionSupport } from './option-text';
import { metersText } from '../units';

/**
 * "Conteúdo da mesa" (MR-025, RN-23, E10-01): what the list says about the table's entries, as pure
 * functions so the filters, the counts and the state words are tested without a screen. The server
 * decides who sees what: a player's list already has no archived entry and no count; nothing here
 * hides or reveals anything by itself, it only writes what the entry carries.
 */

/** The kinds of the left menu, in its order. Subraces are listed under their race's own kind (E10-03's
 * "Raças" group holds them) but they are entries too: they sit in "Raças", after the races. */
export interface ContentNavKind {
  /** The URL segment of "Nova …" and the list's key. */
  readonly slug:
    | 'classes'
    | 'subclasses'
    | 'races'
    | 'backgrounds'
    | 'spells'
    | 'feats'
    | 'options'
    | 'classfeatures';
  /** Plural, the nav item and the panel title. */
  readonly plural: string;
  /** Singular with its article, for "Nova raça" and "Voltar para Raças". */
  readonly singular: string;
  /** The kinds the list shows. */
  readonly kinds: readonly TableContentKind[];
  /** "Nova classe": the one filled button of the list. */
  readonly newLabel: string;
  /** "Já existe uma raça da mesa com este nome." */
  readonly aOne: string;
  /** Whether the app has an editor for it. */
  readonly editable: boolean;
  /** The master's way in: the page of "Nova …" (empty when there is no editor yet). */
  readonly createSegment: string;
}

export const CONTENT_NAV: readonly ContentNavKind[] = [
  {
    slug: 'classes',
    plural: 'Classes',
    singular: 'classe',
    kinds: [TableContentKind.CLASS],
    newLabel: 'Nova classe',
    aOne: 'uma classe',
    editable: true,
    createSegment: 'class',
  },
  {
    slug: 'subclasses',
    plural: 'Subclasses',
    singular: 'subclasse',
    kinds: [TableContentKind.SUBCLASS],
    newLabel: 'Nova subclasse',
    aOne: 'uma subclasse',
    editable: true,
    createSegment: 'subclass',
  },
  {
    slug: 'races',
    plural: 'Raças',
    singular: 'raça',
    kinds: [TableContentKind.RACE, TableContentKind.SUBRACE],
    newLabel: 'Nova raça',
    aOne: 'uma raça',
    editable: true,
    createSegment: 'race',
  },
  {
    slug: 'backgrounds',
    plural: 'Antecedentes',
    singular: 'antecedente',
    kinds: [TableContentKind.BACKGROUND],
    newLabel: 'Novo antecedente',
    aOne: 'um antecedente',
    editable: true,
    createSegment: 'background',
  },
  {
    slug: 'spells',
    plural: 'Magias',
    singular: 'magia',
    kinds: [TableContentKind.SPELL],
    newLabel: 'Nova magia',
    aOne: 'uma magia',
    editable: true,
    createSegment: 'spell',
  },
  {
    slug: 'feats',
    plural: 'Talentos',
    singular: 'talento',
    kinds: [TableContentKind.FEAT],
    newLabel: 'Novo talento',
    aOne: 'um talento',
    editable: true,
    createSegment: 'feat',
  },
  {
    slug: 'options',
    plural: 'Opções',
    singular: 'opção',
    kinds: [TableContentKind.OPTION],
    newLabel: 'Nova opção',
    aOne: 'uma opção',
    editable: true,
    createSegment: 'option',
  },
  {
    slug: 'classfeatures',
    plural: 'Características de classe',
    singular: 'característica de classe',
    kinds: [TableContentKind.CLASS_FEATURE],
    newLabel: 'Nova característica de classe',
    aOne: 'uma característica de classe',
    editable: true,
    createSegment: 'classfeature',
  },
];

export function navOfKind(kind: TableContentKind): ContentNavKind | undefined {
  return CONTENT_NAV.find((n) => n.kinds.includes(kind));
}

export function navBySlug(slug: string | null | undefined): ContentNavKind | undefined {
  return CONTENT_NAV.find((n) => n.slug === slug);
}

/** The singular of one entry's kind ("Raça da mesa" is `${kindWord} da mesa`). */
export const KIND_WORDS: Readonly<Record<number, string>> = {
  [TableContentKind.CLASS]: 'Classe',
  [TableContentKind.SUBCLASS]: 'Subclasse',
  [TableContentKind.RACE]: 'Raça',
  [TableContentKind.SUBRACE]: 'Sub-raça',
  [TableContentKind.BACKGROUND]: 'Antecedente',
  [TableContentKind.SPELL]: 'Magia',
  [TableContentKind.FEAT]: 'Talento',
  [TableContentKind.OPTION]: 'Opção',
  [TableContentKind.CLASS_FEATURE]: 'Característica de classe',
  [TableContentKind.CREATURE]: 'Criatura',
};

/** The noun of a kind with its article and the words that agree with it: "A magia … foi salva", "O antecedente … foi salvo". */
export interface KindNoun {
  readonly article: 'A' | 'O';
  readonly noun: string;
  readonly saved: string;
  readonly archived: string;
}

export const KIND_NOUNS: Readonly<Record<number, KindNoun>> = {
  [TableContentKind.CLASS]: { article: 'A', noun: 'classe', saved: 'salva', archived: 'arquivada' },
  [TableContentKind.SUBCLASS]: {
    article: 'A',
    noun: 'subclasse',
    saved: 'salva',
    archived: 'arquivada',
  },
  [TableContentKind.RACE]: { article: 'A', noun: 'raça', saved: 'salva', archived: 'arquivada' },
  [TableContentKind.SUBRACE]: {
    article: 'A',
    noun: 'sub-raça',
    saved: 'salva',
    archived: 'arquivada',
  },
  [TableContentKind.BACKGROUND]: {
    article: 'O',
    noun: 'antecedente',
    saved: 'salvo',
    archived: 'arquivado',
  },
  [TableContentKind.SPELL]: { article: 'A', noun: 'magia', saved: 'salva', archived: 'arquivada' },
  [TableContentKind.FEAT]: { article: 'O', noun: 'talento', saved: 'salvo', archived: 'arquivado' },
  [TableContentKind.OPTION]: { article: 'A', noun: 'opção', saved: 'salva', archived: 'arquivada' },
  [TableContentKind.CLASS_FEATURE]: {
    article: 'A',
    noun: 'característica de classe',
    saved: 'salva',
    archived: 'arquivada',
  },
  [TableContentKind.CREATURE]: {
    article: 'A',
    noun: 'criatura',
    saved: 'salva',
    archived: 'arquivada',
  },
};

/** "A magia Lâmina de Nanquim foi salva." */
export function savedSentence(kind: TableContentKind, name: string): string {
  const n = KIND_NOUNS[kind];
  return `${n.article} ${n.noun} ${name} foi ${n.saved}.`;
}

/** "Mostrar": which entries the list shows. */
export type ContentFilter = 'all' | 'inUse' | 'unused' | 'archived';

export const CONTENT_FILTERS: readonly { value: ContentFilter; label: string }[] = [
  { value: 'all', label: 'Todas' },
  { value: 'inUse', label: 'Em uso' },
  { value: 'unused', label: 'Sem fichas' },
  { value: 'archived', label: 'Arquivadas' },
];

/** The campaign's limit of entries (RN-23, `limit` with no field). */
export const CONTENT_LIMIT = 1000;

/** The master's count of one kind of the nav, archived entries included. */
export function countOfNav(entries: readonly TableEntry[], nav: ContentNavKind): number {
  return entries.filter((e) => nav.kinds.includes(e.kind)).length;
}

/** "7 de 1000 entradas · o limite de uma campanha". */
export function limitLine(entries: readonly TableEntry[]): string {
  return `${entries.length} de ${CONTENT_LIMIT} entradas · o limite de uma campanha`;
}

/** "7 entradas, 1 arquivada": the line the campaign page and the rules page carry. */
export function summaryLine(entries: readonly TableEntry[]): string {
  const archived = entries.filter((e) => e.archived).length;
  const total = `${entries.length} ${entries.length === 1 ? 'entrada' : 'entradas'}`;
  if (archived === 0) {
    return total;
  }
  return `${total}, ${archived} ${archived === 1 ? 'arquivada' : 'arquivadas'}`;
}

/** Lower case with no accents, so a search finds "Cartografo" for "Cartógrafo". */
export function plain(text: string): string {
  return text
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .toLowerCase()
    .trim();
}

/** What the list shows of one kind: the search, then the filter, in the server's order (sorted by name). Only the master
 * has counts; for a player `charactersUsing` is 0 and the filters other than "Todas" are not offered. */
export function filterEntries(
  entries: readonly TableEntry[],
  nav: ContentNavKind,
  query: string,
  filter: ContentFilter,
): TableEntry[] {
  const q = plain(query);
  const shown = entries.filter((e) => {
    if (!nav.kinds.includes(e.kind)) {
      return false;
    }
    if (q !== '' && !plain(e.namePt).includes(q)) {
      return false;
    }
    switch (filter) {
      case 'inUse':
        return e.charactersUsing > 0;
      case 'unused':
        return e.charactersUsing === 0 && !e.archived;
      case 'archived':
        return e.archived;
      default:
        return true;
    }
  });
  // Archived entries stay listed, at the end, so the master finds them and can bring them back (E10-01).
  return [...shown.filter((e) => !e.archived), ...shown.filter((e) => e.archived)];
}

/** The state of an entry in words (never colour alone): "Em uso por 2 fichas", "Arquivado · 1 ficha usa", or nothing. */
export interface EntryState {
  readonly text: string;
  /** Archived: the tag carries the archive icon and the row is muted. */
  readonly archived: boolean;
}

export function entryState(
  e: Pick<TableEntry, 'archived' | 'charactersUsing' | 'kind'> & { readonly off?: boolean },
  master: boolean,
): EntryState | null {
  if (!master) {
    return null;
  }
  const n = e.charactersUsing;
  const using = n > 0 ? `${n} ${n === 1 ? 'ficha usa' : 'fichas usam'}` : '';
  if (e.archived) {
    const word = KIND_NOUNS[e.kind]?.archived ?? 'arquivada';
    const label = word.charAt(0).toUpperCase() + word.slice(1);
    return { text: using ? `${label} · ${using}` : label, archived: true };
  }
  if (e.off) {
    // "Opções para os jogadores" (RN-23): the master still reads and edits it; the players do not receive it.
    const word = KIND_NOUNS[e.kind]?.article === 'O' ? 'Desligado' : 'Desligada';
    return {
      text: using ? `${word} para os jogadores · ${using}` : `${word} para os jogadores`,
      archived: false,
    };
  }
  return n > 0
    ? { text: `Em uso por ${n} ${n === 1 ? 'ficha' : 'fichas'}`, archived: false }
    : null;
}

/** "2 fichas usam Corujeiro agora.", "Nenhuma ficha usa Corujeiro agora." */
export function usageSentence(name: string, n: number): string {
  if (n === 0) {
    return `Nenhuma ficha usa ${name} agora.`;
  }
  return `${n} ${n === 1 ? 'ficha usa' : 'fichas usam'} ${name} agora.`;
}

type Body<K extends string> = Extract<TableEntry['body'], { case: K }>['value'];
type NameOf = (key: string) => string;

/** The level a class chooses its subclass at when the entry leaves it out. */
export const DEFAULT_SUBCLASS_LEVEL = 3;

function classSupport(c: Body<'tableClass'>): string[] {
  const casting = c.casting
    ? `conjuração ${CASTING_WORDS[c.casting.kind] ?? c.casting.kind}`
    : 'sem conjuração';
  return [
    `d${c.hitDie}`,
    casting,
    `subclasse no nível ${c.subclassLevel || DEFAULT_SUBCLASS_LEVEL}`,
  ];
}

function subclassSupport(
  s: Body<'tableSubclass'>,
  nameOf: NameOf,
  subclassLevelOf: (classKey: string) => number,
): string[] {
  const parts = [`Subclasse de ${nameOf(s.classKey)}`];
  // Level 0 is the class's own level of choosing; the catalog says it.
  const level = s.level || subclassLevelOf(s.classKey);
  if (level > 0) {
    parts.push(`nível ${level}`);
  }
  return parts;
}

function raceSupport(r: Body<'tableRace'>): string[] {
  const parts = [SIZE_WORDS[r.size] ?? r.size, metersText(r.speedFt)];
  if (r.darkvisionFt > 0) {
    parts.push(tight(`visão no escuro ${metersText(r.darkvisionFt)}`));
  }
  return parts;
}

function featSupport(f: Body<'tableFeat'>, nameOf: NameOf): string[] {
  const says = prerequisiteSupport(f.prerequisite, nameOf);
  return [says === '' ? 'Sem pré-requisito' : says];
}

/** The one-line support of a list row, from the body: "d10 · conjuração de metade, Sabedoria · subclasse no nível 3". */
export function entrySupport(
  e: TableEntry,
  nameOf: NameOf,
  subclassLevelOf: (classKey: string) => number = () => 0,
): string {
  return joinDots(supportParts(e.body, nameOf, subclassLevelOf));
}

function supportParts(
  body: TableEntry['body'],
  nameOf: NameOf,
  subclassLevelOf: (classKey: string) => number,
): string[] {
  switch (body.case) {
    case 'tableClass':
      return classSupport(body.value);
    case 'tableSubclass':
      return subclassSupport(body.value, nameOf, subclassLevelOf);
    case 'tableRace':
      return raceSupport(body.value);
    case 'tableSubrace':
      return [`Sub-raça de ${nameOf(body.value.raceKey)}`];
    case 'tableBackground': {
      const n = body.value.skills.length;
      return [`${n} ${n === 1 ? 'perícia' : 'perícias'}`];
    }
    case 'tableSpell':
      return [body.value.level === 0 ? 'Truque' : `${body.value.level}º nível`];
    case 'tableFeat':
      return featSupport(body.value, nameOf);
    case 'tableOption':
      return optionSupport(body.value.set, body.value.prerequisite, nameOf);
    case 'tableClassFeature':
      return classFeatureSupport(
        body.value.classKey,
        body.value.level,
        body.value.replacesFeatureKey,
        nameOf,
      );
    default:
      return [];
  }
}

export const SIZE_WORDS: Readonly<Record<string, string>> = {
  Tiny: 'Miúdo',
  Small: 'Pequeno',
  Medium: 'Médio',
  Large: 'Grande',
};

export const CASTING_WORDS: Readonly<Record<string, string>> = {
  full: 'completa',
  half: 'de metade',
  pact: 'de pacto',
  third: 'de um terço',
};
