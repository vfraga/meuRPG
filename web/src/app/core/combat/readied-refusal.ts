import { Code, ConnectError } from '@connectrpc/connect';

import { type ReadiedAction, ReadiedKind } from '../../../gen/meurpg/play/v1/combat_pb';
import { combatErrorMessage } from './combat-errors';

/**
 * The words of a refused answer of a readied action. A held spell is released with `AnswerReaction`, which carries creatures and the
 * d20 but no darts (Magic Missile) and no point on the map (an area): the server refuses those with `invalid_argument`, and the
 * usual words ("confira os campos") would send the player looking for a field. The readier is off its turn while the window is open,
 * so the server lists no targets or darts for the held spell and the app cannot tell beforehand.
 */
export function readiedRefusalText(
  err: unknown,
  readied: ReadiedAction | undefined,
  doing: string,
): string {
  if (
    readied?.kind === ReadiedKind.SPELL &&
    err instanceof ConnectError &&
    err.code === Code.InvalidArgument
  ) {
    return 'Não deu para soltar a magia por aqui: ela pede dardos ou um ponto no mapa, que este aviso ainda não envia. Dá para deixar passar.';
  }
  return combatErrorMessage(err, doing);
}
