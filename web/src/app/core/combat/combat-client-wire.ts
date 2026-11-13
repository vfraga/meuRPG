import { type MessageInitShape, create } from '@bufbuild/protobuf';
import {
  MonsterHitPoints,
  type ParticipantSchema,
  type RollAttackResponse,
  AttackRollSchema,
} from '../../../gen/meurpg/play/v1/combat_pb';
import {
  type JoinSpec,
  type MonsterHp,
  type AttackDie,
  type PairDie,
  type PoolDie,
  type AttackResult,
  type AreaChoice,
  type CastExtras,
} from './combat-client-types';
import { wallBody } from './spell-wall';
import { heldOfferOf } from '../held-rolls/held-roll-offer';

/** The optional fields of a cast request; each is left out unless it was set. */
export function castExtrasBody(extras: CastExtras) {
  return {
    ...(extras.wall ? { wall: wallBody(extras.wall) } : {}),
    // Left out unless the master chose: the table rule decides then.
    ...(extras.revealHidden === undefined ? {} : { revealHidden: extras.revealHidden }),
    ...(extras.abilityKey ? { abilityKey: extras.abilityKey } : {}),
    ...(extras.sculptedIds?.length ? { sculptedIds: [...extras.sculptedIds] } : {}),
    ...(extras.destination ? { destination: { ...extras.destination } } : {}),
    ...(extras.spellOptions?.length
      ? { spellOptions: extras.spellOptions.map((o) => ({ ...o })) }
      : {}),
    ...(extras.grantedBy ? { grantedBy: extras.grantedBy } : {}),
  };
}

/** The roll of a cast as the request's oneof: the app rolls, a typed pool sum, or a typed d20. */
export function castRollOneof(die: AttackDie | PoolDie | PairDie | null) {
  if (!die) {
    return { case: undefined };
  }
  if ('inApp' in die) {
    return { case: 'rollInApp' as const, value: true };
  }
  if ('poolSum' in die) {
    return { case: 'poolSum' as const, value: die.poolSum };
  }
  if ('face' in die) {
    return { case: 'd20Face' as const, value: die.face };
  }
  return { case: undefined };
}

export function areaOneof(area: AreaChoice | null | undefined) {
  if (!area) {
    return { case: undefined };
  }
  return 'origin' in area
    ? { case: 'origin' as const, value: { col: area.origin.col, row: area.origin.row } }
    : { case: 'direction' as const, value: { dx: area.direction.dx, dy: area.direction.dy } };
}

export function toHitPoints(hp: MonsterHp | undefined): MonsterHitPoints {
  return hp === 'rolled' ? MonsterHitPoints.ROLLED : MonsterHitPoints.AVERAGE;
}

export function toParticipant(spec: JoinSpec): MessageInitShape<typeof ParticipantSchema> {
  return { characterId: spec.characterId, count: spec.count ?? 1, hidden: spec.hidden };
}

/** The answer of `RollAttack`, and the attack of `AnswerHeldRoll`: a held roll has the d20 in its question and no
 * outcome yet. */
export function attackResult(res: RollAttackResponse): AttackResult {
  const held = heldOfferOf(res);
  return {
    encounter: need(res.encounter, 'RollAttack'),
    roll: res.roll ?? create(AttackRollSchema, { d20: need(held?.d20, 'RollAttack') }),
    pending: res.pendingDamage,
    held,
  };
}

export function need<T>(value: T | undefined, call: string): T {
  if (value === undefined) {
    throw new Error(`${call} answered without its result`);
  }
  return value;
}
