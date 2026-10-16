import {
  type Combatant,
  type Encounter,
  type GetTurnOptionsResponse,
  CombatantState,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { type Who, named, possessive, pronoun, whoIs } from './contest-view';
import { metersText } from '../units';
import { tight, upFirst } from '../format/text';

/**
 * What the Help sheet lists (W7-X, board W7-Xc 9): the allies the player may help, the tasks a check help names and the
 * creatures an ally's first attack may aim at. The server judges every choice (`Help` refuses an ally that is not one, a task that
 * is not a check, a target farther than 1,5 m); here only the rows and their words are made.
 */

/** A help on an attack reaches a creature this close to the helper (5 ft, 1,5 m). */
export const HELP_REACH_FT = 5;

/** The checks a help can name, by the key the server takes and the official Portuguese name (`names_pt.json`). */
export const HELP_TASKS: readonly { readonly key: string; readonly name: string }[] = [
  { key: 'skill:acrobatics', name: 'Acrobacia' },
  { key: 'skill:animal-handling', name: 'Adestrar Animais' },
  { key: 'skill:arcana', name: 'Arcanismo' },
  { key: 'skill:athletics', name: 'Atletismo' },
  { key: 'skill:performance', name: 'Atuação' },
  { key: 'skill:deception', name: 'Enganação' },
  { key: 'skill:stealth', name: 'Furtividade' },
  { key: 'skill:history', name: 'História' },
  { key: 'skill:intimidation', name: 'Intimidação' },
  { key: 'skill:insight', name: 'Intuição' },
  { key: 'skill:investigation', name: 'Investigação' },
  { key: 'skill:medicine', name: 'Medicina' },
  { key: 'skill:nature', name: 'Natureza' },
  { key: 'skill:perception', name: 'Percepção' },
  { key: 'skill:persuasion', name: 'Persuasão' },
  { key: 'skill:sleight-of-hand', name: 'Prestidigitação' },
  { key: 'skill:religion', name: 'Religião' },
  { key: 'skill:survival', name: 'Sobrevivência' },
];

/** An ally the player may help. */
export interface HelpAlly {
  readonly id: string;
  readonly label: string;
  readonly who: Who;
}

/** The allies of the helper: the same side, standing in the combat, never the helper itself. */
export function helpAllies(e: Encounter, helperId: string): readonly HelpAlly[] {
  const helper = e.combatants.find((c) => c.id === helperId);
  if (!helper) {
    return [];
  }
  return e.combatants
    .filter(
      (c) =>
        c.id !== helperId &&
        c.side === helper.side &&
        !c.defeated &&
        c.state !== CombatantState.DEAD,
    )
    .map((c) => ({ id: c.id, label: c.label, who: whoIs(c) }));
}

/** A creature the ally's first attack may aim at, with how far it is from the helper. */
export interface HelpTarget {
  readonly id: string;
  readonly label: string;
  readonly who: Who;
  /** In feet; `undefined` where the combat has no map (the master judges the reach). */
  readonly distanceFt: number | undefined;
  readonly reachable: boolean;
}

/** The creatures the helper sees on the other side, with the distance the server worked out for the helper's own attacks and spells. */
export function helpTargets(
  options: GetTurnOptionsResponse | null,
  e: Encounter,
  helperId: string,
): readonly HelpTarget[] {
  const helper = e.combatants.find((c) => c.id === helperId);
  if (!helper || !options) {
    return [];
  }
  const distances = new Map<string, number | undefined>();
  for (const t of options.attackTargets.flatMap((a) => a.targets)) {
    distances.set(t.combatantId, distances.get(t.combatantId) ?? t.distanceFt);
  }
  for (const t of options.spellTargets.flatMap((s) => s.targets)) {
    distances.set(t.combatantId, distances.get(t.combatantId) ?? t.distanceFt);
  }
  if (distances.size === 0) {
    // A helper with no attack and no spell (a familiar: it can't attack, but it can help, SRD 5.1 Find Familiar) gets no
    // targets from its options: the creatures of the other side, at the server's range on the grid (no map, no distance).
    for (const c of e.combatants) {
      if (c.side !== helper.side) {
        distances.set(c.id, rangeFt(helper, c));
      }
    }
  }
  return e.combatants
    .filter((c) => c.side !== helper.side && !c.defeated && distances.has(c.id))
    .map((c) => {
      const distanceFt = distances.get(c.id);
      return {
        id: c.id,
        label: c.label,
        who: whoIs(c),
        distanceFt,
        reachable: distanceFt === undefined || distanceFt <= HELP_REACH_FT,
      };
    })
    .sort((a, b) => Number(b.reachable) - Number(a.reachable));
}

/** A square of the grid is 5 ft (1,5 m) across. */
const FEET_PER_SQUARE = 5;

/** The distance the server measures for a reach (`grid.RangeFt`): the straight line between the squares, in whole squares
 * rounded down, 1,5 m each, so a diagonal neighbour is 1,5 m away. `undefined` when either is off the map. */
export function rangeFt(a: Combatant, b: Combatant): number | undefined {
  if (!a.placed || !b.placed) {
    return undefined;
  }
  const squares = Math.floor(Math.sqrt((a.col - b.col) ** 2 + (a.row - b.row) ** 2));
  return squares * FEET_PER_SQUARE;
}

/** "Tavo · Percepção". */
export function taskRow(ally: HelpAlly, task: string): { name: string; sub: string } {
  return {
    name: `${ally.label} · ${task}`,
    sub: `Vantagem no próximo teste de ${task} ${possessive(ally.label)}.`,
  };
}

/** "Toren ataca o Hobgoblin" with "O Hobgoblin está a 1,5 m de você." or "A 4,5 m de você." and the reason it is refused. */
export function attackRow(
  ally: HelpAlly,
  target: HelpTarget,
): { name: string; sub: string; blocked: string } {
  const the = named(target.who);
  const far = target.distanceFt === undefined ? '' : `a ${metersText(target.distanceFt)} de você`;
  return {
    name: `${ally.label} ataca ${the}`,
    sub: tight(far ? (target.reachable ? `${upFirst(the)} está ${far}.` : `${upFirst(far)}.`) : ''),
    blocked: target.reachable ? '' : tight(`Longe demais: no máximo ${metersText(HELP_REACH_FT)}.`),
  };
}

/** The line over the attack rows: "O alvo precisa estar a até 1,5 m de você. O primeiro ataque de Toren contra ele terá vantagem." */
export function attackIntro(ally: HelpAlly): string {
  return tight(
    `O alvo precisa estar a até ${metersText(HELP_REACH_FT)} de você. O primeiro ataque de ${ally.label} contra ele terá vantagem.`,
  );
}

/** What the player reads once the help is made. */
export function helpedText(
  ally: HelpAlly,
  task: string | null,
  target: HelpTarget | null,
): { lead: string; rest: string } {
  const who = pronoun(ally.label);
  if (task) {
    return {
      lead: `Você ajudou ${ally.label}.`,
      rest: `${upFirst(who)} terá vantagem no próximo teste de ${task}.`,
    };
  }
  return {
    lead: `Você ajudou ${ally.label}.`,
    rest: target
      ? `O primeiro ataque ${possessive(ally.label)} contra ${named(target.who)} terá vantagem.`
      : '',
  };
}
