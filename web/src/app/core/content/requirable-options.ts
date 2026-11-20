import { type TableEntry, TableContentKind } from '../../../gen/meurpg/rules/v1/table_content_pb';
import type { SelectOption } from '../../shared/form-fields/select-field';
import type { EffectMenuVm } from './effect-draft';
import type { FeatureDraft } from './feature-draft';
import { OFFERERS } from './option-draft';

/**
 * The options a prerequisite may name (#580): an option's (S1) and a feat's. The SRD's options of every set, and the table's own
 * (`@mesa`) with "(da mesa)" after the name; a Pact Boon is one of the set "pact_boon".
 */

const byName = (a: SelectOption, b: SelectOption): number =>
  a.label.localeCompare(b.label, 'pt-BR');

/** The SRD's options of these sets, with the set's prefix taken off the name. */
export function srdOptionsOf(
  menu: EffectMenuVm,
  sets: readonly string[],
): { key: string; namePt: string }[] {
  return menu.optionSets
    .filter((s) => sets.includes(s.key))
    .flatMap((s) => s.options.filter((o) => !o.key.endsWith('@mesa')))
    .map((o) => ({ key: o.key, namePt: o.namePt.replace(/^[^:]+: /, '') }));
}

function tableOptionsOf(entries: readonly TableEntry[]): readonly TableEntry[] {
  return entries.filter((e) => e.kind === TableContentKind.OPTION && !e.archived);
}

/** What an entry may require, by name: every SRD option and the table's, but `own` (an option never requires itself). */
export function requirableOptions(
  menu: EffectMenuVm,
  entries: readonly TableEntry[],
  own = '',
  siblings: readonly SelectOption[] = [],
): SelectOption[] {
  const srd = srdOptionsOf(menu, Object.values(OFFERERS).flat());
  const table = tableOptionsOf(entries).filter((o) => o.key !== own);
  return [
    ...siblings.filter((o) => o.value !== own),
    ...[
      ...srd.map((o) => ({ value: o.key, label: o.namePt })),
      ...table.map((o) => ({ value: o.key, label: `${o.namePt} (da mesa)` })),
    ].sort(byName),
  ];
}

/** "Nenhuma", then the Pact Boons: the SRD's and the table's. */
export function pactChoices(menu: EffectMenuVm, entries: readonly TableEntry[]): SelectOption[] {
  return [
    { value: '', label: 'Nenhuma' },
    ...srdOptionsOf(menu, OFFERERS['pact_boon']).map((o) => ({ value: o.key, label: o.namePt })),
    ...tableOptionsOf(entries)
      .filter((o) => o.body.case === 'tableOption' && o.body.value.set === 'pact_boon')
      .map((o) => ({ value: o.key, label: `${o.namePt} (da mesa)` })),
  ];
}

/**
 * The options of the same feature an option may ask for (#580, board 580-03): the saved ones (those with a key), each named
 * as the list names it ("Opção 2"), never the option itself. One not saved has no key yet and cannot be asked for.
 */
export function siblingOptions(list: readonly FeatureDraft[], own: string): SelectOption[] {
  return list.flatMap((o, i) =>
    o.key === '' || o.key === own
      ? []
      : [{ value: o.key, label: o.name.trim() || `Opção ${i + 1}`, badge: `Opção ${i + 1}` }],
  );
}
