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

/** "Depois restam 6,9 m." */
export function afterText(leftDft: number, costDft: number): string {
  // No clamp: the callers only say it for a cost the movement left pays.
  return `Depois restam ${metersFixed((leftDft - costDft) / 10)}.`;
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

/** The warning before a move that may provoke: a warning, since the server says "pode". */
export function provokeWarning(labels: readonly string[]): string {
  return `Sair do alcance ${ofThe(labels)} pode provocar um ataque de oportunidade.`;
}

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
): string {
  const count =
    squares === null
      ? ''
      : ` (${squares} ${squares === 1 ? 'quadrado' : 'quadrados'} de 1,5\u00a0m)`;
  const walked = usedDft > 0 ? ` Você já andou ${metersFixed(usedDft / 10)}.` : '';
  return `Restam ${metersFixed(leftDft / 10)} de ${metersFixed(totalDft / 10)}${count}.${walked}`;
}
