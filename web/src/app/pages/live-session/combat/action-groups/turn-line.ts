import { grappledWord } from '../../../../core/combat/contest-view';
import { moveLine } from '../../../../core/combat/move-line';
import { tight } from '../../../../core/format/text';
import { metersFixed, reachSquares, squaresText } from '../../../../core/units';
import type { Combatant } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { type EconomyShape, economyShapes } from '../../../../shared/economy-shapes/economy-shapes';

/** What the first row of "Ações" reads: the shapes (with the metres left as the walker) and whether "Mover" may be offered. */
export function turnLineOf(
  own: Combatant,
  o: {
    readonly grappled: boolean;
    /** No map, or a surprised character: the metres stay on the walker, "Mover" goes. */
    readonly blocked: boolean;
    readonly attacksLeft: number;
    readonly attacksPerAction: number;
  },
): { readonly canMove: boolean; readonly shapes: EconomyShape[] } {
  const line = moveLine(own, o.grappled);
  return {
    canMove: line.canMove && !o.blocked,
    shapes: economyShapes(own, {
      attacksLeft: o.attacksLeft,
      attacksPerAction: o.attacksPerAction,
      moveText: line.canMove ? line.left : null,
    }),
  };
}

/** The movement the Mover row reads: the feet left, the pill ("Restam 6,9 m") and the sentence that says what the turn allows. */
export function movementOf(
  own: Combatant,
  grappled: boolean,
): { readonly left: number; readonly pill: string; readonly text: string } {
  const left = own.movementLeftFt;
  if (grappled) {
    // Grappled: the speed is 0 (SRD 5.1, Conditions), and the line says why.
    return { left: 0, pill: 'Sem movimento', text: `Indisponível: ${grappledWord()}.` };
  }
  // Tenths of a foot, metres with one decimal ("Restam 6,9 m").
  const leftM = metersFixed(own.movementLeftDft / 10);
  const sentence = `${leftM} (${squaresText(reachSquares(left))})`;
  let text = 'Você já usou todo o movimento deste turno.';
  if (left > 0 && own.disengaged) {
    text = tight(`Dá para andar até ${sentence} sem provocar.`);
  } else if (left > 0) {
    text = tight(
      own.movementUsedDft > 0
        ? `Você já andou ${metersFixed(own.movementUsedDft / 10)}. Dá para andar mais ${sentence}.`
        : `Você ainda não andou. Dá para andar até ${sentence}.`,
    );
  }
  return { left, pill: left > 0 ? tight(`Restam ${leftM}`) : 'Sem movimento', text };
}
