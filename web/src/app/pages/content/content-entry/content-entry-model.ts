import type { MatBottomSheet } from '@angular/material/bottom-sheet';
import type { MatDialog } from '@angular/material/dialog';
import { timestampDate } from '@bufbuild/protobuf/wkt';

import {
  TableContentKind,
  type TableContentViolation,
  type TableEntry,
} from '../../../../gen/meurpg/rules/v1/table_content_pb';
import { formatDateTime } from '../../../core/characters/character-labels';
import type { CatalogVm } from '../../../core/content/catalog';
import { contentErrorText } from '../../../core/content/content-client';
import type { ContentContext } from '../../../core/content/content-context';
import {
  type ContentNavKind,
  KIND_NOUNS,
  KIND_WORDS,
  usageSentence,
} from '../../../core/content/content-kinds';
import {
  type LeftOutModel,
  type LeftOutNote,
  NO_LEFT_OUT,
  buildLeftOut,
  leftOutNote,
  violationOfEntry,
} from '../../../core/content/left-out';
import { readEntry } from '../../../core/content/content-read';
import type { EntryRead, NameOf } from '../../../core/content/content-read-parts';
import { parentViewOf } from '../../../core/content/subrace-traits';
import type { EffectMenuVm } from '../../../core/content/effect-draft';
import { openSheet } from '../../../shared/sheet/sheet-host';
import { ArchiveSheet } from '../archive/archive-sheet';

/** The URL's word for the kind of a new entry ("new/spell"). */
export const NEW_KINDS: Readonly<Record<string, TableContentKind>> = {
  spell: TableContentKind.SPELL,
  race: TableContentKind.RACE,
  subrace: TableContentKind.SUBRACE,
  background: TableContentKind.BACKGROUND,
  class: TableContentKind.CLASS,
  subclass: TableContentKind.SUBCLASS,
  feat: TableContentKind.FEAT,
  option: TableContentKind.OPTION,
  classfeature: TableContentKind.CLASS_FEATURE,
  creature: TableContentKind.CREATURE,
};

export const NEW_TITLES: Readonly<Record<number, string>> = {
  [TableContentKind.SPELL]: 'Nova magia',
  [TableContentKind.RACE]: 'Nova raça',
  [TableContentKind.SUBRACE]: 'Nova sub-raça',
  [TableContentKind.BACKGROUND]: 'Novo antecedente',
  [TableContentKind.CLASS]: 'Nova classe',
  [TableContentKind.SUBCLASS]: 'Nova subclasse',
  [TableContentKind.FEAT]: 'Novo talento',
  [TableContentKind.OPTION]: 'Nova opção',
  [TableContentKind.CLASS_FEATURE]: 'Nova característica de classe',
  [TableContentKind.CREATURE]: 'Nova criatura',
};

/** The table's own feats a "feat" choice may list (the catalog has no feats; the SRD's are not offered by name). */
function featChoices(
  entries: readonly TableEntry[],
): { key: string; namePt: string; hintPt: string }[] {
  return entries
    .filter((e) => e.kind === TableContentKind.FEAT && !e.archived)
    .map((e) => ({ key: e.key, namePt: e.namePt, hintPt: '' }));
}

export const EDITABLE = new Set<TableContentKind>([
  TableContentKind.SPELL,
  TableContentKind.RACE,
  TableContentKind.SUBRACE,
  TableContentKind.BACKGROUND,
  TableContentKind.CLASS,
  TableContentKind.SUBCLASS,
  TableContentKind.FEAT,
  TableContentKind.OPTION,
  TableContentKind.CLASS_FEATURE,
  TableContentKind.CREATURE,
]);

/** The entries a read-again brings, with the one being edited kept as it is typed: when another write changed its body and
 * revision, only the switches and the counts follow. A reader adopts the new entry whole. */
