import { Injectable, inject } from '@angular/core';
import { createClient } from '@connectrpc/connect';

import type { CharacterVitals } from '../../../gen/meurpg/play/v1/play_pb';
import {
  type AnswerOutsideInspirationResponse,
  type AnswerOutsideLuckyResponse,
  type ConvertSpellSlotResponse,
  type CreateSpellSlotResponse,
  type GetOutsideInspirationResponse,
  type GiveBardicInspirationOutsideResponse,
  type GiveBardicInspirationResponse,
  LayOnHandsCure,
  type UseLayOnHandsResponse,
  type GetRestPreviewResponse,
  type RestKind,
  ResourceService,
  type SpendHitDiceResponse,
  type UseArcaneRecoveryResponse,
} from '../../../gen/meurpg/play/v1/resources_pb';
import { type AttackResult } from '../combat/combat-client-types';
import { attackResult } from '../combat/combat-client-wire';
import { CONNECT_TRANSPORT } from '../connect/transport';

/** How the die of a spent hit die comes (RN-18): the app rolls it, or the player typed the face of a real die. */
export type HitDieRoll =
  { readonly inApp: true } | { readonly face: number; readonly songFace?: number };

/** What a Lay on Hands touch does: restore `amount` hit points (1 to what the pool has left), or cure one disease or
 * neutralize one poison (5 points). */
export type LayOnHandsEffect =
  { readonly amount: number } | { readonly cure: 'disease' | 'poison' };

/** How the die of a Bardic Inspiration comes when the player uses it: the app rolls it, or the typed face. */
export type InspirationRoll = HitDieRoll;

/** How the new d20 of the halfling's Lucky comes when the player rolls it again: the app rolls it, or the typed face. */
export type LuckyRoll = { readonly inApp: true } | { readonly face: number };

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

  /** The answer to a held attack roll: use the die (rolled in the app, or its typed face) or keep it. Resolves the
   * attack as `RollAttack` would have. `roll` is only for `use`. */
  async answerBardicInspiration(
    campaignId: string,
    encounterId: string,
    holdId: string,
    use: boolean,
    roll: InspirationRoll | null,
    idempotencyKey: string,
  ): Promise<AttackResult> {
    const res = await this.client.answerBardicInspiration({
      campaignId,
      encounterId,
      holdId,
      use,
      idempotencyKey,
      roll:
        use && roll
          ? 'inApp' in roll
            ? { case: 'rollInApp', value: true }
            : { case: 'typedFace', value: roll.face }
          : { case: undefined },
    });
    if (!res.attack) {
      throw new Error('AnswerBardicInspiration answered without its attack');
    }
    return attackResult(res.attack);
  }

  /** The answer to the Lucky question of a held attack roll ("Tirou 1: rolar de novo?"): roll the d20 that showed the 1
   * again (in the app, or its typed face) or keep the 1. Answers the attack as `RollAttack` would have: resolved, or, when
   * the attacker holds a Bardic Inspiration die, still held for the die's question on the new roll. `roll` is only for
   * `reroll`. */
  async answerLucky(
    campaignId: string,
    encounterId: string,
    holdId: string,
    reroll: boolean,
    roll: LuckyRoll | null,
    idempotencyKey: string,
  ): Promise<AttackResult> {
    const res = await this.client.answerLucky({
      campaignId,
      encounterId,
      holdId,
      reroll,
      idempotencyKey,
      roll:
        reroll && roll
          ? 'inApp' in roll
            ? { case: 'rollInApp', value: true }
            : { case: 'typedFace', value: roll.face }
          : { case: undefined },
    });
    if (!res.attack) {
      throw new Error('AnswerLucky answered without its attack');
    }
    return attackResult(res.attack);
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

  /** The answer to a roll outside a combat that waits for the die: use it (rolled in the app, or its typed face) or keep
   * it. The roll is then written as the scene check or the group check would have written it. */
  answerOutsideInspiration(
    campaignId: string,
    holdId: string,
    use: boolean,
    roll: InspirationRoll | null,
    idempotencyKey: string,
  ): Promise<AnswerOutsideInspirationResponse> {
    return this.client.answerOutsideInspiration({
      campaignId,
      holdId,
      use,
      idempotencyKey,
      roll:
        use && roll
          ? 'inApp' in roll
            ? { case: 'rollInApp', value: true }
            : { case: 'typedFace', value: roll.face }
          : { case: undefined },
    });
  }

  /** The answer to the Lucky question of a held roll that is not a weapon attack (a scene check, a group check, a trap
   * search, a puzzle hint try, a spell attack, an effect's saving throw or a concentration save): roll the d20 that
   * showed the 1 again (in the app, or its typed face) or keep the 1. Answers the held path's own response, in the same
   * oneof as `answerOutsideInspiration`: written, with the d20 rolled again in its `rerolled`; or, when the character
   * holds a Bardic Inspiration die, still held, with the die's question on the new roll. `roll` is only for `reroll`. */
  answerOutsideLucky(
    campaignId: string,
    holdId: string,
    reroll: boolean,
    roll: LuckyRoll | null,
    idempotencyKey: string,
  ): Promise<AnswerOutsideLuckyResponse> {
    return this.client.answerOutsideLucky({
      campaignId,
      holdId,
      reroll,
      idempotencyKey,
      roll:
        reroll && roll
          ? 'inApp' in roll
            ? { case: 'rollInApp', value: true }
            : { case: 'typedFace', value: roll.face }
          : { case: undefined },
    });
  }
}
