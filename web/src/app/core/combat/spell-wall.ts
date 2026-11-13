import { WallSide } from '../../../gen/meurpg/play/v1/combat_pb';
import { SQUARE_FT, metersText, reachSquares } from '../units';
import type { Square } from './combat-grid';
import { type Direction, rangeFt } from './spell-area';
import type { ZoneView } from './zones';

/**
 * The spell that lays a wall (Wall of Fire, SRD 5.1) on the map: a straight wall or a ring, placed by a point, a direction,
 * a length and the side that burns (RN-37). The turn options mark it as a point spell (`AreaPlacement.POINT`) like Moonbeam
 * and carry nothing else that tells a wall apart, so the web names it by its catalog key, here and nowhere else.
 * Follow-up: a flag from the server on the spell's targets (`lays_wall`) would replace this constant.
 */
export const WALL_SPELL_KEY = 'spell:wall-of-fire';

/** Whether the spell lays a wall: its cast sheet asks "Onde fica" (shape, length, side) and sends `SpellWall`. */
export function laysWall(spellKey: string): boolean {
  return spellKey === WALL_SPELL_KEY;
}

/** The wall's two steps, as the board names them: where it lies, then who is in it. The footer's button names the second. */
export const WALL_STEPS = ['Onde fica', 'Quem está nela'] as const;
export const WALL_NEXT = 'Próximo: quem está nela';

/** The largest a straight wall is, and a ring's diameter, in feet: what the server takes (`wallLength`). */
export const WALL_LINE_FT = 60;
export const WALL_RING_FT = 20;
/** A ring is offered at two sizes only: the server rounds the radius up to 5 ft, so 5 and 10 ft are one ring (8 squares) and
 * 15 and 20 ft are another (16 squares). */
const RING_MIN_FT = 10;

/** The wall as the caster is laying it. */
export interface WallChoice {
  readonly ring: boolean;
  /** The way a line runs from its first square; `null` for a ring. */
  readonly direction: Direction | null;
  /** A line's length or a ring's diameter, in feet: always explicit, a multiple of 5. */
  readonly lengthFt: number;
  readonly side: WallSide;
}

const EAST: Direction = { dx: 1, dy: 0 };

export const maxLengthFt = (ring: boolean) => (ring ? WALL_RING_FT : WALL_LINE_FT);
export const minLengthFt = (ring: boolean) => (ring ? RING_MIN_FT : SQUARE_FT);

/** A wall at its full size: a line to the east, or a ring. No side burns until the caster picks one. */
export function newWall(ring: boolean): WallChoice {
  return {
    ring,
    direction: ring ? null : EAST,
    lengthFt: maxLengthFt(ring),
    side: WallSide.UNSPECIFIED,
  };
}

/** The other shape: its own side and direction, the length kept as far as the new size allows. */
export function switchShape(wall: WallChoice, ring: boolean): WallChoice {
  if (wall.ring === ring) {
    return wall;
  }
  const fresh = newWall(ring);
  const lengthFt = ring
    ? wall.lengthFt > RING_MIN_FT + SQUARE_FT
      ? WALL_RING_FT
      : RING_MIN_FT
    : Math.max(minLengthFt(ring), wall.lengthFt);
  return { ...fresh, lengthFt };
}

/** How much one step of the stepper changes the length, in feet: a square for a line, 10 ft (the next ring) for a ring. */
export const stepFt = (ring: boolean) => (ring ? RING_MIN_FT : SQUARE_FT);

/** The length a step more or less (a square, 1,5 m, for a line; the other ring size for a ring), kept inside the shape's sizes. */
export function stepLength(wall: WallChoice, steps: number): WallChoice {
  const next = wall.lengthFt + steps * stepFt(wall.ring);
  return {
    ...wall,
    lengthFt: Math.min(maxLengthFt(wall.ring), Math.max(minLengthFt(wall.ring), next)),
  };
}

/** "12 m": the length as the table says it. */
export function lengthText(wall: WallChoice): string {
  return metersText(wall.lengthFt);
}

/** "De 1,5 m a 18 m: 8 quadrados." for a line, "3 m ou 6 m de diâmetro." for a ring. */
export function lengthNote(wall: WallChoice): string {
  if (wall.ring) {
    return `${metersText(minLengthFt(true))} ou ${metersText(maxLengthFt(true))} de diâmetro.`;
  }
  const range = `De ${metersText(minLengthFt(false))} a ${metersText(maxLengthFt(false))}`;
  const n = reachSquares(wall.lengthFt);
  return `${range}: ${n} ${n === 1 ? 'quadrado' : 'quadrados'}.`;
}

export interface SideChoice {
  readonly side: WallSide;
  readonly label: string;
  readonly icon: string;
}

/** Where a side lies on the screen, by the vector (dx, dy) from the wall (y grows downward): word and arrow. */
const LIES: Readonly<Record<string, readonly [string, string]>> = {
  '0,-1': ['Acima', 'arrow_upward'],
  '0,1': ['Abaixo', 'arrow_downward'],
  '-1,0': ['À esquerda', 'arrow_back'],
  '1,0': ['À direita', 'arrow_forward'],
  '1,-1': ['Acima, à direita', 'north_east'],
  '-1,1': ['Abaixo, à esquerda', 'south_west'],
  '-1,-1': ['Acima, à esquerda', 'north_west'],
  '1,1': ['Abaixo, à direita', 'south_east'],
};

/**
 * The sides the shape offers. A line's are named by where they lie on the screen: for a wall running (dx, dy) the LEFT side lies
 * toward (dy, -dx) and the RIGHT side toward (-dy, dx), so an east wall has "Acima" (left) and "Abaixo" (right). A ring's are
 * "Dentro" and "Fora".
 */
