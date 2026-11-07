import {
  type TableEntry,
  type TableFeature,
  TableContentKind,
} from '../../../gen/meurpg/rules/v1/table_content_pb';

/**
 * A class or subclass feature of the table that has an option list of its own.
 * Only these can be the `offered_by` of a table option of the set "table_feature".
 */
export interface OfferingFeature {
  readonly key: string;
  readonly name: string;
  readonly level: number;
  /** The names of the options the feature lists itself. */
  readonly options: readonly string[];
  /** The owner entry: a subclass or a class of the table. */
  readonly ownerKey: string;
  readonly owner: string;
  readonly ownerKind: 'subclass' | 'class';
  /** The key of the class a subclass belongs to ('' for a class). */
  readonly classKey: string;
}

/** The offering features of the entries; the archived entries' only with `withArchived` (to name a saved choice, never to offer one). */
export function offeringFeatures(
  entries: readonly TableEntry[],
  withArchived = false,
): OfferingFeature[] {
  return entries.filter((e) => withArchived || !e.archived).flatMap(featuresOf);
}

/** "Truques de lâmina (Guarda do Corvo)": the feature with its owner. */
export function offeringLabel(f: OfferingFeature): string {
  return `${f.name} (${f.owner})`;
}

/** "Truques de lâmina (Guarda do Corvo, Guerreiro 3)": the feature, its owner, and the class and level it comes at. */
export function offeringRead(f: OfferingFeature, className: (classKey: string) => string): string {
  const at =
    f.ownerKind === 'subclass' ? `${className(f.classKey)} ${f.level}` : `nível ${f.level}`;
  return `${f.name} (${f.owner}, ${at})`;
}

/** "Guarda do Corvo · subclasse de Guerreiro" or "Guardião do Vale · classe da mesa". */
export function ownerLabel(f: OfferingFeature, className: (classKey: string) => string): string {
  return f.ownerKind === 'subclass'
    ? `${f.owner} · subclasse de ${className(f.classKey)}`
    : `${f.owner} · classe da mesa`;
}

type Row = { features: readonly TableFeature[] };

function featuresOf(e: TableEntry): OfferingFeature[] {
  const b = e.body;
  if (e.kind === TableContentKind.CLASS && b.case === 'tableClass') {
    // A class's rows are the 20 levels in order.
    return rowsOf(e, b.value.levels, 'class', '', (_, i) => i + 1);
  }
  if (e.kind === TableContentKind.SUBCLASS && b.case === 'tableSubclass') {
    return rowsOf(e, b.value.levels, 'subclass', b.value.classKey, (l) => l.level);
  }
  return [];
}

function rowsOf<L extends Row>(
  e: TableEntry,
  levels: readonly L[],
  kind: 'subclass' | 'class',
  classKey: string,
  levelOf: (row: L, index: number) => number,
): OfferingFeature[] {
  return levels.flatMap((row, i) =>
    row.features
      .filter((f) => f.options.length > 0 && f.key !== '')
      .map((f) => feature(f, levelOf(row, i), e, kind, classKey)),
  );
}

function feature(
  f: { key: string; namePt: string; options: readonly { namePt: string }[] },
  level: number,
  owner: TableEntry,
  ownerKind: 'subclass' | 'class',
  classKey: string,
): OfferingFeature {
  return {
    key: f.key,
    name: f.namePt,
    level,
    options: f.options.map((o) => o.namePt),
    ownerKey: owner.key,
    owner: owner.namePt,
    ownerKind,
    classKey,
  };
}
