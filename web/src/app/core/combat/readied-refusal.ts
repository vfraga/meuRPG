import { Code, ConnectError } from '@connectrpc/connect';

import { type ReadiedAction, ReadiedKind } from '../../../gen/meurpg/play/v1/combat_pb';
import { combatErrorMessage } from './combat-errors';
import type { ReleasePlan } from './readied-release';

/**
 * The words of a refused answer of a readied action. A held spell is released with `AnswerReaction`, which carries the targets with
 * their darts (Magic Missile, Scorching Ray) or the creatures, and the d20. The server refuses with `invalid_argument` a choice that
 * does not fit the spell (a dart not placed, a creature that cannot be aimed at, too many targets), and the usual words ("confira os
 * campos") would send the player looking for a field. So the sentence points at what the window asks: the targets, and the darts or rays when the held spell splits them (its `plan`).
 */
export function readiedRefusalText(
  err: unknown,
  readied: ReadiedAction | undefined,
  doing: string,
  plan: ReleasePlan | null = null,
): string {
  if (
    readied?.kind === ReadiedKind.SPELL &&
    err instanceof ConnectError &&
    err.code === Code.InvalidArgument
  ) {
    return `Não deu para soltar a magia com essa escolha. Confira ${asked(plan)}. ${lossText(readied.slotLevel)}`;
  }
  return combatErrorMessage(err, doing);
}

/** What the window asks of a held spell: its targets, and the darts or rays when it splits them. */
function asked(plan: ReleasePlan | null): string {
  if (plan?.rule.kind !== 'darts') {
    return 'os alvos';
  }
  return plan.rule.rays ? 'os alvos e os raios' : 'os alvos e os dardos';
}

/** What passing costs: the spell, and the slot only when one was spent (a cantrip spent none). */
function lossText(slotLevel: number): string {
  return slotLevel > 0 ? 'Deixar passar perde a magia e o espaço.' : 'Deixar passar perde a magia.';
}
