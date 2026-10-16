import { ReactionChoice, type Encounter } from '../../../gen/meurpg/play/v1/combat_pb';
import { type RollMode, type RollModeRequest } from '../../../gen/meurpg/play/v1/combat_rolls_pb';
import {
  type AttackDie,
  type PairDie,
  type ModeChoice,
  type DamageChoice,
  type DamageDie,
  type AttackResult,
  type DamageResult,
  type ReactionAnswer,
  type AnswerResult,
  type AttackEffects,
  type ConcentrationAnswer,
  type ConcentrationOutcome,
} from './combat-client-types';
import { attackResult, need } from './combat-client-wire';
import { type CombatClientBase } from './combat-client-base';

/** The calls of attacks, damage, roll modes, reactions and concentration. */
export class CombatAttackCalls {
  constructor(private readonly base: CombatClientBase) {}

  /** The attack roll. `key` is made once per attack by the caller, so a retry
   * after a lost answer never rolls twice. */
  async rollAttack(
    campaignId: string,
    encounterId: string,
    attackerId: string,
    attackKey: string,
    targetId: string,
    die: AttackDie | PairDie,
    key: string,
    asReaction = false,
    opportunityOfferId = '',
    mode?: ModeChoice,
    catchWindowId = '',
    effects: AttackEffects = {},
  ): Promise<AttackResult> {
    const res = await this.base.client.rollAttack({
      campaignId,
      encounterId,
      attackerId,
      attackKey,
      targetId,
      idempotencyKey: key,
      roll:
        'inApp' in die
          ? { case: 'rollInApp', value: true }
          : 'face' in die
            ? { case: 'd20Face', value: die.face }
            : { case: undefined },
      d20Faces: 'faces' in die ? [...die.faces] : [],
      asReaction,
      opportunityOfferId,
      ...(mode
        ? {
            rollMode: mode.mode,
            modeReason: mode.reason,
            rollModeRequestId: mode.requestId ?? '',
          }
        : {}),
      catchWindowId,
      // The dice effects add to the roll (Bênção, Perdição) typed from a physical die, and the weapon attack of
      // the extra action an effect gives (Velocidade).
      extraDieFaces: [...(effects.extraDieFaces ?? [])],
      useExtraAction: effects.useExtraAction ?? false,
    });
    return attackResult(res);
  }

  async rollDamage(
    campaignId: string,
    encounterId: string,
    pendingDamageId: string,
    die: DamageDie | { readonly parts: true },
    key: string,
    choice?: DamageChoice,
  ): Promise<DamageResult> {
    const res = await this.base.client.rollDamage({
      campaignId,
      encounterId,
      pendingDamageId,
      idempotencyKey: key,
      roll:
        'inApp' in die
          ? { case: 'rollInApp', value: true }
          : 'sum' in die
            ? { case: 'typedSum', value: die.sum }
            : { case: undefined },
      ...(choice
        ? {
            selectedExtras: choice.extras.map((x) => ({
              key: x.key,
              slotLevel: x.slotLevel,
              pact: x.pact,
            })),
            extrasChosen: true,
            typedParts: (choice.typedParts ?? []).map((t) => ({ partKey: t.partKey, sum: t.sum })),
          }
        : {}),
    });
    return {
      encounter: need(res.encounter, 'RollDamage'),
      pending: need(res.pendingDamage, 'RollDamage'),
      cast: res.castPendingDamages,
    };
  }

  /** `amount` is the master's last word: the damage to apply instead of the
   * rolled one (0 to 9,999). `key` is made once per apply, so a retry never
   * applies twice. */
  async applyDamage(
    campaignId: string,
    encounterId: string,
    pendingDamageId: string,
    amount?: number,
    key?: string,
    ignoreModifiers: readonly string[] = [],
  ): Promise<DamageResult> {
    const res = await this.base.keyed(
      ['applyPendingDamage', campaignId, encounterId, pendingDamageId, amount, ignoreModifiers],
      (sent) =>
        this.base.client.applyPendingDamage({
          campaignId,
          encounterId,
          pendingDamageId,
          idempotencyKey: sent,
          amount,
          ignoreModifiers: [...ignoreModifiers],
        }),
      key,
    );
    return {
      encounter: need(res.encounter, 'ApplyPendingDamage'),
      pending: need(res.pendingDamage, 'ApplyPendingDamage'),
      cast: [],
    };
  }

  /** The player asks the master for a mode better than the suggestion (1 to 120 characters of reason). `key` is the caller's, kept across retries. */
  async requestRollMode(
    campaignId: string,
    encounterId: string,
    attackerId: string,
    attackKey: string,
    targetId: string,
    rollMode: RollMode,
    reason: string,
    key: string,
  ): Promise<{ readonly encounter: Encounter; readonly request: RollModeRequest }> {
    const res = await this.base.client.requestRollMode({
      campaignId,
      encounterId,
      attackerId,
      attackKey,
      targetId,
      rollMode,
      reason,
      idempotencyKey: key,
    });
    return {
      encounter: need(res.encounter, 'RequestRollMode'),
      request: need(res.request, 'RequestRollMode'),
    };
  }

