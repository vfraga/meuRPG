import type { Combatant } from '../../../gen/meurpg/play/v1/combat_pb';
import { tight } from '../format/text';
import { metersFixed, reachSquares, squaresText } from '../units';
import { grappledWord } from './contest-view';

/** The line of the movement row under the map ("Restam 9,0 m · 6 quadrados"), its bar, and whether there is a move to make. */
export interface MoveLine {
  /** The bold part: "Restam 9,0 m", "Sem movimento", "Indisponível: Agarrado.". */
  readonly lead: string;
  /** What follows it after a dot: "6 quadrados", "6 quadrados · sem provocar". Empty when nothing follows. */
  readonly rest: string;
  /** The metres left alone ("9,0 m"), for the sticky bar and the walker shape. */
  readonly left: string;
  /** Movement left over speed, 0 to 100. */
  readonly percent: number;
  /** There is movement to spend: "Mover" can open the page. */
  readonly canMove: boolean;
}

/**
 * The movement of the player's own turn, in tenths of a foot as the server keeps it and metres with one decimal. A grappled
 * combatant's speed is 0 (SRD 5.1, Conditions): the line says why. After Desengajar the squares say they do not provoke.
 */
export function moveLine(own: Combatant, grappled: boolean): MoveLine {
  const left = metersFixed(own.movementLeftDft / 10);
  if (grappled) {
    return {
      lead: `Indisponível: ${grappledWord()}.`,
      rest: '',
      left,
      percent: 0,
      canMove: false,
    };
  }
  const total = Math.max(1, own.speedDft);
  const percent = Math.max(0, Math.min(100, (own.movementLeftDft / total) * 100));
  if (own.movementLeftFt <= 0) {
    return { lead: 'Sem movimento', rest: '', left, percent, canMove: false };
  }
  const squares = squaresText(reachSquares(own.movementLeftFt));
  return {
    lead: tight(`Restam ${left}`),
    rest: own.disengaged ? `${squares} · sem provocar` : squares,
    left,
    percent,
    canMove: true,
  };
}
