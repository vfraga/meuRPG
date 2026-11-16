import { Code, ConnectError } from '@connectrpc/connect';

import {
  type Encounter,
  EncounterBlockedReason,
  EncounterBlockedSchema,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { article } from '../format/article';
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

/** Where a bonded steed is when its question is asked: in a running combat, and under its owner. */
export interface SteedWhere {
  readonly combat: boolean;
  readonly mounted: boolean;
}

/** Reads `SteedWhere` from the running combat (`null`: none runs). */
export function steedWhere(
  enc: Pick<Encounter, 'combatants'> | null,
  creatureId: string,
  characterId: string,
): SteedWhere {
  const steed = enc?.combatants.find((c) => c.creatureId === creatureId);
  if (!enc || !steed) {
    return { combat: false, mounted: false };
  }
  return {
    combat: true,
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
  const fem = article(name) === 'a';
  const free = !owner && !where.combat ? ' Fora de combate, não gasta nada.' : '';
  if (going === 'release') {
    const sheet = owner ? `da ficha de ${owner}` : 'da ficha';
    return `O vínculo acaba: ${name} some ${sheet} e do mapa, e a próxima Convocar Montaria escolhe de novo a forma e o tipo.${free}`;
  }
  const sheet = owner ? `da ficha de ${owner}` : 'da sua ficha';
  const lands = where.mounted
    ? `, e ${owner || 'você'} desce de pé no quadrado ${fem ? 'dela' : 'dele'}`
    : '';
  return `${name} some do mapa e ${sheet}${lands}. O vínculo fica: Convocar Montaria ${fem ? 'a' : 'o'} traz de volta.${free}`;
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

/** The dashed "Fora" row's line: "Cavalo de guerra · celestial · fora da mesa". */
export function steedAwaySubtitle(form: string, type: string): string {
  return joinDots([form, steedTypeWord(type), 'fora da mesa'].filter((part) => part !== ''));
}

/** Under the cast button of Convocar Montaria: what the cast does for a steed that is away, or for one that is here. */
export function steedCastHint(name: string, away: boolean): string {
  const fem = article(name) === 'a';
  return away
    ? `Traz ${name} de volta, com a forma e o tipo ${fem ? 'dela' : 'dele'} e os PV cheios.`
    : `Com ${name} aqui, ${fem ? 'ela' : 'ele'} volta aos PV cheios.`;
}
