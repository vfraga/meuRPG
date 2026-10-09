import { Injectable, inject } from '@angular/core';
import { createClient } from '@connectrpc/connect';

import type { Encounter } from '../../../gen/meurpg/play/v1/combat_pb';
import {
  type CreatureActionResult,
  type CreatureTurn,
  CreatureService,
} from '../../../gen/meurpg/play/v1/creatures_pb';
import { newKey } from '../connect/idempotency';
import { CONNECT_TRANSPORT } from '../connect/transport';

/** What using an action answers: the combat and what the action did. */
export interface ActionDone {
  readonly encounter: Encounter;
  readonly result: CreatureActionResult;
}

/**
 * The master's calls for a monster's turn (W7-M, `CreatureService`): its stat block and actions,
 * an action used whole, a legendary action, the offer let pass and the answer to a Legendary
 * Resistance prompt. Each change takes the key the caller keeps while the call waits for its
 * answer, so a retry after a lost answer is the same change (the server answers with what the
 * first call did).
 */
@Injectable({ providedIn: 'root' })
export class CreatureClient {
  private readonly client = createClient(CreatureService, inject(CONNECT_TRANSPORT));

  async turn(campaignId: string, encounterId: string, combatantId: string): Promise<CreatureTurn> {
    const res = await this.client.getCreatureTurn({ campaignId, encounterId, combatantId });
    if (!res.turn) {
      throw new Error('GetCreatureTurn answered without its turn');
    }
    return res.turn;
  }

  async useAction(
    campaignId: string,
    encounterId: string,
    combatantId: string,
    actionKey: string,
    targetIds: readonly string[],
    key: string,
    routine = 0,
  ): Promise<ActionDone> {
    const res = await this.client.useCreatureAction({
      campaignId,
      encounterId,
      combatantId,
      actionKey,
      targetIds: [...targetIds],
      routine,
      idempotencyKey: key,
    });
    return done(res.encounter, res.result, 'UseCreatureAction');
  }

  async useLegendary(
    campaignId: string,
    encounterId: string,
    combatantId: string,
    optionKey: string,
    targetIds: readonly string[],
    key: string,
  ): Promise<ActionDone> {
    const res = await this.client.useLegendaryAction({
      campaignId,
      encounterId,
      combatantId,
      optionKey,
      targetIds: [...targetIds],
      idempotencyKey: key,
    });
    return done(res.encounter, res.result, 'UseLegendaryAction');
  }

  async declineLegendary(
    campaignId: string,
    encounterId: string,
    combatantId: string,
    key: string,
  ): Promise<Encounter> {
    const res = await this.client.declineLegendary({
      campaignId,
      encounterId,
      combatantId,
      idempotencyKey: key,
    });
    if (!res.encounter) {
      throw new Error('DeclineLegendary answered without its combat');
    }
    return res.encounter;
  }

  async answerResistance(
    campaignId: string,
    encounterId: string,
    combatantId: string,
    castId: string,
    use: boolean,
    key: string,
  ): Promise<Encounter> {
    const res = await this.client.answerLegendaryResistance({
      campaignId,
      encounterId,
      combatantId,
      castId,
      use,
      idempotencyKey: key,
    });
    if (!res.encounter) {
      throw new Error('AnswerLegendaryResistance answered without its combat');
    }
    return res.encounter;
  }
}

function done(
  encounter: Encounter | undefined,
  result: CreatureActionResult | undefined,
  call: string,
): ActionDone {
  if (!encounter || !result) {
    throw new Error(`${call} answered without its result`);
  }
  return { encounter, result };
}

export { newKey };
