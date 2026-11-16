import {
  type Combatant,
  type GetMoveOptionsResponse,
  MoveRefusal,
  type ReachableSquare,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { tieNumbers } from '../format/text';
import { metersFixed } from '../units';
import { article } from './combat-log';
import type { Square } from './combat-grid';

/**
 * What the "Mover" page says about a square, read from what the server sent
 * (`GetMoveOptions`, RN-21). The browser does no rules math: the cost of a
 * square, why another is refused, who an exit provokes and which trap a square
 * is in all come from the server. What is here is lookup and words.
 */

/** A square's place in the options: where the player can go, or why not. */
export type SquareVerdict =
  | { readonly kind: 'here' }
  | { readonly kind: 'ok'; readonly square: ReachableSquare }
  | { readonly kind: 'refused'; readonly reason: MoveRefusal }
  /** Not in the options at all: beyond the circle. */
  | { readonly kind: 'beyond' }
  /** The options are not known (not answered yet, or the read failed): the server decides. */
  | { readonly kind: 'unknown' };

const key = (col: number, row: number): number => row * 1000 + col;

/** The options indexed by square, built once per answer. */
export interface MoveIndex {
  readonly reachable: ReadonlyMap<number, ReachableSquare>;
  readonly refused: ReadonlyMap<number, MoveRefusal>;
  /** There is an answer to read the squares from. */
  readonly known: boolean;
}

export function indexOptions(options: GetMoveOptionsResponse | null): MoveIndex {
  const reachable = new Map<number, ReachableSquare>();
  const refused = new Map<number, MoveRefusal>();
  for (const s of options?.reachable ?? []) {
    reachable.set(key(s.col, s.row), s);
  }
  for (const s of options?.refused ?? []) {
    refused.set(key(s.col, s.row), s.reason);
  }
  return { reachable, refused, known: options !== null };
}

/** The trap the character knows on a square the options reach, or `''`. */
export function knownTrapAt(index: MoveIndex, to: Square): string {
  return index.reachable.get(key(to.col, to.row))?.knownTrapName ?? '';
}

export function verdictFor(index: MoveIndex, origin: Square, to: Square): SquareVerdict {
  if (to.col === origin.col && to.row === origin.row) {
    return { kind: 'here' };
  }
  const ok = index.reachable.get(key(to.col, to.row));
  if (ok) {
    return { kind: 'ok', square: ok };
  }
  const reason = index.refused.get(key(to.col, to.row));
  if (reason !== undefined) {
    return { kind: 'refused', reason };
  }
  return index.known ? { kind: 'beyond' } : { kind: 'unknown' };
}

/** What a refused or unreachable square says: a title, the way out. It never
 * names what is in the way beyond the reason the server gave, and never a number
 * the server did not send ("faltam 0,9 m" is the answer to a confirmed move). */
export function refusalText(
  v: SquareVerdict,
  leftDft: number,
): { readonly title: string; readonly detail: string } | null {
  switch (v.kind) {
    case 'here':
      return { title: 'Você já está aqui', detail: 'Escolha um quadrado destacado.' };
    case 'refused':
      switch (v.reason) {
        case MoveRefusal.WALL:
          return {
            title: 'Sem caminho reto',
            detail:
              'Uma parede bloqueia esse caminho, no meio da linha ou no próprio quadrado. Escolha outro quadrado; para contornar uma parede no caminho, mova em partes.',
          };
        case MoveRefusal.ENEMY:
          return {
            title: 'Inimigo no caminho',
            detail: 'Não dá para passar por um inimigo. Escolha outro quadrado ou mova em partes.',
          };
        case MoveRefusal.OCCUPIED:
          return {
            title: 'Ocupado',
            detail: 'Há alguém nesse quadrado. Escolha um quadrado destacado.',
          };
        case MoveRefusal.NO_ROOM_TO_DRAG:
          // The mover drags a creature (W7-X) and no free square is left behind the path to leave it on.
          return {
            title: 'Sem casa para arrastar',
            detail:
              'Não há casa livre atrás de você para quem você segura. Escolha outro quadrado, ou solte o agarrão.',
          };
        default:
          return tooCostly(leftDft);
      }
    case 'beyond':
      return tooCostly(leftDft);
    default:
      return null;
  }
}

function tooCostly(leftDft: number): { title: string; detail: string } {
  return {
    title: 'Longe demais',
    detail: `Esse caminho custa mais do que os ${metersFixed(leftDft / 10)} que você tem. Escolha um quadrado destacado.`,
  };
}

/** "Mover 2,1 m": the title of a move that can be made. */
export function costTitle(costDft: number): string {
  return `Mover ${metersFixed(costDft / 10)}`;
}

/** "Depois restam 6,9 m." from what the server says is left (`left_after_dft` of a square, counted at the speed of the surface
 * it is on, or the counter of where the creature stands): the app never works it out across a switch of surface. */
export function afterLeftText(leftAfterDft: number): string {
  return `Depois restam ${metersFixed(leftAfterDft / 10)}.`;
}

/** One extra a square costs, with its word: "+1,5 m nadando". */
export interface ExtraPart {
  readonly kind: 'swim' | 'climb' | 'crawl';
  readonly text: string;
}

/** The extras of a square as the server counted them (`swimExtraDft`, `climbExtraDft`, `crawlExtraDft`), each with its word, in the order
 * the page says them: swimming, climbing, crawling. `note` is the sentence that follows when only a swim or only a climb is paid ("cada metro
 * na água conta dobrado"): two extras say themselves. Empty when the square pays none. The page never subtracts. */
export function extrasOf(square: ReachableSquare): { parts: readonly ExtraPart[]; note: string } {
  const parts: ExtraPart[] = [];
  const add = (kind: ExtraPart['kind'], dft: number, word: string) => {
    if (dft > 0) {
      parts.push({ kind, text: `+${metersFixed(dft / 10)} ${word}` });
    }
  };
  add('swim', square.swimExtraDft, 'nadando');
  add('climb', square.climbExtraDft, 'escalando');
  add('crawl', square.crawlExtraDft, 'rastejando');
  let note = '';
  if (parts.length === 1 && parts[0].kind === 'swim') {
    note = ': cada metro na água conta dobrado.';
  } else if (parts.length === 1 && parts[0].kind === 'climb') {
    note = ': cada metro escalado conta dobrado.';
  }
  return { parts, note };
}

/** "1,5 m do caminho + 1,5 m nadando + 1,5 m rastejando.": a crawl's cost in its parts, each as the server counted it (`pathDft` and the
 * extras). Empty when the square does not crawl: the cost line says the rest. */
export function extraBreakdown(square: ReachableSquare): string {
  if (square.crawlExtraDft <= 0) {
    return '';
  }
  const parts = [
    `${metersFixed(square.pathDft / 10)} do caminho`,
    ...(square.swimExtraDft > 0 ? [`${metersFixed(square.swimExtraDft / 10)} nadando`] : []),
    ...(square.climbExtraDft > 0 ? [`${metersFixed(square.climbExtraDft / 10)} escalando`] : []),
    `${metersFixed(square.crawlExtraDft / 10)} rastejando`,
  ];
  return `${parts.join(' + ')}.`;
}

/** Who a square's exit may provoke, by name: only combatants the caller sees
 * (the server never lists a hidden one). */
export function provokedBy(square: ReachableSquare, combatants: readonly Combatant[]): string[] {
  return square.provokesReactorIds.flatMap(
    (id) => combatants.find((c) => c.id === id)?.label ?? [],
  );
}

/** "do Goblin 2", "da Brisa", "do Goblin 1 e do Goblin 2". */
export function ofThe(labels: readonly string[]): string {
  const each = labels.map((l) => `${article(l) === 'a' ? 'da' : 'do'} ${l}`);
  return tieNumbers(
    each.length <= 1 ? each.join('') : `${each.slice(0, -1).join(', ')} e ${each[each.length - 1]}`,
  );
}

/** "Ele", "Ela" or "Eles", for the one or the ones whose reach the move leaves. */
function theyWord(labels: readonly string[]): { subject: string; verb: string } {
  if (labels.length === 1) {
    return { subject: article(labels[0]) === 'a' ? 'Ela' : 'Ele', verb: 'pode' };
  }
  return { subject: labels.every((l) => article(l) === 'a') ? 'Elas' : 'Eles', verb: 'podem' };
}

/** The warning before a move that may provoke: a warning, since for a player the server says only "pode". A walk reads "Sair do
 * alcance do Goblin 2 pode provocar um ataque de oportunidade."; a long jump reads the board's "Esse salto sai do alcance do
 * Goblin 2. Ele pode atacar você de graça (ataque de oportunidade)." */
export function provokeWarning(
  labels: readonly string[],
  way: 'caminho' | 'salto' = 'caminho',
): string {
  if (way === 'caminho') {
    return `Sair do alcance ${ofThe(labels)} pode provocar um ataque de oportunidade.`;
  }
  const they = theyWord(labels);
  return `Esse salto sai do alcance ${ofThe(labels)}. ${they.subject} ${they.verb} atacar você de graça (ataque de oportunidade).`;
}

/** The note on the jump's card: where it lands and what is left, with the trap the character knows when the landing square is
 * inside one ("Cai do outro lado do Fosso. Depois restam 1,5 m. O Fosso só dispara onde você cai."), and no word about a trap
 * the character does not know (RN-10). */
export function jumpCardText(
  costDft: number,
  leftAfterDft: number | null,
  knownTrapName = '',
): string {
  const after = leftAfterDft === null ? '' : afterLeftText(leftAfterDft); // the server's number for the landing square
  if (knownTrapName) {
    const the = `${article(knownTrapName) === 'a' ? 'a' : 'o'} ${knownTrapName}`;
    const The = `${article(knownTrapName) === 'a' ? 'A' : 'O'} ${knownTrapName}`;
    return `Cai do outro lado d${the}. ${after ? `${after} ` : ''}${The} só dispara onde você cai.`;
  }
  return `Cai ${metersFixed(costDft / 10)} adiante.${after ? ` ${after}` : ''}`;
}

/** The label under the chosen square of a jump that leaves an enemy's reach. */
export const LANDS_OUT_OF_REACH = 'Cai fora do alcance';

/** The question before a move into a trap the character knows. */
export function trapQuestion(name: string): string {
  return `Isso entra no ${name}. Mover assim mesmo?`;
}

/** The line under the title: "Restam 6,0 m de 9,0 m (4 quadrados de 1,5 m). Você já andou 3,0 m."
 * `squares` is the count of whole squares left, or `null` to leave it out. */
export function leftLine(
  leftDft: number,
  totalDft: number,
  usedDft: number,
  squares: number | null = null,
  /** The mount whose counter this is, when a rider moves it ("ao Cavalo de guerra"). */
  mount = '',
): string {
  const count =
    squares === null
      ? ''
      : ` (${squares} ${squares === 1 ? 'quadrado' : 'quadrados'} de 1,5\u00a0m)`;
  const walked = usedDft > 0 ? ` Você já andou ${metersFixed(usedDft / 10)}.` : '';
  const owner = mount ? ` ${article(mount) === 'a' ? 'à' : 'ao'} ${mount}` : '';
  return `Restam ${metersFixed(leftDft / 10)} de ${metersFixed(totalDft / 10)}${owner}${count}.${walked}`;
}
