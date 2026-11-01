import { type HeldRollOffer, HeldRollKind } from '../../../gen/meurpg/play/v1/combat_pb';
import { LUCKY_TITLE } from '../combat/lucky';
import { inspirationDieName, promptTitle } from '../resources/bardic-inspiration';
import {
  type HeldAnswer,
  asksIndomitable,
  asksLucky,
  asksStroke,
  dieOf,
  indomitableFailed,
  strokeFailed,
} from './held-roll-offer';

/** The words of a question the master reads about someone else's roll. */

/** The title of the fighter's question: it says the save failed only when the roller may know it. */
export function indomitableTitle(offer: HeldRollOffer | null | undefined): string {
  return indomitableFailed(offer)
    ? 'Falhou na salvaguarda. Usar Indomitável para rolar de novo?'
    : 'Usar Indomitável para rolar a salvaguarda de novo?';
}

/** The line under Indomitable's title: the new roll must be used. */
export const INDOMITABLE_WHAT =
  'Indomitável: você rola a salvaguarda de novo e precisa ficar com o novo resultado, mesmo que seja pior. Gasta um uso, que volta depois de um descanso longo.';

/** The title of the rogue's question: it says the attack missed or the check failed only when the roller may know it. */
export function strokeTitle(offer: HeldRollOffer | null | undefined): string {
  const attack =
    offer?.kind === HeldRollKind.WEAPON_ATTACK || offer?.kind === HeldRollKind.SPELL_ATTACK;
  if (attack) {
    return strokeFailed(offer)
      ? 'Errou o ataque. Usar Golpe de Sorte para acertar?'
      : 'Usar Golpe de Sorte para acertar o ataque?';
  }
  return strokeFailed(offer)
    ? 'Falhou no teste. Usar Golpe de Sorte para contar o d20 como 20?'
    : 'Usar Golpe de Sorte para contar o d20 como 20?';
}

/** The line under Stroke of Luck's title. */
export const STROKE_WHAT =
  'Golpe de Sorte: um ataque que erra vira um acerto, ou um teste de habilidade conta o d20 como 20. Gasta o uso, que volta depois de um descanso curto ou longo.';

/** The title the roller reads: "Tirou 1: rolar de novo?", "Usar a Inspiração de Bardo (d8)?" or Indomitable's. */
export function questionTitle(offer: HeldRollOffer): string {
  if (asksStroke(offer)) {
    return strokeTitle(offer);
  }
  if (asksIndomitable(offer)) {
    return indomitableTitle(offer);
  }
  return asksLucky(offer) ? LUCKY_TITLE : promptTitle(dieOf(offer)?.sides ?? 0);
}

const KIND_TEXT: Readonly<Record<HeldRollKind, string>> = {
  [HeldRollKind.UNSPECIFIED]: 'Rolagem de d20',
  [HeldRollKind.WEAPON_ATTACK]: 'Ataque com arma',
  [HeldRollKind.SPELL_ATTACK]: 'Ataque com magia',
  [HeldRollKind.EFFECT_SAVE]: 'Teste de resistência contra um efeito',
  [HeldRollKind.CONCENTRATION_SAVE]: 'Teste de resistência para manter a concentração',
  [HeldRollKind.SCENE_CHECK]: 'Teste da cena',
  [HeldRollKind.GROUP_CHECK]: 'Teste em grupo',
  [HeldRollKind.TRAP_SEARCH]: 'Busca por armadilhas',
  [HeldRollKind.HINT_TRY]: 'Tentativa de dica do quebra-cabeça',
  [HeldRollKind.DEATH_SAVE]: 'Teste contra a morte',
  [HeldRollKind.CONTEST]: 'Disputa',
  [HeldRollKind.HIDE]: 'Esconder-se',
  [HeldRollKind.COUNTERSPELL_CHECK]: 'Teste da Contramágica',
  [HeldRollKind.INITIATIVE]: 'Iniciativa',
};

/** What was rolled: "Ataque com arma contra Goblin chefe." when the target is known (an attack only), else the kind of the
 * roll. */
export function questionWhat(offer: HeldRollOffer, targetLabel: string): string {
  const attack =
    offer.kind === HeldRollKind.WEAPON_ATTACK || offer.kind === HeldRollKind.SPELL_ATTACK;
  const kind = KIND_TEXT[offer.kind] ?? KIND_TEXT[HeldRollKind.UNSPECIFIED];
  return attack && targetLabel ? `${kind} contra ${targetLabel}.` : `${kind}.`;
}

