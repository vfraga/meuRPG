import { type ReadiedAction, ReadiedKind } from '../../../gen/meurpg/play/v1/combat_pb';
import { metersText } from '../units';

/** The words of a held action that the card, the lines and the prompts share: a leaf, so the log can read them. */

/** The name the card and the lines give a held action: "Espada longa no Zumbi 1", "Mover, até 7,5 m", "Esquivar". */
export function heldName(
  r: Pick<ReadiedAction, 'kind' | 'actionNamePt' | 'moveFt'>,
  target = '',
): string {
  if (r.kind === ReadiedKind.MOVE) {
    return r.moveFt > 0 ? `Mover, até ${metersText(r.moveFt)}` : 'Mover';
  }
  const name = r.actionNamePt || 'ação preparada';
  return target ? `${name} ${target}` : name;
}
