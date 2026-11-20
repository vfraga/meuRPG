import type { EffectDraft } from './effect-draft.types';
import type { EffectMenuVm } from './effect-draft';

/**
 * Where the options of a table choice come from, besides a list of keys the master writes (RN-33): for a choice of tools, every
 * proficiency of some kinds (`filter`); for a choice of features, the options of another feature (`from_feature`). The server
 * refuses a choice that names two sources, so changing the source clears the other fields, and what is written never combines them.
 * The lists are the menu's (`proficiency_kinds` and the proficiencies of each kind), so the count is worked out with no other call.
 */

/** A choice of another kind has none of these sources. */
export const noSource = (): Pick<EffectDraft, 'filter' | 'filterOn' | 'fromFeature'> => ({
  filter: [],
  filterOn: false,
  fromFeature: '',
});

export type ToolSource = 'list' | 'filter';

/** The source of a choice of tools: the filter or, by default, the list the master writes (empty: every tool). */
export function toolSourceOf(d: Pick<EffectDraft, 'filterOn'>): ToolSource {
  return d.filterOn ? 'filter' : 'list';
}

/** The effect after the master changed the source of a choice of tools: the other source is cleared. */
export function withToolSource(d: EffectDraft, source: ToolSource): EffectDraft {
  return source === 'filter'
    ? { ...d, filterOn: true, filter: [], from: [] }
    : { ...d, filterOn: false, filter: [] };
}

/** The effect with the kinds ticked, in the order of the menu's list, whatever order they were ticked in. */
export function withFilterKinds(
  d: EffectDraft,
  menu: EffectMenuVm,
  ticked: readonly string[],
): EffectDraft {
  const order = menu.list('proficiency_kinds').map((k) => k.key);
  const filter = order.filter((k) => ticked.includes(k));
  return { ...d, filterOn: true, filter };
}

/** The menu's list of the proficiencies of a kind ("weapon_proficiencies"). */
function proficienciesOf(menu: EffectMenuVm, kind: string): number {
  return menu.list(`${kind}_proficiencies`).length;
}

/** How many proficiencies the filter matches today: the sizes of the lists of the ticked kinds, added. */
export function filterMatches(menu: EffectMenuVm, kinds: readonly string[]): number {
  return kinds.reduce((sum, kind) => sum + proficienciesOf(menu, kind), 0);
}

/** "Oferece 37 proficiências hoje.", or what to do while no kind is ticked. */
export function filterLine(menu: EffectMenuVm, kinds: readonly string[]): string {
  if (kinds.length === 0) {
    return 'Marque ao menos um tipo.';
  }
  const n = filterMatches(menu, kinds);
  return n === 0
    ? 'Nenhuma proficiência destes tipos hoje.'
    : `Oferece ${n} ${n === 1 ? 'proficiência' : 'proficiências'} hoje.`;
}

/** A feature with options a choice of features may offer: the SRD's (the menu's option sets) and the table's. */
export interface FeatureOffer {
  readonly key: string;
  readonly label: string;
  /** How many options the feature offers today. */
  readonly count: number;
}

/** The eldritch invocations are dealt by the class table: the server refuses them as a source (`rules/choicefeature.go`). */
const INVOCATIONS = 'feature:eldritch-invocations';

/** The features that have options, by name: the SRD's first-class sets and the table's own ("Truques de lâmina (Guarda do Corvo)"). */
export function featureOffers(menu: EffectMenuVm): FeatureOffer[] {
  const srd = menu.optionSets
    .filter((s) => s.key !== INVOCATIONS && s.options.length > 0)
    .map((s) => ({ key: s.key, label: s.namePt, count: s.options.length }));
  const table = menu.tableFeatures.map((f) => ({ key: f.key, label: f.namePt, count: f.count }));
  return [...srd, ...table].sort((a, b) => a.label.localeCompare(b.label, 'pt-BR'));
}

/** The effect after the master picked the feature whose options it offers ('' for none): the list, the set and the filter are cleared. */
export function withFromFeature(d: EffectDraft, key: string): EffectDraft {
  return key === ''
    ? { ...d, fromFeature: '' }
    : { ...d, fromFeature: key, from: [], set: '', filter: [], filterOn: false };
}

/** "Oferece 3 opções hoje.", or what the field does while empty; for a key the menu no longer lists, a line that says so. */
export function featureLine(offers: readonly FeatureOffer[], key: string): string {
  if (key === '') {
    return 'Vazio: a escolha usa a lista ou as opções desta característica.';
  }
  const n = offers.find((o) => o.key === key)?.count;
  if (n === undefined) {
    return 'Esta característica saiu da lista ou não tem mais opções. Escolha outra.';
  }
  return n === 0
    ? 'Esta característica não tem opção hoje.'
    : `Oferece ${n} ${n === 1 ? 'opção' : 'opções'} hoje.`;
}
