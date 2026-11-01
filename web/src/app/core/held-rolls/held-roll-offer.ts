import { create } from '@bufbuild/protobuf';

import {
  type DiceRoll,
  type Encounter,
  type HeldRollOffer,
  HeldRollOfferSchema,
  HeldRollKind,
  type InspirationOffer,
  type LuckyOffer,
  type OutsideInspirationOffer,
} from '../../../gen/meurpg/play/v1/combat_pb';
import type { Reroll } from '../../../gen/meurpg/play/v1/combat_rolls_pb';
import { rollText } from '../combat/combat-dice';
import {
  INDOMITABLE_FEATURE,
  LUCKY_TRAIT,
  STROKE_FEATURE,
  type WithRerolls,
} from '../combat/lucky';

/**
 * The question a roll that waits asks, in the one shape every held roll has (`HeldRollOffer`): the halfling's Lucky
 * ("Tirou 1: rolar de novo?") first, then the Bardic Inspiration die. The server sends it as `held_roll_offer` beside the
 * older `lucky_offer` and `inspiration_offer`; `heldOfferOf` reads the new one and falls back on the old ones, so a
 * response that carries only those still asks.
 */

/** How the new d20 of Lucky, or the die of the Bardic Inspiration, comes (RN-18): the app rolls it, or the player typed
 * the face of a real die. */
export type HeldRolled =
  | { readonly inApp: true }
  | { readonly face: number }
  /** The two d20 of a save with advantage or disadvantage, typed (Indomitable's new roll takes both). */
  | { readonly faces: readonly number[] };

/** The player's answer to the question that waits: Lucky's (roll the d20 that showed the 1 again, or keep it), the
 * die's (add it, or keep it for later) or the fighter's Indomitable (roll the failed saving throw again, or keep it).
 * `roll` is only for `reroll` and `use`. */
export type HeldAnswer =
  | { readonly kind: 'lucky'; readonly reroll: boolean; readonly roll: HeldRolled | null }
  | { readonly kind: 'die'; readonly use: boolean; readonly roll: HeldRolled | null }
  | { readonly kind: 'indomitable'; readonly reroll: boolean; readonly roll: HeldRolled | null }
  /** The rogue's Stroke of Luck: use it (the miss is a hit, the d20 of the check counts as 20) or keep the roll. Nothing is
   * rolled. */
  | { readonly kind: 'stroke'; readonly use: boolean };

/** The ways a response carries a question: the new offer, or the two older ones. */
export interface HeldOfferSource {
  readonly heldRollOffer?: HeldRollOffer;
  readonly luckyOffer?: LuckyOffer;
  readonly inspirationOffer?: InspirationOffer | OutsideInspirationOffer;
}

/** What a roll that answered a question instead of its result throws, for the clients whose call returns the result
 * alone (a cast): nothing was spent, the d20 is kept on the server, and the sheet asks. */
export class HeldRollAsked extends Error {
  constructor(
    readonly offer: HeldRollOffer,
    readonly encounter: Encounter,
  ) {
    super('The roll waits for the answer to its question');
    this.name = 'HeldRollAsked';
  }
}

/** The question a response asks now: the new offer, else the old Lucky one (it comes first), else the old die one;
 * `undefined` when the roll does not wait. */
export function heldOfferOf(source: HeldOfferSource): HeldRollOffer | undefined {
  if (source.heldRollOffer) {
    return source.heldRollOffer;
  }
  const lucky = source.luckyOffer;
  if (lucky) {
    return create(HeldRollOfferSchema, {
      holdId: lucky.holdId,
      d20: lucky.d20,
      question: { case: 'lucky', value: { luckyIndex: lucky.luckyIndex } },
      ...attackOf(lucky),
    });
  }
  const die = source.inspirationOffer;
  if (die) {
    const combat = isCombatDie(die);
    return create(HeldRollOfferSchema, {
      holdId: die.holdId,
      d20: die.d20,
      question: {
        case: 'die',
        value: combat
          ? { sides: die.die?.sides ?? 0, fromName: die.die?.fromLabel ?? '' }
          : { sides: die.sides, fromName: die.fromName },
      },
      ...(combat ? attackOf(die) : {}),
    });
  }
  return undefined;
}