  /** The master's answer: the mode the attack rolls with (the requested one, the suggested one, or disadvantage). */
  async answerRollModeRequest(
    campaignId: string,
    encounterId: string,
    requestId: string,
    decidedMode: RollMode,
    key: string,
  ): Promise<Encounter> {
    const res = await this.base.client.answerRollModeRequest({
      campaignId,
      encounterId,
      requestId,
      decidedMode,
      idempotencyKey: key,
    });
    return need(res.encounter, 'AnswerRollModeRequest');
  }

  async cancelRollModeRequest(
    campaignId: string,
    encounterId: string,
    requestId: string,
    key: string,
  ): Promise<Encounter> {
    const res = await this.base.client.cancelRollModeRequest({
      campaignId,
      encounterId,
      requestId,
      idempotencyKey: key,
    });
    return need(res.encounter, 'CancelRollModeRequest');
  }

  /** The master takes an extra out of a rolled damage, with the reason (1 to 120 characters). */
  async removeDamagePart(
    campaignId: string,
    encounterId: string,
    pendingDamageId: string,
    partKey: string,
    reason: string,
    key: string,
  ): Promise<DamageResult> {
    const res = await this.base.client.removeDamagePart({
      campaignId,
      encounterId,
      pendingDamageId,
      partKey,
      reason,
      idempotencyKey: key,
    });
    return {
      encounter: need(res.encounter, 'RemoveDamagePart'),
      pending: need(res.pendingDamage, 'RemoveDamagePart'),
      cast: [],
    };
  }

  async discardDamage(
    campaignId: string,
    encounterId: string,
    pendingDamageId: string,
  ): Promise<DamageResult> {
    const res = await this.base.keyed(
      ['discardPendingDamage', campaignId, encounterId, pendingDamageId],
      (sent) =>
        this.base.client.discardPendingDamage({
          campaignId,
          encounterId,
          pendingDamageId,
          idempotencyKey: sent,
        }),
    );
    return {
      encounter: need(res.encounter, 'DiscardPendingDamage'),
      pending: need(res.pendingDamage, 'DiscardPendingDamage'),
      cast: [],
    };
  }

  /** `AnswerReaction`: "Usar ..." or "Deixar passar" on a reaction window. The caller makes the key from the request. */
  async answerReaction(
    campaignId: string,
    encounterId: string,
    windowId: string,
    answer: ReactionAnswer,
    key: string,
  ): Promise<AnswerResult> {
    const res = await this.base.client.answerReaction({
      campaignId,
      encounterId,
      windowId,
      answer: answer.use ? ReactionChoice.USE : ReactionChoice.PASS,
      slot: answer.slot ? { level: answer.slot.level, pact: answer.slot.pact } : undefined,
      useRacial: answer.useRacial ?? false,
      creatureIds: [...(answer.creatureIds ?? [])],
      maneuverKey: answer.maneuverKey ?? '',
      roll: !answer.die
        ? { case: undefined }
        : 'inApp' in answer.die
          ? { case: 'rollInApp', value: true }
          : { case: 'typed', value: answer.die.typed },
      idempotencyKey: key,
    });
    return { encounter: need(res.encounter, 'AnswerReaction'), result: res.result };
  }

  /**
   * `MarkReactionRolling`: the player says they are using an optional reaction and rolling now, so the master's
   * clock waits longer before it passes the window. Idempotent, so it needs no key.
   */
  async markReactionRolling(
    campaignId: string,
    encounterId: string,
    windowId: string,
  ): Promise<Encounter> {
    const res = await this.base.client.markReactionRolling({ campaignId, encounterId, windowId });
    return need(res.encounter, 'MarkReactionRolling');
  }

  /** `ResolveConcentrationSave`: the Constitution saving throw of a concentration window. */
  async resolveConcentrationSave(
    campaignId: string,
    encounterId: string,
    windowId: string,
    how: ConcentrationAnswer,
    key: string,
  ): Promise<ConcentrationOutcome> {
    const res = await this.base.client.resolveConcentrationSave({
      campaignId,
      encounterId,
      windowId,
      roll:
        how.kind === 'app'
          ? { case: 'rollInApp', value: true }
          : how.kind === 'typed'
            ? { case: 'd20Face', value: how.face }
            : how.kind === 'hand'
              ? { case: 'handToMaster', value: true }
              : { case: 'keep', value: true },
      extraDieFaces: how.kind === 'typed' ? [...(how.extra ?? [])] : [],
      ...(how.kind === 'typed' && how.second !== undefined ? { secondD20Face: how.second } : {}),
      idempotencyKey: key,
    });
    return {
      encounter: need(res.encounter, 'ResolveConcentrationSave'),
      result: res.result,
      ...(res.inspirationOffer ? { offer: res.inspirationOffer } : {}),
      ...(res.luckyOffer ? { lucky: res.luckyOffer } : {}),
    };
  }
}
