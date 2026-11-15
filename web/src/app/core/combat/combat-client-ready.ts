import { inject } from '@angular/core';
import { createClient } from '@connectrpc/connect';

import type { Encounter, ReadiedKind } from '../../../gen/meurpg/play/v1/combat_pb';
import { ReadyService } from '../../../gen/meurpg/play/v1/ready_pb';
import { CONNECT_TRANSPORT } from '../connect/transport';
import { need } from './combat-client-wire';

/** What the player picked to send with `ReadyAction` (#582). */
export interface ReadyArgs {
  readonly campaignId: string;
  readonly encounterId: string;
  readonly combatantId: string;
  /** The trigger in the player's own words, trimmed: 1 to 200 characters. */
  readonly triggerText: string;
  readonly kind: ReadiedKind;
  /** ATTACK and OTHER: the key of the attack or the standard action; empty otherwise. */
  readonly actionKey: string;
  /** SPELL: the spell and the slot level (0 for a cantrip); empty and 0 otherwise. */
  readonly spellKey: string;
  readonly slotLevel: number;
  /** The creature the trigger names, or empty. */
  readonly targetCombatantId: string;
}

/** The calls of Ready (`ReadyService`): ready an action, fire it (the master) and give it up (the readier's turn). */
export class CombatReadyCalls {
  readonly client = createClient(ReadyService, inject(CONNECT_TRANSPORT));

  /** `ReadyAction`, under the caller's idempotency key: a second tap or a try after a lost answer sends the same key. */
  async readyAction(args: ReadyArgs, key: string): Promise<Encounter> {
    const res = await this.client.readyAction({ ...args, idempotencyKey: key });
    return need(res.encounter, 'ReadyAction');
  }

  /** `FireReadied`: the master says the trigger happened; the readier's READIED window opens. */
  async fireReadied(
    campaignId: string,
    encounterId: string,
    readiedId: string,
    key: string,
  ): Promise<Encounter> {
    const res = await this.client.fireReadied({
      campaignId,
      encounterId,
      readiedId,
      idempotencyKey: key,
    });
    return need(res.encounter, 'FireReadied');
  }

  /** `CancelReadied`: the readier's player, or the master, gives the held action up on the readier's own turn. */
  async cancelReadied(
    campaignId: string,
    encounterId: string,
    readiedId: string,
    key: string,
  ): Promise<Encounter> {
    const res = await this.client.cancelReadied({
      campaignId,
      encounterId,
      readiedId,
      idempotencyKey: key,
    });
    return need(res.encounter, 'CancelReadied');
  }
}
