import type { AnswerHeldRollResponse } from '../../../gen/meurpg/play/v1/resources_pb';
import type { HeldRollOffer } from '../../../gen/meurpg/play/v1/combat_pb';
import type { WithRerolls } from '../combat/lucky';
import { inspirationDieName } from '../resources/bardic-inspiration';
import { type HeldAnswer, dieOf, indomitableReroll, luckyReroll } from './held-roll-offer';

/** A d20 an answer settled, with what is known of its total where the path's roll carries it. */
export type SettledRoll = WithRerolls & { readonly modifier?: number; readonly total?: number };

/**
 * The d20 an answer settled, where each path tells it: on the next question when the roll waits for it now, else on the
 * roll written. `undefined` for a path whose result carries no d20 (a cast, the roll that asked another question).
 */
export function settledRoll(res: AnswerHeldRollResponse): SettledRoll | undefined {
  if (res.next) {
    return res.next.d20;
  }
  const r = res.result;
  switch (r.case) {
    case 'attack':
      return r.value.roll?.d20;
    case 'sceneCheck':
      return r.value.roll?.roll;
    case 'trapSearch':
    case 'hintTry':
      return r.value.roll;
    case 'groupCheck':
      // A player reads only their own member of the check.
      return r.value.groupCheck?.members.find((m) => m.roll)?.roll;
    case 'effectSave':
      return r.value.result;
    case 'concentrationSave':
      return r.value.result?.save;
    default:
      return undefined;
  }
}

/** The d20 that counts on a question: "1", or the die of a pair that shows the 1. */
function countedFace(offer: HeldRollOffer): number {
  const roll = offer.d20;
  return roll?.faces[roll.countedIndex] ?? roll?.faces[0] ?? 0;
}

/** What the master's answer did, in the master's own words: "Você respondeu por Brisa: rolou de novo e tirou 14 (19 com o
 * bônus).". The new face and the total come from the path's result; where it carries none the line says only what was
 * chosen. */
export function answeredText(
  roller: string,
  answer: HeldAnswer,
  asked: HeldRollOffer,
  res: AnswerHeldRollResponse,
): string {
  return `Você respondeu por ${roller}: ${whatAnswered(answer, asked, res)}.`;
}

function whatAnswered(
  answer: HeldAnswer,
  asked: HeldRollOffer,
  res: AnswerHeldRollResponse,
): string {
  if (answer.kind === 'die') {
    return answer.use
      ? `somou o ${inspirationDieName(dieOf(asked)?.sides ?? 0)}`
      : 'guardou o dado';
  }
  if (answer.kind === 'stroke') {
    return answer.use ? 'usou o Golpe de Sorte' : 'manteve o resultado';
  }
  if (!answer.reroll) {
    return answer.kind === 'indomitable'
      ? `manteve a salvaguarda (${countedFace(asked)})`
      : `ficou com o ${countedFace(asked)}`;
  }
  const roll = settledRoll(res);
  const again = answer.kind === 'indomitable' ? indomitableReroll(roll) : luckyReroll(roll);
  if (!again) {
    return 'rolou de novo';
  }
  const withBonus =
    roll?.modifier && roll.total !== undefined ? ` (${roll.total} com o bônus)` : '';
  return `rolou de novo e tirou ${again.to}${withBonus}`;
}
