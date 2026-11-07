import {
  Ability,
  type AbilityScores,
  type FeatPrerequisite,
  type FeatUnmet,
  FeatUnmetKind,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import { ABILITY_LABELS } from '../characters/character-labels';
import type { AbilityKey } from '../characters/characters.types';

/**
 * What a feat asks of a character and what a character lacks for it, in Portuguese (MR-025). The server decides who
 * qualifies and lists what is unmet; this only writes the prerequisite and the codes as sentences. A key the app has
 * no name for (a proficiency, a race) is named by `nameOf`, from the content catalog.
 */

export type KeyName = (key: string) => string;

const FIELDS: readonly { field: keyof AbilityScores & string; key: AbilityKey }[] = [
  { field: 'strength', key: 'str' },
  { field: 'dexterity', key: 'dex' },
  { field: 'constitution', key: 'con' },
  { field: 'intelligence', key: 'int' },
  { field: 'wisdom', key: 'wis' },
  { field: 'charisma', key: 'cha' },
];

const WIRE_KEY: Readonly<Record<number, AbilityKey>> = {
  [Ability.STRENGTH]: 'str',
  [Ability.DEXTERITY]: 'dex',
  [Ability.CONSTITUTION]: 'con',
  [Ability.INTELLIGENCE]: 'int',
  [Ability.WISDOM]: 'wis',
  [Ability.CHARISMA]: 'cha',
};

/** "Força", for an ability of the wire. */
export function abilityName(ability: Ability): string {
  const key = WIRE_KEY[ability];
  return key ? ABILITY_LABELS[key] : '';
}

/** "Força 13" for each ability with a score in `scores`, in the sheet's order. */
function scoreList(scores: AbilityScores | undefined): string[] {
  return FIELDS.flatMap(({ field, key }) => {
    const value = scores?.[field] as number | undefined;
    return value ? [`${ABILITY_LABELS[key]} ${value}`] : [];
  });
}

function listWith(items: readonly string[], word: 'e' | 'ou'): string {
  return items.length <= 1
    ? (items[0] ?? '')
    : `${items.slice(0, -1).join(', ')} ${word} ${items[items.length - 1]}`;
}

/** The sizes the server writes in English, in Portuguese ("Médio"). */
export const SIZE_WORDS: Readonly<Record<string, string>> = {
  Tiny: 'Miúdo',
  Small: 'Pequeno',
  Medium: 'Médio',
  Large: 'Grande',
};

/** "médio", for a size the server writes in English ("Medium"); an unknown size is shown as it came. */
function sizeName(size: string): string {
  return SIZE_WORDS[size]?.toLowerCase() ?? size;
}

/** The races (and sub-races) a feat asks for, as the draft has them: the single `raceKey` first, then the list. */
export function askedRaces(raceKey: string, races: readonly string[]): string[] {
  return [...(raceKey ? [raceKey] : []), ...races.filter((r) => r !== raceKey)];
}

/** What the server says for a condition the player may not read; the same words in the "Pede:" line and in the unmet line. */
const HIDDEN_ASK = 'uma condição que a mesa não mostra';
const HIDDEN_UNMET = `Pede ${HIDDEN_ASK}.`;

/** The feature a feat asks not to have, as the player's view gives it: its name, or that it is hidden (then no name). */
export interface LacksView {
  readonly name: string;
  readonly hidden: boolean;
}

/** What the feat asks of who the character is: the races, the size and the feature to lack. */
function whoParts(p: FeatPrerequisite, nameOf: KeyName, lacks?: LacksView): string[] {
  const parts: string[] = [];
  const races = askedRaces(p.raceKey, p.races);
  if (races.length > 0) {
    parts.push(`ser ${listWith(races.map(nameOf), 'ou')}`);
  } else if (p.racesHidden) {
    parts.push(HIDDEN_ASK);
  }
  if (p.size) {
    parts.push(`ter tamanho ${sizeName(p.size)}`);
  }
  if ((lacks?.hidden || p.lacksHidden) && !parts.includes(HIDDEN_ASK)) {
    parts.push(HIDDEN_ASK);
  } else if (p.lacksFeatureKey && !lacks?.hidden && !p.lacksHidden) {
    parts.push(`não ter ${lacks?.name || p.lacksFeatureNamePt || nameOf(p.lacksFeatureKey)}`);
  }
  return parts;
}

/**
 * The conditions of a prerequisite, one phrase each, such as "Força 13 e Destreza 13".
 * Nothing comes back when the prerequisite asks nothing.
 * A player's `lacks` names the feature to lack, or says it is hidden.
 * Without `lacks`, `nameOf` names the key.
 */
export function prerequisiteParts(
  p: FeatPrerequisite | undefined,
  nameOf: KeyName,
  lacks?: LacksView,
): string[] {
  if (!p) {
    return [];
  }
  const parts: string[] = [];
  const all = scoreList(p.minimums);
  if (all.length > 0) {
    parts.push(listWith(all, 'e'));
  }
  const any = scoreList(p.anyOf);
  if (any.length > 0) {
    parts.push(any.length === 1 ? any[0] : `${listWith(any, 'ou')} (uma delas)`);
  }
  if (p.proficiencyKey) {
    parts.push(`proficiência em ${nameOf(p.proficiencyKey)}`);
  }
  if (p.spellcasting) {
    parts.push('poder conjurar ao menos uma magia');
  }
  parts.push(...whoParts(p, nameOf, lacks));
  if (p.level > 0) {
    parts.push(`nível ${p.level} ou mais`);
  }
  return parts;
}

/** "Força 13, ser Gnomo ou Halfling e nível 4 ou mais"; empty when the feat asks nothing. */
export function prerequisiteSupport(
  p: FeatPrerequisite | undefined,
  nameOf: KeyName,
  lacks?: LacksView,
): string {
  return listWith(prerequisiteParts(p, nameOf, lacks), 'e');
}

/** What a character lacks for a feat: "Precisa de Força 13.", in the words of the server's cards, from its `unmet`. */
export function unmetText(u: FeatUnmet, nameOf: KeyName): string {
  const needs = u.abilities.map((a) => `${abilityName(a.ability)} ${a.minimum}`);
  switch (u.kind) {
    case FeatUnmetKind.ABILITY_MINIMUM:
      return `Precisa de ${listWith(needs, 'e')}.`;
    case FeatUnmetKind.ABILITY_ANY_OF:
      return `Precisa de ${listWith(needs, 'ou')}.`;
    case FeatUnmetKind.PROFICIENCY:
      return `Precisa de proficiência em ${nameOf(u.key)}.`;
    case FeatUnmetKind.SPELLCASTING:
      return 'Precisa conseguir conjurar magias.';
    case FeatUnmetKind.RACE: {
      const races = (u.key ? [u.key] : u.keys).map(nameOf);
      return races.length > 0 ? `Só para ${listWith(races, 'ou')}.` : hiddenOr(u);
    }
    case FeatUnmetKind.SIZE:
      return `Só para tamanho ${sizeName(u.key)}.`;
    case FeatUnmetKind.LACKS_FEATURE:
      return u.key ? `Não pode ter ${u.namePt || nameOf(u.key)}.` : hiddenOr(u);
    case FeatUnmetKind.LEVEL:
      return `Precisa de nível ${u.value} ou mais.`;
    default:
      return hiddenOr(u);
  }
}

/** A condition with nothing to name says it is hidden when the server marked it so; never a key. */
function hiddenOr(u: FeatUnmet): string {
  return u.hidden ? HIDDEN_UNMET : 'Não atende ao pré-requisito.';
}