/** The die's offer of a weapon attack in a combat (it names the die as a message), not the one of a roll outside it. */
function isCombatDie(offer: InspirationOffer | OutsideInspirationOffer): offer is InspirationOffer {
  return 'die' in offer;
}

/** The weapon attack an old offer names (empty for a roll that is not one). */
function attackOf(offer: { readonly attackKey: string; readonly targetId: string }) {
  return offer.attackKey
    ? { kind: HeldRollKind.WEAPON_ATTACK, attackKey: offer.attackKey, targetId: offer.targetId }
    : {};
}

/** The question when it is a weapon attack's (the combat view reopens the attack sheet at it), else `null`. */
export function weaponHold(offer: HeldRollOffer | null | undefined): HeldRollOffer | null {
  return offer && (offer.kind === HeldRollKind.WEAPON_ATTACK || offer.attackKey !== '')
    ? offer
    : null;
}

/** Whether the question is the halfling's Lucky (otherwise it is the die's). */
export function asksLucky(offer: HeldRollOffer | null | undefined): boolean {
  return offer?.question.case === 'lucky';
}

/** Whether the question is the fighter's Indomitable, asked after the result of a saving throw. */
export function asksIndomitable(offer: HeldRollOffer | null | undefined): boolean {
  return offer?.question.case === 'indomitable';
}

/** Whether the saving throw failed, when the question may say so; `false` when the table hides the result from the
 * roller until the answer (the question then comes on every save). */
export function indomitableFailed(offer: HeldRollOffer | null | undefined): boolean {
  return offer?.question.case === 'indomitable' && offer.question.value.failed;
}

/** Whether the question is the rogue's Stroke of Luck, asked after the result of an attack or of an ability check. */
export function asksStroke(offer: HeldRollOffer | null | undefined): boolean {
  return offer?.question.case === 'strokeOfLuck';
}

/** Whether the attack missed or the check failed, when the question may say so; `false` when the table hides the result
 * from the roller until the answer (the question then comes on every roll). */
export function strokeFailed(offer: HeldRollOffer | null | undefined): boolean {
  return offer?.question.case === 'strokeOfLuck' && offer.question.value.failed;
}

/** The size and giver of the die the question offers, or `undefined` when it is Lucky's, Indomitable's or Stroke of Luck's. */
export function dieOf(
  offer: HeldRollOffer | null | undefined,
): { readonly sides: number; readonly fromName: string } | undefined {
  return offer?.question.case === 'die' ? offer.question.value : undefined;
}

/** "Seu d20: 1d20 (1) + 5 = 6. O resultado só aparece depois da sua escolha.": the held d20 above the question, on a
 * sheet that has no result to show it in yet. `''` without a d20. */
export function luckyHeldLine(offer: { readonly d20?: DiceRoll } | null | undefined): string {
  return offer?.d20
    ? `Seu d20: ${rollText(offer.d20)}. O resultado só aparece depois da sua escolha.`
    : '';
}

/** The saving throw Indomitable rolled again on a roll (the faces already hold the new one); `undefined` when it rolled
 * nothing. */
export function indomitableReroll(roll: WithRerolls | undefined): Reroll | undefined {
  return roll?.rerolled.find((r) => r.sourceKey === INDOMITABLE_FEATURE);
}

/** What Stroke of Luck did to a roll; `undefined` when it did nothing. */
export function strokeReroll(roll: WithRerolls | undefined): Reroll | undefined {
  return roll?.rerolled.find((r) => r.sourceKey === STROKE_FEATURE);
}

/** The d20 Lucky rolled again on a roll (the faces already hold the new one), for the die that tumbles after an answer
 * rolled in the app; `undefined` when it rolled nothing. */
export function luckyReroll(roll: WithRerolls | undefined): Reroll | undefined {
  return roll?.rerolled.find((r) => r.sourceKey === LUCKY_TRAIT);
}
