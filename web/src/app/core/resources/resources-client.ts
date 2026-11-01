import { Injectable, inject } from '@angular/core';
import { createClient } from '@connectrpc/connect';

import type { CharacterVitals } from '../../../gen/meurpg/play/v1/play_pb';
import {
  type AnswerHeldRollResponse,
  type ConvertSpellSlotResponse,
  type CreateSpellSlotResponse,
  type GetOutsideInspirationResponse,
  type GiveBardicInspirationOutsideResponse,
  type GiveBardicInspirationResponse,
  LayOnHandsCure,
  type UseLayOnHandsResponse,
  type GetRestPreviewResponse,
  type ListHeldRollsResponse,
  type RestKind,
  ResourceService,
  type SpendHitDiceResponse,
  type UseArcaneRecoveryResponse,
  type UseNaturalRecoveryResponse,
} from '../../../gen/meurpg/play/v1/resources_pb';
import { CONNECT_TRANSPORT } from '../connect/transport';
import type { HeldAnswer, HeldRolled } from '../held-rolls/held-roll-offer';

/** How the die of a spent hit die comes (RN-18): the app rolls it, or the player typed the face of a real die. */
export type HitDieRoll =
  { readonly inApp: true } | { readonly face: number; readonly songFace?: number };

/** What a Lay on Hands touch does: restore `amount` hit points (1 to what the pool has left), or cure one disease or
 * neutralize one poison (5 points). */
export type LayOnHandsEffect =
  { readonly amount: number } | { readonly cure: 'disease' | 'poison' };

/** The oneof of the request for each question: Lucky's, the die's, Indomitable's and Stroke of Luck's. */
function answerOf(answer: HeldAnswer) {
  switch (answer.kind) {
    case 'lucky':
      return {
        case: 'lucky',
        value: { reroll: answer.reroll, roll: rollOf(answer.reroll, answer.roll) },
      } as const;
    case 'indomitable':
      return {
        case: 'indomitable',
        value: {
          reroll: answer.reroll,
          roll:
            answer.roll && 'faces' in answer.roll
              ? { case: undefined }
              : rollOf(answer.reroll, answer.roll),
          typedFaces:
            answer.reroll && answer.roll && 'faces' in answer.roll ? [...answer.roll.faces] : [],
        },
      } as const;
    case 'stroke':
      return { case: 'strokeOfLuck', value: { use: answer.use } } as const;
    case 'die':
      return {
        case: 'die',
        value: { use: answer.use, roll: rollOf(answer.use, answer.roll) },
      } as const;
  }
}

/** The roll of an answer that says how it comes: the app's or the typed face; nothing when the die is kept. */
function rollOf(
  rolled: boolean,
  roll: HeldRolled | null,
):
  | { readonly case: 'rollInApp'; readonly value: true }
  | { readonly case: 'typedFace'; readonly value: number }
  | { readonly case: undefined } {
  if (!rolled || !roll) {
    return { case: undefined };
  }
  if ('inApp' in roll) {
    return { case: 'rollInApp', value: true };
  }
  return 'face' in roll ? { case: 'typedFace', value: roll.face } : { case: undefined };
}

/** How many dice of each size come back to one character on a long rest (`HitDiceChoice`). */
export interface HitDiceChoiceSpec {
  readonly characterId: string;
  readonly dice: readonly { readonly faces: number; readonly count: number }[];
}

/**
 * Thin wrapper around the generated `ResourceService` client (the rests and the hit dice), in the shape of
 * `CombatClient`. Only lazy code imports it, so the generated code stays out of the initial bundle. Every write takes
 * the idempotency key its caller made once for that request (`ActionKey`); callers word the errors with
 * `resources-errors.ts`.
 */
@Injectable({ providedIn: 'root' })
export class ResourceClient {
  private readonly client = createClient(ResourceService, inject(CONNECT_TRANSPORT));

  /** What a rest would give back to each living player character; nothing is written. Master only. */
  restPreview(campaignId: string, kind: RestKind): Promise<GetRestPreviewResponse> {
    return this.client.getRestPreview({ campaignId, kind });
  }

  /** The master's rest. Answers the new vitals of every character it touched. */
  async takeRest(
    campaignId: string,
    kind: RestKind,
    idempotencyKey: string,
    hitDiceChoices: readonly HitDiceChoiceSpec[] = [],
    withoutFoodOrDrink = false,
  ): Promise<CharacterVitals[]> {
    const res = await this.client.takeRest({
      campaignId,
      kind,
      idempotencyKey,
      withoutFoodOrDrink,
      hitDiceChoices: hitDiceChoices.map((c) => ({
        characterId: c.characterId,
        dice: c.dice.map((d) => ({ faces: d.faces, count: d.count })),
      })),
    });
    return res.vitals;
  }

