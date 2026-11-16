import { Code, ConnectError } from '@connectrpc/connect';

import {
  EncounterBlockedReason,
  EncounterBlockedSchema,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { joinDots } from '../format/text';
import { steedTypeWord } from './steed';

/** How a bonded steed goes: `dismiss` keeps the bond, `release` ends it (`CreatureDismissal`). */
export type SteedGoing = 'dismiss' | 'release';

/** Under "Libertar Estrela" in the card's "⋮" menu. */
export const STEED_RELEASE_LINE = 'Acaba o vínculo: a próxima é outra montaria.';

/** "Cavalo de guerra · Grande · celestial": the form, the size and the type's word; the tag "Montaria vinculada" says whose. */
export function steedSubtitle(form: string, size: string, type: string): string {
  return joinDots([form, size, steedTypeWord(type)].filter((part) => part !== ''));
}

/** What the question says before a dismissal or a release; `owner` is empty for the owner's own view, the owner's name for the master's. */
export function steedAskText(going: SteedGoing, name: string, owner: string): string {
  const sheet = owner ? `da ficha de ${owner}` : 'da sua ficha';
  if (going === 'release') {
    return `O vínculo acaba: ${name} some do mapa e ${sheet}, e a próxima Convocar Montaria escolhe de novo a forma e o tipo.`;
  }
  return `${name} some do mapa e ${sheet}, e ${owner || 'você'} desce de pé se estiver montado. O vínculo fica: Convocar Montaria traz ${name} de volta.`;
}

/**
 * In a combat the owner's player spends the action of its turn to send the steed away: the two refusals of that, in the
 * words the card shows under its actions. `''` for any other error, which stays in the question.
 */
export function steedRefusal(err: unknown, owner: string): string {
  const connectErr = ConnectError.from(err, Code.Unavailable);
  if (connectErr.code !== Code.FailedPrecondition) {
    return '';
  }
  switch (connectErr.findDetails(EncounterBlockedSchema)[0]?.reason) {
    case EncounterBlockedReason.NOT_YOUR_TURN:
      return `Só na vez de ${owner}: dispensar gasta a ação.`;
    case EncounterBlockedReason.ACTION_USED:
      return 'Você já usou a ação desta vez.';
    default:
      return '';
  }
}

/** Under a bonded steed that is not among the live creatures (the dashed "Fora" row). */
export function steedAwayLine(name: string): string {
  return `Convocar Montaria traz ${name} de volta.`;
}