export function sideChoices(wall: WallChoice): readonly SideChoice[] {
  if (wall.ring) {
    return [
      { side: WallSide.INNER, label: 'Dentro', icon: 'adjust' },
      { side: WallSide.OUTER, label: 'Fora', icon: 'fullscreen' },
    ];
  }
  const { dx, dy } = wall.direction ?? EAST;
  const of = (side: WallSide, vx: number, vy: number): SideChoice => {
    const [label, icon] = LIES[`${vx},${vy}`];
    return { side, label, icon };
  };
  return [of(WallSide.LEFT, dy, -dx), of(WallSide.RIGHT, -dy, dx)];
}

/** The wall turned to another way: the sides swap their sense, so the pick is taken back and asked again. */
export function reorient(wall: WallChoice, direction: Direction): WallChoice {
  const now = wall.direction;
  if (now && now.dx === direction.dx && now.dy === direction.dy) {
    return wall;
  }
  return { ...wall, direction, side: WallSide.UNSPECIFIED };
}

/** The caster chose the side that burns. */
export const sidePicked = (wall: WallChoice): boolean => wall.side !== WallSide.UNSPECIFIED;

/** The wall a preview asks about: the server refuses a wall without a side, so one not picked yet is asked with a provisional
 * side (its squares do not depend on it) and the answer's burning squares are not drawn. */
export function askable(wall: WallChoice): WallChoice {
  return sidePicked(wall) ? wall : { ...wall, side: wall.ring ? WallSide.INNER : WallSide.LEFT };
}

/**
 * "A muralha para na parede: ficou com 9 m (6 quadrados) em vez de 12 m.": the squares a straight wall of this length has on open
 * floor (the map's distance, as the server counts: a square is in while its distance from the start plus the square is at most
 * the length, so 60 ft are 12 squares along a row and 9 along a diagonal) against the squares the server laid. `null` when it is
 * whole, and for a ring, which the line of effect cuts in ways this does not count.
 */
export function stopsText(wall: WallChoice, laid: number): string | null {
  if (wall.ring || !wall.direction) {
    return null;
  }
  const origin = { col: 0, row: 0 };
  let open = 0;
  for (let k = 0; k <= reachSquares(wall.lengthFt); k++) {
    const at = { col: k * wall.direction.dx, row: k * wall.direction.dy };
    if (rangeFt(origin, at) + SQUARE_FT > wall.lengthFt) {
      break;
    }
    open++;
  }
  if (laid >= open) {
    return null;
  }
  return `A muralha para na parede: ficou com ${metersText(laid * SQUARE_FT)} (${laid} ${laid === 1 ? 'quadrado' : 'quadrados'}) em vez de ${metersText(wall.lengthFt)}.`;
}

/** What the burning side does, with the dice of the slot ("5d8"; empty says only "dano de fogo"). */
export function sideNote(wall: WallChoice, dice: string): string {
  const fire = dice ? `${dice} de fogo` : 'dano de fogo';
  if (!wall.ring) {
    return `A seta no mapa mostra o lado: ${fire} para quem termina o turno ali, até ${metersText(10)}.`;
  }
  const where =
    wall.side === WallSide.OUTER
      ? 'fora do anel'
      : wall.side === WallSide.INNER
        ? 'dentro do anel'
        : 'no lado que queima';
  return `Quem termina o turno ${where}, ou entra na muralha, sofre ${dice ? dice : 'dano'} de fogo.`;
}

/** The wall as `CastSpell.wall` and `PreviewSpellArea.wall` take it: a ring sends no direction. */
export function wallBody(wall: WallChoice) {
  return {
    ring: wall.ring,
    ...(!wall.ring && wall.direction
      ? { direction: { dx: wall.direction.dx, dy: wall.direction.dy } }
      : {}),
    lengthFt: wall.lengthFt,
    side: wall.side,
  };
}

/** The hint under the step's title: how to place the wall, then how to change it. */
export function wallHint(wall: WallChoice, placed: boolean): string {
  if (wall.ring) {
    return placed
      ? 'Centro marcado. Toque de novo no mesmo ponto para confirmar, ou mude o diâmetro e o lado abaixo.'
      : 'Toque no centro do anel. Setas movem o centro; Enter confirma.';
  }
  return placed
    ? 'Começo marcado. Arraste no mapa de novo ou use os botões de direção abaixo; toque no mesmo ponto para confirmar.'
    : 'Toque no começo e arraste para onde vai. Ou use as setas abaixo.';
}

/** What the server laid for a wall, as the zone the map draws: the wall's squares, its burning side and the side that burns. */
export function wallZoneView(
  answer: {
    readonly origin?: Square | undefined;
    readonly squares: readonly Square[];
    readonly burningSquares: readonly Square[];
  },
  side: WallSide,
): ZoneView {
  const squares = (list: readonly Square[]) => list.map((q) => ({ col: q.col, row: q.row }));
  return {
    id: 'wall-preview',
    letter: '',
    spellKey: WALL_SPELL_KEY,
    spellName: '',
    label: '',
    pieces: [],
    detail: '',
    roundsLeft: '',
    concentration: '',
    occupants: '',
    masterOnly: '',
    reminder: '',
    dashed: false,
    detailsHidden: false,
    canMove: false,
    canEnd: false,
    origin: answer.origin ? { col: answer.origin.col, row: answer.origin.row } : null,
    squares: squares(answer.squares),
    // Until the side is picked the preview's burning squares belong to a provisional side: nothing burns on the map yet.
    burningSquares: side === WallSide.UNSPECIFIED ? [] : squares(answer.burningSquares),
    wallSide: side,
  };
}
