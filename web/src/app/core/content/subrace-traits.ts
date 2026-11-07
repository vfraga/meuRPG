import type { TableEntry, TableFeature } from '../../../gen/meurpg/rules/v1/table_content_pb';
import type { MenuOption } from './effect-draft';
import { type NameOf, type ParentView, effectLine } from './content-read-parts';

/**
 * The parent race's traits a subrace can replace (MR-025, RN-23).
 * They feed the checklist of the subrace editor and the lines the master and the player read.
 * A table race gives them from its entry; an SRD race gives them through the effect menu's `race_traits`.
 */

export interface ParentTrait {
  readonly key: string;
  readonly name: string;
  /** One line: "Proficiência em Percepção", "Só texto", "Escolha entre 2 opções". */
  readonly summary: string;
}

export interface TraitRow extends ParentTrait {
  readonly checked: boolean;
  /** The marked key is not a trait of the parent; the row stays until it is unmarked. */
  readonly stale: boolean;
}

/** What a trait does in one line. */
export function traitSummary(f: TableFeature, nameOf: NameOf): string {
  if (f.options.length > 0) {
    return `Escolha entre ${f.options.length} opções`;
  }
  const line = f.effects.map((e) => effectLine(e, nameOf)).find((l) => l !== '');
  if (line) {
    return line;
  }
  return f.effects.length > 0 ? 'Com efeito' : 'Só texto';
}

/** The saved traits of a table race entry (a trait with no key is not saved yet and cannot be replaced), or `null` when the key is no table race the reader has. */
export function parentFeaturesOf(
  entries: readonly TableEntry[],
  raceKey: string,
): readonly TableFeature[] | null {
  const body = entries.find((e) => e.key === raceKey)?.body;
  return body?.case === 'tableRace' ? body.value.traits.filter((t) => t.key !== '') : null;
}

/** What a reader of a subrace sees of its race: the table race's traits when the reader has that entry, none otherwise. */
export function parentViewOf(entries: readonly TableEntry[]): (raceKey: string) => ParentView {
  return (raceKey) => ({ traits: parentFeaturesOf(entries, raceKey) ?? [] });
}

export function parentTraitsOf(features: readonly TableFeature[], nameOf: NameOf): ParentTrait[] {
  return features.map((t) => ({ key: t.key, name: t.namePt, summary: traitSummary(t, nameOf) }));
}

/** The traits of an SRD race, from the menu's `race_traits` list (the server's, with a one-line summary where the SRD has one). */
export function srdParentTraits(list: readonly MenuOption[], raceKey: string): ParentTrait[] {
  return list
    .filter((t) => t.group === raceKey)
    .map((t) => ({ key: t.key, name: t.namePt, summary: t.hintPt }));
}

/** The checklist: every parent trait, marked or not, then the marked keys that are no parent trait. */
export function traitRows(parent: readonly ParentTrait[], marked: readonly string[]): TraitRow[] {
  const known = new Set(parent.map((t) => t.key));
  return [
    ...parent.map((t) => ({ ...t, checked: marked.includes(t.key), stale: false })),
    ...marked
      .filter((k) => !known.has(k))
      .map((key) => ({ key, name: '', summary: '', checked: true, stale: true })),
  ];
}

/** The keys with `key` marked or unmarked; order kept, no repeat. */
export function withReplaced(keys: readonly string[], key: string, on: boolean): string[] {
  const rest = keys.filter((k) => k !== key);
  return on ? [...rest, key] : rest;
}

/** The names of the replaced traits found in the parent's list; any other key is left out (RN-23). */
export function replacedNames(parent: readonly ParentTrait[], marked: readonly string[]): string[] {
  return parent.filter((t) => marked.includes(t.key)).map((t) => t.name);
}

/** What a subrace replaces, as the master's note writes it: "Olhos de caçador e os bônus de Corujeiro"; '' when nothing. */
export function replacesLine(names: readonly string[], bonusesOf: string): string {
  const parts = [...names, ...(bonusesOf ? [`os bônus de ${bonusesOf}`] : [])];
  return parts.length <= 1
    ? (parts[0] ?? '')
    : `${parts.slice(0, -1).join(', ')} e ${parts[parts.length - 1]}`;
}