export function mergeRefreshed(
  current: readonly TableEntry[],
  incoming: readonly TableEntry[],
  openKey: string,
  editing: boolean,
): TableEntry[] {
  const open = current.find((e) => e.key === openKey);
  return incoming.map((e) =>
    // The entry being edited keeps its body and revision when another write changed them: what is typed is not thrown
    // away, and "Salvar" says the entry changed (stale) as it always did. Only the switches and the counts follow.
    // A reader (a player, or a master with no editor open) adopts the new entry whole.
    editing && open && e.key === open.key && e.revision !== open.revision
      ? ({
          ...open,
          off: e.off,
          archived: e.archived,
          archivedAt: e.archivedAt,
          charactersUsing: e.charactersUsing,
        } as TableEntry)
      : e,
  );
}

/** The list after a save: the entry in its place, or, when it is new, among the others by name. */
export function entriesAfterSave(
  entries: readonly TableEntry[],
  saved: TableEntry,
): readonly TableEntry[] {
  return entries.some((e) => e.key === saved.key)
    ? entries.map((e) => (e.key === saved.key ? saved : e))
    : [...entries, saved].sort((a, b) => a.namePt.localeCompare(b.namePt, 'pt-BR'));
}

/** The list with one entry as the server has it now. */
export function replaceEntry(entries: readonly TableEntry[], entry: TableEntry): TableEntry[] {
  return entries.map((x) => (x.key === entry.key ? entry : x));
}

/** The settled reads of the catalog, the effect menu and the class defaults: what the page says for each that failed. */
export function extrasErrors(
  results: readonly [
    PromiseSettledResult<unknown>,
    PromiseSettledResult<unknown>,
    PromiseSettledResult<unknown>,
  ],
): {
  catalog: string;
  menu: string;
  defaults: string;
} {
  const [c, m, df] = results;
  return {
    defaults: df.status === 'rejected' ? contentErrorText(df.reason, 'abrir o editor') : '',
    catalog: c.status === 'rejected' ? contentErrorText(c.reason, 'abrir esta entrada') : '',
    menu: m.status === 'rejected' ? contentErrorText(m.reason, 'abrir o editor') : '',
  };
}

const FALLBACK_HEADING = 'Conteúdo da mesa';
/** The length of "dd/mm/aaaa": the header shows the day, not the time. */
const DAY_LENGTH = 10;

/** The page's title: the entry's name, or "Nova magia" for a new one. */
export function headingOf(entry: TableEntry | null, newKind: TableContentKind | null): string {
  if (entry) {
    return entry.namePt;
  }
  return newKind === null ? FALLBACK_HEADING : (NEW_TITLES[newKind] ?? FALLBACK_HEADING);
}

/** "Atualizada em 12/03/2026", or empty for an entry never written. */
export function updatedLine(entry: TableEntry | null): string {
  return entry?.updatedAt
    ? `Atualizada em ${formatDateTime(timestampDate(entry.updatedAt)).slice(0, DAY_LENGTH)}`
    : '';
}

/** The day an entry was archived, or empty. */
export function archivedDay(entry: TableEntry | null): string {
  return entry?.archivedAt
    ? formatDateTime(timestampDate(entry.archivedAt)).slice(0, DAY_LENGTH)
    : '';
}

/** "A raça Corujeiro" / "O antecedente Cartógrafo": the noun of the kind with its article, so the copy agrees. */
export function nounPhraseOf(entry: TableEntry | null): string {
  const n = entry ? KIND_NOUNS[entry.kind] : undefined;
  return entry && n ? `${n.article} ${n.noun} ${entry.namePt}` : '';
}

/** "arquivada" / "arquivado": the participle that agrees with the kind. */
export function archivedWordOf(entry: TableEntry | null): string {
  return (entry ? KIND_NOUNS[entry.kind]?.archived : undefined) ?? 'arquivada';
}

/** The name of a content key: the catalog's, else the table's own entry, else the key without its prefix. */
export function nameResolver(
  catalog: CatalogVm | null,
  entries: readonly TableEntry[],
): (key: string) => string {
  return (key) =>
    catalog?.nameOf(key) ??
    entries.find((e) => e.key === key)?.namePt ??
    key.replace(/^[a-z-]+:/, '');
}

