import type { DiceRoll } from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  checkFloorOf,
  type CheckFloor,
  treatedFormula,
  treatedPreview,
  typedTotal,
} from '../../../../core/combat/combat-dice';
import { MapPointKind, type MapPoint } from '../../../../../gen/meurpg/maps/v1/maps_pb';
import type { SearchForTrapsResponse } from '../../../../../gen/meurpg/play/v1/traps_pb';
import { showOfDice } from '../../../../shared/roll-overlay/roll-animator';
import { type OtherSkill, otherTitle } from '../../../../core/traps/trap-search';
import type { SearchSkill } from '../../../../core/traps/traps-client';

/** The skill the sheet is searching with: one of the two listed, or the "Outra perícia…" row with the skill picked (if any). */
export function chosenSkill<T extends { readonly skill: SearchSkill }>(
  skill: SearchSkill,
  options: readonly T[],
  picked: OtherSkill | null,
): T | { skill: 'other'; title: string; detail: string; bonus: number } {
  if (skill === 'other') {
    return {
      skill: 'other' as const,
      title: picked ? otherTitle(picked) : 'Outra perícia…',
      detail: '',
      bonus: picked?.bonus ?? 0,
    };
  }
  return options.find((o) => o.skill === skill) ?? options[0];
}

/** "1d20 (13) + 4 (Investigação) = 17" for the server's roll; with Talento Confiável "d20: 6 → 10 (Talento Confiável) + 9 (Acrobacia) = 19". */
export function rollFormula(roll: DiceRoll, skill: string): string {
  const treated = treatedFormula(roll, skill);
  if (treated) {
    return treated;
  }
  const mod = roll.modifier < 0 ? `− ${Math.abs(roll.modifier)}` : `+ ${roll.modifier}`;
  return `1d20 (${roll.faces.join(', ')}) ${mod} (${skill}) = ${roll.total}`;
}

/** The typed d20's live total. A floor (Talento Confiável or a table's minimum): the server owns the rule; the sheet knows the feature and the skill, so it shows the account (6 → 10). */
export function typedLineOf(
  face: number,
  bonus: number,
  skillKey: string | null,
  reliableTalent: readonly string[] | undefined,
  tableFloor?: CheckFloor | null,
): string {
  const floor = checkFloorOf(skillKey, reliableTalent, tableFloor);
  const raised = floor ? treatedPreview(face, bonus, floor) : null;
  return raised?.text ?? typedTotal(face, bonus);
}

/** "Role 1d20 para Arcanismo (+8)": the bonus is the end of the chosen skill's title, when the sheet is known (`title` is `null` otherwise). */
export function rollLabel(skillWord: string, title: string | null): string {
  const bonus = title === null ? '' : ` (${title.replace(/^.*\s(?=[+−])/, '')})`;
  return `Role 1d20 para ${skillWord}${bonus}`;
}

/** What a key does on the "Outra perícia…" list: the new cursor, `'pick'` for Enter or Space, `null` for a key it does not use. */
export function listKeyMove(key: string, cursor: number, last: number): number | 'pick' | null {
  switch (key) {
    case 'ArrowDown':
      return Math.min(last, cursor + 1);
    case 'ArrowUp':
      return Math.max(0, cursor - 1);
    case 'Home':
      return 0;
    case 'End':
      return Math.max(0, last);
    case 'Enter':
    case ' ':
      return 'pick';
    default:
      return null;
  }
}

/** Under the typed die, when the search asks for a second d20. */
export const SECOND_DIE_HINT =
  'Esta procura leva dois d20: há penumbra por perto, ou o seu personagem tem vantagem ou desvantagem. Role de novo e digite o segundo dado (1 a 20); o app diz qual vale.';

/** Under the typed die, for the first (and usually only) one. */
export const TYPED_DIE_HINT = 'Role o seu dado e digite o número que saiu (1 a 20).';

/** The totals of the search's dice, "17 · 12" when there are two. */
export function totalOf(res: SearchForTrapsResponse | undefined): string {
  return [res?.roll, res?.secondRoll]
    .filter((x) => x !== undefined)
    .map((x) => x.total)
    .join(' · ');
}

/** The names of the traps the search found, as the map gives them. */
export function foundNames(points: readonly MapPoint[], foundIds: readonly string[]): string[] {
  return points
    .filter((p) => p.kind === MapPointKind.TRAP && foundIds.includes(p.id))
    .map((p) => p.name);
}

/** The d20 of the search, with "Achou" or "Nada": exactly what the result card tells (the DC never comes). */
export function searchShow(res: SearchForTrapsResponse) {
  const found = res.foundPointIds.length > 0;
  return showOfDice('Procurar armadilhas', res.roll, {
    withTotal: true,
    outcome: { word: found ? 'Achou' : 'Nada', good: found },
  });
}

/** The search as the result card tells it. */
export interface SearchResultView {
  readonly res: SearchForTrapsResponse;
  readonly names: readonly string[];
  readonly skill: SearchSkill;
  /** The skill's own name, for "(Arcanismo)" after the bonus. */
  readonly skillLabel: string;
}

/** What the server rolled, written as it counts: "1d20 (13) + 4 (Investigação) = 17"; two lines when there are two dice. */
export function rollsOf(r: SearchResultView | null): string[] {
  return r
    ? [r.res.roll, r.res.secondRoll]
        .filter((x) => x !== undefined)
        .map((x) => rollFormula(x, r.skillLabel))
    : [];
}
