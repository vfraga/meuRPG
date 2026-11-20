import type { ResourceTarget } from '../../../gen/meurpg/play/v1/combat_pb';
import { stateWord } from '../combat/combat-view';
import { joinDots, tight } from '../format/text';
import { metersFixed, metersText } from '../units';

/** A creature of a resource dialog's list (Cura pelas Mãos, Inspiração de Bardo). */
export interface ResourceRow {
  readonly id: string;
  readonly label: string;
  /** "Ferida · a 1,5 m". */
  readonly sub: string;
  /** Why it cannot be chosen; empty when it can. Never a creature's type (RN-10). */
  readonly blocked: string;
}

/**
 * The rows of a resource dialog, from `GetTurnOptions.resource_targets`: how hurt it is and how far, in the words of
 * the attack list. The server says who is disabled (`disabled_reason_pt`: "Já tem um dado", "Não ouve você") and the
 * app prints the reason as it came; a creature beyond `reachFt` without a reason of its own says "Longe demais". The
 * creature `selfId` (the paladin touching itself) is marked "(você)" and has no distance.
 */
export function resourceRows(
  targets: readonly ResourceTarget[],
  reachFt: number,
  selfId = '',
): ResourceRow[] {
  return targets.flatMap((t) => {
    const c = t.target;
    if (!c) {
      return [];
    }
    const self = c.combatantId === selfId;
    const parts: string[] = [];
    const word = stateWord(c.state);
    if (word) {
      parts.push(word);
    }
    if (!self && c.distanceFt !== undefined) {
      parts.push(`a ${metersFixed(c.distanceFt)}`);
    }
    const far = c.tooFar ? tight(`Longe demais: alcance de ${metersText(reachFt)}`) : '';
    return [
      {
        id: c.combatantId,
        label: self ? `${c.label} (você)` : c.label,
        sub: tight(joinDots(parts)),
        blocked: t.disabledReasonPt || far,
      },
    ];
  });
}