/** "Arquivar Guardião do Vale?" as a bottom sheet on a phone: whether the person confirmed. */
export function confirmArchiveInSheet(
  dialog: MatDialog,
  bottomSheet: MatBottomSheet,
  entry: TableEntry,
): Promise<boolean> {
  return new Promise<boolean>((resolve) =>
    openSheet<ArchiveSheet, { name: string; using: number }, boolean>(
      dialog,
      bottomSheet,
      ArchiveSheet,
      {
        data: { name: entry.namePt, using: entry.charactersUsing },
        ariaLabel: `Arquivar ${entry.namePt}?`,
        labelledBy: 'archive-t',
      },
    ).subscribe((r) => resolve(r === true)),
  );
}

/** The editors that do not use the effect menu: the spell's and the creature's. */
export function usesNoMenu(kind: TableContentKind | null): boolean {
  return kind === TableContentKind.SPELL || kind === TableContentKind.CREATURE;
}

/** What blocks the editor: the catalog always, the menu for every editor that uses it, the defaults for the class and the subclass. */
export function extrasErrorOf(e: {
  catalog: string;
  menu: string;
  defaults: string;
  noMenu: boolean;
  needsDefaults: boolean;
}): string {
  return e.catalog || (e.noMenu ? '' : e.menu) || (e.needsDefaults ? e.defaults : '');
}

/** "Voltar para Raças" for the master; the player's list is the whole table's content. */
export function backLabelOf(isMaster: boolean, nav: ContentNavKind | undefined): string {
  return isMaster
    ? `Voltar para ${nav?.plural ?? FALLBACK_HEADING}`
    : `Voltar para ${FALLBACK_HEADING}`;
}

/** The race a subrace belongs to: its own, or the one the link names for a new one. */
export function subraceParentOf(entry: TableEntry | null, linked: string): string {
  const body = entry?.body;
  return body?.case === 'tableSubrace' ? body.value.raceKey : linked;
}

/** The effect menu names a class by the catalog and offers the table's own feats once both are there. */
export function wireMenu(
  menu: EffectMenuVm | null,
  catalog: CatalogVm | null,
  entries: readonly TableEntry[],
): void {
  if (menu && catalog) {
    menu.classNamePt = (key) => catalog.nameOf(key);
    menu.featOptions = featChoices(entries);
  }
}

/** "Raça da mesa": the kind of the open entry, or nothing before there is one. */
export function kindTagOf(kind: TableContentKind | null): string {
  return kind === null ? '' : `${KIND_WORDS[kind]} da mesa`;
}

/** "Em uso por 2 fichas", or nothing without an entry. */
export function usageOf(entry: TableEntry | null): string {
  return entry ? usageSentence(entry.namePt, entry.charactersUsing) : '';
}

/** What a player (or the master, where the app has no editor yet) reads of the entry, with the traits of the race a subrace comes from. */
export function readOfEntry(
  entry: TableEntry | null,
  nameOf: NameOf,
  catalog: CatalogVm | null,
  entries: readonly TableEntry[],
): EntryRead | null {
  return entry
    ? readEntry(
        entry,
        nameOf,
        (key) => catalog?.subclassLevelOf(key) ?? 0,
        parentViewOf(entries),
        catalog?.offeredByName,
      )
    : null;
}

/** What the page says of the entries the rules refuse (the master's only): the model, the violation to put on the editor's field
 * when this entry is out for a field of its own, and "Esta classe está fora da campanha." with what to do. */
export function leftOutView(
  ctx: ContentContext | null,
  key: string,
  editing: boolean,
): {
  readonly model: LeftOutModel;
  readonly violation: TableContentViolation | null;
  readonly note: LeftOutNote | null;
} {
  const model = ctx?.isMaster ? buildLeftOut(ctx.entries, ctx.leftOut) : NO_LEFT_OUT;
  const entry = ctx?.entries.find((e) => e.key === key);
  return {
    model,
    violation:
      ctx && model.rows.some((r) => r.entry.key === key)
        ? (violationOfEntry(ctx.leftOut, key) ?? null)
        : null,
    note: entry ? leftOutNote(entry, model, editing) : null,
  };
}
