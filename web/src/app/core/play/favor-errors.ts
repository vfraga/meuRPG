import { Code, ConnectError } from '@connectrpc/connect';

import {
  FavorBlockedReason,
  type FavorBlocked,
  FavorBlockedSchema,
} from '../../../gen/meurpg/play/v1/favor_pb';

/** The typed detail of a `failed_precondition` from a favor call (the Inspiração the master grants), or
 * `null`. Never read from the message. */
export function favorBlocked(err: unknown): FavorBlocked | null {
  const connectErr = ConnectError.from(err, Code.Unavailable);
  if (connectErr.code !== Code.FailedPrecondition) {
    return null;
  }
  return (
    connectErr
      .findDetails(FavorBlockedSchema)
      .find((d) => d.$typeName === FavorBlockedSchema.typeName) ?? null
  );
}

/** What each refusal says, in words the master or the player can act on. */
export function favorBlockedMessage(reason: FavorBlockedReason): string {
  switch (reason) {
    case FavorBlockedReason.ALREADY_HAS:
      return 'Essa personagem já tem Inspiração.';
    case FavorBlockedReason.NOT_A_PLAYER_CHARACTER:
      return 'Só personagens de jogadores têm Inspiração.';
    case FavorBlockedReason.NOT_THE_HOLDER:
      return 'Só quem tem a Inspiração pode usá-la.';
    case FavorBlockedReason.HAS_NONE:
      return 'Você não tem Inspiração.';
    case FavorBlockedReason.CANNOT_RECEIVE:
      return 'Essa personagem não pode ficar com a Inspiração agora.';
    case FavorBlockedReason.SELF:
      return 'Você não pode passar a Inspiração a si mesmo.';
    case FavorBlockedReason.ADDS_NOTHING:
      return 'A Inspiração não mudaria esta rolagem.';
    case FavorBlockedReason.AFTER_THE_ROLL:
      return 'O d20 já foi rolado: a Inspiração não pode mais ser usada.';
    case FavorBlockedReason.NOTHING_TO_ROLL:
      return 'Esta ação não rola um d20: a Inspiração não pode ser usada.';
    case FavorBlockedReason.PASS_PENDING:
      return 'Você pediu para passar a Inspiração. Cancele o pedido para usar.';
    case FavorBlockedReason.PASS_CLOSED:
      return 'Esse pedido já foi respondido em outra tela.';
    case FavorBlockedReason.NO_SESSION:
      return 'Só numa sessão aberta.';
    default:
      return 'A Inspiração não pode ser usada agora.';
  }
}

/** What a refused spend says (the `FavorBlocked` of a roll's `failed_precondition`), or `null` when the error is another. */
export function favorRefusalText(err: unknown): string | null {
  const blocked = favorBlocked(err);
  return blocked ? favorBlockedMessage(blocked.reason) : null;
}

/** The `spend_favor` a request carries: set only when the roll was made with the Inspiração, so a request without it is the
 * one it always was (and a roll that never spends, like leaving the roll to the master, never carries it). */
export function spendFavorOf(how: { readonly spendFavor?: boolean }): { spendFavor?: true } {
  return how.spendFavor ? { spendFavor: true } : {};
}
