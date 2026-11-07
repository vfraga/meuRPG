import { type TableEntry, TableContentKind } from '../../../gen/meurpg/rules/v1/table_content_pb';
import type { SearchChip, SearchGroup, SearchOption } from '../../shared/form-fields/search-pick';
import type { MenuOption } from './effect-draft';

/*
 * What the feat editor searches.
 * The races and sub-races a feat may ask for come from the catalog.
 * The feature, trait or feat it may ask the character not to have comes from two sources.
 * The SRD's come from the effect menu (`lackable_features`).
 * The table's own come from its entries, which the menu does not list.
 */

export const LACK_GROUPS: readonly SearchGroup[] = [
  { id: 'feat', title: 'Talentos', empty: 'Nenhum talento com esse nome.' },
  { id: 'feature', title: 'Características', empty: 'Nenhuma característica com esse nome.' },
  { id: 'trait', title: 'Traços', empty: 'Nenhum traço com esse nome.' },
];

/** A feature a feat may ask not to have. */
export interface LackOption extends SearchOption {
  /** An archived one is named on its chip but not offered again. */
  readonly archived: boolean;
}

/** The group of an SRD value, from the prefix of its key. */
function srdGroup(key: string): string {
  if (key.startsWith('feat:')) {
    return 'feat';
  }
  return key.startsWith('trait:') ? 'trait' : 'feature';
}

/** Every feature, trait and feat the master may ask not to have: the SRD's (menu) and the table's own (entries). */
export function lackOptions(
  menu: readonly MenuOption[],
  entries: readonly TableEntry[],
  nameOf: (key: string) => string,
): LackOption[] {
  const out: LackOption[] = menu.map((m) => ({
    value: m.key,
    label: m.namePt,
    hint: m.hintPt,
    group: srdGroup(m.key),
    archived: false,
  }));
  const add = (value: string, label: string, hint: string, group: string, archived: boolean) =>
    out.push({ value, label, hint, group, archived });
  for (const e of entries) {
    const body = e.body;
    switch (body.case) {
      case 'tableFeat':
        add(e.key, e.namePt, 'talento da mesa', 'feat', e.archived);
        break;
      case 'tableClassFeature': {
        const at = body.value.level > 0 ? `, nível ${body.value.level}` : '';
        add(e.key, e.namePt, `${nameOf(body.value.classKey)} da mesa${at}`, 'feature', e.archived);
        break;
      }
      case 'tableRace':
      case 'tableSubrace':
        for (const t of body.value.traits) {
          add(t.key, t.namePt, `traço de ${e.namePt}`, 'trait', e.archived);
        }
        break;
      case 'tableBackground':
        if (body.value.feature) {
          add(
            body.value.feature.key,
            body.value.feature.namePt,
            `antecedente ${e.namePt}`,
            'feature',
            e.archived,
          );
        }
        break;
      case 'tableClass':
        body.value.levels.forEach((l, i) =>
          l.features.forEach((f) =>
            add(f.key, f.namePt, `${e.namePt} da mesa, nível ${i + 1}`, 'feature', e.archived),
          ),
        );
        break;
      case 'tableSubclass':
        for (const l of body.value.levels) {
          for (const f of l.features) {
            add(f.key, f.namePt, `${e.namePt} da mesa, nível ${l.level}`, 'feature', e.archived);
          }
        }
        break;
    }
  }
  return out.filter((o) => o.value !== '');
}

/** What the search offers: not the archived, not the feat being edited, not what is already chosen. */
export function pickableLacks(
  all: readonly LackOption[],
  own: string,
  chosen: string,
): LackOption[] {
  return all.filter((o) => !o.archived && o.value !== own && o.value !== chosen);
}

/** The chip of the chosen feature: its name and where it comes from; the name alone for a key no list knows. */
export function lackChips(
  key: string,
  all: readonly LackOption[],
  nameOf: (key: string) => string,
  invalid = false,
): SearchChip[] {
  if (key === '') {
    return [];
  }
  const found = all.find((o) => o.value === key);
  return [
    { value: key, label: found?.label ?? nameOf(key), hint: found?.hint ?? '', tag: '', invalid },
  ];
}

/**
 * The chips of the races asked.
 * "Desligada" marks one the master switched off or archived, which the players do not read.
 * `refused` maps a key to the path its refusal lands on.
 */
export function raceChips(
  keys: readonly string[],
  entries: readonly TableEntry[],
  nameOf: (key: string) => string,
  refused: ReadonlyMap<string, string> = new Map(),
): SearchChip[] {
  return keys.map((key) => {
    const entry = entries.find((e) => e.key === key && isRaceKind(e.kind));
    return {
      value: key,
      label: nameOf(key),
      hint: '',
      tag: entry && (entry.off || entry.archived) ? 'Desligada' : '',
      invalid: refused.has(key),
      path: refused.get(key),
    };
  });
}

function isRaceKind(kind: TableContentKind): boolean {
  return kind === TableContentKind.RACE || kind === TableContentKind.SUBRACE;
}

/** The races to add: the catalog's, not the ones already asked. */
export function pickableRaces(
  choices: readonly { value: string; label: string }[],
  asked: readonly string[],
): SearchOption[] {
  return choices
    .filter((c) => !asked.includes(c.value))
    .map((c) => ({ value: c.value, label: c.label, hint: '', group: '' }));
}
