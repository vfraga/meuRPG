import { Injectable, inject } from '@angular/core';
import { createClient } from '@connectrpc/connect';

import { PlayService } from '../../../gen/meurpg/play/v1/play_pb';
import {
  type ApplyTrapDamageResponse,
  type DiscardTrapDamageResponse,
  type FireTrapResponse,
  type ListTrapActivityResponse,
  type ListTrapDamagesResponse,
  type SearchForTrapsResponse,
  TrapSearchSkill,
} from '../../../gen/meurpg/play/v1/traps_pb';
import { CONNECT_TRANSPORT } from '../connect/transport';

/** How the search's d20 comes (RN-18): the app rolls it, or the player typed the face of a real
 * die; for a Perception search with disadvantage a second face goes with it. */
export type SearchDie =
  { readonly inApp: true } | { readonly face: number; readonly face2?: number };

/** The skill a search rolls: the two that always find, or `other` (any other skill of the sheet, named by the key sent with it). */
export type SearchSkill = 'perception' | 'investigation' | 'other';

const TRAP_SKILL: Readonly<Record<SearchSkill, TrapSearchSkill>> = {
  perception: TrapSearchSkill.PERCEPTION,
  investigation: TrapSearchSkill.INVESTIGATION,
  other: TrapSearchSkill.OTHER,
};

/**
 * Thin wrapper around the trap calls of `PlayService` (MR-035, traps.proto): the player's search, the
 * master's firing and the damage that waits. Callers map errors to Portuguese (`trapErrorMessage`).
 * `providedIn: 'root'`, imported only by lazy code.
 */
@Injectable({ providedIn: 'root' })
export class TrapsClient {
  private readonly client = createClient(PlayService, inject(CONNECT_TRANSPORT));

  /** "Procurar armadilhas". The key is made once per search and sent again on a retry. */
  search(
    campaignId: string,
    skill: SearchSkill,
    die: SearchDie,
    idempotencyKey: string,
    otherSkillKey = '',
    extraDieFaces: readonly number[] = [],
  ): Promise<SearchForTrapsResponse> {
    return this.client.searchForTraps({
      campaignId,
      skill: TRAP_SKILL[skill],
      // Only with `other`: the server refuses a key without it, and the two that always find.
      otherSkillKey: skill === 'other' ? otherSkillKey : '',
      idempotencyKey,
      roll:
        'inApp' in die || die.face2 !== undefined
          ? 'inApp' in die
            ? { case: 'rollInApp', value: true }
            : { case: undefined }
          : { case: 'd20Face', value: die.face },
      d20Faces: 'inApp' in die || die.face2 === undefined ? [] : [die.face, die.face2],
      extraDieFaces: [...extraDieFaces],
    });
  }

  /** `FireTrap`: empty `targetIds` is whoever stands in the area (the server decides). With
   * `extendFiringId` the creatures are added to a firing already made. */
  fire(
    campaignId: string,
    mapId: string,
    pointId: string,
    targetIds: readonly string[],
    idempotencyKey: string,
    extendFiringId = '',
  ): Promise<FireTrapResponse> {
    return this.client.fireTrap({
      campaignId,
      mapId,
      pointId,
      targetIds: [...targetIds],
      idempotencyKey,
      extendFiringId,
    });
  }

  /** The firings, searches and notices of the open session outside a combat, as the caller reads them. */
  activity(campaignId: string): Promise<ListTrapActivityResponse> {
    return this.client.listTrapActivity({ campaignId });
  }

  /** The master's damages that wait, in a combat or outside one. */
  damages(campaignId: string): Promise<ListTrapDamagesResponse> {
    return this.client.listTrapDamages({ campaignId });
  }

  /** Outside a combat. In a combat the damage is a pending one: `CombatClient.attacks.applyDamage`. */
  applyDamage(
    campaignId: string,
    id: string,
    amount: number | undefined,
    idempotencyKey: string,
  ): Promise<ApplyTrapDamageResponse> {
    return this.client.applyTrapDamage({ campaignId, trapDamageId: id, amount, idempotencyKey });
  }

  discardDamage(
    campaignId: string,
    id: string,
    idempotencyKey: string,
  ): Promise<DiscardTrapDamageResponse> {
    return this.client.discardTrapDamage({ campaignId, trapDamageId: id, idempotencyKey });
  }
}
