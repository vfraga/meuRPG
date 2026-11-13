import type { CheckFloor } from '../combat/combat-dice';
import type { SearchSkill } from './traps-client';

/** One of the other skills of the sheet, with the character's bonus: a row of "Perícia". */
export interface OtherSkill {
  /** "skill:arcana". */
  readonly key: string;
  readonly name: string;
  readonly bonus: number;
}

/** The character's bonuses for the two skills a search rolls (from the derived sheet), or `null` while unknown. */
export interface SearchSkills {
  readonly perception: number | null;
  readonly investigation: number | null;
  /** "Outra perícia…": the other 16 skills with the sheet's bonus, alphabetical (the same list for every trap, RN-10). */
  readonly others?: readonly OtherSkill[];
  /** The keys of the skills Talento Confiável raises for this character (the proficient ones); empty without the feature. */
  readonly reliableTalent?: readonly string[];
  /** The highest table minimum that covers checks (`DerivedSheet.roll_minimums`), or `null` without one. */
  readonly checkFloor?: CheckFloor | null;
}

export interface SkillOption {
  readonly skill: SearchSkill;
  /** "Percepção +4". */
  readonly title: string;
  readonly detail: string;
  readonly bonus: number;
}

function signed(n: number): string {
  return `${n < 0 ? '−' : '+'}${Math.abs(n)}`;
}

/** The line under the menu, the same everywhere and for everyone: it says nothing about the traps that are near. */
export const SPECIFIC_SKILL_NOTE = 'Algumas armadilhas só se acham com uma perícia específica.';

/** The three radios of "Como você procura": Percepção and Investigação with the character's bonus, and "Outra perícia…". The
 * menu never changes with what is near (RN-10): a menu that varied would tell the player there is a hidden trap. */
export function skillOptions(skills: SearchSkills | null): readonly SkillOption[] {
  const p = skills?.perception ?? 0;
  const i = skills?.investigation ?? 0;
  return [
    {
      skill: 'perception',
      title: skills?.perception == null ? 'Percepção' : `Percepção ${signed(p)}`,
      detail: 'Reparar em armadilhas à vista',
      bonus: p,
    },
    {
      skill: 'investigation',
      title: skills?.investigation == null ? 'Investigação' : `Investigação ${signed(i)}`,
      detail: 'Examinar o lugar com calma',
      bonus: i,
    },
    {
      skill: 'other',
      title: 'Outra perícia…',
      detail: 'Qualquer outra perícia da sua ficha',
      bonus: 0,
    },
  ];
}

/** "Arcanismo +8", the row of the list and the value of the field. */
export function otherTitle(skill: OtherSkill): string {
  return `${skill.name} ${signed(skill.bonus)}`;
}

/** The name of the skill a search rolled: the two that always find, or the one picked under "Outra perícia…". */
export function skillName(skill: SearchSkill, other = ''): string {
  if (skill === 'other') {
    return other || 'Outra perícia';
  }
  return skill === 'perception' ? 'Percepção' : 'Investigação';
}

/** The SRD's 18 skills in Portuguese, for a line that has only the key ("procurou armadilhas (Arcanismo)"): the same words as the
 * rules' content, which a screen with a whole list reads from the server instead. */
const SKILL_NAMES: Readonly<Record<string, string>> = {
  'skill:acrobatics': 'Acrobacia',
  'skill:animal-handling': 'Adestrar Animais',
  'skill:arcana': 'Arcanismo',
  'skill:athletics': 'Atletismo',
  'skill:deception': 'Enganação',
  'skill:history': 'História',
  'skill:insight': 'Intuição',
  'skill:intimidation': 'Intimidação',
  'skill:investigation': 'Investigação',
  'skill:medicine': 'Medicina',
  'skill:nature': 'Natureza',
  'skill:perception': 'Percepção',
  'skill:performance': 'Atuação',
  'skill:persuasion': 'Persuasão',
  'skill:religion': 'Religião',
  'skill:sleight-of-hand': 'Prestidigitação',
  'skill:stealth': 'Furtividade',
  'skill:survival': 'Sobrevivência',
};

/** A skill's Portuguese name from its key; the key itself for one this app does not know. */
export function skillKeyName(key: string): string {
  return SKILL_NAMES[key] ?? key;
}

/** The three steps of the sheet, 1 to 3: "Como", "Rolar", "Resultado". */
export const SEARCH_STEPS = ['Como', 'Rolar', 'Resultado'] as const;

/** Which step the sheet is on: the result once there is one, typing a die is "Rolar", the skill's choice "Como". */
export function searchStep(hasResult: boolean, typing: boolean): 1 | 2 | 3 {
  return hasResult ? 3 : typing ? 2 : 1;
}

/** The answer in words: the same for "nothing there" and "the roll fell short" (the server's rule: the answer never says which).
 * `count` is how many traps the server said were found; `found` their names, read from the map afterwards. When the map
 * could not be read (fewer names than traps), the answer still says how many were found and sends the player to the map. */
export function resultMessage(
  found: readonly string[],
  count: number = found.length,
): {
  readonly title: string;
  readonly detail: string;
} {
  if (count === 0) {
    return { title: 'Você não encontrou nada.', detail: '' };
  }
  if (found.length < count) {
    return count === 1
      ? { title: 'Você achou uma armadilha.', detail: 'Veja no seu mapa.' }
      : { title: `Você achou ${count} armadilhas.`, detail: 'Veja no seu mapa.' };
  }
  return found.length === 1
    ? { title: `Você achou uma armadilha: ${found[0]}.`, detail: 'Ela já aparece no seu mapa.' }
    : {
        title: `Você achou ${found.length} armadilhas: ${found.join(', ')}.`,
        detail: 'Elas já aparecem no seu mapa.',
      };
}

/** Where the Search action of a combat goes: the trap search needs the combat's map to have a grid and the player's combatant to
 * stand on it (the server answers TRAP_NOT_ON_MAP otherwise); without that it is the SRD's plain Search action. */
export function searchRoute(gridColumns: number, placed: boolean): 'traps' | 'action' {
  return gridColumns > 0 && placed ? 'traps' : 'action';
}
