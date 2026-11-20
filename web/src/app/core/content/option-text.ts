import type { OptionPrerequisite } from '../../../gen/meurpg/rules/v1/table_content_pb';
import { featureKeyName } from './class-feature-text';
import { type KeyName, listWith } from './feat-text';

/**
 * What a table option is and asks for, in Portuguese (MR-025): the set it joins and its prerequisite as phrases. The server
 * decides who qualifies (the choice lists the option blocked, with what it lacks); this only writes the entry for the
 * master's list and for the read page.
 */

/** The option sets a table option may join, by the key's word, as the server names them (`option_set_names`). */
export const OPTION_SET_WORDS: Readonly<Record<string, string>> = {
  invocation: 'Invocação mística',
  pact_boon: 'Dádiva do pacto',
  metamagic: 'Metamagia',
  fighting_style: 'Estilo de luta',
  hunters_prey: 'Presa do Caçador',
  defensive_tactics: 'Táticas defensivas',
  multiattack: 'Ataque múltiplo',
  superior_hunters_defense: 'Defesa superior do caçador',
  table_feature: 'Opção de característica da mesa',
};

/** "Invocação mística", for a set's word; the word itself for one the app does not know. */
export function optionSetName(set: string): string {
  return OPTION_SET_WORDS[set] ?? set;
}

/** The name of an option key: the table's by `nameOf`, else the last word of its key. */
export function optionKeyName(key: string, nameOf: KeyName): string {
  return featureKeyName(key, nameOf);
}

/** Names the feature that offers an option (`offered_by`), with its owner. */
export type OfferedByName = (key: string) => string;

/** The set of a list row: "Opção de Truques de lâmina (Guarda do Corvo)" when the server sent the offering feature, else the set's name. */
export function optionSetLine(
  set: string,
  offeredBy: string,
  offeredByName: OfferedByName,
): string {
  return offeredBy === '' ? optionSetName(set) : `Opção de ${offeredByName(offeredBy)}`;
}

/**
 * The conditions of an option's prerequisite, one phrase each, in the words and the order the feat editor uses (board 580-02):
 * the class level ("Bruxo 5"), the pact, the spell, then the options; none when it asks nothing.
 */
export function optionPrerequisiteParts(
  p: OptionPrerequisite | undefined,
  nameOf: KeyName,
): string[] {
  if (!p) {
    return [];
  }
  const out: string[] = [];
  if (p.classLevel && p.classLevel.classKey !== '') {
    out.push(`${nameOf(p.classLevel.classKey)} ${p.classLevel.level}`);
  }
  if (p.requiresPact !== '') {
    out.push(optionKeyName(p.requiresPact, nameOf));
  }
  if (p.requiresSpellKey !== '') {
    out.push(nameOf(p.requiresSpellKey));
  }
  out.push(...p.requiresOptionKeys.map((k) => optionKeyName(k, nameOf)));
  return out;
}

/** "Bruxo 5, Pacto da Lâmina, Rajada Mística e Olhar de Brasa", as a feat says it; empty when the option asks nothing. */
export function optionPrerequisiteSupport(
  p: OptionPrerequisite | undefined,
  nameOf: KeyName,
): string {
  return listWith(optionPrerequisiteParts(p, nameOf), 'e');
}

/**
 * "Pede Guerreiro 7 e Golpe do Corvo": what an option asks, led by the word the players' text and the editor's rows use; empty when
 * it asks nothing. The one place that words the sentence (the editor's summary line and "Como os jogadores veem" read it here).
 */
export function optionAsksPhrase(p: OptionPrerequisite | undefined, nameOf: KeyName): string {
  const asks = optionPrerequisiteSupport(p, nameOf);
  return asks ? `Pede ${asks}` : '';
}

/** The one line of a list row: the set, then what the option asks (nothing more when it asks nothing). */
export function optionSupport(
  set: string,
  p: OptionPrerequisite | undefined,
  nameOf: KeyName,
  offeredBy = '',
  offeredByName: OfferedByName = (key) => featureKeyName(key, nameOf),
): string[] {
  const asks = optionPrerequisiteSupport(p, nameOf);
  return [optionSetLine(set, offeredBy, offeredByName), ...(asks ? [`Exige ${asks}`] : [])];
}
