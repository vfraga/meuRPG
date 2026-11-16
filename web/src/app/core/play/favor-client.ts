import { Injectable, inject } from '@angular/core';
import { createClient } from '@connectrpc/connect';

import { RollMode } from '../../../gen/meurpg/play/v1/combat_rolls_pb';
import {
  type FavorPass,
  FavorPassStatus,
  FavorRefundStatus,
  FavorService,
} from '../../../gen/meurpg/play/v1/favor_pb';
import { CONNECT_TRANSPORT } from '../connect/transport';

/** One character that has the master's Inspiração. */
export interface FavorHolderVm {
  readonly characterId: string;
  readonly name: string;
}

/** A pass that waits for the master (RN-43): the giver asked to give the Inspiração to the receiver. */
export interface FavorPassVm {
  readonly id: string;
  readonly fromId: string;
  readonly fromName: string;
  readonly toId: string;
  readonly toName: string;
  /** The master's alone: the receiver has one already, so "Aceitar" is dashed with the reason. Never true for a player. */
  readonly receiverHas: boolean;
}

/** A source of advantage the roller could not read, as the master reads it: the sentence the server wrote, cut at its colon. */
export interface HiddenSourceVm {
  /** "Alvo Impedido" from "Alvo Impedido: vantagem". */
  readonly name: string;
  /** The source gives advantage (else disadvantage). */
  readonly advantage: boolean;
}

/** The master's question about an Inspiração a roll spent for nothing. The master's alone. */
export interface FavorRefundVm {
  readonly id: string;
  readonly characterId: string;
  readonly characterName: string;
  /** The roll's name as the server says it ("Ataque"). */
  readonly rollLabel: string;
  readonly hidden: readonly HiddenSourceVm[];
}

/** What `GetFavor` answers, as the reader may read it. */
export interface FavorReadVm {
  readonly holders: readonly FavorHolderVm[];
  readonly passes: readonly FavorPassVm[];
  readonly refunds: readonly FavorRefundVm[];
}

/** "Alvo Impedido" from "Alvo Impedido: vantagem": the part of a sentence before its colon names the source. */
export function sourceName(text: string): string {
  return text.split(':')[0].trim();
}

function passOf(p: FavorPass): FavorPassVm {
  return {
    id: p.id,
    fromId: p.fromCharacterId,
    fromName: p.fromName,
    toId: p.toCharacterId,
    toName: p.toName,
    receiverHas: p.receiverHas,
  };
}

/**
 * The only client of `FavorService` (the master's Inspiração, RN-43): the read, the grant, the ask to pass it on and the answers to
 * it, and the answer to the refund question. Callers map a refusal with `favorBlocked` (`favor-errors.ts`). `providedIn: 'root'`.
 */
@Injectable({ providedIn: 'root' })
export class FavorClient {
  private readonly client = createClient(FavorService, inject(CONNECT_TRANSPORT));

  /** Who has it, the asks that wait and the refund questions: the master reads every holder and ask of the campaign and the questions, a
   * player only their own characters and their own asks (RN-10). */
  async read(campaignId: string): Promise<FavorReadVm> {
    const response = await this.client.getFavor({ campaignId });
    return {
      holders: response.holders.map((h) => ({ characterId: h.characterId, name: h.characterName })),
      passes: response.passes
        .filter((p) => p.status === FavorPassStatus.PENDING)
        .map((p) => passOf(p)),
      refunds: response.refundPrompts
        .filter((r) => r.status === FavorRefundStatus.OPEN)
        .map((r) => ({
          id: r.id,
          characterId: r.characterId,
          characterName: r.characterName,
          rollLabel: r.rollLabelPt,
          hidden: r.hiddenSources.map((s) => ({
            name: sourceName(s.textPt),
            advantage: s.effect === RollMode.ADVANTAGE,
          })),
        })),
    };
  }

  /** The master's grant, in an open session. `idempotencyKey` is one per tap: a retry of the same tap answers as the first. */
  async grant(campaignId: string, characterId: string, idempotencyKey: string): Promise<void> {
    await this.client.grantFavor({ campaignId, characterId, idempotencyKey });
  }

  /** The holder's ask to pass the Inspiração to another player character, in an open session. Nothing moves until the master answers. */
  async pass(
    campaignId: string,
    fromCharacterId: string,
    toCharacterId: string,
    idempotencyKey: string,
  ): Promise<FavorPassVm | null> {
    const res = await this.client.passFavor({
      campaignId,
      fromCharacterId,
      toCharacterId,
      idempotencyKey,
    });
    return res.pass ? passOf(res.pass) : null;
  }

  /** The master's answer to an ask: accept moves the Inspiração, refuse keeps it with the giver. */
  async answerPass(
    campaignId: string,
    passId: string,
    accept: boolean,
    idempotencyKey: string,
  ): Promise<void> {
    await this.client.answerFavorPass({ campaignId, passId, accept, idempotencyKey });
  }

  /** The giver gives the ask up. */
  async cancelPass(campaignId: string, passId: string, idempotencyKey: string): Promise<void> {
    await this.client.cancelFavorPass({ campaignId, passId, idempotencyKey });
  }

  /** The master's answer to the refund question: give the Inspiração back, or keep it spent. */
  async answerRefund(
    campaignId: string,
    promptId: string,
    giveBack: boolean,
    idempotencyKey: string,
  ): Promise<void> {
    await this.client.answerFavorRefund({ campaignId, promptId, giveBack, idempotencyKey });
  }
}
