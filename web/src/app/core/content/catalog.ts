import { Ability, type Content } from '../../../gen/meurpg/rules/v1/rules_pb';
import type { TableEntry } from '../../../gen/meurpg/rules/v1/table_content_pb';
import type { SelectOption } from '../../shared/form-fields/select-field';
import { featureKeyName } from './class-feature-text';
import { offeringFeatures, offeringLabel } from './offering-features';
import { circleLabel } from './spell-options';

/**
 * What the editors' pickers need from the campaign's catalog (`ContentService.ListContent`): the schools of magic, the
 * classes that cast, the skills and the spells, with their Portuguese names. The keys are the content's; nothing is
 * listed by hand here except the order.
 */
export interface CatalogAbility {
  /** The field of `AbilityScores` ("strength"): the path of a bonus, the key of a draft. */
  readonly field:
    'strength' | 'dexterity' | 'constitution' | 'intelligence' | 'wisdom' | 'charisma';
  readonly ability: Ability;
  /** "Força", from the catalog. */
  readonly name: string;
}

export interface CatalogVm {
  /** The six abilities in the sheet's order, with the server's names. */
  readonly abilities: readonly CatalogAbility[];
  /** The damage types, named by the server ("Fogo"), for a spell's damage. */
  readonly damageTypes: readonly SelectOption[];
  /** The races a sub-race may belong to: the SRD's and the table's, not the archived. */
  readonly races: readonly SelectOption[];
  /** The class level a subclass is chosen at, by the class key (0 when the catalog does not know it). */
  subclassLevelOf(classKey: string): number;
  readonly schools: readonly SelectOption[];
  /** The classes a spell can be on the list of: every casting class, the table's marked. */
  readonly castingClasses: readonly {
    readonly key: string;
    readonly name: string;
    readonly table: boolean;
  }[];
  /** The classes a subclass can belong to: the SRD's and the table's, not the archived. */
  readonly classes: readonly SelectOption[];
  readonly skills: readonly SelectOption[];
  /** The SRD's proficiencies a feat may ask for (armor, weapons, tools...), by name. */
  readonly proficiencies: readonly SelectOption[];
  /** The races and sub-races a feat may ask for, not the archived. */
  readonly raceChoices: readonly SelectOption[];
  readonly spells: readonly SelectOption[];
  /** The SRD's spells, not the table's, each named with its circle and school ("Auxílio (2º nível, Abjuração)"): what a class's own list can name. */
  readonly srdSpells: readonly SelectOption[];
  /** The Portuguese name of any key of the catalog and of the table's entries (the key itself when unknown). */
  nameOf(key: string): string;
  /** "Truques de lâmina (Guarda do Corvo)" for the key of a feature with an option list, archived owners included; else the key's last word. */
  offeredByName(key: string): string;
}

/** A table key ends with "@mesa". */
export function isTableKey(key: string): boolean {
  return key.endsWith('@mesa');
}

/** Every Portuguese name the catalog and the table's entries give, by key. */
function nameIndex(content: Content, entries: readonly TableEntry[]): Map<string, string> {
  const names = new Map<string, string>();
  for (const r of content.races) names.set(r.key, r.namePt);
  for (const r of content.subraces) names.set(r.key, r.namePt);
  for (const c of content.classes) names.set(c.key, c.namePt);
  for (const c of content.subclasses) names.set(c.key, c.namePt);
  for (const b of content.backgrounds) names.set(b.key, b.namePt);
  for (const s of content.skills) names.set(s.key, s.namePt);
  for (const s of content.spells) names.set(s.key, s.namePt);
  for (const e of entries) names.set(e.key, e.namePt);
  // What a table entry points at that is not an entry: the server names it for every member (a player has no menu).
  for (const n of content.languages) names.set(n.key, n.namePt);
  for (const n of content.proficiencies) names.set(n.key, n.namePt);
  for (const n of content.damageTypes) names.set(n.key, n.namePt);
  return names;
}

/** The first name given to each school of magic, in the order the spells list them. */
function schoolNames(spells: Content['spells']): Map<string, string> {
  const schools = new Map<string, string>();
  for (const s of spells) {
    if (s.schoolKey && !schools.has(s.schoolKey)) schools.set(s.schoolKey, s.schoolNamePt);
  }
  return schools;
}

export function catalogVm(content: Content, entries: readonly TableEntry[] = []): CatalogVm {
  const names = nameIndex(content, entries);
  const fields = [
    'strength',
    'dexterity',
    'constitution',
    'intelligence',
    'wisdom',
    'charisma',
  ] as const;
  const abilities: CatalogAbility[] = content.abilities.map((a, i) => ({
    field: fields[i],
    ability: a.ability,
    name: a.namePt,
  }));
  for (const a of abilities) {
    names.set(`ability:${a.field}`, a.name);
    names.set(`ability:${a.ability}`, a.name);
  }
  const offerers = new Map(offeringFeatures(entries, true).map((f) => [f.key, offeringLabel(f)]));
  const subclassLevels = new Map(content.classes.map((c) => [c.key, c.subclassLevel]));

  const schools = schoolNames(content.spells);
  const byName = (a: { label: string }, b: { label: string }) =>
    a.label.localeCompare(b.label, 'pt-BR');
  const capital = (text: string) => text.charAt(0).toUpperCase() + text.slice(1);
  return {
    abilities,
    damageTypes: content.damageTypes
      .map((d) => ({ value: d.key, label: capital(d.namePt) }))
      .sort(byName),
    races: content.races
      .filter((r) => !r.archived)
      .map((r) => ({ value: r.key, label: r.namePt }))
      .sort(byName),
    subclassLevelOf: (key) => subclassLevels.get(key) ?? 0,
    schools: [...schools].map(([value, label]) => ({ value, label })).sort(byName),
    castingClasses: content.classes
      .filter((c) => !c.archived && (c.spellcasting !== undefined || c.key.endsWith('@mesa')))
      .map((c) => ({ key: c.key, name: c.namePt, table: isTableKey(c.key) }))
      .sort((a, b) => Number(a.table) - Number(b.table) || a.name.localeCompare(b.name, 'pt-BR')),
    classes: content.classes
      .filter((c) => !c.archived)
      .map((c) => ({ value: c.key, label: isTableKey(c.key) ? `${c.namePt} (da mesa)` : c.namePt }))
      .sort(byName),
    proficiencies: content.proficiencies
      .map((p) => ({ value: p.key, label: capital(p.namePt) }))
      .sort(byName),
    raceChoices: [
      ...content.races.filter((r) => !r.archived).map((r) => ({ value: r.key, label: r.namePt })),
      ...content.subraces
        .filter((r) => !r.archived)
        .map((r) => ({ value: r.key, label: `${r.namePt} (sub-raça)` })),
    ].sort(byName),
    skills: content.skills.map((s) => ({ value: s.key, label: s.namePt })).sort(byName),
    spells: content.spells
      .filter((s) => !s.archived)
      .map((s) => ({ value: s.key, label: s.namePt }))
      .sort(byName),
    srdSpells: content.spells
      .filter((s) => !s.archived && !isTableKey(s.key))
      .map((s) => ({
        value: s.key,
        label: `${s.namePt} (${circleLabel(s.level)}, ${s.schoolNamePt})`,
      }))
      .sort(byName),
    nameOf: (key) => names.get(key) ?? key,
    offeredByName: (key) => offerers.get(key) ?? featureKeyName(key, (k) => k),
  };
}
