import type { SearchSkill } from './traps-client';

/** The character's bonuses for the two skills a search rolls (from the derived sheet), or `null` while unknown. */
export interface SearchSkills {
  readonly perception: number | null;
  readonly investigation: number | null;
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

/** The two radios of "Como você procura": the skill with the character's bonus, and what it does. */
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
  ];
}

export function skillName(skill: SearchSkill): string {
  return skill === 'perception' ? 'Percepção' : 'Investigação';
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
