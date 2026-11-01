import { Code, ConnectError } from '@connectrpc/connect';

import { ResourceBlockedReason } from '../../../gen/meurpg/play/v1/resources_pb';
import { encounterBlocked, sessionClosed } from '../combat/combat-errors';
import { classResourceErrorMessage, resourceBlocked } from '../resources/resources-errors';
import type { HeldAnswer } from './held-roll-offer';

/** The refusal reason (a `ResourceBlockedReason` name) the server gives the master who answers before the roller had
 * the first seconds to (the roller answers first). The server names it with the master's panel; until it does, a
 * refusal whose detail carries this name is read as "wait", never as an error. */
export const MASTER_TOO_EARLY_REASON = 'MASTER_TOO_EARLY';

/** What a refusal of `AnswerHeldRoll` means beyond its words. */
export type HeldRefusal =
  /** The roll was answered already (the roller's player and the master both tried; the first answer won). */
  | 'answered'
  /** The master answered inside the roller's first seconds: wait for the clock, then try again. */
  | 'too-early';

/** The meaning of a refusal that is not a failure: `null` for any other error. */
export function heldRollRefusal(err: unknown): HeldRefusal | null {
  const connectErr = ConnectError.from(err, Code.Unavailable);
  if (connectErr.code !== Code.FailedPrecondition) {
    return null;
  }
  const blocked = resourceBlocked(err);
  if (blocked) {
    return ResourceBlockedReason[blocked.reason] === MASTER_TOO_EARLY_REASON ? 'too-early' : null;
  }
  // A plain `failed_precondition` with no detail of the rules is the race: the roll was answered already.
  return encounterBlocked(err) === null && !sessionClosed(err) ? 'answered' : null;
}

/** "Essa pergunta já foi respondida. O resultado está no registro do combate." */
export const HELD_ROLL_ANSWERED_TEXT =
  'Essa pergunta já foi respondida. O resultado está no registro do combate.';

/** What an answer that did not get through says: the race lost, the master too early (`seconds` is how long the roller
 * still has), or the words every resource call shares. */
export function heldRollErrorMessage(
  err: unknown,
  answer: HeldAnswer,
  who: { readonly roller: string; readonly seconds: number },
): string {
  switch (heldRollRefusal(err)) {
    case 'answered':
      return HELD_ROLL_ANSWERED_TEXT;
    case 'too-early':
      return `Ainda é a vez de ${who.roller || 'quem rolou'} responder. Você pode responder em ${Math.max(1, who.seconds)} s.`;
    default:
      return classResourceErrorMessage(
        err,
        answer.kind === 'lucky'
          ? 'responder ao Sortudo'
          : answer.kind === 'indomitable'
            ? 'responder ao Indomitável'
            : answer.kind === 'stroke'
              ? 'responder ao Golpe de Sorte'
              : 'responder sobre a Inspiração de Bardo',
      );
  }
}