  /** One hit die spent on a short rest: the face, the Constitution modifier and the hit points regained. */
  spendHitDice(
    campaignId: string,
    characterId: string,
    faces: number,
    roll: HitDieRoll,
    idempotencyKey: string,
  ): Promise<SpendHitDiceResponse> {
    return this.client.spendHitDice({
      campaignId,
      characterId,
      faces,
      idempotencyKey,
      roll:
        'inApp' in roll
          ? { case: 'rollInApp', value: true }
          : { case: 'typedFace', value: roll.face },
      typedSongFace: 'inApp' in roll ? 0 : (roll.songFace ?? 0),
    });
  }

  /** Recuperação Arcana: the expended slots to recover, as spell level and count. */
  useArcaneRecovery(
    campaignId: string,
    characterId: string,
    slots: readonly { readonly level: number; readonly count: number }[],
    idempotencyKey: string,
  ): Promise<UseArcaneRecoveryResponse> {
    return this.client.useArcaneRecovery({
      campaignId,
      characterId,
      idempotencyKey,
      slots: slots.map((s) => ({ level: s.level, count: s.count })),
    });
  }

  /** Recuperação Natural (druid, Circle of the Land): the same request as Recuperação Arcana. */
  useNaturalRecovery(
    campaignId: string,
    characterId: string,
    slots: readonly { readonly level: number; readonly count: number }[],
    idempotencyKey: string,
  ): Promise<UseNaturalRecoveryResponse> {
    return this.client.useNaturalRecovery({
      campaignId,
      characterId,
      idempotencyKey,
      slots: slots.map((s) => ({ level: s.level, count: s.count })),
    });
  }

  /** Lay on Hands: the touch, as an action of the paladin's turn. The answer never says why a touch did nothing. */
  useLayOnHands(
    campaignId: string,
    encounterId: string,
    actorId: string,
    targetId: string,
    effect: LayOnHandsEffect,
    idempotencyKey: string,
  ): Promise<UseLayOnHandsResponse> {
    return this.client.useLayOnHands({
      campaignId,
      encounterId,
      actorId,
      targetId,
      idempotencyKey,
      effect:
        'amount' in effect
          ? { case: 'amount', value: effect.amount }
          : {
              case: 'cure',
              value: effect.cure === 'poison' ? LayOnHandsCure.POISON : LayOnHandsCure.DISEASE,
            },
    });
  }

  /** Flexible Casting, points to slot: a slot of level 1 to 5 as a bonus action. */
  createSpellSlot(
    campaignId: string,
    encounterId: string,
    actorId: string,
    slotLevel: number,
    idempotencyKey: string,
  ): Promise<CreateSpellSlotResponse> {
    return this.client.createSpellSlot({
      campaignId,
      encounterId,
      actorId,
      slotLevel,
      idempotencyKey,
    });
  }

  /** Flexible Casting, slot to points: a free slot is expended for as many points as its level. */
  convertSpellSlot(
    campaignId: string,
    encounterId: string,
    actorId: string,
    slotLevel: number,
    idempotencyKey: string,
  ): Promise<ConvertSpellSlotResponse> {
    return this.client.convertSpellSlot({
      campaignId,
      encounterId,
      actorId,
      slotLevel,
      idempotencyKey,
    });
  }

  /** Bardic Inspiration: the die to a creature, as a bonus action. */
  giveBardicInspiration(
    campaignId: string,
    encounterId: string,
    actorId: string,
    targetId: string,
    idempotencyKey: string,
  ): Promise<GiveBardicInspirationResponse> {
    return this.client.giveBardicInspiration({
      campaignId,
      encounterId,
      actorId,
      targetId,
      idempotencyKey,
    });
  }

  /** The Bardic Inspiration of the caller outside a combat: the die their character holds, the party a bard can give it
   * to, and the rolls that wait for an answer about it. The master reads every die held. */
  outsideInspiration(campaignId: string): Promise<GetOutsideInspirationResponse> {
    return this.client.getOutsideInspiration({ campaignId });
  }

  /** The bard gives the die to another character of the party, outside a combat (10 minutes of game time). */
  giveBardicInspirationOutside(
    campaignId: string,
    targetCharacterId: string,
    idempotencyKey: string,
  ): Promise<GiveBardicInspirationOutsideResponse> {
    return this.client.giveBardicInspirationOutside({
      campaignId,
      targetCharacterId,
      idempotencyKey,
    });
  }

  /** The answer to the question of any roll that waits (`AnswerHeldRoll`), whichever roll it is and whichever question it
   * asks: the halfling's Lucky about a 1, or the Bardic Inspiration die. Answers the held path's own response (`result`)
   * and the roll's next question (`next`); the roller's player and the master both may answer. */
  answerHeldRoll(
    campaignId: string,
    holdId: string,
    answer: HeldAnswer,
    idempotencyKey: string,
  ): Promise<AnswerHeldRollResponse> {
    return this.client.answerHeldRoll({
      campaignId,
      holdId,
      idempotencyKey,
      answer: answerOf(answer),
    });
  }

  /** The questions of the rolls that wait and the caller may answer, oldest first: a player's own, the master's every
   * one of the table. Read it again when the stream says the questions changed. */
  listHeldRolls(campaignId: string): Promise<ListHeldRollsResponse> {
    return this.client.listHeldRolls({ campaignId });
  }
}