/** The rule of the question, its lead in bold: "Sortudo: Brisa pode rolar esse d20 de novo e ficar com o novo resultado."
 * With a pair of dice only the one that showed the 1 is rolled again (SRD 5.1). */
export function questionRule(offer: HeldRollOffer): {
  readonly lead: string;
  readonly text: string;
} {
  if (asksStroke(offer)) {
    return {
      lead: 'Golpe de Sorte:',
      text: `${offer.rollerLabel || 'Quem rolou'} pode transformar o erro em acerto ou contar o d20 do teste como 20. Gasta o uso.`,
    };
  }
  if (asksIndomitable(offer)) {
    return {
      lead: 'Indomitável:',
      text: `${offer.rollerLabel || 'Quem rolou'} pode rolar a salvaguarda de novo, mas precisa ficar com o novo resultado, mesmo que seja pior.`,
    };
  }
  if (asksLucky(offer)) {
    const roller = offer.rollerLabel || 'Quem rolou';
    const pair = (offer.d20?.faces.length ?? 0) > 1;
    return {
      lead: 'Sortudo:',
      text: pair
        ? `${roller} pode rolar de novo o dado que deu 1 e ficar com o novo resultado.`
        : `${roller} pode rolar esse d20 de novo e ficar com o novo resultado.`,
    };
  }
  const giver = dieOf(offer)?.fromName;
  return {
    lead: '',
    text: giver
      ? `O dado é de ${giver} e se perde depois de rolado.`
      : 'O dado se perde depois de rolado.',
  };
}

/** The d20 in the chip: "1", or "1 e 13" for a roll with advantage or disadvantage; the die that counts once Lucky is past. */
export function d20Chip(offer: HeldRollOffer): string {
  const roll = offer.d20;
  if (!roll) {
    return '';
  }
  return asksLucky(offer)
    ? roll.faces.join(' e ')
    : String(roll.faces[roll.countedIndex] ?? roll.faces[0] ?? '');
}

/** "d20 · bônus +5": the bonus beside the chip; the die's question adds the total, which Lucky's does not (the 1 may still be
 * rolled again). */
export function d20Line(offer: HeldRollOffer): string {
  const roll = offer.d20;
  if (!roll) {
    return '';
  }
  const bonus =
    roll.modifier === 0
      ? ''
      : ` · bônus ${roll.modifier > 0 ? '+' : '−'}${Math.abs(roll.modifier)}`;
  return `d20${bonus}${asksLucky(offer) ? '' : ` · total ${roll.total}`}`;
}

/** "2 perguntas pendentes"; `''` for none. */
export function pendingCountText(count: number): string {
  if (count <= 0) {
    return '';
  }
  return count === 1 ? '1 pergunta pendente' : `${count} perguntas pendentes`;
}

/** What the master's answer is doing while it is on the way: "Rolando o d20 de novo…". */
export function askingText(answer: HeldAnswer, offer: HeldRollOffer): string {
  return answer.kind === 'stroke' ? strokeAskingText(answer.use) : rollAskingText(answer, offer);
}

function strokeAskingText(use: boolean): string {
  return use ? 'Usando o Golpe de Sorte…' : 'Mantendo o resultado…';
}

function rollAskingText(
  answer: Exclude<HeldAnswer, { readonly kind: 'stroke' }>,
  offer: HeldRollOffer,
): string {
  if (answer.kind === 'indomitable') {
    if (!answer.reroll) {
      return 'Mantendo a salvaguarda…';
    }
    return answer.roll && 'face' in answer.roll
      ? 'Enviando o resultado…'
      : 'Rolando a salvaguarda de novo…';
  }
  if (answer.kind === 'lucky') {
    if (!answer.reroll) {
      return 'Ficando com o resultado do d20…';
    }
    return answer.roll && 'face' in answer.roll
      ? 'Enviando o resultado…'
      : 'Rolando o d20 de novo…';
  }
  if (!answer.use) {
    return 'Guardando o dado…';
  }
  return answer.roll && 'face' in answer.roll
    ? 'Enviando o resultado…'
    : `Rolando o ${inspirationDieName(dieOf(offer)?.sides ?? 0)}…`;
}
