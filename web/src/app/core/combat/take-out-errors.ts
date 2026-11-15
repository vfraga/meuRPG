import { Code, ConnectError } from '@connectrpc/connect';

import { EncounterBlockedReason } from '../../../gen/meurpg/play/v1/combat_pb';
import { combatErrorMessage, encounterBlocked } from './combat-errors';
import { undoChangedText } from './combat-errors-limits';

/** What the master reads when the server refuses "Tirar e pedir a soma" or "Tirar": every refusal changes nothing. */
const REFUSALS: ReadonlyMap<EncounterBlockedReason, string> = new Map([
  [
    EncounterBlockedReason.NOTHING_TO_UNDO,
    'Já aconteceu outra coisa depois deste dano; use Desfazer ou corrija os PV. Nada mudou.',
  ],
  [EncounterBlockedReason.DAMAGE_RESOLVED, 'Esse dano já foi aplicado ou descartado. Nada mudou.'],
  [
    EncounterBlockedReason.DAMAGE_PART_NOT_REMOVABLE,
    'Só um extra pode ser tirado do dano, e uma vez só. Nada mudou.',
  ],
  [
    EncounterBlockedReason.DAMAGE_NOT_ROLLED,
    'Este dano já espera uma nova soma: digite-a antes de tirar outra parte. Nada mudou.',
  ],
]);

const GENERIC = 'Não deu para tirar a parte agora. Nada mudou. Tente de novo.';

/** The banner of a refused take-out, in the app's voice. */
export function takeOutErrorMessage(
  err: unknown,
  labelOf: (id: string) => string | undefined = () => undefined,
): string {
  const blocked = encounterBlocked(err);
  if (blocked?.reason === EncounterBlockedReason.UNDO_TARGET_CHANGED) {
    // The Desfazer's own words: whose hit points changed, and that nothing was undone. A take-out undoes the same way.
    return `${undoChangedText(blocked, labelOf)} Corrija os PV à mão.`;
  }
  const reason = blocked?.reason;
  const said = reason === undefined ? undefined : REFUSALS.get(reason);
  if (said) {
    return said;
  }
  const code = ConnectError.from(err, Code.Unavailable).code;
  if (code === Code.Unavailable || code === Code.Unknown || code === Code.Internal) {
    return GENERIC;
  }
  return combatErrorMessage(err, 'tirar a parte');
}
