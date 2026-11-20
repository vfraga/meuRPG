import { Code, ConnectError } from '@connectrpc/connect';

import {
  InfusionBlockedReason,
  InfusionBlockedSchema,
} from '../../../gen/meurpg/characters/v1/infusions_pb';
import { describeCharacterError } from './character-errors';

/**
 * What a refused infusion says, by the typed `InfusionBlocked` reason and never by the message.
 * `who` is the name of the character that infuses, for the sentence that names it ("Faísca não conhece esta infusão.").
 * Any other error (a dead character, an attunement, a stale sheet) is the sheet's usual wording (`describeCharacterError`).
 */
export function describeInfusionError(err: unknown, who: string): string {
  const connectErr = ConnectError.from(err, Code.Unavailable);
  const [detail] =
    connectErr.code === Code.FailedPrecondition
      ? connectErr.findDetails(InfusionBlockedSchema)
      : [];
  if (!detail) {
    return describeCharacterError(err);
  }
  switch (detail.reason) {
    case InfusionBlockedReason.ITEM_ALREADY_INFUSED:
      return 'Esse item já tem uma infusão.';
    case InfusionBlockedReason.ITEM_NOT_ACCEPTED:
      return 'Esse item não aceita esta infusão.';
    case InfusionBlockedReason.ITEM_CHANGED:
      return 'O item mudou enquanto você escolhia: escolha de novo.';
    case InfusionBlockedReason.INFUSION_NOT_KNOWN:
      return `${who} não conhece esta infusão.`;
    default:
      return 'Não deu para infundir agora.';
  }
}
