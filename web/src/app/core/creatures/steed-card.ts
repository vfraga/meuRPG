import { Code, ConnectError } from '@connectrpc/connect';

import {
  type Encounter,
  EncounterBlockedReason,
  EncounterBlockedSchema,
  EncounterStatus,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { ContestBlockedReason } from '../../../gen/meurpg/play/v1/contest_types_pb';
import { contestBlocked, contestBlockedMessage, encounterBlocked } from '../combat/combat-errors';
import { joinDots } from '../format/text';
import { creatureErrorMessage } from './creature-errors';
import { steedTypeWord } from './steed';

/** How a bonded steed goes: `dismiss` keeps the bond, `release` ends it (`CreatureDismissal`). */
export type SteedGoing = 'dismiss' | 'release';

/** Under "Libertar Estrela" in the card's "⋮" menu. */
export const STEED_RELEASE_LINE = 'Acaba o vínculo: a próxima é outra montaria.';

/** "Cavalo de guerra · Grande · celestial": the form, the size and the type's word; the tag "Montaria vinculada" says whose. */
export function steedSubtitle(form: string, size: string, type: string): string {
  return joinDots([form, size, steedTypeWord(type)].filter((part) => part !== ''));
}

/** Where a bonded steed is when its question is asked: in a running combat, and under its owner. */
export interface SteedWhere {
  /** A combat holds the steed. */
  readonly combat: boolean;
  /** That combat is running: the owner's player spends the action (`SpendDismissal` charges nothing while it is set up). */
  readonly spends: boolean;
  readonly mounted: boolean;
}

export const STEED_NOWHERE: SteedWhere = { combat: false, spends: false, mounted: false };

/** Reads `SteedWhere` from the running combat (`null`: none runs). */
export function steedWhere(
  enc: Pick<Encounter, 'combatants' | 'status'> | null,
  creatureId: string,
  characterId: string,
): SteedWhere {
  const steed = enc?.combatants.find((c) => c.creatureId === creatureId);
  if (!enc || !steed) {
    return STEED_NOWHERE;
  }
  return {
    combat: true,
    spends: enc.status === EncounterStatus.ACTIVE,
    mounted: enc.combatants.some(
      (c) => c.characterId === characterId && c.mountedOnId === steed.id,
    ),
  };
}

/**
 * What the question says before a dismissal or a release, in the words of boards 588-24 and 588-25. `owner` is empty for
 * the owner's own view, the owner's name for the master's. The rider's landing is said only with the owner on the steed;
 * "Fora de combate, não gasta nada." only to the owner (the master never spends) and only with no combat holding the steed.
 */
export function steedAskText(
  going: SteedGoing,
  name: string,
  owner: string,
  where: SteedWhere,
): string {
  const free = !owner && !where.combat ? ' Fora de combate, não gasta nada.' : '';
  if (going === 'release') {
    const sheet = owner ? `da ficha de ${owner}` : 'da ficha';
    return `O vínculo acaba: ${name} some ${sheet} e do mapa, e a próxima Convocar Montaria escolhe de novo a forma e o tipo.${free}`;
  }
  const sheet = owner ? `da ficha de ${owner}` : 'da sua ficha';
  const lands = where.mounted
    ? `, e ${owner || 'você'} desce de pé no quadrado onde ${name} estava`
    : '';
  return `${name} some do mapa e ${sheet}${lands}. O vínculo fica: Convocar Montaria traz ${name} de volta.${free}`;
}

/**
 * In a combat the owner's player spends the action of its turn to send the steed away: the two refusals of that, in the
 * words the card shows under its actions, and the rest of the server's gate (down, cannot act, surprised) in the combat's. `''` for any other error, which stays in the question.
 */
export function steedRefusal(err: unknown, owner: string): string {
  const connectErr = ConnectError.from(err, Code.Unavailable);
  if (connectErr.code !== Code.FailedPrecondition) {
    return '';
  }
  const contest = contestBlocked(err);
  if (contest?.reason === ContestBlockedReason.SURPRISED) {
    return contestBlockedMessage(contest);
  }
  switch (connectErr.findDetails(EncounterBlockedSchema)[0]?.reason) {
    case EncounterBlockedReason.NOT_YOUR_TURN:
      return `Só na vez de ${owner}: dispensar gasta a ação.`;
    case EncounterBlockedReason.ACTION_USED:
      return 'Você já usou a ação desta vez.';
    case EncounterBlockedReason.COMBATANT_DOWN:
      return 'Com 0 PV, não age.';
    case EncounterBlockedReason.CANNOT_ACT:
      return 'Não dá para agir agora.';
    default:
      return '';
  }
}

/** Any other failure, in the question: a refusal of the combat never sends the person to close a sheet (the card is none). */
export function steedAskError(err: unknown): string {
  return encounterBlocked(err)
    ? 'Não dá para fazer isso agora. Tente de novo.'
    : creatureErrorMessage(err, 'dismiss');
}

/** Under the actions of a bonded steed's card (588-23): what "Dispensar" keeps, and to the owner that it is free out of a combat. */
export function steedActionsHint(name: string, master: boolean): string {
  const keeps = `Dispensar mantém o vínculo: Convocar Montaria traz ${name} de volta.`;
  return master ? keeps : `${keeps} Fora de combate, não gasta nada.`;
}

/** The dashed "Fora" row's line: "Cavalo de guerra · celestial · fora da mesa". */
export function steedAwaySubtitle(form: string, type: string): string {
  return joinDots([form, steedTypeWord(type), 'fora da mesa'].filter((part) => part !== ''));
}

/** Under the cast button of Convocar Montaria: what the cast does for a steed that is away, or for one that is here. */
export function steedCastHint(name: string, away: boolean): string {
  return away
    ? `Traz ${name} de volta, com a mesma forma e o mesmo tipo e os PV cheios.`
    : `Com ${name} aqui, os PV voltam cheios.`;
}
